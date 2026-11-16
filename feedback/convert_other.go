//go:build !darwin

package feedback

import "context"

// SystemConverter converts nothing outside macOS: HEIC, WebP, TIFF and BMP
// images and videos over MaxVideoBytes are refused (ErrUnsupported).
func SystemConverter() Converter { return noConverter{} }

type noConverter struct{}

func (noConverter) ToPNG(context.Context, string, string) error { return ErrUnsupported }

func (noConverter) CompressVideo(context.Context, string, string, string) error {
	return ErrUnsupported
}
