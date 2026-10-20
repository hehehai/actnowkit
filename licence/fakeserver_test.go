package licence

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeServer is a small actnow.dev for tests: codes, activations, tokens
// signed with a test key, paid_since, refusals on demand.
type fakeServer struct {
	t   *testing.T
	srv *httptest.Server
	key ed25519.PrivateKey
	pub ed25519.PublicKey
	kid string

	mu          sync.Mutex
	now         time.Time
	codes       map[string]*fakeCode
	paidSince   string
	down        bool   // answer 503
	refuse      string // refresh answers this error code
	wrongNonce  bool
	wrongDevice bool
	requests    []string
}

type fakeCode struct {
	id, edition string
	maxDevices  int
	expires     *time.Time
	devices     []string
	names       []string
}

func newFakeServer(t *testing.T) *fakeServer {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeServer{t: t, key: priv, pub: pub, kid: "lk-test", now: time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC), codes: map[string]*fakeCode{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeServer) keys() map[string]ed25519.PublicKey {
	return map[string]ed25519.PublicKey{f.kid: f.pub}
}

func (f *fakeServer) addCode(code, edition string, maxDevices int, expires *time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.codes[code] = &fakeCode{id: "cid-" + code, edition: edition, maxDevices: maxDevices, expires: expires}
}

func (f *fakeServer) set(fn func(f *fakeServer)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeServer) sign(payload map[string]any) string {
	body, _ := json.Marshal(payload)
	signed := "anl1." + f.kid + "." + base64.RawURLEncoding.EncodeToString(body)
	return signed + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(f.key, []byte(signed)))
}

func (f *fakeServer) issue(c *fakeCode, app, device, version, nonce string) map[string]any {
	iat := f.now.Unix()
	exp := iat + 30*86400
	var lexp any
	if c.expires != nil {
		lexp = c.expires.Unix()
		exp = min(exp, c.expires.Unix())
	}
	var ps any
	if c.edition == EditionWhitelist && f.paidSince != "" {
		ps = f.paidSince
	}
	if f.wrongNonce {
		nonce = "someone-elses-nonce-0000"
	}
	if f.wrongDevice {
		device = strings.Repeat("f", 64)
	}
	return map[string]any{
		"token": f.sign(map[string]any{
			"v": 1, "cid": c.id, "aid": "aid-" + device[:8], "prd": app, "app": app, "ed": c.edition,
			"dev": device, "ver": version, "ps": ps, "iat": iat, "rfa": iat + 7*86400, "exp": exp,
			"lexp": lexp, "md": c.maxDevices, "nonce": nonce,
		}),
		"licence":    map[string]any{"codeId": c.id},
		"serverTime": f.now.Format(time.RFC3339Nano),
	}
}

func (f *fakeServer) fail(w http.ResponseWriter, status int, code string, details any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": code, "message": code, "details": details})
}

func (f *fakeServer) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.URL.Path)
	if f.down {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	str := func(k string) string { s, _ := req[k].(string); return s }
	w.Header().Set("content-type", "application/json")
	switch r.URL.Path {
	case "/api/licence/v1/activate":
		c := f.codes[strings.ToUpper(strings.ReplaceAll(str("code"), " ", "-"))]
		if c == nil {
			c = f.codes[str("code")]
		}
		if c == nil {
			f.fail(w, 404, "code_not_found", nil)
			return
		}
		dev, _ := req["device"].(map[string]any)
		id, _ := dev["id"].(string)
		name, _ := dev["name"].(string)
		if f.paidSince != "" && c.edition == EditionWhitelist && CompareVersions(str("version"), f.paidSince) >= 0 {
			f.fail(w, 403, "whitelist_not_applicable", map[string]any{"paidSince": f.paidSince})
			return
		}
		found := false
		for _, d := range c.devices {
			found = found || d == id
		}
		if !found {
			if len(c.devices) >= c.maxDevices {
				devices := []map[string]any{}
				for _, n := range c.names {
					devices = append(devices, map[string]any{"name": n, "activatedAt": f.now.Format(time.RFC3339)})
				}
				f.fail(w, 409, "device_limit", map[string]any{"maxDevices": c.maxDevices, "devices": devices, "manageUrl": "https://actnow.dev/account/licences?code=" + c.id})
				return
			}
			c.devices = append(c.devices, id)
			c.names = append(c.names, name)
		}
		_ = json.NewEncoder(w).Encode(f.issue(c, str("app"), id, str("version"), str("nonce")))
	case "/api/licence/v1/refresh", "/api/licence/v1/status":
		tok, err := ParseToken(str("token"), f.keys())
		if err != nil {
			f.fail(w, 401, "invalid_token", nil)
			return
		}
		if tok.Device != str("deviceId") {
			f.fail(w, 403, "device_mismatch", nil)
			return
		}
		if f.refuse != "" {
			status := map[string]int{"activation_removed": 410, "code_revoked": 403, "code_expired": 403, "whitelist_not_applicable": 403, "rate_limited": 429}[f.refuse]
			f.fail(w, status, f.refuse, nil)
			return
		}
		var c *fakeCode
		for _, x := range f.codes {
			if x.id == tok.CodeID {
				c = x
			}
		}
		if f.paidSince != "" && c.edition == EditionWhitelist && CompareVersions(str("version"), f.paidSince) >= 0 {
			f.fail(w, 403, "whitelist_not_applicable", map[string]any{"paidSince": f.paidSince})
			return
		}
		if r.URL.Path == "/api/licence/v1/status" {
			_ = json.NewEncoder(w).Encode(map[string]any{"state": "active", "licence": map[string]any{"codeId": c.id, "maxDevices": c.maxDevices, "activeDevices": len(c.devices)}, "serverTime": f.now})
			return
		}
		_ = json.NewEncoder(w).Encode(f.issue(c, str("app"), str("deviceId"), str("version"), str("nonce")))
	default:
		http.NotFound(w, r)
	}
}
