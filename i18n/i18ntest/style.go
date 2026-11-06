package i18ntest

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ZhHansRules are the family's conventions for Simplified Chinese copy:
// macOS's own terms (拷贝, 聚焦, 触控 ID, Apple 智能, 废纸篓), the standard
// spellings (账户, 撤销), and Chinese punctuation (… not "...").
var ZhHansRules = []StyleRule{
	{"复制", "拷贝 (macOS's word)"},
	{"Apple Intelligence", "Apple 智能"},
	{"Touch ID", "触控 ID"},
	{"Spotlight", "聚焦"},
	{"...", "…"},
	{"帐户", "账户"},
	{"回收站", "废纸篓"},
	{"撤消", "撤销"},
	{`"`, "「」"},
}

// StyleRule is a text a translation should not have, and what to write
// instead.
type StyleRule struct{ Bad, Want string }

var han = regexp.MustCompile(`\p{Han}`)

// Style lists the translations of a catalogue (key → translation) with
// Chinese in them that break a rule, sorted.
func Style(messages map[string]string, rules []StyleRule) []string {
	var out []string
	for k, v := range messages {
		if !han.MatchString(v) {
			continue
		}
		for _, r := range rules {
			if strings.Contains(v, r.Bad) {
				out = append(out, fmt.Sprintf("%q → %q: %q, use %s", k, v, r.Bad, r.Want))
			}
		}
	}
	sort.Strings(out)
	return out
}

// StyleProblems is Style for Dir/<tag>.json.
func (p Parity) StyleProblems(tag string, rules []StyleRule) ([]string, error) {
	m, err := p.readLocale(tag)
	if err != nil {
		return nil, err
	}
	return Style(m, rules), nil
}
