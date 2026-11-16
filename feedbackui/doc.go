// Package feedbackui is the feedback window the ActNow apps share: one
// small window per app (Send Feedback — <App>), opened from the Help menu,
// Settings or a "Report this problem…" link beside an error, drawn with
// MyGo's native UI.
//
// The window holds a new feedback (its type, what happened, images and a
// video in a grid of thumbnails, an email to answer, the diagnostics and,
// opened from a link, the error report with its context), previews of the
// attachments (images enlarged, the video played in a web view), the
// progress of the sending, and this Mac's feedback with the replies. The
// work is actnowkit's feedback.Manager: the protocol, the outbox, the
// history; this package draws it.
//
// An app makes one Window with New and keeps it for its life:
//
//	fb := feedbackui.New(feedbackui.Options{
//		Manager: mgr, AppName: "Dicta", Site: "actnow.dev",
//		PrivacyURL: "https://actnow.dev/legal/privacy",
//		Lang: func() string { return i18n.Locale() },
//		Post: app.Post,
//	})
//	// Help → Send Feedback…
//	fb.Open(feedback.Prefill{})
//	// Beside an error the app shows:
//	rep, _ := mgr.Capture(feedback.ErrorContext{Title: "Transcription failed", Code: "E-ASR-0042", Err: err})
//	feedbackui.ReportLink(c, fb, rep)
//	// Settings → Feedback:
//	fb.HistoryPanel(c)
//
// The module depends on MyGo (each app replaces it with its vendored
// copy); it uses only the API every app's copy has (scripts/check-mygo.sh
// builds it against all of them). actnowkit's core packages do not depend
// on MyGo.
package feedbackui
