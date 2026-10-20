//go:build !darwin

package licence

import (
	"errors"
	"os"
)

func platformUUID() (string, error) { return "", errors.New("not macOS") }

func thisDevice() DeviceInfo {
	name, _ := os.Hostname()
	return DeviceInfo{Name: name}
}

func checkSelf(string) error { return ErrSelfCheckUnsupported }

func checkBundle(string, string) error { return ErrSelfCheckUnsupported }
