package licence

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"time"

	"github.com/hehehai/actnowkit/credentials"
)

// TrialLength is the free trial of a commercial app: per app and per Mac,
// from the first launch, without a code.
const TrialLength = 15 * 24 * time.Hour

// EditionTrial is the edition of a trial token (ed = "trial").
const EditionTrial = "trial"

// Trial states (docs/licensing.md §15, trial part). They are taken only
// when no valid code is stored: a valid code always wins.
const (
	// TrialValid: a trial token is stored and valid (Reason "" when not due;
	// "due" or "offline" when a refresh is wanted; "clock" when the clock
	// went back and the server must confirm).
	TrialValid State = "trial_valid"
	// TrialLocal: no trial token yet (offline first launch, or not
	// confirmed): the Mac's own record says the trial is within its 15 days.
	// Reason ReasonUnconfirmed. Usable.
	TrialLocal State = "trial_local"
	// TrialExpired: the trial is over (endsAt reached, the Mac's record is
	// older than 15 days, or the server answered trial_expired). Reason
	// ReasonTrialEnded. Not usable; the data stays.
	TrialExpired State = "trial_expired"
)

// Trial reasons.
const (
	ReasonUnconfirmed      = "unconfirmed"
	ReasonTrialEnded       = "trial_ended"
	ReasonTrialUnavailable = "trial_unavailable"
)

// TrialRecord is what the Mac keeps about its trial: the first launch
// (StartedAt, Unix seconds) and the latest time seen (LastSeen, the same
// now* rule as the code's Record). ServerEnded is the server's last answer
// (trial_expired), which is final for this Mac.
type TrialRecord struct {
	StartedAt   int64 `json:"startedAt"`
	LastSeen    int64 `json:"lastSeen,omitempty"`
	ServerEnded bool  `json:"serverEnded,omitempty"`
}

// ErrNoTrial: no trial record stored (the first launch has not happened).
var ErrNoTrial = errors.New("licence: no trial record")

// TrialPath is the site's trial endpoint.
const TrialPath = "/api/licence/v1/trial"

// TrialService is the Keychain service of the trial records; the account is
// the app's slug.
const TrialService = "actnow.trial"

// TrialStore keeps the TrialRecord of one app. LoadTrial returns ErrNoTrial
// when there is none. It must survive uninstalling the app (Keychain).
type TrialStore interface {
	LoadTrial() (TrialRecord, error)
	SaveTrial(TrialRecord) error
}

// KeychainTrialStore keeps the trial record in the login Keychain, under
// service "actnow.trial" and the app's slug as account.
type KeychainTrialStore struct {
	Keychain credentials.Keychain
	Account  string
}

// NewKeychainTrialStore is the trial store of an app (its slug, e.g.
// "act").
func NewKeychainTrialStore(app string) *KeychainTrialStore {
	return &KeychainTrialStore{Keychain: credentials.Keychain{Service: TrialService}, Account: app}
}

func (s *KeychainTrialStore) LoadTrial() (TrialRecord, error) {
	raw, err := s.Keychain.Get(s.Account)
	if errors.Is(err, credentials.ErrNotFound) {
		return TrialRecord{}, ErrNoTrial
	}
	if err != nil {
		return TrialRecord{}, err
	}
	var r TrialRecord
	if err := json.Unmarshal([]byte(raw), &r); err != nil || r.StartedAt <= 0 {
		return TrialRecord{}, ErrNoTrial
	}
	return r, nil
}

func (s *KeychainTrialStore) SaveTrial(r TrialRecord) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return s.Keychain.Set(s.Account, "ActNow trial", string(b))
}

// MemoryTrialStore keeps the trial record in memory (tests, previews).
type MemoryTrialStore struct {
	rec *TrialRecord
}

func (m *MemoryTrialStore) LoadTrial() (TrialRecord, error) {
	if m.rec == nil {
		return TrialRecord{}, ErrNoTrial
	}
	return *m.rec, nil
}

