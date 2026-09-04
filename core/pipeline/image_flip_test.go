package pipeline

import (
	"image"
	"image/color"
	"testing"
)

func TestFlipHorizontalMirrorsColumns(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 255}) // red .....
	img.SetRGBA(2, 1, color.RGBA{B: 255, A: 255}) // ...... blue

	got := FlipHorizontal(img)

	if got.RGBAAt(2, 0) != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("red column not mirrored to the right edge: %v", got.RGBAAt(2, 0))
	}
	if got.RGBAAt(0, 1) != (color.RGBA{B: 255, A: 255}) {
		t.Fatalf("blue column not mirrored to the left edge: %v", got.RGBAAt(0, 1))
	}
	if got.RGBAAt(1, 0) != (color.RGBA{}) || got.RGBAAt(1, 1) != (color.RGBA{}) {
		t.Fatal("middle column must be untouched")
	}
}

func TestFlipHorizontalIsItsOwnInverse(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 5, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 5; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 40), G: uint8(y * 50), A: 255})
		}
	}

	once := FlipHorizontal(img)
	twice := FlipHorizontal(once)

	for y := 0; y < 4; y++ {
		for x := 0; x < 5; x++ {
			if got, want := twice.RGBAAt(x, y), img.RGBAAt(x, y); got != want {
				t.Fatalf("double flip differs at (%d,%d): got %v want %v", x, y, got, want)
			}
		}
	}
}

func TestFlipHorizontalPreservesAlphaAndSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 128})
	img.SetRGBA(1, 0, color.RGBA{A: 255})

	got := FlipHorizontal(img)
	if got.Bounds().Dx() != 2 || got.Bounds().Dy() != 2 {
		t.Fatalf("bounds changed: %v", got.Bounds())
	}
	if got.RGBAAt(1, 0).A != 128 || got.RGBAAt(1, 0).R != 10 {
		t.Fatalf("semi-transparent pixel not preserved through mirror: %v", got.RGBAAt(1, 0))
	}
	if got.RGBAAt(0, 0).A != 255 {
		t.Fatalf("opaque pixel not preserved through mirror: %v", got.RGBAAt(0, 0))
	}
}

func TestFlipHorizontalNilInput(t *testing.T) {
	if got := FlipHorizontal(nil); got != nil {
		t.Fatalf("FlipHorizontal(nil) = %v, want nil", got)
	}
}
