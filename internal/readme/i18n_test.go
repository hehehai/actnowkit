package readme

import (
	"embed"
	"testing"

	"github.com/hehehai/actnowkit/i18n"
	"github.com/hehehai/actnowkit/i18n/i18ntest"
	"github.com/hehehai/actnowkit/thirdparty"
)

var localeFS embed.FS

// i18nExample mirrors README.md's "Translations" and "Third-party licences".
func i18nExample(t *testing.T) {
	cat := i18n.MustNew(i18n.Config{Locales: localeFS, Dir: "locales", Supported: []string{"en", "zh-Hans"}})
	T := func(msg string, args ...any) string { return cat.T(msg, args...) }
	N := func(n int, one, other string, args ...any) string { return cat.N(n, one, other, args...) }
	X := func(context, msg string, args ...any) string { return cat.X(context, msg, args...) }
	SetLocale := func(tag string) string { return cat.SetLocale(tag) }
	_, _, _, _ = T, N, X, SetLocale
	i18ntest.Parity{
		Root:      "../..",
		Packages:  []string{"github.com/hehehai/Dicta/internal/i18n"},
		Skip:      []string{"engine"},
		Dir:       "locales",
		Supported: cat.Supported(),
	}.Check(t)
	_ = i18ntest.Style(map[string]string{}, i18ntest.ZhHansRules)
	_, comps, _ := thirdparty.Load("resources")
	_ = comps
}

func TestI18nReadmeCompiles(t *testing.T) { _ = i18nExample }
