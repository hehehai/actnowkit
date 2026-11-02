// Package redact removes secrets from text an app logs, shows or exports
// (spec §13, v1.0.9): API keys and tokens in URL queries, signatures of
// presigned URLs, passwords in proxy and other URLs, and bearer tokens.
// Apps pass every log line, error detail and diagnostics file through
// Text (or wrap errors with Error) before writing it.
//
//	log.Print(redact.Text(err.Error()))
//	u := redact.URL("https://x.googleapis.com/v2?key=AIza…&q=hi") // …?key=REDACTED&q=hi
package redact

import (
	"net/url"
	"regexp"
	"strings"
)

// Mask replaces a secret.
const Mask = "REDACTED"

// SecretParams are the query parameters (lower case) whose values are
// secrets. Matching ignores case.
var SecretParams = []string{
	"key", "api_key", "apikey", "api-key", "token", "access_token", "refresh_token", "id_token",
	"auth", "authorization", "password", "passwd", "pwd", "secret", "client_secret",
	"sig", "signature", "x-amz-signature", "x-amz-credential", "x-amz-security-token",
	"x-goog-signature", "x-goog-credential", "pin", "code",
	"accesstoken", "refreshtoken", "idtoken", "clientsecret", "session_token", "sessiontoken",
}

func secretParam(name string) bool {
	n := strings.ToLower(name)
	for _, p := range SecretParams {
		if n == p {
			return true
		}
	}
	return false
}

// URL redacts one URL: the password of its user info (user:pass@ →
// user:REDACTED@; a lone user, which proxies use as a token, too) and the
// values of SecretParams in its query. A URL that net/url rejects (a bad
// escape such as %zz or p%ss, a broken host) is redacted as raw text: its
// user info and secret query values by position, then the rules Text uses
// outside URLs. Text without a scheme goes through Text.
func URL(raw string) string {
	u, err := url.Parse(raw)
	if err == nil && u.Scheme == "" {
		return Text(raw) // Text calls redactURL, never URL: no loop
	}
	return redactURL(raw)
}

// redactURL redacts one URL found by urlRE. It never calls Text or URL,
// so Text ↔ URL cannot recurse.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return rawURL(raw)
	}
	changed := false
	if u.User != nil {
		if _, has := u.User.Password(); has {
			u.User = url.UserPassword(u.User.Username(), Mask)
		} else {
			u.User = url.User(Mask)
		}
		changed = true
	}
	if u.RawQuery != "" {
		if q, ok := redactQuery(u.RawQuery); ok {
			u.RawQuery, changed = q, true
		}
	}
	if !changed {
		return raw
	}
	return strings.Replace(u.String(), url.QueryEscape(Mask), Mask, -1)
}

// rawURL redacts a URL that does not parse, without parsing it: the user
// info before the last @ of the authority (as net/url splits it), the
// secret values of the query (after the first ?, before #), then the
// non-URL rules.
func rawURL(raw string) string {
	s := raw
	if i := strings.Index(s, "://"); i >= 0 {
		start := i + 3
		end := len(s)
		if j := strings.IndexAny(s[start:], "/?#"); j >= 0 {
			end = start + j
		}
		if at := strings.LastIndexByte(s[start:end], '@'); at >= 0 {
			user := s[start : start+at]
			masked := Mask
			if name, _, has := strings.Cut(user, ":"); has {
				masked = name + ":" + Mask
			}
			s = s[:start] + masked + s[start+at:]
		}
	}
	if i := strings.IndexByte(s, '?'); i >= 0 {
		end := len(s)
		if j := strings.IndexByte(s[i+1:], '#'); j >= 0 {
			end = i + 1 + j
		}
		if q, ok := redactQuery(s[i+1 : end]); ok {
			s = s[:i+1] + q + s[end:]
		}
	}
	return nonURL(s)
}

// redactQuery masks secret values in a raw query, keeping its order.
func redactQuery(q string) (string, bool) {
	parts := strings.Split(q, "&")
	changed := false
	for i, p := range parts {
		name, _, has := strings.Cut(p, "=")
		if dn, err := url.QueryUnescape(name); err == nil {
			name = dn
		}
		if has && secretParam(name) {
			parts[i] = p[:strings.IndexByte(p, '=')+1] + Mask
			changed = true
		}
	}
	return strings.Join(parts, "&"), changed
}

