package updates

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hehehai/actnowkit/licence"
)

// changelogServer answers GET /api/changelog/<app> like actnow.dev.
func changelogServer(t *testing.T, paidSince string) (*httptest.Server, *[]string) {
	var mu sync.Mutex
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries = append(queries, r.URL.RawQuery)
		mu.Unlock()
		if r.URL.Path != "/api/changelog/captioncraft" {
			http.NotFound(w, r)
			return
		}
		zh := r.URL.Query().Get("locale") == "zh-Hans"
		entry := func(v, en, cn string) map[string]any {
			notes, locale := en, "en"
			if zh && cn != "" {
				notes, locale = cn, "zh-Hans"
			}
			return map[string]any{"version": v, "channel": "stable", "date": "2026-10-20", "notes": notes, "locale": locale,
				"requiresPaidLicence": paidSince != "" && licence.CompareVersions(v, paidSince) >= 0}
		}
		var ps any
		if paidSince != "" {
			ps = paidSince
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"app": "captioncraft", "locale": map[bool]string{true: "zh-Hans", false: "en"}[zh], "paidSince": ps,
			"entries": []any{
				entry("1.0.0", "### Added\n- On sale", ""),
				entry("0.9.1", "### Fixed\n- Dubbing", "### 修复\n- 配音"),
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &queries
}

type fakeUpdater struct {
	mu       sync.Mutex
	release  *Release
	err      error
	disabled bool
}

func (f *fakeUpdater) Enabled() bool { return !f.disabled }

func (f *fakeUpdater) Check(context.Context) (*Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.release, f.err
}

func release(version string, installErr error) *Release {
	return &Release{
		Version: version,
		Notes:   "manifest notes " + version,
		Install: func(ctx context.Context, progress func(done, total int64)) error {
			for i := int64(1); i <= 4; i++ {
				progress(i*25, 100)
			}
			return installErr
		},
	}
}

func whitelist(paidSince string) func() licence.Status {
	return func() licence.Status {
		var ps *string
		if paidSince != "" {
			ps = &paidSince
		}
		return licence.Status{State: licence.Valid, Token: &licence.Token{Edition: licence.EditionWhitelist, PaidSince: ps}, PaidSince: paidSince}
	}
}

func TestCheckInstallRestart(t *testing.T) {
	srv, queries := changelogServer(t, "")
	up := &fakeUpdater{release: release("0.9.1", nil)}
	restarted := false
	var logs []string
	lang := "zh-Hans"
	m := New(Config{
		Updater:  up,
		Current:  "0.9.0",
		History:  &HistoryClient{BaseURL: srv.URL, App: "captioncraft"},
		Language: func() string { return lang },
		Beta:     func() bool { return true },
		Restart:  func() { restarted = true },
		Log:      func(s string) { logs = append(logs, s) },
	})
	var phases []Phase
	unsubscribe := m.Subscribe(func(s State) { phases = append(phases, s.Phase) })
	st := m.Check(context.Background())
	if st.Phase != Available || st.Notes != "### 修复\n- 配音" || st.NotesLocale != "zh-Hans" || st.NeedsPaidLicence {
		t.Fatalf("%+v", st)
	}
	if !strings.Contains((*queries)[0], "locale=zh-Hans") || !strings.Contains((*queries)[0], "channel=beta") {
		t.Fatal(*queries)
	}
	st = m.Install(context.Background(), false)
	if st.Phase != Ready || st.Downloaded != 100 || st.Total != 100 || st.Fraction() != 1 {
		t.Fatalf("%+v", st)
	}
	if err := m.Restart(); err != nil || !restarted {
		t.Fatal(err)
	}
	unsubscribe()
	if phases[0] != Checking || phases[1] != Available || phases[2] != Downloading || phases[len(phases)-1] != Ready {
		t.Fatalf("phases %v", phases)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "installed 0.9.1") {
		t.Fatal(logs)
	}
	// A check while Ready keeps Ready.
	if m.Check(context.Background()).Phase != Ready {
		t.Fatal("check after install")
	}
}

func TestEnglishFallbackAndManifestNotes(t *testing.T) {
	srv, _ := changelogServer(t, "")
	m := New(Config{
		Updater:  &fakeUpdater{release: release("1.0.0", nil)},
		Current:  "0.9.1",
		History:  &HistoryClient{BaseURL: srv.URL, App: "captioncraft"},
		Language: func() string { return "zh-CN" },
	})
	st := m.Check(context.Background())
	if st.Notes != "### Added\n- On sale" || st.NotesLocale != "en" {
		t.Fatalf("%+v", st)
	}
	// The changelog is down: the manifest's notes.
	m2 := New(Config{
		Updater: &fakeUpdater{release: release("1.0.0", nil)},
		Current: "0.9.1",
		History: &HistoryClient{BaseURL: "http://127.0.0.1:1", App: "captioncraft"},
	})
	if st := m2.Check(context.Background()); st.Notes != "manifest notes 1.0.0" || st.Phase != Available {
		t.Fatalf("%+v", st)
	}
}

func TestPaidUpdateForEarlyAccess(t *testing.T) {
	srv, _ := changelogServer(t, "1.0.0")
	m := New(Config{
		Updater: &fakeUpdater{release: release("1.0.0", nil)},
		Current: "0.9.1",
		History: &HistoryClient{BaseURL: srv.URL, App: "captioncraft"},
		Licence: whitelist(""),
	})
	st := m.Check(context.Background())
	if !st.NeedsPaidLicence || st.PaidSince != "1.0.0" {
		t.Fatalf("%+v", st)
	}
	st = m.Install(context.Background(), false)
	if st.Phase != Available || !errors.Is(st.Err, ErrNeedsPaidLicence) {
		t.Fatalf("%+v", st)
	}
	// The user keeps the version they have.
	m.Dismiss()
	if m.State().Phase != Idle {
		t.Fatal("dismiss")
	}
	// Or accepts.
	m.Check(context.Background())
	if st := m.Install(context.Background(), true); st.Phase != Ready {
		t.Fatalf("%+v", st)
	}

	// Without the changelog, the token's paid_since decides.
	m2 := New(Config{Updater: &fakeUpdater{release: release("1.2.0", nil)}, Current: "0.9.1", Licence: whitelist("1.0.0")})
	if st := m2.Check(context.Background()); !st.NeedsPaidLicence {
		t.Fatalf("%+v", st)
	}
	// Below paid_since, or a paid licence, or no licence: no warning.
	m3 := New(Config{Updater: &fakeUpdater{release: release("0.9.2", nil)}, Current: "0.9.1", History: &HistoryClient{BaseURL: srv.URL, App: "captioncraft"}, Licence: whitelist("")})
	if st := m3.Check(context.Background()); st.NeedsPaidLicence {
		t.Fatalf("%+v", st)
	}
	paid := func() licence.Status {
		return licence.Status{State: licence.Valid, Token: &licence.Token{Edition: licence.EditionPaid}}
	}
	m4 := New(Config{Updater: &fakeUpdater{release: release("1.0.0", nil)}, Current: "0.9.1", History: &HistoryClient{BaseURL: srv.URL, App: "captioncraft"}, Licence: paid})
	if st := m4.Check(context.Background()); st.NeedsPaidLicence {
		t.Fatalf("%+v", st)
	}
}

func TestFailures(t *testing.T) {
	up := &fakeUpdater{err: errors.New("offline")}
	m := New(Config{Updater: up, Current: "0.9.0"})
	st := m.Check(context.Background())
	if st.Phase != Failed || st.Step != "check" {
		t.Fatalf("%+v", st)
	}
	up.mu.Lock()
	up.err, up.release = nil, nil
	up.mu.Unlock()
	if st := m.Check(context.Background()); st.Phase != UpToDate {
		t.Fatalf("%+v", st)
	}
	// An install that fails leaves Failed (the app unchanged), and a new
	// check offers it again.
	up.mu.Lock()
	up.release = release("0.9.1", errors.New("signature mismatch"))
	up.mu.Unlock()
	m.Check(context.Background())
	st = m.Install(context.Background(), false)
	if st.Phase != Failed || st.Step != "download" || !strings.Contains(st.Err.Error(), "signature") {
		t.Fatalf("%+v", st)
	}
	if err := m.Restart(); err == nil {
		t.Fatal("restart after a failure")
	}
	if st := m.Check(context.Background()); st.Phase != Available {
		t.Fatalf("%+v", st)
	}
	// Disabled builds never check.
	d := New(Config{Updater: &fakeUpdater{disabled: true}, Current: "0.9.0"})
	if d.Check(context.Background()).Phase != Disabled {
		t.Fatal("disabled")
	}
}

func TestHistoryAndAutoCheck(t *testing.T) {
	srv, queries := changelogServer(t, "1.0.0")
	m := New(Config{
		Updater:  &fakeUpdater{},
		Current:  "0.9.1",
		History:  &HistoryClient{BaseURL: srv.URL, App: "captioncraft"},
		Language: func() string { return "en" },
	})
	h, err := m.History(context.Background())
	if err != nil || len(h.Entries) != 2 || h.PaidSinceVersion() != "1.0.0" || !h.Entries[0].RequiresPaidLicence {
		t.Fatalf("%+v %v", h, err)
	}
	if !strings.Contains((*queries)[0], "limit=200") || strings.Contains((*queries)[0], "channel=beta") {
		t.Fatal(*queries)
	}
	if _, err := (&HistoryClient{BaseURL: "http://example.com", App: "x"}).Fetch(context.Background(), "en", false, 0); err == nil {
		t.Fatal("plain http accepted")
	}
	if NotesLocale("zh_CN") != "zh-Hans" || NotesLocale("ja") != "en" {
		t.Fatal("NotesLocale")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan State, 4)
	m.Subscribe(func(s State) {
		if s.Phase == UpToDate {
			done <- s
		}
	})
	m.AutoCheck(ctx, 20*time.Millisecond, func() bool { return true })
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("no automatic check")
		}
	}
}

// TestSiteHistory reads a real site's changelog API:
//
//	ACTNOWKIT_SITE=http://localhost:3100 go test -run TestSiteHistory -v ./updates
func TestSiteHistory(t *testing.T) {
	site := os.Getenv("ACTNOWKIT_SITE")
	if site == "" {
		t.Skip("set ACTNOWKIT_SITE to a site (https://actnow.dev or a dev server)")
	}
	h, err := (&HistoryClient{BaseURL: site, App: "audiosmith"}).Fetch(context.Background(), "zh-Hans", true, 5)
	if err != nil {
		t.Fatal(err)
	}
	if h.Locale != "zh-Hans" || len(h.Entries) == 0 {
		t.Fatalf("%+v", h)
	}
	for _, e := range h.Entries {
		t.Logf("%s %s %s (%s): %.60q", e.Version, e.Channel, e.Date, e.Locale, e.Notes)
	}
}
