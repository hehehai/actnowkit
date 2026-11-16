package feedback

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

// Errors of the preparation of attachments.
var (
	ErrUnsupported = errors.New("feedback: this file type cannot be converted on this system")
	ErrTooLarge    = errors.New("feedback: the file stays too large")
	ErrNotImage    = errors.New("feedback: not an image")
	ErrNotVideo    = errors.New("feedback: not an MP4 or QuickTime movie")
)

// Converter does what pure Go cannot: decoding HEIC, WebP, TIFF and BMP
// images, and compressing videos. SystemConverter is macOS's sips and
// avconvert.
type Converter interface {
	// ToPNG converts an image to a PNG file.
	ToPNG(ctx context.Context, src, dst string) error
	// CompressVideo re-encodes a movie with an AVFoundation preset name
	// ("PresetHEVC1920x1080", "Preset1280x720", …), dropping its
	// privacy-sensitive metadata (location).
	CompressVideo(ctx context.Context, src, dst, preset string) error
}

// VideoPresets are tried in order until the movie fits MaxVideoBytes.
var VideoPresets = []string{"PresetHEVC1920x1080", "Preset1280x720", "Preset960x540", "PresetLowQuality"}

// maxPixels bounds the images decoded (memory: 4 bytes a pixel).
const maxPixels = 120_000_000

// imageFormat names the image type by the file's first bytes: png, jpeg,
// gif (decoded here), heic, webp, tiff, bmp (through the Converter); ""
// when it is none of them.
func imageFormat(path string) string {
	head := readHead(path, 32)
	switch {
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF}):
		return "jpeg"
	case bytes.HasPrefix(head, []byte("GIF8")):
		return "gif"
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return "webp"
	case bytes.HasPrefix(head, []byte("II*\x00")), bytes.HasPrefix(head, []byte("MM\x00*")):
		return "tiff"
	case bytes.HasPrefix(head, []byte("BM")):
		return "bmp"
	case len(head) >= 12 && string(head[4:8]) == "ftyp":
		switch string(head[8:12]) {
		case "heic", "heix", "hevc", "heim", "heis", "mif1", "msf1", "avif":
			return "heic"
		}
	}
	return ""
}

// videoMIME is video/mp4 or video/quicktime by the file's first bytes, ""
// for anything else.
func videoMIME(path string) string {
	head := readHead(path, 16)
	if len(head) < 12 {
		return ""
	}
	switch string(head[4:8]) {
	case "ftyp":
		switch string(head[8:12]) {
		case "qt  ":
			return "video/quicktime"
		case "isom", "iso2", "iso4", "iso5", "iso6", "mp41", "mp42", "avc1", "M4V ", "M4VH", "M4VP", "dash", "mmp4", "MSNV":
			return "video/mp4"
		}
	case "moov", "mdat", "wide", "free", "skip", "pnot":
		return "video/quicktime" // old QuickTime files start with an atom, no ftyp
	}
	return ""
}

func readHead(path string, n int) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	b := make([]byte, n)
	m, _ := io.ReadFull(f, b)
	return b[:m]
}