var (
	urlRE   = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s"'<>]+`)
	paramRE = regexp.MustCompile(`(?i)([?&;\s"]|^)(` + paramAlt() + `)=([^&\s"';]+)`)
	// The words too common in prose for paramRE count as parameters only in
	// a query (after ? or &).
	queryWordRE = regexp.MustCompile(`(?i)([?&])(key|auth|code|pin)=([^&\s"';]+)`)
	// JSON fields whose string values are secrets: the whole value goes.
	jsonRE   = regexp.MustCompile(`(?i)("(?:` + paramAlt() + `|auth|authorization|x-api-key|x-goog-api-key|xi-api-key|proxy-authorization|cookie|set-cookie)"\s*:\s*")(?:[^"\\]|\\.)*"`)
	bearerRE = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`)
	// Header-like name: value or name=value. An Authorization value keeps
	// its scheme word (Bearer, Basic, Token, …) and loses its credentials;
	// a raw value goes whole.
	authRE = regexp.MustCompile(`(?i)\b((?:proxy-)?authorization)(\s*[:=]\s*)("?)(?:(bearer|basic|token|digest|negotiate|apikey|key)\s+)?[^\s",]+`)
	// Cookie values have spaces after their semicolons: up to the line's end.
	cookieRE = regexp.MustCompile(`(?i)\b((?:set-)?cookie)(\s*:\s*)("?)[^\r\n"]+`)
	headerRE = regexp.MustCompile(`(?i)\b(x-api-key|x-goog-api-key|api-key|xi-api-key)(\s*[:=]\s*)("?)[^\s",]+`)
	// Well-known key shapes, wherever they appear.
	keyRE = regexp.MustCompile(`\b(AIza[0-9A-Za-z_-]{30,}|sk-[A-Za-z0-9_-]{16,}|[sr]k_(?:live|test)_[A-Za-z0-9]{16,}|hf_[A-Za-z0-9]{20,}|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|glpat-[A-Za-z0-9_-]{20,}|xox[abpr]-[A-Za-z0-9-]{10,}|eyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+)`)
)

// paramAlt is SecretParams as a regexp alternation, without the words
// too common in prose (handled inside URLs and queries) and without
// authorization (authRE keeps its scheme word).
func paramAlt() string {
	var alt []string
	for _, p := range SecretParams {
		if p == "code" || p == "pin" || p == "auth" || p == "key" || p == "authorization" {
			continue
		}
		alt = append(alt, regexp.QuoteMeta(p))
	}
	return strings.Join(alt, "|")
}

// Text redacts every URL in s (URL), secret parameters outside URLs
// (token=…; key=, auth=, code=, pin= only after ? or &), secret JSON string
// fields ("api_key": "…"), Authorization and Proxy-Authorization headers
// (the scheme word stays: "Authorization: Bearer REDACTED"), API-key and
// Cookie headers, bearer tokens, and well-known key shapes (Google AIza…,
// OpenAI/Anthropic sk-…, Stripe sk_live_…, Hugging Face hf_…, GitHub gh*_…
// and github_pat_…, GitLab glpat-…, Slack xox*-…, JWTs).
func Text(s string) string {
	if s == "" {
		return s
	}
	s = urlRE.ReplaceAllStringFunc(s, func(m string) string {
		// Trailing punctuation is not part of the URL.
		trail := ""
		for len(m) > 0 && strings.ContainsRune(".,;:)]}", rune(m[len(m)-1])) {
			trail = m[len(m)-1:] + trail
			m = m[:len(m)-1]
		}
		return redactURL(m) + trail
	})
	return nonURL(s)
}

// nonURL applies the rules for secrets outside URLs. It calls nothing else
// in the package.
func nonURL(s string) string {
	s = jsonRE.ReplaceAllString(s, "${1}"+Mask+`"`)
	s = paramRE.ReplaceAllString(s, "${1}${2}="+Mask)
	s = queryWordRE.ReplaceAllString(s, "${1}${2}="+Mask)
	s = authRE.ReplaceAllStringFunc(s, func(m string) string {
		g := authRE.FindStringSubmatch(m)
		scheme := ""
		if g[4] != "" {
			scheme = g[4] + " "
		}
		return g[1] + g[2] + g[3] + scheme + Mask
	})
	s = cookieRE.ReplaceAllString(s, "${1}${2}${3}"+Mask)
	s = headerRE.ReplaceAllString(s, "${1}${2}${3}"+Mask)
	s = bearerRE.ReplaceAllString(s, "${1} "+Mask)
	s = keyRE.ReplaceAllString(s, Mask)
	return s
}

// Error wraps err so that its message is redacted (Text); errors.Is and
// errors.As still see the original.
func Error(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if r := Text(msg); r != msg {
		return &redacted{msg: r, err: err}
	}
	return err
}

type redacted struct {
	msg string
	err error
}

func (r *redacted) Error() string { return r.msg }
func (r *redacted) Unwrap() error { return r.err }
