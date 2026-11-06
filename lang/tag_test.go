package lang

import "testing"

func TestParse(t *testing.T) {
	cases := map[string][2]string{
		"zh-TW":      {"zh-Hant", "Chinese (Traditional, Taiwan)"},
		"zh-Hant-HK": {"zh-Hant", "Chinese (Traditional, Hong Kong)"},
		"zh-Hans-HK": {"zh-Hans", "Chinese (Simplified, Hong Kong)"},
		"zh":         {"zh-Hans", "Chinese (Simplified)"},
		"zh-CN":      {"zh-Hans", "Chinese (Simplified, Mainland China)"},
		"pt-BR":      {"pt-BR", "Portuguese (Brazil)"},
		"pt_BR":      {"pt-BR", "Portuguese (Brazil)"},
		"es-419":     {"es-419", "Spanish (Latin America)"},
		"en":         {"en", "English"},
		"cmn":        {"zh-Hans", "Chinese (Simplified)"},
		"sr-Latn":    {"sr-Latn", "Serbian (Latin script)"},
		"sr":         {"sr", "Serbian (Cyrillic script)"},
		"de-LU":      {"de-LU", "German (LU)"},
	}
	for tag, want := range cases {
		l, ok := Parse(tag)
		if !ok || l.Variant() != want[0] || l.Describe() != want[1] {
			t.Errorf("%s: %q %q (%+v)", tag, l.Variant(), l.Describe(), l)
		}
	}
	for _, bad := range []string{"", "  ", "xx-nope", "auto"} {
		if _, ok := Parse(bad); ok {
			t.Errorf("Parse(%q) accepted", bad)
		}
	}
	if l, _ := Parse(" ar-EG "); l.Tag != "ar-EG" || l.Code != "ar" || !l.RTL || l.Region != "EG" || l.Native != "العربية" {
		t.Errorf("ar-EG: %+v", l)
	}
	if l, _ := Parse("ja"); l.Region != "" || l.Script != "Jpan" {
		t.Errorf("ja: a likely region is not the tag's: %+v", l)
	}
}

func TestUnspaced(t *testing.T) {
	for tag, want := range map[string]bool{"zh-Hant": true, "ja": true, "th": true, "yue": true, "ko": false, "en": false, "vi": false} {
		if l, _ := Parse(tag); l.Unspaced() != want {
			t.Errorf("%s: Unspaced %v", tag, !want)
		}
	}
}
