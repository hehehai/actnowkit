package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// licenceName matches the files gen copies: licences, notices, patent
// grants and acknowledgements. isLicenceFile also leaves out source files
// whose names start the same way.
var licenceName = regexp.MustCompile(`(?i)^(licen[cs]e|copying|unlicense|notice|patents|acknowledge?ments|third[-_]party[-_]notices)([._-].*)?$`)

// noticeName matches the files that need not be a licence gen knows.
var noticeName = regexp.MustCompile(`(?i)^(notice|patents|acknowledge?ments|third[-_]party[-_]notices)([._-].*)?$`)

// sourceName is a program's source file: actnowkit's licence package has
// licence.go and licence_test.go, which are code, not licences.
var sourceName = regexp.MustCompile(`(?i)\.(go|swift|c|cc|cpp|cxx|h|hh|hpp|hxx|m|mm|s|py|js|mjs|ts|rs|java|kt|rb|sh|pl|php|cs)$`)

// isLicenceFile says whether gen copies the file called name.
func isLicenceFile(name string) bool {
	return licenceName.MatchString(name) && !sourceName.MatchString(name)
}

// goModule is a module linked into the app.
type goModule struct {
	path, version, dir string
	pkgDirs            map[string]bool // the directories of its packages linked
}

// goComponents are the Go toolchain and the modules linked into the
// darwin/arm64 app.
func (g *gen) goComponents() ([]component, error) {
	goVersion, err := g.goDirective()
	if err != nil {
		return nil, err
	}
	toolchain := component{
		inventory: "go-toolchain " + goVersion,
		name:      "Go",
		version:   goVersion,
		source:    "https://go.dev",
		use:       "The Go runtime and standard library, compiled into " + g.cfg.App + ".",
	}
	if g.fetch {
		goroot, err := g.goOutput(nil, "env", "GOROOT")
		if err != nil {
			return nil, err
		}
		files, err := dirLicences(strings.TrimSpace(goroot), "", true)
		if err != nil || len(files) == 0 {
			return nil, fmt.Errorf("the Go toolchain's LICENSE: %v", err)
		}
		toolchain.files = files
	}
	mods, err := g.linkedModules()
	if err != nil {
		return nil, err
	}
	comps := []component{toolchain}
	for _, m := range mods {
		c := component{
			inventory: "go " + m.path + " " + m.version,
			name:      m.path,
			version:   m.version,
			source:    "https://" + m.path,
		}
		if stamp, ok := g.cfg.Go.Patched[m.path]; ok {
			// A vendored copy with the app's patches.
			if stamp != "" {
				data, err := os.ReadFile(abs(g.root, stamp))
				if err != nil {
					return nil, fmt.Errorf("%s's stamp: %w", m.path, err)
				}
				c.inventory += " " + strings.TrimSpace(string(data))
			}
			c.version += " (with " + g.cfg.App + "'s patches)"
		}
		if g.fetch {
			if c.files, err = moduleLicences(m); err != nil {
				return nil, err
			}
			if len(c.files) == 0 {
				return nil, fmt.Errorf("%s has no licence file", m.path)
			}
		}
		comps = append(comps, c)
	}
	return comps, nil
}

// goDirective is go.mod's go version.
func (g *gen) goDirective() (string, error) {
	data, err := os.ReadFile(filepath.Join(g.root, "go.mod"))
	if err != nil {
		return "", err
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		if v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "go "); ok {
			return strings.TrimSpace(v), nil
		}
	}
	return "", fmt.Errorf("go.mod has no go directive")
}

// linkedModules runs go list for the app as built for Apple silicon.
func (g *gen) linkedModules() ([]goModule, error) {
	args := append([]string{"list", "-deps", "-f", "{{if not .Standard}}{{with .Module}}{{if not .Main}}{{.Path}}\t{{.Version}}\t{{.Dir}}\t{{$.Dir}}{{end}}{{end}}{{end}}"}, g.cfg.Go.Packages...)
	out, err := g.goOutput([]string{"GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0"}, args...)
	if err != nil {
		return nil, err
	}
	byPath := map[string]*goModule{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			continue
		}
		m := byPath[f[0]]
		if m == nil {
			m = &goModule{path: f[0], version: f[1], dir: f[2], pkgDirs: map[string]bool{}}
			byPath[f[0]] = m
		}
		m.pkgDirs[f[3]] = true
	}
	var mods []goModule
	for _, m := range byPath {
		mods = append(mods, *m)
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].path < mods[j].path })
	return mods, nil
}

// moduleLicences are the module's root licence files and those of the
// directories, from each linked package's up to the root.
func moduleLicences(m goModule) ([]licFile, error) {
	files, err := dirLicences(m.dir, "", true)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{m.dir: true}
	var dirs []string
	for d := range m.pkgDirs {
		for ; strings.HasPrefix(d, m.dir+string(filepath.Separator)) && !seen[d]; d = filepath.Dir(d) {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		rel, _ := filepath.Rel(m.dir, d)
		more, err := dirLicences(d, filepath.ToSlash(rel)+"/", false)
		if err != nil {
			return nil, err
		}
		files = append(files, more...)
	}
	return files, nil
}

// dirLicences reads the licence files directly in dir; prefix goes before
// their names in labels.
func dirLicences(dir, prefix string, root bool) ([]licFile, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []licFile
	for _, e := range ents {
		if e.IsDir() || !isLicenceFile(e.Name()) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		files = append(files, licFile{label: prefix + e.Name(), text: string(data), root: root})
	}
	return files, nil
}

// goOutput runs the go command in the repository.
func (g *gen) goOutput(env []string, args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = g.root
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go %s: %w\n%s", strings.Join(args, " "), err, stderr.Bytes())
	}
	return string(out), nil
}
