package feedbackui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/actnowkit/feedback"
)

// Metrics of the design, in DIPs.
const (
	padX     = 18
	tileSide = 92
	labelW   = 96
)

func (w *Window) frame(c *ui.Context) *frame {
	th := &w.opt.Theme
	return &frame{c: c, w: w, p: th.palette(c.Theme()), th: th, fs: th.fontSize(), r: th.radius()}
}

// View draws the window; the window's Content calls it (an app may show
// it elsewhere, e.g. in a test or a sheet of its own).
func (w *Window) View(c *ui.Context) {
	f := w.frame(c)
	c.SetTheme(f.th.mygoTheme(c.Theme(), f.p))
	ui.Column(c.Key("fb-root")).Fill().Background(f.p.Background).Children(func() {
		if bar := c.TitleBar(); bar.Height > 0 {
			ui.Box(c.Key("fb-titlebar")).Height(bar.Height).Shrink(0).DragWindow()
		}
		switch w.page {
		case pageDone:
			w.donePage(f)
		case pageHistory:
			w.historyPage(f)
		case pageThread:
			w.threadPage(f)
		default:
			w.formPage(f)
		}
	})
	w.viewerLayer(f)
	w.sheetLayer(f)
}

// header is the window's head: the app's icon, the title and to whom it
// goes, the close button.
func (w *Window) header(f *frame, title string) {
	c, p := f.c, f.p
	ui.Row(c.Key("fb-head")).AlignItems(ui.Center).Gap(10).Padding(16, padX, 6, padX).Shrink(0).DragWindow().Children(func() {
		w.icon(f)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(600).TextColor(p.Text).Role(ui.RoleHeading)
			ui.Text(c, w.tx(w.texts.WindowSub)).FontSize(12).TextColor(p.TextMuted)
		})
		x := ui.ButtonBase(c.Key("fb-close")).Size(24, 24).Radius(6).Label(w.tx(w.texts.Close)).Shrink(0)
		x.Children(func() { cross(c, 14, 1.5, p.TextMuted) })
		if x.Clicked() {
			w.Close()
		}
	})
}

func (w *Window) icon(f *frame) {
	c := f.c
	if w.opt.Icon != nil {
		ui.Image(c, w.opt.Icon).Size(28, 28).Radius(7).Fit(ui.Cover).Shrink(0)
		return
	}
	initial := "?"
	for _, r := range w.opt.AppName {
		initial = string(unicode.ToUpper(r))
		break
	}
	ui.Box(c).Size(28, 28).Radius(7).Shrink(0).Gradient(ui.Hex("#0d9384"), ui.Hex("#2bb3a3"), 135).
		AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Text(c, initial).FontSize(14).FontWeight(700).TextColor(ui.Hex("#ffffff"))
	})
}

// ---- the form ----

func (w *Window) formPage(f *frame) {
	c, p := f.c, f.p
	title := w.tx(w.texts.WindowTitle)
	if w.f.Report != nil {
		title = w.tx(w.texts.ReportTitle)
	}
	w.header(f, title)
	busy := w.busy() || w.failed()
	ui.Scroll(c.Key("fb-body")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(8, padX, 12, padX).Children(func() {
			if w.f.Report != nil {
				w.contextCard(f)
			}
			ui.Row(c.Key("fb-kind")).AlignItems(ui.Center).Gap(12).Margin(10, 0).Children(func() {
				f.label(w.tx(w.texts.Kind)).Width(labelW).Shrink(0)
				labels := make([]string, len(feedback.Kinds))
				for i, k := range feedback.Kinds {
					labels[i] = w.tx(w.kindText(k))
				}
				f.segmented("fb-kind-seg", &w.f.Kind, labels...).Label(w.tx(w.texts.Kind)).Disabled(busy)
			})
			f.label(w.tx(w.texts.Body)).Margin(10, 0, 4, 0)
			h := float32(112)
			if w.f.Report != nil {
				h = 76
			}
			ui.TextArea(c.Key("fb-body-in"), &w.f.Body).Height(h).Placeholder(w.tx(w.texts.BodyPlaceholder)).
				Label(w.tx(w.texts.Body)).Background(p.Raised).Border(1, p.Border).Radius(f.r+1).Padding(8, 10).
				FontSize(f.fs).TextColor(p.Text).Disabled(busy)
			w.problemLine(f, "fb-body-problem", feedback.ProblemBodyShort, feedback.ProblemBodyLong, feedback.ProblemKind)
			w.attachments(f, busy)
			w.options(f, busy)
		})
	})
	w.footer(f)
}

