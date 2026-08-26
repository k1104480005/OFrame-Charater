package pipeline

import (
	"image"
	"image/color"
	"testing"
)

// manyColorsFrame builds a frame whose pixels use the given number of distinct
// colors (cycling through them), all opaque.
func manyColorsFrame(n int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			i := (y*64 + x) % n
			c := color.RGBA{R: uint8((i * 7) % 256), G: uint8((i * 13) % 256), B: uint8((i * 29) % 256), A: 255}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestBuildSharedPaletteCap(t *testing.T) {
	frames := []*image.RGBA{manyColorsFrame(100), manyColorsFrame(100)}
	palette, err := BuildSharedPalette(frames, 32)
	if err != nil {
		t.Fatalf("BuildSharedPalette: %v", err)
	}
	if len(palette) != 32 {
		t.Fatalf("palette = %d colors, want 32 (cap)", len(palette))
	}
	// Palette entries must be unique.
	seen := map[color.RGBA]bool{}
	for _, p := range palette {
		if seen[p] {
			t.Fatalf("duplicate palette entry %v", p)
		}
		seen[p] = true
	}
}

func TestBuildSharedPaletteFrequencyOrder(t *testing.T) {
	// Frame with one dominant color (10x10 = 100 px) and one rare color (1 px).
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 10, G: 10, B: 10, A: 255})
		}
	}
	img.SetRGBA(31, 31, color.RGBA{R: 200, G: 0, B: 0, A: 255})
	palette, err := BuildSharedPalette([]*image.RGBA{img}, 32)
	if err != nil {
		t.Fatalf("BuildSharedPalette: %v", err)
	}
	if len(palette) != 2 {
		t.Fatalf("palette = %d, want 2", len(palette))
	}
	if palette[0] != (color.RGBA{R: 10, G: 10, B: 10, A: 255}) {
		t.Errorf("palette[0] = %v, want the dominant color first", palette[0])
	}
}

func TestQuantizeToPalette(t *testing.T) {
	frames := []*image.RGBA{manyColorsFrame(100)}
	palette, err := BuildSharedPalette(frames, 16)
	if err != nil {
		t.Fatalf("BuildSharedPalette: %v", err)
	}
	quantized, err := QuantizeToPalette(frames, palette)
	if err != nil {
		t.Fatalf("QuantizeToPalette: %v", err)
	}
	// Every opaque pixel must now be exactly a palette color (alpha kept).
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			c := quantized[0].RGBAAt(x, y)
			if c.A != 255 {
				t.Fatalf("alpha lost at (%d,%d): %v", x, y, c)
			}
			if !colorInPalette(c, palette) {
				t.Fatalf("pixel (%d,%d) = %v not in palette", x, y, c)
			}
		}
	}
}

func TestQuantizePreservesTransparency(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	img.SetRGBA(0, 0, color.RGBA{R: 5, G: 6, B: 7, A: 128}) // semi-transparent
	palette, _ := BuildSharedPalette([]*image.RGBA{img}, 32)
	quantized, err := QuantizeToPalette([]*image.RGBA{img}, palette)
	if err != nil {
		t.Fatalf("QuantizeToPalette: %v", err)
	}
	if got := quantized[0].RGBAAt(0, 0); got.A != 128 {
		t.Errorf("alpha = %d, want 128 preserved", got.A)
	}
	if got := quantized[0].RGBAAt(5, 5); got.A != 0 {
		t.Errorf("transparent pixel alpha = %d, want 0", got.A)
	}
}

func TestNearestPaletteColorTie(t *testing.T) {
	palette := []color.RGBA{
		{R: 0, G: 0, B: 0, A: 255},
		{R: 255, G: 0, B: 0, A: 255},
	}
	// (0,0,0) is equidistant from (1,0,0)? No — nearest is black. Use a color
	// exactly on the boundary between two entries: (128,0,0) → distance to
	// black 128², to red 127² → red wins. Pick (127,0,0): 127² vs 128² →
	// black. The tie case: (0,0,0) vs palette containing black twice is
	// disallowed (unique). Instead verify deterministic nearest behavior.
	got := NearestPaletteColor(color.RGBA{R: 200, G: 0, B: 0, A: 255}, palette)
	if got != palette[1] {
		t.Errorf("nearest = %v, want red", got)
	}
	got = NearestPaletteColor(color.RGBA{R: 10, G: 0, B: 0, A: 255}, palette)
	if got != palette[0] {
		t.Errorf("nearest = %v, want black", got)
	}
}

func TestPaletteDeterministic(t *testing.T) {
	frames := []*image.RGBA{manyColorsFrame(50)}
	a, _ := BuildSharedPalette(frames, 24)
	b, _ := BuildSharedPalette(frames, 24)
	if len(a) != len(b) {
		t.Fatalf("palette sizes differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("palette[%d] differs: %v vs %v", i, a[i], b[i])
		}
	}
}
