package credentials

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
)

// EnvNoKeychain set to "1" keeps every Keychain of the process away from
// the macOS Keychain (scripted checks, CI): the items go to a stand-in
// file (EnvKeychainFile, else $HOME/.actnow-keychain-standin.json).
const (
	EnvNoKeychain   = "ACTNOW_NO_KEYCHAIN"
	EnvKeychainFile = "ACTNOW_KEYCHAIN_FILE"
)

// guardDecision says whether /usr/bin/security may run, and where the
// items go when not.
type guardDecision struct {
	off  bool
	why  string
	file string
}

var guardOnce = sync.OnceValue(func() guardDecision {
	return decideGuard(os.Getenv("HOME"), userHome(), os.Getenv(EnvNoKeychain), os.Getenv(EnvKeychainFile), runtime.GOOS)
})

// decideGuard: a process whose HOME is not the user's home (a test copy
// of an app with a HOME of its own) or has no Library/Keychains must
// never run security: macOS then shows "Keychain Not Found … Reset To
// Defaults" on the user's screen, again at every access — and pressing it
// resets the user's login keychain. Such a process, and one with
// ACTNOW_NO_KEYCHAIN=1, keeps its items in a file instead.
func decideGuard(home, real, env, file, goos string) guardDecision {
	other := home != "" && real != "" && filepath.Clean(home) != filepath.Clean(real)
	if file == "" && home != "" && (other || env == "1") {
		file = filepath.Join(home, ".actnow-keychain-standin.json")
	}
	switch {
	case env == "1":
		return guardDecision{true, EnvNoKeychain + "=1", file}
	case other:
		return guardDecision{true, "HOME is not the user's home (" + home + ")", file}
	case goos == "darwin" && home != "" && !exists(filepath.Join(home, "Library", "Keychains")):
		return guardDecision{true, "no keychain in " + home, file}
	}
	return guardDecision{}
}

// KeychainGuarded reports whether this process keeps away from the macOS
// Keychain (and why): its Keychains then use a stand-in file.
func KeychainGuarded() (bool, string) {
	d := guardOnce()
	return d.off, d.why
}

func userHome() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.HomeDir
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
