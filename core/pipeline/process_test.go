package pipeline

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
)

// buildSyntheticStrip builds a deterministic 4-frame filmstrip: each 32×32
// frame has a magenta technical background with a colored character block.
func buildSyntheticStrip(t *testing.T) (*image.RGBA, FrameList, PromptSnapshot) {
	t.Helper()
	canvas, err := identity.NewCanvasSpec(32, 32)
	if err != nil {
		t.Fatalf("canvas: %v", err)
	}
	layout, err := NormalizeFrameList(*canvas, 4)
	if err != nil {
		t.Fatalf("NormalizeFrameList: %v", err)
	}
	frames := []*image.RGBA{
		blockOnMagenta(11, 22, color.RGBA{R: 120, G: 200, B: 80, A: 255}),
		blockOnMagenta(12, 21, color.RGBA{R: 120, G: 200, B: 80, A: 255}),
		blockOnMagenta(10, 22, color.RGBA{R: 120, G: 200, B: 80, A: 255}),
		blockOnMagenta(12, 23, color.RGBA{R: 120, G: 200, B: 80, A: 255}),
	}
	strip, err := AssembleFilmstrip(frames, layout)
	if err != nil {
		t.Fatalf("AssembleFilmstrip: %v", err)
	}
	prompt, err := BuildPrompt(PromptInput{
		StylePreset:  StylePresetClassic,
		ActionPreset: ActionWalk,
		CanvasWidth:  32,
		CanvasHeight: 32,
		FrameCount:   4,
		Directions:   1,
	})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	return strip, layout, prompt
}

