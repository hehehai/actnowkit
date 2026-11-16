package feedback

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// fakeSite is the feedback protocol in memory, with a fake bucket for the
// direct uploads.
type fakeSite struct {
	mu     sync.Mutex
	srv    *httptest.Server
	now    func() time.Time
	items  map[string]*fakeItem
	n      int
	tokens map[string]string // licence token → owner email
	calls  []string
	// knobs
	directDown   bool          // the bucket cannot be reached (network error)
	expireFirst  int           // the first n direct PUTs answer 403 (expired)
	failCreate   int           // the first n creates answer 500
	rateLimited  bool          // create answers 429 rate_limited
	urlTTL       time.Duration // upload URL lifetime
	completeDrop string        // complete reports this ref missing once
}

type fakeItem struct {
	Item
	Body      string
	ViewToken string
	Email     string
	Req       CreateRequest
	Files     map[string]*fakeFile // by attachment id
	Messages  []Message
	Completed bool
}

type fakeFile struct {
	Meta AttachmentMeta
	ID   string
	Data []byte
}

func newFakeSite() *fakeSite {
	f := &fakeSite{items: map[string]*fakeItem{}, tokens: map[string]string{}, now: time.Now, urlTTL: time.Hour}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	return f
}

func (f *fakeSite) Close() { f.srv.Close() }

func apiErr(w http.ResponseWriter, status int, code string, details any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": code, "message": code, "details": details})
}

