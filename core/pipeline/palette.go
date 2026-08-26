package pipeline

import (
	"image"
	"image/color"
	"sort"
)

// PaletteOptions controls shared palette quantization (task 5.4: 共享调色板量化
// 与调色板一致性).
type PaletteOptions struct {
	// MaxColors caps the shared palette size (default 32).
	MaxColors int
}

// DefaultMaxPaletteColors is the default shared palette size cap.
const DefaultMaxPaletteColors = 32

// BuildSharedPalette builds the shared palette from the union of opaque pixel
// colors across all frames of a sequence: ranked by frequency (ties broken by
// R, G, B for determinism) and capped at maxColors. Because the palette is
// shared by every frame, quantizing to it guarantees 色数/调色板一致性 across the
// whole motion (and, via the same palette, across mirrored directions).
func BuildSharedPalette(frames []*image.RGBA, maxColors int) ([]color.RGBA, error) {
	if maxColors <= 0 {
		maxColors = DefaultMaxPaletteColors
	}
	counts := map[color.RGBA]int{}
	for _, f := range frames {
		if f == nil {
			continue
		}
		b := f.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := f.RGBAAt(x, y)
				if c.A == 0 {
					continue
				}
				counts[c]++
			}
		}
	}
	if len(counts) == 0 {
		return nil, nil
	}
	keys := make([]color.RGBA, 0, len(counts))
	for c := range counts {
		keys = append(keys, c)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if counts[a] != counts[b] {
			return counts[a] > counts[b]
		}
		if a.R != b.R {
			return a.R < b.R
		}
		if a.G != b.G {
			return a.G < b.G
		}
		if a.B != b.B {
			return a.B < b.B
		}
		return a.A < b.A
	})
	if len(keys) > maxColors {
		keys = keys[:maxColors]
	}
	return keys, nil
}

// QuantizeToPalette maps every opaque pixel of each frame to its nearest
// palette color (Euclidean RGB distance, ties broken by palette index);
// alpha is preserved. The result is deterministic for identical input.
func QuantizeToPalette(frames []*image.RGBA, palette []color.RGBA) ([]*image.RGBA, error) {
	if len(palette) == 0 {
		return frames, nil
	}
	out := make([]*image.RGBA, 0, len(frames))
	for _, f := range frames {
		if f == nil {
			continue
		}
		b := f.Bounds()
		nf := image.NewRGBA(b)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := f.RGBAAt(x, y)
				if c.A == 0 {
					continue
				}
				p := NearestPaletteColor(c, palette)
				nf.SetRGBA(x, y, color.RGBA{R: p.R, G: p.G, B: p.B, A: c.A})
			}
		}
		out = append(out, nf)
	}
	return out, nil
}

// NearestPaletteColor returns the palette entry nearest to c by squared
// Euclidean RGB distance; ties resolve to the earliest palette index
// (deterministic).
func NearestPaletteColor(c color.RGBA, palette []color.RGBA) color.RGBA {
	best := palette[0]
	bestD := distSq(c, best)
	for _, p := range palette[1:] {
		if d := distSq(c, p); d < bestD {
			bestD = d
			best = p
		}
	}
	return best
}

func distSq(a, b color.RGBA) int {
	dr := int(a.R) - int(b.R)
	dg := int(a.G) - int(b.G)
	db := int(a.B) - int(b.B)
	return dr*dr + dg*dg + db*db
}
