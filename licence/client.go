package licence

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the site.
const DefaultBaseURL = "https://actnow.dev"

// DefaultTimeout of one request.
const DefaultTimeout = 20 * time.Second

// Config of a Client.
type Config struct {
	// App is the site's slug ("captioncraft").
	App string
	// Version is the running app's version ("0.3.0").
	Version string
	// DeviceSalt: a constant of the app (bundle id + random hex), mixed into
	// the device ID so it differs between apps (docs/licensing.md §4).
	DeviceSalt string
	// Keys: the public keys the app trusts (ProductionKeys()).
	Keys map[string]ed25519.PublicKey
	// Store keeps the token; NewKeychainStore(bundleID) in apps.
	Store Store
	// PaidSince: a paid_since the build knows itself (optional; the token
	// and the changelog API carry the site's).
	PaidSince string

	// BaseURL: DefaultBaseURL. Only https (http on loopback for tests).
	BaseURL string
	// HTTPClient: one with DefaultTimeout.
	HTTPClient *http.Client
	// Now: time.Now (tests move the clock).
	Now func() time.Time
	// DeviceID: DeviceID(DeviceSalt) (tests pretend to be another Mac).
	DeviceID func() (string, error)
	// Device: ThisDevice.
	Device func() DeviceInfo
}

// Client checks, activates and refreshes the licence of one app on this
// Mac. Safe for concurrent use.
type Client struct {
	cfg Config
	mu  sync.Mutex
}

// New checks the config and fills in the defaults.
func New(cfg Config) (*Client, error) {
	if cfg.App == "" || cfg.Version == "" {
		return nil, errors.New("licence: App and Version are required")
	}
	if len(cfg.Keys) == 0 {
		return nil, errors.New("licence: no public keys (use ProductionKeys())")
	}
	if cfg.Store == nil {
		return nil, errors.New("licence: no Store (NewKeychainStore(bundleID))")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || !(u.Scheme == "https" || (u.Scheme == "http" && isLoopback(u.Hostname()))) {
		return nil, fmt.Errorf("licence: %s: the licence server needs https", cfg.BaseURL)
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: DefaultTimeout}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.DeviceID == nil {
		salt := cfg.DeviceSalt
		cfg.DeviceID = func() (string, error) { return DeviceID(salt) }
	}
	if cfg.Device == nil {
		cfg.Device = ThisDevice
	}
	return &Client{cfg: cfg}, nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// ManageURL is the account page where the user sees their codes and
// removes Macs (removal happens on the site only, with an email).
func (c *Client) ManageURL(codeID string) string {
	u := c.cfg.BaseURL + "/account/licences"
	if codeID != "" {
		u += "?code=" + url.QueryEscape(codeID)
	}
	return u
}

// EarlyAccessURL is the early-access request page of the app.
func (c *Client) EarlyAccessURL() string {
	return c.cfg.BaseURL + "/account/early-access?app=" + url.QueryEscape(c.cfg.App)
}

// ActivationHelpURL explains codes, Macs and offline use.
func (c *Client) ActivationHelpURL() string { return c.cfg.BaseURL + "/help/activation" }

// Status is the licence state now, from what is stored: no network. It
// also saves the latest time seen (clock-rollback check).
func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.statusLocked()
}

func (c *Client) statusLocked() Status {
	rec, err := c.cfg.Store.Load()
	if err != nil {
		return Status{State: Inactive, ManageURL: c.ManageURL("")}
	}
	device, err := c.cfg.DeviceID()
	if err != nil {
		return Status{State: Inactive, Reason: ReasonInvalid, LastError: err.Error()}
	}
	st, update := evaluate(evalInput{
		Record:    rec,
		Keys:      c.cfg.Keys,
		App:       c.cfg.App,
		Device:    device,
		Version:   c.cfg.Version,
		PaidSince: c.cfg.PaidSince,
		Now:       c.cfg.Now(),
	})
	if update != nil {
		_ = c.cfg.Store.Save(*update)
	}
	if st.Token != nil {
		st.ManageURL = c.ManageURL(st.Token.CodeID)
	} else {
		st.ManageURL = c.ManageURL("")
	}
	return st
}

// APIError is an error answer of the licence API (docs/licensing.md §6.6).
type APIError struct {
	HTTPStatus int
	Code       string
	Message    string
	Details    json.RawMessage
}

func (e *APIError) Error() string {
	return fmt.Sprintf("licence: %s (%d): %s", e.Code, e.HTTPStatus, e.Message)
}

// ErrNetwork wraps failures to reach the server (offline, timeouts, 5xx).
var ErrNetwork = errors.New("licence: cannot reach actnow.dev")

type tokenAnswer struct {
	Token      string    `json:"token"`
	ServerTime time.Time `json:"serverTime"`
	Licence    struct {
		CodeID string `json:"codeId"`
	} `json:"licence"`
}

func (c *Client) post(ctx context.Context, endpoint string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/api/licence/v1/"+endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("user-agent", "actnowkit-licence "+c.cfg.App+"/"+c.cfg.Version)
	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("%w: %s", ErrNetwork, resp.Status)
	}
	if resp.StatusCode >= 400 {
		var e struct {
			Error   string          `json:"error"`
			Message string          `json:"message"`
			Details json.RawMessage `json:"details"`
		}
		if json.Unmarshal(data, &e) != nil || e.Error == "" {
			return fmt.Errorf("%w: %s", ErrNetwork, resp.Status)
		}
		return &APIError{HTTPStatus: resp.StatusCode, Code: e.Error, Message: e.Message, Details: e.Details}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: bad answer: %v", ErrNetwork, err)
	}
	return nil
}

func newNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// accept checks a token answer (signature, app, device, nonce) and stores
// it with the server's time.
func (c *Client) accept(answer tokenAnswer, device, nonce string) (*Token, error) {
	tok, err := ParseToken(answer.Token, c.cfg.Keys)
	if err != nil {
		return nil, err
	}
	if tok.App != c.cfg.App || tok.Device != device || tok.Nonce != nonce {
		return nil, errors.New("licence: the answer is not for this request")
	}
	local := c.cfg.Now()
	server := answer.ServerTime
	if server.IsZero() {
		server = tok.Issued()
	}
	offset := int64(0)
	if d := server.Sub(local); d > ClockTolerance || d < -ClockTolerance {
		offset = int64(d / time.Second)
	}
	rec := Record{Token: tok.Raw, LastSeen: server.Unix(), Offset: offset, CheckedAt: local.Unix()}
	if err := c.cfg.Store.Save(rec); err != nil {
		return nil, fmt.Errorf("licence: saving the token: %w", err)
	}
	return tok, nil
}

// Activate activates a code on this Mac (docs/licensing.md §6.1). The code
// may be typed loosely (spaces, lower case). Errors: *APIError (wrong code,
// revoked, expired, whitelist_not_applicable…) or ErrNetwork; a full code
// gives a Status with State DeviceLimit and no error.
func (c *Client) Activate(ctx context.Context, code string) (Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	device, err := c.cfg.DeviceID()
	if err != nil {
		return Status{State: Inactive}, err
	}
	info := c.cfg.Device()
	nonce := newNonce()
	var answer tokenAnswer
	err = c.post(ctx, "activate", map[string]any{
		"code":    strings.TrimSpace(code),
		"app":     c.cfg.App,
		"version": c.cfg.Version,
		"device": map[string]string{
			"id": device, "name": info.Name, "model": info.Model, "os": info.OS,
		},
		"nonce": nonce,
	}, &answer)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) && api.Code == "device_limit" {
			var d struct {
				Devices   []BoundDevice `json:"devices"`
				ManageURL string        `json:"manageUrl"`
			}
			_ = json.Unmarshal(api.Details, &d)
			return Status{State: DeviceLimit, Devices: d.Devices, ManageURL: d.ManageURL}, nil
		}
		return c.statusLocked(), err
	}
	if _, err := c.accept(answer, device, nonce); err != nil {
		return c.statusLocked(), err
	}
	return c.statusLocked(), nil
}

