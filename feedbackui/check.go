package feedbackui

// What an app's scripted check does in place of the user (no input
// events: the checks run on a shared Mac over ssh), and what it reads
// back. UI thread only, as the rest.

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/hehehai/actnowkit/feedback"
)

// Snapshot is what the window shows, for a scripted check.
type Snapshot struct {
	Open    bool
	Page    string // "form", "done", "history", "thread"
	Kind    feedback.Kind
	Body    string
	Email   string
	Images  []string
	Video   string
	Report  string // the report's ID ("" none)
	Include bool   // the report goes with it
	Tried   bool
	// Problems shown (after Send), the file's problem, the preparation's.
	Problems []feedback.Problem
	FileErr  string
	PrepErr  string
	Busy     bool
	Failed   bool
	Phase    feedback.Phase
	Sent     string // the number of the feedback just sent
	Viewer   int    // the attachment previewed (-1 none)
	Sheet    string // "What is sent" open: its text
	History  int
	Unread   int
	Thread   string // the thread open
	Messages int
}

// Snapshot of the window now.
func (w *Window) Snapshot() Snapshot {
	pages := map[page]string{pageForm: "form", pageDone: "done", pageHistory: "history", pageThread: "thread"}
	s := Snapshot{Open: w.IsOpen(), Page: pages[w.page], Kind: w.f.draft().Kind, Body: w.f.Body, Email: w.f.Email,
		Images: append([]string(nil), w.f.Images...), Video: w.f.Video, Include: w.f.Report != nil && w.f.IncludeReport,
		Tried: w.tried, Problems: append([]feedback.Problem(nil), w.problems...), FileErr: w.fileErr, PrepErr: w.prepErr,
		Busy: w.busy(), Failed: w.failed(), Phase: w.st.Phase, Sent: w.doneNum, Viewer: w.viewer,
		History: len(w.history), Unread: w.Unread(), Thread: w.threadID}
	if w.f.Report != nil {
		s.Report = w.f.Report.ID
	}
	if w.sheet.open {
		s.Sheet = w.sheet.text
	}
	if w.thread != nil {
		s.Messages = len(w.thread.Messages)
	}
	return s
}

// SetForm types into the form: the kind, the description, the email
// ("" leaves them), whether the diagnostics and the report go.
func (w *Window) SetForm(kind feedback.Kind, body, email string, diagnostics, includeReport bool) {
	if kind != "" {
		w.f.Kind = kindIndex(kind)
	}
	if body != "" {
		w.f.Body = body
	}
	if email != "" {
		w.f.Email = email
	}
	w.f.Diagnostics, w.f.IncludeReport = diagnostics, includeReport
	w.invalidate()
}

// AddFiles adds files as a pick or a drop does ("+ Add" asks
// Options.PickFiles).
func (w *Window) AddFiles(paths ...string) { w.addFiles(paths); w.invalidate() }

// PickFiles presses "+ Add" (Options.PickFiles answers).
func (w *Window) PickFiles() { w.pickFiles() }

// RemoveFile presses ✕ on the i-th tile.
func (w *Window) RemoveFile(i int) { w.removeFile(i); w.invalidate() }

// Preview opens the preview of the i-th attachment (-1 closes it).
func (w *Window) Preview(i int) {
	if i >= len(w.files()) {
		i = -1
	}
	w.viewer = i
	if i < 0 {
		w.video.stop()
	}
	w.invalidate()
}

// OpenWith presses "Open in QuickTime Player" on the video.
func (w *Window) OpenWith() {
	if w.f.Video != "" {
		w.openFile(w.f.Video)
	}
}

// ShowWhatIsSent opens "What is sent": the diagnostics, or the report.
func (w *Window) ShowWhatIsSent(report bool) {
	if report {
		w.openReportSheet()
	} else {
		w.openDiagnosticsSheet()
	}
	w.invalidate()
}

// CloseSheet closes "What is sent".
func (w *Window) CloseSheet() { w.sheet = sheet{}; w.invalidate() }

// Send presses Send (the form is checked first: Snapshot.Problems).
func (w *Window) Send() { w.send(); w.invalidate() }

// CancelSend presses Cancel while sending.
func (w *Window) CancelSend() { w.cancelSend() }

// Retry and Discard press the failure's buttons.
func (w *Window) Retry()   { w.retry() }
func (w *Window) Discard() { w.discard(); w.invalidate() }

// ViewSent presses "View it" on the page of a feedback sent.
func (w *Window) ViewSent() {
	if w.doneID != "" {
		w.openThread(w.doneID)
	}
}

// Reply sends a reply in the thread open.
func (w *Window) Reply(body string) { w.replyText = body; w.sendReply() }

// Back goes back a page (the thread to the history, the history to the
// form).
func (w *Window) Back() {
	switch w.page {
	case pageThread:
		w.page, w.threadID = pageHistory, ""
	default:
		w.page = pageForm
	}
	w.invalidate()
}

// VideoServed counts the requests the video's web view made of the
// file and the bytes it got (ranges: a player seeking asks for parts).
func VideoServed() (requests, ranges, bytes int64) {
	return servedRequests.Load(), servedRanges.Load(), servedBytes.Load()
}

var servedRequests, servedRanges, servedBytes atomic.Int64

// VideoState is the <video> of the preview's web view wv (VideoWebView,
// read on the UI thread) as its page reports it: readyState, currentTime,
// duration, paused, error code, width (JSON). It waits for the page: call
// it off the UI thread.
func VideoState(ctx context.Context, wv *mygo.WebView) (string, error) {
	if wv == nil {
		return "", errors.New("feedbackui: no video in the preview")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	v, err := wv.Page().EvalContext(ctx, `const v=document.querySelector('video');if(!v)return null;`+
		`return {ready:v.readyState,time:v.currentTime,duration:v.duration,paused:v.paused,error:v.error?v.error.code:0,width:v.videoWidth}`)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(v)
	return string(b), nil
}

// VideoWebView is the preview's web view while it plays a video (nil
// otherwise).
func (w *Window) VideoWebView() *mygo.WebView {
	if w.video.path == "" {
		return nil
	}
	return w.video.wv
}
