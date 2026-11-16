package feedback

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hehehai/actnowkit/family"
	"github.com/hehehai/actnowkit/redact"
)

// Schemas of the files in Config.Dir.
const (
	SubmissionKind = "feedback-submission"
	HistoryKind    = "feedback-history"
)

// OutboxMaxAge: a submission not sent after this long is dropped.
const OutboxMaxAge = 7 * 24 * time.Hour

// Config of a Manager.
type Config struct {
	// App is the family id ("dicta"); Version, Build (commit) and Channel
	// ("release", "beta") of the running app.
	App, Version, Build, Channel string
	// BaseURL of the site the app belongs to: "https://actnow.dev", or
	// "https://refto.one" for Refto.
	BaseURL string
	// WebPath of the site's feedback form ("" = "/feedback"; Refto:
	// "/app/feedback").
	WebPath string
	// Dir holds the outbox and the history (the app's data folder +
	// "/feedback"); created 0700.
	Dir string
	// HTTPClient: the app's client to its site (its proxy rule); nil = a
	// default client.
	HTTPClient *http.Client
	// LicenceToken returns the licence token of this Mac ("" for none):
	// the site answers its code's owner. Optional.
	LicenceToken func() string
	// DeviceID returns this Mac's device ID for the app (licence.DeviceID).
	// Optional.
	DeviceID func() string
	// Locale returns the app's language ("zh-Hans", "en"). Optional.
	Locale func() string
	// LicenceSummary returns the licence's edition and state for the
	// diagnostics ("paid, valid"); never the code. Optional.
	LicenceSummary func() string
	// Collect says what the diagnostics include.
	Collect CollectConfig
	// Converter for images and videos (nil: SystemConverter()).
	Converter Converter
	// Log receives one line per step and every failure. Optional.
	Log func(string)
	// Now and Sleep (tests).
	Now   func() time.Time
	Sleep func(context.Context, time.Duration) error
}

// Phase of the Manager's state.
type Phase string

const (
	Idle      Phase = "idle"
	Preparing Phase = "preparing" // re-encoding the files, collecting diagnostics
	Sending   Phase = "sending"
	Sent      Phase = "sent"
	Failed    Phase = "failed"
)

// State is what the feedback UI shows.
type State struct {
	Phase Phase
	// Current: the submission being prepared or sent (or just sent, or
	// failed).
	Current *Submission
	// Sent and Total bytes while Sending.
	SentBytes, TotalBytes int64
	// Step: "prepare", "create", "upload", "complete".
	Step string
	Err  error
	// Pending: submissions of the outbox not sent (failed ones too).
	Pending []Submission
}

// Fraction of the upload (0–1).
func (s State) Fraction() float64 {
	if s.TotalBytes <= 0 {
		return 0
	}
	return float64(s.SentBytes) / float64(s.TotalBytes)
}

// Manager prepares, sends and remembers feedback.
type Manager struct {
	cfg    Config
	client *Client

	mu    sync.Mutex
	state State
	subs  map[int]func(State)
	next  int
	who   *Identity

	sendMu sync.Mutex // one send at a time
	histMu sync.Mutex
	hist   []HistoryEntry // history.json in memory (histMu)
}

// New makes a Manager; it touches neither the disk nor the network.
func New(cfg Config) (*Manager, error) {
	if cfg.App == "" || cfg.Dir == "" {
		return nil, errors.New("feedback: Config needs App and Dir")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://actnow.dev"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if err := checkBase(cfg.BaseURL); err != nil {
		return nil, err
	}
	if cfg.WebPath == "" {
		cfg.WebPath = "/feedback"
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Sleep == nil {
		cfg.Sleep = func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		}
	}
	if cfg.Converter == nil {
		cfg.Converter = SystemConverter()
	}
	c := &Client{BaseURL: cfg.BaseURL, App: cfg.App, Version: cfg.Version}
	c.HTTP = cfg.HTTPClient
	return &Manager{cfg: cfg, client: c, state: State{Phase: Idle}, subs: map[int]func(State){}}, nil
}