// blockOnMagenta is a 32×32 magenta frame with a 10×10 block at (bx,by).
func blockOnMagenta(bx, by int, c color.RGBA) *image.RGBA {
	img := magentaFrame(32, 32)
	for y := by; y < by+10; y++ {
		for x := bx; x < bx+10; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func magentaFrame(w, h int) *image.RGBA {
	img := solidFrame(w, h, color.RGBA{R: 255, G: 0, B: 255, A: 255})
	return img
}

func TestPerfectPixelCompatibilityDefaults(t *testing.T) {
	standard, err := identity.NewCanvasSpec(PerfectPixelCellSize, PerfectPixelCellSize)
	if err != nil {
		t.Fatalf("standard canvas: %v", err)
	}
	if !IsPerfectPixelCanvas(*standard) {
		t.Fatal("256x256 should be recognized as the PerfectPixel canvas")
	}
	got := effectiveAlignOptions(FrameList{Canvas: *standard}, AlignOptions{}, true)
	if got.BaselineY != PerfectPixelCellSize-PerfectPixelSafeMargin {
		t.Fatalf("PerfectPixel baseline = %d, want %d", got.BaselineY, PerfectPixelCellSize-PerfectPixelSafeMargin)
	}
	custom, err := identity.NewCanvasSpec(128, 128)
	if err != nil {
		t.Fatalf("custom canvas: %v", err)
	}
	if IsPerfectPixelCanvas(*custom) {
		t.Fatal("128x128 should not be recognized as the PerfectPixel canvas")
	}
	if got := effectiveAlignOptions(FrameList{Canvas: *custom}, AlignOptions{}, false); got.BaselineY != 0 {
		t.Fatalf("custom baseline = %d, want untouched zero default", got.BaselineY)
	}
}

func TestProcessFilmstripFullPipeline(t *testing.T) {
	strip, layout, prompt := buildSyntheticStrip(t)
	res, err := ProcessFilmstrip(strip, prompt, layout, ProcessOptions{})
	if err != nil {
		t.Fatalf("ProcessFilmstrip: %v", err)
	}
	c := res.Candidate
	if c.Status != CandidatePending {
		t.Errorf("status = %q, want pending", c.Status)
	}
	if len(c.Frames) != 4 {
		t.Fatalf("frames = %d, want 4", len(c.Frames))
	}
	for i, f := range c.Frames {
		if f.Bounds().Dx() != 32 || f.Bounds().Dy() != 32 {
			t.Errorf("frame %d = %dx%d, want 32x32", i, f.Bounds().Dx(), f.Bounds().Dy())
		}
	}
	// Magenta technical background must be gone (alpha 0) in the corners.
	for i, f := range c.Frames {
		if f.RGBAAt(0, 0).A != 0 {
			t.Errorf("frame %d corner not transparent: %v", i, f.RGBAAt(0, 0))
		}
	}
	// All frames share the baseline (foot row 31) — alpha-weighted centroid
	// alignment + shared baseline.
	for i, f := range c.Frames {
		if _, _, foot := AlphaCentroid(f); foot != 31 {
			t.Errorf("frame %d foot = %d, want 31", i, foot)
		}
	}
	// Slice completeness must be perfect.
	if c.Scores.SliceCompleteness != 1 {
		t.Errorf("SliceCompleteness = %v, want 1", c.Scores.SliceCompleteness)
	}
	// Original artifact preserved: the PNG round-trips to the input strip
	// (保留原始产物) and the prompt snapshot matches (保留 prompt 快照).
	decoded, err := DecodeFilmstrip(c.FilmstripPNG)
	if err != nil {
		t.Fatalf("decode preserved filmstrip: %v", err)
	}
	if decoded.Bounds().Dx() != strip.Bounds().Dx() || decoded.Bounds().Dy() != strip.Bounds().Dy() {
		t.Errorf("preserved filmstrip = %dx%d, want %dx%d", decoded.Bounds().Dx(), decoded.Bounds().Dy(), strip.Bounds().Dx(), strip.Bounds().Dy())
	}
	if c.Prompt.Prompt != prompt.Prompt {
		t.Error("prompt snapshot not preserved")
	}
	// Transforms must be the exact integer pixel displacements the alignment
	// stage applied (task 5.4 整数位移): recompute the deterministic
	// pre-alignment frames (key the whole strip → slice → grid, with the same
	// options the pipeline used) and verify (a) every recorded transform equals
	// the integer rounding of the fractional alpha-weighted centroid / foot
	// displacement, and (b) applying it to the pre-alignment frame reproduces
	// exactly the aligned frame. A non-integer or mis-rounded displacement
	// fails both checks.
	opts := ProcessOptions{}
	preSlices, err := SliceFilmstrip(KeyChroma(strip, opts.Key), layout, opts.Slice)
	if err != nil {
		t.Fatalf("re-slice for transform verification: %v", err)
	}
	pre := make([]*image.RGBA, 0, len(preSlices))
	for _, s := range preSlices {
		pre = append(pre, GridCorrect(s, layout.Canvas.UnitWidth, layout.Canvas.UnitHeight, opts.Grid))
	}
	aligned, _, err := AlignSequence(pre, opts.Align)
	if err != nil {
		t.Fatalf("re-align for transform verification: %v", err)
	}
	if len(res.Transforms) != len(pre) {
		t.Fatalf("transform count = %d, want %d", len(res.Transforms), len(pre))
	}
	for i, tr := range res.Transforms {
		// (a) Integer displacement: the transform is the integer rounding of
		// the true (fractional) centroid offset to the shared axis, and the
		// exact foot offset to the shared baseline.
		w, h := pre[i].Bounds().Dx(), pre[i].Bounds().Dy()
		cx, _, foot := AlphaCentroid(pre[i])
		wantDx := w/2 - int(math.Round(cx))
		wantDy := h - 1 - foot
		if tr.Dx != wantDx || tr.Dy != wantDy {
			t.Errorf("transform %d = (%d,%d), want integer displacement (%d,%d) for centroid %.3f / foot %d",
				i, tr.Dx, tr.Dy, wantDx, wantDy, cx, foot)
		}
		// (b) The displaced frame is exactly the aligned frame.
		shifted, _, _ := TranslateFrame(pre[i], tr.Dx, tr.Dy)
		if !framesPixelEqual(shifted, aligned[i]) {
			t.Errorf("frame %d displaced by (%d,%d) does not reproduce the aligned frame", i, tr.Dx, tr.Dy)
		}
	}
}

func TestProcessFilmstripPaletteOptions(t *testing.T) {
	canvas, err := identity.NewCanvasSpec(32, 32)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := NormalizeFrameList(*canvas, 1)
	if err != nil {
		t.Fatal(err)
	}
	frame := magentaFrame(32, 32)
	for i := 0; i < 20; i++ {
		x0 := 6 + (i%5)*4
		y0 := 6 + (i/5)*4
		c := color.RGBA{R: uint8(i * 10), G: uint8(255 - i*10), B: uint8(i * 7), A: 255}
		for y := y0; y < y0+4; y++ {
			for x := x0; x < x0+4; x++ {
				frame.SetRGBA(x, y, c)
			}
		}
	}
	strip, err := AssembleFilmstrip([]*image.RGBA{frame}, layout)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildPrompt(PromptInput{StylePreset: StylePresetRetro16, ActionPreset: ActionIdle, CanvasWidth: 32, CanvasHeight: 32, FrameCount: 1, Directions: 1})
	if err != nil {
		t.Fatal(err)
	}
	quantized, err := ProcessFilmstrip(strip, prompt, layout, ProcessOptions{Palette: PaletteOptions{MaxColors: 16}})
	if err != nil {
		t.Fatal(err)
	}
	if got := opaqueColorCount(quantized.Candidate.Frames[0]); got > 16 {
		t.Fatalf("quantized opaque colors = %d, want <= 16", got)
	}
	skipped, err := ProcessFilmstrip(strip, prompt, layout, ProcessOptions{Palette: PaletteOptions{Skip: true}})
	if err != nil {
		t.Fatal(err)
	}
	if got := opaqueColorCount(skipped.Candidate.Frames[0]); got <= 16 {
		t.Fatalf("skipped opaque colors = %d, want > 16", got)
	}
}

func opaqueColorCount(frame *image.RGBA) int {
	colors := map[color.RGBA]struct{}{}
	b := frame.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := frame.RGBAAt(x, y)
			if c.A != 0 {
				colors[c] = struct{}{}
			}
		}
	}
	return len(colors)
}

