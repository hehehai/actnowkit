// Package transcheck checks a translation of a line, from a model or a
// system engine: left untranslated, in the wrong script, without words,
// much shorter or longer than its source, repeating another line's
// translation, or Chinese in the other script (hanzi). The apps translate
// a line that fails one again on its own; the checks are the ones their
// subtitle and transcript translations share.
//
// Lengths are compared in rough units of meaning (MeaningUnits): a CJK
// character is one, three letters or digits of another script are one.
package transcheck

import (
	"strings"
	"unicode"

	"github.com/hehehai/actnowkit/hanzi"
	"github.com/hehehai/actnowkit/lang"
	"github.com/hehehai/actnowkit/script"
)

// Untranslated reports whether a translation is the source left as it
// was although it is not written in the target's script (the model
// echoed it), or is written in another script than the target's while
// the source was too (the model rephrased it in the source language). A
// line already in the target language may come back the same: that is
// accepted.
func Untranslated(src, out string, tgt lang.Tag) bool {
	if strings.TrimSpace(src) == strings.TrimSpace(out) {
		s := script.Dominant(src)
		return s != "" && s != script.Of(tgt)
	}
	return WrongScript(src, out, tgt)
}

// CutShort reports a translation that says much less than its source: a
// model that split one line's sentences over two ids (and dropped the
// next line) leaves each half far too short, and one that stopped at the
// first sentence of a line leaves it short too. The line is then
// translated again on its own. Lengths are compared in
// MeaningUnits.
func CutShort(src, out string) bool {
	su, ou := MeaningUnits(src), MeaningUnits(out)
	if su < 20 {
		return false
	}
	ratio := ou / su / expansion(src, out)
	if ratio < 0.35 {
		return true
	}
	ss, os := Sentences(src), Sentences(out)
	return ss >= 2 && os < ss && ratio < 0.6
}

// expansion is how many more meaning units a full translation has than
// its source, roughly, between the CJK scripts and the others: a Chinese
// line said fully in English takes about 1.4 times its units (a
// translation of only its first sentence would pass for whole otherwise).
func expansion(src, out string) float64 {
	cjk := func(s string) bool {
		switch script.Dominant(s) {
		case script.Han, script.Japanese, script.Hangul:
			return true
		}
		return false
	}
	// The other way the units already come out about even (three letters
	// a unit), and the thresholds were set on English into Chinese.
	if cjk(src) && !cjk(out) {
		return 1.4
	}
	return 1
}

// TooLong reports a translation that says much more than its source: a
// model that put another line's text (the next one, or a context line)
// into this one. Lengths are compared as in CutShort; a short
// translation is never too long (names, interjections).
func TooLong(src, out string) bool {
	su, ou := MeaningUnits(src), MeaningUnits(out)
	if ou < 8 {
		return false
	}
	ratio := ou / max(su, 1)
	if ratio > 2.6 {
		return true
	}
	return Sentences(out) >= Sentences(src)+2 && ratio > 1.6
}

// MeaningUnits is how much s says, roughly: a Han, kana or Hangul
// character is one unit, three letters or digits of another script are
// one.
func MeaningUnits(s string) float64 {
	n := 0.0
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r), unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r), unicode.Is(unicode.Hangul, r):
			n++
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			n += 1.0 / 3
		}
	}
	return n
}

// SplitSentences cuts a text after each sentence's final punctuation
// (as Sentences counts it).
func SplitSentences(s string) []string {
	var out []string
	rs := []rune(strings.TrimSpace(s))
	start := 0
	for i := range rs {
		if sentenceEnd(rs, i) {
			if p := strings.TrimSpace(string(rs[start : i+1])); p != "" {
				out = append(out, p)
			}
			start = i + 1
		}
	}
	if p := strings.TrimSpace(string(rs[start:])); p != "" {
		out = append(out, p)
	}
	return out
}

// sentenceEnd reports whether rs[i] ends a sentence.
func sentenceEnd(rs []rune, i int) bool {
	switch rs[i] {
	case '。', '？', '！', '?', '!':
		return true
	case '.':
		// "snow. Then", "cooling.\"" and a final "." end a sentence;
		// "1.5" and "U.S. Army" do not ("Mr. Li" does: a lower-case letter
		// before the full stop).
		end := i == len(rs)-1 || unicode.IsSpace(rs[i+1]) || strings.ContainsRune(`"'”’)`, rs[i+1])
		return end && i > 0 && (unicode.IsLower(rs[i-1]) || unicode.IsDigit(rs[i-1]))
	}
	return false
}