// Client is the protocol client the Manager uses.
func (m *Manager) Client() *Client { return m.client }

func (m *Manager) logf(format string, args ...any) {
	if m.cfg.Log != nil {
		m.cfg.Log(redact.Text(fmt.Sprintf("feedback: "+format, args...)))
	}
}

// State now.
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Subscribe calls fn on every change (from the goroutine making it).
func (m *Manager) Subscribe(fn func(State)) (unsubscribe func()) {
	m.mu.Lock()
	id := m.next
	m.next++
	m.subs[id] = fn
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		delete(m.subs, id)
		m.mu.Unlock()
	}
}

func (m *Manager) set(update func(*State)) {
	m.mu.Lock()
	update(&m.state)
	st := m.state
	fns := make([]func(State), 0, len(m.subs))
	for _, fn := range m.subs {
		fns = append(fns, fn)
	}
	m.mu.Unlock()
	for _, fn := range fns {
		fn(st)
	}
}

// Identity asks the site whether the email is optional (cached for the
// Manager's life after a successful answer). On a failure the answer is
// Identity{} (the email is required) and the error.
func (m *Manager) Identity(ctx context.Context) (Identity, error) {
	m.mu.Lock()
	if m.who != nil {
		who := *m.who
		m.mu.Unlock()
		return who, nil
	}
	m.mu.Unlock()
	tok := m.licenceToken()
	if tok == "" {
		who := Identity{Known: true}
		return who, nil
	}
	who, err := m.client.Identity(ctx, tok)
	if err != nil {
		m.logf("identity: %v", err)
		return Identity{}, err
	}
	m.mu.Lock()
	m.who = &who
	m.mu.Unlock()
	return who, nil
}

// ForgetIdentity drops the cached answer: call it when the licence
// changes (activated, removed), else the old answer stays for the
// Manager's life.
func (m *Manager) ForgetIdentity() {
	m.mu.Lock()
	m.who = nil
	m.mu.Unlock()
}

func (m *Manager) licenceToken() string {
	if m.cfg.LicenceToken == nil {
		return ""
	}
	return m.cfg.LicenceToken()
}

func (m *Manager) locale() string {
	if m.cfg.Locale == nil {
		return ""
	}
	return m.cfg.Locale()
}

// Diagnostics collects what a submission's diagnostics.json would hold
// now (the form's "See what is sent"); email is the address typed in the
// form, left unmasked.
func (m *Manager) Diagnostics(email string) Diagnostics {
	cc := m.cfg.Collect
	if cc.Personal == nil {
		p := redact.ThisUser()
		cc.Personal = &p
	}
	if email = strings.TrimSpace(email); email != "" {
		p := *cc.Personal
		p.KeepEmails = append(append([]string(nil), p.KeepEmails...), email)
		cc.Personal = &p
	}
	app := AppInfo{ID: m.cfg.App, Version: m.cfg.Version, Build: m.cfg.Build, Channel: m.cfg.Channel}
	if m.cfg.LicenceSummary != nil {
		app.Licensed = m.cfg.LicenceSummary()
	}
	d := Collect(cc, app, m.cfg.Now())
	d.System.Locale = m.locale()
	return d
}

func newLocalID(now time.Time) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return now.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b[:])
}

func (m *Manager) outboxDir() string { return filepath.Join(m.cfg.Dir, "outbox") }

