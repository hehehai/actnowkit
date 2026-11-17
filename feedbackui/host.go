package feedbackui

import (
	"crypto/rand"
	"encoding/hex"
	"html"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Size of the window, in DIPs.
const (
	WindowWidth     = 600
	WindowHeight    = 760
	WindowMinWidth  = 520
	WindowMinHeight = 520
)

// host is the window on screen (a MyGo window; tests have their own).
type host interface {
	native() *mygo.Window
	focus()
	close()
	invalidate()
}

// headlessHost is the window of Options.Headless: nothing on screen.
type headlessHost struct{}

func (headlessHost) native() *mygo.Window { return nil }
func (headlessHost) focus()               {}
func (headlessHost) close()               {}
func (headlessHost) invalidate()          {}

type mygoHost struct{ win *mygo.Window }

func (h *mygoHost) native() *mygo.Window { return h.win }
func (h *mygoHost) focus()               { h.win.Show(); h.win.Focus() }
func (h *mygoHost) close()               { h.win.Close() }
func (h *mygoHost) invalidate()          { h.win.Invalidate() }

// newMyGoHost opens the window.
func newMyGoHost(w *Window) host {
	registerScheme(w.logf)
	win := mygo.NewWindow(mygo.WindowOptions{
		Title: w.title(), Width: WindowWidth, Height: WindowHeight, MinWidth: WindowMinWidth, MinHeight: WindowMinHeight,
		MaxWidth: 900, DisableMaximize: true, DisableFullScreen: true,
		TitleBarStyle: mygo.TitleBarHidden,
		Content:       ui.View(w.View),
	})
	if win == nil {
		return nil
	}
	win.Center()
	h := &mygoHost{win: win}
	win.OnClosed(func() {
		w.video.drop()
		w.post(func() { w.closed(h) })
	})
	return h
}

// ---- the video's web page ----

// scheme serves the video previewed to its web view (a page made from a
// string may not read files): actnowfb://localhost/v/<token>, only the
// file the window gave a token, with ranges (http.ServeContent).
const scheme = "actnowfb"

var (
	schemeOnce sync.Once
	served     sync.Map // token → path
)

func registerScheme(logf func(string)) {
	schemeOnce.Do(func() {
		if mygo.Protocol.IsHandled(scheme) {
			return
		}
		err := mygo.Protocol.Handle(scheme, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			tok := strings.TrimPrefix(r.URL.Path, "/v/")
			p, ok := served.Load(tok)
			if !ok {
				http.NotFound(rw, r)
				return
			}
			f, err := os.Open(p.(string))
			if err != nil {
				http.NotFound(rw, r)
				return
			}
			defer f.Close()
			st, err := f.Stat()
			if err != nil {
				http.NotFound(rw, r)
				return
			}
			rw.Header().Set("Content-Type", videoType(p.(string)))
			servedRequests.Add(1)
			if r.Header.Get("Range") != "" {
				servedRanges.Add(1)
			}
			cw := &countingWriter{ResponseWriter: rw}
			http.ServeContent(cw, r, "", st.ModTime(), f)
		}))
		if err != nil {
			logf("video scheme: " + err.Error())
		}
	})
}

func videoType(p string) string {
	if strings.HasSuffix(strings.ToLower(p), ".mp4") || strings.HasSuffix(strings.ToLower(p), ".m4v") {
		return "video/mp4"
	}
	return "video/quicktime"
}

// videoView is the web view playing the video previewed.
type videoView struct {
	wv    *mygo.WebView
	path  string // loaded
	token string
	err   string // the web view could not be made
}

// load plays path in the window's web view (made the first time).
func (v *videoView) load(win *mygo.Window, path string) {
	if win == nil || v.err != "" {
		return
	}
	if v.wv == nil {
		wv, err := win.NewWebView(mygo.WebViewOptions{BackgroundColor: "#000000"})
		if err != nil {
			v.err = err.Error()
			return
		}
		v.wv = wv
	}
	if v.path == path {
		return
	}
	if v.token != "" {
		served.Delete(v.token)
	}
	var b [12]byte
	_, _ = rand.Read(b[:])
	v.token, v.path = hex.EncodeToString(b[:]), path
	served.Store(v.token, path)
	src := scheme + "://localhost/v/" + v.token
	v.wv.Page().LoadHTML(`<!doctype html><html><head><meta charset="utf-8"><style>html,body{margin:0;height:100%;background:#000;overflow:hidden}`+
		`video{width:100%;height:100%;object-fit:contain;background:#000}</style></head><body>`+
		`<video controls autoplay playsinline src="`+html.EscapeString(src)+`"></video></body></html>`, scheme+"://localhost/")
}

// stop shows a blank page (WebKit lets go of the decoder).
func (v *videoView) stop() {
	if v.token != "" {
		served.Delete(v.token)
		v.token = ""
	}
	if v.wv != nil && v.path != "" {
		v.wv.Page().LoadHTML(`<!doctype html><html><body style="background:#000"></body></html>`, "about:blank")
	}
	v.path = ""
}

// drop forgets the web view (its window closed).
func (v *videoView) drop() {
	if v.token != "" {
		served.Delete(v.token)
	}
	*v = videoView{}
}

// countingWriter counts the bytes served (VideoServed).
type countingWriter struct{ http.ResponseWriter }

func (c *countingWriter) Write(b []byte) (int, error) {
	n, err := c.ResponseWriter.Write(b)
	servedBytes.Add(int64(n))
	return n, err
}
