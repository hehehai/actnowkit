package feedbackui

// The layers over the window: the preview of an attachment (an image
// enlarged, the video played) and "What is sent".

import (
	"encoding/json"
	"path/filepath"
	"strconv"

	"github.com/egoist/mygo/ui"
)

// sheet is "What is sent": the diagnostics, or the error report.
type sheet struct {
	open        bool
	title, sub  string
	text        string
	loading     bool
	copied      bool
	diagnostics bool
}

func (w *Window) openDiagnosticsSheet() {
	if w.m == nil {
		return
	}
	if w.opt.BeforeCollect != nil {
		w.opt.BeforeCollect()
	}
	w.sheet = sheet{open: true, title: w.tx(w.texts.WhatIsSent), sub: w.tx(w.texts.WhatIsSentDiag), loading: true, diagnostics: true}
	m, email := w.m, w.f.Email
	w.async(func() {
		text := m.Diagnostics(email).Preview()
		w.post(func() {
			if w.sheet.open && w.sheet.diagnostics {
				w.sheet.text, w.sheet.loading = text, false
			}
		})
	})
}

func (w *Window) openReportSheet() {
	r := w.f.Report
	if r == nil {
		return
	}
	data, _ := json.MarshalIndent(r, "", "  ")
	w.sheet = sheet{open: true, title: w.tx(w.texts.WhatIsSent), sub: w.tx(w.texts.WhatIsSentReport), text: string(data)}
}

func (w *Window) sheetLayer(f *frame) {
	if !w.sheet.open {
		return
	}
	c, p := f.c, f.p
	cw, ch := c.Size()
	ui.DialogBase(c.Key("fb-sheet"), &w.sheet.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.3))
		panel.Width(min(cw-40, 560)).MaxHeight(ch-40).Background(p.Background).Radius(12).Border(1, p.Border).
			Shadow(0, 12, 40, 0, ui.RGBA(0, 0, 0, 0.3)).Padding(16, padX).Gap(10).Label(w.sheet.title)
		panel.Children(func() {
			ui.Column(c).Gap(2).Shrink(0).Children(func() {
				ui.Text(c, w.sheet.title).FontSize(15).FontWeight(600).TextColor(p.Text).Role(ui.RoleHeading)
				f.muted(w.sheet.sub)
			})
			ui.Scroll(c.Key("fb-sheet-scroll")).Grow(1).MinHeight(80).Background(p.Raised).Border(1, p.Border).Radius(f.r + 1).Children(func() {
				if w.sheet.loading {
					ui.Row(c).Padding(10).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Spinner(c).Size(14, 14)
						f.muted(w.tx(w.texts.Loading))
					})
					return
				}
				ui.Text(c, w.sheet.text).Font(f.th.mono()).FontSize(11).TextColor(p.Text).Padding(10).Selectable()
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
				if w.sheet.copied {
					f.muted(w.tx(w.texts.Copied))
				}
				ui.Box(c).Grow(1)
				if f.button("fb-sheet-copy", w.tx(w.texts.Copy)).Disabled(w.sheet.loading).Clicked() {
					c.WriteClipboard(w.sheet.text)
					w.sheet.copied = true
				}
				if f.primary("fb-sheet-done", w.tx(w.texts.Done), false).Clicked() {
					w.sheet = sheet{}
				}
			})
		})
	})
}

