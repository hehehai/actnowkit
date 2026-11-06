// Package i18ntest checks an app's catalogues (package i18n) against its
// sources, from the app's own test:
//
//	func TestKeyParity(t *testing.T) { parity.Check(t) }
//
//	var parity = i18ntest.Parity{
//		Root:      "../..",                                        // the module
//		Packages:  []string{"github.com/hehehai/Dicta/internal/i18n"},
//		Skip:      []string{"engine"},
//		Dir:       "locales",
//		Supported: i18n.Supported(),
//	}
//
// Keys scans every non-test .go file of the module for calls to the T, N
// and X functions of Packages whose message arguments are string literals
// (nested modules, hidden, build, testdata, vendor and node_modules
// directories left out). Check fails when a supported language's
// catalogue lacks one of those keys (or of Data's), keeps one nothing
// uses, or formats it with other verbs (a translation may reorder them
// with %[2]s), and when the source language's catalogue has entries. Sync
// rewrites the catalogues from the sources.
package i18ntest

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Parity says what to scan and which catalogues to check.
type Parity struct {
	// Root is the module's root directory (from the test's package:
	// "../..").
	Root string
	// Packages are the import paths of the app's i18n packages whose T, N
	// and X calls are scanned.
	Packages []string
	// Skip are directories under Root not scanned, slash-separated and
	// relative to it ("engine", "swift/engine").
	Skip []string
	// Dir holds the catalogues, <tag>.json (relative to the test's
	// package: "locales").
	Dir string
	// Supported are the UI languages, the source language first.
	Supported []string
	// Data are messages the UI translates that the sources do not spell
	// out (a catalogue's texts, read from its JSON), each with where it
	// comes from.
	Data map[string]string
	// MinKeys is the fewest messages a working scan finds (default 1).
	MinKeys int
}

