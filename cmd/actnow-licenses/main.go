// Command actnow-licenses writes an ActNow app's ThirdPartyLicenses.txt
// (format: package thirdparty): the copyright notices and licence texts of
// everything third-party that ships inside the app. Run it from the app's
// repository after a dependency changes, and with -check before a release:
//
//	go run github.com/hehehai/actnowkit/cmd/actnow-licenses [-config FILE] [-check]
//
// The app's config (default third_party/licenses/actnow-licenses.json)
// says what it is built from; paths are relative to the repository:
//
//	{
//	  "app": "CaptionCraft",
//	  "out": "resources/ThirdPartyLicenses.txt",
//	  "header": "third_party/licenses/header.txt",
//	  "regenerate": "scripts/gen-licenses.sh",
//	  "go": {"packages": ["."], "patched": {"github.com/egoist/mygo": "third_party/mygo/VENDORED"}},
//	  "swift": {
//	    "resolved": "engine/Package.resolved",
//	    "checkouts": "engine/.build/xcode/SourcePackages/checkouts",
//	    "scripts": [{"identity": "mlx-audio-swift", "file": "scripts/fetch-engine-deps.sh"}],
//	    "patched": ["mlx-audio-swift"]
//	  },
//	  "npm": {"list": "extension/licenses.json", "use": "In the browser extension."},
//	  "extras": "third_party/licenses/extras.json",
//	  "programs": "third_party/licenses/programs.json"
//	}
//
// What it lists, in this order:
//   - Go: the toolchain's runtime and standard library, and every module
//     linked into the darwin/arm64 app (go list -deps of go.packages), with
//     the licence files of the module and of the directories of the
//     packages linked; a module in go.patched is marked "with the app's
//     patches", and its stamp file's text joins the inventory;
//   - Swift: every package pinned in swift.resolved (none when the file
//     does not exist: an engine without packages), plus the packages a
//     script pins (its UPSTREAM="url" and COMMIT="sha" lines), with every
//     licence, notice and acknowledgements file in its checkout (vendored
//     C++ included; tests, examples and docs left out);
//   - npm: the packages of npm.list (name, version, licence, source,
//     files: [{name, text}]), as a JavaScript build writes them;
//   - extras: code, data and artwork copied into the app or vendored
//     without a licence file (extras.json: [{Name, Version, Source, Use,
//     Licence, Files}], the files beside it);
//   - programs: separate executables that ship inside the app and that it
//     runs, not linked into it (programs.json, as extras.json, Licence
//     required): they keep their own licence, copyleft ones included.
//
// The file starts with the header file's text. The Swift checkouts are
// read from swift.checkouts (a SwiftPM checkouts directory) when their
// revisions match, else cloned at their pinned revisions into -cache. It
// stops on a licence it cannot classify and on any GNU licence (GPL, LGPL,
// AGPL) in what is linked in: the family's apps are closed source.
//
// -check computes the inventory (the components and versions, without
// fetching anything but Go modules) and fails unless the file was made
// from the same one with the current header, or if it names a GNU licence
// outside separate programs.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func main() {
	log.SetFlags(0)
	root := flag.String("root", ".", "the app's repository")
	config := flag.String("config", "third_party/licenses/actnow-licenses.json", "the app's config, relative to -root")
	check := flag.Bool("check", false, "check the file matches the dependencies instead of writing it")
	cache := flag.String("cache", defaultCache(), "where to clone Swift packages")
	flag.Parse()
	msg, err := run(*root, *config, *cache, *check)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(msg)
}

// run generates or checks, and says what it did.
func run(root, config, cache string, check bool) (string, error) {
	r, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	cfg, err := loadConfig(abs(r, config))
	if err != nil {
		return "", err
	}
	g := &gen{cfg: cfg, root: r, cache: cache, fetch: !check}
	comps, err := g.components()
	if err != nil {
		return "", err
	}
	head, err := g.header()
	if err != nil {
		return "", err
	}
	inv := inventory(comps)
	path := abs(r, cfg.Out)
	if check {
		if err := checkFile(path, head, inv); err != nil {
			return "", fmt.Errorf("%v\nrun %s", err, cfg.Regenerate)
		}
		return fmt.Sprintf("%s matches the dependencies (%d components)", cfg.Out, len(comps)), nil
	}
	text, err := render(head, comps, inv)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %s: %d components, %d KB", cfg.Out, len(comps), len(text)/1024), nil
}

// gen holds the config and where things are.
type gen struct {
	cfg         *config
	root, cache string
	fetch       bool // read licence texts (false: the inventory only)
}

// components lists everything, in the file's order.
func (g *gen) components() ([]component, error) {
	var all []component
	for _, part := range []struct {
		name string
		get  func() ([]component, error)
	}{
		{"go", g.goComponents},
		{"swift", g.swiftComponents},
		{"npm", g.npmComponents},
		{"extras", func() ([]component, error) { return g.extraComponents(g.cfg.Extras, false) }},
		{"programs", func() ([]component, error) { return g.extraComponents(g.cfg.Programs, true) }},
	} {
		comps, err := part.get()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", part.name, err)
		}
		all = append(all, comps...)
	}
	return all, nil
}

func abs(root, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(root, p)
}

// defaultCache is shared by the apps: the same Swift pins are cloned once.
func defaultCache() string {
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "actnow-licenses")
	}
	return filepath.Join(os.TempDir(), "actnow-licenses")
}