// Refresh asks the server for a new token (docs/licensing.md §6.2). A
// network failure is remembered (Grace) and returned; refusals that end the
// licence (removed, revoked, expired, whitelist_not_applicable) are stored
// and give their State.
func (c *Client) Refresh(ctx context.Context) (Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rec, err := c.cfg.Store.Load()
	if err != nil || rec.Token == "" {
		return Status{State: Inactive, ManageURL: c.ManageURL("")}, nil
	}
	device, err := c.cfg.DeviceID()
	if err != nil {
		return c.statusLocked(), err
	}
	nonce := newNonce()
	var answer tokenAnswer
	err = c.post(ctx, "refresh", map[string]any{
		"token":    rec.Token,
		"app":      c.cfg.App,
		"version":  c.cfg.Version,
		"deviceId": device,
		"nonce":    nonce,
	}, &answer)
	if err != nil {
		var api *APIError
		switch {
		case errors.As(err, &api):
			switch api.Code {
			case "activation_removed", "code_revoked", "code_expired", "whitelist_not_applicable":
				rec.Stopped = api.Code
				rec.StoppedVersion = ""
				if api.Code == "whitelist_not_applicable" {
					rec.StoppedVersion = c.cfg.Version
				}
				rec.LastError = ""
			case "invalid_token", "device_mismatch":
				// Not a token of this Mac: forget it.
				_ = c.cfg.Store.Delete()
				return Status{State: Inactive, Reason: ReasonInvalid, ManageURL: c.ManageURL("")}, err
			default:
				rec.LastError = err.Error()
			}
		default:
			rec.LastError = err.Error()
		}
		rec.CheckedAt = c.cfg.Now().Unix()
		_ = c.cfg.Store.Save(rec)
		st := c.statusLocked()
		if st.State == WhitelistNotApplicable || st.State == Revoked || st.State == Expired {
			return st, nil
		}
		return st, err
	}
	if _, err := c.accept(answer, device, nonce); err != nil {
		return c.statusLocked(), err
	}
	return c.statusLocked(), nil
}

// Check is what an app calls at launch and from the scheduler: the stored
// state, refreshed online when it is due or must be confirmed.
func (c *Client) Check(ctx context.Context) Status {
	st := c.Status()
	switch st.State {
	case NeedsRefresh, Grace:
	case Expired:
		if st.Reason != ReasonOffline {
			return st
		}
	default:
		return st
	}
	next, _ := c.Refresh(ctx) // a failure is in next (Grace, LastError)
	return next
}

// LicenceInfo is the server's view of the licence (POST /status).
type LicenceInfo struct {
	State   string `json:"state"`
	Licence struct {
		CodeID        string     `json:"codeId"`
		Product       string     `json:"product"`
		Edition       string     `json:"edition"`
		CodeHint      string     `json:"codeHint"`
		Status        string     `json:"status"`
		ExpiresAt     *time.Time `json:"expiresAt"`
		MaxDevices    int        `json:"maxDevices"`
		ActiveDevices int        `json:"activeDevices"`
		PaidSince     *string    `json:"paidSince"`
		ManageURL     string     `json:"manageUrl"`
	} `json:"licence"`
	ServerTime time.Time `json:"serverTime"`
}

// ServerStatus asks the server about the stored licence (the app's licence
// page: Macs in use). No new token.
func (c *Client) ServerStatus(ctx context.Context) (*LicenceInfo, error) {
	rec, err := c.cfg.Store.Load()
	if err != nil || rec.Token == "" {
		return nil, ErrNoRecord
	}
	device, err := c.cfg.DeviceID()
	if err != nil {
		return nil, err
	}
	var info LicenceInfo
	err = c.post(ctx, "status", map[string]any{
		"token": rec.Token, "app": c.cfg.App, "version": c.cfg.Version, "deviceId": device,
	}, &info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// Forget deletes the stored licence on this Mac (the Mac keeps its place
// on the code until it is removed on the site).
func (c *Client) Forget() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.cfg.Store.Delete()
	if errors.Is(err, ErrNoRecord) {
		return nil
	}
	return err
}
