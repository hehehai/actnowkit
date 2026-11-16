package feedbackui

// Your feedback: this Mac's feedback (the window's second page, and the
// panel an app puts in its Settings), a feedback with its replies, the
// submissions waiting to be sent; and the "Report this problem…" link.

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/actnowkit/feedback"
)

// pageHead is the head of the second pages: Back, the title, an action.
func (w *Window) pageHead(f *frame, title, sub string, back func(), action func()) {
	c, p := f.c, f.p
	ui.Row(c.Key("fb-page-head")).AlignItems(ui.Center).Gap(8).Padding(14, padX, 8, padX).Shrink(0).DragWindow().Children(func() {
		b := ui.ButtonBase(c.Key("fb-back")).Size(26, 26).Radius(6).Label(w.tx(w.texts.Back)).Shrink(0)
		b.Children(func() { chevron(c, 16, 1.8, -1, p.TextSecondary) })
		if b.Clicked() {
			back()
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(600).TextColor(p.Text).MaxLines(2).Role(ui.RoleHeading)
			if sub != "" {
				ui.Text(c, sub).FontSize(12).TextColor(p.TextMuted)
			}
		})
		if action != nil {
			action()
		}
		x := ui.ButtonBase(c.Key("fb-close")).Size(24, 24).Radius(6).Label(w.tx(w.texts.Close)).Shrink(0)
		x.Children(func() { cross(c, 14, 1.5, p.TextMuted) })
		if x.Clicked() {
			w.Close()
		}
	})
	ui.Box(c).Height(1).Background(p.Separator).Shrink(0)
}

func (w *Window) historyPage(f *frame) {
	c, p := f.c, f.p
	if !w.refreshed {
		w.Refresh()
	}
	w.pageHead(f, w.tx(w.texts.History), w.tx(w.texts.WindowSub), func() { w.page = pageForm }, func() {
		if w.refreshing {
			ui.Spinner(c).Size(14, 14)
		} else if f.link("fb-refresh", w.tx(w.texts.Refresh), f.fs-1).Clicked() {
			w.Refresh()
		}
	})
	ui.Scroll(c.Key("fb-hist")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(8, padX, 16, padX).Gap(0).Children(func() {
			if w.histErr != "" {
				ui.Text(c.Key("fb-hist-err"), w.tx(w.texts.Unreachable, "error", w.histErr)).FontSize(f.fs-1.5).TextColor(p.Danger).
					Margin(4, 0, 8, 0).Selectable()
			}
			w.pendingList(f)
			if len(w.history) == 0 {
				f.muted(w.tx(w.texts.NoHistory)).Margin(12, 0)
				return
			}
			for i, e := range w.history {
				if i > 0 {
					ui.Box(c).Height(1).Background(p.Separator).Shrink(0)
				}
				w.historyRow(f, e)
			}
		})
	})
}

// pendingList is the outbox: the submissions not sent.
func (w *Window) pendingList(f *frame) {
	c, p := f.c, f.p
	var list []feedback.Submission
	for _, s := range w.st.Pending {
		if w.st.Current != nil && s.LocalID == w.st.Current.LocalID && (w.st.Phase == feedback.Preparing || w.st.Phase == feedback.Sending) {
			continue
		}
		list = append(list, s)
	}
	if len(list) == 0 {
		return
	}
	f.label(w.tx(w.texts.Pending)).FontWeight(600).Margin(6, 0, 4, 0)
	for _, s := range list {
		id := s.LocalID
		ui.Row(c.Key("pending:"+id)).AlignItems(ui.Center).Gap(8).Padding(8, 0).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, w.tx(w.kindText(s.Kind))+": "+feedback.Summary(s.Body, 60)).FontSize(f.fs).TextColor(p.Text).MaxLines(1)
				note := s.CreatedAt.Local().Format("2006-01-02 15:04")
				if s.LastError != "" {
					note += " · " + s.LastError
				}
				f.muted(note).MaxLines(2)
			})
			if f.button("retry", w.tx(w.texts.Retry)).Disabled(w.busy()).Clicked() {
				w.retryPending(id)
			}
			if f.button("discard", w.tx(w.texts.Discard)).Disabled(w.busy()).Clicked() {
				w.discardPending(id)
			}
		})
	}
	ui.Box(c).Height(1).Background(p.Separator).Margin(4, 0, 8, 0).Shrink(0)
}