// problemLine shows the form's problem among ps, once Send was pressed.
func (w *Window) problemLine(f *frame, key string, ps ...feedback.Problem) {
	if !w.tried {
		return
	}
	for _, x := range w.problems {
		if slices.Contains(ps, x) {
			ui.Text(f.c.Key(key), w.problemText(x)).FontSize(f.fs-1.5).TextColor(f.p.Danger).Margin(4, 0, 0, 0)
			return
		}
	}
}

// contextCard is the error report the window was opened with.
func (w *Window) contextCard(f *frame) {
	c, p := f.c, f.p
	r := w.f.Report
	bg, line := p.DangerBG, ui.RGBA(p.Danger.R, p.Danger.G, p.Danger.B, 0.35)
	ui.Column(c.Key("fb-context")).Background(bg).Border(1, line).Radius(f.r+3).Padding(10, 12).Margin(6, 0, 10, 0).Gap(8).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Box(c).Size(16, 16).Radius(8).Background(p.Danger).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Children(func() {
				ui.Text(c, "!").FontSize(11).FontWeight(700).TextColor(ui.Hex("#ffffff"))
			})
			title := r.Title
			if title == "" {
				title = w.tx(w.texts.ReportTitle)
			}
			ui.Text(c, title).FontSize(f.fs).FontWeight(600).TextColor(p.Danger).Grow(1).MinWidth(0)
		})
		ui.Column(c).Gap(3).Children(func() {
			for _, kv := range w.contextRows(r) {
				ui.Row(c.Key("ctx-" + kv[0])).Gap(10).Children(func() {
					ui.Text(c, kv[0]).FontSize(12).TextColor(p.TextMuted).Width(88).Shrink(0)
					ui.Text(c, kv[1]).FontSize(12).TextColor(p.Text).Grow(1).MinWidth(0).Selectable()
				})
			}
		})
		ui.Row(c.Key("fb-include")).AlignItems(ui.Center).Gap(4).Wrap().Children(func() {
			ui.Checkbox(c.Key("fb-include-box"), &w.f.IncludeReport, w.tx(w.texts.IncludeReport, "count", strconv.Itoa(len(r.Logs)))).
				FontSize(12).TextColor(p.TextSecondary).Disabled(w.busy() || w.failed())
			ui.Text(c, "·").FontSize(12).TextColor(p.TextMuted)
			if f.link("fb-report-preview", w.tx(w.texts.Preview), 12).Clicked() {
				w.openReportSheet()
			}
		})
	})
}

// contextRows are the report's lines: Error, Where, When, Steps, Report.
func (w *Window) contextRows(r *feedback.Report) [][2]string {
	var rows [][2]string
	add := func(k, v string) {
		if v != "" {
			rows = append(rows, [2]string{k, v})
		}
	}
	errLine := r.Kind
	switch {
	case r.Kind != "" && r.Code != "":
		if w.zh() {
			errLine = r.Kind + "（" + r.Code + "）"
		} else {
			errLine = r.Kind + " (" + r.Code + ")"
		}
	case r.Code != "":
		errLine = r.Code
	case errLine == "":
		errLine = r.Error
	}
	add(w.tx(w.texts.ContextError), errLine)
	add(w.tx(w.texts.ContextWhere), r.Where)
	if !r.At.IsZero() {
		at := r.At.Local()
		when := at.Format("2006-01-02 15:04:05")
		if y, m, d := time.Now().Date(); at.Year() == y && at.Month() == m && at.Day() == d {
			when = w.tx(w.texts.Today) + " " + at.Format("15:04:05")
		}
		if r.Version != "" {
			when += " · " + w.opt.AppName + " " + r.Version
		}
		add(w.tx(w.texts.ContextWhen), when)
	}
	if len(r.Steps) > 0 {
		steps := r.Steps
		if len(steps) > 5 {
			steps = steps[len(steps)-5:]
		}
		names := make([]string, len(steps))
		for i, s := range steps {
			names[i] = w.stepName(s.Name)
		}
		add(w.tx(w.texts.ContextSteps), strings.Join(names, " → "))
	}
	add(w.tx(w.texts.ContextReport), w.tx(w.texts.ReportLines, "id", r.ID, "count", strconv.Itoa(len(r.Logs))))
	return rows
}

