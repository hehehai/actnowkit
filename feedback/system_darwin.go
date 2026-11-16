//go:build darwin

package feedback

import (
	"encoding/binary"
	"syscall"
)

func thisSystem() System {
	s := System{}
	s.OS, _ = syscall.Sysctl("kern.osproductversion")
	if s.OS != "" {
		s.OS = "macOS " + s.OS
	}
	if b, _ := syscall.Sysctl("kern.osversion"); b != "" {
		s.OSBuild = b
	}
	s.Model, _ = syscall.Sysctl("hw.model")
	s.Chip, _ = syscall.Sysctl("machdep.cpu.brand_string")
	s.CPUs = int(sysctlUint("hw.ncpu"))
	s.MemoryGB = int((sysctlUint("hw.memsize") + 1<<29) >> 30)
	return s
}

// sysctlUint reads a numeric sysctl: syscall.Sysctl drops one trailing
// NUL byte, which a little-endian number may end with.
func sysctlUint(name string) uint64 {
	v, err := syscall.Sysctl(name)
	if err != nil || v == "" || len(v) > 8 {
		return 0
	}
	b := make([]byte, 8)
	copy(b, v)
	return binary.LittleEndian.Uint64(b)
}

func freeBytes(path string) int64 {
	var st syscall.Statfs_t
	if syscall.Statfs(path, &st) != nil {
		return 0
	}
	return int64(st.Bavail) * int64(st.Bsize)
}