func (m *MemoryTrialStore) SaveTrial(r TrialRecord) error {
	m.rec = &r
	return nil
}

// trialRecordLocked returns the Mac's trial record, writing the first launch
// (now) when there is none. changed is true when it was written or the
// caller must save it. A store that cannot be read is not overwritten: the
// trial then starts from now for this run only.
func (c *Client) trialRecordLocked(now time.Time) (TrialRecord, bool) {
	tr, err := c.cfg.TrialStore.LoadTrial()
	switch {
	case err == nil:
		return tr, false
	case errors.Is(err, ErrNoTrial):
		tr = TrialRecord{StartedAt: now.Unix()}
		_ = c.cfg.TrialStore.SaveTrial(tr) // retried at the next launch if it fails
		return tr, true
	default:
		return TrialRecord{StartedAt: now.Unix()}, false
	}
}

// isTrialRaw reports whether raw is a valid trial token (ed = trial).
func isTrialRaw(raw string, keys map[string]ed25519.PublicKey) bool {
	if raw == "" {
		return false
	}
	tok, err := ParseToken(raw, keys)
	return err == nil && tok.Edition == EditionTrial
}

// IsTrial reports whether the state is one of the trial states.
func (s Status) IsTrial() bool {
	return s.State == TrialValid || s.State == TrialLocal || s.State == TrialExpired
}

// Licensed reports whether a valid code (not a trial) is in use.
func (s Status) Licensed() bool {
	return s.Token != nil && s.Token.Edition != EditionTrial &&
		(s.State == Valid || s.State == NeedsRefresh || s.State == Grace)
}

// needsTrialCall: the trial wants the server (a trial token, or confirmation
// after an offline start, a refresh, or a clock rollback).
func (s Status) needsTrialCall() bool {
	switch s.State {
	case TrialLocal:
		return true
	case TrialValid:
		return s.Reason == ReasonDue || s.Reason == ReasonOffline || s.Reason == ReasonClock
	}
	return false
}

// localTrial decides the trial from the Mac's own record (no token). The
// time used is now* = max(local, lastSeen), so turning the clock back does
// not extend the trial. changed: LastSeen moved and must be saved.
func localTrial(tr *TrialRecord, offset int64, now time.Time) (Status, bool) {
	startedAt := time.Unix(tr.StartedAt, 0)
	endsAt := startedAt.Add(TrialLength)
	nowStar := now.Add(time.Duration(offset) * time.Second)
	changed := false
	if tr.LastSeen > 0 && nowStar.Before(time.Unix(tr.LastSeen, 0)) {
		nowStar = time.Unix(tr.LastSeen, 0)
	}
	if nowStar.Unix() > tr.LastSeen {
		tr.LastSeen = nowStar.Unix()
		changed = true
	}
	st := Status{TrialStartedAt: startedAt, TrialEndsAt: endsAt}
	if tr.ServerEnded || !nowStar.Before(endsAt) {
		st.State, st.Reason = TrialExpired, ReasonTrialEnded
	} else {
		st.State, st.Reason, st.Usable = TrialLocal, ReasonUnconfirmed, true
	}
	return st, changed
}

// trialFromToken maps the evaluation of a stored trial token to a trial
// state.
func trialFromToken(ev Status, startedAt int64) Status {
	st := ev
	st.TrialStartedAt = time.Unix(startedAt, 0)
	st.TrialEndsAt = ev.CodeExpires
	switch ev.State {
	case Valid:
		st.State, st.Reason = TrialValid, ""
	case NeedsRefresh:
		st.State = TrialValid
	case Grace:
		st.State, st.Reason = TrialValid, ReasonOffline
	default:
		st.State, st.Reason, st.Usable = TrialExpired, ReasonTrialEnded, false
	}
	return st
}

