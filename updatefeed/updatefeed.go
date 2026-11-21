// Package updatefeed is the update feed of the ActNow apps on actnow.dev
// (or refto.one) as a source of MyGo's updater (mygo.Updater.SetSource):
//
//	<Base>/update-<target>.json        releases (the stable channel)
//	<Base>/beta/update-<target>.json   the beta channel: the newest version
//	                                   of both channels, as the site keeps it
//
// A Source reads the releases' manifest, or with the beta channel both
// manifests and the newer of the two; downloads the archive or delta the
// manifest names through the app's own transport (its proxy rule), going
// on with a Range request from where a download broke off; and says
// "nothing published" for a channel without a manifest (404), which MyGo
// reports as no update. It does not check what it downloads: MyGo checks
// the size and the release signature, made with the app's key, of every
// archive and delta, whatever its source.
//
// actnowkit does not depend on MyGo: Source has the methods of
// mygo.UpdateSource (Go interfaces are structural), so the app passes it
// as it is:
//
//	mygo.Updater.SetSource(&updatefeed.Source{
//		Base:      "https://actnow.dev/updates/dicta",
//		Target:    "darwin-arm64",
//		Beta:      func() bool { return settings.Beta() },
//		Transport: netclient.Transport(),
//	})
package updatefeed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// BetaDir is the directory of the beta channel beside the releases'
// manifests (MyGo's update.PrereleaseDir, the site's channel directory).
const BetaDir = "beta"

// MaxManifestSize is the most a manifest may hold, as MyGo reads it.
const MaxManifestSize = 1 << 20

// ErrNotPublished is wrapped by Manifest's error when the channels read
// have no manifest (404): nothing is published there yet, or every
// release was withdrawn. It is an fs.ErrNotExist (errors.Is), which MyGo's
// Check reports as no update rather than as an error.
var ErrNotPublished = fmt.Errorf("updatefeed: nothing published: %w", fs.ErrNotExist)

// Source is an update feed of the ActNow site. Its zero value is not
// usable: set Base. Safe for concurrent use; its fields must not change
// once in use.
type Source struct {
	// Base is the feed's directory, the updates.url of the releases:
	// "https://actnow.dev/updates/dicta" (without /beta).
	Base string
	// Target is MyGo's build target, which names the manifest:
	// "darwin-arm64", "darwin-universal". Empty: GOOS-GOARCH.
	Target string
	// Beta reports whether the app reads the beta channel even when MyGo
	// does not ask for prereleases (mygo.Updater.SetPrereleases), such as
	// an app running a beta whose user never chose: nil reads it only when
	// asked. Called at each check.
	Beta func() bool
	// Transport is how the source reaches the network: the app's proxy
	// rule, retries, a fallback to a direct connection. Nil:
	// http.DefaultTransport. Redirects are followed to HTTPS only.
	Transport http.RoundTripper
	// UserAgent of the requests; empty: "actnowkit-updatefeed".
	UserAgent string
	// HeaderTimeout is how long a response's headers may take (default
	// 30 s); StallTimeout how long a body may send nothing before the
	// connection is given up and resumed (default 1 minute).
	HeaderTimeout, StallTimeout time.Duration
	// Resumes is how many times in a row a download that broke off is
	// asked for again from where it stopped (default 5; negative: none),
	// ResumeWait how long before the first time, doubling after each
	// (default 1 s). A resume that brings bytes starts the count again.
	Resumes    int
	ResumeWait time.Duration
	// Log receives a line for each fallback and resume (nil: none).
	Log func(string)

	// resumed counts the resumes of all downloads (Resumed).
	resumed atomic.Int64
}

// Resumed is how many times a download of the source went on from where
// it broke off, since the source was made (for diagnostics).
func (s *Source) Resumed() int64 { return s.resumed.Load() }

// ManifestURL is the URL of the manifest of a channel: the releases', or
// with beta the beta channel's.
func (s *Source) ManifestURL(beta bool) string {
	base := strings.TrimSuffix(s.Base, "/")
	if beta {
		base += "/" + BetaDir
	}
	target := s.Target
	if target == "" {
		target = runtime.GOOS + "-" + runtime.GOARCH
	}
	return base + "/update-" + url.PathEscape(target) + ".json"
}

