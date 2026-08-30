package pipeline

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
)

// DecodeImageAny decodes a provider-returned image in any registered raster
// format (PNG/JPEG/GIF, plus WebP/BMP when a decoder is registered by the
// host build) and normalizes it to *image.RGBA. Providers do not guarantee
// PNG, so every single-image consumer (base character) MUST decode and
// re-encode via EncodeFilmstripPNG before persisting — a raw JPEG/WebP payload
// renamed to .png is not a valid PNG artifact.
func DecodeImageAny(data []byte) (*image.RGBA, error) {
	if img, err := png.Decode(bytes.NewReader(data)); err == nil {
		return ToRGBA(img), nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("pipeline: decode image: %w", err)
	}
	return ToRGBA(img), nil
}