func (w *Window) stepName(s string) string {
	if w.opt.StepName != nil {
		if n := w.opt.StepName(s); n != "" {
			return n
		}
	}
	s = strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(s)
	rs := []rune(s)
	if len(rs) > 0 {
		rs[0] = unicode.ToUpper(rs[0])
	}
	return string(rs)
}

// attachments is the grid of the images and the video.
func (w *Window) attachments(f *frame, busy bool) {
	c, p := f.c, f.p
	nImg, nVid := len(w.f.Images), 0
	if w.f.Video != "" {
		nVid = 1
	}
	ui.Row(c.Key("fb-att-head")).AlignItems(ui.End).Margin(12, 0, 0, 0).Children(func() {
		f.label(w.tx(w.texts.Attachments)).Grow(1)
		if nImg+nVid > 0 {
			f.muted(w.tx(w.texts.AttachCount, "images", strconv.Itoa(nImg), "videos", strconv.Itoa(nVid)))
		}
	})
	grid := ui.Row(c.Key("fb-grid")).Gap(8).Margin(8, 0, 4, 0)
	grid.Children(func() {
		for i, path := range w.files() {
			w.tile(f, i, path, busy)
		}
		if (nImg < feedback.MaxImages || nVid == 0) && !busy {
			add := ui.ButtonBase(c.Key("fb-add")).Size(tileSide, tileSide).Radius(f.r+3).Border(1.5, p.BorderStrong).BorderStyle(ui.BorderDashed).
				Column().AlignItems(ui.Center).Justify(ui.Center).Gap(4).Padding(0, 6).Shrink(0).Label(w.tx(w.texts.AddFiles))
			add.Children(func() {
				plus(c, 18, 1.2, p.TextMuted).Margin(0, 0, 2, 0)
				ui.Text(c, w.tx(w.texts.Add)).FontSize(11.5).TextColor(p.TextMuted).TextAlign(ui.Center)
				ui.Text(c, w.tx(w.texts.AddSub)).FontSize(11.5).TextColor(p.TextMuted).TextAlign(ui.Center)
			})
			if add.Clicked() {
				w.pickFiles()
			}
		}
	})
	if fs := grid.DroppedFiles(); len(fs) > 0 && !busy {
		w.addFiles(fs)
	}
	f.muted(w.tx(w.texts.AttachHint))
	if w.fileErr != "" {
		ui.Text(c.Key("fb-file-err"), w.fileErr).FontSize(f.fs-1.5).TextColor(p.Danger).Margin(4, 0, 0, 0)
	}
	w.problemLine(f, "fb-files-problem", feedback.ProblemTooManyImages, feedback.ProblemImageMissing, feedback.ProblemImageTooLarge,
		feedback.ProblemImageType, feedback.ProblemVideoMissing, feedback.ProblemVideoTooLarge, feedback.ProblemVideoType)
}

// tile is an attachment's thumbnail: a click previews it, ✕ removes it.
func (w *Window) tile(f *frame, i int, path string, busy bool) {
	c, p := f.c, f.p
	t := w.picture(path, thumbPx)
	video := w.f.Video == path && i == len(w.f.Images)
	name := filepath.Base(path)
	b := ui.ButtonBase(c.Key("tile:"+path)).Size(tileSide, tileSide).Radius(f.r+3).Border(1, p.Border).Background(p.Track).Clip().Shrink(0).Label(name)
	b.Children(func() {
		switch {
		case t.bmp != nil:
			ui.Image(c, t.bmp).Absolute().Left(0).Top(0).Size(tileSide, tileSide).Fit(ui.Cover)
		case t.loading:
			ui.Spinner(c).Size(14, 14)
		case video:
			ui.Box(c).Absolute().Left(0).Top(0).Size(tileSide, tileSide).Background(ui.Hex("#1a1a1a"))
		default:
			ui.Text(c, strings.ToUpper(strings.TrimPrefix(filepath.Ext(name), "."))).FontSize(11).TextColor(p.TextMuted)
		}
		if video {
			playButton(c, 28).Absolute().Left((tileSide - 28) / 2).Top((tileSide - 28) / 2)
			if t.dur > 0 {
				ui.Box(c).Absolute().Left(6).Bottom(6).Background(ui.RGBA(0, 0, 0, 0.62)).Radius(5).Padding(1, 5).Children(func() {
					ui.Text(c, clock(t.dur)).FontSize(10.5).TextColor(ui.Hex("#ffffff"))
				})
			}
		}
		if !busy {
			rm := ui.ButtonBase(c.Key("rm")).Absolute().Top(5).Right(5).Size(18, 18).Radius(9).Background(ui.RGBA(0, 0, 0, 0.6)).
				Label(w.tx(w.texts.Remove) + " " + name)
			rm.Children(func() { cross(c, 10, 1.4, ui.Hex("#ffffff")) })
			if rm.Clicked() {
				w.removeFile(i)
			}
		}
	})
	if b.Clicked() {
		w.viewer = i
	}
}