// Manifest returns the manifest of the app's target, as MyGo's
// UpdateSource: the releases', or with prereleases (or Beta) the newer of
// the beta channel's and the releases'. When one channel's cannot be read
// it returns the other's; when neither has one, an error wrapping
// ErrNotPublished; else the releases' error, or the beta channel's when
// the releases have nothing published.
func (s *Source) Manifest(ctx context.Context, prereleases bool) (io.ReadCloser, error) {
	if s.Base == "" {
		return nil, errors.New("updatefeed: no Base")
	}
	beta := prereleases || s.Beta != nil && s.Beta()
	if !beta {
		b, err := s.readManifest(ctx, s.ManifestURL(false))
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(b)), nil
	}
	pre, perr := s.readManifest(ctx, s.ManifestURL(true))
	if perr != nil && ctx.Err() != nil {
		return nil, perr
	}
	rel, err := s.readManifest(ctx, s.ManifestURL(false))
	var b []byte
	switch {
	case perr == nil && err == nil:
		b = rel
		if Compare(version(pre), version(rel)) > 0 {
			b = pre
		}
	case perr == nil:
		if !errors.Is(err, ErrNotPublished) {
			s.logf("checked the beta channel, as the releases' manifest could not be read: %v", err)
		}
		b = pre
	case err == nil:
		if !errors.Is(perr, ErrNotPublished) {
			s.logf("checked the releases, as the beta channel's manifest could not be read: %v", perr)
		}
		b = rel
	case errors.Is(err, ErrNotPublished) && !errors.Is(perr, ErrNotPublished):
		return nil, perr
	default:
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

// readManifest reads the manifest at u, at most MaxManifestSize.
func (s *Source) readManifest(ctx context.Context, u string) ([]byte, error) {
	body, err := s.Open(ctx, u)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	b, err := io.ReadAll(io.LimitReader(body, MaxManifestSize+1))
	switch {
	case err != nil:
		return nil, fmt.Errorf("reading %s: %w", u, err)
	case len(b) > MaxManifestSize:
		return nil, fmt.Errorf("%s is larger than %d bytes", u, MaxManifestSize)
	}
	return b, nil
}

// version is the version of a manifest, "" when it has none.
func version(manifest []byte) string {
	var m struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(manifest, &m)
	return m.Version
}

// Open returns the file at rawURL, which the manifest names (the archive
// of an update or one of its deltas), as MyGo's UpdateSource: over HTTPS
// (plain HTTP on the loopback interface, for tests), through Transport. A
// body whose connection fails part way is asked for again from where it
// stopped (Resumes). A 404 is ErrNotPublished.
func (s *Source) Open(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopback(u.Hostname())) {
		return nil, fmt.Errorf("updatefeed: refusing %s: updates need HTTPS", u.Redacted())
	}
	body, validator, err := s.request(ctx, rawURL, 0, "")
	if err != nil {
		return nil, err
	}
	return &resumingBody{s: s, ctx: ctx, url: rawURL, body: body, validator: validator}, nil
}

// Retries of a request that failed for a while (a network error, 429,
// 5xx): three tries in all.
var retryWaits = []time.Duration{time.Second, 3 * time.Second}

// StatusError is a response other than the one asked for. A 404 is
// ErrNotPublished (errors.Is).
type StatusError struct {
	URL    string
	Code   int
	Status string
}

func (e *StatusError) Error() string { return "GET " + e.URL + ": " + e.Status }

// Is reports a 404 as ErrNotPublished and fs.ErrNotExist.
func (e *StatusError) Is(target error) bool {
	return e.Code == http.StatusNotFound && (target == ErrNotPublished || target == fs.ErrNotExist)
}

// errNeedHTTPS is the error of a redirect to plain HTTP.
var errNeedHTTPS = errors.New("updates need HTTPS")

func (s *Source) client() *http.Client {
	rt := s.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	return &http.Client{Transport: rt, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if req.URL.Scheme != "https" && !(req.URL.Scheme == "http" && isLoopback(req.URL.Hostname())) {
			return fmt.Errorf("refusing a redirect to %s: %w", req.URL.Redacted(), errNeedHTTPS)
		}
		return nil
	}}
}

