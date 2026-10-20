package licence

import (
	"crypto/ed25519"
	"time"
)

// State of the licence on this Mac (docs/licensing.md §15).
type State string

const (
	// Inactive: no licence here (never activated, removed locally, or the
	// stored token is not valid for this Mac and app).
	Inactive State = "inactive"
	// Valid: a valid token, not due for refresh.
	Valid State = "valid"
	// NeedsRefresh: due for a refresh (Reason "due": still usable), or it
	// must refresh online before use (Reason "updated" or "clock").
	NeedsRefresh State = "needs_refresh"
	// Grace: due, the last refresh failed (offline); usable until GraceUntil.
	Grace State = "grace"
	// Expired: the code ended (Reason "code_expired") or the Mac was offline
	// past the grace period (Reason "offline"). Show "needs renewal"; keep
	// the user's data.
	Expired State = "expired"
	// WhitelistNotApplicable: an early-access licence and this version is at
	// or above paid_since: a paid licence is needed.
	WhitelistNotApplicable State = "whitelist_not_applicable"
	// DeviceLimit: activation refused, the code is on its maximum of Macs
	// (Status.Devices lists them; remove one at ManageURL).
	DeviceLimit State = "device_limit"
	// Revoked: the code was revoked, or this Mac was removed from it.
	Revoked State = "revoked"
)

// Reasons of a Status.
const (
	ReasonDue         = "due"
	ReasonUpdated     = "updated"
	ReasonClock       = "clock"
	ReasonCodeExpired = "code_expired"
	ReasonOffline     = "offline"
	ReasonRemoved     = "activation_removed"
	ReasonRevoked     = "code_revoked"
	ReasonOtherDevice = "other_device"
	ReasonInvalid     = "invalid_token"
)

// ClockTolerance is how far the clock may go back before it counts as a
// rollback.
const ClockTolerance = 10 * time.Minute

// BoundDevice is a Mac a code is used on (the device_limit answer).
type BoundDevice struct {
	Name        string    `json:"name"`
	ActivatedAt time.Time `json:"activatedAt"`
}

// Status is what the app shows.
type Status struct {
	State  State
	Reason string
	// Token is the verified token (nil when Inactive).
	Token *Token
	// Usable: the app's licensed features may run.
	Usable bool
	// RefreshAfter and GraceUntil (zero when there is no token).
	RefreshAfter time.Time
	GraceUntil   time.Time
	// CodeExpires: when the code ends (zero: never, or no token).
	CodeExpires time.Time
	// PaidSince for early-access licences ("" when not set).
	PaidSince string
	// Devices: the Macs using the code (DeviceLimit only).
	Devices []BoundDevice
	// ManageURL: the code's page on actnow.dev (remove a Mac there).
	ManageURL string
	// LastError: the last refresh's error, if it failed.
	LastError string
}

// Edition of the licence ("" when there is none).
func (s Status) Edition() string {
	if s.Token == nil {
		return ""
	}
	return s.Token.Edition
}

// evalInput is everything Evaluate needs; pure, for tests.
type evalInput struct {
	Record    Record
	Keys      map[string]ed25519.PublicKey
	App       string
	Device    string
	Version   string
	PaidSince string // the build's own, if it knows one
	Now       time.Time
}

// evaluate decides the state from the stored record, offline.
func evaluate(in evalInput) (Status, *Record) {
	r := in.Record
	if r.Token == "" {
		return Status{State: Inactive}, nil
	}
	tok, err := ParseToken(r.Token, in.Keys)
	if err != nil {
		return Status{State: Inactive, Reason: ReasonInvalid}, nil
	}
	if tok.App != in.App || tok.Device != in.Device {
		return Status{State: Inactive, Reason: ReasonOtherDevice}, nil
	}
	st := Status{
		Token:        tok,
		RefreshAfter: tok.RefreshAfter(),
		GraceUntil:   tok.Expires(),
		CodeExpires:  tok.CodeExpires(),
		LastError:    r.LastError,
	}
	if tok.Edition == EditionWhitelist {
		st.PaidSince = tok.PaidSinceVersion()
		if st.PaidSince == "" {
			st.PaidSince = in.PaidSince
		}
	}
	switch r.Stopped {
	case ReasonRemoved:
		st.State, st.Reason = Revoked, ReasonRemoved
		return st, nil
	case ReasonRevoked:
		st.State, st.Reason = Revoked, ReasonRevoked
		return st, nil
	case ReasonCodeExpired:
		st.State, st.Reason = Expired, ReasonCodeExpired
		return st, nil
	case string(WhitelistNotApplicable):
		// Only for the version refused (or later): an older build is fine.
		if r.StoppedVersion == "" || CompareVersions(in.Version, r.StoppedVersion) >= 0 {
			st.State = WhitelistNotApplicable
			return st, nil
		}
	}

	local := in.Now.Add(time.Duration(r.Offset) * time.Second)
	lastSeen := time.Unix(r.LastSeen, 0)
	if r.LastSeen > 0 && local.Before(lastSeen.Add(-ClockTolerance)) {
		st.State, st.Reason = NeedsRefresh, ReasonClock
		return st, nil
	}
	now := local
	if lastSeen.After(now) {
		now = lastSeen
	}
	if tok.Issued().After(now) {
		now = tok.Issued()
	}
	// Keep the latest time seen (the caller saves it).
	var update *Record
	if now.Unix() > r.LastSeen {
		next := r
		next.LastSeen = now.Unix()
		update = &next
	}

	if !st.CodeExpires.IsZero() && !now.Before(st.CodeExpires) {
		st.State, st.Reason = Expired, ReasonCodeExpired
		return st, update
	}
	if tok.Edition == EditionWhitelist {
		if WhitelistBlocked(in.Version, st.PaidSince) {
			st.State = WhitelistNotApplicable
			return st, update
		}
		// Updated since the token was issued: confirm online first, so a
		// paid_since set after it was issued still applies.
		if CompareVersions(in.Version, tok.Version) > 0 {
			st.State, st.Reason = NeedsRefresh, ReasonUpdated
			return st, update
		}
	}
	if !now.Before(tok.Expires()) {
		st.State, st.Reason = Expired, ReasonOffline
		return st, update
	}
	if !now.Before(tok.RefreshAfter()) {
		st.Usable = true
		if r.LastError != "" {
			st.State, st.Reason = Grace, ReasonOffline
		} else {
			st.State, st.Reason = NeedsRefresh, ReasonDue
		}
		return st, update
	}
	st.State, st.Usable = Valid, true
	return st, update
}
