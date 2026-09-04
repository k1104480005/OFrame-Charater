package pipeline

import (
	"image"
	"image/color"
	"strings"
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

// TestSliceFilmstripLetterboxBand verifies the layout guard's recovery path:
// a model that returns the requested strip drawn inside a LARGER square canvas
// (strip band in the middle rows, "letterbox") still slices correctly — the
// band is cropped out first and the frames land exactly on the spec cells.
func TestSliceFilmstripLetterboxBand(t *testing.T) {
	canvas, _ := identity.NewCanvasSpec(32, 32)
	layout, _ := NormalizeFrameList(*canvas, 4)
	// Correct 4-frame strip (140x32) with 4px gutters …
	inner := stripWithFrames(4, 32, 32, 4, color.RGBA{R: 255, G: 64, A: 255})
	// … placed in the middle of a larger square canvas (letterbox margins).
	square := image.NewRGBA(image.Rect(0, 0, inner.Bounds().Dx(), inner.Bounds().Dy()+108))
	b := inner.Bounds()
	top := (square.Bounds().Dy() - b.Dy()) / 2
	for y := 0; y < b.Dy(); y++ {
		dst := square.Pix[(y+top)*square.Stride : (y+top+1)*square.Stride]
		copy(dst, inner.Pix[y*inner.Stride:(y+1)*inner.Stride])
	}
	frames, err := SliceFilmstrip(square, layout, SliceOptions{})
	if err != nil {
		t.Fatalf("SliceFilmstrip on letterboxed strip: %v", err)
	}
	if len(frames) != 4 {
		t.Fatalf("frames = %d, want 4", len(frames))
	}
	for i, f := range frames {
		if f.Bounds().Dx() != 32 || f.Bounds().Dy() != 32 {
			t.Errorf("frame %d size = %dx%d, want 32x32", i, f.Bounds().Dx(), f.Bounds().Dy())
		}
	}
}

// TestSliceFilmstripFullBleedRejected verifies the layout guard's rejection
// path: a keyed strip with NO transparent gap at all (the model ignored the
// magenta-background / horizontal-strip contract and painted a full-bleed
// scene) must fail with a readable error instead of being force-sliced into
// squashed garbage frames.
func TestSliceFilmstripFullBleedRejected(t *testing.T) {
	canvas, _ := identity.NewCanvasSpec(32, 32)
	layout, _ := NormalizeFrameList(*canvas, 4)
	bleed := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			bleed.SetRGBA(x, y, color.RGBA{R: 60, G: 120, B: 200, A: 255})
		}
	}
	frames, err := SliceFilmstrip(bleed, layout, SliceOptions{})
	if err == nil {
		t.Fatal("expected gapless canvas to be rejected")
	}
	if len(frames) != 0 {
		t.Fatalf("partial frames produced on rejection: %d (must be 0)", len(frames))
	}
	if !strings.Contains(err.Error(), "no transparent gap between poses") {
		t.Fatalf("error = %v, want readable layout-contract failure", err)
	}
}

// TestProcessFilmstripMagentaStripEndToEnd verifies the perfectpixel-aligned
// contract end to end: the model returns a full-bleed canvas with a MAGENTA
// technical background and separated poses; keying the whole strip first makes
// the poses separable and every frame lands on its spec cell.
func TestProcessFilmstripMagentaStripEndToEnd(t *testing.T) {
	canvas, _ := identity.NewCanvasSpec(32, 32)
	layout, _ := NormalizeFrameList(*canvas, 4)
	// 满幅洋红技术底 + 4 个分离的绿色姿势块，画布（180x128）大于规格条带
	// （128x32）—— 键控后按内容比例缩放装格。
	square := image.NewRGBA(image.Rect(0, 0, 180, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 180; x++ {
			square.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 255, A: 255})
		}
	}
	green := color.RGBA{R: 120, G: 200, B: 80, A: 255}
	for i, bx := range []int{20, 60, 100, 140} {
		by := 48 + i%2
		for y := by; y < by+16; y++ {
			for x := bx; x < bx+16; x++ {
				square.SetRGBA(x, y, green)
			}
		}
	}
	res, err := ProcessFilmstrip(square, PromptSnapshot{}, layout, ProcessOptions{})
	if err != nil {
		t.Fatalf("ProcessFilmstrip on magenta strip: %v", err)
	}
	if len(res.Candidate.Frames) != 4 {
		t.Fatalf("frames = %d, want 4", len(res.Candidate.Frames))
	}
	for i, f := range res.Candidate.Frames {
		if f.Bounds().Dx() != 32 || f.Bounds().Dy() != 32 {
			t.Errorf("frame %d size = %dx%d, want 32x32", i, f.Bounds().Dx(), f.Bounds().Dy())
		}
		// The magenta technical background must be gone in the corners.
		if f.RGBAAt(0, 0).A != 0 {
			t.Errorf("frame %d corner not transparent: %v", i, f.RGBAAt(0, 0))
		}
	}
}

