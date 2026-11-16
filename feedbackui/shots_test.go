package feedbackui

// Snapshots of the window, as the design's scenes (threads/84-ui-r0):
//
//	FBUI_SHOTS=<dir> [FBUI_ART=<the design's src>] go test -run TestShots
//
// FBUI_ART holds shot1.png, shot2.png, shot3.png and video.png (the
// design's pictures); without it the attachments are plain colors.

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/actnowkit/feedback"
)

func TestShots(t *testing.T) {
	out := os.Getenv("FBUI_SHOTS")
	if out == "" {
		t.Skip("FBUI_SHOTS not set")
	}
	os.MkdirAll(out, 0o755)
	scenes := []string{"empty", "files", "preview", "video", "entry", "context", "sending", "done"}
	for i, scene := range scenes {
		for _, lang := range []string{"en", "zh"} {
			for _, dark := range []bool{false, true} {
				theme := "light"
				if dark {
					theme = "dark"
				}
				name := fmt.Sprintf("%02d-%s-%s-%s.png", i+1, scene, lang, theme)
				img := shot(t, scene, lang, dark)
				f, err := os.Create(filepath.Join(out, name))
				if err != nil {
					t.Fatal(err)
				}
				png.Encode(f, img)
				f.Close()
			}
		}
	}
}

func shot(t *testing.T, scene, lang string, dark bool) image.Image {
	e := newEnv(t, "tok")
	e.site.Tokens["tok"] = "river@gmail.com"
	e.lang = lang
	fs := artFiles(t, e)
	body := map[string]string{
		"en": "Transcribing a 2-hour meeting stops at 63% every time. The engine log says it ran out of memory, though Activity Monitor shows 9 GB free.",
		"zh": "转写一个 2 小时的会议，每次都停在 63%。引擎日志说内存不足，但活动监视器显示还有 9 GB 空闲。",
	}[lang]
	tt := e.tt
	if scene == "entry" {
		tt = ui.NewTester(func(c *ui.Context) { entryView(c, e.w, capture(t, e.m), lang) }, 1000, 300)
	}
	tt.SetScale(2)
	tt.SetDark(dark)
	switch scene {
	case "empty":
		e.w.Open(feedback.Prefill{})
	case "files", "preview", "video", "sending":
		e.w.Open(feedback.Prefill{Body: body, Images: fs[:3], Video: fs[3]})
		switch scene {
		case "preview":
			e.w.viewer = 0
		case "video":
			e.w.viewer = 3
		case "sending":
			sub := &feedback.Submission{LocalID: "shot"}
			e.w.mine = "shot"
			e.w.stateChanged(feedback.State{Phase: feedback.Sending, Current: sub, SentBytes: 13002342, TotalBytes: 21076377})
		}
	case "context":
		e.w.Open(feedback.Prefill{Kind: feedback.KindBug, Report: capture(t, e.m)})
	case "done":
		e.w.Open(feedback.Prefill{Body: body})
		e.w.mine = "shot"
		e.w.history = []feedback.HistoryEntry{{ID: "fb_1"}, {ID: "fb_2"}}
		e.w.stateChanged(feedback.State{Phase: feedback.Sent, Current: &feedback.Submission{LocalID: "shot", ID: "fb_1", Number: "FB-1042"}})
	}
	if scene != "done" {
		e.w.history = []feedback.HistoryEntry{{ID: "fb_1"}, {ID: "fb_2"}}
	}
	tt.Frame()
	tt.Frame()
	tt.Frame()
	return tt.Image()
}

// artFiles are the design's pictures (FBUI_ART), or plain ones.
func artFiles(t *testing.T, e *env) []string {
	art := os.Getenv("FBUI_ART")
	d := filepath.Join(e.dir, "art")
	os.MkdirAll(d, 0o700)
	var out []string
	for i, c := range []color.RGBA{{240, 240, 240, 255}, {30, 30, 30, 255}, {250, 245, 235, 255}} {
		name := fmt.Sprintf("shot%d.png", i+1)
		p := filepath.Join(d, name)
		if data, err := os.ReadFile(filepath.Join(art, name)); art != "" && err == nil {
			os.WriteFile(p, data, 0o600)
		} else {
			writePNG(t, p, solid(800, 500, c))
		}
		out = append(out, p)
	}
	if data, err := os.ReadFile(filepath.Join(art, "video.png")); art != "" && err == nil {
		os.WriteFile(filepath.Join(d, "frame.png"), data, 0o600)
	}
	return append(out, movie(t, filepath.Join(d, "screen-recording.mov")))
}

// entryView is an app showing an error with "Report this problem…".
func entryView(c *ui.Context, w *Window, rep *feedback.Report, lang string) {
	p := w.opt.Theme.palette(c.Theme())
	msg, retry := "Transcription failed: the engine ran out of memory.", "Try again"
	if lang == "zh" {
		msg, retry = "转写失败：引擎内存不足。", "重试"
	}
	ui.Row(c).Fill().Background(p.Track).Children(func() {
		ui.Column(c).Width(200).FillHeight().Background(p.Raised).Padding(46, 12).Gap(4).Children(func() {
			for range 6 {
				ui.Box(c).Height(26).Radius(6).Background(p.Separator).Opacity(0.6)
			}
		})
		ui.Column(c).Grow(1).Padding(60, 30).Children(func() {
			ui.Row(c).AlignItems(ui.Start).Gap(10).Padding(12, 14).Radius(12).Background(p.Background).Border(1, p.Border).
				Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.12)).Children(func() {
				ui.Box(c).Size(18, 18).Radius(9).Background(p.Danger).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Children(func() {
					ui.Text(c, "!").FontSize(12).FontWeight(700).TextColor(ui.Hex("#ffffff"))
				})
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, msg).FontSize(13).FontWeight(600).TextColor(p.Text)
					ui.Text(c, rep.Code+" · "+rep.At.Local().Format("15:04:05")).FontSize(11.5).TextColor(p.TextMuted)
				})
				f := w.frame(c)
				f.button("retry", retry)
				ReportLink(c, w, rep).AlignSelf(ui.Center)
			})
		})
	})
}
