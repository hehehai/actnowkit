package feedbackui

import (
	"context"
	"errors"
	"image"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/hehehai/actnowkit/family"
	"github.com/hehehai/actnowkit/feedback"
)

// Options of New. Manager and AppName are needed; the rest is optional.
type Options struct {
	Manager *feedback.Manager
	// AppName as people see it ("Dicta"); Site the site's host
	// ("actnow.dev", "refto.one"); PrivacyURL its privacy policy.
	AppName, Site, PrivacyURL string
	// Icon: the app's icon in the window's header (nil: its initial on the
	// accent color).
	Icon  *ui.Bitmap
	Theme Theme
	// Lang is the app's language ("en", "zh-Hans"): Chinese for "zh…",
	// English otherwise (nil: English).
	Lang func() string
	// T translates a text from the app's catalogue (nil: Texts as they
	// are, in Lang).
	T func(family.Text) string
	// Texts replaces the wordings (nil: DefaultTexts()).
	Texts *Texts

	// OpenURL opens a web address (nil: the browser, mygo.Shell);
	// OpenFile a file in its default app ("Open in QuickTime Player"; nil:
	// mygo.Shell.OpenPath).
	OpenURL  func(url string)
	OpenFile func(path string) error
	// PickFiles asks for images and a video (it blocks: it is called off
	// the UI thread; nil: MyGo's open dialog over the window).
	PickFiles func(parent *mygo.Window, title string) ([]string, error)
	// Post runs fn on the UI thread and redraws the app's windows (the
	// history panel in its Settings); the feedback window redraws itself.
	// nil: mygo.RunOnMain.
	Post func(fn func())
	// Async runs fn off the UI thread (nil: a goroutine).
	Async func(fn func())
	// Log gets a line per failure. Optional.
	Log func(string)

	// BeforeCollect runs on the UI thread before the diagnostics are
	// collected (an app reading its settings for CollectConfig.Settings).
	BeforeCollect func()
	// StepName shows a step of an error report ("import_audio" →
	// "Import audio"; nil: its words, the first capitalised).
	StepName func(string) string
	// Converter makes the thumbnails of HEIC images and videos (nil:
	// feedback.SystemConverter()); Thumbnail replaces the whole making.
	Converter feedback.Converter
	Thumbnail func(path string, px int) (image.Image, error)

	// WindowOpened and WindowClosed: a menu-bar app shows itself in the
	// Dock while the window is open (mygo.App.SetActivationPolicy).
	WindowOpened func(*mygo.Window)
	WindowClosed func()

	// Headless: Open makes no native window (an app's tests, which draw
	// View in their own ui.Tester): the window counts as open until Close.
	Headless bool
}

// Window is the feedback window of an app: one, opened and closed as the
// user asks (Open, Close), its state kept between. Its methods are for
// the UI thread.
type Window struct {
	opt   Options
	m     *feedback.Manager
	texts *Texts

	h       host // the window while it is open
	newHost func(*Window) host

	f        form
	page     page
	tried    bool
	problems []feedback.Problem
	fileErr  string // a file picked that cannot be used
	who      feedback.Identity
	whoAsked bool

	st        feedback.State
	mine      string // the LocalID of the submission this window sent
	awaiting  bool   // Submit called, its first state not seen yet
	cancel    context.CancelFunc
	cancelled bool
	prepErr   string // the preparation failed (nothing in the outbox)
	doneNum   string
	doneID    string

	history    []feedback.HistoryEntry
	refreshing bool
	refreshed  bool
	histErr    string

	threadID  string
	thread    *feedback.Thread
	threadErr string
	loading   bool
	replyText string
	replying  bool

	sheet sheet // "What is sent"

	viewer int // the attachment previewed (index in files()), -1 none
	thumbs map[string]*thumb
	full   map[string]*thumb
	video  videoView

	unsub func()
}

type page int

const (
	pageForm page = iota
	pageDone
	pageHistory
	pageThread
)

// form is the new feedback.
type form struct {
	Kind          int // in feedback.Kinds
	Body, Email   string
	Images        []string
	Video         string
	Diagnostics   bool
	Report        *feedback.Report
	IncludeReport bool
}