// TestContentBoundsTrueMinimum is the letterbox-crop amputation regression:
// the topmost content row may start far RIGHT of lower rows (a pose's head vs
// another pose's full body). minX must be the true minimum across all rows —
// locking it to the first scan-order hit crops whole poses off the left side
// before slicing (the half-body root cause on agnes/Doubao wide strips).
func TestContentBoundsTrueMinimum(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	// Top row: a narrow patch at x=[150..170).
	for x := 150; x < 170; x++ {
		img.SetRGBA(x, 10, color.RGBA{R: 255, A: 255})
	}
	// Lower rows: a body extending left to x=20.
	for y := 40; y < 90; y++ {
		for x := 20; x < 80; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	x0, y0, x1, y1, ok := ContentBounds(img)
	if !ok {
		t.Fatal("ContentBounds: no content found")
	}
	if x0 != 20 || y0 != 10 || x1 != 170 || y1 != 90 {
		t.Fatalf("ContentBounds = (%d,%d,%d,%d), want (20,10,170,90)", x0, y0, x1, y1)
	}
}

// TestSliceFilmstripWideCanvasKeepsFullPoses is the half-body regression test:
// stripImageSize asks the model for a WIDE canvas (e.g. 2048×877 for 4 frames,
// so every cell is actually 512px wide while the logical unit width stays
// 256). The DP must target the REAL per-cell width (strip width / frame count)
// — targeting the unit width drops every cut on a 256px multiple and slices
// each pose into vertical slivers (observed on agnes walk + Doubao idle).
func TestSliceFilmstripWideCanvasKeepsFullPoses(t *testing.T) {
	canvas, _ := identity.NewCanvasSpec(256, 256)
	layout, _ := NormalizeFrameList(*canvas, 4)
	// 2048×877 keyed strip: one 320×752 pose centered in each 512px cell.
	strip := image.NewRGBA(image.Rect(0, 0, 2048, 877))
	for i := 0; i < 4; i++ {
		x0 := i*512 + 96
		for y := 62; y < 62+752; y++ {
			for x := x0; x < x0+320; x++ {
				strip.SetRGBA(x, y, color.RGBA{R: 120, G: 200, B: 80, A: 255})
			}
		}
	}
	frames, err := SliceFilmstrip(strip, layout, SliceOptions{})
	if err != nil {
		t.Fatalf("SliceFilmstrip on wide canvas: %v", err)
	}
	if len(frames) != 4 {
		t.Fatalf("frames = %d, want 4", len(frames))
	}
	for i, f := range frames {
		if f.Bounds().Dx() != 256 || f.Bounds().Dy() != 256 {
			t.Fatalf("frame %d size = %dx%d, want 256x256", i, f.Bounds().Dx(), f.Bounds().Dy())
		}
		x0, y0, x1, y1, ok := ContentBounds(f)
		if !ok {
			t.Fatalf("frame %d has no content", i)
		}
		w, h := x1-x0, y1-y0
		// Full pose: 320×752 proportionally fitted → ~109×256. A half-body
		// sliver (the regression) lands around ~54px wide.
		if w < 90 || w > 130 {
			t.Errorf("frame %d content width = %d, want ~109 (full pose, not a half-body sliver)", i, w)
		}
		if h < 250 {
			t.Errorf("frame %d content height = %d, want ~256", i, h)
		}
	}
}

// TestSliceFilmstripMissingPoseRejected verifies the pose-band guard: a keyed
// strip holding fewer separable pose bands than the spec frame count (the
// model skipped or merged poses) must fail with a readable error instead of
// force-fitting the existing poses into all cells.
func TestSliceFilmstripMissingPoseRejected(t *testing.T) {
	canvas, _ := identity.NewCanvasSpec(32, 32)
	layout, _ := NormalizeFrameList(*canvas, 4)
	// Only 3 separated frames on the strip.
	strip := stripWithFrames(3, 32, 32, 8, color.RGBA{R: 255, G: 64, A: 255})
	frames, err := SliceFilmstrip(strip, layout, SliceOptions{})
	if err == nil {
		t.Fatal("expected missing-pose failure for 3 bands in a 4-frame layout")
	}
	if len(frames) != 0 {
		t.Fatalf("partial frames produced on failure: %d (must be 0)", len(frames))
	}
	if !strings.Contains(err.Error(), "separable pose bands") {
		t.Fatalf("error = %v, want readable pose-band failure", err)
	}
}
