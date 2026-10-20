//go:build darwin

package licence

import (
	"os"
	"syscall"

	"github.com/hehehai/actnowkit/internal/darwin"
)

func platformUUID() (string, error) { return darwin.PlatformUUID() }

func thisDevice() DeviceInfo {
	info := DeviceInfo{Name: darwin.ComputerName()}
	if info.Name == "" {
		info.Name, _ = os.Hostname()
	}
	info.Model, _ = syscall.Sysctl("hw.model")
	info.OS, _ = syscall.Sysctl("kern.osproductversion")
	return info
}

// The requirement of VerifySelf: Developer ID code of the team.
func checkSelf(requirement string) error {
	err := darwin.CheckSelfRequirement(requirement)
	if err == darwin.ErrUnsigned {
		return ErrUnsigned
	}
	return err
}

func checkBundle(path, requirement string) error {
	err := darwin.CheckBundleRequirement(path, requirement)
	if err == darwin.ErrUnsigned {
		return ErrUnsigned
	}
	return err
}
