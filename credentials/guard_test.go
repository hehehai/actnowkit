package credentials

import (
	"errors"
	"os"
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

// A release build in the user's own HOME (with its keychain) is not
// guarded: it runs security exactly as before. Symbolic links to the same
// folder are the same HOME.
func TestGuardLeavesTheUsersHomeAlone(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, "Library", "Keychains"), 0o700)
	if d := decideGuard(home, home, "", "", "darwin"); d.off {
		t.Fatalf("user's home guarded: %+v", d)
	}
	link := filepath.Join(t.TempDir(), "home-link")
	if err := os.Symlink(home, link); err != nil {
		t.Fatal(err)
	}
	if d := decideGuard(link, home, "", "", "darwin"); d.off {
		t.Fatalf("a link to the user's home guarded: %+v", d)
	}
	if d := decideGuard(home+"/", home, "", "", "darwin"); d.off {
		t.Fatalf("trailing slash guarded: %+v", d)
	}
	// Unknown user database answer: the keychain folder decides.
	if d := decideGuard(home, "", "", "", "darwin"); d.off {
		t.Fatalf("unknown real home, keychain present: %+v", d)
	}
	other := t.TempDir()
	if d := decideGuard(other, "", "", "", "darwin"); !d.off {
		t.Fatalf("unknown real home, no keychain: %+v", d)
	}
}

// Not guarded, Keychain runs security (through OnSecurity); guarded, it
// never does.
func TestOnSecurityProbe(t *testing.T) {
	calls := 0
	OnSecurity = func(args ...string) { calls++ }
	defer func() { OnSecurity = nil }()
	RunSecurity("", "find-generic-password", "-s", "x")
	if calls != 1 {
		t.Fatalf("probe not called: %d", calls)
	}
	k := Keychain{Service: "x", Run: standIn(filepath.Join(t.TempDir(), "k.json"))}
	k.Get("a")
	if calls != 1 {
		t.Fatalf("the stand-in ran security: %d", calls)
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
