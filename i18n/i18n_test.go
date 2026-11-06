package i18n

import (
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

var locales = fstest.MapFS{
	"locales/en.json": {Data: []byte("{}\n")},
	"locales/zh-Hans.json": {Data: []byte(`{
		"Settings": "设置",
		"%d transcript": "%d 条转录",
		"%d transcripts": "%d 条转录",
		"Moved to the trash": "已移到废纸篓",
		"Moved %d items to the trash": "已将 %d 项移到废纸篓",
		"Empty": "",
		"menu\u0004Open": "",
		"menu\u0004Close": "关闭",
		"%[1]s of %[2]d": "%[2]d 中的 %[1]s"
	}`)},
}

func newCatalog(t *testing.T) *Catalog {
	t.Helper()
	c, err := New(Config{Locales: locales, Dir: "locales", Supported: []string{"en", "zh-Hans"}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEnglishIsIdentity(t *testing.T) {
	c := newCatalog(t)
	if c.Locale() != "en" {
		t.Fatal("the source language is active first:", c.Locale())
	}
	if got := c.T("Generate"); got != "Generate" {
		t.Error(got)
	}
	if got := c.T("%d takes", 3); got != "3 takes" {
		t.Error(got)
	}
	if got := c.T("100%"); got != "100%" {
		t.Error("a message without args must not be formatted:", got)
	}
	if got := c.N(1, "%d take", "%d takes", 1); got != "1 take" {
		t.Error(got)
	}
	if got := c.N(2, "%d take", "%d takes", 2); got != "2 takes" {
		t.Error(got)
	}
	if got := c.N(0, "%d take", "%d takes", 0); got != "0 takes" {
		t.Error(got)
	}
	if got := c.X("verb", "Export"); got != "Export" {
		t.Error(got)
	}
	if got := c.X("verb", "Export %s", "a"); got != "Export a" {
		t.Error(got)
	}
}

func TestChinese(t *testing.T) {
	c := newCatalog(t)
	if got := c.SetLocale("zh_CN"); got != "zh-Hans" || c.Locale() != "zh-Hans" {
		t.Fatal(got)
	}
	if got := c.T("Settings"); got != "设置" {
		t.Error(got)
	}
	// Missing and empty translations fall back to English.
	if got := c.T("A message nobody translated"); got != "A message nobody translated" {
		t.Error(got)
	}
	if got := c.T("Empty"); got != "Empty" {
		t.Error(got)
	}
	if got := c.N(1, "%d transcript", "%d transcripts", 1); got != "1 条转录" {
		t.Error("zh-Hans has no singular, so it uses the translation of other:", got)
	}
	if got := c.T("%[1]s of %[2]d", "a", 2); got != "2 中的 a" {
		t.Error("reordered verbs:", got)
	}
}

// X falls back to English for an empty translation, as T does.
func TestXEmptyTranslation(t *testing.T) {
	c := newCatalog(t)
	c.SetLocale("zh-Hans")
	if got := c.X("menu", "Open"); got != "Open" {
		t.Errorf("empty: %q", got)
	}
	if got := c.X("menu", "Close"); got != "关闭" {
		t.Errorf("translated: %q", got)
	}
}

// A singular that says no number is shown as it is, the count passed for
// the plural not appended as "%!(EXTRA int=1)"; the plural and a singular
// with verbs are formatted.
func TestPluralSingularWithoutNumber(t *testing.T) {
	c := newCatalog(t)
	for _, tc := range []struct {
		n         int
		one, want string
	}{
		{1, "Moved to the trash", "Moved to the trash"},
		{1, "50%% moved", "50%% moved"},
		{1, "%d item moved", "1 item moved"},
		{2, "Moved to the trash", "Moved 2 items to the trash"},
	} {
		if got := c.N(tc.n, tc.one, "Moved %d items to the trash", tc.n); got != tc.want {
			t.Errorf("N(%d, %q) = %q, want %q", tc.n, tc.one, got, tc.want)
		}
	}
	c.SetLocale("zh-Hans")
	if got := c.N(1, "Moved to the trash", "Moved %d items to the trash", 1); got != "已将 1 项移到废纸篓" {
		t.Errorf("zh-Hans: %q (the plural, with its number)", got)
	}
}

func TestPluralOne(t *testing.T) {
	for _, tc := range []struct {
		tag  string
		n    int
		want bool
	}{
		{"en", 1, true}, {"en", 0, false}, {"en", 2, false}, {"de", 1, true},
		{"fr", 0, true}, {"fr", 1, true}, {"fr", 2, false}, {"pt-BR", 0, true},
		{"zh-Hans", 1, false}, {"ja", 1, false}, {"ko", 1, false},
	} {
		if got := PluralOne(tc.tag, tc.n); got != tc.want {
			t.Errorf("PluralOne(%s, %d) = %v", tc.tag, tc.n, got)
		}
	}
}

func TestMatch(t *testing.T) {
	two := []string{"en", "zh-Hans"}
	for in, want := range map[string]string{
		"": "en", "en": "en", "en-GB": "en", "en_US": "en", "fr-FR": "en", "xx": "en",
		"zh": "zh-Hans", "zh-CN": "zh-Hans", "zh-Hans": "zh-Hans", "zh_Hans_CN": "zh-Hans", "zh-SG": "zh-Hans", "ZH-HANS": "zh-Hans",
		"zh-TW": "en", "zh-Hant": "en", "zh-HK": "en",
	} {
		if got := Match(in, two); got != want {
			t.Errorf("Match(%q, en+zh-Hans) = %q, want %q", in, got, want)
		}
	}
	more := []string{"en", "zh-Hans", "zh-Hant", "ja", "pt-BR"}
	for in, want := range map[string]string{
		"zh-TW": "zh-Hant", "zh_Hant_HK": "zh-Hant", "zh-MO": "zh-Hant", "zh-Hans-TW": "zh-Hans", "zh-CN": "zh-Hans",
		"ja-JP": "ja", "pt": "pt-BR", "pt-PT": "pt-BR", "de": "en",
	} {
		if got := Match(in, more); got != want {
			t.Errorf("Match(%q, %v) = %q, want %q", in, more, got, want)
		}
	}
	if got := Match("zh-CN", []string{"en", "zh"}); got != "zh" {
		t.Errorf("plain zh: %q", got)
	}
	if got := Match("en", nil); got != "" {
		t.Errorf("nothing supported: %q", got)
	}
}

func TestNew(t *testing.T) {
	c := newCatalog(t)
	if s := c.Supported(); len(s) != 2 || s[0] != "en" {
		t.Error(s)
	}
	c.Supported()[0] = "xx"
	if c.Supported()[0] != "en" {
		t.Error("Supported must return a copy")
	}
	// The source language needs no file; the others do.
	noEN := fstest.MapFS{"zh-Hans.json": locales["locales/zh-Hans.json"]}
	if _, err := New(Config{Locales: noEN, Supported: []string{"en", "zh-Hans"}}); err != nil {
		t.Error("root dir, no en.json:", err)
	}
	for name, cfg := range map[string]Config{
		"none supported": {Locales: locales, Dir: "locales"},
		"missing":        {Locales: locales, Dir: "locales", Supported: []string{"en", "ja"}},
		"twice":          {Locales: locales, Dir: "locales", Supported: []string{"en", "zh-Hans", "zh-Hans"}},
		"not json":       {Locales: fstest.MapFS{"zh-Hans.json": {Data: []byte("{")}}, Supported: []string{"en", "zh-Hans"}},
		"not strings":    {Locales: fstest.MapFS{"zh-Hans.json": {Data: []byte(`{"a": 1}`)}}, Supported: []string{"en", "zh-Hans"}},
		"source entries": {Locales: fstest.MapFS{"en.json": {Data: []byte(`{"a": "b"}`)}}, Supported: []string{"en"}},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("MustNew did not panic")
		}
	}()
	MustNew(Config{Locales: locales})
}

func TestNames(t *testing.T) {
	if Name("en") != "English" || Name("zh-Hans") != "简体中文" || Name("xx") != "xx" {
		t.Error(Name("en"), Name("zh-Hans"))
	}
}

func TestDates(t *testing.T) {
	c := newCatalog(t)
	c.now = func() time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.Local) }
	this := time.Date(2026, 3, 4, 15, 6, 0, 0, time.Local)
	old := time.Date(2024, 11, 22, 9, 5, 0, 0, time.Local)
	if got := c.DateTime(this); got != "Mar 4 15:06" {
		t.Errorf("en this year: %q", got)
	}
	if got := c.DateTime(old); got != "Nov 22, 2024 09:05" {
		t.Errorf("en: %q", got)
	}
	if got := c.Date(old); got != "Nov 22, 2024" {
		t.Errorf("en date: %q", got)
	}
	c.SetLocale("zh-Hans")
	if got := c.DateTime(this); got != "3月4日 15:06" {
		t.Errorf("zh this year: %q", got)
	}
	if got := c.DateTime(old); got != "2024年11月22日 09:05" {
		t.Errorf("zh: %q", got)
	}
	if got := c.Date(old); got != "2024年11月22日" {
		t.Errorf("zh date: %q", got)
	}
}

// SetLocale and the lookups run from any goroutine (go test -race).
func TestConcurrent(t *testing.T) {
	c := newCatalog(t)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 200 {
				if (i+j)%2 == 0 {
					c.SetLocale("zh-Hans")
				} else {
					c.SetLocale("en")
				}
				if s := c.T("Settings"); s != "Settings" && s != "设置" {
					t.Error(s)
				}
				_ = c.N(j, "%d transcript", "%d transcripts", j)
				_ = strings.TrimSpace(c.X("menu", "Close"))
			}
		}()
	}
	wg.Wait()
}
