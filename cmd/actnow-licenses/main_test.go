package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hehehai/actnowkit/thirdparty"
)

const (
	mitText = "MIT License\n\nCopyright (c) 2026 Someone\n\nPermission is hereby granted, free of charge, to any person obtaining a copy\nof this software and associated documentation files"
	apache  = "                                 Apache License\n                           Version 2.0, January 2004\n"
	bsd3    = "Redistribution and use in source and binary forms, with or without modification... Neither the name of the copyright holder"
	gplText = "GNU GENERAL PUBLIC LICENSE\n Version 2, June 1991"
	lgpl    = "GNU LESSER GENERAL PUBLIC LICENSE\nVersion 2.1, February 1999"
)

// write writes files by slash path under root.
func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Licence files are copied whatever their extension; source files named
// like one (actnowkit's licence/licence_test.go) are not.
func TestIsLicenceFile(t *testing.T) {
	for name, want := range map[string]bool{
		"LICENSE": true, "LICENSE.txt": true, "LICENSE.md": true, "LICENSE-MIT": true,
		"LICENSE.APACHE2": true, "licence": true, "COPYING.LIB": true, "NOTICE": true,
		"NOTICE.txt": true, "PATENTS": true, "ACKNOWLEDGEMENTS.md": true, "Acknowledgments": true,
		"third_party_notices.txt": true, "UNLICENSE": true,
		"licence_test.go": false, "licence.go": false, "License.swift": false,
		"license.h": false, "notice.py": false, "README.md": false, "licensing.md": false,
	} {
		if got := isLicenceFile(name); got != want {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
	}
}

func TestLicenceLine(t *testing.T) {
	cases := []struct {
		name string
		c    component
		want string // "" for an error
		err  string
	}{
		{"own", component{files: []licFile{{"LICENSE", mitText, true}}}, "MIT", ""},
		{"vendored", component{files: []licFile{{"LICENSE", mitText, true}, {"vendor/x/LICENSE", apache, false}, {"vendor/y/LICENSE", mitText, false}}}, "MIT (also Apache-2.0)", ""},
		{"notice quotes others", component{files: []licFile{{"LICENSE", apache, true}, {"NOTICE", bsd3, true}}}, "Apache-2.0 (also BSD-3-Clause)", ""},
		{"unknown notice", component{files: []licFile{{"LICENSE", apache, true}, {"NOTICE", "Copyright Foo", true}}}, "Apache-2.0", ""},
		{"named in extras", component{licence: "Solar-Icons", files: []licFile{{"Solar.txt", "use them freely", true}}}, "Solar-Icons", ""},
		{"program", component{program: true, licence: "LGPL-2.1-or-later", files: []licFile{{"COPYING", lgpl, true}}}, "LGPL-2.1-or-later", ""},
		{"unknown", component{name: "x", files: []licFile{{"LICENSE", "Do what you like", true}}}, "", "cannot tell the licence of LICENSE"},
		{"gpl", component{name: "x", files: []licFile{{"LICENSE", mitText, true}, {"sub/COPYING", gplText, false}}}, "", "sub/COPYING is under GPL"},
		{"no root licence", component{name: "x", files: []licFile{{"sub/LICENSE", mitText, false}}}, "", "no licence at its root"},
	}
	for _, c := range cases {
		got, err := licenceLine(c.c)
		switch {
		case c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)):
			t.Errorf("%s: %q, %v; want error %q", c.name, got, err, c.err)
		case c.err == "" && (err != nil || got != c.want):
			t.Errorf("%s: %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"ok.json":      `{"app": "A", "out": "o", "header": "h", "regenerate": "r"}`,
		"missing.json": `{"app": "A", "out": "o"}`,
		"unknown.json": `{"app": "A", "out": "o", "header": "h", "regenerate": "r", "programz": "x"}`,
		"script.json":  `{"app": "A", "out": "o", "header": "h", "regenerate": "r", "swift": {"scripts": [{"identity": "x"}]}}`,
		"npm.json":     `{"app": "A", "out": "o", "header": "h", "regenerate": "r", "npm": {"list": "l"}}`,
		"bad.json":     `{`,
	})
	c, err := loadConfig(filepath.Join(dir, "ok.json"))
	if err != nil || !slices.Equal(c.Go.Packages, []string{"."}) {
		t.Errorf("ok: %+v %v", c, err)
	}
	for _, name := range []string{"missing", "unknown", "script", "npm", "bad", "nope"} {
		if _, err := loadConfig(filepath.Join(dir, name+".json")); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestScriptPins(t *testing.T) {
	root := t.TempDir()
	sha := strings.Repeat("ab", 20)
	write(t, root, map[string]string{
		"a.sh": "#!/bin/sh\nUPSTREAM=\"https://github.com/o/a.git\"\nCOMMIT=\"" + sha + "\"\n",
		"b.sh": "B_URL=\"https://github.com/o/b\"\nB_SHA=\"" + sha + "\"\n",
		"c.sh": "UPSTREAM=\"https://x\"\nCOMMIT=\"short\"\n",
	})
	g := &gen{cfg: &config{}, root: root}
	g.cfg.Swift.Resolved = "missing/Package.resolved" // none: an engine without packages
	g.cfg.Swift.Scripts = []scriptPin{{Identity: "b", File: "b.sh", Upstream: "B_URL", Commit: "B_SHA"}, {Identity: "a", File: "a.sh"}}
	pins, err := g.swiftPins()
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 2 || pins[0] != (swiftPin{"a", "https://github.com/o/a.git", sha, "abababa"}) || pins[1].location != "https://github.com/o/b" {
		t.Errorf("%+v", pins)
	}
	for _, s := range []scriptPin{{Identity: "c", File: "c.sh"}, {Identity: "d", File: "d.sh"}} {
		g.cfg.Swift.Scripts = []scriptPin{s}
		if _, err := g.swiftPins(); err == nil {
			t.Errorf("%s: no error", s.File)
		}
	}
	write(t, root, map[string]string{"missing/Package.resolved": "{"})
	g.cfg.Swift.Scripts = nil
	if _, err := g.swiftPins(); err == nil {
		t.Error("bad Package.resolved: no error")
	}
}

func TestExtras(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"lic/programs.json": `[{"Name": "tool", "Files": ["COPYING"]}]`,
		"lic/COPYING":       gplText,
		"lic/extras.json":   `[{"Name": "x", "Files": ["nope.txt"]}]`,
		"lic/bad.json":      `{}`,
	})
	g := &gen{cfg: &config{}, root: root}
	if comps, err := g.extraComponents("", false); err != nil || comps != nil {
		t.Errorf("none: %v %v", comps, err)
	}
	for _, c := range []struct {
		path    string
		program bool
	}{{"lic/programs.json", true}, {"lic/extras.json", false}, {"lic/bad.json", false}, {"lic/nope.json", false}} {
		if _, err := g.extraComponents(c.path, c.program); err == nil {
			t.Errorf("%s: no error", c.path)
		}
	}
	g.cfg.NPM.List = "npm.json"
	g.cfg.NPM.Use = "In the extension."
	write(t, root, map[string]string{"npm.json": `[{"Name": "left-pad", "Version": "1.0.0", "Files": []}]`})
	if _, err := g.npmComponents(); err == nil || !strings.Contains(err.Error(), "no licence text") {
		t.Errorf("npm without texts: %v", err)
	}
}

func TestCheckFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, thirdparty.FileName)
	comps := []component{
		{inventory: "a", name: "a", source: "https://a", files: []licFile{{"LICENSE", mitText, true}}},
		{inventory: "p", name: "tool", source: "https://t", licence: "GPL-2.0", program: true, files: []licFile{{"COPYING", gplText, true}}},
	}
	text, err := render("App — notices", comps, inventory(comps))
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, map[string]string{thirdparty.FileName: strings.ReplaceAll(text, "\n", "\r\n")})
	if err := checkFile(path, "App — notices", inventory(comps)); err != nil {
		t.Errorf("fresh (CRLF): %v", err)
	}
	if err := checkFile(path, "App — new notices", inventory(comps)); err == nil || !strings.Contains(err.Error(), "header") {
		t.Errorf("header changed: %v", err)
	}
	if err := checkFile(path, "App", inventory(comps)); err == nil {
		t.Error("a header that is a prefix of the line: no error")
	}
	if err := checkFile(path, "App — notices", inventory(comps[:1])); err == nil || !strings.Contains(err.Error(), "dependencies changed") {
		t.Errorf("inventory changed: %v", err)
	}
	write(t, dir, map[string]string{thirdparty.FileName: strings.Replace(text, "Licence: MIT", "Licence: MIT (also LGPL)", 1)})
	if err := checkFile(path, "App — notices", inventory(comps)); err == nil || !strings.Contains(err.Error(), "LGPL") {
		t.Errorf("linked LGPL: %v", err)
	}
	if err := checkFile(filepath.Join(dir, "nope"), "x", "y"); err == nil {
		t.Error("no file: no error")
	}
}

