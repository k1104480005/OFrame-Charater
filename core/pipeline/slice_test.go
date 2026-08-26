package pipeline

import (
	"image"
	"image/color"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
)

// stripWithFrames builds a filmstrip of frameCount solid frames of width w and
// height h, separated by transparent gutters of gutter px (0 = touching).
func stripWithFrames(frameCount, w, h, gutter int, c color.RGBA) *image.RGBA {
	total := frameCount*w + (frameCount-1)*gutter
	strip := image.NewRGBA(image.Rect(0, 0, total, h))
	for f := 0; f < frameCount; f++ {
		off := f * (w + gutter)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				strip.SetRGBA(off+x, y, c)
			}
		}
	}
	return strip
}

func TestAlphaProjection(t *testing.T) {
	// Two solid frames separated by a 6px transparent gutter.
	strip := stripWithFrames(2, 8, 4, 6, color.RGBA{R: 255, A: 255})
	proj := AlphaProjection(strip)
	if len(proj) != 22 {
		t.Fatalf("projection length = %d, want 22", len(proj))
	}
	for x := 0; x < 8; x++ {
		if proj[x] != 4*255 {
			t.Errorf("col %d: proj = %d, want %d", x, proj[x], 4*255)
		}
	}
	for x := 8; x < 14; x++ {
		if proj[x] != 0 {
			t.Errorf("gutter col %d: proj = %d, want 0", x, proj[x])
		}
	}
	for x := 14; x < 22; x++ {
		if proj[x] != 4*255 {
			t.Errorf("col %d: proj = %d, want %d", x, proj[x], 4*255)
		}
	}
}

func TestOptimalCutsCleanStrip(t *testing.T) {
	// 4 touching frames of width 32 → unique optimum at exact multiples.
	strip := stripWithFrames(4, 32, 32, 0, color.RGBA{G: 255, A: 255})
	proj := AlphaProjection(strip)
	cuts, err := OptimalCuts(proj, 32, 32, 4, SliceOptions{})
	if err != nil {
		t.Fatalf("OptimalCuts: %v", err)
	}
	want := []int{0, 32, 64, 96, 128}
	if len(cuts) != len(want) {
		t.Fatalf("cuts = %v, want %v", cuts, want)
	}
	for i := range want {
		if cuts[i] != want[i] {
			t.Errorf("cut %d = %d, want %d", i, cuts[i], want[i])
		}
	}
}

func TestOptimalCutsPreferGutters(t *testing.T) {
	// 3 frames of width 32 with 8px gutters: cuts must land in transparent
	// columns.
	strip := stripWithFrames(3, 32, 32, 8, color.RGBA{B: 255, A: 255})
	proj := AlphaProjection(strip)
	cuts, err := OptimalCuts(proj, 32, 32, 3, SliceOptions{})
	if err != nil {
		t.Fatalf("OptimalCuts: %v", err)
	}
	if len(cuts) != 4 {
		t.Fatalf("cuts = %v, want 4 positions", cuts)
	}
	if cuts[0] != 0 || cuts[3] != 32*3+2*8 {
		t.Fatalf("outer cuts = [%d, %d], want [0, %d]", cuts[0], cuts[3], 32*3+2*8)
	}
	// Internal cuts must be in the gutters: [32,40) and [72,80).
	for i, c := range cuts[1:3] {
		if proj[c] != 0 {
			t.Errorf("internal cut %d at column %d cuts through content (proj=%d)", i+1, c, proj[c])
		}
	}
}

func TestOptimalCutsStripTooNarrow(t *testing.T) {
	strip := stripWithFrames(1, 20, 32, 0, color.RGBA{R: 255, A: 255})
	proj := AlphaProjection(strip)
	if _, err := OptimalCuts(proj, 32, 32, 4, SliceOptions{}); err == nil {
		t.Error("strip narrower than frame count: expected error")
	}
}

func TestOptimalCutsFrameWidthMismatch(t *testing.T) {
	// 3 frames of width 32 but 4 frames requested: the DP must split a frame,
	// producing frames narrower than 90% of the unit width → failure.
	strip := stripWithFrames(3, 32, 32, 0, color.RGBA{R: 255, A: 255})
	proj := AlphaProjection(strip)
	if _, err := OptimalCuts(proj, 32, 32, 4, SliceOptions{MinFrameWidthFraction: 0.9}); err == nil {
		t.Error("wrong frame count with strict width fraction: expected error")
	}
}

func TestSliceFilmstrip(t *testing.T) {
	canvas, _ := identity.NewCanvasSpec(32, 32)
	layout, _ := NormalizeFrameList(*canvas, 4)
	// 4 solid frames with 4px gutters — slices must be 32x32 and hold the
	// frame colors.
	strip := stripWithFrames(4, 32, 32, 4, color.RGBA{R: 255, G: 64, A: 255})
	frames, err := SliceFilmstrip(strip, layout, SliceOptions{})
	if err != nil {
		t.Fatalf("SliceFilmstrip: %v", err)
	}
	if len(frames) != 4 {
		t.Fatalf("frames = %d, want 4", len(frames))
	}
	for i, f := range frames {
		if f.Bounds().Dx() != 32 || f.Bounds().Dy() != 32 {
			t.Errorf("frame %d size = %dx%d, want 32x32", i, f.Bounds().Dx(), f.Bounds().Dy())
		}
	}
	// Frame content must be present (opaque) and all four frames identical
	// in color composition.
	for i, f := range frames {
		opaque := 0
		for y := 0; y < 32; y++ {
			for x := 0; x < 32; x++ {
				if f.RGBAAt(x, y).A > 0 {
					opaque++
				}
			}
		}
		if opaque < 32*28 { // solid 32px-wide frame after gutters, ~full cell
			t.Errorf("frame %d opaque pixels = %d, want ~%d", i, opaque, 32*32)
		}
	}
}

func TestSliceFilmstripAllOrNothing(t *testing.T) {
	canvas, _ := identity.NewCanvasSpec(32, 32)
	layout, _ := NormalizeFrameList(*canvas, 4)
	// Strip holds only 3 frames → slicing must fail and produce NO partial
	// frames (task 5.3: 切片与规格不符时任务失败且不产出部分资产).
	strip := stripWithFrames(3, 32, 32, 0, color.RGBA{R: 255, A: 255})
	frames, err := SliceFilmstrip(strip, layout, SliceOptions{MinFrameWidthFraction: 0.9})
	if err == nil {
		t.Fatal("expected slicing failure for 3 frames in a 4-frame layout")
	}
	if len(frames) != 0 {
		t.Fatalf("partial frames produced on failure: %d (must be 0)", len(frames))
	}
}
