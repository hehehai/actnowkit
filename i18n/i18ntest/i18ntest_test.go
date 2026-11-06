package i18ntest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const pkg = "example.com/app/internal/i18n"

// module writes a small app module: files by slash path under a temp root.
func module(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, text := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

var appFiles = map[string]string{
	"go.mod": "module example.com/app\n",
	"main.go": `package main

import (
	"example.com/app/internal/i18n"
	tr "example.com/app/internal/i18n"
)

func main() {
	_ = i18n.T("Open")
	_ = i18n.T("%d files", 2)
	_ = tr.N(2, "%d take", "%d takes", 2)
	_ = i18n.X("verb", "Export")
	var about string
	_ = i18n.T(about)
	_ = i18n.N(1, about, "x")
	_ = i18n.X(about, "y")
	_ = i18n.Locale()
	_ = other.T("Not ours")
}
`,
	"main_test.go":                  "package main\n\nimport \"example.com/app/internal/i18n\"\n\nvar _ = i18n.T(\"Only in a test\")\n",
	"engine/x.go":                   "package engine\n\nimport \"example.com/app/internal/i18n\"\n\nvar _ = i18n.T(\"Skipped dir\")\n",
	"testdata/x.go":                 "package x\n\nimport \"example.com/app/internal/i18n\"\n\nvar _ = i18n.T(\"Testdata\")\n",
	".hidden/x.go":                  "package x\n\nimport \"example.com/app/internal/i18n\"\n\nvar _ = i18n.T(\"Hidden\")\n",
	"third_party/lib/go.mod":        "module lib\n",
	"third_party/lib/x.go":          "package lib\n\nimport \"example.com/app/internal/i18n\"\n\nvar _ = i18n.T(\"Nested module\")\n",
	"internal/ui/ui.go":             "package ui\n\nimport \"example.com/app/internal/i18n\"\n\nvar _ = i18n.T(\"Settings\")\n",
	"internal/noi18n/x.go":          "package noi18n\n\nfunc T(string) string { return \"\" }\n\nvar _ = T(\"Unqualified\")\n",
	"internal/i18n/locales/en.json": "{}\n",
	"internal/i18n/locales/zh-Hans.json": `{
  "Open": "打开",
  "%d files": "%d 个文件",
  "%d take": "%d 条",
  "%d takes": "%d 条",
  "verb\u0004Export": "导出",
  "Settings": "设置",
  "About text": "关于"
}
`,
}

func parity(root string) Parity {
	return Parity{
		Root:      root,
		Packages:  []string{pkg},
		Skip:      []string{"engine"},
		Dir:       filepath.Join(root, "internal/i18n/locales"),
		Supported: []string{"en", "zh-Hans"},
		Data:      map[string]string{"About text": "catalog.json"},
	}
}

func TestKeys(t *testing.T) {
	root := module(t, appFiles)
	keys, dynamic, err := parity(root).Keys()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for k := range keys {
		got = append(got, k)
	}
	slices.Sort(got)
	want := []string{"%d files", "%d take", "%d takes", "Open", "Settings", "verb\x04Export"}
	if !slices.Equal(got, want) {
		t.Errorf("keys %q, want %q", got, want)
	}
	if !strings.Contains(keys["Open"], "main.go:") {
		t.Errorf("position %q", keys["Open"])
	}
	if len(dynamic) != 3 {
		t.Errorf("dynamic %v", dynamic)
	}
}

func TestCheckClean(t *testing.T) {
	root := module(t, appFiles)
	problems, err := parity(root).Problems()
	if err != nil || len(problems) != 0 {
		t.Fatalf("%v %q", err, problems)
	}
	parity(root).Check(t)
}

func TestProblems(t *testing.T) {
	files := map[string]string{}
	for k, v := range appFiles {
		files[k] = v
	}
	files["internal/i18n/locales/en.json"] = `{"Open": "Open"}`
	files["internal/i18n/locales/zh-Hans.json"] = `{
  "Open": "",
  "%d files": "%s 个文件",
  "%d take": "%d 条",
  "%d takes": "%[1]d 条",
  "verb\u0004Export": "导出",
  "Settings": "设置",
  "Gone": "没了"
}`
	p := parity(module(t, files))
	got, err := p.Problems()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"en.json must stay empty (en is the source): 1 entries",
		`zh-Hans has "Gone", which nothing uses`,
		`zh-Hans lacks "About text" (catalog.json)`,
		`zh-Hans lacks "Open" (`,
		`zh-Hans: "%d files" → "%s 个文件": verbs [%d] ≠ [%s]`,
	}
	if len(got) != len(want) {
		t.Fatalf("problems:\n%s", strings.Join(got, "\n"))
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Errorf("problem %d: %q, want %q…", i, got[i], want[i])
		}
	}
}

