package licence

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

// ErrNoDeviceID: the Mac's IOPlatformUUID cannot be read (not macOS, or
// IOKit failed). There is no fallback identifier.
var ErrNoDeviceID = errors.New("licence: cannot read this Mac's hardware ID")

// DeviceIDFromUUID is the device ID the site stores for a Mac
// (docs/licensing.md §4): SHA-256 of "actnow-device-v1", the app's salt and
// the IOPlatformUUID (upper case), NUL-separated, as 64 hex digits. The
// salt makes the ID differ between apps; it is not secret.
func DeviceIDFromUUID(platformUUID, salt string) string {
	h := sha256.New()
	h.Write([]byte("actnow-device-v1"))
	h.Write([]byte{0})
	h.Write([]byte(salt))
	h.Write([]byte{0})
	h.Write([]byte(strings.ToUpper(strings.TrimSpace(platformUUID))))
	return hex.EncodeToString(h.Sum(nil))
}

// DeviceID is this Mac's device ID for an app's salt.
func DeviceID(salt string) (string, error) {
	uuid, err := platformUUID()
	if err != nil || uuid == "" {
		return "", errors.Join(ErrNoDeviceID, err)
	}
	return DeviceIDFromUUID(uuid, salt), nil
}

// DeviceInfo describes this Mac for the site's device list: its name as in
// System Settings, the model identifier (Mac15,12) and the macOS version.
type DeviceInfo struct {
	Name  string `json:"name,omitempty"`
	Model string `json:"model,omitempty"`
	OS    string `json:"os,omitempty"`
}

// ThisDevice is DeviceInfo of the running Mac (empty fields when unknown).
func ThisDevice() DeviceInfo { return thisDevice() }
