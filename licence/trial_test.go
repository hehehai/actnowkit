package licence

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hehehai/actnowkit/credentials"
)

const day = 24 * time.Hour

// fakeKeychain is a login Keychain for credentials.Keychain: items by
// "service/account"; calls records the writes.
func fakeKeychain(items map[string]string, calls *[]string) func(string, ...string) ([]byte, []byte, error) {
	return func(stdin string, args ...string) ([]byte, []byte, error) {
		cmd := strings.Join(args, " ")
		switch {
		case strings.HasPrefix(cmd, "-i"):
			// add-generic-password -U -s "svc" -a "acct" -l "label" -X HEX
			f := strings.Fields(stdin)
			svc, acct := strings.Trim(f[3], `"`), strings.Trim(f[5], `"`)
			*calls = append(*calls, svc+"/"+acct)
			b, err := hex.DecodeString(f[len(f)-1])
			if err != nil {
				return nil, nil, err
			}
			items[svc+"/"+acct] = string(b)
			return nil, nil, nil
		case strings.HasPrefix(cmd, "find-generic-password"):
			f := strings.Fields(cmd)
			v, ok := items[f[2]+"/"+f[4]]
			if !ok {
				return nil, nil, credentials.ErrNotFound
			}
			return nil, []byte("password: 0x" + hex.EncodeToString([]byte(v)) + "  \"...\"\n"), nil
		}
		return nil, nil, errors.New("unexpected " + cmd)
	}
}

func (h *harness) requestsTo(path string) int {
	n := 0
	for _, r := range h.server.requests {
		if r == path {
			n++
		}
	}
	return n
}

func expectTrial(t *testing.T, st Status, state State, reason string, usable bool) {
	t.Helper()
	expectState(t, st, state, reason, usable)
	if !st.IsTrial() || st.Licensed() {
		t.Fatalf("%s is not a trial state", st.State)
	}
}

func TestFirstLaunchWritesKeychainAndSurvivesReinstall(t *testing.T) {
	h := newHarness(t, "0.3.0")
	items := map[string]string{}
	var writes []string
	kc := &KeychainTrialStore{Keychain: credentials.Keychain{Service: TrialService, Run: fakeKeychain(items, &writes)}, Account: "captioncraft"}
	h.trials = kc
	h.client = h.newClient("0.3.0")
	h.server.set(func(f *fakeServer) { f.down = true }) // offline first launch

	expectTrial(t, h.client.Status(), TrialLocal, ReasonUnconfirmed, true)
	if len(writes) == 0 || writes[0] != "actnow.trial/captioncraft" {
		t.Fatalf("writes %v", writes)
	}
	start := h.now.Unix()
	if got := items["actnow.trial/captioncraft"]; got != `{"startedAt":`+strconv.FormatInt(start, 10)+`,"lastSeen":`+strconv.FormatInt(start, 10)+`}` {
		t.Fatalf("record %s", got)
	}
	// Later launches read it, the start does not move.
	h.advance(3 * day)
	st := h.client.Status()
	expectTrial(t, st, TrialLocal, ReasonUnconfirmed, true)
	if want := h.now.Add(-3 * day).Add(TrialLength); !st.TrialEndsAt.Equal(want) {
		t.Fatalf("ends %v, want %v", st.TrialEndsAt, want)
	}
	if got := items["actnow.trial/captioncraft"]; !strings.HasPrefix(got, `{"startedAt":`+strconv.FormatInt(start, 10)+`,`) {
		t.Fatalf("the start moved: %s", got)
	}
	// Uninstall and reinstall: the Keychain keeps the start, so no reset.
	h.client = h.newClient("0.3.0")
	h.advance(13 * day)
	expectTrial(t, h.client.Status(), TrialExpired, ReasonTrialEnded, false)
}

func TestOfflineWithin15DaysIsLocalAndAfterIsExpired(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.server.set(func(f *fakeServer) { f.down = true })
	expectTrial(t, h.client.Status(), TrialLocal, ReasonUnconfirmed, true)
	h.advance(14*day + 23*time.Hour)
	expectTrial(t, h.client.Status(), TrialLocal, ReasonUnconfirmed, true)
	h.advance(2 * time.Hour)
	expectTrial(t, h.client.Status(), TrialExpired, ReasonTrialEnded, false)
	// Check offline does not change that.
	expectTrial(t, h.client.Check(context.Background()), TrialExpired, ReasonTrialEnded, false)
}

