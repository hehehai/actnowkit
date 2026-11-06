// Package thirdparty is the third-party licence file an ActNow app ships
// (ThirdPartyLicenses.txt in the app's resources): its format, which
// cmd/actnow-licenses writes and the app's Settings read, and the licence
// classifier the generator uses to refuse copyleft licences. The family's
// apps are closed source: no GPL, LGPL or AGPL in what is linked into
// them; separate programs that ship beside an app, such as ffmpeg, keep
// their own licence (Ships: program).
//
// The file is plain text, readable as is: a preamble, then one block per
// component, each opened by a Rule line and a few "Key: value" lines:
//
//	################################################################################
//	Name: mlx-swift
//	Version: 0.32.3
//	Source: https://github.com/ml-explore/mlx-swift
//	Licence: MIT (also Apache-2.0, BSD-3-Clause)
//	Use: …                       (optional)
//	Ships: program               (optional: a separate program in the app,
//	                              not linked into it)
//
//	<the licence texts>
package thirdparty

import (
	"os"
	"path/filepath"
	"strings"
)

// FileName is the file's name in the app's resources directory.
const FileName = "ThirdPartyLicenses.txt"

// Rule opens a component's block.
var Rule = strings.Repeat("#", 80)

// ShipsProgram is the Ships value of a separate program: an executable
// that ships inside the app and that the app runs, not code linked into it.
const ShipsProgram = "program"

// Component is one block of the file.
type Component struct {
	Name, Version, Source, Licence, Use string
	Ships                               string // ShipsProgram, or "" for what is linked in
	Text                                string // the licence texts
}

// Program reports a separate program (Ships: program), whose licence may
// be a copyleft one: it is not linked into the app.
func (c Component) Program() bool { return c.Ships == ShipsProgram }

// Parse reads the file's text: its preamble and components, in order.
// Blocks without a Name are skipped.
func Parse(text string) (preamble string, comps []Component) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	blocks := strings.Split(text, "\n"+Rule+"\n")
	preamble = strings.TrimSpace(blocks[0])
	for _, b := range blocks[1:] {
		head, body, _ := strings.Cut(b, "\n\n")
		var c Component
		for _, line := range strings.Split(head, "\n") {
			k, v, ok := strings.Cut(line, ": ")
			if !ok {
				continue
			}
			switch k {
			case "Name":
				c.Name = v
			case "Version":
				c.Version = v
			case "Source":
				c.Source = v
			case "Licence":
				c.Licence = v
			case "Use":
				c.Use = v
			case "Ships":
				c.Ships = v
			}
		}
		if c.Name == "" {
			continue
		}
		c.Text = strings.Trim(body, "\n")
		comps = append(comps, c)
	}
	return preamble, comps
}

// Load reads and parses the file in the resources directory dir.
func Load(dir string) (preamble string, comps []Component, err error) {
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		return "", nil, err
	}
	p, c := Parse(string(data))
	return p, c, nil
}

// Check reports the first component of the file's text that a closed-source
// app may not ship: one linked in (not a separate program) under a GNU
// licence, with those licences; "" when there is none.
func Check(text string) (name string, copyleft []string) {
	_, comps := Parse(text)
	for _, c := range comps {
		if bad := CopyleftIn(c.Licence); len(bad) > 0 && !c.Program() {
			return c.Name, bad
		}
	}
	return "", nil
}
