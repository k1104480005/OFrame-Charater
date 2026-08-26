package pipeline

import (
	"image"
	"image/color"
	"testing"
)

// contentFrame builds a 32×32 frame with a 10×10 opaque block at (10,12).
func contentFrame() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 12; y < 22; y++ {
		for x := 10; x < 20; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 90, G: 140, B: 30, A: 255})
		}
	}
	return img
}

func TestCropToContent(t *testing.T) {
	crop := CropToContent(contentFrame(), 0)
	if crop.Bounds().Dx() != 10 || crop.Bounds().Dy() != 10 {
		t.Fatalf("crop = %dx%d, want 10x10", crop.Bounds().Dx(), crop.Bounds().Dy())
	}
	if got := crop.RGBAAt(0, 0); got.R != 90 || got.G != 140 || got.B != 30 {
		t.Errorf("crop origin pixel = %v, want the content color", got)
	}
}

func TestCropToContentMargin(t *testing.T) {
	crop := CropToContent(contentFrame(), 2)
	// Bounding box 10..19 × 12..21 + 2px margin each side → 14×14.
	if crop.Bounds().Dx() != 14 || crop.Bounds().Dy() != 14 {
		t.Fatalf("crop with margin = %dx%d, want 14x14", crop.Bounds().Dx(), crop.Bounds().Dy())
	}
	if got := crop.RGBAAt(2, 2); got.R != 90 {
		t.Errorf("content origin after margin = %v, want content color at (2,2)", got)
	}
}

func TestCropToContentEmpty(t *testing.T) {
	crop := CropToContent(image.NewRGBA(image.Rect(0, 0, 32, 32)), 0)
	if crop.Bounds().Dx() != 0 || crop.Bounds().Dy() != 0 {
		t.Fatalf("empty crop = %dx%d, want 0x0", crop.Bounds().Dx(), crop.Bounds().Dy())
	}
}

func TestPadToCanvas(t *testing.T) {
	crop := CropToContent(contentFrame(), 0)
	padded := PadToCanvas(crop, 32, 32, 5, 7)
	if padded.Bounds().Dx() != 32 || padded.Bounds().Dy() != 32 {
		t.Fatalf("padded = %dx%d, want 32x32", padded.Bounds().Dx(), padded.Bounds().Dy())
	}
	if got := padded.RGBAAt(5, 7); got.R != 90 {
		t.Errorf("content at offset = %v, want content color at (5,7)", got)
	}
	if padded.RGBAAt(0, 0).A != 0 {
		t.Error("transparent padding expected at (0,0)")
	}
}

func TestGridOffset(t *testing.T) {
	cases := []struct {
		v, step, want int
	}{
		{27, 8, 24}, {24, 8, 24}, {32, 8, 32}, {-3, 8, -8}, {10, 1, 10}, {0, 8, 0},
	}
	for _, c := range cases {
		if got := GridOffset(c.v, c.step); got != c.want {
			t.Errorf("GridOffset(%d, %d) = %d, want %d", c.v, c.step, got, c.want)
		}
	}
}

func TestGridCorrectSnapsToGrid(t *testing.T) {
	// Content 10×10 in a 32×32 canvas, step 8: centered offset would be 11,
	// snapped down to 8 → content top-left at (8,8).
	corrected := GridCorrect(contentFrame(), 32, 32, GridOptions{Step: 8})
	if got := corrected.RGBAAt(8, 8); got.R != 90 {
		t.Errorf("content top-left = %v at (8,8), want content color", got)
	}
	if corrected.RGBAAt(8, 7).A != 0 || corrected.RGBAAt(7, 8).A != 0 {
		t.Error("content must not leak outside the snapped grid offset")
	}
}

func TestGridCorrectDefaultStep(t *testing.T) {
	corrected := GridCorrect(contentFrame(), 32, 32, GridOptions{})
	if corrected.Bounds().Dx() != 32 || corrected.Bounds().Dy() != 32 {
		t.Fatalf("corrected = %dx%d, want 32x32", corrected.Bounds().Dx(), corrected.Bounds().Dy())
	}
	if _, _, foot := AlphaCentroid(corrected); foot == -1 {
		t.Error("content lost by grid correction")
	}
}

func TestFitToCanvasDownscale(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 64, 64))
	src.SetRGBA(0, 0, color.RGBA{R: 1, A: 255})
	src.SetRGBA(62, 62, color.RGBA{B: 2, A: 255})
	out, err := FitToCanvas(src, 32, 32)
	if err != nil {
		t.Fatalf("FitToCanvas: %v", err)
	}
	if out.Bounds().Dx() != 32 || out.Bounds().Dy() != 32 {
		t.Fatalf("out = %dx%d, want 32x32", out.Bounds().Dx(), out.Bounds().Dy())
	}
	if got := out.RGBAAt(0, 0); got.R != 1 {
		t.Errorf("downscale origin = %v, want src(0,0)", got)
	}
	// Integer-factor downscale: dst(31,31) samples src(62,62).
	if got := out.RGBAAt(31, 31); got.B != 2 {
		t.Errorf("downscale corner = %v, want src(62,62)", got)
	}
}

func TestFitToCanvasUpscale(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 16, 16))
	src.SetRGBA(1, 1, color.RGBA{R: 5, A: 255})
	out, err := FitToCanvas(src, 32, 32)
	if err != nil {
		t.Fatalf("FitToCanvas: %v", err)
	}
	// 2× upscale: dst(2,2) samples src(1,1).
	if got := out.RGBAAt(2, 2); got.R != 5 {
		t.Errorf("upscale sample = %v, want src(1,1) color", got)
	}
	if got := out.RGBAAt(3, 3); got.R != 5 {
		t.Errorf("upscale neighbor = %v, want duplicated pixel", got)
	}
}

func TestFitToCanvasExactCopy(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 32, 32))
	src.SetRGBA(5, 5, color.RGBA{R: 7, A: 255})
	out, err := FitToCanvas(src, 32, 32)
	if err != nil {
		t.Fatalf("FitToCanvas: %v", err)
	}
	if got := out.RGBAAt(5, 5); got.R != 7 {
		t.Errorf("exact copy = %v at (5,5), want R=7", got)
	}
}

func TestFitToCanvasCenterPlace(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 20, 10))
	src.SetRGBA(0, 0, color.RGBA{R: 9, A: 255})
	out, err := FitToCanvas(src, 32, 32)
	if err != nil {
		t.Fatalf("FitToCanvas: %v", err)
	}
	// 20×10 → 32×32, not a multiple → centered: dx=(32-20)/2=6, dy=(32-10)/2=11.
	if got := out.RGBAAt(6, 11); got.R != 9 {
		t.Errorf("center place = %v at (6,11), want R=9", got)
	}
}