// request asks for rawURL from byte offset on: with offset > 0, a Range
// request (If-Range validator: the first response's ETag or
// Last-Modified, so that a file that changed is not joined to another),
// which must be answered with 206 Partial Content from offset. It returns
// the body, which gives up when nothing arrives for StallTimeout, and the
// response's validator.
func (s *Source) request(ctx context.Context, rawURL string, offset int64, validator string) (io.ReadCloser, string, error) {
	client := s.client()
	headerTimeout := orDuration(s.HeaderTimeout, 30*time.Second)
	for try := 0; ; try++ {
		reqCtx, cancel := context.WithCancelCause(ctx)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
		if err != nil {
			cancel(nil)
			return nil, "", err
		}
		ua := s.UserAgent
		if ua == "" {
			ua = "actnowkit-updatefeed"
		}
		req.Header.Set("User-Agent", ua)
		want := http.StatusOK
		if offset > 0 {
			want = http.StatusPartialContent
			req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
			if validator != "" {
				req.Header.Set("If-Range", validator)
			}
		}
		// The headers within headerTimeout: the app's transport may have
		// no deadline of its own.
		timer := time.AfterFunc(headerTimeout, func() { cancel(errHeaderTimeout) })
		resp, err := client.Do(req)
		timer.Stop()
		var wait time.Duration
		switch {
		case err != nil:
			if errors.Is(context.Cause(reqCtx), errHeaderTimeout) {
				err = fmt.Errorf("GET %s: %w", rawURL, errHeaderTimeout)
			}
			cancel(nil)
			if ctx.Err() != nil || try >= len(retryWaits) || errors.Is(err, errNeedHTTPS) {
				return nil, "", err
			}
		case resp.StatusCode == want && (offset == 0 || contentRangeFrom(resp.Header.Get("Content-Range")) == offset):
			v := resp.Header.Get("ETag")
			if v == "" || strings.HasPrefix(v, "W/") {
				v = resp.Header.Get("Last-Modified")
			}
			return newStalledBody(resp.Body, cancel, orDuration(s.StallTimeout, time.Minute)), v, nil
		case offset > 0 && resp.StatusCode/100 == 2:
			// The whole file, or another part: the server does not resume
			// this one, or it changed.
			resp.Body.Close()
			cancel(nil)
			return nil, "", fmt.Errorf("GET %s: %s, not the bytes from %d", rawURL, resp.Status, offset)
		default:
			resp.Body.Close()
			cancel(nil)
			if !transient(resp.StatusCode) || try >= len(retryWaits) {
				return nil, "", &StatusError{URL: rawURL, Code: resp.StatusCode, Status: resp.Status}
			}
			wait = retryAfter(resp.Header.Get("Retry-After"))
		}
		wait = max(wait, retryWaits[try])
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-time.After(wait):
		}
	}
}

var errHeaderTimeout = errors.New("no response headers in time")

// transient reports whether a response of status code may succeed when
// asked again.
func transient(code int) bool {
	return code == http.StatusTooManyRequests || code == http.StatusInternalServerError || code == http.StatusBadGateway ||
		code == http.StatusServiceUnavailable || code == http.StatusGatewayTimeout
}

// retryAfter is the wait of a Retry-After header in seconds, up to 30 s.
func retryAfter(h string) time.Duration {
	n, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || n <= 0 {
		return 0
	}
	return min(time.Duration(n)*time.Second, 30*time.Second)
}

// contentRangeFrom is the first byte of a Content-Range header,
// "bytes 100-199/200", or -1.
func contentRangeFrom(h string) int64 {
	rest, ok := strings.CutPrefix(h, "bytes ")
	from, _, ok2 := strings.Cut(rest, "-")
	n, err := strconv.ParseInt(from, 10, 64)
	if !ok || !ok2 || err != nil {
		return -1
	}
	return n
}

// resumingBody is the body of Open: when its connection fails part way it
// asks for the rest with a Range request.
type resumingBody struct {
	s         *Source
	ctx       context.Context
	url       string
	body      io.ReadCloser // nil after a failure, until resumed
	validator string
	n         int64 // bytes read
	failures  int   // resumes in a row without a byte
	err       error // the failure being resumed from
}

func (b *resumingBody) Read(p []byte) (int, error) {
	resumes := b.s.Resumes
	if resumes == 0 {
		resumes = 5
	}
	for {
		if b.body == nil {
			if err := b.resume(); err != nil {
				return 0, err
			}
		}
		n, err := b.body.Read(p)
		b.n += int64(n)
		if n > 0 {
			b.failures = 0
		}
		if err == nil || err == io.EOF || b.ctx.Err() != nil || b.failures >= resumes {
			return n, err
		}
		b.body.Close()
		b.body, b.err = nil, err
		if n > 0 {
			return n, nil // the next read resumes
		}
	}
}

