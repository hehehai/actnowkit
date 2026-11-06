// Package i18n translates an app's UI. Messages are keyed by their English
// source text, as Apple's NSLocalizedString and Qt's tr() do:
//
//	cat.T("Generate")
//	cat.T("%d takes", len(takes))
//	cat.N(n, "%d take", "%d takes", n)
//	cat.X("verb", "Export")  // same English, different translations
//
// so wrapping a string never changes the English UI, and missing (or
// empty) translations fall back to English. A Catalog's messages are JSON
// objects {source: translation}, one file <tag>.json per supported
// language, read from the app's fs.FS (its embedded locales); the source
// language's file, when there is one, is empty by design.
//
// An app keeps a small package of its own that embeds its locales, makes
// the Catalog and offers T, N and X as functions (the calls i18ntest's
// parity check scans for):
//
//	//go:embed locales/*.json
//	var localeFS embed.FS
//
//	var cat = i18n.MustNew(i18n.Config{Locales: localeFS, Dir: "locales", Supported: []string{"en", "zh-Hans"}})
//
//	func T(msg string, args ...any) string { return cat.T(msg, args...) }
//
// Package i18ntest checks the catalogues against the sources: every message
// translated, none left over, the same formatting verbs.
//
// A Catalog is safe from any goroutine; SetLocale takes effect at the next
// call.
package i18n

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync/atomic"
	"time"
)

// Config says where a Catalog's messages are.
type Config struct {
	// Locales holds the catalogues: Dir/<tag>.json for every supported
	// language but the source one, which needs none.
	Locales fs.FS
	// Dir is the catalogues' directory in Locales ("locales"; "" or "."
	// for its root).
	Dir string
	// Supported lists the UI languages as BCP-47 tags, the source language
	// (the messages' language, English) first.
	Supported []string
}

// Catalog is an app's translations and the language in use.
type Catalog struct {
	supported []string
	messages  map[string]map[string]string // by tag
	current   atomic.Pointer[active]
	now       func() time.Time // the dates' current year; time.Now
}

type active struct {
	tag      string
	messages map[string]string
}

// New reads the catalogues of cfg.Supported and activates the source
// language. It fails when a catalogue is missing or not a JSON object of
// strings, or when the source language's catalogue has entries.
func New(cfg Config) (*Catalog, error) {
	if len(cfg.Supported) == 0 {
		return nil, errors.New("i18n: no supported language")
	}
	c := &Catalog{supported: slices.Clone(cfg.Supported), messages: map[string]map[string]string{}, now: time.Now}
	dir := cfg.Dir
	if dir == "" {
		dir = "."
	}
	for i, tag := range c.supported {
		if slices.Index(c.supported, tag) != i {
			return nil, fmt.Errorf("i18n: %s listed twice", tag)
		}
		name := path.Join(dir, tag+".json")
		data, err := fs.ReadFile(cfg.Locales, name)
		if err != nil {
			if i == 0 && errors.Is(err, fs.ErrNotExist) {
				c.messages[tag] = map[string]string{}
				continue
			}
			return nil, fmt.Errorf("i18n: %w", err)
		}
		m := map[string]string{}
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("i18n: %s: %w", name, err)
		}
		if i == 0 && len(m) > 0 {
			return nil, fmt.Errorf("i18n: %s must be empty (%s is the source language): %d entries", name, tag, len(m))
		}
		c.messages[tag] = m
	}
	c.SetLocale(c.supported[0])
	return c, nil
}

// MustNew is New, panicking on an error: for a package variable over
// embedded catalogues, which the app's tests load.
func MustNew(cfg Config) *Catalog {
	c, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return c
}

// Supported lists the UI languages, the source language first.
func (c *Catalog) Supported() []string { return slices.Clone(c.supported) }

// Locale returns the active tag.
func (c *Catalog) Locale() string { return c.current.Load().tag }

// SetLocale activates the closest supported language to tag (Match) and
// returns it.
func (c *Catalog) SetLocale(tag string) string {
	tag = c.Match(tag)
	c.current.Store(&active{tag: tag, messages: c.messages[tag]})
	return tag
}

// Match maps a system or user locale to the closest supported tag: Match
// with the Catalog's languages.
func (c *Catalog) Match(tag string) string { return Match(tag, c.supported) }