// Submit prepares the draft (re-encodes the images, fits the video,
// collects the diagnostics), saves it to the outbox and sends it. It
// blocks until sent or failed: run it off the UI thread. A failed send
// stays in the outbox (State.Pending) for Retry or Resume.
func (m *Manager) Submit(ctx context.Context, d Draft) (*Submission, error) {
	who, _ := m.Identity(ctx)
	if ps := Validate(d, who); len(ps) > 0 {
		return nil, fmt.Errorf("feedback: the form is not complete: %v", ps)
	}
	now := m.cfg.Now()
	sub := &Submission{
		Schema: family.SchemaOf(SubmissionKind, 1), LocalID: newLocalID(now), CreatedAt: now.UTC(),
		App: m.cfg.App, Version: m.cfg.Version, Build: m.cfg.Build, Channel: m.cfg.Channel, Locale: m.locale(),
		Kind: d.Kind, Body: strings.TrimSpace(d.Body), Email: strings.TrimSpace(d.Email), State: SendQueued,
	}
	m.set(func(s *State) { *s = State{Phase: Preparing, Step: "prepare", Current: sub, Pending: s.Pending} })
	dir := filepath.Join(m.outboxDir(), sub.LocalID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, m.fail(sub, "prepare", err)
	}
	if err := m.prepare(ctx, sub, d, dir); err != nil {
		os.RemoveAll(dir)
		return nil, m.fail(sub, "prepare", err)
	}
	if err := m.save(sub); err != nil {
		os.RemoveAll(dir)
		return nil, m.fail(sub, "prepare", err)
	}
	return sub, m.send(ctx, sub)
}

func (m *Manager) prepare(ctx context.Context, sub *Submission, d Draft, dir string) error {
	sys := ThisSystem()
	sys.Locale = sub.Locale
	sub.System = sys
	for i, p := range d.Images {
		f, err := PrepareImage(ctx, m.cfg.Converter, p, dir, fmt.Sprintf("image-%d", i+1))
		if err != nil {
			return err
		}
		sub.Files = append(sub.Files, f)
	}
	if d.Video != "" {
		f, err := PrepareVideo(ctx, m.cfg.Converter, d.Video, dir, "video")
		if err != nil {
			return err
		}
		sub.Files = append(sub.Files, f)
	}
	if d.Diagnostics {
		diag := m.Diagnostics(sub.Email)
		data, err := json.Marshal(diag)
		if err != nil {
			return err
		}
		if err := family.WriteFileAtomic(filepath.Join(dir, "diagnostics.json"), data, 0o600); err != nil {
			return err
		}
		n, sum, err := hashFile(filepath.Join(dir, "diagnostics.json"))
		if err != nil {
			return err
		}
		sub.Files = append(sub.Files, File{Ref: "diagnostics", Role: RoleDiagnostics, Name: "diagnostics.json", MIME: "application/json", Bytes: n, SHA256: sum})
	}
	return nil
}

func (m *Manager) save(sub *Submission) error {
	data, err := family.MarshalIndent(sub)
	if err != nil {
		return err
	}
	return family.WriteFileAtomic(filepath.Join(m.outboxDir(), sub.LocalID, "submission.json"), data, 0o600)
}

func (m *Manager) fail(sub *Submission, step string, err error) error {
	m.logf("%s %s: %v", sub.LocalID, step, err)
	pending := m.loadPending()
	m.set(func(s *State) {
		*s = State{Phase: Failed, Current: sub, Step: step, Err: err, Pending: pending}
	})
	return err
}

// retryDelays between the attempts of one send.
var retryDelays = []time.Duration{2 * time.Second, 8 * time.Second, 30 * time.Second}

// send runs the steps with retries; it saves the submission after each.
func (m *Manager) send(ctx context.Context, sub *Submission) error {
	m.sendMu.Lock()
	defer m.sendMu.Unlock()
	var err error
	for attempt := 0; ; attempt++ {
		sub.Attempts++
		err = m.sendOnce(ctx, sub)
		if err == nil {
			return nil
		}
		var api *APIError
		if errors.As(err, &api) && !api.Retryable() {
			sub.Rejected, sub.ErrorCode = true, api.Code
			break
		}
		if ctx.Err() != nil || attempt >= len(retryDelays) {
			break
		}
		if api != nil && api.Code == "rate_limited" {
			break // wait for the next Resume
		}
		m.logf("%s: %v; again in %s", sub.LocalID, err, retryDelays[attempt])
		if m.cfg.Sleep(ctx, retryDelays[attempt]) != nil {
			break
		}
	}
	sub.State, sub.LastError = SendFailed, err.Error()
	_ = m.save(sub)
	return m.fail(sub, m.State().Step, err)
}

