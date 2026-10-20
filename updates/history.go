package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Entry is one release in the changelog history (GET /api/changelog/<app>).
type Entry struct {
	Version string `json:"version"`
	Channel string `json:"channel"`
	// Date: YYYY-MM-DD.
	Date string `json:"date"`
	// Notes in Markdown, in Locale: the asked language, or English where a
	// release has no translation.
	Notes  string `json:"notes"`
	Locale string `json:"locale"`
	// RequiresPaidLicence: paid_since is set and this version is at or above
	// it (early-access codes do not work with it).
	RequiresPaidLicence bool `json:"requiresPaidLicence"`
}

// History is the changelog of an app in one language.
type History struct {
	App       string  `json:"app"`
	Locale    string  `json:"locale"`
	PaidSince *string `json:"paidSince"`
	Entries   []Entry `json:"entries"`
}

// PaidSinceVersion is the site's paid_since, "" when not set.
func (h *History) PaidSinceVersion() string {
	if h == nil || h.PaidSince == nil {
		return ""
	}
	return *h.PaidSince
}

// Find returns the entry of a version, or nil.
func (h *History) Find(version string) *Entry {
	if h == nil {
		return nil
	}
	for i := range h.Entries {
		if h.Entries[i].Version == strings.TrimPrefix(version, "v") {
			return &h.Entries[i]
		}
	}
	return nil
}

// NotesLocale maps an app's language to a changelog language: zh, zh-CN,
// zh-Hans… → "zh-Hans"; everything else → "en" (the only two for now).
func NotesLocale(language string) string {
	l := strings.ToLower(strings.TrimSpace(language))
	if strings.HasPrefix(l, "zh") {
		return "zh-Hans"
	}
	return "en"
}

// HistoryClient reads the changelog API of actnow.dev.
type HistoryClient struct {
	// BaseURL: https://actnow.dev (http only on loopback, for tests).
	BaseURL string
	App     string
	// HTTPClient: one with a 20 s timeout.
	HTTPClient *http.Client
}

// Fetch gets the history in a language (NotesLocale of the app's
// language), stable releases or, with beta, betas too; limit 0 = 50.
func (c *HistoryClient) Fetch(ctx context.Context, language string, beta bool, limit int) (*History, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://actnow.dev"
	}
	u, err := url.Parse(base)
	if err != nil || !(u.Scheme == "https" || (u.Scheme == "http" && loopback(u.Hostname()))) {
		return nil, fmt.Errorf("updates: %s: the changelog needs https", base)
	}
	q := url.Values{"locale": {NotesLocale(language)}}
	if beta {
		q.Set("channel", "beta")
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprint(limit))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/changelog/"+url.PathEscape(c.App)+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("updates: changelog: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("updates: changelog: %s", resp.Status)
	}
	var h History
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&h); err != nil {
		return nil, fmt.Errorf("updates: changelog: %w", err)
	}
	if h.App != c.App {
		return nil, errors.New("updates: changelog of another app")
	}
	return &h, nil
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
