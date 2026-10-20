// Package updates is the in-app updater of the ActNow apps, on top of
// MyGo's updater API (mygo.Updater.Check, Update.Install, App.Relaunch)
// without MyGo's update window: the apps draw their own panel.
//
// A Manager runs the state machine — idle → checking → available →
// downloading (progress) → ready (restart to finish) or failed — reads the
// release notes in the app's language from actnow.dev's changelog API
// (English where a release has no translation), and marks updates that need
// a paid licence for early-access (whitelist) users: at or above the app's
// paid_since, an early-access code stops working, so the user is told and
// may keep the version they have (docs/licensing.md §7).
//
// actnowkit does not depend on MyGo: the app passes MyGo's updater as
// Funcs (see README).
package updates

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hehehai/actnowkit/licence"
)

// Release is an available update, as MyGo's updater gives it.
type Release struct {
	Version string
	// Notes from the update manifest (English, MyGo's CHANGELOG section).
	Notes string
	Date  time.Time
	// Install downloads, verifies and installs the update (MyGo's
	// Update.Install); the running app is not affected until Restart.
	Install func(ctx context.Context, progress func(done, total int64)) error
}

// Updater is what the Manager needs of MyGo.
type Updater interface {
	// Enabled: the build has updates and can write where it is installed.
	Enabled() bool
	// Check returns the newer release, or nil when up to date.
	Check(ctx context.Context) (*Release, error)
}

// Funcs adapts functions (MyGo's) to Updater.
type Funcs struct {
	EnabledFunc func() bool
	CheckFunc   func(ctx context.Context) (*Release, error)
}

func (f Funcs) Enabled() bool { return f.EnabledFunc == nil || f.EnabledFunc() }

func (f Funcs) Check(ctx context.Context) (*Release, error) { return f.CheckFunc(ctx) }

// Phase of the state machine.
type Phase string

const (
	Idle        Phase = "idle"
	Disabled    Phase = "disabled" // development builds, read-only installs
	Checking    Phase = "checking"
	UpToDate    Phase = "up_to_date"
	Available   Phase = "available"
	Downloading Phase = "downloading"
	Ready       Phase = "ready" // installed: restart to finish
	Failed      Phase = "failed"
)

// State is what the update UI shows.
type State struct {
	Phase Phase
	// Current is the running version.
	Current string
	// Release: the available (or installed, when Ready) update.
	Release *Release
	// Notes of Release in the app's language (Markdown), and its language.
	Notes       string
	NotesLocale string
	// NeedsPaidLicence: the user has an early-access licence and this
	// update is at or above paid_since: after it the code stops working.
	// Show licence.TextUpdateNeedsPaid; Install needs AcceptPaid.
	NeedsPaidLicence bool
	PaidSince        string
	// Downloaded and Total bytes while Downloading.
	Downloaded, Total int64
	// Err and the step it happened in ("check", "download") when Failed.
	Err  error
	Step string
	// CheckedAt: the last check.
	CheckedAt time.Time
}

// Fraction of the download (0–1).
func (s State) Fraction() float64 {
	if s.Total <= 0 {
		return 0
	}
	return float64(s.Downloaded) / float64(s.Total)
}

// Config of a Manager.
type Config struct {
	Updater Updater
	// Current version of the app.
	Current string
	// History reads the changelog API (nil: manifest notes only).
	History *HistoryClient
	// Language: the app's language now ("zh-Hans", "en"); called at each
	// check, so a language change applies to the next one.
	Language func() string
	// Beta: the user gets beta updates (the history includes betas).
	Beta func() bool
	// Licence: the app's licence state (nil for free apps): its edition
	// and paid_since decide NeedsPaidLicence.
	Licence func() licence.Status
	// Restart relaunches the app (mygo.App.Relaunch).
	Restart func()
	// Log receives one line per step and every failure.
	Log func(string)
	// Now: time.Now.
	Now func() time.Time
}