// framesPixelEqual reports whether two frames are pixel-identical.
func framesPixelEqual(a, b *image.RGBA) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return false
			}
		}
	}
	return true
}

func TestProcessFilmstripSlicingFailureKeepsArtifact(t *testing.T) {
	// Strip holding only 3 frames in a 4-frame layout → pipeline failure with
	// a recorded reason, NO partial frames, but the original artifact and
	// prompt snapshot are retained (非空手返回).
	canvas, _ := identity.NewCanvasSpec(32, 32)
	layout, _ := NormalizeFrameList(*canvas, 4)
	strip := stripWithFrames(3, 32, 32, 0, color.RGBA{R: 255, G: 0, B: 255, A: 255})
	prompt, _ := BuildPrompt(PromptInput{
		StylePreset:  StylePresetClassic,
		ActionPreset: ActionWalk,
		CanvasWidth:  32,
		CanvasHeight: 32,
		FrameCount:   4,
		Directions:   1,
	})
	res, err := ProcessFilmstrip(strip, prompt, layout, ProcessOptions{Slice: SliceOptions{MinFrameWidthFraction: 0.9}})
	if err == nil {
		t.Fatal("expected slicing failure")
	}
	if res.Candidate.Status != CandidateFailed {
		t.Errorf("status = %q, want failed", res.Candidate.Status)
	}
	if res.Candidate.Reason == "" {
		t.Error("failure reason not recorded")
	}
	if len(res.Candidate.Frames) != 0 {
		t.Errorf("partial frames produced on failure: %d (must be 0)", len(res.Candidate.Frames))
	}
	if len(res.Candidate.FilmstripPNG) == 0 {
		t.Error("original filmstrip artifact not retained on failure")
	}
	if res.Candidate.Prompt.Prompt != prompt.Prompt {
		t.Error("prompt snapshot not retained on failure")
	}
}

func TestProcessFilmstripNilStrip(t *testing.T) {
	canvas, _ := identity.NewCanvasSpec(32, 32)
	layout, _ := NormalizeFrameList(*canvas, 4)
	if _, err := ProcessFilmstrip(nil, PromptSnapshot{}, layout, ProcessOptions{}); err == nil {
		t.Error("nil strip: expected error")
	}
}

func TestRegenerate(t *testing.T) {
	strip, layout, prompt := buildSyntheticStrip(t)
	first, err := ProcessFilmstrip(strip, prompt, layout, ProcessOptions{})
	if err != nil {
		t.Fatalf("first ProcessFilmstrip: %v", err)
	}
	// Regeneration with a new filmstrip (same synthetic source is fine — the
	// point is a NEW candidate linked to the previous one, task 5.6).
	second, err := Regenerate(first.Candidate, strip, prompt, layout, ProcessOptions{})
	if err != nil {
		t.Fatalf("Regenerate: %v", err)
	}
	if second.Candidate.RegenerationOf != first.Candidate.ID {
		t.Errorf("RegenerationOf = %q, want %q", second.Candidate.RegenerationOf, first.Candidate.ID)
	}
	if second.Candidate.ID == first.Candidate.ID {
		t.Error("regeneration produced the same candidate id")
	}
	// A CandidateSet over both keeps the best (生成结果保留最佳候选而非空手返回).
	cs := NewCandidateSet()
	cs.Add(first.Candidate)
	cs.Add(second.Candidate)
	if cs.Best() == nil {
		t.Fatal("candidate set must never return empty")
	}
}
