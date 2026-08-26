package pipeline

import (
	"fmt"
	"image"
	"math"
)

// SliceOptions tunes the deterministic DP slicing (task 5.3).
type SliceOptions struct {
	// WidthDeviationWeight penalizes a frame whose width deviates from the
	// canvas unit width (default 1.0).
	WidthDeviationWeight float64
	// CutPenaltyWeight penalizes placing a cut through opaque content so cuts
	// prefer transparent gutters (default 1.5).
	CutPenaltyWeight float64
	// MinFrameWidthFraction: a produced frame narrower than this fraction of
	// the unit width is treated as a missing/wrong-size frame and the slicing
	// fails (default 0.5).
	MinFrameWidthFraction float64
}

func (o SliceOptions) withDefaults() SliceOptions {
	if o.WidthDeviationWeight == 0 {
		o.WidthDeviationWeight = 1.0
	}
	if o.CutPenaltyWeight == 0 {
		o.CutPenaltyWeight = 1.5
	}
	if o.MinFrameWidthFraction <= 0 {
		o.MinFrameWidthFraction = 0.5
	}
	return o
}

// OptimalCuts finds the optimal frame boundaries for a filmstrip via dynamic
// programming over the alpha projection: exactly frameCount frames, with cuts
// preferring transparent gutters and frame widths close to frameWidth. It
// returns frameCount+1 cut positions (first = 0, last = strip width), all
// integer pixel coordinates. The search is fully deterministic.
//
// Cost model (documented in the phase-4 design): each candidate frame
// [a,b) pays a width-deviation cost |(b-a)-frameWidth|/frameWidth weighted by
// WidthDeviationWeight, and each internal cut at column p pays an opaque-mass
// cost proj[p]/(255·stripHeight) weighted by CutPenaltyWeight. The DP finds
// the minimum-cost segmentation, so a clean strip (frames of exactly
// frameWidth separated by transparent gutters) cuts exactly at the gutters.
func OptimalCuts(proj []int, stripHeight, frameWidth, frameCount int, opts SliceOptions) ([]int, error) {
	opts = opts.withDefaults()
	n := len(proj)
	if n <= 0 || stripHeight <= 0 {
		return nil, fmt.Errorf("pipeline: empty filmstrip for slicing")
	}
	if frameWidth <= 0 || frameCount <= 0 {
		return nil, fmt.Errorf("pipeline: invalid slicing specification (%d frames of width %d)", frameCount, frameWidth)
	}
	if n < frameCount {
		return nil, fmt.Errorf("pipeline: filmstrip width %d cannot hold %d frames (missing frames)", n, frameCount)
	}

	maxAlpha := float64(stripHeight * 255)
	cutCost := func(x int) float64 {
		if x <= 0 || x >= n {
			return 0
		}
		return opts.CutPenaltyWeight * float64(proj[x]) / maxAlpha
	}
	widthCost := func(a, b int) float64 {
		dev := math.Abs(float64((b - a) - frameWidth))
		return opts.WidthDeviationWeight * dev / float64(frameWidth)
	}

	inf := math.Inf(1)
	kMax := frameCount
	dp := make([][]float64, n+1)
	bt := make([][]int, n+1)
	for i := 0; i <= n; i++ {
		dp[i] = make([]float64, kMax+1)
		bt[i] = make([]int, kMax+1)
		for k := 0; k <= kMax; k++ {
			dp[i][k] = inf
			bt[i][k] = -1
		}
	}
	dp[0][0] = 0
	for i := 1; i <= n; i++ {
		for k := 1; k <= kMax && k <= i; k++ {
			best, bestP := inf, -1
			for p := k - 1; p < i; p++ {
				if dp[p][k-1] == inf {
					continue
				}
				c := dp[p][k-1] + widthCost(p, i) + cutCost(p)
				if c < best {
					best = c
					bestP = p
				}
			}
			dp[i][k] = best
			bt[i][k] = bestP
		}
	}
	if dp[n][kMax] == inf {
		return nil, fmt.Errorf("pipeline: cannot slice filmstrip into %d frames", kMax)
	}

	// Backtrack the cuts.
	cuts := make([]int, 0, kMax+1)
	cur := n
	for k := kMax; k >= 1; k-- {
		p := bt[cur][k]
		if p < 0 {
			return nil, fmt.Errorf("pipeline: slicing backtrack failed")
		}
		cuts = append(cuts, p)
		cur = p
	}
	// cuts is currently [c_{K-1}, ..., c0]; reverse to [c0, ..., c_{K-1}],
	// then append the final boundary (the strip's right edge).
	for i, j := 0, len(cuts)-1; i < j; i, j = i+1, j-1 {
		cuts[i], cuts[j] = cuts[j], cuts[i]
	}
	cuts = append(cuts, n)

	// Validate the produced frame widths against the specification
	// (task 5.3: 切片与规格不符时任务失败且不产出部分资产).
	for i := 0; i < kMax; i++ {
		w := cuts[i+1] - cuts[i]
		if float64(w) < opts.MinFrameWidthFraction*float64(frameWidth) {
			return nil, fmt.Errorf("pipeline: slicing result does not match specification: frame %d width %d < %v of unit width %d (missing frames or wrong sizes)",
				i, w, opts.MinFrameWidthFraction, frameWidth)
		}
	}
	return cuts, nil
}

// SliceFilmstrip slices the filmstrip into exactly FrameCount independent
// frames at the DP-optimal boundaries (task 5.3). Each slice is fitted to the
// logical canvas with integer-pixel rules only (FitToCanvas, no secondary
// interpolation blur). All-or-nothing: on any mismatch the task fails with
// the recorded reason and no partial frames are produced.
func SliceFilmstrip(strip *image.RGBA, layout FrameList, opts SliceOptions) ([]*image.RGBA, error) {
	if strip == nil {
		return nil, fmt.Errorf("pipeline: filmstrip is nil")
	}
	b := strip.Bounds()
	proj := AlphaProjection(strip)
	cuts, err := OptimalCuts(proj, b.Dy(), layout.Canvas.UnitWidth, layout.FrameCount, opts)
	if err != nil {
		return nil, err
	}
	frames := make([]*image.RGBA, 0, layout.FrameCount)
	for i := 0; i < layout.FrameCount; i++ {
		x0, x1 := cuts[i], cuts[i+1]
		// Direct integer-pixel copy of columns [x0, x1) × full height.
		slice := image.NewRGBA(image.Rect(0, 0, x1-x0, b.Dy()))
		for y := 0; y < b.Dy(); y++ {
			srcRow := strip.Pix[(y-b.Min.Y)*strip.Stride:]
			dstRow := slice.Pix[y*slice.Stride:]
			copy(dstRow, srcRow[x0*4:x1*4])
		}
		f, err := FitToCanvas(slice, layout.Canvas.UnitWidth, layout.Canvas.UnitHeight)
		if err != nil {
			return nil, fmt.Errorf("pipeline: fit frame %d: %w", i, err)
		}
		frames = append(frames, f)
	}
	return frames, nil
}
