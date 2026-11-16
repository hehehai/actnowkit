package feedback

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hehehai/actnowkit/redact"
)

func TestValidate(t *testing.T) {
	dir := t.TempDir()
	img := writePNG(t, dir, "a.png", 40, 30, false)
	ok := Draft{Kind: KindBug, Body: "It crashes when I paste", Email: "ann@example.com", Images: []string{img}}
	if ps := Validate(ok, Identity{}); len(ps) != 0 {
		t.Fatalf("valid draft: %v", ps)
	}
	cases := []struct {
		d    Draft
		who  Identity
		want Problem
	}{
		{Draft{Kind: "x", Body: "hello there", Email: "a@b.co"}, Identity{}, ProblemKind},
		{Draft{Kind: KindBug, Body: " hi ", Email: "a@b.co"}, Identity{}, ProblemBodyShort},
		{Draft{Kind: KindBug, Body: strings.Repeat("长", MaxBodyRunes+1), Email: "a@b.co"}, Identity{}, ProblemBodyLong},
		{Draft{Kind: KindBug, Body: "hello there"}, Identity{}, ProblemEmailRequired},
		{Draft{Kind: KindBug, Body: "hello there", Email: "not an email"}, Identity{Linked: true}, ProblemEmailInvalid},
		{Draft{Kind: KindBug, Body: "hello there", Email: "a@b"}, Identity{}, ProblemEmailInvalid},
		{Draft{Kind: KindBug, Body: "hello there", Email: "a@b.co", Images: []string{img, img, img, img}}, Identity{}, ProblemTooManyImages},
		{Draft{Kind: KindBug, Body: "hello there", Email: "a@b.co", Images: []string{filepath.Join(dir, "none.png")}}, Identity{}, ProblemImageMissing},
		{Draft{Kind: KindBug, Body: "hello there", Email: "a@b.co", Video: img}, Identity{}, ProblemVideoType},
	}
	for i, c := range cases {
		ps := Validate(c.d, c.who)
		found := false
		for _, p := range ps {
			found = found || p == c.want
		}
		if !found {
			t.Errorf("case %d: got %v, want %s", i, ps, c.want)
		}
	}
	// Linked: the email may be empty.
	if ps := Validate(Draft{Kind: KindIdea, Body: "hello there"}, Identity{Linked: true}); len(ps) != 0 {
		t.Errorf("linked: %v", ps)
	}
}

func writePNG(t *testing.T, dir, name string, w, h int, noise bool) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	r := rand.New(rand.NewSource(1))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{uint8(x), uint8(y), 128, 255}
			if noise {
				c = color.RGBA{uint8(r.Intn(256)), uint8(r.Intn(256)), uint8(r.Intn(256)), 255}
			}
			img.Set(x, y, c)
		}
	}
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return p
}