func TestOnlineTrialTokenIsValid(t *testing.T) {
	h := newHarness(t, "0.3.0")
	st := h.client.Check(context.Background())
	expectTrial(t, st, TrialValid, "", true)
	if st.Token == nil || st.Token.Edition != EditionTrial || st.Token.Device != h.device || st.Token.App != "captioncraft" {
		t.Fatalf("token %+v", st.Token)
	}
	if st.Edition() != "trial" {
		t.Fatalf("edition %q", st.Edition())
	}
	if !st.TrialEndsAt.Equal(h.now.Add(TrialLength)) || !st.TrialStartedAt.Equal(h.now) {
		t.Fatalf("trial %v..%v", st.TrialStartedAt, st.TrialEndsAt)
	}
	if h.requestsTo(TrialPath) != 1 {
		t.Fatalf("requests %v", h.server.requests)
	}
	// A current trial token: no more calls.
	n := len(h.server.requests)
	h.client.Check(context.Background())
	if len(h.server.requests) != n {
		t.Fatal("Check went online for a current trial token")
	}
}

func TestTrialTokenRefreshAndGrace(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.client.Check(context.Background())
	// Due after 7 days: refreshed through the trial endpoint, never /refresh.
	h.advance(8 * day)
	expectTrial(t, h.client.Status(), TrialValid, ReasonDue, true)
	expectTrial(t, h.client.Check(context.Background()), TrialValid, "", true)
	if h.requestsTo("/api/licence/v1/refresh") != 0 || h.requestsTo(TrialPath) != 2 {
		t.Fatalf("requests %v", h.server.requests)
	}
	// Due and offline: usable in grace; the trial ends at 15 days regardless.
	h.advance(8 * day) // day 16 of the trial: the token's lexp (day 15) has passed
	expectTrial(t, h.client.Status(), TrialExpired, ReasonTrialEnded, false)
}

func TestGraceOfTrialWhenOffline(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.client.Check(context.Background())
	h.advance(8 * day) // due
	h.server.set(func(f *fakeServer) { f.down = true })
	st, err := h.client.StartTrial(context.Background())
	if !errors.Is(err, ErrNetwork) {
		t.Fatalf("err %v", err)
	}
	expectTrial(t, st, TrialValid, ReasonOffline, true)
	if !strings.Contains(st.LastError, "cannot reach") {
		t.Fatalf("no error remembered: %q", st.LastError)
	}
}

func TestServerTrialExpiredIs403AndFinal(t *testing.T) {
	h := newHarness(t, "0.3.0")
	// The server knows this Mac's trial started 16 days ago, though this
	// Mac's first launch (offline) was just now.
	h.server.set(func(f *fakeServer) { f.trials = map[string]time.Time{"captioncraft|" + h.device: h.now.Add(-16 * day)} })
	h.server.set(func(f *fakeServer) { f.down = true })
	expectTrial(t, h.client.Status(), TrialLocal, ReasonUnconfirmed, true)
	h.server.set(func(f *fakeServer) { f.down = false })
	st := h.client.Check(context.Background())
	expectTrial(t, st, TrialExpired, ReasonTrialEnded, false)
	if st.Token != nil {
		t.Fatal("a token after trial_expired")
	}
	// The server's start (its error "details") replaces the Mac's.
	if rec, _ := h.trials.LoadTrial(); rec.StartedAt != h.now.Add(-16*day).Unix() || !rec.ServerEnded {
		t.Fatalf("record %+v", rec)
	}
	// Final: offline or with the clock back, it stays ended.
	h.server.set(func(f *fakeServer) { f.down = true })
	h.now = h.now.Add(-5 * day)
	expectTrial(t, h.client.Status(), TrialExpired, ReasonTrialEnded, false)
	if _, err := h.client.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.requestsTo("/api/licence/v1/refresh") != 0 {
		t.Fatal("refreshed a trial")
	}
}