func newForm() form { return form{Diagnostics: true} }

// dirty: the user wrote or picked something.
func (f *form) dirty() bool {
	return strings.TrimSpace(f.Body) != "" || len(f.Images) > 0 || f.Video != ""
}

func (f *form) draft() feedback.Draft {
	k := feedback.KindBug
	if f.Kind >= 0 && f.Kind < len(feedback.Kinds) {
		k = feedback.Kinds[f.Kind]
	}
	return feedback.Draft{Kind: k, Body: f.Body, Email: strings.TrimSpace(f.Email), Images: append([]string(nil), f.Images...),
		Video: f.Video, Diagnostics: f.Diagnostics, Report: f.Report, IncludeReport: f.Report != nil && f.IncludeReport}
}

func kindIndex(k feedback.Kind) int {
	for i, x := range feedback.Kinds {
		if x == k {
			return i
		}
	}
	return 0
}

// New makes the window of an app (not shown: Open shows it). It follows
// the Manager's state from now on; Stop ends that.
func New(o Options) *Window {
	w := &Window{opt: o, m: o.Manager, texts: o.Texts, f: newForm(), viewer: -1,
		thumbs: map[string]*thumb{}, full: map[string]*thumb{}, newHost: newMyGoHost}
	if w.texts == nil {
		w.texts = DefaultTexts()
	}
	if o.Headless {
		w.newHost = func(*Window) host { return headlessHost{} }
	}
	if w.m != nil {
		w.st = w.m.State()
		w.history = w.m.History()
		w.unsub = w.m.Subscribe(func(st feedback.State) { w.post(func() { w.stateChanged(st) }) })
	}
	return w
}

// Stop ends following the Manager (the app quits) and closes the window.
func (w *Window) Stop() {
	if w.unsub != nil {
		w.unsub()
		w.unsub = nil
	}
	w.Close()
}

// Manager is the window's.
func (w *Window) Manager() *feedback.Manager { return w.m }

// IsOpen reports whether the window shows.
func (w *Window) IsOpen() bool { return w.h != nil }

// NativeWindow is the open window (nil when closed).
func (w *Window) NativeWindow() *mygo.Window {
	if w.h == nil {
		return nil
	}
	return w.h.native()
}

// Open shows the window, or brings it to the front, with p: a new
// feedback filled as p says (Prefill.Draft). When the user has already
// written or picked something, p adds to it (its report, its files where
// there is room, its text when the description is empty) and nothing is
// cleared. A feedback being sent stays as it is.
func (w *Window) Open(p feedback.Prefill) {
	if w.page == pageDone {
		w.page = pageForm
	}
	if !w.busy() && !w.failed() {
		w.prefill(p)
		if w.page != pageForm && (p.Report != nil || p.Body != "" || len(p.Images) > 0 || p.Video != "") {
			w.page = pageForm
		}
	}
	w.show()
}

// OpenHistory shows the window at Your feedback.
func (w *Window) OpenHistory() {
	w.page = pageHistory
	w.Refresh()
	w.show()
}

// OpenThread shows the window at one feedback of the history.
func (w *Window) OpenThread(id string) {
	w.openThread(id)
	w.show()
}

func (w *Window) show() {
	if w.h == nil {
		w.h = w.newHost(w)
		if w.h == nil {
			return
		}
		if w.opt.WindowOpened != nil {
			w.opt.WindowOpened(w.h.native())
		}
	}
	w.askWho()
	w.h.focus()
	w.invalidate()
}

// Invalidate redraws the window (an app's setting it follows changed:
// its language, its appearance).
func (w *Window) Invalidate() { w.invalidate() }

// title is the window's title in the app's language.
func (w *Window) title() string {
	t := w.tx(w.texts.WindowTitle)
	if w.opt.AppName != "" {
		t += " — " + w.opt.AppName
	}
	return t
}

// Close closes the window (its draft stays for the next Open).
func (w *Window) Close() {
	if w.h != nil {
		h := w.h
		h.close()
		w.closed(h)
	}
}

