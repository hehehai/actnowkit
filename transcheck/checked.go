package transcheck

import (
	"strings"
	"unicode/utf8"

	"github.com/hehehai/actnowkit/lang"
)

// AsItIs reports a line that may stand for its own translation when the
// model will not translate it: three words at most ("Sam.", "Google
// DeepMind.", "Mm-hmm."), names or interjections that read the same in
// any language.
func AsItIs(src string) bool {
	f := strings.Fields(src)
	return len(f) > 0 && len(f) <= 3 && utf8.RuneCountInString(src) <= 32
}

// Checked checks a translation another engine made (the system's) as the
// apps check a model's: Chinese is written in the target's script (the
// system sometimes answers Simplified Chinese in Traditional characters);
// ok is false when it is empty or the source left as it was in another
// script (an echo), unless the source is a short line that may stand as
// it is (AsItIs). A target lang.Parse does not know only needs a
// translation that is not empty.
func Checked(src, out, target string) (string, bool) {
	tgt, ok := lang.Parse(target)
	if !ok {
		return out, strings.TrimSpace(out) != ""
	}
	out = strings.TrimSpace(ToScript(out, tgt))
	if out == "" {
		return "", strings.TrimSpace(src) == ""
	}
	if Untranslated(src, out, tgt) && !AsItIs(src) {
		return out, false
	}
	return out, true
}