// resume waits, then asks for the rest of the body, or returns the failure
// it resumes from when that does not work.
func (b *resumingBody) resume() error {
	wait := orDuration(b.s.ResumeWait, time.Second) << b.failures
	b.failures++
	select {
	case <-b.ctx.Done():
		return b.err
	case <-time.After(wait):
	}
	b.s.resumed.Add(1)
	b.s.logf("resuming %s from byte %d after: %v", b.url, b.n, b.err)
	body, _, err := b.s.request(b.ctx, b.url, b.n, b.validator)
	if err != nil {
		return fmt.Errorf("%w (resuming: %v)", b.err, err)
	}
	b.body = body
	return nil
}

func (b *resumingBody) Close() error {
	if b.body == nil {
		return nil
	}
	return b.body.Close()
}

// stalledBody cancels the request of a body that stops arriving, and says
// so instead of the canceled request it would otherwise be read as.
type stalledBody struct {
	body    io.ReadCloser
	cancel  context.CancelCauseFunc
	d       time.Duration
	last    atomic.Int64
	stalled atomic.Bool
	done    atomic.Bool
	timer   *time.Timer
}

var errStalled = errors.New("the download stalled: the connection sent nothing")

func newStalledBody(body io.ReadCloser, cancel context.CancelCauseFunc, d time.Duration) *stalledBody {
	b := &stalledBody{body: body, cancel: cancel, d: d}
	b.last.Store(time.Now().UnixNano())
	b.timer = time.AfterFunc(1<<63-1, b.check)
	b.timer.Stop()
	b.timer.Reset(b.d / 4)
	return b
}

func (b *stalledBody) check() {
	if b.done.Load() {
		return
	}
	if time.Since(time.Unix(0, b.last.Load())) >= b.d {
		b.stalled.Store(true)
		b.cancel(errStalled)
		return
	}
	b.timer.Reset(b.d / 4)
}

func (b *stalledBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	if n > 0 {
		b.last.Store(time.Now().UnixNano())
	}
	if err != nil && b.stalled.Load() {
		return n, errStalled
	}
	return n, err
}

func (b *stalledBody) Close() error {
	b.done.Store(true)
	b.timer.Stop()
	b.cancel(nil)
	return b.body.Close()
}

func (s *Source) logf(format string, args ...any) {
	if s.Log != nil {
		s.Log(fmt.Sprintf(format, args...))
	}
}

func orDuration(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Compare compares versions such as 1.2.3 and 1.3.0-beta.1 as MyGo's
// updater does (semantic versioning: numbers numerically, a pre-release
// before its release, build metadata ignored).
func Compare(a, b string) int {
	a, b = strings.TrimPrefix(a, "v"), strings.TrimPrefix(b, "v")
	a, _, _ = strings.Cut(a, "+")
	b, _, _ = strings.Cut(b, "+")
	ar, apre, _ := strings.Cut(a, "-")
	br, bpre, _ := strings.Cut(b, "-")
	if c := compareFields(ar, br); c != 0 {
		return c
	}
	switch {
	case apre == bpre:
		return 0
	case apre == "":
		return 1
	case bpre == "":
		return -1
	}
	return compareFields(apre, bpre)
}

// IsPrerelease reports whether a version such as 1.3.0-beta.1 is a
// prerelease (the beta channel's).
func IsPrerelease(version string) bool {
	version, _, _ = strings.Cut(strings.TrimPrefix(version, "v"), "+")
	_, pre, _ := strings.Cut(version, "-")
	return pre != ""
}

func compareFields(a, b string) int {
	af, bf := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(af) || i < len(bf); i++ {
		var x, y string
		if i < len(af) {
			x = af[i]
		}
		if i < len(bf) {
			y = bf[i]
		}
		xn, xerr := strconv.Atoi(x)
		yn, yerr := strconv.Atoi(y)
		switch {
		case x == y:
			continue
		case x == "":
			return -1
		case y == "":
			return 1
		case xerr == nil && yerr == nil:
			if xn < yn {
				return -1
			}
			if xn > yn {
				return 1
			}
		case xerr == nil:
			return -1 // numbers sort before words
		case yerr == nil:
			return 1
		default:
			return strings.Compare(x, y)
		}
	}
	return 0
}