// Sentences counts a text's sentences (its final punctuation; one at least).
func Sentences(s string) int {
	n := 0
	rs := []rune(strings.TrimSpace(s))
	for i := range rs {
		if sentenceEnd(rs, i) {
			n++
		}
	}
	return max(n, 1)
}

// WrongScript reports a translation without a letter of the target's
// script from a source in another one (English "translated" into English
// when Chinese was asked for).
func WrongScript(src, out string, tgt lang.Tag) bool {
	want := script.Of(tgt)
	ss := script.Dominant(src)
	return ss != "" && ss != want && script.Dominant(out) != "" && !script.Has(out, want)
}

// Wordless reports a translation without a letter of a source that has
// words ("1", "}1": a model that wrote a line's number, or lost its
// place in the JSON).
func Wordless(src, out string) bool {
	letters := func(s string) int {
		n := 0
		for _, r := range s {
			if unicode.IsLetter(r) {
				n++
			}
		}
		return n
	}
	return letters(src) >= 2 && letters(out) == 0
}

// ShortenedTooMuch reports a shortened translation that kept less than a
// third of what it said: not a shorter way to say it, but another text.
func ShortenedTooMuch(was, now string) bool {
	return MeaningUnits(now) < MeaningUnits(was)/3
}

// Repeats: a translation that repeats much of another line's (its word
// trigrams, OverlapShare of them at least) while their sources differ
// took that line's text: the model translated a context line, or the
// rest of the line before, under this line's id.
const OverlapShare = 0.45

// trigrams are a text's runs of three tokens: lower-case words, and each
// character of a script without spaces. nil when it has fewer than
// three tokens (too short to tell).
func trigrams(s string) map[string]bool {
	var toks []string
	var word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			toks = append(toks, word.String())
			word.Reset()
		}
	}
	for _, r := range strings.ToLower(s) {
		switch {
		case script.IsUnspaced(r) && script.IsWordRune(r) || unicode.Is(unicode.Hangul, r):
			flush()
			toks = append(toks, string(r))
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			word.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	if len(toks) < 3 {
		return nil
	}
	out := map[string]bool{}
	for i := 0; i+2 < len(toks); i++ {
		out[toks[i]+"\x00"+toks[i+1]+"\x00"+toks[i+2]] = true
	}
	return out
}

// share is the part of a's trigrams that b has too.
func share(a, b map[string]bool) float64 {
	if len(a) == 0 {
		return 0
	}
	n := 0
	for g := range a {
		if b[g] {
			n++
		}
	}
	return float64(n) / float64(len(a))
}

// Repeats reports whether translation out (of source src) repeats the
// translation other of another line (of source otherSrc): it shares
// OverlapShare of its trigrams with it (or other with it), while the
// sources do not (a speaker saying the same thing twice).
func Repeats(src, out, otherSrc, other string) bool {
	a, b := trigrams(out), trigrams(other)
	if a == nil || b == nil || max(share(a, b), share(b, a)) < OverlapShare {
		return false
	}
	sa, sb := trigrams(src), trigrams(otherSrc)
	return sa == nil || sb == nil || max(share(sa, sb), share(sb, sa)) < OverlapShare
}

// ChineseScript is the Chinese script a target writes, "Hans" or "Hant"
// (zh-TW, zh-Hant, yue-Hant), "" when it is not Chinese. Cantonese is
// Traditional as the table writes it ("yue": Hant).
func ChineseScript(tgt lang.Tag) string {
	if tgt.Code != "zh" && tgt.Code != "yue" {
		return ""
	}
	if tgt.Script == "Hant" {
		return "Hant"
	}
	return "Hans"
}

// OtherHan reports whether a translation into tgt has a character only
// the other Chinese script writes (hanzi): a model asked for Simplified
// Chinese wrote Traditional, or the other way round.
func OtherHan(out string, tgt lang.Tag) bool {
	switch ChineseScript(tgt) {
	case "Hans":
		return hanzi.HasTraditionalOnly(out)
	case "Hant":
		return hanzi.HasSimplifiedOnly(out)
	}
	return false
}

// ToScript writes the characters of out that only the other Chinese
// script writes in tgt's script; out itself when tgt is not Chinese or
// out has none.
func ToScript(out string, tgt lang.Tag) string {
	switch ChineseScript(tgt) {
	case "Hans":
		return hanzi.ToSimplified(out)
	case "Hant":
		return hanzi.ToTraditional(out)
	}
	return out
}
