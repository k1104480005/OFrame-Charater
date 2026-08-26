package pipeline

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
)

// perfectFrames returns 4 identical 32×32 frames with a 10×10 block at the
// canonical bottom-center position (foot at row 31, centroid x ≈ 16).
func perfectFrames(n int) []*image.RGBA {
	frames := make([]*image.RGBA, n)
	for i := 0; i < n; i++ {
		frames[i] = blockFrame(11, 22)
	}
	return frames
}

func qualityLayout(t *testing.T, frameCount int) FrameList {
	t.Helper()
	canvas, err := identity.NewCanvasSpec(32, 32)
	if err != nil {
		t.Fatalf("canvas: %v", err)
	}
	layout, err := NormalizeFrameList(*canvas, frameCount)
	if err != nil {
		t.Fatalf("NormalizeFrameList: %v", err)
	}
	return layout
}

func TestScoreCandidatePerfect(t *testing.T) {
	layout := qualityLayout(t, 4)
	frames := perfectFrames(4)
	palette, _ := BuildSharedPalette(frames, 32)
	flip := horizontalFlip(frames[0])
	scores := ScoreCandidate(QualityInput{
		Frames:        frames,
		Layout:        layout,
		Transforms:    make([]FrameTransform, 4),
		AnchorSets:    [][]AnchorPoint{{{Name: "feet", X: 16, Y: 31}}, {{Name: "feet", X: 16, Y: 31}}, {{Name: "feet", X: 16, Y: 31}}, {{Name: "feet", X: 16, Y: 31}}},
		TargetAnchors: []AnchorPoint{{Name: "feet", X: 16, Y: 31}},
		Palette:       palette,
		MirrorPair:    [2]*image.RGBA{frames[0], flip},
	})
	if scores.SliceCompleteness != 1 {
		t.Errorf("SliceCompleteness = %v, want 1", scores.SliceCompleteness)
	}
	if scores.AnchorDeviationPx != 0 {
		t.Errorf("AnchorDeviationPx = %v, want 0", scores.AnchorDeviationPx)
	}
	if scores.AreaJitter != 0 {
		t.Errorf("AreaJitter = %v, want 0", scores.AreaJitter)
	}
	if scores.CentroidJitter != 0 {
		t.Errorf("CentroidJitter = %v, want 0", scores.CentroidJitter)
	}
	if scores.PaletteConsistency != 1 {
		t.Errorf("PaletteConsistency = %v, want 1", scores.PaletteConsistency)
	}
	if scores.MirrorSymmetry != 1 {
		t.Errorf("MirrorSymmetry = %v, want 1", scores.MirrorSymmetry)
	}
	if scores.OutOfBoundsRatio != 0 {
		t.Errorf("OutOfBoundsRatio = %v, want 0", scores.OutOfBoundsRatio)
	}
	if math.Abs(scores.Overall-1) > 1e-9 {
		t.Errorf("Overall = %v, want 1", scores.Overall)
	}
}

func TestScoreCandidateBroken(t *testing.T) {
	// 3 frames instead of 4 → slice completeness 0.
	layout := qualityLayout(t, 4)
	frames := []*image.RGBA{
		blockIn(2, 2, 10, 10), // small, top-left
		blockIn(5, 20, 20, 5), // different size
		blockIn(10, 22, 6, 6), // different size/position
	}
	scores := ScoreCandidate(QualityInput{
		Frames:        frames,
		Layout:        layout,
		Transforms:    make([]FrameTransform, 3),
		AnchorSets:    [][]AnchorPoint{{{Name: "feet", X: 0, Y: 0}}, {{Name: "feet", X: 0, Y: 0}}, {{Name: "feet", X: 0, Y: 0}}},
		TargetAnchors: []AnchorPoint{{Name: "feet", X: 16, Y: 31}},
		Palette:       []color.RGBA{{R: 1, G: 1, B: 1, A: 255}},
	})
	if scores.SliceCompleteness != 0 {
		t.Errorf("SliceCompleteness = %v, want 0 (wrong frame count)", scores.SliceCompleteness)
	}
	if scores.AnchorDeviationPx == 0 {
		t.Error("AnchorDeviationPx = 0, want > 0")
	}
	if scores.AreaJitter == 0 {
		t.Error("AreaJitter = 0, want > 0 (different areas)")
	}
	if scores.Overall >= 1 {
		t.Errorf("Overall = %v, want < 1", scores.Overall)
	}
}

func TestScoreCandidateAnchorDeviationPx(t *testing.T) {
	layout := qualityLayout(t, 1)
	frames := []*image.RGBA{blockFrame(11, 22)}
	scores := ScoreCandidate(QualityInput{
		Frames:        frames,
		Layout:        layout,
		Transforms:    []FrameTransform{{}},
		AnchorSets:    [][]AnchorPoint{{{Name: "feet", X: 18, Y: 33}}},
		TargetAnchors: []AnchorPoint{{Name: "feet", X: 16, Y: 31}},
	})
	if math.Abs(scores.AnchorDeviationPx-math.Hypot(2, 2)) > 1e-9 {
		t.Errorf("AnchorDeviationPx = %v, want %v", scores.AnchorDeviationPx, math.Hypot(2, 2))
	}
}