// Manager runs the update state machine. Safe for concurrent use.
type Manager struct {
	cfg Config

	mu      sync.Mutex
	state   State
	subs    map[int]func(State)
	nextSub int
	// history: the last check's changelog (LastHistory).
	history *History
}

// LastHistory is the changelog fetched by the last check (nil if none).
func (m *Manager) LastHistory() *History {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.history
}

// ErrNeedsPaidLicence: Install of an update that ends an early-access
// licence without AcceptPaid.
var ErrNeedsPaidLicence = errors.New("updates: this update needs a paid licence")

// ErrBusy: a check or an install is running.
var ErrBusy = errors.New("updates: busy")

// New makes a Manager; Disabled when the updater is.
func New(cfg Config) *Manager {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Log == nil {
		cfg.Log = func(string) {}
	}
	m := &Manager{cfg: cfg, subs: map[int]func(State){}}
	m.state = State{Phase: Idle, Current: cfg.Current}
	if cfg.Updater == nil || !cfg.Updater.Enabled() {
		m.state.Phase = Disabled
	}
	return m
}

// State now.
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Subscribe calls fn with every new state (from any goroutine: the app
// moves it to its UI thread); the returned func stops it.
func (m *Manager) Subscribe(fn func(State)) (unsubscribe func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextSub
	m.nextSub++
	m.subs[id] = fn
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.subs, id)
	}
}

func (m *Manager) set(update func(*State)) State {
	m.mu.Lock()
	update(&m.state)
	st := m.state
	subs := make([]func(State), 0, len(m.subs))
	for _, fn := range m.subs {
		subs = append(subs, fn)
	}
	m.mu.Unlock()
	for _, fn := range subs {
		fn(st)
	}
	return st
}

func (m *Manager) language() string {
	if m.cfg.Language == nil {
		return "en"
	}
	return m.cfg.Language()
}

func (m *Manager) beta() bool { return m.cfg.Beta != nil && m.cfg.Beta() }

// Check asks the feed for an update and, when there is one, its notes in
// the app's language and whether it needs a paid licence.
func (m *Manager) Check(ctx context.Context) State {
	m.mu.Lock()
	switch m.state.Phase {
	case Disabled:
		st := m.state
		m.mu.Unlock()
		return st
	case Checking, Downloading:
		st := m.state
		m.mu.Unlock()
		return st
	case Ready:
		// Installed already: restart to finish.
		st := m.state
		m.mu.Unlock()
		return st
	}
	m.mu.Unlock()
	m.set(func(s *State) { s.Phase, s.Err, s.Step = Checking, nil, "" })
	m.cfg.Log("checking for updates")

	release, err := m.cfg.Updater.Check(ctx)
	now := m.cfg.Now()
	if err != nil {
		m.cfg.Log("check failed: " + err.Error())
		return m.set(func(s *State) { s.Phase, s.Err, s.Step, s.CheckedAt = Failed, err, "check", now })
	}
	if release == nil {
		m.cfg.Log("up to date: " + m.cfg.Current)
		return m.set(func(s *State) { s.Phase, s.Release, s.CheckedAt = UpToDate, nil, now })
	}

	notes, locale := release.Notes, "en"
	var hist *History
	if m.cfg.History != nil {
		h, herr := m.cfg.History.Fetch(ctx, m.language(), m.beta(), 0)
		if herr != nil {
			m.cfg.Log("changelog: " + herr.Error())
		} else {
			hist = h
			if e := h.Find(release.Version); e != nil && strings.TrimSpace(e.Notes) != "" {
				notes, locale = e.Notes, e.Locale
			}
		}
	}
	m.mu.Lock()
	m.history = hist
	m.mu.Unlock()

	needsPaid, paidSince := m.needsPaid(release.Version, hist)
	m.cfg.Log(fmt.Sprintf("update available: %s → %s (notes %s, needs paid licence %v)", m.cfg.Current, release.Version, locale, needsPaid))
	return m.set(func(s *State) {
		s.Phase, s.Release, s.Notes, s.NotesLocale = Available, release, notes, locale
		s.NeedsPaidLicence, s.PaidSince, s.CheckedAt = needsPaid, paidSince, now
	})
}

