package feedbackui

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"context"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/actnowkit/feedback"
	"github.com/hehehai/actnowkit/feedback/feedbacktest"
)

// fakeHost is the window on screen, counted.
type fakeHost struct {
	opened, focused, closed, invalidated int
}

func (h *fakeHost) native() *mygo.Window { return nil }
func (h *fakeHost) focus()               { h.focused++ }
func (h *fakeHost) close()               { h.closed++ }
func (h *fakeHost) invalidate()          { h.invalidated++ }

// env is a window over a fake site, in a Tester.
type env struct {
	t      *testing.T
	w      *Window
	m      *feedback.Manager
	site   *feedbacktest.Site
	tt     *ui.Tester
	host   *fakeHost
	hosts  int
	dir    string
	picked []string
	opened []string // OpenFile
	urls   []string
	lang   string
}

// newEnv makes a Manager (licence token tok, "" none) and its window.
func newEnv(t *testing.T, tok string) *env {
	t.Helper()
	e := &env{t: t, site: feedbacktest.New(), dir: t.TempDir(), lang: "en"}
	t.Cleanup(e.site.Close)
	logDir := filepath.Join(e.dir, "logs")
	os.MkdirAll(logDir, 0o700)
	var lines []string
	for i := range 60 {
		lines = append(lines, fmt.Sprintf("2026-11-16T10:42:%02dZ INFO step %d", i%60, i))
	}
	os.WriteFile(filepath.Join(logDir, "dicta.log"), []byte(strings.Join(lines, "\n")+"\n"), 0o600)
	m, err := feedback.New(feedback.Config{
		App: "dicta", Version: "0.1.0-beta.7", BaseURL: e.site.URL, Dir: filepath.Join(e.dir, "feedback"),
		HTTPClient:   e.site.Client(),
		LicenceToken: func() string { return tok },
		Locale:       func() string { return e.lang },
		Collect:      feedback.CollectConfig{LogDir: logDir, Logs: []feedback.LogSpec{{Name: "dicta.log"}}},
		Sleep:        func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	e.m = m
	e.w = New(Options{
		Manager: m, AppName: "Dicta", Site: "actnow.dev", PrivacyURL: "https://actnow.dev/legal/privacy",
		Lang:      func() string { return e.lang },
		Post:      func(fn func()) { fn() },
		Async:     func(fn func()) { fn() },
		PickFiles: func(*mygo.Window, string) ([]string, error) { return e.picked, nil },
		OpenFile:  func(p string) error { e.opened = append(e.opened, p); return nil },
		OpenURL:   func(u string) { e.urls = append(e.urls, u) },
		Thumbnail: testThumbnail,
	})
	e.w.newHost = func(*Window) host {
		e.hosts++
		e.host = &fakeHost{}
		return e.host
	}
	t.Cleanup(e.w.Stop)
	e.tt = ui.NewTester(e.w.View, WindowWidth, WindowHeight)
	return e
}

// testThumbnail decodes the images; a video's first frame is a picture
// of its own (no Quick Look here).
func testThumbnail(path string, px int) (image.Image, error) {
	if isVideo(path) {
		if img, err := decodePNG(filepath.Join(filepath.Dir(path), "frame.png")); err == nil {
			return img, nil
		}
		return solid(320, 200, color.RGBA{20, 30, 40, 255}), nil
	}
	return feedback.Thumbnail(context.Background(), nil, path, px)
}

func decodePNG(p string) (image.Image, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	return img
}

func writePNG(t *testing.T, p string, img image.Image) string {
	t.Helper()
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return p
}

// movie is a QuickTime file of 42 s (ftyp, moov/mvhd, some mdat).
func movie(t *testing.T, p string) string {
	t.Helper()
	box := func(typ string, body []byte) []byte {
		b := make([]byte, 8, 8+len(body))
		binary.BigEndian.PutUint32(b, uint32(8+len(body)))
		copy(b[4:], typ)
		return append(b, body...)
	}
	mvhd := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhd[12:], 1000)  // time scale
	binary.BigEndian.PutUint32(mvhd[16:], 42000) // duration
	data := append(box("ftyp", []byte("qt  \x00\x00\x02\x00qt  ")), box("moov", box("mvhd", mvhd))...)
	data = append(data, box("mdat", make([]byte, 4096))...)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// files makes three screenshots and a recording.
func (e *env) files() []string {
	d := filepath.Join(e.dir, "pick")
	os.MkdirAll(d, 0o700)
	var out []string
	for i, c := range []color.RGBA{{240, 240, 240, 255}, {30, 30, 30, 255}, {250, 245, 235, 255}} {
		out = append(out, writePNG(e.t, filepath.Join(d, fmt.Sprintf("shot%d.png", i+1)), solid(400, 250, c)))
	}
	return append(out, movie(e.t, filepath.Join(d, "screen-recording.mov")))
}

func (e *env) frame() { e.tt.Frame(); e.tt.Frame() }

func (e *env) click(s string) {
	e.t.Helper()
	e.frame()
	if err := e.tt.Click(s); err != nil {
		e.t.Fatalf("click %q: %v\n%s", s, err, strings.Join(e.tt.Texts(), " | "))
	}
	e.frame()
}

func (e *env) has(s string) {
	e.t.Helper()
	if !e.tt.HasText(s) {
		e.t.Fatalf("no %q in\n%s", s, strings.Join(e.tt.Texts(), " | "))
	}
}

func (e *env) hasNot(s string) {
	e.t.Helper()
	if e.tt.HasText(s) {
		e.t.Fatalf("%q shows", s)
	}
}

func TestEmptyFormValidates(t *testing.T) {
	e := newEnv(t, "")
	e.w.Open(feedback.Prefill{})
	e.frame()
	e.has("Send Feedback")
	e.has("to the Dicta team · actnow.dev")
	// Send waits for a description.
	e.click("Send")
	if e.site.Count("POST /api/feedback/v1/create") != 0 {
		t.Fatal("sent an empty form")
	}
	// Too short, and no email (no licence): both problems show.
	e.w.f.Body = "hi"
	e.click("Send")
	e.has(feedback.TextProblemBodyShort.EN)
	e.has(feedback.TextProblemEmailRequired.EN)
	if e.site.Count("POST /api/feedback/v1/create") != 0 {
		t.Fatal("sent an incomplete form")
	}
	e.w.f.Email = "not an email"
	e.w.f.Body = "Export stops at 99%."
	e.click("Send")
	e.has(feedback.TextProblemEmailInvalid.EN)
	e.hasNot(feedback.TextProblemBodyShort.EN)
	e.click("Privacy policy")
	if len(e.urls) != 1 || e.urls[0] != "https://actnow.dev/legal/privacy" {
		t.Fatalf("opened %v", e.urls)
	}
}

func TestThemeDrawsControls(t *testing.T) {
	e := newEnv(t, "")
	var drawn []string
	e.w.opt.Theme.PrimaryButton = func(c *ui.Context, label string) ui.Element {
		drawn = append(drawn, label)
		return ui.Button(c, label)
	}
	e.w.Open(feedback.Prefill{})
	e.frame()
	if len(drawn) == 0 || drawn[len(drawn)-1] != "Send" {
		t.Fatalf("drawn %v", drawn)
	}
	// The palettes follow the appearance.
	e.tt.SetDark(true)
	e.frame()
	img := e.tt.Image()
	if c := img.RGBAAt(5, 300); c.R > 40 {
		t.Fatalf("dark background %v", c)
	}
}

func TestGridAddsAndRemoves(t *testing.T) {
	e := newEnv(t, "")
	fs := e.files()
	e.picked = fs
	e.w.Open(feedback.Prefill{})
	e.click("Add images or a video")
	if len(e.w.f.Images) != 3 || e.w.f.Video != fs[3] {
		t.Fatalf("form: %+v", e.w.f)
	}
	e.has("3 of 3 images · 1 of 1 video")
	e.has("0:42") // the video's length
	if _, ok := e.tt.Find("Add images or a video"); ok {
		t.Fatal("the Add tile shows with no room left")
	}
	// A fourth image has no room.
	e.w.addFiles([]string{fs[0]})
	e.frame()
	e.has(feedback.TextProblemTooManyImages.Fill("max", "3").EN)
	e.click("Remove shot2.png")
	if len(e.w.f.Images) != 2 || e.w.f.Images[1] != fs[2] {
		t.Fatalf("images after removing: %v", e.w.f.Images)
	}
	e.has("2 of 3 images · 1 of 1 video")
	e.click("Remove screen-recording.mov")
	if e.w.f.Video != "" {
		t.Fatal("the video stays")
	}
	e.has("Add images or a video")
	// Dropping a video puts it back.
	e.w.addFiles([]string{fs[3]})
	if e.w.f.Video != fs[3] {
		t.Fatal("no video after the drop")
	}
}

func TestPreviewOpensAndCloses(t *testing.T) {
	e := newEnv(t, "")
	fs := e.files()
	e.w.Open(feedback.Prefill{Images: fs[:3], Video: fs[3]})
	e.click("shot1.png")
	if e.w.viewer != 0 {
		t.Fatalf("viewer %d", e.w.viewer)
	}
	e.has("Image 1 of 3 · shot1.png")
	e.click("Next")
	e.has("Image 2 of 3 · shot2.png")
	e.tt.Key(0, ui.KeyRight)
	e.frame()
	e.has("Image 3 of 3 · shot3.png")
	e.click("Next")
	e.has("Video · screen-recording.mov")
	e.click("Open in QuickTime Player")
	if len(e.opened) != 1 || e.opened[0] != fs[3] {
		t.Fatalf("opened %v", e.opened)
	}
	e.click("Previous")
	e.click("Remove")
	if len(e.w.f.Images) != 2 {
		t.Fatalf("images %v", e.w.f.Images)
	}
	e.tt.Key(0, ui.KeyEscape)
	e.frame()
	if e.w.viewer != -1 {
		t.Fatalf("the preview stays open: %d", e.w.viewer)
	}
	e.hasNot("Image 1 of 2")
}

func capture(t *testing.T, m *feedback.Manager) *feedback.Report {
	t.Helper()
	m.Step("import_audio")
	m.Step("transcribe")
	m.Step("speakers_on")
	rep, err := m.Capture(feedback.ErrorContext{Title: "Transcription failed", Code: "E-ASR-0042", Kind: "engine_out_of_memory",
		Where: "Meetings › Transcribe", Err: errors.New("engine: out of memory")})
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestReportPrefillAndUntick(t *testing.T) {
	for _, include := range []bool{true, false} {
		e := newEnv(t, "tok")
		e.site.Tokens["tok"] = "river@gmail.com"
		rep := capture(t, e.m)
		e.w.Open(feedback.Prefill{Kind: feedback.KindBug, Report: rep})
		e.frame()
		e.has("Report this problem")
		e.has("Transcription failed")
		e.has("engine_out_of_memory (E-ASR-0042)")
		e.has("Import audio → Transcribe → Speakers on")
		e.has(rep.ID + " (40 log lines)")
		e.has("We'll reply to r***@gmail.com")
		label := "Include this error report and its 40 log lines"
		if !include {
			e.click(label)
			if e.w.f.IncludeReport {
				t.Fatal("still included")
			}
		}
		e.click("See what is sent")
		if !e.w.sheet.open || !strings.Contains(e.w.sheet.text, rep.ID) {
			t.Fatalf("report sheet: %+v", e.w.sheet)
		}
		e.click("Done")
		e.w.f.Body = "Transcribing a 2-hour meeting stops at 63% every time."
		e.click("Send")
		var it *feedbacktest.Item
		for _, x := range e.site.Items() {
			it = x
		}
		if it == nil {
			t.Fatal("not sent")
		}
		if got := it.Request.Context != nil; got != include {
			t.Fatalf("include %v: context %+v", include, it.Request.Context)
		}
		if include && it.Request.Context.ReportID != rep.ID {
			t.Fatalf("context %+v", it.Request.Context)
		}
	}
}

func TestSendSucceeds(t *testing.T) {
	e := newEnv(t, "tok")
	e.site.Tokens["tok"] = "river@gmail.com"
	fs := e.files()
	e.w.Open(feedback.Prefill{Images: fs[:1]})
	e.w.f.Body = "Export to SRT stops at 99% for a two-hour meeting."
	e.click("Send")
	e.has("Thanks — feedback sent")
	e.has("FB-1042")
	it := e.site.Items()["fb_1"]
	if it == nil || !it.Completed || len(it.Files) != 2 {
		t.Fatalf("site: %+v", it)
	}
	if e.w.f.Body != "" || len(e.w.f.Images) != 0 {
		t.Fatal("the form stays filled")
	}
	e.site.SupportReplies("fb_1", "Thanks! Which macOS version is this?")
	e.click("View it")
	e.has("Export to SRT stops at 99%")
	e.has("Which macOS version is this?")
	e.w.replyText = "macOS 26.1"
	e.click("Reply")
	if n := len(e.site.Items()["fb_1"].Messages); n != 2 {
		t.Fatalf("messages %d", n)
	}
	e.click("Back")
	e.has("Your feedback")
	e.has("FB-1042")
	e.click("Back")
	e.has("Your feedback (1)")
}

func TestSendFailsThenRetries(t *testing.T) {
	e := newEnv(t, "")
	e.site.FailCreate = 100
	e.w.Open(feedback.Prefill{})
	e.w.f.Body, e.w.f.Email = "The app quits when I paste.", "ann@example.com"
	e.click("Send")
	if !e.w.failed() {
		t.Fatalf("state %+v", e.w.st)
	}
	e.has("Try again")
	e.has(feedback.TextFailed.EN)
	e.site.Mu.Lock()
	e.site.FailCreate = 0
	e.site.Mu.Unlock()
	e.click("Try again")
	e.has("Thanks — feedback sent")
	if len(e.m.Pending()) != 0 {
		t.Fatal("still pending")
	}

	// Discard: back to the form as it was, nothing waiting.
	e.click("Close")
	e.site.FailCreate = 100
	e.w.Open(feedback.Prefill{})
	e.w.f.Body, e.w.f.Email = "Another problem here.", "ann@example.com"
	e.click("Send")
	e.click("Discard")
	if e.w.failed() || len(e.m.Pending()) != 0 {
		t.Fatalf("after discard: %+v %d", e.w.st, len(e.m.Pending()))
	}
	if e.w.f.Body != "Another problem here." {
		t.Fatal("the form was cleared")
	}
	e.has("Send")
}

func TestOpenIsOneWindow(t *testing.T) {
	e := newEnv(t, "")
	e.w.Open(feedback.Prefill{})
	e.w.Open(feedback.Prefill{})
	if e.hosts != 1 || e.host.focused != 2 {
		t.Fatalf("hosts %d, focused %d", e.hosts, e.host.focused)
	}
	// What the user wrote stays; a report adds its context.
	e.w.f.Body = "Something odd."
	rep := capture(t, e.m)
	e.w.Open(feedback.Prefill{Report: rep, Body: "ignored"})
	if e.hosts != 1 || e.w.f.Body != "Something odd." || e.w.f.Report != rep || !e.w.f.IncludeReport {
		t.Fatalf("form %+v", e.w.f)
	}
	e.frame()
	e.has("Report this problem")
	e.click("Close")
	if e.w.IsOpen() || e.host.closed != 1 {
		t.Fatal("not closed")
	}
	e.w.Open(feedback.Prefill{})
	if e.hosts != 2 || e.w.f.Body != "Something odd." {
		t.Fatalf("reopened: hosts %d, body %q", e.hosts, e.w.f.Body)
	}
	// Cancel drops the draft.
	e.click("Cancel")
	e.w.Open(feedback.Prefill{})
	if e.w.f.dirty() || e.w.f.Report != nil {
		t.Fatalf("after cancel %+v", e.w.f)
	}
}

func TestReportLinkAndPanel(t *testing.T) {
	e := newEnv(t, "")
	rep := capture(t, e.m)
	app := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			ui.Text(c, "Transcription failed: the engine ran out of memory.")
			ReportLink(c, e.w, rep)
			e.w.HistoryPanel(c)
		})
	}, 600, 400)
	app.Frame()
	if err := app.Click("Report this problem…"); err != nil {
		t.Fatal(err)
	}
	if !e.w.IsOpen() || e.w.f.Report != rep || e.w.f.Kind != 0 {
		t.Fatalf("open %v, form %+v", e.w.IsOpen(), e.w.f)
	}
	app.Frame()
	if err := app.Click("Open the feedback window"); err != nil {
		t.Fatal(err)
	}
	if e.hosts != 1 {
		t.Fatalf("hosts %d", e.hosts)
	}
}

