//go:build !darwin

package feedback

import (
	"os"
	"runtime"
)

// thisSystem outside macOS: what Go knows (tests, Linux builds).
func thisSystem() System {
	s := System{OS: runtime.GOOS, CPUs: runtime.NumCPU()}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		s.OSBuild = string(b)
	}
	return s
}

func freeBytes(string) int64 { return 0 }