// jpegWithOrientation writes a w×h JPEG whose left half is red, with an
// EXIF APP1 holding the orientation (and a GPS-looking marker string).
func jpegWithOrientation(t *testing.T, dir string, w, h, o int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{0, 0, 255, 255}
			if x < w/2 {
				c = color.RGBA{255, 0, 0, 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	raw := buf.Bytes()
	// TIFF: II, 42, IFD at 8 with one entry 0x0112 SHORT 1 = o.
	tiff := []byte("II*\x00\x08\x00\x00\x00")
	tiff = binary.LittleEndian.AppendUint16(tiff, 1)
	tiff = binary.LittleEndian.AppendUint16(tiff, 0x0112)
	tiff = binary.LittleEndian.AppendUint16(tiff, 3)
	tiff = binary.LittleEndian.AppendUint32(tiff, 1)
	tiff = binary.LittleEndian.AppendUint16(tiff, uint16(o))
	tiff = append(tiff, 0, 0, 0, 0, 0, 0)
	tiff = append(tiff, []byte("GPSSECRET")...)
	seg := append([]byte("Exif\x00\x00"), tiff...)
	app1 := []byte{0xFF, 0xE1}
	app1 = binary.BigEndian.AppendUint16(app1, uint16(len(seg)+2))
	app1 = append(app1, seg...)
	out := append(append(append([]byte{}, raw[:2]...), app1...), raw[2:]...)
	p := filepath.Join(dir, "photo.jpg")
	if err := os.WriteFile(p, out, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPrepareImage(t *testing.T) {
	ctx := context.Background()
	src, out := t.TempDir(), t.TempDir()

	// A small screenshot stays a PNG.
	f, err := PrepareImage(ctx, nil, writePNG(t, src, "shot.png", 300, 200, false), out, "image-1")
	if err != nil || f.MIME != "image/png" || f.Width != 300 || f.Bytes > MaxImageBytes || f.Original != "shot.png" {
		t.Fatalf("small png: %+v %v", f, err)
	}

	// Noise does not compress: a big one becomes a JPEG within the limit,
	// scaled to MaxImageSide.
	f, err = PrepareImage(ctx, nil, writePNG(t, src, "noise.png", 4200, 1400, true), out, "image-2")
	if err != nil {
		t.Fatal(err)
	}
	if f.MIME != "image/jpeg" || f.Bytes > MaxImageBytes || f.Width > MaxImageSide {
		t.Fatalf("noise: %+v", f)
	}
	if st, _ := os.Stat(filepath.Join(out, f.Name)); st == nil || st.Size() != f.Bytes {
		t.Fatalf("noise file: %v", st)
	}

	// EXIF orientation 6 (rotate 90° clockwise): 80×40 becomes 40×80, the
	// red half on top; the EXIF is gone.
	f, err = PrepareImage(ctx, nil, jpegWithOrientation(t, src, 80, 40, 6), out, "image-3")
	if err != nil {
		t.Fatal(err)
	}
	if f.Width != 40 || f.Height != 80 {
		t.Fatalf("orientation: %dx%d", f.Width, f.Height)
	}
	data, _ := os.ReadFile(filepath.Join(out, f.Name))
	if bytes.Contains(data, []byte("Exif")) || bytes.Contains(data, []byte("GPSSECRET")) {
		t.Fatal("metadata kept")
	}
	img, _ := jpeg.Decode(bytes.NewReader(data))
	if r, _, b, _ := img.At(20, 10).RGBA(); r < b {
		t.Errorf("top should be red after rotation")
	}

	// HEIC without a converter: unsupported; with one, converted.
	heic := filepath.Join(src, "x.heic")
	os.WriteFile(heic, append([]byte("\x00\x00\x00\x18ftypheic"), make([]byte, 64)...), 0o600)
	if _, err := PrepareImage(ctx, SystemConverterForTest(nil), heic, out, "image-4"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("heic without converter: %v", err)
	}
	conv := &fakeConverter{png: writePNG(t, src, "converted.png", 50, 50, false)}
	if f, err := PrepareImage(ctx, conv, heic, out, "image-4"); err != nil || f.MIME != "image/png" {
		t.Errorf("heic: %+v %v", f, err)
	}
	// Not an image.
	txt := filepath.Join(src, "a.txt")
	os.WriteFile(txt, []byte("hello"), 0o600)
	if _, err := PrepareImage(ctx, nil, txt, out, "x"); !errors.Is(err, ErrNotImage) {
		t.Errorf("text: %v", err)
	}
}

func SystemConverterForTest(c Converter) Converter {
	if c == nil {
		return noConv{}
	}
	return c
}

type noConv struct{}

func (noConv) ToPNG(context.Context, string, string) error { return ErrUnsupported }
func (noConv) CompressVideo(context.Context, string, string, string) error {
	return ErrUnsupported
}

type fakeConverter struct {
	png     string
	sizes   map[string]int64 // preset → output size
	presets []string
}

func (c *fakeConverter) ToPNG(_ context.Context, src, dst string) error {
	data, err := os.ReadFile(c.png)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}

func (c *fakeConverter) CompressVideo(_ context.Context, src, dst, preset string) error {
	c.presets = append(c.presets, preset)
	n, ok := c.sizes[preset]
	if !ok {
		return errors.New("preset not usable")
	}
	return os.WriteFile(dst, movie("mp42", int(n)), 0o600)
}

func movie(brand string, size int) []byte {
	b := make([]byte, size)
	copy(b, "\x00\x00\x00\x18ftyp"+brand)
	return b
}

func TestPrepareVideo(t *testing.T) {
	ctx := context.Background()
	src, out := t.TempDir(), t.TempDir()
	small := filepath.Join(src, "rec.mov")
	os.WriteFile(small, movie("qt  ", 1<<20), 0o600)
	f, err := PrepareVideo(ctx, nil, small, out, "video")
	if err != nil || f.MIME != "video/quicktime" || f.Name != "video.mov" || f.Bytes != 1<<20 {
		t.Fatalf("small: %+v %v", f, err)
	}
	big := filepath.Join(src, "big.mp4")
	os.WriteFile(big, movie("isom", MaxVideoBytes+10), 0o600)
	if _, err := PrepareVideo(ctx, nil, big, out, "video"); !errors.Is(err, ErrTooLarge) {
		t.Errorf("big without converter: %v", err)
	}
	conv := &fakeConverter{sizes: map[string]int64{"PresetHEVC1920x1080": MaxVideoBytes + 5, "Preset1280x720": 3 << 20}}
	f, err = PrepareVideo(ctx, conv, big, out, "video")
	if err != nil || f.Bytes != 3<<20 || f.MIME != "video/mp4" {
		t.Fatalf("compressed: %+v %v", f, err)
	}
	if strings.Join(conv.presets, ",") != "PresetHEVC1920x1080,Preset1280x720" {
		t.Errorf("presets tried: %v", conv.presets)
	}
	conv = &fakeConverter{sizes: map[string]int64{}}
	if _, err := PrepareVideo(ctx, conv, big, out, "video"); !errors.Is(err, ErrTooLarge) {
		t.Errorf("nothing fits: %v", err)
	}
	txt := filepath.Join(src, "a.mp4")
	os.WriteFile(txt, []byte("not a movie at all"), 0o600)
	if _, err := PrepareVideo(ctx, nil, txt, out, "video"); !errors.Is(err, ErrNotVideo) {
		t.Errorf("text: %v", err)
	}
}

func TestCollect(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.Local)
	old := now.Add(-10 * 24 * time.Hour).Format("2006-01-02 15:04:05.000")
	recent := now.Add(-2 * time.Hour).Format("2006-01-02 15:04:05.000")
	os.WriteFile(filepath.Join(dir, "dicta.log.1"), []byte(
		old+" ERROR   app  an old error\n"+
			recent+" ERROR   app  proxy socks5://me:p%ss@10.1.2.3:7897 failed for ann@example.com\n"), 0o600)
	var main strings.Builder
	for i := 0; i < 1500; i++ {
		fmt.Fprintf(&main, "%s INFO    app  line %d in /Users/ann/Library/x token=abcd1234\n", recent, i)
	}
	main.WriteString(recent + " WARN    engine  https://evil.example/%zz?key=AIzaSecretSecretSecretSecretSecret12 Ann's MacBook\n")
	os.WriteFile(filepath.Join(dir, "dicta.log"), []byte(main.String()), 0o600)
	os.WriteFile(filepath.Join(dir, "crash.log"), []byte("=== 2026-11-15 Dicta 1.0 pid 1\npanic: boom\n\ngoroutine 1:\nmain.main()\n\t/Users/ann/src/main.go:10\n"), 0o600)

	pers := redact.Personal{Home: "/Users/ann", Names: []string{"ann", "Ann's MacBook"}}
	d := Collect(CollectConfig{
		LogDir: dir, Logs: []LogSpec{{Name: "dicta.log"}}, CrashFile: "crash.log", Personal: &pers,
		Settings: func() json.RawMessage {
			return json.RawMessage(`{"proxy":"http://u:pw@h:1","folder":"/Users/ann/Dicta"}`)
		},
		Extra: func() map[string]string { return map[string]string{"Engine": "1.2 (ready)"} },
		Scrub: func(s string) string { return strings.ReplaceAll(s, "line 7 ", "line ? ") },
	}, AppInfo{ID: "dicta", Version: "1.0"}, now)

	all := d.Preview()
	for _, leak := range []string{"p%ss", "10.1.2.3", "ann@example.com", "/Users/ann", "abcd1234", "AIzaSecret", "Ann's MacBook", "pw@", "an old error"} {
		if strings.Contains(all, leak) {
			t.Errorf("diagnostics keep %q", leak)
		}
	}
	if len(d.Logs) != 1 || len(d.Logs[0].Lines) != 1000 || !d.Logs[0].Truncated {
		t.Fatalf("tail: %d lines", len(d.Logs[0].Lines))
	}
	if len(d.Problems) != 2 || !strings.Contains(d.Problems[0], "proxy") || !strings.Contains(d.Problems[1], "evil.example") {
		t.Errorf("problems: %q", d.Problems)
	}
	os.WriteFile(filepath.Join(dir, "crash-helper.log"), []byte("panic: helper died\n"), 0o600)
	dh := Collect(CollectConfig{LogDir: dir, CrashFile: "crash.log", CrashFiles: []string{"crash-helper.log", "none.log"}, Personal: &pers}, AppInfo{ID: "dicta"}, now)
	if !strings.Contains(dh.Crash, "--- crash-helper.log ---\npanic: helper died") || !strings.HasPrefix(dh.Crash, "=== 2026-11-15") {
		t.Errorf("crash files: %q", dh.Crash)
	}
	if !strings.HasPrefix(d.Crash, "=== 2026-11-15") || !strings.Contains(d.Crash, "~/src/main.go") {
		t.Errorf("crash: %q", d.Crash)
	}
	if d.Extra["Engine"] != "1.2 (ready)" || !json.Valid(d.Settings) || d.App.Go == "" || d.System.Arch == "" {
		t.Errorf("app parts: %+v", d)
	}

	// Huge logs are cut to fit.
	var huge strings.Builder
	for huge.Len() < 6<<20 {
		huge.WriteString(recent + " ERROR   app  " + strings.Repeat("x", 500) + "\n")
	}
	os.WriteFile(filepath.Join(dir, "dicta.log"), []byte(huge.String()), 0o600)
	d = Collect(CollectConfig{LogDir: dir, Logs: []LogSpec{{Name: "dicta.log", MaxBytes: 4 << 20, Lines: 100000}}, ProblemLines: 100000, Personal: &pers}, AppInfo{ID: "dicta"}, now)
	b, _ := json.Marshal(d)
	if len(b) > MaxDiagnosticsBytes {
		t.Errorf("diagnostics %d bytes", len(b))
	}
}

func newTestManager(t *testing.T, site *fakeSite, token string) *Manager {
	t.Helper()
	m, err := New(Config{
		App: "dicta", Version: "1.0.0", Channel: "beta", BaseURL: site.srv.URL, Dir: t.TempDir(),
		LicenceToken: func() string { return token }, DeviceID: func() string { return "dev123" },
		Locale:    func() string { return "zh-Hans" },
		Collect:   CollectConfig{Personal: &redact.Personal{}},
		Converter: noConv{},
		Sleep:     func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSubmitAndHistory(t *testing.T) {
	site := newFakeSite()
	defer site.Close()
	site.tokens["tok"] = "owner@example.com"
	m := newTestManager(t, site, "tok")
	ctx := context.Background()

	who, err := m.Identity(ctx)
	if err != nil || !who.Linked || who.EmailHint != "o***@example.com" {
		t.Fatalf("identity: %+v %v", who, err)
	}
	var phases []Phase
	m.Subscribe(func(s State) {
		if len(phases) == 0 || phases[len(phases)-1] != s.Phase {
			phases = append(phases, s.Phase)
		}
	})
	src := t.TempDir()
	d := Draft{Kind: KindBug, Body: "The export fails\nsteps: …", Diagnostics: true,
		Images: []string{writePNG(t, src, "a.png", 100, 80, false), writePNG(t, src, "b.png", 60, 60, false)}}
	sub, err := m.Submit(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if sub.Number != "FB-1001" || sub.State != SendSent {
		t.Fatalf("submission: %+v", sub)
	}
	if strings.Join(phasesStr(phases), ",") != "preparing,sending,sent" {
		t.Errorf("phases: %v", phases)
	}
	it := site.items[sub.ID]
	if !it.Completed || len(it.Files) != 3 || it.Email != "owner@example.com" || it.Req.DeviceID != "dev123" || it.Req.Locale != "zh-Hans" {
		t.Fatalf("site item: %+v", it)
	}
	var diag Diagnostics
	for _, ff := range it.Files {
		if ff.Meta.Role == RoleDiagnostics {
			json.Unmarshal(ff.Data, &diag)
		}
	}
	if diag.Schema != DiagnosticsSchema || diag.App.ID != "dicta" {
		t.Errorf("diagnostics: %+v", diag)
	}
	if len(m.Pending()) != 0 {
		t.Errorf("outbox not empty")
	}

	h := m.History()
	if len(h) != 1 || h[0].Number != "FB-1001" || h[0].Summary != "The export fails" {
		t.Fatalf("history: %+v", h)
	}
	// A reply from support: Refresh shows it unread; Thread marks it seen.
	site.items[sub.ID].Messages = append(site.items[sub.ID].Messages, Message{ID: "m1", Author: "support", Body: "Fixed in 1.0.1"})
	site.items[sub.ID].Status = StatusResolved
	h, err = m.Refresh(ctx)
	if err != nil || h[0].Status != StatusResolved || !h[0].Unread() || m.Unread() != 1 {
		t.Fatalf("refresh: %+v %v", h, err)
	}
	th, err := m.Thread(ctx, sub.ID)
	if err != nil || len(th.Messages) != 1 || m.Unread() != 0 {
		t.Fatalf("thread: %+v %v", th, err)
	}
	if msg, err := m.Reply(ctx, sub.ID, "Thanks!"); err != nil || msg.Author != "user" {
		t.Fatalf("reply: %+v %v", msg, err)
	}
	if h, _ := m.Refresh(ctx); h[0].Unread() || m.Unread() != 0 {
		t.Errorf("own reply counted unread: %+v", h[0])
	}
	if !strings.HasPrefix(m.WebURL(), site.srv.URL+"/feedback?app=dicta&lang=zh-Hans&v=1.0.0") {
		t.Errorf("web url: %s", m.WebURL())
	}
	if err := m.Forget(sub.ID); err != nil || len(m.History()) != 0 {
		t.Errorf("forget: %v", err)
	}
}

func phasesStr(ps []Phase) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = string(p)
	}
	return out
}

func TestFallbackExpiryAndMissing(t *testing.T) {
	site := newFakeSite()
	defer site.Close()
	m := newTestManager(t, site, "")
	ctx := context.Background()
	src := t.TempDir()
	img := writePNG(t, src, "a.png", 50, 50, false)

	// The bucket cannot be reached: the files go through the site.
	site.directDown = true
	sub, err := m.Submit(ctx, Draft{Kind: KindIdea, Body: "Add dark mode please", Email: "a@example.com", Images: []string{img}})
	if err != nil || !site.items[sub.ID].Completed || site.count("PUT /api/feedback/v1/upload/") != 1 {
		t.Fatalf("fallback: %v %v", err, site.calls)
	}
	site.directDown = false

	// No direct URL at all (a site without presign credentials): the
	// fallback with the token, not a bare PUT.
	site.noDirect = true
	before := site.count("PUT /api/feedback/v1/upload/")
	sub, err = m.Submit(ctx, Draft{Kind: KindIdea, Body: "Through the site only", Email: "a@example.com", Images: []string{img}})
	if err != nil || site.count("PUT /api/feedback/v1/upload/") != before+1 {
		t.Fatalf("no direct: %v %v", err, site.calls)
	}
	site.noDirect = false

	// An expired URL: new ones are asked for.
	site.expireFirst = 1
	sub, err = m.Submit(ctx, Draft{Kind: KindIdea, Body: "Another idea here", Email: "a@example.com", Images: []string{img}})
	if err != nil || site.count("POST /api/feedback/v1/uploads") != 1 {
		t.Fatalf("expired: %v %v", err, site.calls)
	}

	// complete says a file is missing: it is uploaded again.
	site.completeDrop = "image-1"
	sub, err = m.Submit(ctx, Draft{Kind: KindIdea, Body: "Third idea here", Email: "a@example.com", Images: []string{img}})
	if err != nil || !site.items[sub.ID].Completed {
		t.Fatalf("missing: %v", err)
	}
}

func TestFailuresStayInOutbox(t *testing.T) {
	site := newFakeSite()
	defer site.Close()
	m := newTestManager(t, site, "")
	ctx := context.Background()

	// The site is down for longer than the retries: kept, then Resume.
	site.failCreate = 10
	_, err := m.Submit(ctx, Draft{Kind: KindBug, Body: "Crash on start", Email: "a@example.com", Diagnostics: true})
	if !errors.Is(err, ErrNetwork) {
		t.Fatalf("want network error, got %v", err)
	}
	if st := m.State(); st.Phase != Failed || len(st.Pending) != 1 {
		t.Fatalf("state: %+v", st)
	}
	site.failCreate = 0
	m.Resume(ctx)
	if len(m.Pending()) != 0 || len(m.History()) != 1 {
		t.Fatalf("resume: pending %d history %d", len(m.Pending()), len(m.History()))
	}

	// Rejected (no email for an unlinked sender): not retried by Resume.
	m2 := newTestManager(t, site, "")
	m2.mu.Lock()
	m2.who = &Identity{Linked: true, Known: true} // the app believed it was linked
	m2.mu.Unlock()
	_, err = m2.Submit(ctx, Draft{Kind: KindBug, Body: "Crash on start"})
	var api *APIError
	if !errors.As(err, &api) || api.Code != "email_required" {
		t.Fatalf("want email_required, got %v", err)
	}
	p := m2.Pending()
	if len(p) != 1 || !p[0].Rejected || p[0].ErrorCode != "email_required" {
		t.Fatalf("pending: %+v", p)
	}
	creates := site.count("POST /api/feedback/v1/create")
	m2.Resume(ctx)
	if site.count("POST /api/feedback/v1/create") != creates {
		t.Error("a rejected submission was sent again by Resume")
	}
	if err := m2.Discard(p[0].LocalID); err != nil || len(m2.Pending()) != 0 {
		t.Errorf("discard: %v", err)
	}

	// Rate limited: kept for later, no retry loop.
	site.rateLimited = true
	_, err = m.Submit(ctx, Draft{Kind: KindBug, Body: "Too many", Email: "a@example.com"})
	if !errors.As(err, &api) || api.Code != "rate_limited" {
		t.Fatalf("want rate_limited, got %v", err)
	}
	if p := m.Pending(); len(p) != 1 || p[0].Rejected {
		t.Fatalf("rate limited pending: %+v", p)
	}
	// Old entries leave the outbox.
	m.cfg.Now = func() time.Time { return time.Now().Add(OutboxMaxAge + time.Hour) }
	if len(m.Pending()) != 0 {
		t.Error("old submission kept")
	}
}

func TestNewChecksBase(t *testing.T) {
	for _, base := range []string{"http://actnow.dev", "ftp://x", "::"} {
		if _, err := New(Config{App: "x", Dir: t.TempDir(), BaseURL: base}); err == nil {
			t.Errorf("%s accepted", base)
		}
	}
	if _, err := New(Config{App: "x", Dir: t.TempDir(), BaseURL: "https://refto.one", WebPath: "/app/feedback"}); err != nil {
		t.Error(err)
	}
}

func TestLastCrash(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "crash.log")
	cases := map[string]string{
		// The running session's line comes last, with nothing after it.
		"=== Refto 0.1 pid 1 ===\npanic: boom\n\ngoroutine 1:\nmain.main()\n=== Refto 0.1 pid 2 ===\n": "=== Refto 0.1 pid 1 ===\npanic: boom\n\ngoroutine 1:\nmain.main()",
		"=== a ===\n=== b ===\n":           "",
		"panic: old\nx\n":                  "panic: old\nx",
		"":                                 "",
		"=== a ===\nfatal error: maps\n\n": "=== a ===\nfatal error: maps",
	}
	for in, want := range cases {
		os.WriteFile(p, []byte(in), 0o600)
		if got := lastCrash(p); got != want {
			t.Errorf("lastCrash(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExifOrientationBounds(t *testing.T) {
	for _, b := range [][]byte{nil, {0xFF, 0xD8}, {0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x01}, {0xFF, 0xD8, 0xFF, 0xE1, 0xFF, 0xFF, 'E'}} {
		if o := exifOrientation(b); o != 1 {
			t.Errorf("%v: %d", b, o)
		}
	}
}
