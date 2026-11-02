package redact

import (
	"strings"
	"testing"
)

// URLs that net/url rejects used to send URL and Text into each other
// until the stack overflowed (a fatal error no recover catches).
var malformed = map[string]string{
	"https://evil.example/%zz":                          "",
	"socks5://me:p%ss@127.0.0.1:7897":                   "p%ss",
	"http://bob:pa%55%@10.0.0.1:3128/x?token=abc":       "pa%55%",
	"http://tok%en@proxy:8080":                          "tok%en",
	"https://x.example/%zz?key=AIzaSecret&q=1":          "AIzaSecret",
	"https://x.example/%zz?access_token=zzz&sig=s1g":    "zzz",
	"https://[::1/x?api_key=k3y":                        "k3y",
	"http://a b@c":                                      "",
	"socks5://u:p@[bad:7897%":                           "u:p@",
	"redirect to https://evil.example/%zz?pwd=hunter2.": "hunter2",
}

func TestMalformedURL(t *testing.T) {
	for in, secret := range malformed {
		for name, f := range map[string]func(string) string{"URL": URL, "Text": Text} {
			got := f(in)
			if secret != "" && strings.Contains(got, secret) {
				t.Errorf("%s(%q) = %q, leaks %q", name, in, got, secret)
			}
		}
	}
	if got, want := URL("socks5://me:p%ss@127.0.0.1:7897"), "socks5://me:REDACTED@127.0.0.1:7897"; got != want {
		t.Errorf("URL: got %q want %q", got, want)
	}
	if got, want := URL("https://evil.example/%zz"), "https://evil.example/%zz"; got != want {
		t.Errorf("URL: got %q want %q", got, want)
	}
}

func FuzzText(f *testing.F) {
	for in := range malformed {
		f.Add(in)
	}
	f.Add("socks5://user:hunter2@127.0.0.1:7897 https://x/?key=AIzaSyA1234567890abcdefghijklmnopqrs")
	f.Add("Authorization: Bearer sk-abcdefghijklmnopqrstuvwx")
	f.Fuzz(func(t *testing.T, s string) {
		Text(s)
		URL(s)
		Error(errString(s))
	})
}

type errString string

func (e errString) Error() string { return string(e) }

// TestMalformedRegression: the v0.18 rules still apply around a URL that
// does not parse, and inputs that used to overflow the stack end.
func TestMalformedRegression(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://evil.example/%zz", "https://evil.example/%zz"},
		{"GET https://evil.example/%zz?token=abc&q=1 failed", "GET https://evil.example/%zz?token=REDACTED&q=1 failed"},
		{"proxy socks5://me:p%ss@127.0.0.1:7897.", "proxy socks5://me:REDACTED@127.0.0.1:7897."},
		{"http://tok%en@proxy:8080", "http://REDACTED@proxy:8080"},
		{"https://x/%zz Authorization: Bearer abcdefgh12345678", "https://x/%zz Authorization: Bearer REDACTED"},
		{`{"url":"https://x/%ZZ","api_key":"s3cret"}`, `{"url":"https://x/%ZZ","api_key":"REDACTED"}`},
		{"https://x/%zz?key=AIzaSyA1234567890abcdefghijklmnopqrs", "https://x/%zz?key=REDACTED"},
		{"https://x/%%%?code=123456#frag", "https://x/%%%?code=REDACTED"}, // the value runs to the line's end (v0.18 rule),
		{"ftp://[::1%zz]/hf_abcdefghijklmnopqrstuvwx", "ftp://[::1%zz]/REDACTED"},
	}
	for _, c := range cases {
		if got := Text(c.in); got != c.want {
			t.Errorf("Text(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// A bad escape in every URL of a long line, and megabyte inputs.
	line := strings.Repeat("see https://evil.example/%zz?token=t0k3n ", 2000)
	if got := Text(line); strings.Contains(got, "t0k3n") || !strings.Contains(got, "%zz") {
		t.Errorf("long line: leak or lost text (%d bytes)", len(got))
	}
	big := []string{
		"https://evil.example/" + strings.Repeat("%zz", 1<<18),
		"https://" + strings.Repeat("a%", 1<<19) + "@host/?pwd=hunter2",
		strings.Repeat("%", 1<<20),
		"socks5://" + strings.Repeat("u:p%ss@", 1<<16) + "host",
	}
	for i, in := range big {
		got := Text(in)
		if strings.Contains(got, "hunter2") {
			t.Errorf("big[%d]: leaks the password", i)
		}
		URL(in)
		Error(errString(in))
	}
}