func (f *fakeSite) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	switch {
	case strings.HasPrefix(r.URL.Path, "/bucket/"):
		f.bucketPut(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/api/feedback/v1/upload/"):
		f.fallbackPut(w, r)
		return
	}
	var body map[string]json.RawMessage
	data, _ := io.ReadAll(r.Body)
	json.Unmarshal(data, &body)
	str := func(k string) string {
		var s string
		json.Unmarshal(body[k], &s)
		return s
	}
	item := func() *fakeItem {
		it := f.items[str("id")]
		if it == nil || it.ViewToken != str("viewToken") {
			return nil
		}
		return it
	}
	out := func(v any) {
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
	switch strings.TrimPrefix(r.URL.Path, "/api/feedback/v1/") {
	case "identity":
		email, ok := f.tokens[str("licenceToken")]
		if ok && email != "" {
			out(Identity{Linked: true, EmailHint: email[:1] + "***" + email[strings.IndexByte(email, '@'):]})
			return
		}
		out(Identity{})
	case "create":
		if f.failCreate > 0 {
			f.failCreate--
			http.Error(w, "boom", 500)
			return
		}
		if f.rateLimited {
			apiErr(w, 429, "rate_limited", map[string]int{"retryAfter": 60})
			return
		}
		var req CreateRequest
		json.Unmarshal(data, &req)
		email := req.Email
		if email == "" {
			email = f.tokens[req.LicenceToken]
		}
		if email == "" {
			apiErr(w, 422, "email_required", nil)
			return
		}
		for _, a := range req.Attachments {
			limit := map[Role]int64{RoleImage: MaxImageBytes, RoleVideo: MaxVideoBytes, RoleDiagnostics: MaxDiagnosticsBytes}[a.Role]
			if limit == 0 {
				apiErr(w, 422, "attachment_type", nil)
				return
			}
			if a.Bytes > limit {
				apiErr(w, 422, "attachment_too_large", nil)
				return
			}
		}
		f.n++
		it := &fakeItem{Item: Item{ID: fmt.Sprintf("fb_%d", f.n), Number: fmt.Sprintf("FB-%d", 1000+f.n), App: req.App, Kind: req.Kind,
			Status: StatusNew, Summary: Summary(req.Body, 80), CreatedAt: f.now().UTC(), UpdatedAt: f.now().UTC()},
			Body: req.Body, ViewToken: fmt.Sprintf("vt_%d", f.n), Email: email, Req: req, Files: map[string]*fakeFile{}}
		it.WebURL = f.srv.URL + "/feedback/" + it.ID
		for i, a := range req.Attachments {
			id := fmt.Sprintf("%s_a%d", it.ID, i)
			it.Files[id] = &fakeFile{Meta: a, ID: id}
		}
		f.items[it.ID] = it
		out(CreateAnswer{ID: it.ID, Number: it.Number, ViewToken: it.ViewToken, WebURL: it.WebURL, Uploads: f.uploads(it)})
	case "uploads":
		it := item()
		if it == nil {
			apiErr(w, 404, "not_found", nil)
			return
		}
		out(map[string]any{"uploads": f.uploads(it)})
	case "complete":
		it := item()
		if it == nil {
			apiErr(w, 404, "not_found", nil)
			return
		}
		var missing []string
		for id, ff := range it.Files {
			if ff.Data == nil {
				missing = append(missing, id)
			}
			if f.completeDrop != "" && ff.Meta.Ref == f.completeDrop {
				f.completeDrop = ""
				ff.Data = nil
				missing = append(missing, id)
			}
		}
		if len(missing) > 0 {
			apiErr(w, 409, "upload_missing", map[string]any{"missing": missing})
			return
		}
		it.Completed = true
		out(CompleteAnswer{ID: it.ID, Number: it.Number, Status: StatusNew, WebURL: it.WebURL})
	case "list":
		var req struct {
			Items []Ref `json:"items"`
		}
		json.Unmarshal(data, &req)
		var items []Item
		for _, ref := range req.Items {
			if it := f.items[ref.ID]; it != nil && it.ViewToken == ref.ViewToken && it.Completed {
				x := it.Item
				x.Messages = len(it.Messages)
				items = append(items, x)
			}
		}
		out(map[string]any{"items": items})
	case "get":
		it := item()
		if it == nil {
			apiErr(w, 404, "not_found", nil)
			return
		}
		x := it.Item
		x.Messages = len(it.Messages)
		out(Thread{Item: x, Body: it.Body, Messages: it.Messages})
	case "reply":
		it := item()
		if it == nil {
			apiErr(w, 404, "not_found", nil)
			return
		}
		m := Message{ID: fmt.Sprintf("m%d", len(it.Messages)+1), Author: "user", Body: str("body"), CreatedAt: f.now().UTC()}
		it.Messages = append(it.Messages, m)
		out(m)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeSite) uploads(it *fakeItem) []Upload {
	var us []Upload
	for id, ff := range it.Files {
		if ff.Data != nil {
			continue
		}
		direct := f.srv.URL + "/bucket/" + id + "?X-Amz-Signature=abc&exp=" + fmt.Sprint(f.now().Add(f.urlTTL).Unix())
		if f.directDown {
			direct = "http://127.0.0.1:1/bucket/" + id + "?X-Amz-Signature=abc"
		}
		us = append(us, Upload{Ref: ff.Meta.Ref, AttachmentID: id, Method: "PUT", URL: direct,
			Headers:   map[string]string{"content-type": ff.Meta.MIME, "x-amz-checksum-sha256": ChecksumHeader(ff.Meta.SHA256)},
			ExpiresAt: f.now().Add(f.urlTTL), FallbackURL: f.srv.URL + "/api/feedback/v1/upload/" + id})
	}
	return us
}

func (f *fakeSite) file(id string) (*fakeItem, *fakeFile) {
	for _, it := range f.items {
		if ff := it.Files[id]; ff != nil {
			return it, ff
		}
	}
	return nil, nil
}

func (f *fakeSite) store(w http.ResponseWriter, r *http.Request, ff *fakeFile, sum string) {
	data, _ := io.ReadAll(r.Body)
	if int64(len(data)) != ff.Meta.Bytes || r.ContentLength != ff.Meta.Bytes {
		apiErr(w, 400, "size_mismatch", nil)
		return
	}
	h := sha256.Sum256(data)
	if hex.EncodeToString(h[:]) != ff.Meta.SHA256 || (sum != "" && sum != ff.Meta.SHA256) {
		apiErr(w, 400, "checksum_mismatch", nil)
		return
	}
	ff.Data = data
	w.WriteHeader(200)
}

func (f *fakeSite) bucketPut(w http.ResponseWriter, r *http.Request) {
	if f.expireFirst > 0 {
		f.expireFirst--
		w.WriteHeader(403)
		io.WriteString(w, "<Error><Code>AccessDenied</Code><Message>Request has expired</Message></Error>")
		return
	}
	_, ff := f.file(strings.TrimPrefix(r.URL.Path, "/bucket/"))
	if ff == nil {
		w.WriteHeader(404)
		return
	}
	if r.Header.Get("x-amz-checksum-sha256") != ChecksumHeader(ff.Meta.SHA256) {
		w.WriteHeader(400)
		return
	}
	f.store(w, r, ff, "")
}

func (f *fakeSite) fallbackPut(w http.ResponseWriter, r *http.Request) {
	it, ff := f.file(strings.TrimPrefix(r.URL.Path, "/api/feedback/v1/upload/"))
	if ff == nil || r.Header.Get("authorization") != "Feedback "+it.ViewToken {
		apiErr(w, 404, "not_found", nil)
		return
	}
	f.store(w, r, ff, r.Header.Get("x-feedback-sha256"))
}

func (f *fakeSite) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}
