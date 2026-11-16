package feedbackui

import (
	"context"
	"fmt"
	"image"
	"os"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/hehehai/actnowkit/feedback"
)

// Sizes of the pictures made of the attachments, in pixels.
const (
	thumbPx = 184  // the grid's tiles (92 DIPs at 2x)
	fullPx  = 1600 // the preview
)

// thumb is a picture of an attachment, made off the UI thread.
type thumb struct {
	bmp     *ui.Bitmap
	w, h    int
	dur     time.Duration // a video's
	size    int64
	failed  bool
	loading bool
}

func (w *Window) makeThumb(path string, px int) (image.Image, error) {
	if w.opt.Thumbnail != nil {
		return w.opt.Thumbnail(path, px)
	}
	conv := w.opt.Converter
	if conv == nil {
		conv = feedback.SystemConverter()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return feedback.Thumbnail(ctx, conv, path, px)
}

// picture is the thumbnail (px thumbPx) or the preview's picture of
// path, asked for the first time it is wanted.
func (w *Window) picture(path string, px int) *thumb {
	cache := w.thumbs
	if px > thumbPx {
		cache = w.full
	}
	if t := cache[path]; t != nil {
		return t
	}
	t := &thumb{loading: true}
	if st, err := os.Stat(path); err == nil {
		t.size = st.Size()
	}
	video := isVideo(path)
	cache[path] = t
	w.async(func() {
		img, err := w.makeThumb(path, px)
		var dur time.Duration
		if video {
			dur, _ = feedback.VideoDuration(path)
		}
		w.post(func() {
			t.loading, t.dur = false, dur
			if err != nil || img == nil {
				t.failed = true
				return
			}
			t.bmp = ui.NewBitmap(img)
			b := img.Bounds()
			t.w, t.h = b.Dx(), b.Dy()
		})
	})
	return t
}

// clock is a duration as a video's ("0:42", "1:02:03").
func clock(d time.Duration) string {
	s := int(d.Round(time.Second) / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// humanBytes is a size as people read it ("1.8 MB").
func humanBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