// closed: the window closed (Close, or the user closed it).
func (w *Window) closed(h host) {
	if w.h != h {
		return
	}
	w.h = nil
	w.viewer = -1
	w.video.stop()
	w.sheet = sheet{}
	if w.page == pageDone {
		w.page = pageForm
	}
	if w.failed() {
		// It stays in the outbox (sent on the next Resume, or retried from
		// Your feedback): this window starts a new one.
		w.mine = ""
		w.f, w.tried, w.problems = newForm(), false, nil
		w.m.Reset()
	}
	if w.page == pageThread {
		w.page = pageHistory
	}
	if w.opt.WindowClosed != nil {
		w.opt.WindowClosed()
	}
}

// prefill applies p to the form.
func (w *Window) prefill(p feedback.Prefill) {
	if !w.f.dirty() {
		d := p.Draft()
		w.f = form{Kind: kindIndex(d.Kind), Body: d.Body, Email: w.f.Email, Images: d.Images, Video: d.Video,
			Diagnostics: true, Report: d.Report, IncludeReport: d.IncludeReport}
		w.tried, w.problems, w.fileErr, w.prepErr = false, nil, "", ""
		w.page = pageForm
		return
	}
	if p.Report != nil {
		w.f.Report, w.f.IncludeReport = p.Report, true
	}
	if strings.TrimSpace(w.f.Body) == "" && p.Body != "" {
		w.f.Body = p.Body
	}
	for _, img := range p.Images {
		if len(w.f.Images) < feedback.MaxImages {
			w.f.Images = append(w.f.Images, img)
		}
	}
	if w.f.Video == "" {
		w.f.Video = p.Video
	}
}

// ---- threads ----

func (w *Window) post(fn func()) {
	run := func() {
		fn()
		w.invalidate()
	}
	if w.opt.Post != nil {
		w.opt.Post(run)
		return
	}
	mygo.RunOnMain(run)
}

func (w *Window) async(fn func()) {
	if w.opt.Async != nil {
		w.opt.Async(fn)
		return
	}
	go fn()
}

func (w *Window) invalidate() {
	if w.h != nil {
		w.h.invalidate()
	}
}

func (w *Window) logf(s string) {
	if w.opt.Log != nil {
		w.opt.Log("feedbackui: " + s)
	}
}

func (w *Window) openURL(u string) {
	if u == "" {
		return
	}
	if w.opt.OpenURL != nil {
		w.opt.OpenURL(u)
		return
	}
	if err := mygo.Shell.OpenExternal(u); err != nil {
		w.logf("open " + u + ": " + err.Error())
	}
}

func (w *Window) openFile(p string) {
	var err error
	if w.opt.OpenFile != nil {
		err = w.opt.OpenFile(p)
	} else {
		err = mygo.Shell.OpenPath(p)
	}
	if err != nil {
		w.logf("open file: " + err.Error())
	}
}

// ---- the Manager's state ----

func (w *Window) ours(st feedback.State) bool {
	return st.Current != nil && w.mine != "" && st.Current.LocalID == w.mine
}

// busy: this window's feedback is being prepared or sent.
func (w *Window) busy() bool {
	if w.awaiting {
		return true
	}
	return w.ours(w.st) && (w.st.Phase == feedback.Preparing || w.st.Phase == feedback.Sending)
}

// failed: this window's feedback could not be sent (it waits in the
// outbox: Retry or Discard).
func (w *Window) failed() bool { return w.ours(w.st) && w.st.Phase == feedback.Failed }