// options are the email and the diagnostics.
func (w *Window) options(f *frame, busy bool) {
	c, p := f.c, f.p
	ui.Box(c).Height(1).Background(p.Separator).Margin(12, 0, 4, 0).Shrink(0)
	ui.Row(c.Key("fb-email")).AlignItems(ui.Center).Gap(12).Padding(8, 0).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			f.text(w.tx(w.texts.Email))
			desc := w.tx(w.texts.EmailNeeded)
			if w.who.Linked && w.who.EmailHint != "" {
				desc = w.tx(w.texts.EmailLinked, "email", w.who.EmailHint)
			}
			f.muted(desc)
		})
		ph := w.tx(w.texts.EmailPlaceholder)
		if w.who.Linked {
			ph = w.tx(w.texts.Optional)
		}
		ui.TextInput(c.Key("fb-email-in"), &w.f.Email).Placeholder(ph).Label(w.tx(w.texts.Email)).Width(230).Shrink(1).MinWidth(140).
			Background(p.Raised).Border(1, p.Border).Radius(f.r+1).Padding(8, 10).FontSize(f.fs).Disabled(busy)
	})
	w.problemLine(f, "fb-email-problem", feedback.ProblemEmailRequired, feedback.ProblemEmailInvalid)
	ui.Box(c).Height(1).Background(p.Separator).Shrink(0)
	ui.Row(c.Key("fb-diag")).AlignItems(ui.Center).Gap(12).Padding(8, 0).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			f.text(w.tx(w.texts.Diagnostics))
			f.muted(w.tx(w.texts.DiagnosticsHint))
			if f.link("fb-diag-preview", w.tx(w.texts.Preview), f.fs-1).Clicked() {
				w.openDiagnosticsSheet()
			}
		})
		f.toggle("fb-diag-switch", &w.f.Diagnostics, w.tx(w.texts.Diagnostics)).Disabled(busy)
	})
}

// footer: the privacy line and Your feedback, or the progress, or the
// failure; Cancel and Send.
func (w *Window) footer(f *frame) {
	c, p := f.c, f.p
	ui.Box(c).Height(1).Background(p.Separator).Shrink(0)
	ui.Row(c.Key("fb-foot")).AlignItems(ui.Center).Gap(8).Padding(12, padX, 16, padX).Background(p.Raised).Shrink(0).Children(func() {
		left := ui.Column(c).Grow(1).MinWidth(0).Gap(3)
		switch {
		case w.busy():
			left.Children(func() { w.progress(f) })
			if f.button("fb-cancel", w.tx(w.texts.Cancel)).Clicked() {
				w.cancelSend()
			}
			f.primary("fb-send", w.tx(w.texts.Send), true)
		case w.failed():
			left.Children(func() {
				ui.Text(c.Key("fb-failed"), w.failText()).FontSize(f.fs - 1.5).TextColor(p.Danger).Selectable()
			})
			if f.button("fb-discard", w.tx(w.texts.Discard)).Clicked() {
				w.discard()
			}
			if f.primary("fb-retry", w.tx(w.texts.Retry), false).Clicked() {
				w.retry()
			}
		default:
			left.Children(func() {
				if w.prepErr != "" {
					ui.Text(c.Key("fb-prep-err"), w.tx(w.texts.SendFailed, "error", w.prepErr)).FontSize(f.fs - 1.5).TextColor(p.Danger).Selectable()
				}
				ui.Text(c.Key("fb-privacy-line"), w.tx(w.texts.Privacy)).FontSize(f.fs - 1.5).TextColor(p.TextMuted)
				ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
					// A link people can tab to (a focusable button), not a span.
					if w.opt.PrivacyURL != "" && f.link("fb-privacy", w.tx(w.texts.PrivacyLink), f.fs-1.5).Clicked() {
						w.openURL(w.opt.PrivacyURL)
					}
					if w.m == nil {
						return
					}
					if w.opt.PrivacyURL != "" {
						ui.Text(c, "·").FontSize(f.fs - 1.5).TextColor(p.TextMuted)
					}
					n := len(w.history)
					if f.link("fb-history", w.tx(w.texts.HistoryCount, "count", strconv.Itoa(n)), f.fs-1).Clicked() {
						w.OpenHistory()
					}
					if w.Unread() > 0 {
						dot(c, 7, p.Accent).Tooltip(w.tx(w.texts.NewReply))
					}
				})
			})
			if f.button("fb-cancel", w.tx(w.texts.Cancel)).Clicked() {
				w.f, w.tried, w.problems, w.fileErr, w.prepErr = form{Diagnostics: true, Email: w.f.Email}, false, nil, "", ""
				w.dropMedia()
				w.Close()
			}
			if f.primary("fb-send", w.tx(w.texts.Send), strings.TrimSpace(w.f.Body) == "" || w.m == nil).Clicked() {
				w.send()
			}
		}
	})
}

