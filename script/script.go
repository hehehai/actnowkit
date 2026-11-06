// Package script tells which writing system text is in, as the
// translation checks (transcheck) and the apps' text code need it: the
// scripts written without spaces between words, word characters, the
// script most of a text's letters are in, and the script a language is
// written in.
//
// Script names here are coarse, one per writing system the checks tell
// apart (Han, Japanese, Hangul, Cyrillic, Arabic, Hebrew, Thai,
// Devanagari, Greek, Latin), not ISO 15924 codes: Japanese is kana with
// or without Han, and every other script counts as Latin.
package script

import (
	"unicode"

	"github.com/hehehai/actnowkit/lang"
)

// The script names Dominant, Has and Of use.
const (
	Han        = "Han"
	Japanese   = "Japanese"
	Hangul     = "Hangul"
	Cyrillic   = "Cyrillic"
	Arabic     = "Arabic"
	Hebrew     = "Hebrew"
	Thai       = "Thai"
	Devanagari = "Devanagari"
	Greek      = "Greek"
	Latin      = "Latin"
)

// IsUnspaced reports whether r belongs to a script written without spaces
// between words (Han, kana, Thai, Lao, Khmer, Myanmar, Tibetan, Bopomofo,
// CJK punctuation and full-width forms), where a term may match anywhere
// instead of only at word boundaries. Hangul is spaced and is not
// included.
func IsUnspaced(r rune) bool {
	if r < 0x0E00 {
		return false
	}
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Thai, r) || unicode.Is(unicode.Lao, r) || unicode.Is(unicode.Khmer, r) ||
		unicode.Is(unicode.Myanmar, r) || unicode.Is(unicode.Tibetan, r) || unicode.Is(unicode.Bopomofo, r) ||
		(r >= 0x3000 && r <= 0x303F) || (r >= 0xFF00 && r <= 0xFFEF)
}

// IsWordRune reports whether r is part of a word: a letter, a digit or a
// combining mark.
func IsWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r)
}

// Dominant is the script most letters of s are in, "" when s has fewer
// than two letters. Kana weigh double, and Han with any kana is Japanese;
// a tie goes to the name first in alphabetical order.
func Dominant(s string) string {
	counts := map[string]int{}
	n := 0
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		n++
		switch {
		case unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r):
			counts[Japanese] += 2
		case unicode.Is(unicode.Han, r):
			counts[Han]++
		case unicode.Is(unicode.Hangul, r):
			counts[Hangul]++
		case unicode.Is(unicode.Cyrillic, r):
			counts[Cyrillic]++
		case unicode.Is(unicode.Arabic, r):
			counts[Arabic]++
		case unicode.Is(unicode.Hebrew, r):
			counts[Hebrew]++
		case unicode.Is(unicode.Thai, r):
			counts[Thai]++
		case unicode.Is(unicode.Devanagari, r):
			counts[Devanagari]++
		case unicode.Is(unicode.Greek, r):
			counts[Greek]++
		case unicode.Is(unicode.Latin, r):
			counts[Latin]++
		}
	}
	if n < 2 {
		return ""
	}
	best, bn := "", 0
	for k, v := range counts {
		if v > bn || v == bn && k < best {
			best, bn = k, v
		}
	}
	if best == Han && counts[Japanese] > 0 {
		return Japanese
	}
	return best
}

// Has reports whether s has a letter of script (one of the names above;
// Japanese takes kana or Han, an unknown name Latin).
func Has(s, script string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		var ok bool
		switch script {
		case Han:
			ok = unicode.Is(unicode.Han, r)
		case Japanese:
			ok = unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Han, r)
		case Hangul:
			ok = unicode.Is(unicode.Hangul, r)
		case Cyrillic:
			ok = unicode.Is(unicode.Cyrillic, r)
		case Arabic:
			ok = unicode.Is(unicode.Arabic, r)
		case Hebrew:
			ok = unicode.Is(unicode.Hebrew, r)
		case Thai:
			ok = unicode.Is(unicode.Thai, r)
		case Devanagari:
			ok = unicode.Is(unicode.Devanagari, r)
		case Greek:
			ok = unicode.Is(unicode.Greek, r)
		default:
			ok = unicode.Is(unicode.Latin, r)
		}
		if ok {
			return true
		}
	}
	return false
}

// Of is the script a language is written in, as Dominant names it:
// Serbian in Latin script (sr-Latn) is Latin, a language not listed is
// Latin.
func Of(l lang.Tag) string {
	switch l.Code {
	case "zh", "yue", "wuu":
		return Han
	case "ja":
		return Japanese
	case "ko":
		return Hangul
	case "ru", "uk", "bg", "sr", "be", "kk", "mk":
		if l.Code == "sr" && l.Script == "Latn" {
			return Latin
		}
		return Cyrillic
	case "ar", "fa", "ur":
		return Arabic
	case "he", "yi":
		return Hebrew
	case "th":
		return Thai
	case "hi", "mr", "ne":
		return Devanagari
	case "el":
		return Greek
	}
	return Latin
}