func TestBrokenScan(t *testing.T) {
	p := parity(module(t, appFiles))
	p.Packages = []string{"example.com/other/i18n"}
	if _, err := p.Problems(); err == nil {
		t.Error("no messages found: no error")
	}
	p = parity(module(t, appFiles))
	p.MinKeys = 100
	if _, err := p.Problems(); err == nil || !strings.Contains(err.Error(), "at least 100") {
		t.Errorf("MinKeys: %v", err)
	}
	p = parity(module(t, appFiles))
	p.Supported = nil
	if _, err := p.Problems(); err == nil {
		t.Error("no languages: no error")
	}
	p = parity(module(t, map[string]string{"go.mod": "module x\n", "bad.go": "package x\nimport \"" + pkg + "\"\nfunc {"}))
	if _, _, err := p.Keys(); err == nil {
		t.Error("syntax error: no error")
	}
}

func TestSync(t *testing.T) {
	files := map[string]string{}
	for k, v := range appFiles {
		files[k] = v
	}
	files["internal/i18n/locales/zh-Hans.json"] = `{"Open": "打开", "Gone": "没了"}`
	root := module(t, files)
	p := parity(root)
	newer := filepath.Join(root, "new.json")
	if err := os.WriteFile(newer, []byte(`{"Settings": "设置", "%d take": "", "Unused": "x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "missing.json")
	report, err := p.Sync(missing, newer, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 1 || report[0] != "zh-Hans: 2 messages, 5 missing" {
		t.Errorf("report %q", report)
	}
	read := func(path string) map[string]string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]string{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	if m := read(filepath.Join(p.Dir, "zh-Hans.json")); len(m) != 2 || m["Open"] != "打开" || m["Settings"] != "设置" {
		t.Errorf("zh-Hans %v", m)
	}
	if m := read(missing); len(m) != 5 || m["About text"] != "" {
		t.Errorf("missing %v", m)
	}
	if _, err := p.Sync("", filepath.Join(root, "nope.json")); err == nil {
		t.Error("missing from file: no error")
	}
}

func TestVerbs(t *testing.T) {
	if got := Verbs("%[2]s of %[1]d, %.1f%% done, %-5v"); !slices.Equal(got, []string{"%-5v", "%.1f", "%d", "%s"}) {
		t.Error(got)
	}
	if got := Verbs("100%%"); got != nil {
		t.Error(got)
	}
}

func TestStyle(t *testing.T) {
	got := Style(map[string]string{
		"Copy":     "复制",
		"Loading…": "正在载入...",
		"Name":     `名称 "a"`,
		"OK":       "好",
		"English":  "Touch ID", // no Chinese: not checked
	}, ZhHansRules)
	if len(got) != 3 || !strings.Contains(got[0], "拷贝") {
		t.Errorf("%q", got)
	}
	p := parity(module(t, appFiles))
	if probs, err := p.StyleProblems("zh-Hans", ZhHansRules); err != nil || len(probs) != 0 {
		t.Errorf("%v %q", err, probs)
	}
}