// viewerLayer previews the attachment w.viewer: an image fitted to the
// window, or the video in a web view; ‹ › go through them.
func (w *Window) viewerLayer(f *frame) {
	files := w.files()
	if w.viewer < 0 || w.viewer >= len(files) {
		if w.video.path != "" {
			w.video.stop()
		}
		w.viewer = -1
		return
	}
	c := f.c
	i := w.viewer
	path := files[i]
	video := i == len(w.f.Images) && w.f.Video != ""
	if !video && w.video.path != "" {
		w.video.stop()
	}
	open := true
	cw, ch := c.Size()
	picW, picH := min(cw-120, 760), min(ch-140, 475)
	white := ui.Hex("#ffffff")
	dlg := ui.DialogBase(c.Key("fb-viewer"), &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.78))
		panel.Background(ui.Transparent).AlignItems(ui.Center).Gap(10).Label(filepath.Base(path))
		panel.Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
				w.navButton(c, "fb-prev", -1, i > 0, len(files))
				if video {
					w.videoBox(f, path, picW, picH)
				} else {
					t := w.picture(path, fullPx)
					box := ui.Box(c.Key("fb-pic")).Size(picW, picH).AlignItems(ui.Center).Justify(ui.Center)
					box.Children(func() {
						switch {
						case t.bmp != nil:
							ui.Image(c, t.bmp).Size(picW, picH).Fit(ui.Contain).Radius(10)
						case t.loading:
							ui.Spinner(c).Size(20, 20).TextColor(white)
						default:
							ui.Text(c, filepath.Base(path)).TextColor(white)
						}
					})
				}
				w.navButton(c, "fb-next", 1, i < len(files)-1, len(files))
			})
			ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
				t := w.picture(path, thumbPx)
				name := filepath.Base(path)
				var caption string
				if video {
					caption = w.tx(w.texts.VideoOf, "name", name, "size", humanBytes(t.size))
				} else {
					caption = w.tx(w.texts.ImageOf, "n", strconv.Itoa(i+1), "count", strconv.Itoa(len(w.f.Images)), "name", name, "size", humanBytes(t.size))
				}
				ui.Text(c, caption).FontSize(12.5).TextColor(ui.Hex("#eeeeee"))
				if video && barButton(c, "fb-open-with", w.tx(w.texts.OpenWith)).Clicked() {
					w.openFile(path)
				}
				if barButton(c, "fb-viewer-remove", w.tx(w.texts.Remove)).Disabled(w.busy() || w.failed()).Clicked() {
					w.removeFile(i)
					if len(w.files()) == 0 {
						w.viewer = -1
					}
				}
				x := ui.ButtonBase(c.Key("fb-viewer-close")).Padding(5, 10).Radius(7).Background(ui.RGBA(255, 255, 255, 0.14)).Label(w.tx(w.texts.Close))
				x.Children(func() { cross(c, 12, 1.5, white) })
				if x.Clicked() {
					open = false
				}
			})
		})
	})
	// ← and → go through the attachments while the preview shows.
	left := dlg.Shortcut(0, ui.KeyLeft) || c.Shortcut(0, ui.KeyLeft)
	right := dlg.Shortcut(0, ui.KeyRight) || c.Shortcut(0, ui.KeyRight)
	if left && w.viewer > 0 {
		w.viewer--
	}
	if right && w.viewer >= 0 && w.viewer < len(w.files())-1 {
		w.viewer++
	}
	if !open {
		w.viewer = -1
		w.video.stop()
	}
}

func barButton(c *ui.Context, key, label string) ui.Element {
	b := ui.ButtonBase(c.Key(key)).Padding(5, 10).Radius(7).Background(ui.RGBA(255, 255, 255, 0.14)).Label(label)
	b.Children(func() { ui.Text(c, label).FontSize(12.5).TextColor(ui.Hex("#eeeeee")).NoWrap() })
	return b
}

func (w *Window) navButton(c *ui.Context, key string, dir int, enabled bool, n int) {
	label := w.tx(w.texts.Next)
	if dir < 0 {
		label = w.tx(w.texts.Previous)
	}
	b := ui.ButtonBase(c.Key(key)).Size(36, 36).Radius(18).Background(ui.RGBA(255, 255, 255, 0.16)).Label(label).Shrink(0).Disabled(!enabled)
	if !enabled {
		b.Opacity(0.3)
	}
	if n <= 1 {
		b.Invisible()
	}
	b.Children(func() { chevron(c, 18, 2, float32(dir), ui.Hex("#ffffff")) })
	if b.Clicked() {
		w.viewer += dir
	}
}

// videoBox plays the video in the window's web view; without one (a
// test, or WebKit refused) its first frame and Open in QuickTime Player.
func (w *Window) videoBox(f *frame, path string, bw, bh float32) {
	c := f.c
	win := w.NativeWindow()
	if win != nil {
		w.video.load(win, path)
	}
	if w.video.wv != nil && w.video.path == path {
		ui.WebView(c.Key("fb-video"), w.video.wv).Size(bw, bh).Radius(10)
		return
	}
	t := w.picture(path, fullPx)
	ui.Box(c.Key("fb-video")).Size(bw, bh).Radius(10).Background(ui.Hex("#000000")).Clip().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		if t.bmp != nil {
			ui.Image(c, t.bmp).Absolute().Left(0).Top(0).Size(bw, bh).Fit(ui.Contain)
		}
		playButton(c, 44)
	})
}