// hashFile is a file's size and SHA-256 (hex).
func hashFile(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// PrepareImage writes src re-encoded into dir as <name>.png or .jpg: its
// orientation applied, scaled to MaxImageSide, every metadata (EXIF,
// location) gone, at most MaxImageBytes. A PNG or GIF stays a PNG when it
// fits (screenshots keep their sharp text); otherwise JPEG at falling
// quality, then smaller.
func PrepareImage(ctx context.Context, conv Converter, src, dir, name string) (File, error) {
	if pr := CheckImage(src); pr != "" {
		if pr == ProblemImageType {
			return File{}, ErrNotImage
		}
		return File{}, fmt.Errorf("feedback: %s: %s", filepath.Base(src), pr)
	}
	format := imageFormat(src)
	path := src
	if format != "png" && format != "jpeg" && format != "gif" {
		if conv == nil {
			return File{}, ErrUnsupported
		}
		tmp := filepath.Join(dir, "."+name+".convert.png")
		defer os.Remove(tmp)
		if err := conv.ToPNG(ctx, src, tmp); err != nil {
			return File{}, fmt.Errorf("feedback: converting %s: %w", filepath.Base(src), err)
		}
		path, format = tmp, "png"
	}
	img, err := decodeImage(path, format)
	if err != nil {
		return File{}, err
	}
	keepPNG := format == "png" || format == "gif"
	data, ext, w, h, err := encodeImage(ctx, img, keepPNG, MaxImageBytes)
	if err != nil {
		return File{}, err
	}
	out := name + ext
	if err := os.WriteFile(filepath.Join(dir, out), data, 0o600); err != nil {
		return File{}, err
	}
	sum := sha256.Sum256(data)
	mime := "image/jpeg"
	if ext == ".png" {
		mime = "image/png"
	}
	return File{Ref: name, Role: RoleImage, Name: out, MIME: mime, Bytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:]),
		Width: w, Height: h, Original: filepath.Base(src)}, nil
}

func decodeImage(path, format string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotImage, err)
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return nil, fmt.Errorf("%w: %d×%d pixels", ErrTooLarge, cfg.Width, cfg.Height)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var img image.Image
	switch format {
	case "png":
		img, err = png.Decode(f)
	case "jpeg":
		img, err = jpeg.Decode(f)
	case "gif":
		img, err = gif.Decode(f) // the first frame
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotImage, err)
	}
	rgba := toRGBA(img)
	if format == "jpeg" {
		rgba = orient(rgba, jpegOrientation(path))
	}
	return rgba, nil
}

func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok && r.Rect.Min == (image.Point{}) {
		return r
	}
	b := img.Bounds()
	r := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(r, r.Rect, img, b.Min, draw.Src)
	return r
}

// encodeImage encodes img within limit bytes.
func encodeImage(ctx context.Context, img *image.RGBA, keepPNG bool, limit int) (data []byte, ext string, w, h int, err error) {
	img = fit(img, MaxImageSide)
	for round := 0; round < 8; round++ {
		if err := ctx.Err(); err != nil {
			return nil, "", 0, 0, err
		}
		w, h = img.Rect.Dx(), img.Rect.Dy()
		if keepPNG {
			var buf bytes.Buffer
			enc := png.Encoder{CompressionLevel: png.BestCompression}
			if err := enc.Encode(&buf, img); err != nil {
				return nil, "", 0, 0, err
			}
			if buf.Len() <= limit {
				return buf.Bytes(), ".png", w, h, nil
			}
		}
		flat := flatten(img)
		for _, q := range []int{90, 82, 72} {
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: q}); err != nil {
				return nil, "", 0, 0, err
			}
			if buf.Len() <= limit {
				return buf.Bytes(), ".jpg", w, h, nil
			}
		}
		if w < 64 || h < 64 {
			break
		}
		img = scale(img, w*3/4, h*3/4)
	}
	return nil, "", 0, 0, ErrTooLarge
}

// fit scales img down so its longest side is at most side.
func fit(img *image.RGBA, side int) *image.RGBA {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w <= side && h <= side {
		return img
	}
	if w >= h {
		return scale(img, side, max(1, h*side/w))
	}
	return scale(img, max(1, w*side/h), side)
}

// scale is an area-average (box) downscale; it does not enlarge.
func scale(src *image.RGBA, dw, dh int) *image.RGBA {
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	if dw >= sw && dh >= sh {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		y0, y1 := y*sh/dh, max((y+1)*sh/dh, y*sh/dh+1)
		for x := 0; x < dw; x++ {
			x0, x1 := x*sw/dw, max((x+1)*sw/dw, x*sw/dw+1)
			var r, g, b, a, n uint32
			for sy := y0; sy < y1; sy++ {
				row := src.Pix[sy*src.Stride:]
				for sx := x0; sx < x1; sx++ {
					p := row[sx*4 : sx*4+4]
					r += uint32(p[0])
					g += uint32(p[1])
					b += uint32(p[2])
					a += uint32(p[3])
					n++
				}
			}
			o := dst.Pix[y*dst.Stride+x*4:]
			o[0], o[1], o[2], o[3] = uint8(r/n), uint8(g/n), uint8(b/n), uint8(a/n)
		}
	}
	return dst
}

