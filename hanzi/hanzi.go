// Package hanzi tells the two Chinese scripts apart, character by
// character: the characters only Traditional Chinese writes and their
// Simplified forms, and the other way round. A model asked for
// Simplified Chinese writes Traditional now and then
// ("最近我又見到一個很久沒見的老同學了"), and the other way round; a
// system translation engine may too. transcheck finds and fixes that in a
// translation; Paralign compares a Traditional track with a Simplified one
// through Simplified.
//
// The tables (table.go) are ICU's Hant-Hans and Hans-Hant transforms of
// U+3400–U+9FFF, keeping the characters the other script's national
// charset cannot encode (GB 2312 for Simplified, Big5 for Traditional), so
// that a character both scripts write (著, 乾, 後) is never taken for the
// other script. Made on a Mac with:
//
//	let gb = String.Encoding(rawValue: CFStringConvertEncodingToNSStringEncoding(CFStringEncoding(CFStringEncodings.EUC_CN.rawValue)))
//	let big5 = String.Encoding(rawValue: CFStringConvertEncodingToNSStringEncoding(CFStringEncoding(CFStringEncodings.big5.rawValue)))
//	for v in 0x3400...0x9FFF, c = String(Character(Unicode.Scalar(v)!)):
//	  T: s = c.applyingTransform(StringTransform("Hant-Hans"), reverse: false); s != c, one character, c not in gb, s in gb
//	  S: t = c.applyingTransform(StringTransform("Hans-Hant"), reverse: false); t != c, one character, c not in big5, t in big5
package hanzi

import (
	"strings"
	"sync"
)

var maps = sync.OnceValues(func() (toHans, toHant map[rune]rune) {
	pairs := func(s string) map[rune]rune {
		r := []rune(s)
		m := make(map[rune]rune, len(r)/2)
		for i := 0; i+1 < len(r); i += 2 {
			m[r[i]] = r[i+1]
		}
		return m
	}
	return pairs(traditionalOnly), pairs(simplifiedOnly)
})

// Simplified is r in Simplified Chinese when only Traditional Chinese
// writes it, else r: a character both scripts write (著, 乾, 後) stays.
func Simplified(r rune) rune {
	toHans, _ := maps()
	if s, ok := toHans[r]; ok {
		return s
	}
	return r
}

// Traditional is r in Traditional Chinese when only Simplified Chinese
// writes it, else r.
func Traditional(r rune) rune {
	_, toHant := maps()
	if t, ok := toHant[r]; ok {
		return t
	}
	return r
}

// TraditionalOnly reports whether r is a character only Traditional
// Chinese writes.
func TraditionalOnly(r rune) bool {
	toHans, _ := maps()
	_, ok := toHans[r]
	return ok
}

// SimplifiedOnly reports whether r is a character only Simplified Chinese
// writes.
func SimplifiedOnly(r rune) bool {
	_, toHant := maps()
	_, ok := toHant[r]
	return ok
}

// HasTraditionalOnly reports whether s has a character only Traditional
// Chinese writes: s is not (all) Simplified Chinese.
func HasTraditionalOnly(s string) bool { return hasAny(s, TraditionalOnly) }

// HasSimplifiedOnly reports whether s has a character only Simplified
// Chinese writes: s is not (all) Traditional Chinese.
func HasSimplifiedOnly(s string) bool { return hasAny(s, SimplifiedOnly) }

// ToSimplified writes the characters of s that only Traditional Chinese
// writes in Simplified Chinese; s itself when it has none.
func ToSimplified(s string) string {
	if !HasTraditionalOnly(s) {
		return s
	}
	return strings.Map(Simplified, s)
}

// ToTraditional writes the characters of s that only Simplified Chinese
// writes in Traditional Chinese; s itself when it has none.
func ToTraditional(s string) string {
	if !HasSimplifiedOnly(s) {
		return s
	}
	return strings.Map(Traditional, s)
}

func hasAny(s string, f func(rune) bool) bool {
	for _, r := range s {
		if f(r) {
			return true
		}
	}
	return false
}
