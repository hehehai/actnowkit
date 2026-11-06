package thirdparty

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"MIT License\n\nCopyright (c) 2023 ml-explore\n\nPermission is hereby granted, free of charge, to any person obtaining a copy\nof this software and associated documentation files", []string{"MIT"}},
		{"                                 Apache License\n                           Version 2.0, January 2004", []string{"Apache-2.0"}},
		{"Redistribution and use in source and binary forms, with or without modification... * Neither the name of Google LLC nor", []string{"BSD-3-Clause"}},
		{"Redistribution and use in source and binary forms ... the names of its contributors may not be used", []string{"BSD-3-Clause"}},
		{"Redistribution and use in source and binary forms, with or without\nmodification, are permitted", []string{"BSD-2-Clause"}},
		{"This project is provided under the terms of the UNLICENSE or the BSD license\nThis is free and unencumbered software released into the public domain.\nRedistribution and use in source and binary forms ... Neither the name", []string{"BSD-3-Clause", "Unlicense"}},
		{"Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee", []string{"ISC"}},
		{"Boost Software License - Version 1.0", []string{"BSL-1.0"}},
		{"This software is provided ‘as-is’, without any express or implied warranty. 2. Altered source versions must be plainly marked as such", []string{"Zlib"}},
		{"The author disclaims copyright to this source code.", []string{"blessing"}},
		{"Licensed under https://creativecommons.org/licenses/by/4.0/", []string{"CC-BY-4.0"}},
		{"UNICODE LICENSE V3 ... Permission is hereby granted, free of charge, to any person obtaining a copy of data files", []string{"Unicode-3.0"}},
		{"GNU AFFERO GENERAL PUBLIC LICENSE\nVersion 3", []string{"AGPL-3.0"}},
		{"GNU LESSER GENERAL PUBLIC LICENSE Version 3 ... the GNU General Public License", []string{"LGPL", "GPL"}},
		{"GNU LIBRARY GENERAL PUBLIC LICENSE Version 2", []string{"LGPL"}},
		{"GNU GENERAL PUBLIC LICENSE\n Version 2, June 1991", []string{"GPL"}},
		{"Mozilla Public License Version 2.0 ... GNU General Public License, Version 2.0, the GNU Lesser General Public License", []string{"MPL-2.0"}},
		{"Some licence nobody knows.", nil},
	}
	for _, c := range cases {
		if got := Classify(c.text); !slices.Equal(got, c.want) {
			t.Errorf("Classify(%.40q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestCopyleft(t *testing.T) {
	for id, want := range map[string]bool{"GPL": true, "gpl-3.0": true, "LGPL-2.1-or-later": true, "AGPL-3.0": true, "MIT": false, "MPL-2.0": false, "Apache-2.0": false} {
		if got := Copyleft(id); got != want {
			t.Errorf("Copyleft(%s) = %v", id, got)
		}
	}
	if got := CopyleftIn("MIT (also Apache-2.0, BSD-3-Clause)"); got != nil {
		t.Errorf("permissive: %v", got)
	}
	if got := CopyleftIn("Apache-2.0 (also GPL, LGPL, GPL)"); !slices.Equal(got, []string{"GPL", "LGPL"}) {
		t.Errorf("copyleft: %v", got)
	}
	if got := CopyleftIn("LGPL-2.1-or-later/GPL+"); !slices.Equal(got, []string{"LGPL-2.1-or-later", "GPL"}) {
		t.Errorf("separators: %v", got)
	}
}

const sample = "Preamble\nInventory: x\n\n" + "################################################################################" + "\nName: a\nVersion: 1\nSource: https://a\nLicence: MIT\nUse: Things.\n\n----- LICENSE -----\n\nMIT text\n\n" +
	"################################################################################" + "\nName: b\nSource: https://b\nLicence: Apache-2.0\n\n----- LICENSE -----\n\nApache text\n" +
	"################################################################################" + "\nNo name here\n\ntext\n" +
	"################################################################################" + "\nName: c\nSource: https://c\nLicence: GPL-3.0\nShips: program\n\n----- COPYING -----\n\nGPL text\n"

func TestParse(t *testing.T) {
	for _, text := range []string{sample, strings.ReplaceAll(sample, "\n", "\r\n")} {
		pre, comps := Parse(text)
		if pre != "Preamble\nInventory: x" {
			t.Errorf("preamble %q", pre)
		}
		if len(comps) != 3 || !comps[2].Program() || comps[1].Program() || comps[0].Name != "a" || comps[0].Version != "1" || comps[0].Use != "Things." || comps[1].Version != "" || comps[1].Licence != "Apache-2.0" {
			t.Fatalf("components %+v", comps)
		}
		if comps[0].Text != "----- LICENSE -----\n\nMIT text" || !strings.HasPrefix(comps[1].Text, "----- LICENSE") {
			t.Errorf("texts %q %q", comps[0].Text, comps[1].Text)
		}
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := Load(dir); err == nil {
		t.Error("no file: no error")
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	pre, comps, err := Load(dir)
	if err != nil || pre == "" || len(comps) != 3 {
		t.Errorf("%q %d %v", pre, len(comps), err)
	}
}

func TestCheck(t *testing.T) {
	if name, bad := Check(sample); name != "" {
		t.Errorf("a GPL program is allowed: %s %v", name, bad)
	}
	linked := strings.Replace(sample, "Licence: Apache-2.0", "Licence: Apache-2.0 (also LGPL)", 1)
	if name, bad := Check(linked); name != "b" || !slices.Equal(bad, []string{"LGPL"}) {
		t.Errorf("linked LGPL: %s %v", name, bad)
	}
}

func TestRule(t *testing.T) {
	if len(Rule) != 80 || strings.Trim(Rule, "#") != "" {
		t.Error(Rule)
	}
}
