package updatefeed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mygoSource is mygo.UpdateSource, which Source must satisfy without
// importing MyGo.
type mygoSource interface {
	Manifest(ctx context.Context, prereleases bool) (io.ReadCloser, error)
	Open(ctx context.Context, url string) (io.ReadCloser, error)
}

var _ mygoSource = (*Source)(nil)

func init() { retryWaits = []time.Duration{time.Millisecond, time.Millisecond} }

// feedServer serves the manifests of the two channels of /updates/app: the
// version of each ("" for 404), or a status other than 200 that fails it.
type feedServer struct {
	*httptest.Server
	mu       sync.Mutex
	versions map[string]string
	status   map[string]int
	asked    []string
}

func newFeedServer(t *testing.T) *feedServer {
	f := &feedServer{versions: map[string]string{}, status: map[string]int{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.asked = append(f.asked, r.URL.Path)
		if s := f.status[r.URL.Path]; s != 0 {
			http.Error(w, "down", s)
			return
		}
		v := f.versions[r.URL.Path]
		if v == "" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"version": %q, "url": "https://actnow.dev/updates/app/app-%s-darwin-arm64.tar.gz", "size": 1, "signature": "s", "release": "r"}`, v, v)
	}))
	t.Cleanup(f.Close)
	return f
}

const (
	stablePath = "/updates/app/update-darwin-arm64.json"
	betaPath   = "/updates/app/beta/update-darwin-arm64.json"
)

func (f *feedServer) set(stable, beta string, betaStatus int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.versions[stablePath], f.versions[betaPath] = stable, beta
	f.status[betaPath] = betaStatus
	f.asked = nil
}

func (f *feedServer) askedBeta() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.asked, betaPath)
}

func TestManifestChannels(t *testing.T) {
	f := newFeedServer(t)
	var userBeta atomic.Bool
	src := &Source{Base: f.URL + "/updates/app/", Target: "darwin-arm64", Beta: userBeta.Load}
	for _, c := range []struct {
		stable, beta string
		betaStatus   int
		pre, user    bool
		want         string // a version, "none" for ErrNotPublished, or "error: …"
	}{
		{"1.2.1", "1.3.0-beta.2", 0, false, false, "1.2.1"},
		{"1.2.1", "1.3.0-beta.2", 0, true, false, "1.3.0-beta.2"},
		{"1.2.1", "1.3.0-beta.2", 0, false, true, "1.3.0-beta.2"}, // Beta without prereleases
		{"1.3.0", "1.3.0-beta.2", 0, true, false, "1.3.0"},        // the newer, whichever channel
		{"1.3.0", "1.3.0", 0, true, false, "1.3.0"},
		{"1.3.0", "", 0, true, false, "1.3.0"},               // no beta channel yet
		{"1.3.0", "1.4.0-beta.1", 502, true, false, "1.3.0"}, // the beta channel down
		{"", "0.1.0-beta.2", 0, true, false, "0.1.0-beta.2"}, // only betas yet
		{"", "0.1.0-beta.2", 0, false, false, "none"},
		{"", "", 0, true, false, "none"},
		{"", "", 502, true, false, "error: 502"},
	} {
		f.set(c.stable, c.beta, c.betaStatus)
		userBeta.Store(c.user)
		got := ""
		body, err := src.Manifest(context.Background(), c.pre)
		switch {
		case errors.Is(err, ErrNotPublished):
			got = "none"
			if !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("ErrNotPublished is not fs.ErrNotExist: %v", err)
			}
		case err != nil:
			got = "error: " + err.Error()
		default:
			b, _ := io.ReadAll(body)
			got = version(b)
		}
		ok := got == c.want || strings.HasPrefix(c.want, "error: ") && strings.HasPrefix(got, "error: ") && strings.Contains(got, strings.TrimPrefix(c.want, "error: "))
		if !ok {
			t.Errorf("stable %q, beta %q (%d), prereleases %t, Beta %t: %s, want %s", c.stable, c.beta, c.betaStatus, c.pre, c.user, got, c.want)
		}
		if !c.pre && !c.user && f.askedBeta() {
			t.Errorf("read the beta channel without being asked (stable %q, beta %q)", c.stable, c.beta)
		}
	}
	if _, err := (&Source{}).Manifest(context.Background(), false); err == nil {
		t.Error("a source without Base read something")
	}
}

func TestManifestURL(t *testing.T) {
	s := &Source{Base: "https://actnow.dev/updates/pik", Target: "darwin-universal"}
	if got := s.ManifestURL(false); got != "https://actnow.dev/updates/pik/update-darwin-universal.json" {
		t.Errorf("releases: %s", got)
	}
	if got := s.ManifestURL(true); got != "https://actnow.dev/updates/pik/beta/update-darwin-universal.json" {
		t.Errorf("beta: %s", got)
	}
	s.Target = ""
	if got := s.ManifestURL(false); !strings.HasSuffix(got, "/update-"+runtime.GOOS+"-"+runtime.GOARCH+".json") {
		t.Errorf("default target: %s", got)
	}
}

func TestManifestTooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte(" "), MaxManifestSize+10))
	}))
	defer srv.Close()
	if _, err := (&Source{Base: srv.URL}).Manifest(context.Background(), false); err == nil || !strings.Contains(err.Error(), "larger") {
		t.Errorf("a manifest too large: %v", err)
	}
}

// Open goes through the app's transport, over HTTPS (plain HTTP on the
// loopback interface), follows redirects to HTTPS only, and tries a
// failing gateway again.
func TestOpenTransportAndRules(t *testing.T) {
	var tries atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/flaky":
			if tries.Add(1) < 3 {
				w.WriteHeader(http.StatusGatewayTimeout)
				return
			}
			io.WriteString(w, "ok")
		case "/insecure":
			http.Redirect(w, r, "http://example.com/x", http.StatusFound)
		case "/missing":
			http.NotFound(w, r)
		default:
			if r.Header.Get("User-Agent") != "Dicta/1.0" {
				t.Errorf("User-Agent %q", r.Header.Get("User-Agent"))
			}
			io.WriteString(w, "file")
		}
	}))
	defer srv.Close()
	var through atomic.Int32
	src := &Source{Base: srv.URL, UserAgent: "Dicta/1.0", Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		through.Add(1)
		return http.DefaultTransport.RoundTrip(r)
	})}
	read := func(path string) (string, error) {
		body, err := src.Open(context.Background(), srv.URL+path)
		if err != nil {
			return "", err
		}
		defer body.Close()
		b, err := io.ReadAll(body)
		return string(b), err
	}
	if got, err := read("/file"); got != "file" || err != nil || through.Load() != 1 {
		t.Errorf("read %q, %v, %d requests through the transport", got, err, through.Load())
	}
	src.UserAgent = ""
	if got, err := read("/flaky"); got != "ok" || err != nil || tries.Load() != 3 {
		t.Errorf("a flaky gateway: %q, %v in %d tries", got, err, tries.Load())
	}
	start := time.Now()
	if _, err := read("/insecure"); err == nil || !strings.Contains(err.Error(), "HTTPS") || time.Since(start) > 2*time.Second {
		t.Errorf("a redirect to plain HTTP: %v", err)
	}
	if _, err := read("/missing"); !errors.Is(err, ErrNotPublished) || !strings.Contains(err.Error(), "404") {
		t.Errorf("a missing file: %v", err)
	}
	if _, err := src.Open(context.Background(), "http://actnow.dev/updates/app/update-darwin-arm64.json"); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("plain HTTP: %v", err)
	}
}

// A download whose connection drops part way goes on with a Range request
// from where it stopped (If-Range the first response's ETag), also when
// the resume drops; a server that sends the whole file again fails it.
func TestOpenResumes(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcdef"), 40_000)
	var requests, ranges atomic.Int32
	var ignoreRange atomic.Bool
	var ifRange atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requests.Add(1)
		w.Header().Set("ETag", `"v1"`)
		if rg := r.Header.Get("Range"); rg != "" && !ignoreRange.Load() {
			ranges.Add(1)
			ifRange.Store(r.Header.Get("If-Range"))
			from, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(data)-1, len(data)))
			w.Header().Set("Content-Length", strconv.Itoa(len(data)-from))
			w.WriteHeader(http.StatusPartialContent)
			end := len(data)
			if n == 2 { // the first resume drops too, after some bytes
				end = from + 1000
			}
			w.Write(data[from:end])
			if end < len(data) {
				hijackClose(w)
			}
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		if n == 1 { // the first connection drops a third of the way
			w.Write(data[:len(data)/3])
			hijackClose(w)
			return
		}
		w.Write(data)
	}))
	defer srv.Close()
	var logs []string
	src := &Source{Base: srv.URL, ResumeWait: time.Millisecond, Log: func(s string) { logs = append(logs, s) }}
	body, err := src.Open(context.Background(), srv.URL+"/app.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(body)
	body.Close()
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("read %d bytes of %d: %v", len(got), len(data), err)
	}
	if ranges.Load() != 2 || ifRange.Load() != `"v1"` || src.Resumed() != 2 || len(logs) != 2 {
		t.Errorf("%d Range requests (If-Range %v), %d resumed, logs %q", ranges.Load(), ifRange.Load(), src.Resumed(), logs)
	}

	// The whole file again is not joined to the part read.
	requests.Store(0)
	ignoreRange.Store(true)
	body, err = src.Open(context.Background(), srv.URL+"/app.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(body)
	body.Close()
	if err == nil || !strings.Contains(err.Error(), "not the bytes from") {
		t.Errorf("a server that does not resume: %v", err)
	}
	// Without resumes, the first failure is the download's.
	requests.Store(0)
	ignoreRange.Store(false)
	src.Resumes = -1
	body, err = src.Open(context.Background(), srv.URL+"/app.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(body)
	body.Close()
	if err == nil {
		t.Error("a download that broke off was resumed with Resumes -1")
	}
}

// A body that stops arriving is given up on and resumed; a server that
// never answers is given up on.
func TestOpenStallAndHeaderTimeout(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 10_000)
	block := make(chan struct{})
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/silent" {
			<-block
			return
		}
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.Write(data[:100])
			w.(http.Flusher).Flush()
			<-block
			return
		}
		from, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(r.Header.Get("Range"), "bytes="), "-"))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(data)-1, len(data)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(data[from:])
	}))
	defer srv.Close()
	defer close(block) // before the server closes, which waits for its handlers
	src := &Source{Base: srv.URL, StallTimeout: 100 * time.Millisecond, HeaderTimeout: 100 * time.Millisecond, ResumeWait: time.Millisecond}
	body, err := src.Open(context.Background(), srv.URL+"/app.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(body)
	body.Close()
	if err != nil || !bytes.Equal(got, data) || src.Resumed() != 1 {
		t.Errorf("read %d bytes, %v, resumed %d", len(got), err, src.Resumed())
	}
	start := time.Now()
	if _, err := src.Open(context.Background(), srv.URL+"/silent"); err == nil || !strings.Contains(err.Error(), "headers") || time.Since(start) > 5*time.Second {
		t.Errorf("a server that never answers: %v after %v", err, time.Since(start))
	}
}

func TestCompare(t *testing.T) {
	ordered := []string{"0.1.0-beta.2", "0.1.0-beta.9", "0.1.0-beta.10", "0.1.0-rc.1", "0.1.0", "v0.1.1", "0.2.0-alpha", "0.2.0"}
	for i := range ordered {
		for j := range ordered {
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got := Compare(ordered[i], ordered[j]); got != want {
				t.Errorf("Compare(%s, %s) = %d, want %d", ordered[i], ordered[j], got, want)
			}
		}
	}
	for v, want := range map[string]bool{"1.2.3": false, "v1.2.3+b-5": false, "1.3.0-beta.1": true, "v1.3.0-rc1+x": true} {
		if IsPrerelease(v) != want {
			t.Errorf("IsPrerelease(%s) = %t", v, !want)
		}
	}
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// hijackClose ends the connection of a response under way.
func hijackClose(w http.ResponseWriter) {
	w.(http.Flusher).Flush()
	if c, _, err := w.(http.Hijacker).Hijack(); err == nil {
		c.Close()
	}
}