// flatten puts img on white (JPEG has no alpha); an opaque image is
// returned as it is.
func flatten(img *image.RGBA) *image.RGBA {
	if img.Opaque() {
		return img
	}
	out := image.NewRGBA(img.Rect)
	draw.Draw(out, out.Rect, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(out, out.Rect, img, img.Rect.Min, draw.Over)
	return out
}

// jpegOrientation is the EXIF orientation of a JPEG file (1 when none).
func jpegOrientation(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 1
	}
	defer f.Close()
	head := make([]byte, 128<<10)
	n, _ := io.ReadFull(f, head)
	return exifOrientation(head[:n])
}

// exifOrientation reads tag 0x0112 from the APP1 Exif segment of JPEG
// bytes.
func exifOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return 1
		}
		marker := b[i+1]
		if marker == 0xDA || marker == 0xD9 { // start of scan, end
			return 1
		}
		size := int(binary.BigEndian.Uint16(b[i+2:]))
		if size < 2 || i+2+size > len(b) {
			return 1
		}
		seg := b[i+4 : i+2+size]
		if marker == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 1
}

func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	off := int(bo.Uint32(t[4:]))
	if off < 8 || off+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[off:]))
	for k := 0; k < n; k++ {
		e := off + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			v := int(bo.Uint16(t[e+8:]))
			if v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// orient applies an EXIF orientation (2–8) so the image is upright.
func orient(src *image.RGBA, o int) *image.RGBA {
	if o <= 1 || o > 8 {
		return src
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			copy(dst.Pix[dy*dst.Stride+dx*4:dy*dst.Stride+dx*4+4], src.Pix[y*src.Stride+x*4:y*src.Stride+x*4+4])
		}
	}
	return dst
}

// PrepareVideo copies src into dir as <name>.mp4/.mov when it fits
// MaxVideoBytes; a larger one is compressed with VideoPresets in turn
// (conv; macOS's avconvert drops the location and other private metadata)
// until it fits, or ErrTooLarge.
func PrepareVideo(ctx context.Context, conv Converter, src, dir, name string) (File, error) {
	switch CheckVideo(src) {
	case "":
	case ProblemVideoType:
		return File{}, ErrNotVideo
	default:
		return File{}, fmt.Errorf("feedback: %s: cannot read it, or larger than 1 GB", filepath.Base(src))
	}
	mime := videoMIME(src)
	ext := ".mp4"
	if mime == "video/quicktime" {
		ext = ".mov"
	}
	st, err := os.Stat(src)
	if err != nil {
		return File{}, err
	}
	out := filepath.Join(dir, name+ext)
	if st.Size() <= MaxVideoBytes {
		if err := copyFile(src, out); err != nil {
			return File{}, err
		}
	} else {
		if conv == nil {
			return File{}, ErrTooLarge
		}
		out = filepath.Join(dir, name+".mp4")
		mime = "video/mp4"
		fitted := false
		for _, p := range VideoPresets {
			if err := conv.CompressVideo(ctx, src, out, p); err != nil {
				if ctx.Err() != nil {
					return File{}, ctx.Err()
				}
				continue // a preset the source cannot use (e.g. smaller already)
			}
			if fi, err := os.Stat(out); err == nil && fi.Size() <= MaxVideoBytes {
				if videoMIME(out) == "video/quicktime" {
					mime = "video/quicktime"
				}
				fitted = true
				break
			}
		}
		if !fitted {
			os.Remove(out)
			return File{}, ErrTooLarge
		}
	}
	n, sum, err := hashFile(out)
	if err != nil {
		return File{}, err
	}
	return File{Ref: name, Role: RoleVideo, Name: filepath.Base(out), MIME: mime, Bytes: n, SHA256: sum, Original: filepath.Base(src)}, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
