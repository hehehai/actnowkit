package script

import (
	"testing"

	"github.com/hehehai/actnowkit/lang"
)

func TestIsUnspaced(t *testing.T) {
	for r, want := range map[rune]bool{
		'中': true, 'の': true, 'カ': true, 'ก': true, 'ລ': true, 'ក': true, 'က': true, 'ཀ': true, 'ㄅ': true,
		'。': true, '，': true, 'Ａ': true,
		'a': false, 'é': false, '한': false, 'Я': false, 'ع': false, ' ': false, '1': false, '.': false,
	} {
		if IsUnspaced(r) != want {
			t.Errorf("IsUnspaced(%q) = %v", r, !want)
		}
	}
}

func TestIsWordRune(t *testing.T) {
	for r, want := range map[rune]bool{'a': true, '中': true, '7': true, '́': true, 'ั': true, 'ा': true, ' ': false, '.': false, '-': false, '。': false} {
		if IsWordRune(r) != want {
			t.Errorf("IsWordRune(%q) = %v", r, !want)
		}
	}
}

func TestDominant(t *testing.T) {
	for s, want := range map[string]string{
		"预算还没定":          Han,
		"今日は雨です":         Japanese, // Han with kana
		"東京タワー":          Japanese,
		"안녕하세요":          Hangul,
		"Привет":         Cyrillic,
		"مرحبا":          Arabic,
		"שלום":           Hebrew,
		"สวัสดี":         Thai,
		"नमस्ते":         Devanagari,
		"Γειά":           Greek,
		"Hello, 世界":      Latin,
		"Bonjour à tous": Latin,
		"a":              "", // too few letters
		"5 7 9":          "",
		"ab":             Latin,
		"中a":             Han, // a tie: the first name
	} {
		if got := Dominant(s); got != want {
			t.Errorf("Dominant(%q) = %q, want %q", s, got, want)
		}
	}
}

func TestHas(t *testing.T) {
	cases := []struct {
		s, script string
		want      bool
	}{
		{"Hello 世界", Han, true},
		{"Hello", Han, false},
		{"漢字", Japanese, true},
		{"かな", Japanese, true},
		{"かな", Han, false},
		{"한", Hangul, true},
		{"Я", Cyrillic, true},
		{"ع", Arabic, true},
		{"ש", Hebrew, true},
		{"ก", Thai, true},
		{"न", Devanagari, true},
		{"Γ", Greek, true},
		{"a", Latin, true},
		{"a", "Klingon", true}, // unknown names are Latin
		{"123 !", Latin, false},
	}
	for _, c := range cases {
		if got := Has(c.s, c.script); got != c.want {
			t.Errorf("Has(%q, %s) = %v", c.s, c.script, got)
		}
	}
}

func TestOf(t *testing.T) {
	for tag, want := range map[string]string{
		"zh-Hant": Han, "yue": Han, "ja": Japanese, "ko": Hangul, "ru": Cyrillic, "sr": Cyrillic, "sr-Latn": Latin,
		"fa": Arabic, "yi": Hebrew, "th": Thai, "ne": Devanagari, "el": Greek, "en": Latin, "vi": Latin, "ka": Latin,
	} {
		l, ok := lang.Parse(tag)
		if !ok {
			t.Fatalf("Parse(%q)", tag)
		}
		if got := Of(l); got != want {
			t.Errorf("Of(%s) = %q, want %q", tag, got, want)
		}
	}
}
