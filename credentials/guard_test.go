package credentials

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestDecideGuard(t *testing.T) {
	home := t.TempDir()
	cases := []struct {
		home, real, env, file, goos string
		off                         bool
		wantFile                    string
	}{
		{"/Users/ann", "/Users/ann", "", "", "darwin", true, ""}, // no Library/Keychains there (not on this machine)
		{home, home, "", "", "linux", false, ""},
		{home, "/Users/ann", "", "", "linux", true, filepath.Join(home, ".actnow-keychain-standin.json")},
		{home, home, "1", "", "linux", true, filepath.Join(home, ".actnow-keychain-standin.json")},
		{home, "/Users/ann", "", "/tmp/k.json", "darwin", true, "/tmp/k.json"},
	}
	for i, c := range cases {
		d := decideGuard(c.home, c.real, c.env, c.file, c.goos)
		if d.off != c.off || d.file != c.wantFile {
			t.Errorf("case %d: %+v", i, d)
		}
	}
}

func TestStandInRoundTrip(t *testing.T) {
	file := filepath.Join(t.TempDir(), "k.json")
	k := Keychain{Service: "dev.hehehai.dicta.check.licence", Run: standIn(file)}
	if _, err := k.Get("token"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty: %v", err)
	}
	if err := k.Set("token", "Dicta Check licence", "secret value"); err != nil {
		t.Fatal(err)
	}
	// Another process (a new store of the same file) reads it.
	storesMu.Lock()
	delete(stores, file)
	storesMu.Unlock()
	k2 := Keychain{Service: "dev.hehehai.dicta.check.licence", Run: standIn(file)}
	if v, err := k2.Get("token"); err != nil || v != "secret value" {
		t.Fatalf("read back: %q %v", v, err)
	}
	if err := k2.Delete("token"); err != nil {
		t.Fatal(err)
	}
	if _, err := k2.Get("token"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted: %v", err)
	}
}
