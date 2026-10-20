//go:build darwin

package licence

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hehehai/actnowkit/credentials"
)

// TestMac runs against the real Mac (IOKit, SystemConfiguration, the
// Security framework, /usr/bin/security on a temporary keychain):
//
//	ACTNOWKIT_MAC=1 go test -run TestMac -v ./licence
//
// ACTNOWKIT_SIGNED_APP names an installed app signed by TeamActNow
// (default /Applications/HereNow.app) for VerifyBundle.
func TestMac(t *testing.T) {
	if os.Getenv("ACTNOWKIT_MAC") == "" {
		t.Skip("set ACTNOWKIT_MAC=1 on a Mac")
	}
	t.Run("device", func(t *testing.T) {
		uuid, err := platformUUID()
		if err != nil || !regexp.MustCompile(`^[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}$`).MatchString(uuid) {
			t.Fatalf("IOPlatformUUID %q: %v", uuid, err)
		}
		out, _ := exec.Command("/usr/sbin/ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
		if !strings.Contains(string(out), uuid) {
			t.Fatalf("ioreg does not show %s", uuid)
		}
		id, err := DeviceID("dev.hehehai.test/salt")
		if err != nil || id != DeviceIDFromUUID(uuid, "dev.hehehai.test/salt") {
			t.Fatalf("device id %s: %v", id, err)
		}
		info := ThisDevice()
		t.Logf("device: %+v", info)
		name, _ := exec.Command("/usr/sbin/scutil", "--get", "ComputerName").Output()
		if info.Name != strings.TrimSpace(string(name)) || !strings.HasPrefix(info.Model, "Mac") || info.OS == "" {
			t.Fatalf("device info %+v (ComputerName %q)", info, name)
		}
	})
	t.Run("self signature", func(t *testing.T) {
		// A test binary is ad hoc signed by the linker: not our team.
		err := VerifySelf(TeamActNow)
		t.Logf("VerifySelf (test binary): %v", err)
		if err == nil {
			t.Fatal("an ad hoc test binary passed")
		}
		if err := VerifyBundle("/System/Applications/Calculator.app", TeamActNow); err == nil {
			t.Fatal("Apple's Calculator passed as team 779M533DL5")
		}
		app := os.Getenv("ACTNOWKIT_SIGNED_APP")
		if app == "" {
			app = "/Applications/HereNow.app"
		}
		if _, err := os.Stat(app); err == nil {
			if err := VerifyBundle(app, TeamActNow); err != nil {
				t.Fatalf("%s: %v", app, err)
			}
			t.Logf("%s: signed by %s", app, TeamActNow)
			if err := VerifyBundle(app, "RY2TQTYSKQ"); err == nil {
				t.Fatalf("%s passed as another team", app)
			}
			// A changed copy fails.
			copyDir := filepath.Join(t.TempDir(), filepath.Base(app))
			if out, err := exec.Command("/usr/bin/ditto", app, copyDir).CombinedOutput(); err != nil {
				t.Fatalf("ditto: %v %s", err, out)
			}
			plist := filepath.Join(copyDir, "Contents", "Resources", "actnowkit-tamper.txt")
			if err := os.WriteFile(plist, []byte("changed"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := VerifyBundle(copyDir, TeamActNow); err == nil {
				t.Fatal("a changed copy passed")
			} else {
				t.Logf("changed copy: %v", err)
			}
		}
	})
	t.Run("keychain store", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "actnowkit-licence-test.keychain-db")
		if out, err := exec.Command("/usr/bin/security", "create-keychain", "-p", "test", file).CombinedOutput(); err != nil {
			t.Fatalf("create-keychain: %v %s", err, out)
		}
		defer exec.Command("/usr/bin/security", "delete-keychain", file).Run()
		if out, err := exec.Command("/usr/bin/security", "unlock-keychain", "-p", "test", file).CombinedOutput(); err != nil {
			t.Fatalf("unlock-keychain: %v %s", err, out)
		}
		store := &KeychainStore{Keychain: credentials.Keychain{Service: "dev.hehehai.actnowkit-test.licence", File: file}}
		if _, err := store.Load(); !errors.Is(err, ErrNoRecord) {
			t.Fatal(err)
		}
		// A real flow against the fake server, stored in the keychain.
		h := newHarness(t, "0.3.0")
		h.store = nil
		c, err := New(Config{
			App: "captioncraft", Version: "0.3.0", DeviceSalt: testSalt, Keys: h.server.keys(),
			Store: store, BaseURL: h.server.srv.URL, Now: func() time.Time { return h.now },
		})
		if err != nil {
			t.Fatal(err)
		}
		h.server.addCode("CC-MAC", EditionPaid, 2, nil)
		st, err := c.Activate(context.Background(), "CC-MAC")
		if err != nil || st.State != Valid {
			t.Fatalf("%s %v", st.State, err)
		}
		again, _ := New(Config{App: "captioncraft", Version: "0.3.0", DeviceSalt: testSalt, Keys: h.server.keys(), Store: store, BaseURL: h.server.srv.URL, Now: func() time.Time { return h.now }})
		if st := again.Status(); st.State != Valid {
			t.Fatalf("after reading the keychain: %s", st.State)
		}
		if err := again.Forget(); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(); !errors.Is(err, ErrNoRecord) {
			t.Fatal(err)
		}
	})
}