func (w *Window) stateChanged(st feedback.State) {
	if w.awaiting && st.Current != nil && st.Phase == feedback.Preparing {
		w.mine, w.awaiting = st.Current.LocalID, false
	}
	w.st = st
	if !w.ours(st) {
		if st.Phase == feedback.Sent && w.m != nil {
			w.history = w.m.History() // a submission of the outbox went
		}
		return
	}
	switch st.Phase {
	case feedback.Sent:
		w.doneNum, w.doneID = st.Current.Number, st.Current.ID
		w.mine, w.cancel = "", nil
		w.f, w.tried, w.problems, w.fileErr = form{Diagnostics: true, Email: w.f.Email}, false, nil, ""
		w.page = pageDone
		w.history = w.m.History()
		w.dropMedia()
	case feedback.Failed:
		switch {
		case w.cancelled:
			// Cancel while sending: back to the form as it was.
			w.cancelled = false
			if st.Step != "prepare" {
				_ = w.m.Discard(w.mine)
			}
			w.mine = ""
			w.m.Reset()
		case st.Step == "prepare":
			w.prepErr = errText(st.Err)
			w.mine = ""
			w.m.Reset()
		}
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// askWho asks the site whether the email may stay empty (once).
func (w *Window) askWho() {
	if w.m == nil || w.whoAsked {
		return
	}
	w.whoAsked = true
	m := w.m
	w.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		who, err := m.Identity(ctx)
		w.post(func() {
			w.who = who
			if err != nil {
				w.whoAsked = false // asked again next time
			}
		})
	})
}

// LicenceChanged forgets the site's answer on the email (another code,
// another owner).
func (w *Window) LicenceChanged() {
	if w.m != nil {
		w.m.ForgetIdentity()
	}
	w.whoAsked = false
	w.who = feedback.Identity{}
	if w.h != nil {
		w.askWho()
	}
}

// ---- actions ----

// send checks the form and submits it off the UI thread.
func (w *Window) send() {
	if w.m == nil || w.busy() || w.failed() {
		return
	}
	w.tried = true
	w.prepErr = ""
	d := w.f.draft()
	w.problems = feedback.Validate(d, w.who)
	if len(w.problems) > 0 {
		return
	}
	if w.opt.BeforeCollect != nil {
		w.opt.BeforeCollect()
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel, w.cancelled, w.awaiting = cancel, false, true
	m := w.m
	w.async(func() {
		_, err := m.Submit(ctx, d)
		w.post(func() {
			if w.awaiting { // refused before preparing (the form, as the site sees it)
				w.awaiting = false
				if err != nil {
					w.prepErr = err.Error()
				}
			}
		})
	})
}

// cancelSend stops the sending: the form comes back as it was.
func (w *Window) cancelSend() {
	if w.cancel == nil || !w.busy() {
		return
	}
	w.cancelled = true
	w.cancel()
}

func (w *Window) retry() {
	if !w.failed() {
		return
	}
	id, m := w.mine, w.m
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.async(func() { _ = m.Retry(ctx, id) })
}

func (w *Window) discard() {
	if !w.failed() {
		return
	}
	if err := w.m.Discard(w.mine); err != nil {
		w.logf("discard: " + err.Error())
	}
	w.mine = ""
	w.m.Reset()
}

// retryPending sends a submission of the outbox again (Your feedback).
func (w *Window) retryPending(id string) {
	if w.m == nil || w.busy() {
		return
	}
	m := w.m
	w.async(func() { _ = m.Retry(context.Background(), id) })
}

func (w *Window) discardPending(id string) {
	if w.m == nil {
		return
	}
	if err := w.m.Discard(id); err != nil {
		w.logf("discard: " + err.Error())
	}
	w.st.Pending = w.m.Pending()
}

// Refresh asks the site for the statuses and replies of the history.
func (w *Window) Refresh() {
	if w.m == nil || w.refreshing {
		return
	}
	w.refreshing, w.refreshed = true, true
	m := w.m
	w.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := m.Refresh(ctx)
		w.post(func() {
			w.refreshing = false
			w.history = m.History()
			w.histErr = errText(err)
		})
	})
}

// Unread counts the feedback with replies not read (a dot on the app's
// Settings → Feedback).
func (w *Window) Unread() int {
	n := 0
	for _, e := range w.history {
		if e.Unread() {
			n++
		}
	}
	return n
}

func (w *Window) openThread(id string) {
	if w.m == nil {
		return
	}
	w.page, w.threadID, w.thread, w.threadErr, w.loading, w.replyText = pageThread, id, nil, "", true, ""
	m := w.m
	w.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		t, err := m.Thread(ctx, id)
		w.post(func() {
			if w.threadID != id {
				return
			}
			w.loading = false
			w.history = m.History()
			if err != nil {
				w.threadErr = err.Error()
				return
			}
			w.thread = &t
		})
	})
}

