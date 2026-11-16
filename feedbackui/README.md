# feedbackui

The feedback window the ActNow apps share (`Send Feedback — <App>`): a
module of its own (`github.com/hehehai/actnowkit/feedbackui`) because it
draws with MyGo, which actnowkit's core packages do not depend on.

- One window per app, opened again it comes to the front (`Open`); menu-bar
  apps open it without a main window.
- The form: type, description, images and a video in a grid of thumbnails
  (92 DIPs; a video's first frame, its length and a play mark; ✕ removes,
  `+ Add` picks, files dropped on the grid are added), the email (optional
  when the licence's owner is known), the diagnostics and what they hold;
  opened from "Report this problem…" the error report's card (error, where,
  when, the last steps, the report's id) and whether it goes with it.
- Previews in the window: images fitted, ‹ › and the arrow keys; the video
  in a web view (`<video controls>`, served through an `actnowfb://` scheme
  with ranges), and "Open in QuickTime Player" (the file's default app).
- Sending: progress in the footer (`Sending… 12.4 / 20.1 MB`), Cancel
  stops it and gives the form back; a failure says why with Try again and
  Discard; sent, the feedback's number with View it and Close.
- Your feedback: the window's second page (the history, unread dots, the
  submissions waiting, a feedback's messages and the reply), and
  `HistoryPanel` for the app's Settings.

## An app

```go
// internal/app/feedback.go (the Manager as for v0.21's Settings section)
a.feedbackWin = feedbackui.New(feedbackui.Options{
	Manager:    a.feedbackMgr,
	AppName:    "Dicta",
	Site:       "actnow.dev",
	PrivacyURL: "https://actnow.dev/legal/privacy",
	Icon:       appIcon28, // optional *ui.Bitmap
	Lang:       func() string { return i18n.Locale() },
	T:          func(t family.Text) string { return i18n.T(t.EN) }, // optional: the catalogue
	Post:       a.Post,  // runs on the UI thread and redraws the app's windows
	OpenURL:    a.OpenWeb,
	BeforeCollect: a.feedbackUI.TakeSnapshot, // optional
	// Menu-bar apps (Pik, HereNow, Refto): show in the Dock while it is open.
	WindowOpened: func(*mygo.Window) { mygo.App.SetActivationPolicy(mygo.ActivationPolicyRegular) },
	WindowClosed: func() { if !a.mainWindowOpen() { mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory) } },
})

// Help → Send Feedback…
menu.Item(i18n.T("Send Feedback…"), func() { a.feedbackWin.Open(feedback.Prefill{}) })

// Where the app shows an error the user can report:
rep, _ := a.feedbackMgr.Capture(feedback.ErrorContext{
	Title: i18n.T("Transcription failed"), Code: "E-ASR-0042", Kind: "engine_out_of_memory",
	Where: "Meetings › Transcribe", Err: err,
})
// … in the error's view:
feedbackui.ReportLink(c, a.feedbackWin, rep)

// Recent actions, for the reports:
a.feedbackMgr.Step("transcribe")

// Settings → Feedback:
a.feedbackWin.HistoryPanel(c)

// The licence changed:
a.feedbackWin.LicenceChanged()
// Quitting:
a.feedbackWin.Stop()
```

Its look: `Options.Theme` (palettes for light and dark, or `FromMyGo` to
take the app's MyGo theme, radius, fonts, and callbacks drawing the primary
and secondary buttons, the segmented control and the switch). Its words:
`Options.Texts` (a copy of `DefaultTexts()` with some changed) and
`Options.T`.

## Building

The module requires the actnowkit next to it (`v0.22.0`) and MyGo
`v0.5.5`, which every app replaces with its `third_party/mygo`. Here:

```sh
scripts/work.sh ../../Dicta/third_party/mygo   # writes go.work (not committed)
go test ./...
FBUI_SHOTS=/tmp/shots FBUI_ART=<design>/src go test -run TestShots   # the snapshots
scripts/check-mygo.sh --test ~/helper          # every app's MyGo, macOS and the host
```

The app's go.mod: `require github.com/hehehai/actnowkit/feedbackui v0.22.0`
(tag `feedbackui/v0.22.0`) beside `github.com/hehehai/actnowkit v0.22.0`.