// historyRow is one feedback: a dot when a reply is not read.
func (w *Window) historyRow(f *frame, e feedback.HistoryEntry) {
	c, p := f.c, f.p
	b := ui.ButtonBase(c.Key("hist:"+e.ID)).Justify(ui.Start).AlignItems(ui.Center).Gap(10).Padding(10, 0).Label(e.Number + " " + e.Summary)
	b.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if e.Unread() {
					dot(c, 7, p.Accent).Tooltip(w.tx(w.texts.NewReply))
				}
				ui.Text(c, e.Summary).FontSize(f.fs).TextColor(p.Text).MaxLines(1).Grow(1).MinWidth(0)
			})
			parts := []string{e.Number, w.tx(w.statusText(e.Status)), w.tx(w.kindText(e.Kind)), e.CreatedAt.Local().Format("2006-01-02")}
			if e.Unread() {
				parts = append([]string{w.tx(w.texts.NewReply)}, parts...)
			}
			f.muted(strings.Join(nonEmpty(parts...), " · "))
		})
		chevron(c, 14, 1.5, 1, p.TextMuted)
	})
	if b.Clicked() {
		w.openThread(e.ID)
	}
}

func nonEmpty(xs ...string) []string {
	var out []string
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

// threadPage is a feedback with its messages and the reply field.
func (w *Window) threadPage(f *frame) {
	c, p := f.c, f.p
	title, sub, web := w.tx(w.texts.History), "", ""
	for _, e := range w.history {
		if e.ID == w.threadID {
			title, sub, web = e.Summary, strings.Join(nonEmpty(e.Number, w.tx(w.statusText(e.Status))), " · "), e.WebURL
		}
	}
	if w.thread != nil && w.thread.Item.WebURL != "" {
		web = w.thread.Item.WebURL
	}
	w.pageHead(f, title, sub, func() { w.page = pageHistory; w.threadID = "" }, nil)
	ui.Scroll(c.Key("fb-thread")).Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).Padding(12, padX, 12, padX).Gap(10).Children(func() {
			switch {
			case w.loading:
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Size(14, 14)
					f.muted(w.tx(w.texts.Loading))
				})
			case w.thread != nil:
				th := w.thread
				w.message(f, "msg-body", w.tx(w.texts.You), th.Item.CreatedAt, th.Body, false)
				for _, m := range th.Messages {
					who := w.tx(w.texts.You)
					if m.Author == "support" {
						who = w.tx(w.texts.Support)
					}
					w.message(f, "msg-"+m.ID, who, m.CreatedAt, m.Body, m.Author == "support")
				}
				if len(th.Attachments) > 0 {
					names := make([]string, len(th.Attachments))
					for i, a := range th.Attachments {
						names[i] = a.Name
					}
					f.muted(w.tx(w.texts.Attachments) + ": " + strings.Join(names, ", "))
				}
			}
			if w.threadErr != "" {
				ui.Text(c.Key("fb-thread-err"), w.tx(w.texts.Unreachable, "error", w.threadErr)).FontSize(f.fs - 1.5).TextColor(p.Danger).Selectable()
			}
		})
	})
	ui.Box(c).Height(1).Background(p.Separator).Shrink(0)
	ui.Column(c.Key("fb-reply")).Padding(12, padX, 16, padX).Gap(8).Background(p.Raised).Shrink(0).Children(func() {
		if w.thread != nil {
			ui.TextArea(c.Key("fb-reply-in"), &w.replyText).Height(64).Placeholder(w.tx(w.texts.YourReply)).Label(w.tx(w.texts.YourReply)).
				Background(p.Background).Border(1, p.Border).Radius(f.r+1).Padding(8, 10).FontSize(f.fs).Disabled(w.replying)
		}
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			if web != "" && f.link("fb-thread-web", w.tx(w.texts.ViewOnWeb), f.fs-1).Clicked() {
				w.openURL(web)
			}
			ui.Box(c).Grow(1)
			if w.thread != nil && f.primary("fb-reply-send", w.tx(w.texts.Reply), w.replying || strings.TrimSpace(w.replyText) == "").Clicked() {
				w.sendReply()
			}
		})
	})
}

