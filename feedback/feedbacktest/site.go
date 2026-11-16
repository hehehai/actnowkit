// Package feedbacktest is the feedback protocol of actnow.dev and
// refto.one in memory, for the tests of apps and of feedbackui: a Site on
// an httptest server that takes feedback, its uploads and replies.
package feedbacktest

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

	"github.com/hehehai/actnowkit/feedback"
)

// Site is a feedback site. Its fields are its knobs; lock Mu to change
// them while a test runs requests.
type Site struct {
	Mu  sync.Mutex
	URL string // the base URL (Config.BaseURL)

	// Tokens: licence token → its owner's email (identity answers it
	// linked, create takes it as the email).
	Tokens map[string]string
	// FailCreate: the next n creates answer 500.
	FailCreate int
	// Reject: create answers 422 with this code ("" none).
	Reject string
	// Down: every request answers 503.
	Down bool
	// PutDelay holds each upload this long (a test sees the progress).
	PutDelay time.Duration
	// Now (time.Now).
	Now func() time.Time

	srv   *httptest.Server
	items map[string]*Item
	n     int
	calls []string
}

// Item is a feedback the site has.
type Item struct {
	feedback.Item
	Body      string
	ViewToken string
	Email     string
	Request   feedback.CreateRequest
	Files     map[string]*File // by attachment id
	Messages  []feedback.Message
	Completed bool
}

// File is an attachment, Data once uploaded.
type File struct {
	Meta feedback.AttachmentMeta
	ID   string
	Data []byte
}

// New starts a site; Close stops it.
func New() *Site {
	s := &Site{Tokens: map[string]string{}, Now: time.Now, items: map[string]*Item{}}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	s.URL = s.srv.URL
	return s
}

// Close stops the server.
func (s *Site) Close() { s.srv.Close() }

// Client is an HTTP client to the site (Config.HTTPClient).
func (s *Site) Client() *http.Client { return s.srv.Client() }

// Items are the feedback the site has, by id.
func (s *Site) Items() map[string]*Item {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	out := make(map[string]*Item, len(s.items))
	for k, v := range s.items {
		out[k] = v
	}
	return out
}

// Count is the number of requests whose "METHOD /path" starts with prefix
// ("POST /api/feedback/v1/create").
func (s *Site) Count(prefix string) int {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	n := 0
	for _, c := range s.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// SupportReplies adds support's message to the feedback id.
func (s *Site) SupportReplies(id, body string) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	it := s.items[id]
	if it == nil {
		return
	}
	it.Messages = append(it.Messages, feedback.Message{ID: fmt.Sprintf("m%d", len(it.Messages)+1), Author: "support", Body: body, CreatedAt: s.Now().UTC()})
	it.UpdatedAt = s.Now().UTC()
}

// SetStatus changes the feedback id's status.
func (s *Site) SetStatus(id string, st feedback.Status) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if it := s.items[id]; it != nil {
		it.Status = st
	}
}

func apiErr(w http.ResponseWriter, status int, code string) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": code, "message": code})
}

