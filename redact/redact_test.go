package redact

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://translation.googleapis.com/language/translate/v2?key=AIzaSyA1234567890abcdefghijklmnopqrs&q=hi":                  "https://translation.googleapis.com/language/translate/v2?key=REDACTED&q=hi",
		"https://bucket.r2.cloudflarestorage.com/a.dmg?X-Amz-Algorithm=AWS4&X-Amz-Credential=abc%2F2026&X-Amz-Signature=deadbeef": "https://bucket.r2.cloudflarestorage.com/a.dmg?X-Amz-Algorithm=AWS4&X-Amz-Credential=REDACTED&X-Amz-Signature=REDACTED",
		"socks5://user:hunter2@127.0.0.1:7897": "socks5://user:REDACTED@127.0.0.1:7897",
		"http://tok@proxy:8080":                "http://REDACTED@proxy:8080",
		"https://huggingface.co/api/models/x":  "https://huggingface.co/api/models/x",
	} {
		if got := URL(in); got != want {
			t.Errorf("URL(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestText(t *testing.T) {
	e := &url.Error{Op: "Post", URL: "https://translation.googleapis.com/v2?key=AIzaSyA1234567890abcdefghijklmnopqrs", Err: errors.New("proxyconnect tcp: dial tcp 127.0.0.1:7897: connection refused")}
	got := Text(e.Error())
	if strings.Contains(got, "AIza") || !strings.Contains(got, "key=REDACTED") || !strings.Contains(got, "connection refused") {
		t.Errorf("url.Error: %q", got)
	}
	for in, bad := range map[string]string{
		"--proxy http://bob:pa55@10.0.0.1:3128 -- https://youtu.be/x": "pa55",
		"Authorization: Bearer sk-abcdefghijklmnopqrstuvwx":           "sk-abc",
		`{"api_key":"x", "token=abcdef123456"}`:                       "abcdef123456",
		"using hf_abcdefghijklmnopqrstuvwxyz0123 for gated":           "hf_abc",
		"x-goog-api-key: AIzaSyA1234567890abcdefghijklmnopqrs":        "AIza",
		"see https://example.com/x?access_token=zzz.":                 "zzz",
	} {
		if got := Text(in); strings.Contains(got, bad) {
			t.Errorf("Text(%q) = %q", in, got)
		}
	}
	plain := "loaded mlx-community/Qwen3-ASR-0.6B-4bit in 2.1 s; key frames 12"
	if got := Text(plain); got != plain {
		t.Errorf("plain text changed: %q", got)
	}
}

// TestTextExact pins Text's output: the credential goes, the words around
// it (header names, scheme words, field names) stay.
func TestTextExact(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		// The reported case: the scheme word was masked and the token kept.
		{"Authorization: Bearer abcdefghijklmnop", "Authorization: Bearer REDACTED"},
		{"Authorization: Bearer abc", "Authorization: Bearer REDACTED"},
		{"authorization: bearer sk-abcdefghijklmnopqrstuvwx", "authorization: bearer REDACTED"},
		{"Authorization: Basic dXNlcjpwYXNzd29yZA==", "Authorization: Basic REDACTED"},
		{"Authorization: Token 0123456789abcdef", "Authorization: Token REDACTED"},
		{"Authorization: 0123456789abcdef", "Authorization: REDACTED"},
		{"Authorization=Bearer abcdefghijklmnop", "Authorization=Bearer REDACTED"},
		{"Proxy-Authorization: Basic dXNlcjpwYXNz", "Proxy-Authorization: Basic REDACTED"},
		{"proxy-authorization: rawtoken123", "proxy-authorization: REDACTED"},
		{"header Authorization: Bearer abcdefghijklmnop, retrying", "header Authorization: Bearer REDACTED, retrying"},
		{"Bearer abcdefghijklmnop", "Bearer REDACTED"},
		{"x-api-key: abcdef0123456789", "x-api-key: REDACTED"},
		{"X-Api-Key=abcdef0123456789", "X-Api-Key=REDACTED"},
		{"api-key: abcdef0123456789", "api-key: REDACTED"},
		{"xi-api-key: abcdef0123456789", "xi-api-key: REDACTED"},
		{"Cookie: session=abc123; csrf=def456", "Cookie: REDACTED"},
		{"Set-Cookie: sid=abc123; Path=/; HttpOnly\nnext line", "Set-Cookie: REDACTED\nnext line"},
		{"token=abcdef123456 next", "token=REDACTED next"},
		{"access_token=abc.def&expires=3600", "access_token=REDACTED&expires=3600"},
		{"/v1/models?key=abcdef123456&alt=json", "/v1/models?key=REDACTED&alt=json"},
		{"form: grant_type=x&code=abc123&refresh_token=r1", "form: grant_type=x&code=REDACTED&refresh_token=REDACTED"},
		{`{"api_key":"abc 123","model":"m"}`, `{"api_key":"REDACTED","model":"m"}`},
		{`{"access_token": "eyJ.x", "refresh_token" : "r\"tok", "expires_in": 3600}`, `{"access_token": "REDACTED", "refresh_token" : "REDACTED", "expires_in": 3600}`},
		{`{"Authorization":"Bearer abcdefghijklmnop"}`, `{"Authorization":"REDACTED"}`},
		{`{"apiKey":"abc","clientSecret":"def"}`, `{"apiKey":"REDACTED","clientSecret":"REDACTED"}`},
		{"key sk-proj-abcdefghijklmnopqrstuvwx in env", "key REDACTED in env"},
		{"sk-ant-api03-abcdefghijklmnopqrstuvwx", "REDACTED"},
		{"ghp_abcdefghijklmnopqrstuvwxyz0123 gho_abcdefghijklmnopqrstuvwxyz0123", "REDACTED REDACTED"},
		{"github_pat_11ABCDEFG0123456789_abcdefghij", "REDACTED"},
		{"id eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiIxMjM0NSJ9.c2lnbmF0dXJl end", "id REDACTED end"},
		// Prose stays.
		{"the auth code is wrong; key frames 12; a token of thanks", "the auth code is wrong; key frames 12; a token of thanks"},
		{"Authorization failed", "Authorization failed"},
		{"cookies are off", "cookies are off"},
	} {
		if got := Text(c.in); got != c.want {
			t.Errorf("Text(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestError(t *testing.T) {
	base := errors.New("boom")
	err := Error(&url.Error{Op: "Get", URL: "https://x/?token=abc", Err: base})
	if strings.Contains(err.Error(), "abc") || !errors.Is(err, base) {
		t.Errorf("%v", err)
	}
	if Error(nil) != nil || Error(base) != base {
		t.Error("passthrough")
	}
}
