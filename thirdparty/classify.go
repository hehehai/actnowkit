package thirdparty

import (
	"slices"
	"strings"
)

// The licence classifier: which licences a text is (or contains, for an
// acknowledgements file quoting several), by their usual wording. It
// knows the licences the family's dependencies use; a text it does not
// know classifies as nothing, and the generator (cmd/actnow-licenses)
// stops on it.

// marker is a licence and phrases that, all present, identify it.
type marker struct {
	id      string
	phrases []string
}

// markers in the order they are tried. Copyleft comes after the licences
// whose texts mention it (MPL names the GNU licences as secondary ones).
var markers = []marker{
	{"Apache-2.0", []string{"apache license", "version 2.0"}},
	{"Unicode-3.0", []string{"unicode", "permission is hereby granted, free of charge, to any person obtaining a copy of data files"}},
	{"MIT", []string{"permission is hereby granted, free of charge, to any person obtaining a copy of this software"}},
	{"BSD-3-Clause", []string{"redistribution and use in source and binary forms", "neither the name"}},
	{"BSD-3-Clause", []string{"redistribution and use in source and binary forms", "names of its contributors may"}},
	{"BSD-2-Clause", []string{"redistribution and use in source and binary forms"}},
	{"ISC", []string{"permission to use, copy, modify, and/or distribute this software for any purpose"}},
	{"BSL-1.0", []string{"boost software license"}},
	{"Zlib", []string{"this software is provided 'as-is'", "altered source versions must be plainly marked"}},
	{"Unlicense", []string{"this is free and unencumbered software released into the public domain"}},
	{"CC-BY-4.0", []string{"creative commons attribution 4.0"}},
	{"CC-BY-4.0", []string{"creativecommons.org/licenses/by/4.0"}},
	{"blessing", []string{"the author disclaims copyright to this source code"}},
	{"MPL-2.0", []string{"mozilla public license", "version 2.0"}},
	{"AGPL-3.0", []string{"gnu affero general public license"}},
	{"LGPL", []string{"gnu lesser general public license"}},
	{"LGPL", []string{"gnu library general public license"}},
	{"GPL", []string{"gnu general public license"}},
}

// Classify returns the licences text is or contains, without repeats, in
// markers' order; nil when it knows none.
func Classify(text string) []string {
	norm := strings.ToLower(strings.Join(strings.Fields(text), " "))
	// Typographic quotes in some copies of the zlib text.
	norm = strings.NewReplacer("’", "'", "‘", "'").Replace(norm)
	var ids []string
	mpl := false
	for _, m := range markers {
		if slices.Contains(ids, m.id) || (strings.HasPrefix(m.id, "BSD-") && slices.ContainsFunc(ids, func(id string) bool { return strings.HasPrefix(id, "BSD-") })) {
			continue // one BSD variant: the 3-clause text contains the 2-clause one
		}
		if mpl && (m.id == "GPL" || m.id == "LGPL" || m.id == "AGPL-3.0") {
			continue // MPL's text names them
		}
		if !containsAll(norm, m.phrases) {
			continue
		}
		ids = append(ids, m.id)
		if m.id == "MPL-2.0" {
			mpl = true
		}
	}
	return ids
}

func containsAll(s string, subs []string) bool {
	for _, p := range subs {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

// Copyleft reports whether a licence id is one a closed-source app may not
// link: the GNU licences.
func Copyleft(id string) bool {
	id = strings.ToUpper(id)
	return strings.HasPrefix(id, "GPL") || strings.HasPrefix(id, "LGPL") || strings.HasPrefix(id, "AGPL")
}

// CopyleftIn returns the copyleft licences a Licence line names.
func CopyleftIn(licence string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(licence, func(r rune) bool { return strings.ContainsRune(" ,;()/+", r) }) {
		if Copyleft(f) && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}
