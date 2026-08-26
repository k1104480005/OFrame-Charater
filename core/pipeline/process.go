package pipeline

import (
	"fmt"
	"image"
	"time"
)

// ProcessOptions collects the tunable stage options for the deterministic
// filmstrip pipeline (tasks 5.1–5.4).
type ProcessOptions struct {
	Slice      SliceOptions
	Key        KeyOptions
	Align      AlignOptions
	Grid       GridOptions
	Palette    PaletteOptions
	Anchors    []AnchorPoint  // identity-level anchors (corrected per frame)
	Targets    []AnchorPoint  // normalized target anchors (default feet bottom-center)
	MirrorPair [2]*image.RGBA // optional cross-direction pair for symmetry scoring
}

// ProcessResult is the outcome of one pipeline run.
type ProcessResult struct {
	Candidate  Candidate
	Transforms []FrameTransform
}

// ProcessFilmstrip runs the full deterministic image-processing pipeline on a
// raw filmstrip (tasks 5.1–5.4): normalize → DP-optimal integer slicing →
// YCbCr chroma key (despill + flood fill) → pixel-grid correction → shared
// baseline / alpha-weighted centroid alignment → shared palette quantization →
// quality scoring. The original filmstrip and the immutable prompt snapshot
// are preserved in the returned candidate.
//
// All-or-nothing: when slicing fails the task fails with the recorded reason
// and no partial assets are produced, but the failed candidate still retains
// the original artifact and prompt snapshot (生成结果保留最佳候选而非空手返回).
func ProcessFilmstrip(strip *image.RGBA, prompt PromptSnapshot, layout FrameList, opts ProcessOptions) (ProcessResult, error) {
	if strip == nil {
		return ProcessResult{}, fmt.Errorf("pipeline: filmstrip is nil")
	}
	fail := func(err error) (ProcessResult, error) {
		pngData, perr := EncodeFilmstripPNG(strip)
		if perr != nil {
			return ProcessResult{}, fmt.Errorf("%w (and filmstrip preservation failed: %v)", err, perr)
		}
		c := Candidate{
			ID:           newCandidateID(),
			CreatedAt:    time.Now().UTC(),
			Prompt:       prompt,
			Layout:       layout,
			FilmstripPNG: pngData,
			Status:       CandidateFailed,
			Reason:       err.Error(),
		}
		return ProcessResult{Candidate: c}, err
	}

	// 1. Deterministic slicing (task 5.3).
	slices, err := SliceFilmstrip(strip, layout, opts.Slice)
	if err != nil {
		return fail(err)
	}

	// 2. YCbCr chroma key with despill + flood fill (task 5.4).
	keyed := make([]*image.RGBA, 0, len(slices))
	for _, s := range slices {
		keyed = append(keyed, KeyChroma(s, opts.Key))
	}

	// 3. Pixel-grid correction: crop to content, snap to grid, pad to canvas
	// (task 5.4 裁边与留白按逻辑画布对齐).
	gridded := make([]*image.RGBA, 0, len(keyed))
	for _, k := range keyed {
		gridded = append(gridded, GridCorrect(k, layout.Canvas.UnitWidth, layout.Canvas.UnitHeight, opts.Grid))
	}

	// 4. Alpha-weighted centroid alignment to the shared baseline (task 5.4).
	aligned, transforms, err := AlignSequence(gridded, opts.Align)
	if err != nil {
		return fail(err)
	}

	// 5. Anchor correction (task 5.4: 锚点校正).
	targets := opts.Targets
	if len(targets) == 0 {
		targets = DefaultTargetAnchors(layout)
	}
	anchorSets := make([][]AnchorPoint, 0, len(transforms))
	for _, t := range transforms {
		anchorSets = append(anchorSets, CorrectAnchors(opts.Anchors, t))
	}

	// 6. Shared palette quantization (task 5.4).
	maxColors := opts.Palette.MaxColors
	if maxColors <= 0 {
		maxColors = DefaultMaxPaletteColors
	}
	palette, err := BuildSharedPalette(aligned, maxColors)
	if err != nil {
		return fail(fmt.Errorf("pipeline: build shared palette: %w", err))
	}
	final, err := QuantizeToPalette(aligned, palette)
	if err != nil {
		return fail(fmt.Errorf("pipeline: quantize to shared palette: %w", err))
	}

	// 7. Quality scoring (task 8.1).
	scores := ScoreCandidate(QualityInput{
		Frames:        aligned,
		Layout:        layout,
		Transforms:    transforms,
		AnchorSets:    anchorSets,
		TargetAnchors: targets,
		Palette:       palette,
		MirrorPair:    opts.MirrorPair,
	})

	// 8. Preserve the original artifact + prompt snapshot.
	pngData, err := EncodeFilmstripPNG(strip)
	if err != nil {
		return fail(err)
	}
	c := Candidate{
		ID:           newCandidateID(),
		CreatedAt:    time.Now().UTC(),
		Prompt:       prompt,
		Layout:       layout,
		FilmstripPNG: pngData,
		Frames:       final,
		AnchorSets:   anchorSets,
		Scores:       scores,
		Status:       CandidatePending,
	}
	return ProcessResult{Candidate: c, Transforms: transforms}, nil
}

// Regenerate builds a new candidate from a new filmstrip for a previously
// produced candidate (task 5.6: 验收未通过后的重新生成). It performs no
// external calls — the generation confirmation gate (generation spec 4.5) is
// enforced by the caller before invoking. The new candidate is linked to the
// previous one; the caller's CandidateSet retains the best-scoring result so
// the user is never left empty-handed.
func Regenerate(prev Candidate, strip *image.RGBA, prompt PromptSnapshot, layout FrameList, opts ProcessOptions) (ProcessResult, error) {
	res, err := ProcessFilmstrip(strip, prompt, layout, opts)
	res.Candidate.RegenerationOf = prev.ID
	return res, err
}

// DefaultTargetAnchors returns the normalized target anchors for a layout:
// the feet anchor at bottom-center (脚底), the default identity anchor
// position.
func DefaultTargetAnchors(layout FrameList) []AnchorPoint {
	w, h := layout.Canvas.UnitWidth, layout.Canvas.UnitHeight
	return []AnchorPoint{
		{Name: "feet", X: w / 2, Y: h - 1},
	}
}
