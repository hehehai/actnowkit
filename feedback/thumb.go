package feedback

import (
	"context"
	"encoding/binary"
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Draft is the form a Prefill opens: its kind (a problem when it carries a
// report), body and files, diagnostics and the report included.
func (p Prefill) Draft() Draft {
	d := Draft{Kind: p.Kind, Body: p.Body, Images: append([]string(nil), p.Images...), Video: p.Video,
		Diagnostics: true, Report: p.Report, IncludeReport: p.Report != nil}
	if d.Kind == "" {
		d.Kind = KindBug
	}
	if len(d.Images) > MaxImages {
		d.Images = d.Images[:MaxImages]
	}
	return d
}

// Thumbnail is a picture of an image or a video for the attachment grid,
// its longest side at most px: images decoded here (HEIC and the others
// through conv), videos through conv's VideoThumbnail (macOS: Quick Look).
func Thumbnail(ctx context.Context, conv Converter, path string, px int) (image.Image, error) {
	if px <= 0 {
		px = 256
	}
	if videoMIME(path) != "" {
		vt, ok := conv.(VideoThumbnailer)
		if !ok {
			return nil, ErrUnsupported
		}
		dir, err := os.MkdirTemp("", "fbthumb")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		out := filepath.Join(dir, "thumb.png")
		if err := vt.VideoThumbnail(ctx, path, out, px); err != nil {
			return nil, err
		}
		img, err := decodeImage(out, "png")
		if err != nil {
			return nil, err
		}
		return fit(img, px), nil
	}
	format := imageFormat(path)
	src := path
	switch format {
	case "":
		return nil, ErrNotImage
	case "png", "jpeg", "gif":
	default:
		if conv == nil {
			return nil, ErrUnsupported
		}
		dir, err := os.MkdirTemp("", "fbthumb")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		src = filepath.Join(dir, "conv.png")
		if err := conv.ToPNG(ctx, path, src); err != nil {
			return nil, err
		}
		format = "png"
	}
	img, err := decodeImage(src, format)
	if err != nil {
		return nil, err
	}
	return fit(img, px), nil
}

// VideoThumbnailer is a Converter that also draws a video's first frame
// (SystemConverter on macOS).
type VideoThumbnailer interface {
	VideoThumbnail(ctx context.Context, src, dstPNG string, px int) error
}

// VideoDuration reads an MP4 or QuickTime movie's length from its moov/mvhd
// box (no decoding).
func VideoDuration(path string) (time.Duration, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return mvhdDuration(f, 0, st.Size(), 0)
}

var errNoMvhd = errors.New("feedback: no movie header")

// mvhdDuration walks the boxes in [off, end) looking for moov/mvhd.
func mvhdDuration(r io.ReaderAt, off, end int64, depth int) (time.Duration, error) {
	hdr := make([]byte, 16)
	for off+8 <= end {
		if _, err := r.ReadAt(hdr[:8], off); err != nil {
			return 0, err
		}
		size := int64(binary.BigEndian.Uint32(hdr[:4]))
		typ := string(hdr[4:8])
		head := int64(8)
		switch size {
		case 1:
			if _, err := r.ReadAt(hdr[8:16], off+8); err != nil {
				return 0, err
			}
			size = int64(binary.BigEndian.Uint64(hdr[8:16]))
			head = 16
		case 0:
			size = end - off
		}
		if size < head || off+size > end {
			return 0, errNoMvhd
		}
		switch typ {
		case "moov":
			if depth == 0 {
				return mvhdDuration(r, off+head, off+size, 1)
			}
		case "mvhd":
			b := make([]byte, 32)
			n, _ := r.ReadAt(b, off+head)
			if n < 20 {
				return 0, errNoMvhd
			}
			var scale, dur uint64
			if b[0] == 1 { // version 1: 64-bit times
				if n < 32 {
					return 0, errNoMvhd
				}
				scale = uint64(binary.BigEndian.Uint32(b[20:24]))
				dur = binary.BigEndian.Uint64(b[24:32])
			} else {
				scale = uint64(binary.BigEndian.Uint32(b[12:16]))
				dur = uint64(binary.BigEndian.Uint32(b[16:20]))
			}
			if scale == 0 {
				return 0, errNoMvhd
			}
			return time.Duration(float64(dur) / float64(scale) * float64(time.Second)), nil
		}
		off += size
	}
	return 0, errNoMvhd
}