func (m *Manager) sendOnce(ctx context.Context, sub *Submission) error {
	dir := filepath.Join(m.outboxDir(), sub.LocalID)
	total, stored := sub.Bytes()
	m.set(func(s *State) {
		s.Phase, s.Current, s.Err, s.TotalBytes, s.SentBytes = Sending, sub, nil, total, stored
	})
	step := func(name string) { m.set(func(s *State) { s.Step = name }) }

	var uploads []Upload
	if sub.ID == "" {
		step("create")
		req := CreateRequest{
			App: sub.App, Version: sub.Version, Build: sub.Build, Channel: sub.Channel, Locale: sub.Locale,
			Kind: sub.Kind, Body: sub.Body, Email: sub.Email, LicenceToken: m.licenceToken(), System: sub.System,
		}
		if m.cfg.DeviceID != nil {
			req.DeviceID = m.cfg.DeviceID()
		}
		for _, f := range sub.Files {
			req.Attachments = append(req.Attachments, AttachmentMeta{Ref: f.Ref, Role: f.Role, Name: f.Name, MIME: f.MIME, Bytes: f.Bytes, SHA256: f.SHA256, Width: f.Width, Height: f.Height})
		}
		ans, err := m.client.Create(ctx, req)
		if err != nil {
			return err
		}
		sub.ID, sub.Number, sub.ViewToken, sub.WebURL = ans.ID, ans.Number, ans.ViewToken, ans.WebURL
		uploads = ans.Uploads
		for i := range sub.Files {
			for _, u := range uploads {
				if u.Ref == sub.Files[i].Ref {
					sub.Files[i].AttachmentID = u.AttachmentID
				}
			}
		}
		sub.State = SendUploading
		if err := m.save(sub); err != nil {
			return err
		}
	}
	refreshed := false
	for i := range sub.Files {
		f := &sub.Files[i]
		if f.Stored {
			continue
		}
		step("upload")
		for {
			u, ok := findUpload(uploads, f)
			expiring := ok && !u.ExpiresAt.IsZero() && u.ExpiresAt.Before(m.cfg.Now().Add(time.Minute))
			if (!ok || expiring) && !refreshed {
				fresh, err := m.client.Uploads(ctx, sub.ID, sub.ViewToken)
				if err != nil {
					return err
				}
				uploads, refreshed = fresh, true
				continue
			}
			if !ok {
				return &APIError{HTTPStatus: 409, Code: "upload_missing", Message: "no upload URL for " + f.Ref}
			}
			base := m.State().SentBytes
			err := m.client.Put(ctx, u, sub.ViewToken, filepath.Join(dir, f.Name), *f, func(sent int64) {
				m.set(func(s *State) { s.SentBytes = base + sent })
			})
			var api *APIError
			if errors.As(err, &api) && api.Code == "upload_expired" && !refreshed {
				uploads = nil // ask again
				continue
			}
			if err != nil {
				m.set(func(s *State) { s.SentBytes = base })
				return err
			}
			f.Stored = true
			m.set(func(s *State) { s.SentBytes = base + f.Bytes })
			if err := m.save(sub); err != nil {
				return err
			}
			break
		}
	}
	step("complete")
	ans, err := m.client.Complete(ctx, sub.ID, sub.ViewToken)
	var api *APIError
	if errors.As(err, &api) && api.Code == "upload_missing" {
		var det struct {
			Missing []string `json:"missing"`
		}
		_ = json.Unmarshal(api.Details, &det)
		for i := range sub.Files {
			for _, id := range det.Missing {
				if sub.Files[i].AttachmentID == id {
					sub.Files[i].Stored = false
				}
			}
		}
		_ = m.save(sub)
		return err
	}
	if err != nil {
		return err
	}
	if ans.Number != "" {
		sub.Number = ans.Number
	}
	if ans.WebURL != "" {
		sub.WebURL = ans.WebURL
	}
	sub.State, sub.LastError = SendSent, ""
	status := ans.Status
	if status == "" {
		status = StatusNew
	}
	now := m.cfg.Now().UTC()
	if err := m.remember(HistoryEntry{Item: Item{ID: sub.ID, Number: sub.Number, App: sub.App, Kind: sub.Kind, Status: status,
		Summary: Summary(sub.Body, 80), CreatedAt: sub.CreatedAt, UpdatedAt: now, Messages: 0, WebURL: sub.WebURL}, ViewToken: sub.ViewToken}); err != nil {
		m.logf("history: %v", err)
	}
	os.RemoveAll(dir)
	m.logf("%s sent as %s (%s)", sub.LocalID, sub.Number, sub.ID)
	pending := m.loadPending()
	m.set(func(s *State) { *s = State{Phase: Sent, Current: sub, Step: "complete", Pending: pending} })
	return nil
}

