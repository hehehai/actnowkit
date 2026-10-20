//go:build darwin

package darwin

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	deviceOnce sync.Once
	deviceErr  error

	ioServiceMatching               func(name string) uintptr
	ioServiceGetMatchingService     func(mainPort uint32, matching uintptr) uint32
	ioRegistryEntryCreateCFProperty func(entry uint32, key uintptr, allocator uintptr, options uint32) uintptr
	ioObjectRelease                 func(object uint32) int32

	scDynamicStoreCopyComputerName func(store uintptr, encoding *uint32) uintptr

	secRequirementCreateWithString func(text uintptr, flags uint32, requirement *uintptr) int32
	secCodeCheckValidity           func(code uintptr, flags uint32, requirement uintptr) int32
	secStaticCodeCreateWithPath    func(path uintptr, flags uint32, code *uintptr) int32
	secStaticCodeCheckValidity     func(code uintptr, flags uint32, requirement uintptr) int32
	cfURLCreateFromFileSystemRep   func(allocator uintptr, buffer *byte, length int, isDirectory bool) uintptr
)

func loadDevice() error {
	deviceOnce.Do(func() {
		if err := loadFrameworks(); err != nil {
			deviceErr = err
			return
		}
		iokit, err := purego.Dlopen("/System/Library/Frameworks/IOKit.framework/IOKit", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			deviceErr = err
			return
		}
		purego.RegisterLibFunc(&ioServiceMatching, iokit, "IOServiceMatching")
		purego.RegisterLibFunc(&ioServiceGetMatchingService, iokit, "IOServiceGetMatchingService")
		purego.RegisterLibFunc(&ioRegistryEntryCreateCFProperty, iokit, "IORegistryEntryCreateCFProperty")
		purego.RegisterLibFunc(&ioObjectRelease, iokit, "IOObjectRelease")
		sc, err := purego.Dlopen("/System/Library/Frameworks/SystemConfiguration.framework/SystemConfiguration", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			deviceErr = err
			return
		}
		purego.RegisterLibFunc(&scDynamicStoreCopyComputerName, sc, "SCDynamicStoreCopyComputerName")
		sec, err := purego.Dlopen("/System/Library/Frameworks/Security.framework/Security", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			deviceErr = err
			return
		}
		purego.RegisterLibFunc(&secRequirementCreateWithString, sec, "SecRequirementCreateWithString")
		purego.RegisterLibFunc(&secCodeCheckValidity, sec, "SecCodeCheckValidity")
		purego.RegisterLibFunc(&secStaticCodeCreateWithPath, sec, "SecStaticCodeCreateWithPath")
		purego.RegisterLibFunc(&secStaticCodeCheckValidity, sec, "SecStaticCodeCheckValidity")
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			deviceErr = err
			return
		}
		purego.RegisterLibFunc(&cfURLCreateFromFileSystemRep, cf, "CFURLCreateFromFileSystemRepresentation")
	})
	return deviceErr
}

// PlatformUUID is the IOPlatformUUID of the Mac (IOPlatformExpertDevice),
// read through IOKit: no ioreg process.
func PlatformUUID() (string, error) {
	if err := loadDevice(); err != nil {
		return "", err
	}
	var out string
	var err error
	withPool(func() {
		matching := ioServiceMatching("IOPlatformExpertDevice")
		if matching == 0 {
			err = errors.New("IOServiceMatching failed")
			return
		}
		// Consumes one reference of matching.
		service := ioServiceGetMatchingService(0, matching)
		if service == 0 {
			err = errors.New("no IOPlatformExpertDevice")
			return
		}
		defer ioObjectRelease(service)
		key := nsString("IOPlatformUUID")
		value := ioRegistryEntryCreateCFProperty(service, uintptr(key), 0, 0)
		if value == 0 {
			err = errors.New("no IOPlatformUUID")
			return
		}
		defer cfRelease(value)
		out = goString(objc.ID(value))
	})
	if err == nil && out == "" {
		err = errors.New("empty IOPlatformUUID")
	}
	return out, err
}

// ComputerName is the Mac's name as in System Settings → General → Sharing
// ("Wei's MacBook Air"); "" when unknown.
func ComputerName() string {
	if loadDevice() != nil {
		return ""
	}
	var out string
	withPool(func() {
		name := scDynamicStoreCopyComputerName(0, nil)
		if name == 0 {
			return
		}
		defer cfRelease(name)
		out = goString(objc.ID(name))
	})
	return out
}

// Security framework status codes worth naming.
const (
	errSecCSUnsigned      = -67062
	errSecCSReqFailed     = -67050
	errSecCSSignatureFail = -67061
)

// ErrUnsigned: the code has no signature (development builds).
var ErrUnsigned = errors.New("the app is not signed")

// CheckSelfRequirement checks the running process's code signature against
// a code-signing requirement ("anchor apple generic and certificate
// leaf[subject.OU] = \"779M533DL5\""): the signature must be valid (nothing
// changed since signing) and satisfy it.
func CheckSelfRequirement(requirement string) error {
	if err := loadDevice(); err != nil {
		return err
	}
	var code uintptr
	if st := secCodeCopySelf(0, &code); st != 0 || code == 0 {
		return fmt.Errorf("SecCodeCopySelf: status %d", st)
	}
	defer cfRelease(code)
	var status int32
	withPool(func() {
		var req uintptr
		if st := secRequirementCreateWithString(uintptr(nsString(requirement)), 0, &req); st != 0 || req == 0 {
			status = st
			if status == 0 {
				status = -1
			}
			return
		}
		defer cfRelease(req)
		status = secCodeCheckValidity(code, 0, req)
	})
	return securityError(status)
}

// CheckBundleRequirement checks an app bundle on disk the same way (its
// files as well: kSecCSCheckAllArchitectures | kSecCSStrictValidate).
func CheckBundleRequirement(path, requirement string) error {
	if err := loadDevice(); err != nil {
		return err
	}
	b := append([]byte(path), 0)
	url := cfURLCreateFromFileSystemRep(0, &b[0], len(path), true)
	if url == 0 {
		return fmt.Errorf("bad path %s", path)
	}
	defer cfRelease(url)
	var code uintptr
	if st := secStaticCodeCreateWithPath(url, 0, &code); st != 0 || code == 0 {
		return securityError(st)
	}
	defer cfRelease(code)
	var status int32
	withPool(func() {
		var req uintptr
		if st := secRequirementCreateWithString(uintptr(nsString(requirement)), 0, &req); st != 0 || req == 0 {
			status = st
			if status == 0 {
				status = -1
			}
			return
		}
		defer cfRelease(req)
		const kSecCSCheckAllArchitectures = 1 << 0
		const kSecCSStrictValidate = 1 << 4
		status = secStaticCodeCheckValidity(code, kSecCSCheckAllArchitectures|kSecCSStrictValidate, req)
	})
	return securityError(status)
}

func securityError(status int32) error {
	switch status {
	case 0:
		return nil
	case errSecCSUnsigned:
		return ErrUnsigned
	case errSecCSReqFailed:
		return errors.New("the signature does not meet the requirement (another team)")
	case errSecCSSignatureFail:
		return errors.New("the signature is invalid (the app was changed)")
	}
	return fmt.Errorf("code signature check failed (status %d): the app was changed, or is not signed by the team", status)
}