func (w *Window) message(f *frame, key, who string, at time.Time, body string, support bool) {
	c, p := f.c, f.p
	bg := p.Raised
	if support {
		bg = ui.RGBA(p.Accent.R, p.Accent.G, p.Accent.B, 0.1)
	}
	ui.Column(c.Key(key)).Gap(4).Padding(10, 12).Radius(f.r+3).Background(bg).Border(1, p.Separator).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, who).FontWeight(600).FontSize(f.fs - 1).TextColor(p.Text)
			f.muted(at.Local().Format("2006-01-02 15:04"))
		})
		ui.Text(c, body).FontSize(f.fs).TextColor(p.Text).Selectable()
	})
}

// HistoryPanel is the part of an app's Settings → Feedback: the newest of
// this Mac's feedback (a dot on those with a reply not read; a click
// opens it in the window), Your feedback (n), and the button opening the
// window. It draws in the app's window, in its colors (Theme).
func (w *Window) HistoryPanel(c *ui.Context) ui.Element {
	f := w.frame(c)
	p := f.p
	if !w.refreshed && w.m != nil {
		w.Refresh()
	}
	col := ui.Column(c.Key("fb-panel")).Gap(0)
	col.Children(func() {
		for i, e := range w.history {
			if i >= 5 {
				break
			}
			if i > 0 {
				ui.Box(c).Height(1).Background(p.Separator).Shrink(0)
			}
			e := e
			b := ui.ButtonBase(c.Key("panel:"+e.ID)).Justify(ui.Start).AlignItems(ui.Center).Gap(8).Padding(8, 0).Label(e.Number + " " + e.Summary)
			b.Children(func() {
				if e.Unread() {
					dot(c, 7, p.Accent).Tooltip(w.tx(w.texts.NewReply))
				}
				ui.Text(c, e.Summary).FontSize(f.fs).TextColor(p.Text).MaxLines(1).Grow(1).MinWidth(0)
				f.muted(strings.Join(nonEmpty(e.Number, w.tx(w.statusText(e.Status))), " · ")).Shrink(0)
				chevron(c, 12, 1.4, 1, p.TextMuted)
			})
			if b.Clicked() {
				w.OpenThread(e.ID)
			}
		}
		if len(w.history) == 0 {
			f.muted(w.tx(w.texts.NoHistory)).Margin(4, 0)
		}
		ui.Row(c).AlignItems(ui.Center).Gap(10).Margin(10, 0, 0, 0).Children(func() {
			if f.button("fb-panel-open", w.tx(w.texts.OpenWindow)).Clicked() {
				w.Open(feedback.Prefill{})
			}
			if len(w.history) > 0 && f.link("fb-panel-history", w.tx(w.texts.HistoryCount, "count", strconv.Itoa(len(w.history))), f.fs-1).Clicked() {
				w.OpenHistory()
			}
		})
	})
	return col
}

// ReportLink is the "Report this problem…" link an app puts beside an
// error it shows: a click opens the window with the report (a problem,
// its context card, the report included). rep is from Manager.Capture;
// nil opens the window as Send Feedback does.
func ReportLink(c *ui.Context, w *Window, rep *feedback.Report) ui.Element {
	f := w.frame(c)
	label := w.tx(w.texts.ReportLink)
	b := ui.ButtonBase(c).Padding(0).Role(ui.RoleLink).Label(label).Shrink(0)
	b.Children(func() {
		ui.Text(c, label).FontSize(f.fs - 1).TextColor(f.p.Accent).NoWrap()
	})
	if b.Clicked() {
		w.Open(feedback.Prefill{Kind: feedback.KindBug, Report: rep})
	}
	return b
}