func findUpload(us []Upload, f *File) (Upload, bool) {
	for _, u := range us {
		if (f.AttachmentID != "" && u.AttachmentID == f.AttachmentID) || (f.AttachmentID == "" && u.Ref == f.Ref) {
			return u, true
		}
	}
	return Upload{}, false
}

// loadPending reads the outbox, dropping submissions older than
// OutboxMaxAge and broken entries.
func (m *Manager) loadPending() []Submission {
	ents, _ := os.ReadDir(m.outboxDir())
	var out []Submission
	now := m.cfg.Now()
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(m.outboxDir(), e.Name())
		var sub Submission
		if err := family.Load(filepath.Join(dir, "submission.json"), SubmissionKind, 1, &sub); err != nil {
			if fi, err2 := e.Info(); err2 == nil && now.Sub(fi.ModTime()) > time.Hour {
				os.RemoveAll(dir) // a preparation cut short
			}
			continue
		}
		if sub.State == SendSent || now.Sub(sub.CreatedAt) > OutboxMaxAge {
			m.logf("%s: dropped from the outbox (%s)", sub.LocalID, sub.State)
			os.RemoveAll(dir)
			continue
		}
		out = append(out, sub)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// Pending is the outbox now (read from disk: call it to fill the UI before
// Resume has run; State.Pending follows afterwards).
func (m *Manager) Pending() []Submission { return m.loadPending() }

// Resume sends the submissions a previous run left (not the rejected
// ones). Call it at start-up (in a goroutine) and when the network comes
// back.
func (m *Manager) Resume(ctx context.Context) {
	pending := m.loadPending()
	m.set(func(s *State) { s.Pending = pending })
	for i := range pending {
		sub := pending[i]
		if sub.Rejected {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		_ = m.send(ctx, &sub)
	}
}

// Retry sends one submission of the outbox again (a rejected one too: the
// site may accept it now).
func (m *Manager) Retry(ctx context.Context, localID string) error {
	for _, sub := range m.loadPending() {
		if sub.LocalID == localID {
			sub.Rejected, sub.ErrorCode = false, ""
			return m.send(ctx, &sub)
		}
	}
	return os.ErrNotExist
}

// Discard removes a submission from the outbox.
func (m *Manager) Discard(localID string) error {
	if localID == "" || strings.ContainsAny(localID, `/\`) || strings.HasPrefix(localID, ".") {
		return os.ErrNotExist
	}
	err := os.RemoveAll(filepath.Join(m.outboxDir(), localID))
	pending := m.loadPending()
	m.set(func(s *State) {
		s.Pending = pending
		if s.Current != nil && s.Current.LocalID == localID {
			s.Phase, s.Current, s.Err = Idle, nil, nil
		}
	})
	return err
}

// Reset returns the state to Idle after Sent or Failed (a new form).
func (m *Manager) Reset() {
	m.set(func(s *State) {
		if s.Phase == Sent || s.Phase == Failed {
			*s = State{Phase: Idle, Pending: s.Pending}
		}
	})
}

// WebURL is the site's feedback form for this app (the "Send on the web"
// link): the app, its version and language, nothing else.
func (m *Manager) WebURL() string {
	q := url.Values{}
	q.Set("app", m.cfg.App)
	if m.cfg.Version != "" {
		q.Set("v", m.cfg.Version)
	}
	if l := m.locale(); l != "" {
		q.Set("lang", l)
	}
	return m.cfg.BaseURL + m.cfg.WebPath + "?" + q.Encode()
}
