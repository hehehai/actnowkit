//go:build darwin

package feedback

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// SystemConverter is macOS's /usr/bin/sips (images) and
// /usr/bin/avconvert (movies; its metadata filter drops the location).
func SystemConverter() Converter { return macConverter{} }

type macConverter struct{}

func (macConverter) ToPNG(ctx context.Context, src, dst string) error {
	return run(ctx, "/usr/bin/sips", "-s", "format", "png", src, "--out", dst)
}

func (macConverter) CompressVideo(ctx context.Context, src, dst, preset string) error {
	os.Remove(dst)
	return run(ctx, "/usr/bin/avconvert", "--source", src, "--output", dst, "--preset", preset, "--replace")
}

// VideoThumbnail draws a movie's first frame with Quick Look
// (/usr/bin/qlmanage -t), as the Finder's thumbnail.
func (macConverter) VideoThumbnail(ctx context.Context, src, dstPNG string, px int) error {
	dir, err := os.MkdirTemp("", "fbql")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := run(ctx, "/usr/bin/qlmanage", "-t", "-s", strconv.Itoa(px), "-o", dir, src); err != nil {
		return err
	}
	out := filepath.Join(dir, filepath.Base(src)+".png")
	data, err := os.ReadFile(out)
	if err != nil {
		return fmt.Errorf("qlmanage made no thumbnail: %w", err)
	}
	return os.WriteFile(dstPNG, data, 0o600)
}

func run(ctx context.Context, name string, args ...string) error {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return fmt.Errorf("%s: %w: %s", name, err, msg)
	}
	return nil
}
