package pipeline

import (
	"fmt"
	"image"
)

// GridOptions controls pixel-grid correction (task 5.4: 像素网格校正, 裁边与
// 留白按逻辑画布对齐).
type GridOptions struct {
	// Step is the pixel grid step; content offsets are snapped to multiples
	// of Step (default 1 = already on the pixel grid, no snapping).
	Step int
	// Margin is the integer margin kept around the content bounding box when
	// cropping (default 0).
	Margin int
}

// FitToCanvas fits src onto a tw×th canvas using integer-pixel rules only:
// exact match → copy; integer-factor nearest-neighbor scaling when one size is
// an exact integer multiple of the other; otherwise integer-centered
// placement with transparent padding/cropping. Never interpolates
// (task 5.3: 无二次插值模糊).
func FitToCanvas(src *image.RGBA, tw, th int) (*image.RGBA, error) {
	if tw <= 0 || th <= 0 {
		return nil, fmt.Errorf("pipeline: invalid target canvas %dx%d", tw, th)
	}
	if src == nil {
		return nil, fmt.Errorf("pipeline: source image is nil")
	}
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if sw == tw && sh == th {
		out := image.NewRGBA(image.Rect(0, 0, tw, th))
		copy(out.Pix, src.Pix)
		return out, nil
	}
	if sw >= tw && sh >= th && sw%tw == 0 && sh%th == 0 {
		return ScaleNearest(src, tw, th), nil
	}
	if tw%sw == 0 && th%sh == 0 {
		return ScaleNearest(src, tw, th), nil
	}
	return CenterPlace(src, tw, th), nil
}

// ScaleNearest scales by nearest-neighbor sampling (integer pixel rules, no
// interpolation). With integer factors this duplicates or drops whole pixels
// deterministically.
func ScaleNearest(src *image.RGBA, tw, th int) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	out := image.NewRGBA(image.Rect(0, 0, tw, th))
	for y := 0; y < th; y++ {
		sy := y * sh / th
		for x := 0; x < tw; x++ {
			sx := x * sw / tw
			out.SetRGBA(x, y, src.RGBAAt(sx, sy))
		}
	}
	return out
}

// CenterPlace places src at integer-centered offsets onto a tw×th canvas,
// cropping overflow and padding underflow with transparency.
func CenterPlace(src *image.RGBA, tw, th int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, tw, th))
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dx := (tw - sw) / 2
	dy := (th - sh) / 2
	for y := 0; y < sh; y++ {
		oy := dy + y
		if oy < 0 || oy >= th {
			continue
		}
		for x := 0; x < sw; x++ {
			ox := dx + x
			if ox < 0 || ox >= tw {
				continue
			}
			out.SetRGBA(ox, oy, src.RGBAAt(x, y))
		}
	}
	return out
}

// CropToContent crops the alpha bounding box of img expanded by margin
// (integer), so the content's top-left lands at (0,0). Returns an empty
// 0×0 image when there is no opaque content.
func CropToContent(img *image.RGBA, margin int) *image.RGBA {
	if img == nil {
		return nil
	}
	b := img.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y).A == 0 {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < minX || maxY < minY {
		return image.NewRGBA(image.Rect(0, 0, 0, 0))
	}
	minX -= margin
	minY -= margin
	maxX += margin
	maxY += margin
	if minX < b.Min.X {
		minX = b.Min.X
	}
	if minY < b.Min.Y {
		minY = b.Min.Y
	}
	if maxX > b.Max.X-1 {
		maxX = b.Max.X - 1
	}
	if maxY > b.Max.Y-1 {
		maxY = b.Max.Y - 1
	}
	w, h := maxX-minX+1, maxY-minY+1
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out.SetRGBA(x, y, img.RGBAAt(minX+x, minY+y))
		}
	}
	return out
}

// PadToCanvas places img at the integer offset (dx, dy) on a w×h transparent
// canvas, cropping overflow. Content placement is fully integer (像素网格).
func PadToCanvas(img *image.RGBA, w, h, dx, dy int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	if img == nil {
		return out
	}
	sw, sh := img.Bounds().Dx(), img.Bounds().Dy()
	for y := 0; y < sh; y++ {
		oy := dy + y
		if oy < 0 || oy >= h {
			continue
		}
		for x := 0; x < sw; x++ {
			ox := dx + x
			if ox < 0 || ox >= w {
				continue
			}
			out.SetRGBA(ox, oy, img.RGBAAt(x, y))
		}
	}
	return out
}

// GridOffset floors v down to the nearest multiple of step (integer pixel
// grid). A step <= 1 returns v unchanged.
func GridOffset(v, step int) int {
	if step <= 1 {
		return v
	}
	mod := ((v % step) + step) % step
	return v - mod
}

// GridCorrect applies the pixel-grid correction to one frame (task 5.4): crop
// the alpha bounding box with margin, snap the centered placement offset to
// the pixel grid, and pad back to the logical canvas size — 裁边与留白按逻辑画布
// 对齐, all in integer pixels.
func GridCorrect(img *image.RGBA, w, h int, opts GridOptions) *image.RGBA {
	if img == nil {
		return image.NewRGBA(image.Rect(0, 0, w, h))
	}
	step := opts.Step
	if step <= 0 {
		step = 1
	}
	content := CropToContent(img, opts.Margin)
	sw, sh := content.Bounds().Dx(), content.Bounds().Dy()
	dx := GridOffset((w-sw)/2, step)
	dy := GridOffset((h-sh)/2, step)
	return PadToCanvas(content, w, h, dx, dy)
}