// StartTrial asks the site for this Mac's trial token (POST
// /api/licence/trial) when it is wanted: no valid code, and no trial token
// that is current. The first launch is kept offline; a server answer
// replaces it (startedAt and endsAt of the server). trial_expired is final
// for the Mac. A network failure returns the state (trial_local, or the
// valid trial's grace) and the error. Check calls it.
func (c *Client) StartTrial(ctx context.Context) (Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.statusLocked()
	if !c.cfg.Trial || !st.needsTrialCall() {
		return st, nil
	}
	device, err := c.cfg.DeviceID()
	if err != nil {
		return st, err
	}
	tr, _ := c.trialRecordLocked(c.cfg.Now())
	var answer struct {
		Token string `json:"token"`
		Trial struct {
			StartedAt time.Time `json:"startedAt"`
			EndsAt    time.Time `json:"endsAt"`
		} `json:"trial"`
		ServerTime time.Time `json:"serverTime"`
	}
	err = c.postPath(ctx, TrialPath, map[string]any{
		"app":       c.cfg.App,
		"dev":       device,
		"version":   c.cfg.Version,
		"startedAt": time.Unix(tr.StartedAt, 0).UTC().Format(time.RFC3339),
	}, &answer)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) && api.Code == "trial_unavailable" {
			// Trial switched off on the site: locked for this run, asked again
			// at the next launch (not final, unlike trial_expired).
			c.trialUnavailable = true
			return c.statusLocked(), nil
		}
		if errors.As(err, &api) && api.Code == "trial_expired" {
			var d struct {
				StartedAt time.Time `json:"startedAt"`
			}
			// The site's error body carries it in "details" (actnow.dev) or
			// "data" (either is read).
			raw := api.Details
			if len(api.Data) > 0 {
				raw = api.Data
			}
			if json.Unmarshal(raw, &d) == nil && !d.StartedAt.IsZero() {
				tr.StartedAt = d.StartedAt.Unix()
			}
			tr.ServerEnded = true
			_ = c.cfg.TrialStore.SaveTrial(tr)
			// Drop the trial token: the server has ended it.
			if rec, lerr := c.cfg.Store.Load(); lerr == nil && isTrialRaw(rec.Token, c.cfg.Keys) {
				rec.Token, rec.LastError = "", ""
				_ = c.cfg.Store.Save(rec)
			}
			return c.statusLocked(), nil
		}
		// Keep the error for the state (grace of a stored trial token).
		if rec, lerr := c.cfg.Store.Load(); lerr == nil && isTrialRaw(rec.Token, c.cfg.Keys) {
			rec.LastError = err.Error()
			rec.CheckedAt = c.cfg.Now().Unix()
			_ = c.cfg.Store.Save(rec)
		}
		return c.statusLocked(), err
	}
	tok, err := ParseToken(answer.Token, c.cfg.Keys)
	if err != nil {
		return c.statusLocked(), err
	}
	if tok.Edition != EditionTrial || tok.App != c.cfg.App || tok.Device != device {
		return c.statusLocked(), errors.New("licence: the trial answer is not for this Mac")
	}
	local := c.cfg.Now()
	server := answer.ServerTime
	if server.IsZero() {
		server = tok.Issued()
	}
	rec := Record{Token: tok.Raw, LastSeen: server.Unix(), Offset: offsetFor(server, local), CheckedAt: local.Unix()}
	if err := c.cfg.Store.Save(rec); err != nil {
		return c.statusLocked(), err
	}
	if !answer.Trial.StartedAt.IsZero() {
		tr.StartedAt = answer.Trial.StartedAt.Unix()
	}
	tr.ServerEnded = false
	if rec.LastSeen > tr.LastSeen {
		tr.LastSeen = rec.LastSeen
	}
	_ = c.cfg.TrialStore.SaveTrial(tr)
	return c.statusLocked(), nil
}

// offsetFor is serverTime − local time, 0 within ClockTolerance.
func offsetFor(server, local time.Time) int64 {
	if d := server.Sub(local); d > ClockTolerance || d < -ClockTolerance {
		return int64(d / time.Second)
	}
	return 0
}