func TestTrialExpiredNoCallWhenLocalEnded(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.server.set(func(f *fakeServer) { f.down = true })
	h.client.Status()
	h.advance(16 * day)
	n := len(h.server.requests)
	expectTrial(t, h.client.Check(context.Background()), TrialExpired, ReasonTrialEnded, false)
	if len(h.server.requests) != n {
		t.Fatal("went online for an ended trial")
	}
}

func TestCodeTokenBeatsExpiredTrial(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.client.Status() // the first launch: day 0
	h.advance(16 * day)
	expectTrial(t, h.client.Check(context.Background()), TrialExpired, ReasonTrialEnded, false)
	h.server.addCode("CC-TR", EditionPaid, 2, nil)
	st := h.activate("CC-TR")
	expectState(t, st, Valid, "", true)
	if st.IsTrial() || st.Edition() != EditionPaid {
		t.Fatalf("%+v", st)
	}
	h.advance(10 * day)
	expectState(t, h.client.Status(), NeedsRefresh, ReasonDue, true)
	expectState(t, h.client.Check(context.Background()), Valid, "", true) // refreshed as a code
}

func TestStoredCodeRefusalLocksWithoutTrial(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.server.addCode("CC-RV", EditionPaid, 2, nil)
	h.activate("CC-RV")
	// The code is revoked while the trial is still running: locked, no trial.
	h.server.set(func(f *fakeServer) { f.refuse = "code_revoked" })
	h.advance(8 * day)
	expectState(t, h.client.Check(context.Background()), Revoked, ReasonRevoked, false)
	expectState(t, h.client.Status(), Revoked, ReasonRevoked, false)
	if h.requestsTo(TrialPath) != 0 {
		t.Fatal("a trial was asked for a revoked code")
	}
}

func TestClockRollbackDoesNotExtendTrial(t *testing.T) {
	// Offline: the Mac's clock goes back; the time used stays the latest seen.
	h := newHarness(t, "0.3.0")
	h.server.set(func(f *fakeServer) { f.down = true })
	start := h.client.Status().TrialStartedAt
	h.advance(14 * day)
	expectTrial(t, h.client.Status(), TrialLocal, ReasonUnconfirmed, true)
	h.now = h.now.Add(-13 * day)
	expectTrial(t, h.client.Status(), TrialLocal, ReasonUnconfirmed, true)
	h.now = h.now.Add(-10 * day)
	expectTrial(t, h.client.Status(), TrialLocal, ReasonUnconfirmed, true)
	h.now = start.Add(TrialLength) // day 15 by the Mac's clock, the time seen is no later
	expectTrial(t, h.client.Status(), TrialExpired, ReasonTrialEnded, false)
	h.now = start.Add(day)
	expectTrial(t, h.client.Status(), TrialExpired, ReasonTrialEnded, false)

	// Online token: a rollback makes it due for the server, usable no more.
	h2 := newHarness(t, "0.3.0")
	h2.client.Check(context.Background())
	h2.now = h2.now.Add(-3 * day)
	expectTrial(t, h2.client.Status(), TrialValid, ReasonClock, false)
	h2.server.set(func(f *fakeServer) { f.down = true })
	expectTrial(t, h2.client.Check(context.Background()), TrialValid, ReasonClock, false)
}

func TestTrialDoesNotReachCodeRefreshOrStatus(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.client.Check(context.Background())
	if _, err := h.client.ServerStatus(context.Background()); !errors.Is(err, ErrNoRecord) {
		t.Fatalf("ServerStatus with a trial token: %v", err)
	}
	st, err := h.client.Refresh(context.Background())
	if err != nil || st.State != TrialValid {
		t.Fatalf("refresh of a trial: %v %v", st.State, err)
	}
	if h.requestsTo("/api/licence/v1/refresh") != 0 || h.requestsTo("/api/licence/v1/status") != 0 {
		t.Fatalf("requests %v", h.server.requests)
	}
}

