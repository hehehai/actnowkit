package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// swiftPin is a Swift package at a revision.
type swiftPin struct {
	identity, location, revision, version string
}

// skipSwiftDirs are not built into the engine.
var skipSwiftDirs = []string{".git", ".github", ".swiftpm", ".build", "Tests", "tests", "test", "Examples", "examples", "Benchmarks", "benchmarks", "docs", "Docs", "Documentation"}

// swiftComponents are the engine's packages.
func (g *gen) swiftComponents() ([]component, error) {
	pins, err := g.swiftPins()
	if err != nil {
		return nil, err
	}
	var comps []component
	for _, p := range pins {
		c := component{
			inventory: "swift " + p.identity + " " + p.revision,
			name:      strings.TrimSuffix(filepath.Base(p.location), ".git"),
			version:   p.version,
			source:    strings.TrimSuffix(p.location, ".git"),
		}
		if slices.Contains(g.cfg.Swift.Patched, p.identity) {
			c.version += " (with " + g.cfg.App + "'s patches)"
		}
		if g.fetch {
			dir, err := g.swiftCheckout(p)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p.identity, err)
			}
			if c.files, err = treeLicences(dir); err != nil {
				return nil, err
			}
			if !slices.ContainsFunc(c.files, func(f licFile) bool { return f.root }) {
				return nil, fmt.Errorf("%s has no licence file at its root", p.identity)
			}
		}
		comps = append(comps, c)
	}
	return comps, nil
}

// swiftPins reads the Package.resolved (none without it) and the scripts'
// pins, sorted by identity.
func (g *gen) swiftPins() ([]swiftPin, error) {
	var pins []swiftPin
	if g.cfg.Swift.Resolved != "" {
		data, err := os.ReadFile(abs(g.root, g.cfg.Swift.Resolved))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if err == nil {
			var resolved struct {
				Pins []struct {
					Identity, Location string
					State              struct{ Revision, Version string }
				}
			}
			if err := json.Unmarshal(data, &resolved); err != nil {
				return nil, fmt.Errorf("%s: %w", g.cfg.Swift.Resolved, err)
			}
			for _, p := range resolved.Pins {
				v := p.State.Version
				if v == "" {
					v = p.State.Revision[:min(7, len(p.State.Revision))]
				}
				pins = append(pins, swiftPin{p.Identity, p.Location, p.State.Revision, v})
			}
		}
	}
	for _, s := range g.cfg.Swift.Scripts {
		p, err := g.scriptPin(s)
		if err != nil {
			return nil, err
		}
		pins = append(pins, p)
	}
	sort.Slice(pins, func(i, j int) bool { return pins[i].identity < pins[j].identity })
	return pins, nil
}

// scriptPin reads the url and commit a script checks out.
func (g *gen) scriptPin(s scriptPin) (swiftPin, error) {
	script, err := os.ReadFile(abs(g.root, s.File))
	if err != nil {
		return swiftPin{}, err
	}
	upName, commitName := cmp(s.Upstream, "UPSTREAM"), cmp(s.Commit, "COMMIT")
	up := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(upName) + `="([^"]+)"`).FindSubmatch(script)
	commit := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(commitName) + `="([0-9a-f]{40})"`).FindSubmatch(script)
	if up == nil || commit == nil {
		return swiftPin{}, fmt.Errorf("%s: no %s=\"url\" or %s=\"sha\" line", s.File, upName, commitName)
	}
	return swiftPin{s.Identity, string(up[1]), string(commit[1]), string(commit[1][:7])}, nil
}

func cmp(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// swiftCheckout is a checkout of p: SwiftPM's when at the revision, else
// a clone in the cache (with submodules).
func (g *gen) swiftCheckout(p swiftPin) (string, error) {
	if g.cfg.Swift.Checkouts != "" {
		if dir := filepath.Join(abs(g.root, g.cfg.Swift.Checkouts), p.identity); revision(dir) == p.revision && !hasEmptySubmodules(dir) {
			return dir, nil
		}
	}
	dir := filepath.Join(g.cache, p.identity+"@"+p.revision[:min(12, len(p.revision))])
	stamp := filepath.Join(dir, ".actnow-licenses-ok")
	if _, err := os.Stat(stamp); err == nil {
		return dir, nil
	}
	fmt.Fprintf(os.Stderr, "fetching %s %s\n", p.identity, p.version)
	os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	steps := [][]string{
		{"init", "-q"},
		{"remote", "add", "origin", p.location},
		{"fetch", "-q", "--depth", "1", "origin", p.revision},
		{"checkout", "-q", "--detach", "FETCH_HEAD"},
		{"submodule", "update", "-q", "--init", "--recursive", "--depth", "1"},
	}
	for _, s := range steps {
		cmd := exec.Command("git", s...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git %s: %v\n%s", strings.Join(s, " "), err, out)
		}
	}
	return dir, os.WriteFile(stamp, nil, 0o644)
}

// revision is the commit checked out in dir, "" when it is not a checkout.
func revision(dir string) string {
	if _, err := os.Stat(dir); err != nil {
		return ""
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// hasEmptySubmodules reports whether a submodule of dir is not checked out.
func hasEmptySubmodules(dir string) bool {
	out, err := exec.Command("git", "-C", dir, "submodule", "status", "--recursive").Output()
	if err != nil {
		return true
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "-") {
			return true
		}
	}
	return false
}

// treeLicences are the licence files anywhere in dir but tests, examples
// and docs; those at its root are root.
func treeLicences(dir string) ([]licFile, error) {
	var files []licFile
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && slices.Contains(skipSwiftDirs, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !isLicenceFile(d.Name()) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		files = append(files, licFile{label: filepath.ToSlash(rel), text: string(data), root: filepath.Dir(rel) == "."})
		return nil
	})
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].root != files[j].root {
			return files[i].root
		}
		return files[i].label < files[j].label
	})
	return files, err
}