func (w *Window) sendReply() {
	body := strings.TrimSpace(w.replyText)
	if w.m == nil || w.thread == nil || body == "" || w.replying {
		return
	}
	w.replying = true
	id, m := w.threadID, w.m
	w.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		msg, err := m.Reply(ctx, id, body)
		w.post(func() {
			w.history = m.History()
			w.replying = false
			if err != nil {
				w.threadErr = err.Error()
				return
			}
			w.replyText, w.threadErr = "", ""
			if w.thread != nil && w.threadID == id {
				w.thread.Messages = append(w.thread.Messages, msg)
			}
		})
	})
}

// ---- files ----

var errNoPicker = errors.New("feedbackui: no file dialog")

// pickFiles asks for images or a video.
func (w *Window) pickFiles() {
	title := w.tx(w.texts.AddFiles)
	parent := w.NativeWindow()
	pick := w.opt.PickFiles
	if pick == nil {
		pick = func(parent *mygo.Window, title string) ([]string, error) {
			return mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: parent, Title: title, Multiple: true,
				Filters: []mygo.FileFilter{{Name: title, Extensions: []string{"png", "jpg", "jpeg", "gif", "heic", "heif", "webp", "tiff", "tif", "bmp", "mov", "mp4", "m4v"}}}})
		}
	}
	w.async(func() {
		paths, err := pick(parent, title)
		if err != nil || len(paths) == 0 {
			return
		}
		w.post(func() { w.addFiles(paths) })
	})
}

// addFiles adds picked or dropped files: movies to the video's place,
// the others as images, while there is room.
func (w *Window) addFiles(paths []string) {
	w.fileErr = ""
	for _, p := range paths {
		if isVideo(p) {
			if pr := feedback.CheckVideo(p); pr != "" {
				w.fileErr = w.problemText(pr)
				continue
			}
			if w.f.Video != "" {
				w.fileErr = w.tx(w.texts.AttachHint) // one video
				continue
			}
			w.f.Video = p
			continue
		}
		if pr := feedback.CheckImage(p); pr != "" {
			w.fileErr = w.problemText(pr)
			continue
		}
		if len(w.f.Images) >= feedback.MaxImages {
			w.fileErr = w.problemText(feedback.ProblemTooManyImages)
			continue
		}
		w.f.Images = append(w.f.Images, p)
	}
}

func isVideo(p string) bool {
	l := strings.ToLower(p)
	for _, ext := range []string{".mov", ".mp4", ".m4v", ".qt"} {
		if strings.HasSuffix(l, ext) {
			return true
		}
	}
	return feedback.CheckVideo(p) == ""
}

// files are the attachments in the grid's order: the images, the video.
func (w *Window) files() []string {
	fs := append([]string(nil), w.f.Images...)
	if w.f.Video != "" {
		fs = append(fs, w.f.Video)
	}
	return fs
}

// removeFile drops the i-th of files().
func (w *Window) removeFile(i int) {
	if fs := w.files(); i >= 0 && i < len(fs) {
		delete(w.thumbs, fs[i])
		delete(w.full, fs[i])
	}
	if i < len(w.f.Images) {
		w.f.Images = append(w.f.Images[:i:i], w.f.Images[i+1:]...)
	} else if i == len(w.f.Images) {
		w.f.Video = ""
		w.video.stop()
	}
	w.fileErr = ""
	if w.viewer >= len(w.files()) {
		w.viewer = len(w.files()) - 1
	}
}

// dropMedia forgets the thumbnails of files no longer in the form.
func (w *Window) dropMedia() {
	keep := map[string]bool{}
	for _, p := range w.files() {
		keep[p] = true
	}
	for p := range w.thumbs {
		if !keep[p] {
			delete(w.thumbs, p)
		}
	}
	for p := range w.full {
		if !keep[p] {
			delete(w.full, p)
		}
	}
	w.viewer = -1
	w.video.stop()
}