func TestTrialTokenFromAnotherMacIsIgnored(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.client.Check(context.Background())
	h.device = DeviceIDFromUUID("00000000-AAAA-BBBB-CCCC-DDDDDDDDDDDD", testSalt)
	h.server.set(func(f *fakeServer) { f.down = true })
	// The token is someone else's: the Mac's own record applies.
	expectTrial(t, h.client.Status(), TrialLocal, ReasonUnconfirmed, true)
}

func TestTrialRecordIsPerApp(t *testing.T) {
	items := map[string]string{}
	var writes []string
	a := NewKeychainTrialStore("act")
	a.Keychain.Run = fakeKeychain(items, &writes)
	if err := a.SaveTrial(TrialRecord{StartedAt: 42}); err != nil {
		t.Fatal(err)
	}
	if writes[0] != "actnow.trial/act" {
		t.Fatal(writes)
	}
	got, err := a.LoadTrial()
	if err != nil || got.StartedAt != 42 {
		t.Fatalf("%+v %v", got, err)
	}
	b := NewKeychainTrialStore("inboxly")
	b.Keychain.Run = fakeKeychain(items, &writes)
	if _, err := b.LoadTrial(); !errors.Is(err, ErrNoTrial) {
		t.Fatalf("another app's trial: %v", err)
	}
	var rec TrialRecord
	raw := items["actnow.trial/act"]
	if err := json.Unmarshal([]byte(raw), &rec); err != nil || strings.Contains(raw, "token") {
		t.Fatalf("stored %s", raw)
	}
}

func TestTrialUnavailableLocksThisRunOnly(t *testing.T) {
	h := newHarness(t, "0.3.0")
	if len(h.device) != 64 || strings.ToLower(h.device) != h.device {
		t.Fatalf("device %q is not 64 lowercase hex", h.device)
	}
	h.client.Status()
	h.server.set(func(f *fakeServer) { f.trialOff = true })
	expectTrial(t, h.client.Check(context.Background()), TrialExpired, ReasonTrialUnavailable, false)
	// Not final: a new launch asks again (the site may have switched it on).
	h.server.set(func(f *fakeServer) { f.trialOff = false })
	h.client = h.newClient("0.3.0")
	expectTrial(t, h.client.Check(context.Background()), TrialValid, "", true)
}

// An app without Config.Trial: no trial record, no trial call, Inactive
// with no code (as before the trial), and a code works as usual.
func TestTrialOff(t *testing.T) {
	h := newHarness(t, "0.3.0")
	h.noTrial = true
	h.client = h.newClient("0.3.0")
	st := h.client.Check(context.Background())
	if st.State != Inactive || st.IsTrial() || st.Usable {
		t.Fatalf("no trial: %v %q", st.State, st.Reason)
	}
	if _, err := h.trials.LoadTrial(); err != ErrNoTrial {
		t.Fatalf("a trial record was written: %v", err)
	}
	if st, err := h.client.StartTrial(context.Background()); err != nil || st.State != Inactive {
		t.Fatalf("StartTrial without Trial: %v %v", st.State, err)
	}
	for _, p := range h.server.requests {
		if p == TrialPath {
			t.Fatal("the trial endpoint was called")
		}
	}
	h.advance(20 * day)
	if st := h.client.Status(); st.State != Inactive {
		t.Fatalf("after 20 days: %v", st.State)
	}
	h.server.addCode("CC-NT", EditionPaid, 2, nil)
	expectState(t, h.activate("CC-NT"), Valid, "", true)
}

// A site that sends trial_expired's start in "data" (refto.one) instead of
// "details" (actnow.dev): read the same.
func TestServerTrialExpiredData(t *testing.T) {
	h := newHarness(t, "0.3.0")
	start := h.now.Add(-20 * day)
	h.server.set(func(f *fakeServer) {
		f.trials = map[string]time.Time{"captioncraft|" + h.device: start}
		f.expiredInData = true
	})
	expectTrial(t, h.client.Check(context.Background()), TrialExpired, ReasonTrialEnded, false)
	if rec, _ := h.trials.LoadTrial(); rec.StartedAt != start.Unix() || !rec.ServerEnded {
		t.Fatalf("record %+v", rec)
	}
}
