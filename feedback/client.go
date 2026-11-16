package feedback

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

// APIError is an error answer of the feedback API.
type APIError struct {
	HTTPStatus int
	Code       string
	Message    string
	Details    json.RawMessage
}

func (e *APIError) Error() string {
	return fmt.Sprintf("feedback: %s (%d): %s", e.Code, e.HTTPStatus, e.Message)
}

// Retryable: the same request may succeed later (rate limits, a site
// switched off for a while, expired upload URLs).
func (e *APIError) Retryable() bool {
	switch e.Code {
	case "rate_limited", "disabled", "upload_expired", "upload_missing":
		return true
	}
	return e.HTTPStatus == http.StatusRequestTimeout || e.HTTPStatus == http.StatusTooManyRequests
}

// ErrNetwork wraps failures to reach the site (offline, timeouts, 5xx).
var ErrNetwork = errors.New("feedback: cannot reach the site")

// Client speaks the feedback protocol (see the package doc).
type Client struct {
	// BaseURL: "https://actnow.dev" or "https://refto.one" (http only on
	// loopback, for tests).
	BaseURL string
	App     string
	Version string
	HTTP    *http.Client
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

// checkBase refuses a base URL that is not https (or loopback http).
func checkBase(base string) error {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return fmt.Errorf("feedback: bad base URL %q", base)
	}
	if u.Scheme == "https" {
		return nil
	}
	h := u.Hostname()
	if u.Scheme == "http" && (h == "localhost" || h == "127.0.0.1" || h == "::1") {
		return nil
	}
	return fmt.Errorf("feedback: %s: the feedback site needs https", base)
}

// readOnly endpoints change nothing on the site: a request that failed on
// the connection may be sent again.
var readOnly = map[string]bool{"identity": true, "uploads": true, "list": true, "get": true}

func (c *Client) post(ctx context.Context, endpoint string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		err := c.postOnce(ctx, endpoint, b, out)
		// Go's client retries a GET on a reused connection the server closed,
		// never a POST: a read-only call gets one more try of its own (an
		// idle keep-alive connection a proxy or the site dropped: EOF,
		// connection reset).
		if err != nil && attempt == 0 && readOnly[endpoint] && ctx.Err() == nil && connectionLost(err) {
			continue
		}
		return err
	}
}

func (c *Client) postOnce(ctx context.Context, endpoint string, b []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/api/feedback/v1/"+endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("user-agent", "actnowkit-feedback "+c.App+"/"+c.Version)
	resp, err := c.http().Do(req)
	if err != nil {
		return &netError{err}
	}
	defer resp.Body.Close()
	return decodeAnswer(resp, out)
}

// netError is ErrNetwork with the transport's error kept for connectionLost.
type netError struct{ err error }

func (e *netError) Error() string   { return fmt.Sprintf("%v: %v", ErrNetwork, e.err) }
func (e *netError) Is(t error) bool { return t == ErrNetwork }
func (e *netError) Unwrap() error   { return e.err }

// connectionLost: the connection ended under the request (EOF, reset,
// broken pipe, a closed idle connection), not a refusal, a timeout or an
// answer.
func connectionLost(err error) bool {
	var ne *netError
	if !errors.As(err, &ne) {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNABORTED) || errors.Is(err, net.ErrClosed) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "server closed idle connection") || strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") || strings.HasSuffix(msg, "EOF")
}

func decodeAnswer(resp *http.Response, out any) error {
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("%w: %s", ErrNetwork, resp.Status)
	}
	if resp.StatusCode >= 400 {
		var e struct {
			Error   string          `json:"error"`
			Message string          `json:"message"`
			Details json.RawMessage `json:"details"`
		}
		if json.Unmarshal(data, &e) != nil || e.Error == "" {
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout {
				return &APIError{HTTPStatus: resp.StatusCode, Code: "rate_limited", Message: resp.Status}
			}
			return fmt.Errorf("%w: %s", ErrNetwork, resp.Status)
		}
		return &APIError{HTTPStatus: resp.StatusCode, Code: e.Error, Message: e.Message, Details: e.Details}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%w: bad answer: %v", ErrNetwork, err)
	}
	return nil
}

// Identity asks whether the site can answer the holder of licenceToken
// ("" for none) without an email.
func (c *Client) Identity(ctx context.Context, licenceToken string) (Identity, error) {
	var out Identity
	err := c.post(ctx, "identity", map[string]string{"app": c.App, "licenceToken": licenceToken}, &out)
	if err == nil {
		out.Known = true
	}
	return out, err
}

