package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// config is the app's actnow-licenses.json (see the package doc).
type config struct {
	App        string `json:"app"`        // the app's name, in texts ("with CaptionCraft's patches")
	Out        string `json:"out"`        // the file written
	Header     string `json:"header"`     // the file whose text starts it
	Regenerate string `json:"regenerate"` // the command that writes it, named when -check fails
	Go         struct {
		Packages []string          `json:"packages"` // the app's main packages (default ".")
		Patched  map[string]string `json:"patched"`  // module path → its stamp file ("": none)
	} `json:"go"`
	Swift struct {
		Resolved  string      `json:"resolved"`  // Package.resolved
		Checkouts string      `json:"checkouts"` // SwiftPM's checkouts, read when their revisions match
		Scripts   []scriptPin `json:"scripts"`   // packages a script pins
		Patched   []string    `json:"patched"`   // identities with the app's patches
	} `json:"swift"`
	NPM struct {
		List string `json:"list"` // the build's list of packages and licence texts
		Use  string `json:"use"`  // their Use line
	} `json:"npm"`
	Extras   string `json:"extras"`   // extras.json
	Programs string `json:"programs"` // programs.json
}

// scriptPin is a Swift package a shell script checks out: its lines
// UPSTREAM="<url>" and COMMIT="<40 hex digits>" (other names with
// upstream and commit).
type scriptPin struct {
	Identity string `json:"identity"`
	File     string `json:"file"`
	Upstream string `json:"upstream"` // default UPSTREAM
	Commit   string `json:"commit"`   // default COMMIT
}

// loadConfig reads and checks the config; unknown fields are errors.
func loadConfig(path string) (*config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var missing []string
	for _, f := range []struct{ name, v string }{{"app", c.App}, {"out", c.Out}, {"header", c.Header}, {"regenerate", c.Regenerate}} {
		if f.v == "" {
			missing = append(missing, f.name)
		}
	}
	for _, s := range c.Swift.Scripts {
		if s.Identity == "" || s.File == "" {
			missing = append(missing, "swift.scripts[].identity and file")
		}
	}
	if c.NPM.List != "" && c.NPM.Use == "" {
		missing = append(missing, "npm.use")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s: missing %v", path, missing)
	}
	if len(c.Go.Packages) == 0 {
		c.Go.Packages = []string{"."}
	}
	return &c, nil
}

// header is the header file's text, trailing white space trimmed.
func (g *gen) header() (string, error) {
	data, err := os.ReadFile(abs(g.root, g.cfg.Header))
	if err != nil {
		return "", err
	}
	h := string(bytes.TrimRight(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), "\n \t"))
	if h == "" {
		return "", errors.New("the header is empty")
	}
	return h, nil
}
