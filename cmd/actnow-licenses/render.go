package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hehehai/actnowkit/thirdparty"
)

// component is one block of the output.
type component struct {
	inventory string // its inventory line: what, when changed, needs a new file
	name      string
	version   string
	source    string
	use       string
	licence   string // set by extras, programs and npm; else from the files
	program   bool   // a separate program (programs.json), not linked in
	files     []licFile
}

// licFile is a licence, notice or acknowledgements file.
type licFile struct {
	label string // its path in the component
	text  string
	root  bool // at the component's root (its own licence), not vendored
}

// extra is an entry of extras.json or programs.json.
type extra struct {
	Name, Version, Source, Use string
	Licence                    string   // when the texts do not say it plainly; required for a program
	Files                      []string // beside the JSON file
}

// extraComponents are the entries of the JSON file at path (none when path
// is ""), in its order; program: they are separate programs.
func (g *gen) extraComponents(path string, program bool) ([]component, error) {
	if path == "" {
		return nil, nil
	}
	file := abs(g.root, path)
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var extras []extra
	if err := json.Unmarshal(data, &extras); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var comps []component
	for _, e := range extras {
		h := sha256.New()
		fmt.Fprintf(h, "%s\n%s\n%s\n%s\n%s\n", e.Name, e.Version, e.Source, e.Use, e.Licence)
		c := component{name: e.Name, version: e.Version, source: e.Source, use: e.Use, licence: e.Licence, program: program}
		if program && e.Licence == "" {
			return nil, fmt.Errorf("%s: %s: a program needs its licence named", path, e.Name)
		}
		for _, f := range e.Files {
			text, err := os.ReadFile(filepath.Join(filepath.Dir(file), f))
			if err != nil {
				return nil, err
			}
			h.Write(text)
			c.files = append(c.files, licFile{label: f, text: string(text), root: true})
		}
		c.inventory = "extra " + hex.EncodeToString(h.Sum(nil))
		if program {
			c.inventory = "program " + hex.EncodeToString(h.Sum(nil))
		}
		comps = append(comps, c)
	}
	return comps, nil
}

// npmPackage is an entry of the npm list.
type npmPackage struct {
	Name, Version, Licence, Source string
	Files                          []struct{ Name, Text string }
}

// npmComponents are the npm list's packages, in its order.
func (g *gen) npmComponents() ([]component, error) {
	list := g.cfg.NPM.List
	if list == "" {
		return nil, nil
	}
	data, err := os.ReadFile(abs(g.root, list))
	if err != nil {
		return nil, err
	}
	var pkgs []npmPackage
	if err := json.Unmarshal(data, &pkgs); err != nil {
		return nil, fmt.Errorf("%s: %w", list, err)
	}
	var comps []component
	for _, p := range pkgs {
		h := sha256.New()
		fmt.Fprintf(h, "%s\n%s\n%s\n", p.Name, p.Version, p.Source)
		c := component{name: p.Name, version: p.Version, source: p.Source, use: g.cfg.NPM.Use}
		for _, f := range p.Files {
			h.Write([]byte(f.Text))
			c.files = append(c.files, licFile{label: f.Name, text: f.Text, root: true})
		}
		if len(c.files) == 0 {
			return nil, fmt.Errorf("%s: %s has no licence text", list, p.Name)
		}
		c.inventory = "npm " + p.Name + " " + p.Version + " " + hex.EncodeToString(h.Sum(nil))
		comps = append(comps, c)
	}
	return comps, nil
}

// licenceLine is what c's Licence line says: its own licences, then the
// others its vendored files are under.
func licenceLine(c component) (string, error) {
	if c.program {
		// A separate program keeps its own licence, copyleft included:
		// it is not linked into the app.
		return c.licence, nil
	}
	var own, other []string
	for _, f := range c.files {
		ids := thirdparty.Classify(f.text)
		if len(ids) == 0 {
			if noticeName.MatchString(filepath.Base(f.label)) || c.licence != "" {
				continue
			}
			return "", fmt.Errorf("%s: cannot tell the licence of %s; read it and, if it allows closed-source distribution, list it in extras.json with its licence", c.name, f.label)
		}
		for _, id := range ids {
			if thirdparty.Copyleft(id) {
				return "", fmt.Errorf("%s: %s is under %s, which a closed-source app may not ship", c.name, f.label, id)
			}
			// A notice or acknowledgements file at the root quotes what the
			// component includes, not its own licence.
			mine := f.root && !noticeName.MatchString(filepath.Base(f.label))
			switch {
			case mine && !slices.Contains(own, id):
				own = append(own, id)
			case !mine && !slices.Contains(other, id):
				other = append(other, id)
			}
		}
	}
	if c.licence != "" {
		own = []string{c.licence}
	}
	if len(own) == 0 {
		return "", fmt.Errorf("%s: no licence at its root", c.name)
	}
	other = slices.DeleteFunc(other, func(id string) bool { return slices.Contains(own, id) })
	line := strings.Join(own, ", ")
	if len(other) > 0 {
		line += " (also " + strings.Join(other, ", ") + ")"
	}
	return line, nil
}

// inventory is the hash of the components' inventory lines.
func inventory(comps []component) string {
	h := sha256.New()
	fmt.Fprintln(h, "format 1")
	for _, c := range comps {
		fmt.Fprintln(h, c.inventory)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// render writes the file: head, the inventory, the contents, the blocks.
func render(head string, comps []component, inv string) (string, error) {
	var b strings.Builder
	b.WriteString(head)
	fmt.Fprintf(&b, "\nInventory: %s\n\nContents:\n", inv)
	lines := make([]string, len(comps))
	for i, c := range comps {
		line, err := licenceLine(c)
		if err != nil {
			return "", err
		}
		lines[i] = line
		fmt.Fprintf(&b, "  %s — %s\n", strings.TrimSpace(c.name+" "+c.version), line)
	}
	for i, c := range comps {
		fmt.Fprintf(&b, "\n%s\nName: %s\n", thirdparty.Rule, c.name)
		if c.version != "" {
			fmt.Fprintf(&b, "Version: %s\n", c.version)
		}
		fmt.Fprintf(&b, "Source: %s\nLicence: %s\n", c.source, lines[i])
		if c.use != "" {
			fmt.Fprintf(&b, "Use: %s\n", c.use)
		}
		if c.program {
			fmt.Fprintf(&b, "Ships: %s\n", thirdparty.ShipsProgram)
		}
		for _, f := range c.files {
			fmt.Fprintf(&b, "\n----- %s -----\n\n%s\n", f.label, strings.TrimRight(strings.ReplaceAll(f.text, "\r\n", "\n"), "\n \t"))
		}
	}
	return b.String(), nil
}

// checkFile fails unless the file at path starts with head, was made from
// inventory inv and names no GNU licence outside separate programs.
func checkFile(path, head, inv string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, head+"\n") {
		return fmt.Errorf("%s is out of date: its header changed since it was made", filepath.Base(path))
	}
	pre, _ := thirdparty.Parse(text)
	if !strings.Contains(pre, "Inventory: "+inv) {
		return fmt.Errorf("%s is out of date: the dependencies changed since it was made", filepath.Base(path))
	}
	if name, bad := thirdparty.Check(text); name != "" {
		return fmt.Errorf("%s is under %s, which a closed-source app may not ship", name, strings.Join(bad, ", "))
	}
	return nil
}
