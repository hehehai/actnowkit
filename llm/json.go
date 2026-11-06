package llm

import (
	"encoding/json"
	"strings"
)

// ExtractJSON finds the JSON value in a model's reply: the whole text,
// else the content of a ``` fence, else the longest balanced {…} or […]
// that parses, outside any other. A value left open (a reply cut off)
// ends the search: what it holds is a fragment (CloseJSON closes it). It
// returns nil when there is none.
func ExtractJSON(text string) json.RawMessage {
	t := strings.TrimSpace(text)
	// Reasoning models sometimes leak a <think>…</think> preamble.
	if i := strings.Index(t, "</think>"); i >= 0 && strings.HasPrefix(t, "<think>") {
		t = strings.TrimSpace(t[i+len("</think>"):])
	}
	if json.Valid([]byte(t)) && t != "" {
		return json.RawMessage(t)
	}
	if i := strings.Index(t, "```"); i >= 0 {
		rest := t[i+3:]
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			rest = rest[nl+1:]
		}
		if j := strings.Index(rest, "```"); j >= 0 {
			if f := strings.TrimSpace(rest[:j]); json.Valid([]byte(f)) {
				return json.RawMessage(f)
			}
		}
	}
	// The values in the text, not inside one another: the longest that
	// parses (not a "[1]" citing before the answer).
	var best string
	for i := 0; i < len(t); i++ {
		if t[i] != '{' && t[i] != '[' {
			continue
		}
		end := balanced(t, i)
		if end < 0 {
			break
		}
		if c := t[i:end]; len(c) > len(best) && json.Valid([]byte(c)) {
			best = c
		}
		i = end - 1
	}
	if best == "" {
		return nil
	}
	return json.RawMessage(best)
}

// ReplyJSON is the JSON of a reply. One cut off (it ran out of tokens) is
// closed first (CloseJSON): what ExtractJSON finds of it is a fragment.
// Then ExtractJSON, then CloseJSON again (a reply left open though it
// says it stopped). nil when there is none.
func ReplyJSON(resp Response) json.RawMessage {
	if resp.FinishReason == FinishLength {
		if raw := CloseJSON(resp.Text); raw != nil {
			return raw
		}
	}
	if raw := ExtractJSON(resp.Text); raw != nil {
		return raw
	}
	return CloseJSON(resp.Text)
}

// CloseJSON closes a JSON object cut off mid-way (a reply that ran out of
// tokens): the open string, arrays and objects, dropping a dangling key
// or comma. An item of a list cut off goes, rather than closed half
// written (a suggestion without its "right" would delete words). nil
// when text has no object, the object is whole, or the result does not
// parse.
func CloseJSON(text string) json.RawMessage {
	i := strings.IndexByte(text, '{')
	if i < 0 {
		return nil
	}
	t := text[i:]
	stack, at, inStr, esc := openAt(t)
	if len(stack) == 0 && !inStr {
		return nil // balanced: ExtractJSON would have found it if it parsed
	}
	b := t
	if n := len(stack); n >= 2 && stack[n-1] == '{' && stack[n-2] == '[' {
		b, stack = b[:at[n-1]], stack[:n-1]
	} else {
		if esc {
			b = b[:len(b)-1]
		}
		if inStr {
			b += `"`
		}
	}
	// Try closing as is; else drop the last incomplete member.
	for range 6 {
		out := []byte(trimDangling(b))
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
		cut := lastComma(b)
		if cut < 0 {
			return nil
		}
		b = b[:cut]
		stack, _, _, _ = openAt(b)
	}
	return nil
}

// leftOpen reports whether text ends inside a JSON value: a bracket from
// its first one on is still open.
func leftOpen(text string) bool {
	i := strings.IndexAny(text, "{[")
	if i < 0 {
		return false
	}
	stack, _, _, _ := openAt(text[i:])
	return len(stack) > 0
}

// trimDangling removes a trailing comma or a key without a value.
func trimDangling(b string) string {
	s := strings.TrimRight(b, " \t\r\n")
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
	return s
}

// scan calls fn with the index and byte of each character of t outside
// its strings (their quotes excluded) until fn returns false. It reports
// whether t ends inside a string, and right after a backslash in it.
func scan(t string, fn func(j int, c byte) bool) (inStr, esc bool) {
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
		if c == '"' {
			inStr = true
			continue
		}
		if !fn(j, c) {
			break
		}
	}
	return inStr, esc
}

// balanced returns the end (exclusive) of the bracketed value starting at
// t[i], honouring strings, or -1.
func balanced(t string, i int) int {
	depth, end := 0, -1
	scan(t[i:], func(j int, c byte) bool {
		switch c {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				end = i + j + 1
				return false
			}
		}
		return true
	})
	return end
}

// openAt is the brackets open at the end of t, where each opened, and
// whether t ends inside a string (and after its backslash).
func openAt(t string) (stack []byte, at []int, inStr, esc bool) {
	inStr, esc = scan(t, func(j int, c byte) bool {
		switch c {
		case '{', '[':
			stack, at = append(stack, c), append(at, j)
		case '}', ']':
			if len(stack) > 0 {
				stack, at = stack[:len(stack)-1], at[:len(at)-1]
			}
		}
		return true
	})
	return stack, at, inStr, esc
}

// lastComma is the index of the last comma outside strings, or -1.
func lastComma(t string) int {
	last := -1
	scan(t, func(j int, c byte) bool {
		if c == ',' {
			last = j
		}
		return true
	})
	return last
}