// git runs git in dir.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "init.defaultBranch=main", "-c", "protocol.file.allow=always"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestEndToEnd generates and checks the file of a small app: a Go module
// linked through a replace, a Swift package in a local git repository, a
// pinned one in SwiftPM's checkouts, npm packages, extras and a program.
func TestEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go command")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	base := t.TempDir()
	root, cache := filepath.Join(base, "app"), filepath.Join(base, "cache")

	// A Swift package in a git repository, cloned into the cache.
	pkg := filepath.Join(base, "swiftpkg")
	write(t, pkg, map[string]string{
		"LICENSE":                  mitText,
		"Sources/C/vendor/LICENSE": apache,
		"Sources/C/License.swift":  "// code",
		"Tests/LICENSE":            gplText, // tests are left out
	})
	git(t, pkg, "init", "-q")
	git(t, pkg, "config", "uploadpack.allowAnySHA1InWant", "true")
	git(t, pkg, "add", ".")
	git(t, pkg, "commit", "-q", "-m", "x")
	rev := git(t, pkg, "rev-parse", "HEAD")
	// Another, already in SwiftPM's checkouts at its pin.
	co := filepath.Join(root, "engine/.build/checkouts/other")
	write(t, co, map[string]string{"LICENSE.txt": bsd3})
	git(t, co, "init", "-q")
	git(t, co, "add", ".")
	git(t, co, "commit", "-q", "-m", "x")
	otherRev := git(t, co, "rev-parse", "HEAD")

	write(t, root, map[string]string{
		"go.mod":                             "module example.com/app\n\ngo 1.22\n\nrequire example.com/dep v0.1.0\n\nreplace example.com/dep => ./third_party/dep\n",
		"main.go":                            "package main\n\nimport _ \"example.com/dep/sub\"\n\nfunc main() {}\n",
		"third_party/dep/go.mod":             "module example.com/dep\n\ngo 1.22\n",
		"third_party/dep/LICENSE":            mitText,
		"third_party/dep/VENDORED":           "dep v0.1.0 + 2 patches\n",
		"third_party/dep/sub/sub.go":         "package sub\n",
		"third_party/dep/sub/NOTICE":         "Sub includes things.",
		"third_party/dep/sub/licence.go":     "package sub\n",
		"third_party/dep/unused/LICENSE":     gplText, // not linked
		"third_party/dep/unused/x.go":        "package unused\n",
		"engine/Package.resolved":            `{"pins": [{"identity": "other", "location": "https://github.com/o/other.git", "state": {"revision": "` + otherRev + `", "version": "1.2.3"}}]}`,
		"scripts/fetch.sh":                   "UPSTREAM=\"" + pkg + "\"\nCOMMIT=\"" + rev + "\"\n",
		"extension/licenses.json":            `[{"Name": "left-pad", "Version": "1.0.0", "Source": "https://npm/left-pad", "Files": [{"Name": "LICENSE", "Text": "` + strings.ReplaceAll(mitText, "\n", `\n`) + `"}]}]`,
		"third_party/licenses/extras.json":   `[{"Name": "Icons", "Source": "https://icons", "Use": "The icons.", "Licence": "Icons-1.0", "Files": ["icons.txt"]}]`,
		"third_party/licenses/icons.txt":     "Use them freely.",
		"third_party/licenses/programs.json": `[{"Name": "tool", "Version": "7", "Source": "https://tool", "Licence": "LGPL-2.1-or-later", "Files": ["COPYING"]}]`,
		"third_party/licenses/COPYING":       lgpl,
		"third_party/licenses/header.txt":    "App — third-party software notices\n\nGenerated by scripts/gen-licenses.sh; do not edit.\n",
		"third_party/licenses/actnow-licenses.json": `{
  "app": "App",
  "out": "resources/ThirdPartyLicenses.txt",
  "header": "third_party/licenses/header.txt",
  "regenerate": "scripts/gen-licenses.sh",
  "go": {"patched": {"example.com/dep": "third_party/dep/VENDORED"}},
  "swift": {
    "resolved": "engine/Package.resolved",
    "checkouts": "engine/.build/checkouts",
    "scripts": [{"identity": "swiftpkg", "file": "scripts/fetch.sh"}],
    "patched": ["swiftpkg"]
  },
  "npm": {"list": "extension/licenses.json", "use": "In the extension."},
  "extras": "third_party/licenses/extras.json",
  "programs": "third_party/licenses/programs.json"
}`,
	})
	if err := os.MkdirAll(filepath.Join(root, "resources"), 0o755); err != nil {
		t.Fatal(err)
	}
	const cfg = "third_party/licenses/actnow-licenses.json"

	if _, err := run(root, cfg, cache, true); err == nil {
		t.Fatal("check before the file exists: no error")
	}
	msg, err := run(root, cfg, cache, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "7 components") {
		t.Errorf("message %q", msg)
	}
	if msg, err := run(root, cfg, cache, true); err != nil || !strings.Contains(msg, "matches") {
		t.Fatalf("check: %q %v", msg, err)
	}
	pre, comps, err := thirdparty.Load(filepath.Join(root, "resources"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pre, "App — third-party") || !strings.Contains(pre, "Inventory: ") || !strings.Contains(pre, "  example.com/dep v0.1.0 (with App's patches) — MIT\n") {
		t.Errorf("preamble:\n%s", pre)
	}
	var names []string
	byName := map[string]thirdparty.Component{}
	for _, c := range comps {
		names = append(names, c.Name)
		byName[c.Name] = c
	}
	if want := []string{"Go", "example.com/dep", "other", "swiftpkg", "left-pad", "Icons", "tool"}; !slices.Equal(names, want) {
		t.Fatalf("components %v, want %v", names, want)
	}
	if c := byName["Go"]; !strings.Contains(c.Use, "compiled into App.") || !strings.Contains(c.Text, "----- LICENSE -----") {
		t.Errorf("Go: %+v", c.Use)
	}
	if c := byName["example.com/dep"]; c.Licence != "MIT" || !strings.Contains(c.Text, "----- sub/NOTICE -----") || strings.Contains(c.Text, "licence.go") || strings.Contains(c.Text, "unused") {
		t.Errorf("dep: %s\n%s", c.Licence, c.Text)
	}
	if c := byName["swiftpkg"]; c.Licence != "MIT (also Apache-2.0)" || c.Version != rev[:7]+" (with App's patches)" || strings.Contains(c.Text, "Tests/") || strings.Contains(c.Text, "License.swift") {
		t.Errorf("swiftpkg: %+v", c)
	}
	if c := byName["other"]; c.Licence != "BSD-3-Clause" || c.Version != "1.2.3" || c.Source != "https://github.com/o/other" {
		t.Errorf("other: %+v", c)
	}
	if c := byName["left-pad"]; c.Licence != "MIT" || c.Use != "In the extension." {
		t.Errorf("npm: %+v", c)
	}
	if c := byName["Icons"]; c.Licence != "Icons-1.0" || c.Program() {
		t.Errorf("extra: %+v", c)
	}
	if c := byName["tool"]; c.Licence != "LGPL-2.1-or-later" || !c.Program() {
		t.Errorf("program: %+v", c)
	}
	if _, err := os.Stat(filepath.Join(cache, "swiftpkg@"+rev[:12], ".actnow-licenses-ok")); err != nil {
		t.Errorf("the clone is not cached: %v", err)
	}

	// The cached clone is used again; the header and the inventory are checked.
	if _, err := run(root, cfg, cache, false); err != nil {
		t.Fatal(err)
	}
	write(t, root, map[string]string{"third_party/licenses/header.txt": "App — notices, reworded\n"})
	if _, err := run(root, cfg, cache, true); err == nil || !strings.Contains(err.Error(), "header changed") || !strings.Contains(err.Error(), "run scripts/gen-licenses.sh") {
		t.Errorf("header changed: %v", err)
	}
	if _, err := run(root, cfg, cache, false); err != nil {
		t.Fatal(err)
	}
	write(t, root, map[string]string{"third_party/dep/VENDORED": "dep v0.1.0 + 3 patches\n"})
	if _, err := run(root, cfg, cache, true); err == nil || !strings.Contains(err.Error(), "dependencies changed") {
		t.Errorf("patches changed: %v", err)
	}
	// A GPL text in what is linked in stops the generator.
	write(t, root, map[string]string{"third_party/dep/sub/COPYING": gplText})
	if _, err := run(root, cfg, cache, false); err == nil || !strings.Contains(err.Error(), "GPL") {
		t.Errorf("GPL linked in: %v", err)
	}
}