// AttachmentMeta is a file in the create request.
type AttachmentMeta struct {
	Ref    string `json:"ref"`
	Role   Role   `json:"role"`
	Name   string `json:"name"`
	MIME   string `json:"mime"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// CreateRequest is the body of create.
type CreateRequest struct {
	App          string           `json:"app"`
	Version      string           `json:"version"`
	Build        string           `json:"build,omitempty"`
	Channel      string           `json:"channel,omitempty"`
	Locale       string           `json:"locale,omitempty"`
	Kind         Kind             `json:"kind"`
	Body         string           `json:"body"`
	Email        string           `json:"email,omitempty"`
	LicenceToken string           `json:"licenceToken,omitempty"`
	DeviceID     string           `json:"deviceId,omitempty"`
	Source       string           `json:"source"` // "app"
	System       System           `json:"system"`
	Context      *Context         `json:"context,omitempty"`
	Attachments  []AttachmentMeta `json:"attachments"`
}

// Upload is where one file goes.
type Upload struct {
	Ref          string            `json:"ref"`
	AttachmentID string            `json:"attachmentId"`
	Method       string            `json:"method"`
	URL          string            `json:"url"`
	Headers      map[string]string `json:"headers"`
	ExpiresAt    time.Time         `json:"expiresAt"`
	// FallbackURL takes the file through the site when URL cannot be
	// reached (Authorization: Feedback <viewToken>).
	FallbackURL string `json:"fallbackUrl"`
}

// CreateAnswer is the answer of create.
type CreateAnswer struct {
	ID        string   `json:"id"`
	Number    string   `json:"number"`
	ViewToken string   `json:"viewToken"`
	WebURL    string   `json:"webUrl"`
	Uploads   []Upload `json:"uploads"`
}

// Create files the feedback; its files are uploaded next.
func (c *Client) Create(ctx context.Context, r CreateRequest) (CreateAnswer, error) {
	var out CreateAnswer
	r.Source = "app"
	if r.Attachments == nil {
		r.Attachments = []AttachmentMeta{}
	}
	err := c.post(ctx, "create", r, &out)
	if err == nil && (out.ID == "" || out.ViewToken == "") {
		err = fmt.Errorf("%w: bad answer: no id", ErrNetwork)
	}
	return out, err
}

// Uploads asks for new URLs for the files not yet stored.
func (c *Client) Uploads(ctx context.Context, id, viewToken string) ([]Upload, error) {
	var out struct {
		Uploads []Upload `json:"uploads"`
	}
	err := c.post(ctx, "uploads", map[string]string{"id": id, "viewToken": viewToken}, &out)
	return out.Uploads, err
}

// progressReader counts what was read.
type progressReader struct {
	r    io.Reader
	done func(n int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 && p.done != nil {
		p.done(int64(n))
	}
	return n, err
}

// Put uploads the file at path to u (direct, then the fallback through
// the site on a network failure). progress gets the bytes sent (it may go
// back to 0 when the fallback starts again).
func (c *Client) Put(ctx context.Context, u Upload, viewToken, path string, f File, progress func(sent int64)) error {
	fallback := func() error {
		h := map[string]string{"content-type": f.MIME, "authorization": "Feedback " + viewToken, "x-feedback-sha256": f.SHA256}
		return c.put(ctx, http.MethodPut, u.FallbackURL, h, path, f, progress)
	}
	// No direct URL (the site has no presign credentials): through the site.
	if u.FallbackURL != "" && (u.URL == "" || u.URL == u.FallbackURL) {
		return fallback()
	}
	err := c.put(ctx, u.Method, u.URL, u.Headers, path, f, progress)
	if err == nil || u.FallbackURL == "" || !errors.Is(err, ErrNetwork) {
		return err
	}
	if progress != nil {
		progress(0)
	}
	return fallback()
}

func (c *Client) put(ctx context.Context, method, target string, headers map[string]string, path string, f File, progress func(int64)) error {
	if method == "" {
		method = http.MethodPut
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	var sent int64
	body := &progressReader{r: file, done: func(n int64) {
		sent += n
		if progress != nil {
			progress(sent)
		}
	}}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return err
	}
	req.ContentLength = f.Bytes
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", f.MIME)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest {
		// The storage refuses an expired or mismatched signature (S3 XML).
		io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		if strings.Contains(target, "X-Amz-Signature") {
			return &APIError{HTTPStatus: resp.StatusCode, Code: "upload_expired", Message: resp.Status}
		}
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return nil
	}
	return decodeAnswer(resp, nil)
}

// ChecksumHeader is the S3 header carrying a file's SHA-256 (base64), as
// the site signs it into the upload URL.
func ChecksumHeader(hexSum string) string {
	b, err := hex.DecodeString(hexSum)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

// CompleteAnswer is the answer of complete.
type CompleteAnswer struct {
	ID     string `json:"id"`
	Number string `json:"number"`
	Status Status `json:"status"`
	WebURL string `json:"webUrl"`
}

// Complete tells the site every file is uploaded.
func (c *Client) Complete(ctx context.Context, id, viewToken string) (CompleteAnswer, error) {
	var out CompleteAnswer
	err := c.post(ctx, "complete", map[string]string{"id": id, "viewToken": viewToken}, &out)
	return out, err
}

// Ref names a feedback for list, get and reply.
type Ref struct {
	ID        string `json:"id"`
	ViewToken string `json:"viewToken"`
}

// List returns the items of refs the site still has.
func (c *Client) List(ctx context.Context, refs []Ref) ([]Item, error) {
	var out struct {
		Items []Item `json:"items"`
	}
	if refs == nil {
		refs = []Ref{}
	}
	err := c.post(ctx, "list", map[string]any{"items": refs}, &out)
	return out.Items, err
}

// Get returns one feedback with its messages.
func (c *Client) Get(ctx context.Context, r Ref) (Thread, error) {
	var out Thread
	err := c.post(ctx, "get", r, &out)
	return out, err
}

// Reply adds the user's message to a feedback.
func (c *Client) Reply(ctx context.Context, r Ref, body string) (Message, error) {
	var out Message
	err := c.post(ctx, "reply", map[string]string{"id": r.ID, "viewToken": r.ViewToken, "body": body}, &out)
	return out, err
}
