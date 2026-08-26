package pipeline

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
)

// Filmstrip is the horizontal filmstrip of one motion (task 5.2): all frames
// laid side by side in frame order, generated with a single prompt so
// inter-frame consistency is guaranteed. The original image and the immutable
// prompt snapshot are preserved for regeneration and audit (保留原始产物与
// prompt 快照).
type Filmstrip struct {
	Image  *image.RGBA
	Layout FrameList
	Prompt PromptSnapshot
}

// AssembleFilmstrip deterministically builds a horizontal filmstrip by copying
// each frame at its normalized integer position with nearest-neighbor
// semantics (no interpolation). This is the pipeline-side filmstrip
// construction used by synthetic/offline generation and deterministic tests;
// provider output is decoded with DecodeFilmstrip.
func AssembleFilmstrip(frames []*image.RGBA, layout FrameList) (*image.RGBA, error) {
	if err := layout.ValidateFrames(len(frames)); err != nil {
		return nil, err
	}
	if layout.StripWidth <= 0 || layout.StripHeight <= 0 {
		return nil, fmt.Errorf("pipeline: invalid filmstrip layout %dx%d", layout.StripWidth, layout.StripHeight)
	}
	out := image.NewRGBA(image.Rect(0, 0, layout.StripWidth, layout.StripHeight))
	for i, f := range frames {
		spec := layout.Frames[i]
		fw, fh := f.Bounds().Dx(), f.Bounds().Dy()
		// Integer-pixel placement, centered within the cell.
		dx := (spec.Width - fw) / 2
		dy := (spec.Height - fh) / 2
		for y := 0; y < fh; y++ {
			oy := spec.Y + dy + y
			if oy < 0 || oy >= layout.StripHeight {
				continue
			}
			for x := 0; x < fw; x++ {
				ox := spec.X + dx + x
				if ox < 0 || ox >= layout.StripWidth {
					continue
				}
				out.SetRGBA(ox, oy, f.RGBAAt(x, y))
			}
		}
	}
	return out, nil
}

// DecodeFilmstrip decodes a raw filmstrip image from PNG bytes (the original
// artifact returned by a provider call).
func DecodeFilmstrip(data []byte) (*image.RGBA, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("pipeline: decode filmstrip: %w", err)
	}
	return ToRGBA(img), nil
}

// EncodeFilmstripPNG encodes a filmstrip as PNG bytes for preservation
// (原始产物保留).
func EncodeFilmstripPNG(img *image.RGBA) ([]byte, error) {
	if img == nil {
		return nil, fmt.Errorf("pipeline: filmstrip is nil")
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("pipeline: encode filmstrip: %w", err)
	}
	return buf.Bytes(), nil
}

// ToRGBA converts any image to an independent *image.RGBA.
func ToRGBA(src image.Image) *image.RGBA {
	if r, ok := src.(*image.RGBA); ok {
		b := r.Bounds()
		cp := image.NewRGBA(b)
		copy(cp.Pix, r.Pix)
		return cp
	}
	b := src.Bounds()
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.Set(x, y, src.At(x, y))
		}
	}
	return out
}
