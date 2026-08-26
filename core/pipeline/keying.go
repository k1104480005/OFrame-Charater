package pipeline

import (
	"image"
	"image/color"
	"math"
)

// KeyOptions tunes the deterministic YCbCr chroma key (task 5.4 透明背景处理;
// 洋红仅用于抠图技术背景).
type KeyOptions struct {
	// KeyColor is the technical matting background color (default magenta
	// #FF00FF).
	KeyColor color.RGBA
	// CbCrTolerance is the normalized chroma distance (0..1) below which a
	// pixel is key-like (default 0.25).
	CbCrTolerance float64
	// DespillStrength scales the linear falloff that removes key-color spill
	// from fringe pixels (default 0.6).
	DespillStrength float64
	// DespillRange is the normalized chroma distance within which fringe
	// pixels (those above the key tolerance but close to the key color) get
	// despilled (default 0.45).
	DespillRange float64
	// NoFloodFill disables border flood fill: when false (default), background
	// removal is restricted to key-like pixels connected to the image border,
	// preserving interior key-colored details; when true, every key-like pixel
	// becomes background.
	NoFloodFill bool
}

// DefaultKeyColor is the magenta technical matting background (洋红仅用于抠图
// 技术背景).
var DefaultKeyColor = color.RGBA{R: 255, G: 0, B: 255, A: 255}

func (o KeyOptions) withDefaults() KeyOptions {
	if o.KeyColor == (color.RGBA{}) {
		o.KeyColor = DefaultKeyColor
	}
	if o.CbCrTolerance == 0 {
		o.CbCrTolerance = 0.25
	}
	if o.DespillStrength == 0 {
		o.DespillStrength = 0.6
	}
	if o.DespillRange == 0 {
		o.DespillRange = 0.45
	}
	return o
}

// chromaDist returns the normalized YCbCr chroma distance (0..1) between two
// RGB colors, ignoring luminance so keying is robust to shading.
func chromaDist(a, b color.RGBA) float64 {
	_, acb, acr := color.RGBToYCbCr(a.R, a.G, a.B)
	_, bcb, bcr := color.RGBToYCbCr(b.R, b.G, b.B)
	// Subtract as ints: the uint8 components would wrap otherwise.
	dcb := float64(int(acb) - int(bcb))
	dcr := float64(int(acr) - int(bcr))
	return math.Hypot(dcb, dcr) / (255.0 * math.Sqrt2)
}

// KeyChroma removes the technical background from img using YCbCr chroma
// keying with despill and border flood fill (task 5.4):
//
//   - key-like pixels (chroma distance < CbCrTolerance) connected to the image
//     border become fully transparent, with RGB kept at the key color (the
//     magenta technical background of alpha-check views);
//   - key-like pixels NOT connected to the border (interior details such as a
//     magenta button on the character) are preserved untouched;
//   - fringe pixels (above the key tolerance but within DespillRange) have the
//     key color's contribution removed with a linear falloff, eliminating
//     magenta spill on the character's edges.
//
// The result is deterministic for identical input.
func KeyChroma(img *image.RGBA, opts KeyOptions) *image.RGBA {
	opts = opts.withDefaults()
	key := opts.KeyColor
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewRGBA(b)

	keyLike := make([]bool, w*h)
	dists := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			c := img.RGBAAt(b.Min.X+x, b.Min.Y+y)
			if c.A == 0 {
				// Transparent source pixels act as empty space for flood-fill
				// connectivity.
				keyLike[i] = true
				continue
			}
			d := chromaDist(c, key)
			dists[i] = d
			keyLike[i] = d < opts.CbCrTolerance
		}
	}

	// Background = key-like pixels reachable from the border (flood fill).
	background := make([]bool, w*h)
	if !opts.NoFloodFill {
		queue := make([]int, 0, w*h)
		mark := func(x, y int) {
			i := y*w + x
			if !background[i] && keyLike[i] {
				background[i] = true
				queue = append(queue, i)
			}
		}
		for x := 0; x < w; x++ {
			mark(x, 0)
			mark(x, h-1)
		}
		for y := 0; y < h; y++ {
			mark(0, y)
			mark(w-1, y)
		}
		for len(queue) > 0 {
			i := queue[0]
			queue = queue[1:]
			x, y := i%w, i/w
			if x > 0 {
				mark(x-1, y)
			}
			if x < w-1 {
				mark(x+1, y)
			}
			if y > 0 {
				mark(x, y-1)
			}
			if y < h-1 {
				mark(x, y+1)
			}
		}
	} else {
		copy(background, keyLike)
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			c := img.RGBAAt(b.Min.X+x, b.Min.Y+y)
			if background[i] {
				// 洋红技术背景: RGB kept at the key color, alpha 0.
				out.SetRGBA(x, y, color.RGBA{R: key.R, G: key.G, B: key.B, A: 0})
				continue
			}
			// Despill: fringe pixels above the key tolerance but within the
			// despill range lose the key color's contribution linearly.
			d := dists[i]
			if d >= opts.CbCrTolerance && d < opts.DespillRange {
				spill := opts.DespillStrength * (1 - d/opts.DespillRange)
				r := clampInt(int(float64(c.R)-float64(key.R)*spill+0.5), 0, 255)
				g := clampInt(int(float64(c.G)-float64(key.G)*spill+0.5), 0, 255)
				bl := clampInt(int(float64(c.B)-float64(key.B)*spill+0.5), 0, 255)
				c = color.RGBA{R: uint8(r), G: uint8(g), B: uint8(bl), A: c.A}
			}
			out.SetRGBA(x, y, c)
		}
	}
	return out
}