func TestChineseTexts(t *testing.T) {
	e := newEnv(t, "")
	e.lang = "zh-Hans"
	e.w.Open(feedback.Prefill{})
	e.frame()
	for _, s := range []string{"发送反馈", "发给 Dicta 团队 · actnow.dev", "问题", "建议", "提问", "其他", "添加", "图片或视频", "附上诊断信息", "提交", "取消"} {
		e.has(s)
	}
	// An app's own wording, and its catalogue.
	tx := DefaultTexts()
	tx.Send.ZH = "发送"
	e.w.texts = tx
	e.frame()
	e.has("发送")
}

func TestCancelWhileSending(t *testing.T) {
	e := newEnv(t, "")
	e.w.Open(feedback.Prefill{Body: "Keep this text, please."})
	cancelled := false
	e.w.cancel = func() { cancelled = true }
	e.w.awaiting = true
	sub := &feedback.Submission{LocalID: "x"}
	e.w.stateChanged(feedback.State{Phase: feedback.Sending, Current: sub, SentBytes: 1 << 20, TotalBytes: 4 << 20})
	if e.w.mine != "" {
		t.Fatal("took a state without Preparing first")
	}
	e.w.stateChanged(feedback.State{Phase: feedback.Preparing, Current: sub})
	e.w.stateChanged(feedback.State{Phase: feedback.Sending, Current: sub, SentBytes: 1 << 20, TotalBytes: 4 << 20})
	e.frame()
	e.has("Sending… 1.0 / 4.0 MB")
	e.click("Cancel")
	if !cancelled || !e.w.cancelled {
		t.Fatal("not cancelled")
	}
	e.w.stateChanged(feedback.State{Phase: feedback.Failed, Current: sub, Step: "upload", Err: context.Canceled})
	if e.w.busy() || e.w.failed() || e.w.mine != "" || e.w.f.Body != "Keep this text, please." {
		t.Fatalf("after cancel: mine %q, form %+v", e.w.mine, e.w.f)
	}
	if !e.w.IsOpen() {
		t.Fatal("cancelling the sending closed the window")
	}
}