// T translates msg and, when args are given, formats it with fmt.Sprintf.
func (c *Catalog) T(msg string, args ...any) string {
	return format(c.lookup(msg), args)
}

// X translates msg in a context, for one English text that needs two
// translations; the catalogue key is "context\x04msg" (gettext's
// convention). It falls back to msg, not to the key.
func (c *Catalog) X(context, msg string, args ...any) string {
	if s := c.current.Load().messages[context+"\x04"+msg]; s != "" {
		return format(s, args)
	}
	return format(msg, args)
}

// N picks the singular (one) or plural (other) English source for n by the
// active language's plural rule (PluralOne), translates it and formats it
// with args. Languages without a singular (Chinese, Japanese…) always use
// other. A singular that says no number ("Moved to the trash" for "Moved
// %d items to the trash") is not formatted: the args are other's, and
// would show as "%!(EXTRA int=1)".
func (c *Catalog) N(n int, one, other string, args ...any) string {
	if PluralOne(c.Locale(), n) {
		s := c.lookup(one)
		if !hasVerb(s) {
			return s
		}
		return format(s, args)
	}
	return format(c.lookup(other), args)
}

func (c *Catalog) lookup(msg string) string {
	if s := c.current.Load().messages[msg]; s != "" {
		return s
	}
	return msg
}

func format(s string, args []any) string {
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}

// hasVerb reports whether s has a formatting verb (%d, %s, %[2]s…; not
// %%).
func hasVerb(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '%' {
			if s[i+1] != '%' {
				return true
			}
			i++
		}
	}
	return false
}

// PluralOne reports whether the language tag uses its singular form for
// n: n == 1 for English and most European languages, 0 and 1 for French
// and Portuguese, never for languages without one (Chinese, Japanese,
// Korean, Vietnamese, Thai, Indonesian, Malay).
func PluralOne(tag string, n int) bool {
	switch base(tag) {
	case "zh", "ja", "ko", "vi", "th", "id", "ms", "lo", "my", "km":
		return false
	case "fr", "pt":
		return n == 0 || n == 1
	}
	return n == 1
}

// Match maps a system or user locale ("zh-CN", "zh_Hans_CN", "en-GB") to
// the closest tag of supported, whose first tag (the source language) is
// the answer when none is close. Chinese goes by script: Traditional for
// zh-Hant, zh-TW, zh-HK and zh-MO, else Simplified; one script is not
// offered for the other. Other languages match on their language subtag
// ("en-GB" → "en", "pt-BR" → "pt").
func Match(tag string, supported []string) string {
	if len(supported) == 0 {
		return ""
	}
	t := strings.ToLower(strings.ReplaceAll(tag, "_", "-"))
	for _, s := range supported {
		if strings.ToLower(s) == t {
			return s
		}
	}
	want := ""
	if base(t) == "zh" {
		want = "zh-hans"
		for _, sub := range strings.Split(t, "-")[1:] {
			switch sub {
			case "hant", "tw", "hk", "mo":
				want = "zh-hant"
			}
			if sub == "hans" || sub == "hant" {
				break
			}
		}
	}
	for _, s := range supported {
		ls := strings.ToLower(s)
		if want != "" && (ls == want || strings.HasPrefix(ls, want+"-") || ls == "zh" && want == "zh-hans") {
			return s
		}
		if want == "" && t != "" && base(ls) == base(t) {
			return s
		}
	}
	return supported[0]
}

// base is a tag's language subtag, lower case.
func base(tag string) string {
	b, _, _ := strings.Cut(strings.ToLower(strings.ReplaceAll(tag, "_", "-")), "-")
	return b
}

// names are the family's UI languages in their own language.
var names = map[string]string{
	"en":      "English",
	"zh-Hans": "简体中文",
	"zh-Hant": "繁體中文",
	"ja":      "日本語",
	"ko":      "한국어",
	"de":      "Deutsch",
	"fr":      "Français",
	"es":      "Español",
	"it":      "Italiano",
	"pt":      "Português",
	"pt-BR":   "Português (Brasil)",
	"ru":      "Русский",
	"vi":      "Tiếng Việt",
}

// Name is a language's name in that language, as a language picker shows
// it ("English", "简体中文"); the tag itself for one it does not know.
func Name(tag string) string {
	if n, ok := names[tag]; ok {
		return n
	}
	return tag
}
