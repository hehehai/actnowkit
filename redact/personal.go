package redact

import (
	"net"
	"os"
	"os/user"
	"regexp"
	"sort"
	"strings"
)

// Personal removes what identifies the person from text an app sends
// away (feedback diagnostics, crash reports), after Text has removed the
// secrets: the home folder (→ "~"), other users' folders
// (/Users/<name> → /Users/USER), the user's and the Mac's names, email
// addresses (r***@gmail.com), and IP addresses (the last two parts of an
// IPv4 address, all of an IPv6 one but loopback). The zero value only
// masks emails, folders of /Users and addresses; ThisUser fills Home and
// Names for the running user.
type Personal struct {
	// Home is the user's home folder ("/Users/ann").
	Home string
	// Names are words to replace with "USER" wherever they stand alone:
	// the login and full name, the Mac's name. Names shorter than 3
	// characters are ignored (they would match too much), except
	// two-character names in other scripts ("张三").
	Names []string
	// KeepEmails are addresses left as they are (the one the user typed as
	// their contact, which the site sees anyway).
	KeepEmails []string
}

// ThisUser is Personal for the running user: their home, login and full
// name, and the host and computer names given.
func ThisUser(computerNames ...string) Personal {
	p := Personal{}
	if h, err := os.UserHomeDir(); err == nil {
		p.Home = h
	}
	if u, err := user.Current(); err == nil {
		p.Names = append(p.Names, u.Username, u.Name)
	}
	if h, err := os.Hostname(); err == nil {
		p.Names = append(p.Names, strings.TrimSuffix(h, ".local"))
	}
	p.Names = append(p.Names, computerNames...)
	return p
}

var (
	usersRE = regexp.MustCompile(`/Users/[^/\s"'<>:]+`)
	emailRE = regexp.MustCompile(`\b([A-Za-z0-9])[A-Za-z0-9._%+-]*@([A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,})\b`)
	ipv4RE  = regexp.MustCompile(`\b((?:25[0-5]|2[0-4]\d|1?\d?\d)\.(?:25[0-5]|2[0-4]\d|1?\d?\d))\.(?:25[0-5]|2[0-4]\d|1?\d?\d)\.(?:25[0-5]|2[0-4]\d|1?\d?\d)\b`)
	// IPv6 candidates, checked with net.ParseIP ("12:34:56" is not one).
	ipv6RE = regexp.MustCompile(`(?i)(?:[0-9a-f]{0,4}:){2,7}[0-9a-f]{0,4}`)
)

// Text masks the personal parts of s.
func (p Personal) Text(s string) string {
	if s == "" {
		return s
	}
	if p.Home != "" && p.Home != "/" {
		home := strings.TrimRight(p.Home, "/")
		s = strings.ReplaceAll(s, home+"/", "~/")
		s = replaceWord(s, home, "~")
	}
	s = usersRE.ReplaceAllStringFunc(s, func(m string) string {
		if m == "/Users/Shared" {
			return m
		}
		return "/Users/USER"
	})
	s = emailRE.ReplaceAllStringFunc(s, func(m string) string {
		for _, k := range p.KeepEmails {
			if strings.EqualFold(m, k) {
				return m
			}
		}
		g := emailRE.FindStringSubmatch(m)
		return g[1] + "***@" + g[2]
	})
	names := make([]string, 0, len(p.Names))
	for _, n := range p.Names {
		n = strings.TrimSpace(n)
		if l := len([]rune(n)); l >= 3 || l == 2 && len(n) > 2 { // "ann", "张三"
			names = append(names, n)
		}
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for _, n := range names {
		s = replaceWord(s, n, "USER")
	}
	s = ipv4RE.ReplaceAllStringFunc(s, func(m string) string {
		if strings.HasPrefix(m, "127.") || m == "0.0.0.0" {
			return m
		}
		g := ipv4RE.FindStringSubmatch(m)
		return g[1] + ".x.x"
	})
	s = ipv6RE.ReplaceAllStringFunc(s, func(m string) string {
		ip := net.ParseIP(m)
		if ip == nil || ip.To4() != nil || ip.IsLoopback() || ip.IsUnspecified() {
			return m
		}
		return "IPv6"
	})
	return s
}

// replaceWord replaces old in s where it is not inside a longer ASCII word
// (letters, digits, '_' on either side), matching case. A name in another
// script is replaced wherever it stands.
func replaceWord(s, old, repl string) string {
	if old == "" {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(s, old)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		end := i + len(old)
		if (i > 0 && isWordByte(s[i-1])) || (end < len(s) && isWordByte(s[end])) {
			b.WriteString(s[:end])
			s = s[end:]
			continue
		}
		b.WriteString(s[:i])
		b.WriteString(repl)
		s = s[end:]
	}
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
