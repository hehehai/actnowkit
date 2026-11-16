//go:build darwin

package feedback

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMac runs against the real Mac (sysctl, /usr/bin/sips,
// /usr/bin/avconvert):
//
//	ACTNOWKIT_MAC=1 go test -run TestMac -v ./feedback
//
// ACTNOWKIT_FEEDBACK_VIDEO names a movie over 20 MB to compress (skipped
// when unset).
func TestMac(t *testing.T) {
	if os.Getenv("ACTNOWKIT_MAC") == "" {
		t.Skip("set ACTNOWKIT_MAC=1 on a Mac")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	t.Run("system", func(t *testing.T) {
		s := ThisSystem()
		t.Logf("%+v", s)
		if !strings.HasPrefix(s.OS, "macOS ") || s.Model == "" || s.Chip == "" || s.CPUs < 1 || s.MemoryGB < 4 || s.FreeDiskGB < 1 {
			t.Errorf("system: %+v", s)
		}
	})
	t.Run("heic", func(t *testing.T) {
		dir := t.TempDir()
		png := writePNG(t, dir, "src.png", 640, 480, false)
		heic := filepath.Join(dir, "src.heic")
		if out, err := exec.Command("/usr/bin/sips", "-s", "format", "heic", png, "--out", heic).CombinedOutput(); err != nil {
			t.Skipf("sips cannot write HEIC here: %v %s", err, out)
		}
		if imageFormat(heic) != "heic" {
			t.Fatalf("format %q", imageFormat(heic))
		}
		f, err := PrepareImage(ctx, SystemConverter(), heic, dir, "image-1")
		if err != nil || f.MIME != "image/png" || f.Width != 640 || f.Height != 480 {
			t.Fatalf("heic: %+v %v", f, err)
		}
	})
	t.Run("video", func(t *testing.T) {
		src := os.Getenv("ACTNOWKIT_FEEDBACK_VIDEO")
		if src == "" {
			t.Skip("set ACTNOWKIT_FEEDBACK_VIDEO to a movie over 20 MB")
		}
		st, err := os.Stat(src)
		if err != nil || st.Size() <= MaxVideoBytes {
			t.Fatalf("%s: %v (%d bytes)", src, err, st.Size())
		}
		start := time.Now()
		f, err := PrepareVideo(ctx, SystemConverter(), src, t.TempDir(), "video")
		t.Logf("%d MB → %+v in %s", st.Size()>>20, f, time.Since(start))
		if err != nil || f.Bytes > MaxVideoBytes || f.Bytes == 0 {
			t.Fatalf("video: %v", err)
		}
	})
}