// needsPaid: an early-access licence and a version at or above paid_since
// (the site's, from the history or the licence token).
func (m *Manager) needsPaid(version string, hist *History) (bool, string) {
	if m.cfg.Licence == nil {
		return false, ""
	}
	st := m.cfg.Licence()
	if st.Edition() != licence.EditionWhitelist {
		return false, ""
	}
	paidSince := hist.PaidSinceVersion()
	if paidSince == "" {
		paidSince = st.PaidSince
	}
	if e := hist.Find(version); e != nil && e.RequiresPaidLicence {
		return true, paidSince
	}
	return licence.WhitelistBlocked(version, paidSince), paidSince
}

// Install downloads and installs the available update (progress in
// State.Downloaded/Total). An update that needs a paid licence for an
// early-access user installs only with acceptPaid. On failure the app is
// unchanged (MyGo puts the old bundle back if the swap fails) and the
// State is Failed; Check again to retry.
func (m *Manager) Install(ctx context.Context, acceptPaid bool) State {
	m.mu.Lock()
	st := m.state
	if st.Phase != Available || st.Release == nil {
		m.mu.Unlock()
		return st
	}
	if st.NeedsPaidLicence && !acceptPaid {
		m.mu.Unlock()
		return m.set(func(s *State) { s.Err = ErrNeedsPaidLicence })
	}
	m.mu.Unlock()
	release := st.Release
	m.cfg.Log("downloading " + release.Version)
	m.set(func(s *State) { s.Phase, s.Downloaded, s.Total, s.Err, s.Step = Downloading, 0, 0, nil, "" })

	var last time.Time
	err := release.Install(ctx, func(done, total int64) {
		// At most ~10 updates a second to the UI.
		now := time.Now()
		if done < total && now.Sub(last) < 100*time.Millisecond {
			return
		}
		last = now
		m.set(func(s *State) { s.Downloaded, s.Total = done, total })
	})
	if err != nil {
		m.cfg.Log("install failed (the app is unchanged): " + err.Error())
		return m.set(func(s *State) { s.Phase, s.Err, s.Step = Failed, err, "download" })
	}
	m.cfg.Log("installed " + release.Version + ": restart to finish")
	return m.set(func(s *State) { s.Phase = Ready })
}

// Restart relaunches the app into the installed update (Ready only).
func (m *Manager) Restart() error {
	if m.State().Phase != Ready {
		return errors.New("updates: nothing installed")
	}
	if m.cfg.Restart == nil {
		return errors.New("updates: no Restart in the config")
	}
	m.cfg.Log("restarting into the update")
	m.cfg.Restart()
	return nil
}

// Dismiss returns to Idle from Available, UpToDate or Failed (the user
// closed the panel or chose to keep the version they have).
func (m *Manager) Dismiss() {
	m.set(func(s *State) {
		switch s.Phase {
		case Available, UpToDate, Failed:
			s.Phase, s.Err, s.Step = Idle, nil, ""
		}
	})
}

// History is the changelog in the app's language (Settings → About →
// Changelog): fetched at each call.
func (m *Manager) History(ctx context.Context) (*History, error) {
	if m.cfg.History == nil {
		return nil, errors.New("updates: no history client")
	}
	return m.cfg.History.Fetch(ctx, m.language(), m.beta(), 200)
}

// AutoCheck checks now and then every interval (24 h by default) while
// enabled() says so (the app's "check automatically" setting), until ctx
// ends. A check is skipped while one runs or an update is downloading.
func (m *Manager) AutoCheck(ctx context.Context, interval time.Duration, enabled func() bool) {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	go func() {
		for {
			if enabled == nil || enabled() {
				m.Check(ctx)
			}
			t := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}
		}
	}()
}