// progress is the sending's line and bar.
func (w *Window) progress(f *frame) {
	c, p := f.c, f.p
	st := w.st
	if st.Phase == feedback.Sending && st.TotalBytes > 0 {
		f.muted(w.tx(w.texts.Sending, "size", mb(st.SentBytes)+" / "+mb(st.TotalBytes)+" MB"))
	} else if st.Phase == feedback.Sending {
		f.muted(strings.TrimSpace(w.tx(w.texts.Sending, "size", "")))
	} else {
		f.muted(w.tx(w.texts.Preparing))
	}
	v := -1.0
	if st.Phase == feedback.Sending && st.TotalBytes > 0 {
		v = st.Fraction()
	}
	bar := ui.Box(c.Key("fb-progress")).Height(4).Radius(2).Background(p.Separator).Margin(3, 0, 0, 0).Clip().Role(ui.RoleProgress)
	bar.Children(func() {
		if v >= 0 {
			ui.Box(c).Height(4).WidthPercent(float32(v * 100)).Background(p.Accent).Radius(2)
		} else {
			ui.Progress(c, -1).Height(4)
		}
	})
}

func mb(n int64) string { return fmt.Sprintf("%.1f", float64(n)/(1<<20)) }

// failText says why this window's feedback was not sent.
func (w *Window) failText() string {
	st := w.st
	if st.Current != nil && st.Current.ErrorCode == "rate_limited" {
		return w.tx(w.texts.RateLimited)
	}
	if st.Current != nil && !st.Current.Rejected {
		if st.Err != nil && strings.Contains(st.Err.Error(), feedback.ErrNetwork.Error()) {
			return w.tx(w.texts.Failed)
		}
	}
	return w.tx(w.texts.SendFailed, "error", errText(st.Err))
}

// ---- sent ----

func (w *Window) donePage(f *frame) {
	c, p := f.c, f.p
	w.header(f, w.tx(w.texts.WindowTitle))
	ui.Column(c.Key("fb-done")).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Padding(34, 28, 26, 28).Gap(6).Children(func() {
		ui.Box(c).Size(44, 44).Radius(22).Background(ui.RGBA(p.Accent.R, p.Accent.G, p.Accent.B, 0.15)).AlignItems(ui.Center).Justify(ui.Center).
			Margin(0, 0, 6, 0).Children(func() { check(c, 22, 2.2, p.Accent) })
		ui.Text(c, w.tx(w.texts.SentTitle)).FontSize(16).FontWeight(600).TextColor(p.Text).Role(ui.RoleHeading)
		body := w.tx(w.texts.SentBody)
		before, after, _ := strings.Cut(body, "{number}")
		ui.RichText(c.Key("fb-done-body"),
			ui.Span{Text: before},
			ui.Span{Text: "\u00a0" + w.doneNum + "\u00a0", Font: f.th.mono(), Color: p.Text, Background: p.Track, Radius: 5},
			ui.Span{Text: after},
		).FontSize(f.fs).TextColor(p.TextMuted).TextAlign(ui.Center).MaxWidth(460).Selectable()
	})
	ui.Box(c).Height(1).Background(p.Separator).Shrink(0)
	ui.Row(c.Key("fb-foot")).AlignItems(ui.Center).Gap(8).Padding(12, padX, 16, padX).Background(p.Raised).Shrink(0).Children(func() {
		ui.Box(c).Grow(1)
		if w.doneID != "" && f.button("fb-view", w.tx(w.texts.ViewIt)).Clicked() {
			w.openThread(w.doneID)
		}
		if f.primary("fb-done-close", w.tx(w.texts.Close), false).Clicked() {
			w.m.Reset()
			w.page = pageForm
			w.Close()
		}
	})
}