// Keys scans the sources: the catalogue keys they need, each with a
// position, and the positions of calls with computed messages
// (i18n.T(entry.About)), whose texts must come from Data.
func (p Parity) Keys() (keys map[string]string, dynamic []string, err error) {
	keys = map[string]string{}
	fset := token.NewFileSet()
	root := filepath.Clean(p.Root)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			switch name := d.Name(); {
			case strings.HasPrefix(name, "."), name == "build", name == "testdata", name == "vendor", name == "node_modules",
				slices.Contains(p.Skip, filepath.ToSlash(rel)):
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir // another module
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution|parser.ImportsOnly)
		if err != nil {
			return err
		}
		locals := p.localNames(f)
		if len(locals) == 0 {
			return nil
		}
		if f, err = parser.ParseFile(fset, path, nil, parser.SkipObjectResolution); err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); !ok || !slices.Contains(locals, id.Name) {
				return true
			}
			pos := fset.Position(call.Pos()).String()
			lit := func(i int) (string, bool) {
				if i >= len(call.Args) {
					return "", false
				}
				bl, ok := call.Args[i].(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					return "", false
				}
				s, err := strconv.Unquote(bl.Value)
				return s, err == nil
			}
			switch sel.Sel.Name {
			case "T":
				if s, ok := lit(0); ok {
					keys[s] = pos
				} else {
					dynamic = append(dynamic, pos)
				}
			case "N":
				one, ok1 := lit(1)
				other, ok2 := lit(2)
				if ok1 && ok2 {
					keys[one] = pos
					keys[other] = pos
				} else {
					dynamic = append(dynamic, pos)
				}
			case "X":
				ctx, ok1 := lit(0)
				msg, ok2 := lit(1)
				if ok1 && ok2 {
					keys[ctx+"\x04"+msg] = pos
				} else {
					dynamic = append(dynamic, pos)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return keys, dynamic, nil
}

// localNames are the names f imports the scanned packages under.
func (p Parity) localNames(f *ast.File) []string {
	var out []string
	for _, imp := range f.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		if !slices.Contains(p.Packages, path) {
			continue
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name != "_" && name != "." {
			out = append(out, name)
		}
	}
	return out
}

// allKeys are the sources' keys and Data's.
func (p Parity) allKeys() (map[string]string, []string, error) {
	keys, dynamic, err := p.Keys()
	if err != nil {
		return nil, nil, err
	}
	min := max(p.MinKeys, 1)
	if len(keys) < min {
		return nil, nil, fmt.Errorf("found only %d messages under %s, want at least %d: is the scan broken (Root, Packages)?", len(keys), p.Root, min)
	}
	for k, from := range p.Data {
		if _, ok := keys[k]; !ok {
			keys[k] = from
		}
	}
	return keys, dynamic, nil
}

// readLocale reads Dir/<tag>.json; a missing file is empty.
func (p Parity) readLocale(tag string) (map[string]string, error) {
	m := map[string]string{}
	data, err := os.ReadFile(filepath.Join(p.Dir, tag+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s.json: %w", tag, err)
	}
	return m, nil
}

// Problems lists what Check reports, sorted; err is a scan or file that
// failed.
func (p Parity) Problems() (problems []string, err error) {
	if len(p.Supported) == 0 {
		return nil, errors.New("i18ntest: no supported language")
	}
	keys, _, err := p.allKeys()
	if err != nil {
		return nil, err
	}
	src, err := p.readLocale(p.Supported[0])
	if err != nil {
		return nil, err
	}
	if len(src) != 0 {
		problems = append(problems, fmt.Sprintf("%s.json must stay empty (%s is the source): %d entries", p.Supported[0], p.Supported[0], len(src)))
	}
	for _, tag := range p.Supported[1:] {
		m, err := p.readLocale(tag)
		if err != nil {
			return nil, err
		}
		var out []string
		for k, pos := range keys {
			v := m[k]
			switch {
			case v == "":
				out = append(out, fmt.Sprintf("%s lacks %s (%s)", tag, strconv.Quote(k), pos))
			case !slices.Equal(Verbs(k), Verbs(v)):
				out = append(out, fmt.Sprintf("%s: %q → %q: verbs %v ≠ %v", tag, k, v, Verbs(k), Verbs(v)))
			}
		}
		for k := range m {
			if _, ok := keys[k]; !ok {
				out = append(out, fmt.Sprintf("%s has %q, which nothing uses", tag, k))
			}
		}
		sort.Strings(out)
		problems = append(problems, out...)
	}
	return problems, nil
}

// Check fails t for every problem (Problems), and logs the calls with
// computed messages.
func (p Parity) Check(t testing.TB) {
	t.Helper()
	if _, dynamic, err := p.Keys(); err == nil {
		for _, pos := range dynamic {
			t.Logf("computed message at %s: its texts must come from Data", pos)
		}
	}
	problems, err := p.Problems()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range problems {
		t.Error(s)
	}
}

// Sync rewrites Dir/<tag>.json of every supported language but the source
// one from the sources: it keeps the translations still used, drops the
// others, takes missing ones from the JSON files from (e.g. a file of new
// translations), and writes the keys still missing to missing (a JSON
// object of key → "", to translate and pass in from next time; one
// language's, the last, when there are several). It returns how many
// messages each language has and lacks.
func (p Parity) Sync(missing string, from ...string) (report []string, err error) {
	if len(p.Supported) == 0 {
		return nil, errors.New("i18ntest: no supported language")
	}
	keys, _, err := p.allKeys()
	if err != nil {
		return nil, err
	}
	extra := map[string]string{}
	for _, f := range from {
		if f == "" {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		for k, v := range m {
			if v != "" {
				extra[k] = v
			}
		}
	}
	for _, tag := range p.Supported[1:] {
		cur, err := p.readLocale(tag)
		if err != nil {
			return nil, err
		}
		next, lack := map[string]string{}, map[string]string{}
		for k := range keys {
			switch {
			case cur[k] != "":
				next[k] = cur[k]
			case extra[k] != "":
				next[k] = extra[k]
			default:
				lack[k] = ""
			}
		}
		if err := writeJSON(filepath.Join(p.Dir, tag+".json"), next); err != nil {
			return nil, err
		}
		if missing != "" {
			if err := writeJSON(missing, lack); err != nil {
				return nil, err
			}
		}
		report = append(report, fmt.Sprintf("%s: %d messages, %d missing", tag, len(next), len(lack)))
	}
	return report, nil
}

// writeJSON writes m as the catalogues are: indented, keys sorted, no
// HTML escapes.
func writeJSON(path string, m map[string]string) error {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

var verbRE = regexp.MustCompile(`%(?:\[\d+\])?[-+# 0]*\d*(?:\.\d+)?[a-zA-Z%]`)

// Verbs lists the formatting verbs of s, without argument indexes, sorted:
// a translation may reorder its arguments with %[2]s but must use the
// same ones.
func Verbs(s string) []string {
	var out []string
	for _, v := range verbRE.FindAllString(s, -1) {
		if v == "%%" {
			continue
		}
		if i := strings.Index(v, "]"); i > 0 {
			v = "%" + v[i+1:]
		}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
