package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// droppingSite closes the connection without an answer for the first
// drop requests to each endpoint, then answers.
type droppingSite struct {
	mu    sync.Mutex
	drop  int
	calls map[string]int
}

func (d *droppingSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ep := strings.TrimPrefix(r.URL.Path, "/api/feedback/v1/")
	d.mu.Lock()
	d.calls[ep]++
	n := d.calls[ep]
	d.mu.Unlock()
	if n <= d.drop {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close() // EOF for the client: no status line
		}
		return
	}
	w.Header().Set("content-type", "application/json")
	switch ep {
	case "identity":
		json.NewEncoder(w).Encode(Identity{Linked: true, EmailHint: "o***@example.com"})
	case "list":
		json.NewEncoder(w).Encode(map[string]any{"items": []Item{{ID: "fb_1", Number: "FB-1001", Status: StatusNew}}})
	case "get":
		json.NewEncoder(w).Encode(Thread{Item: Item{ID: "fb_1"}, Body: "hello"})
	default:
		json.NewEncoder(w).Encode(map[string]any{})
	}
}

func (d *droppingSite) n(ep string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls[ep]
}

func TestReadOnlyRetriesALostConnection(t *testing.T) {
	site := &droppingSite{drop: 1, calls: map[string]int{}}
	srv := httptest.NewServer(site)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, App: "inboxly", Version: "1.0"}
	ctx := context.Background()

	who, err := c.Identity(ctx, "tok")
	if err != nil || !who.Linked || !who.Known {
		t.Fatalf("identity after one EOF: %+v %v", who, err)
	}
	if items, err := c.List(ctx, []Ref{{ID: "fb_1", ViewToken: "vt"}}); err != nil || len(items) != 1 {
		t.Fatalf("list after one EOF: %v %v", items, err)
	}
	if th, err := c.Get(ctx, Ref{ID: "fb_1", ViewToken: "vt"}); err != nil || th.Body != "hello" {
		t.Fatalf("get after one EOF: %+v %v", th, err)
	}
	for _, ep := range []string{"identity", "list", "get"} {
		if site.n(ep) != 2 {
			t.Errorf("%s: %d requests, want 2", ep, site.n(ep))
		}
	}

	// Only once: two lost connections in a row are a network error.
	site2 := &droppingSite{drop: 2, calls: map[string]int{}}
	srv2 := httptest.NewServer(site2)
	defer srv2.Close()
	c2 := &Client{BaseURL: srv2.URL, App: "inboxly", Version: "1.0"}
	if _, err := c2.Identity(ctx, "tok"); !errors.Is(err, ErrNetwork) || site2.n("identity") != 2 {
		t.Errorf("two drops: %v after %d requests", err, site2.n("identity"))
	}

	// Calls that change something are not sent twice.
	site3 := &droppingSite{drop: 1, calls: map[string]int{}}
	srv3 := httptest.NewServer(site3)
	defer srv3.Close()
	c3 := &Client{BaseURL: srv3.URL, App: "inboxly", Version: "1.0"}
	if _, err := c3.Create(ctx, CreateRequest{App: "inboxly", Kind: KindBug, Body: "hello there"}); !errors.Is(err, ErrNetwork) || site3.n("create") != 1 {
		t.Errorf("create: %v after %d requests", err, site3.n("create"))
	}
	if _, err := c3.Reply(ctx, Ref{ID: "fb_1", ViewToken: "vt"}, "x"); !errors.Is(err, ErrNetwork) || site3.n("reply") != 1 {
		t.Errorf("reply: %v after %d requests", err, site3.n("reply"))
	}

	// A refused connection (nothing listening) is not retried either.
	dead := httptest.NewServer(http.NotFoundHandler())
	url := dead.URL
	dead.Close()
	c4 := &Client{BaseURL: url, App: "inboxly", Version: "1.0"}
	if _, err := c4.Identity(ctx, "tok"); !errors.Is(err, ErrNetwork) || connectionLost(err) {
		t.Errorf("refused: %v (lost %v)", err, connectionLost(err))
	}
}

// The Manager's Identity, which the form uses, gets the answer after a
// dropped connection instead of falling back to "email required".
func TestManagerIdentityAfterDroppedConnection(t *testing.T) {
	site := &droppingSite{drop: 1, calls: map[string]int{}}
	srv := httptest.NewServer(site)
	defer srv.Close()
	m, err := New(Config{App: "inboxly", BaseURL: srv.URL, Dir: t.TempDir(), LicenceToken: func() string { return "tok" }})
	if err != nil {
		t.Fatal(err)
	}
	who, err := m.Identity(context.Background())
	if err != nil || !who.Linked {
		t.Fatalf("%+v %v", who, err)
	}
}