func TestScoreCandidateOutOfBounds(t *testing.T) {
	layout := qualityLayout(t, 1)
	frames := []*image.RGBA{blockFrame(11, 22)}
	scores := ScoreCandidate(QualityInput{
		Frames:     frames,
		Layout:     layout,
		Transforms: []FrameTransform{{ClippedOpaque: 25, TotalOpaque: 100}},
	})
	if math.Abs(scores.OutOfBoundsRatio-0.25) > 1e-9 {
		t.Errorf("OutOfBoundsRatio = %v, want 0.25", scores.OutOfBoundsRatio)
	}
}

func TestScoreCandidatePaletteConsistency(t *testing.T) {
	layout := qualityLayout(t, 1)
	frames := []*image.RGBA{blockFrame(11, 22)} // block color {120,200,80}
	palette := []color.RGBA{{R: 120, G: 200, B: 80, A: 255}}
	scores := ScoreCandidate(QualityInput{
		Frames:     frames,
		Layout:     layout,
		Transforms: []FrameTransform{{}},
		Palette:    palette,
	})
	if scores.PaletteConsistency != 1 {
		t.Errorf("PaletteConsistency = %v, want 1 (all pixels in palette)", scores.PaletteConsistency)
	}
	// A palette without the block color → 0 consistency.
	scores = ScoreCandidate(QualityInput{
		Frames:     frames,
		Layout:     layout,
		Transforms: []FrameTransform{{}},
		Palette:    []color.RGBA{{R: 1, G: 1, B: 1, A: 255}},
	})
	if scores.PaletteConsistency != 0 {
		t.Errorf("PaletteConsistency = %v, want 0 (no pixel in palette)", scores.PaletteConsistency)
	}
}

func TestScoreCandidateColorCount(t *testing.T) {
	layout := qualityLayout(t, 1)
	frames := []*image.RGBA{manyColorsFrame(16)}
	scores := ScoreCandidate(QualityInput{
		Frames:     frames,
		Layout:     layout,
		Transforms: []FrameTransform{{}},
	})
	if scores.ColorCount != 16 {
		t.Errorf("ColorCount = %d, want 16", scores.ColorCount)
	}
}

func TestScoreCandidateMirrorSymmetry(t *testing.T) {
	layout := qualityLayout(t, 1)
	frames := []*image.RGBA{blockFrame(11, 22)}
	// Right frame vs its own horizontal flip → perfect symmetry.
	scores := ScoreCandidate(QualityInput{
		Frames:     frames,
		Layout:     layout,
		MirrorPair: [2]*image.RGBA{frames[0], horizontalFlip(frames[0])},
	})
	if scores.MirrorSymmetry != 1 {
		t.Errorf("MirrorSymmetry = %v, want 1", scores.MirrorSymmetry)
	}
	// Unrelated frames → low symmetry.
	other := blockFrame(2, 2)
	scores = ScoreCandidate(QualityInput{
		Frames:     frames,
		Layout:     layout,
		MirrorPair: [2]*image.RGBA{frames[0], other},
	})
	if scores.MirrorSymmetry >= 1 {
		t.Errorf("MirrorSymmetry = %v, want < 1 for unrelated pair", scores.MirrorSymmetry)
	}
	// No pair → not penalized.
	scores = ScoreCandidate(QualityInput{Frames: frames, Layout: layout})
	if scores.MirrorSymmetry != 1 {
		t.Errorf("MirrorSymmetry without pair = %v, want 1", scores.MirrorSymmetry)
	}
}

func TestScoreCandidateAreaJitter(t *testing.T) {
	// Two frames: 100 and 200 opaque pixels → CV = 50/150 ≈ 0.3333.
	a := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			a.SetRGBA(x, y, color.RGBA{R: 9, A: 255})
		}
	}
	b := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 10; y++ {
		for x := 0; x < 20; x++ {
			b.SetRGBA(x, y, color.RGBA{R: 9, A: 255})
		}
	}
	layout := qualityLayout(t, 2)
	scores := ScoreCandidate(QualityInput{
		Frames:     []*image.RGBA{a, b},
		Layout:     layout,
		Transforms: []FrameTransform{{}, {}},
	})
	if math.Abs(scores.AreaJitter-1.0/3.0) > 1e-6 {
		t.Errorf("AreaJitter = %v, want %v", scores.AreaJitter, 1.0/3.0)
	}
}

// blockIn builds a 32×32 frame with a bw×bh opaque block at (bx,by).
func blockIn(bx, by, bw, bh int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := by; y < by+bh; y++ {
		for x := bx; x < bx+bw; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 120, G: 200, B: 80, A: 255})
		}
	}
	return img
}

// horizontalFlip returns the left-right mirrored copy of img.
func horizontalFlip(img *image.RGBA) *image.RGBA {
	b := img.Bounds()
	w := b.Dx()
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.SetRGBA(x, y, img.RGBAAt(w-1-x, y))
		}
	}
	return out
}