func (s *Site) serve(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/bucket/") {
		s.put(w, r)
		return
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.calls = append(s.calls, r.Method+" "+r.URL.Path)
	if s.Down {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	data, _ := io.ReadAll(r.Body)
	var body map[string]json.RawMessage
	json.Unmarshal(data, &body)
	str := func(k string) string {
		var v string
		json.Unmarshal(body[k], &v)
		return v
	}
	item := func() *Item {
		it := s.items[str("id")]
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
		if email := s.Tokens[str("licenceToken")]; email != "" {
			at := strings.IndexByte(email, '@')
			out(feedback.Identity{Known: true, Linked: true, EmailHint: email[:1] + "***" + email[at:]})
			return
		}
		out(feedback.Identity{Known: true})
	case "create":
		if s.FailCreate > 0 {
			s.FailCreate--
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		if s.Reject != "" {
			apiErr(w, http.StatusUnprocessableEntity, s.Reject)
			return
		}
		var req feedback.CreateRequest
		json.Unmarshal(data, &req)
		email := req.Email
		if email == "" {
			email = s.Tokens[req.LicenceToken]
		}
		if email == "" {
			apiErr(w, http.StatusUnprocessableEntity, "email_required")
			return
		}
		s.n++
		now := s.Now().UTC()
		it := &Item{Item: feedback.Item{ID: fmt.Sprintf("fb_%d", s.n), Number: fmt.Sprintf("FB-%d", 1041+s.n), App: req.App, Kind: req.Kind,
			Status: feedback.StatusNew, Summary: feedback.Summary(req.Body, 80), CreatedAt: now, UpdatedAt: now},
			Body: req.Body, ViewToken: fmt.Sprintf("vt_%d", s.n), Email: email, Request: req, Files: map[string]*File{}}
		it.WebURL = s.URL + "/feedback/" + it.ID
		for i, a := range req.Attachments {
			id := fmt.Sprintf("%s_a%d", it.ID, i)
			it.Files[id] = &File{Meta: a, ID: id}
		}
		s.items[it.ID] = it
		out(feedback.CreateAnswer{ID: it.ID, Number: it.Number, ViewToken: it.ViewToken, WebURL: it.WebURL, Uploads: s.uploads(it)})
	case "uploads":
		it := item()
		if it == nil {
			apiErr(w, http.StatusNotFound, "not_found")
			return
		}
		out(map[string]any{"uploads": s.uploads(it)})
	case "complete":
		it := item()
		if it == nil {
			apiErr(w, http.StatusNotFound, "not_found")
			return
		}
		for _, f := range it.Files {
			if f.Data == nil {
				apiErr(w, http.StatusConflict, "upload_missing")
				return
			}
		}
		it.Completed = true
		out(feedback.CompleteAnswer{ID: it.ID, Number: it.Number, Status: feedback.StatusNew, WebURL: it.WebURL})
	case "list":
		var req struct {
			Items []feedback.Ref `json:"items"`
		}
		json.Unmarshal(data, &req)
		items := []feedback.Item{}
		for _, ref := range req.Items {
			if it := s.items[ref.ID]; it != nil && it.ViewToken == ref.ViewToken && it.Completed {
				x := it.Item
				x.Messages = len(it.Messages)
				items = append(items, x)
			}
		}
		out(map[string]any{"items": items})
	case "get":
		it := item()
		if it == nil {
			apiErr(w, http.StatusNotFound, "not_found")
			return
		}
		x := it.Item
		x.Messages = len(it.Messages)
		out(feedback.Thread{Item: x, Body: it.Body, Messages: it.Messages})
	case "reply":
		it := item()
		if it == nil {
			apiErr(w, http.StatusNotFound, "not_found")
			return
		}
		m := feedback.Message{ID: fmt.Sprintf("m%d", len(it.Messages)+1), Author: "user", Body: str("body"), CreatedAt: s.Now().UTC()}
		it.Messages = append(it.Messages, m)
		out(m)
	default:
		http.NotFound(w, r)
	}
}

func (s *Site) uploads(it *Item) []feedback.Upload {
	var us []feedback.Upload
	for id, f := range it.Files {
		if f.Data != nil {
			continue
		}
		us = append(us, feedback.Upload{Ref: f.Meta.Ref, AttachmentID: id, Method: "PUT", URL: s.URL + "/bucket/" + id,
			Headers:   map[string]string{"content-type": f.Meta.MIME, "x-amz-checksum-sha256": feedback.ChecksumHeader(f.Meta.SHA256)},
			ExpiresAt: s.Now().Add(time.Hour), FallbackURL: s.URL + "/bucket/" + id})
	}
	return us
}

// put stores an upload (outside the lock while PutDelay holds it).
func (s *Site) put(w http.ResponseWriter, r *http.Request) {
	s.Mu.Lock()
	delay := s.PutDelay
	s.calls = append(s.calls, r.Method+" /bucket")
	s.Mu.Unlock()
	data, _ := io.ReadAll(r.Body)
	if delay > 0 {
		time.Sleep(delay)
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()
	id := strings.TrimPrefix(r.URL.Path, "/bucket/")
	var f *File
	for _, it := range s.items {
		if x := it.Files[id]; x != nil {
			f = x
		}
	}
	if f == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != f.Meta.Bytes || hex.EncodeToString(sum[:]) != f.Meta.SHA256 {
		apiErr(w, http.StatusBadRequest, "checksum_mismatch")
		return
	}
	f.Data = data
	w.WriteHeader(http.StatusOK)
}
