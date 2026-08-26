package pipeline

import (
	"image"
	"image/color"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
)

// solidFrame creates a w×h image filled with the given color and alpha.
func solidFrame(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestAssembleFilmstrip(t *testing.T) {
	canvas, _ := identity.NewCanvasSpec(32, 32)
	layout, _ := NormalizeFrameList(*canvas, 3)
	colors := []color.RGBA{
		{R: 255, A: 255},
		{G: 255, A: 255},
		{B: 255, A: 255},
	}
	frames := []*image.RGBA{
		solidFrame(32, 32, colors[0]),
		solidFrame(32, 32, colors[1]),
		solidFrame(32, 32, colors[2]),
	}
	strip, err := AssembleFilmstrip(frames, layout)
	if err != nil {
		t.Fatalf("AssembleFilmstrip: %v", err)
	}
	if strip.Bounds().Dx() != 96 || strip.Bounds().Dy() != 32 {
		t.Fatalf("strip = %dx%d, want 96x32", strip.Bounds().Dx(), strip.Bounds().Dy())
	}
	// Verify each cell holds its frame color (single-call filmstrip contains
	// all frames in order — task 5.2).
	for i, c := range colors {
		got := strip.RGBAAt(i*32+16, 16)
		if got.R != c.R || got.G != c.G || got.B != c.B || got.A != 255 {
			t.Errorf("cell %d color = %v, want %v", i, got, c)
		}
	}
}

func TestAssembleFilmstripMismatch(t *testing.T) {
	canvas, _ := identity.NewCanvasSpec(32, 32)
	layout, _ := NormalizeFrameList(*canvas, 4)
	if _, err := AssembleFilmstrip([]*image.RGBA{solidFrame(32, 32, color.RGBA{A: 255})}, layout); err == nil {
		t.Error("frame count mismatch: expected error")
	}
}

func TestFilmstripPNGRoundTrip(t *testing.T) {
	canvas, _ := identity.NewCanvasSpec(16, 16)
	layout, _ := NormalizeFrameList(*canvas, 2)
	strip, err := AssembleFilmstrip([]*image.RGBA{
		solidFrame(16, 16, color.RGBA{R: 200, A: 255}),
		solidFrame(16, 16, color.RGBA{B: 200, A: 255}),
	}, layout)
	if err != nil {
		t.Fatalf("AssembleFilmstrip: %v", err)
	}
	data, err := EncodeFilmstripPNG(strip)
	if err != nil {
		t.Fatalf("EncodeFilmstripPNG: %v", err)
	}
	back, err := DecodeFilmstrip(data)
	if err != nil {
		t.Fatalf("DecodeFilmstrip: %v", err)
	}
	if back.Bounds().Dx() != strip.Bounds().Dx() || back.Bounds().Dy() != strip.Bounds().Dy() {
		t.Fatalf("round trip size = %dx%d, want %dx%d", back.Bounds().Dx(), back.Bounds().Dy(), strip.Bounds().Dx(), strip.Bounds().Dy())
	}
	if got := back.RGBAAt(8, 8); got.R != 200 || got.A != 255 {
		t.Errorf("cell 0 color = %v, want R=200 A=255", got)
	}
	if got := back.RGBAAt(24, 8); got.B != 200 || got.A != 255 {
		t.Errorf("cell 1 color = %v, want B=200 A=255", got)
	}
}
