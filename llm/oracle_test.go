package llm

// The apps' closeJSON (CaptionCraft's internal/ai/parse_json.go at
// v0.15.0 of actnowkit, with its fix for a list item cut off), kept as
// the oracle of CloseJSON (FuzzExtractJSON, TestCloseJSONSameAsTheApps').

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCloseJSONSameAsTheApps(t *testing.T) {
	for _, s := range []string{`{"a":"x\`, `{"a":1,`, `{"a":1,"b":`, `{"a":[1,2`, `x {"a":{"b":[{"c":1},{"c":2},{"c`, `{"a":1}`,
		`{"items":[{"id":"1","wrong":"a","right":"b"},{"id":"2","wrong":"e"`, `{"t":"a\"b`, `{"a":[[1,2],[3`, `{"a":tru`, "{\"a\":1,\n  ", `{`, `}{`} {
		if got, want := string(CloseJSON(s)), string(oldCloseJSON(s)); got != want {
			t.Errorf("CloseJSON(%q) = %q, the apps' %q", s, got, want)
		}
	}
}

// oldCloseJSON closes a JSON object cut off mid-way (a reply that ran out of
// tokens): the open string, arrays and objects, dropping a dangling key
// or comma. nil when text has no object or the result does not parse.
func oldCloseJSON(text string) json.RawMessage {
	i := strings.IndexByte(text, '{')
	if i < 0 {
		return nil
	}
	t := text[i:]
	var stack []byte
	var at []int // where each of stack opened
	inStr, esc := false, false
	for j := 0; j < len(t); j++ {
		c := t[j]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{', '[':
			stack, at = append(stack, c), append(at, j)
		case '}', ']':
			if len(stack) > 0 {
				stack, at = stack[:len(stack)-1], at[:len(at)-1]
			}
		}
	}
	if len(stack) == 0 && !inStr {
		return nil // balanced: ExtractJSON would have found it if it parsed
	}
	b := []byte(t)
	if n := len(stack); n >= 2 && stack[n-1] == '{' && stack[n-2] == '[' {
		// Cut inside an item of a list: the item goes, not closed half
		// written (a suggestion without its "right" would delete words).
		b, stack = b[:at[n-1]], stack[:n-1]
	} else {
		if esc {
			b = b[:len(b)-1]
		}
		if inStr {
			b = append(b, '"')
		}
	}
	// Try closing as is; else drop the last incomplete member.
	for attempt := 0; attempt < 6; attempt++ {
		out := append([]byte(nil), b...)
		out = oldTrimDangling(out)
		for k := len(stack) - 1; k >= 0; k-- {
			if stack[k] == '{' {
				out = append(out, '}')
			} else {
				out = append(out, ']')
			}
		}
		if json.Valid(out) {
			return out
		}
		// Drop back to the last comma outside a string and try again.
		cut := oldLastComma(b)
		if cut < 0 {
			return nil
		}
		b = b[:cut]
		stack = oldOpenStack(b)
	}
	return nil
}

// oldTrimDangling removes a trailing comma or a key without a value.
func oldTrimDangling(b []byte) []byte {
	s := strings.TrimRight(string(b), " \t\r\n")
	s = strings.TrimSuffix(s, ",")
	if strings.HasSuffix(s, ":") {
		// "key": with no value: drop the key.
		k := strings.LastIndex(s[:len(s)-1], `"`)
		if k > 0 {
			if k2 := strings.LastIndex(s[:k], `"`); k2 >= 0 {
				s = strings.TrimRight(s[:k2], " \t\r\n")
				s = strings.TrimSuffix(s, ",")
			}
		}
	}
	return []byte(s)
}

// oldLastComma is the index of the last comma outside strings.
func oldLastComma(b []byte) int {
	last := -1
	inStr, esc := false, false
	for j, c := range b {
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case ',':
			last = j
		}
	}
	return last
}

// oldOpenStack is the brackets open at the end of b.
func oldOpenStack(b []byte) []byte {
	var stack []byte
	inStr, esc := false, false
	for _, c := range b {
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{', '[':
			stack = append(stack, c)
		case '}', ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return stack
}
