package licence

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hehehai/actnowkit/credentials"
)

const testSalt = "dev.hehehai.captioncraft/test"

type harness struct {
	t      *testing.T
	server *fakeServer
	store  *MemoryStore
	now    time.Time
	device string
	client *Client
}

func newHarness(t *testing.T, version string) *harness {
	h := &harness{t: t, server: newFakeServer(t), store: &MemoryStore{}}
	h.now = h.server.now
	h.device = DeviceIDFromUUID("6C1A5C38-1D0B-5E3A-9F55-0A1B2C3D4E5F", testSalt)
	h.client = h.newClient(version)
	return h
}

func (h *harness) newClient(version string) *Client {
	c, err := New(Config{
		App:        "captioncraft",
		Version:    version,
		DeviceSalt: testSalt,
		Keys:       h.server.keys(),
		Store:      h.store,
		BaseURL:    h.server.srv.URL,
		Now:        func() time.Time { return h.now },
		DeviceID:   func() (string, error) { return h.device, nil },
		Device:     func() DeviceInfo { return DeviceInfo{Name: "Test Mac", Model: "Mac15,12", OS: "26.1"} },
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return c
}

// advance moves both clocks (the Mac's and the server's).
func (h *harness) advance(d time.Duration) {
	h.now = h.now.Add(d)
	h.server.set(func(f *fakeServer) { f.now = f.now.Add(d) })
}

func (h *harness) activate(code string) Status {
	h.t.Helper()
	st, err := h.client.Activate(context.Background(), code)
	if err != nil {
		h.t.Fatalf("activate: %v", err)
	}
	return st
}

func expectState(t *testing.T, st Status, state State, reason string, usable bool) {
	t.Helper()
	if st.State != state || st.Reason != reason || st.Usable != usable {
		t.Fatalf("got %s/%s usable=%v (err %q), want %s/%s usable=%v", st.State, st.Reason, st.Usable, st.LastError, state, reason, usable)
	}
}

func TestActivateAndStatusOffline(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.server.addCode("CC-AAAA", EditionWhitelist, 2, nil)
	st := h.activate("cc aaaa")
	expectState(t, st, Valid, "", true)
	if st.Token.Edition != EditionWhitelist || st.Token.Device != h.device || st.Token.App != "captioncraft" {
		t.Fatalf("token: %+v", st.Token)
	}
	if want := h.now.Add(7 * 24 * time.Hour); !st.RefreshAfter.Equal(want.Truncate(time.Second)) {
		t.Fatalf("refresh after %v, want %v", st.RefreshAfter, want)
	}
	if !strings.Contains(st.ManageURL, "/account/licences?code=cid-CC-AAAA") {
		t.Fatalf("manage URL %s", st.ManageURL)
	}
	// Offline from now on: the stored token is enough.
	h.server.set(func(f *fakeServer) { f.down = true })
	expectState(t, h.client.Status(), Valid, "", true)
	n := len(h.server.requests)
	h.client.Check(context.Background())
	if len(h.server.requests) != n {
		t.Fatal("Check went online for a valid token")
	}
}

func TestRefreshScheduleGraceAndOfflineExpiry(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.server.addCode("CC-B", EditionPaid, 2, nil)
	h.activate("CC-B")

	h.advance(8 * 24 * time.Hour)
	expectState(t, h.client.Status(), NeedsRefresh, ReasonDue, true)
	st := h.client.Check(context.Background())
	expectState(t, st, Valid, "", true)

	// Offline: due, the refresh fails, still usable (grace) until 30 days.
	h.server.set(func(f *fakeServer) { f.down = true })
	h.advance(8 * 24 * time.Hour)
	st = h.client.Check(context.Background())
	expectState(t, st, Grace, ReasonOffline, true)
	if !strings.Contains(st.LastError, "cannot reach") {
		t.Fatal("no error remembered")
	}
	h.advance(23 * 24 * time.Hour) // 31 days after the last refresh
	expectState(t, h.client.Check(context.Background()), Expired, ReasonOffline, false)

	// Back online: a refresh brings it back.
	h.server.set(func(f *fakeServer) { f.down = false })
	expectState(t, h.client.Check(context.Background()), Valid, "", true)
}

func TestTamperedTokenAndOtherMac(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.server.addCode("CC-C", EditionWhitelist, 2, nil)
	h.activate("CC-C")
	rec, _ := h.store.Load()

	// Change the edition in the payload, keep the signature.
	parts := strings.Split(rec.Token, ".")
	body, _ := base64.RawURLEncoding.DecodeString(parts[2])
	var payload map[string]any
	_ = json.Unmarshal(body, &payload)
	payload["ed"] = "paid"
	payload["exp"] = h.now.Add(10 * 365 * 24 * time.Hour).Unix()
	changed, _ := json.Marshal(payload)
	parts[2] = base64.RawURLEncoding.EncodeToString(changed)
	forged := rec
	forged.Token = strings.Join(parts, ".")
	_ = h.store.Save(forged)
	expectState(t, h.client.Status(), Inactive, ReasonInvalid, false)

	// A token signed by another key (a fake server).
	_, other, _ := ed25519.GenerateKey(nil)
	signed := "anl1.lk-test." + parts[2]
	_ = h.store.Save(Record{Token: signed + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(other, []byte(signed)))})
	expectState(t, h.client.Status(), Inactive, ReasonInvalid, false)

	// The genuine token copied to another Mac.
	_ = h.store.Save(rec)
	expectState(t, h.client.Status(), Valid, "", true)
	h.device = DeviceIDFromUUID("00000000-AAAA-BBBB-CCCC-DDDDDDDDDDDD", testSalt)
	expectState(t, h.client.Status(), Inactive, ReasonOtherDevice, false)
}

func TestClockRollback(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.server.addCode("CC-D", EditionPaid, 2, nil)
	h.activate("CC-D")
	h.now = h.now.Add(3 * 24 * time.Hour)
	expectState(t, h.client.Status(), Valid, "", true) // remembers day 3

	// Back two days: needs the server.
	h.now = h.now.Add(-2 * 24 * time.Hour)
	expectState(t, h.client.Status(), NeedsRefresh, ReasonClock, false)
	// Small differences are tolerated.
	h.now = h.now.Add(2*24*time.Hour - 5*time.Minute)
	expectState(t, h.client.Status(), Valid, "", true)

	// Rolling back far enough to make an expired token young again does
	// not work either: the time used never goes below the last seen.
	h.advance(40 * 24 * time.Hour)
	h.server.set(func(f *fakeServer) { f.down = true })
	expectState(t, h.client.Status(), Expired, ReasonOffline, false)
	h.now = h.now.Add(-39 * 24 * time.Hour)
	expectState(t, h.client.Status(), NeedsRefresh, ReasonClock, false)

	// A Mac whose clock is simply wrong (a day behind the server) works
	// after one refresh: the offset is kept.
	h.server.set(func(f *fakeServer) { f.down = false })
	h.now = h.server.now.Add(-24 * time.Hour)
	st := h.client.Check(context.Background())
	expectState(t, st, Valid, "", true)
	rec, _ := h.store.Load()
	if rec.Offset != 24*3600 {
		t.Fatalf("offset %d", rec.Offset)
	}
	expectState(t, h.client.Status(), Valid, "", true)
}

func TestWhitelistPaidSince(t *testing.T) {
	h := newHarness(t, "0.9.0")
	h.server.addCode("CC-E", EditionWhitelist, 2, nil)
	h.activate("CC-E")

	// The site sets paid_since 1.0.0 after the token was issued. The old
	// version keeps working, offline and online.
	h.server.set(func(f *fakeServer) { f.paidSince = "1.0.0" })
	expectState(t, h.client.Status(), Valid, "", true)
	h.advance(8 * 24 * time.Hour)
	st := h.client.Check(context.Background())
	expectState(t, st, Valid, "", true)
	if st.PaidSince != "1.0.0" {
		t.Fatalf("paid since %q", st.PaidSince)
	}

	// The user updates to 1.0.0: refused offline at once (the token says
	// ps), and online.
	paid := h.newClient("1.0.0")
	expectState(t, paid.Status(), WhitelistNotApplicable, "", false)
	st, err := paid.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	expectState(t, st, WhitelistNotApplicable, "", false)

	// Going back to the early-access build works again.
	old := h.newClient("0.9.0")
	expectState(t, old.Status(), Valid, "", true)
}

func TestWhitelistUpdateMustConfirmOnline(t *testing.T) {
	h := newHarness(t, "0.9.0")
	h.server.addCode("CC-F", EditionWhitelist, 2, nil)
	h.activate("CC-F")
	// paid_since appears on the site; the token (no ps) does not know.
	h.server.set(func(f *fakeServer) { f.paidSince = "1.0.0" })
	updated := h.newClient("1.0.0")
	// Updated since the token was issued: not usable until confirmed.
	expectState(t, updated.Status(), NeedsRefresh, ReasonUpdated, false)
	expectState(t, updated.Check(context.Background()), WhitelistNotApplicable, "", false)

	// A 0.9.1 update (below paid_since) confirms online and goes on.
	h2 := newHarness(t, "0.9.0")
	h2.server.addCode("CC-G", EditionWhitelist, 2, nil)
	h2.activate("CC-G")
	h2.server.set(func(f *fakeServer) { f.paidSince = "1.0.0" })
	next := h2.newClient("0.9.1")
	expectState(t, next.Status(), NeedsRefresh, ReasonUpdated, false)
	expectState(t, next.Check(context.Background()), Valid, "", true)
	// Offline after an update: still blocked (no grace).
	h3 := newHarness(t, "0.9.0")
	h3.server.addCode("CC-H", EditionWhitelist, 2, nil)
	h3.activate("CC-H")
	h3.server.set(func(f *fakeServer) { f.down = true })
	expectState(t, h3.newClient("0.9.1").Check(context.Background()), NeedsRefresh, ReasonUpdated, false)
	// Paid editions do not care about updates.
	h4 := newHarness(t, "0.9.0")
	h4.server.addCode("CC-I", EditionPaid, 2, nil)
	h4.activate("CC-I")
	expectState(t, h4.newClient("2.0.0").Status(), Valid, "", true)
}

func TestDeviceLimitAndRemoval(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.server.addCode("CC-J", EditionPaid, 2, nil)
	for i, uuid := range []string{"11111111-0000-0000-0000-000000000000", "22222222-0000-0000-0000-000000000000"} {
		h.device = DeviceIDFromUUID(uuid, testSalt)
		h.store = &MemoryStore{}
		h.client = h.newClient("0.3.0")
		if st := h.activate("CC-J"); st.State != Valid {
			t.Fatalf("Mac %d: %s", i+1, st.State)
		}
	}
	h.device = DeviceIDFromUUID("33333333-0000-0000-0000-000000000000", testSalt)
	h.store = &MemoryStore{}
	h.client = h.newClient("0.3.0")
	st, err := h.client.Activate(context.Background(), "CC-J")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != DeviceLimit || len(st.Devices) != 2 || st.Devices[0].Name != "Test Mac" || !strings.Contains(st.ManageURL, "cid-CC-J") {
		t.Fatalf("%+v", st)
	}

	// Mac 2 removed on the site: its next refresh ends the licence.
	h2 := newHarness(t, "0.3.0")
	h2.server.addCode("CC-K", EditionPaid, 2, nil)
	h2.activate("CC-K")
	h2.server.set(func(f *fakeServer) { f.refuse = "activation_removed" })
	h2.advance(8 * 24 * time.Hour)
	expectState(t, h2.client.Check(context.Background()), Revoked, ReasonRemoved, false)
	// It stays so offline.
	h2.server.set(func(f *fakeServer) { f.down = true })
	expectState(t, h2.client.Status(), Revoked, ReasonRemoved, false)
	// Activating again (a free place) replaces the record.
	h2.server.set(func(f *fakeServer) { f.down = false; f.refuse = "" })
	expectState(t, h2.activate("CC-K"), Valid, "", true)
	// A revoked code.
	h2.server.set(func(f *fakeServer) { f.refuse = "code_revoked" })
	st, err = h2.client.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	expectState(t, st, Revoked, ReasonRevoked, false)
}

func TestCodeExpiry(t *testing.T) {
	h := newHarness(t, "0.3.0")
	end := h.now.Add(10 * 24 * time.Hour)
	h.server.addCode("CC-L", EditionWhitelist, 2, &end)
	st := h.activate("CC-L")
	if !st.GraceUntil.Equal(end.Truncate(time.Second)) || !st.CodeExpires.Equal(end.Truncate(time.Second)) {
		t.Fatalf("grace %v, code %v, want %v", st.GraceUntil, st.CodeExpires, end)
	}
	h.server.set(func(f *fakeServer) { f.down = true })
	h.advance(11 * 24 * time.Hour)
	expectState(t, h.client.Check(context.Background()), Expired, ReasonCodeExpired, false)
}

func TestAnswersAreChecked(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.server.addCode("CC-M", EditionPaid, 2, nil)
	h.server.set(func(f *fakeServer) { f.wrongNonce = true })
	if _, err := h.client.Activate(context.Background(), "CC-M"); err == nil {
		t.Fatal("a replayed answer (other nonce) was accepted")
	}
	h.server.set(func(f *fakeServer) { f.wrongNonce = false; f.wrongDevice = true })
	if _, err := h.client.Activate(context.Background(), "CC-M"); err == nil {
		t.Fatal("an answer for another Mac was accepted")
	}
	h.server.set(func(f *fakeServer) { f.wrongDevice = false })
	_, err := h.client.Activate(context.Background(), "CC-NOPE")
	var api *APIError
	if !errors.As(err, &api) || api.Code != "code_not_found" || ErrorText(api.Code) != TextErrNotFound {
		t.Fatalf("err %v", err)
	}
	h.server.set(func(f *fakeServer) { f.down = true })
	if _, err := h.client.Activate(context.Background(), "CC-M"); !errors.Is(err, ErrNetwork) {
		t.Fatalf("err %v", err)
	}
	if _, err := New(Config{App: "x", Version: "1.0.0", Keys: h.server.keys(), Store: h.store, BaseURL: "http://example.com"}); err == nil {
		t.Fatal("plain http accepted")
	}
}

func TestServerStatusAndForget(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.server.addCode("CC-N", EditionPaid, 2, nil)
	h.activate("CC-N")
	info, err := h.client.ServerStatus(context.Background())
	if err != nil || info.State != "active" || info.Licence.ActiveDevices != 1 {
		t.Fatalf("%+v %v", info, err)
	}
	if err := h.client.Forget(); err != nil {
		t.Fatal(err)
	}
	expectState(t, h.client.Status(), Inactive, "", false)
	if h.client.EarlyAccessURL() != h.server.srv.URL+"/account/early-access?app=captioncraft" {
		t.Fatal(h.client.EarlyAccessURL())
	}
}

func TestRun(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.server.addCode("CC-O", EditionPaid, 2, nil)
	h.activate("CC-O")
	h.advance(8 * 24 * time.Hour)
	h.server.set(func(f *fakeServer) { f.down = true })
	oldRetry := RetryAfter
	RetryAfter = []time.Duration{10 * time.Millisecond}
	t.Cleanup(func() { RetryAfter = oldRetry })

	var mu sync.Mutex
	var seen []State
	changed := make(chan struct{}, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wake := h.client.Run(ctx, func(st Status) {
		mu.Lock()
		seen = append(seen, st.State)
		mu.Unlock()
		changed <- struct{}{}
	})
	<-changed // Grace: the refresh failed
	h.server.set(func(f *fakeServer) { f.down = false })
	wake()
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("no change after the network came back")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) < 2 || seen[0] != Grace || seen[len(seen)-1] != Valid {
		t.Fatalf("states %v", seen)
	}
}

func TestDeviceID(t *testing.T) {
	a := DeviceIDFromUUID("6c1a5c38-1d0b-5e3a-9f55-0a1b2c3d4e5f", "salt-a")
	if a != DeviceIDFromUUID(" 6C1A5C38-1D0B-5E3A-9F55-0A1B2C3D4E5F ", "salt-a") {
		t.Fatal("case or spaces change the ID")
	}
	if a == DeviceIDFromUUID("6C1A5C38-1D0B-5E3A-9F55-0A1B2C3D4E5F", "salt-b") {
		t.Fatal("the salt does not change the ID")
	}
	if len(a) != 64 {
		t.Fatal(a)
	}
}

// TestSiteToken checks a token the website signed (testdata: the local
// dev server, its dev key) with this package: the two implementations
// agree on the format and the device ID.
func TestSiteToken(t *testing.T) {
	raw, err := os.ReadFile("testdata/site-token.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		PlatformUUID, Salt, DeviceID, Nonce, Token, CodeID string
		Keys                                               []struct{ Kid, PublicKey string }
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	if got := DeviceIDFromUUID(fx.PlatformUUID, fx.Salt); got != fx.DeviceID {
		t.Fatalf("device id %s, site %s", got, fx.DeviceID)
	}
	key, err := ParsePublicKey(fx.Keys[0].PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := ParseToken(fx.Token, map[string]ed25519.PublicKey{fx.Keys[0].Kid: key})
	if err != nil {
		t.Fatal(err)
	}
	if tok.CodeID != fx.CodeID || tok.Device != fx.DeviceID || tok.Nonce != fx.Nonce || tok.Edition != EditionWhitelist || tok.App != "captioncraft" || tok.Version != "0.3.0" {
		t.Fatalf("%+v", tok)
	}
	if tok.RefreshAt-tok.IssuedAt != 7*86400 || tok.ExpiresAt-tok.IssuedAt != 30*86400 || tok.CodeExpiry == nil {
		t.Fatalf("times %+v", tok)
	}
	// The production key does not accept a development token.
	if _, err := ParseToken(fx.Token, ProductionKeys()); !errors.Is(err, ErrTokenKey) {
		t.Fatalf("err %v", err)
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0}, {"1.0.0", "0.9.9", 1}, {"0.9.0", "1.0.0", -1},
		{"1.0.0-beta.3", "1.0.0", -1}, {"1.0.0-beta.10", "1.0.0-beta.9", 1},
		{"1.0.0-alpha", "1.0.0-beta", -1}, {"1.0.0-beta.1", "1.0.0-beta", 1},
		{"v1.2.3", "1.2.3", 0}, {"1.2.3+build", "1.2.3", 0}, {"1.10.0", "1.9.0", 1},
	} {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	if !WhitelistBlocked("1.0.0", "1.0.0") || WhitelistBlocked("1.0.0-beta.2", "1.0.0") || WhitelistBlocked("9.0.0", "") {
		t.Fatal("WhitelistBlocked")
	}
}

func TestKeychainStore(t *testing.T) {
	items := map[string]string{}
	var services []string
	store := &KeychainStore{Keychain: credentials.Keychain{
		Service: "dev.hehehai.captioncraft.licence",
		Run: func(stdin string, args ...string) ([]byte, []byte, error) {
			cmd := strings.Join(args, " ")
			switch {
			case strings.HasPrefix(cmd, "-i"):
				// add-generic-password -U -s "svc" -a "acct" -l "label" -X hex
				fields := strings.Fields(stdin)
				services = append(services, strings.Trim(fields[3], `"`))
				hexed := fields[len(fields)-1]
				b := make([]byte, len(hexed)/2)
				for i := range b {
					var v byte
					for _, c := range hexed[2*i : 2*i+2] {
						v <<= 4
						switch {
						case c >= '0' && c <= '9':
							v |= byte(c - '0')
						default:
							v |= byte(c-'a') + 10
						}
					}
					b[i] = v
				}
				items[KeychainAccount] = string(b)
				return nil, nil, nil
			case strings.HasPrefix(cmd, "find-generic-password"):
				v, ok := items[KeychainAccount]
				if !ok {
					return nil, nil, credentials.ErrNotFound
				}
				return nil, []byte("password: 0x" + hexOf(v) + "  \"...\"\n"), nil
			case strings.HasPrefix(cmd, "delete-generic-password"):
				delete(items, KeychainAccount)
				return nil, nil, nil
			}
			return nil, nil, errors.New("unexpected " + cmd)
		},
	}}
	if _, err := store.Load(); !errors.Is(err, ErrNoRecord) {
		t.Fatal(err)
	}
	rec := Record{Token: "anl1.x.y.z", LastSeen: 42, Offset: -3}
	if err := store.Save(rec); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil || got != rec {
		t.Fatalf("%+v %v", got, err)
	}
	if services[0] != "dev.hehehai.captioncraft.licence" || NewKeychainStore("dev.hehehai.pik").Keychain.Service != "dev.hehehai.pik.licence" {
		t.Fatal(services)
	}
	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); !errors.Is(err, ErrNoRecord) {
		t.Fatal(err)
	}
}

func hexOf(s string) string {
	const digits = "0123456789ABCDEF"
	out := make([]byte, 0, len(s)*2)
	for _, b := range []byte(s) {
		out = append(out, digits[b>>4], digits[b&15])
	}
	return string(out)
}

func TestStateTexts(t *testing.T) {
	for _, st := range []Status{
		{State: Valid}, {State: Grace}, {State: NeedsRefresh, Reason: ReasonUpdated}, {State: NeedsRefresh, Reason: ReasonClock},
		{State: Expired, Reason: ReasonOffline}, {State: Expired, Reason: ReasonCodeExpired}, {State: WhitelistNotApplicable},
		{State: DeviceLimit}, {State: Revoked, Reason: ReasonRemoved}, {State: Revoked}, {State: Inactive},
	} {
		text := StateText(st).Fill("app", "CaptionCraft", "date", "2027-01-20", "version", "1.0.0", "max", "2")
		if text.EN == "" || text.ZH == "" || strings.Contains(text.EN+text.ZH, "{") {
			t.Errorf("%s/%s: %+v", st.State, st.Reason, text)
		}
	}
	if !strings.Contains(TextUpdateNeedsPaid.ZH, "体验码将不能继续使用") {
		t.Fatal(TextUpdateNeedsPaid.ZH)
	}
}

func TestProductionKey(t *testing.T) {
	keys := ProductionKeys()
	if len(keys[ProductionKeyID]) != ed25519.PublicKeySize {
		t.Fatal("production key")
	}
}
