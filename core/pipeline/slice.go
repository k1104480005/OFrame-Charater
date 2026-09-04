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
	// the target frame width is treated as a missing/wrong-size frame and the
	// slicing fails (default 0.5).
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
			return nil, fmt.Errorf("pipeline: slicing result does not match specification: frame %d width %d < %v of frame width %d (missing frames or wrong sizes)",
				i, w, opts.MinFrameWidthFraction, frameWidth)
		}
	}
	return cuts, nil
}

// SliceFilmstrip slices the (already keyed) filmstrip into exactly FrameCount
// independent frames at the DP-optimal boundaries (task 5.3). Each slice is
// fitted to the logical canvas with integer-pixel rules only (FitToCanvas —
// oversized slices are proportionally downscaled, never cropped). All-or-
// nothing: on any mismatch the task fails with the recorded reason and no
// partial frames are produced.
//
// 布局守卫（模型出图尺寸不可控）：返回画布大于规格条带时（模型常把条带画在
// 更大画布的中部带区），先裁到内容包围盒去掉"信箱边"，帧按内容实际尺寸比例
// 缩放装格。随后做两层帧结构守卫：① 帧间隙守卫 —— 抠图后的合规条带（帧数
// ≥ 2）必然存在完全透明的列（姿势间的键控间隙）；整幅无任何透明列说明模型
// 无视键控/条带契约画了满幅内容，立即给出可读失败。② 带区数量守卫 —— 可分
// 离姿势带区少于规格帧数说明模型少画/粘连了姿势，DP 无法凭空补足，同样可读
// 失败（两者都在重试预算内换图重试），绝不把错位内容硬切成压缩半身的怪异帧。
func SliceFilmstrip(strip *image.RGBA, layout FrameList, opts SliceOptions) ([]*image.RGBA, error) {
	if strip == nil {
		return nil, fmt.Errorf("pipeline: filmstrip is nil")
	}
	b := strip.Bounds()
	if b.Dy() > layout.StripHeight || b.Dx() > layout.StripWidth {
		if x0, y0, x1, y1, ok := ContentBounds(strip); ok {
			strip = CropRGBA(strip, x0, y0, x1, y1)
			b = strip.Bounds()
		}
	}
	if layout.FrameCount > 1 {
		gapless := true
		for _, v := range AlphaProjection(strip) {
			if v == 0 {
				gapless = false
				break
			}
		}
		if gapless {
			return nil, fmt.Errorf(
				"pipeline: no transparent gap between poses after keying — the model ignored the magenta-background / horizontal-strip contract (%d poses of %dx%d expected)",
				layout.FrameCount, layout.Canvas.UnitWidth, layout.Canvas.UnitHeight)
		}
	}
	proj := AlphaProjection(strip)
	// 带区守卫：抠像后可分离的姿势带区（被透明间隙隔开的内容段）必须不少于
	// 规格帧数 —— 模型少画了姿势时 DP 无法凭空补足，继续切只会把现有姿势
	// 压缩错位成半身帧，直接给出可读失败（重试预算内换图重试）。
	if spans := ContentSpans(proj); spans < layout.FrameCount {
		return nil, fmt.Errorf(
			"pipeline: filmstrip holds %d separable pose bands, expected %d frames (missing or merged poses — the model ignored the horizontal-strip contract)",
			spans, layout.FrameCount)
	}
	// DP 的目标帧宽取条带实宽均分（裁边后每格的真实宽度），而不是逻辑画布单元
	// 宽：stripImageSize 让模型在更大画布上出图（如 4 帧画 2048×877，每格实际
	// 512px），若仍按单元宽 256 定刀口，刀口全部落在 256 的整数倍上，把每个
	// 姿势切成半身（对齐 perfectpixel：按条带实际几何切分；规格条带
	// StripWidth == FrameCount×UnitWidth 时两者相等，行为不变）。
	cellWidth := b.Dx() / layout.FrameCount
	if cellWidth < 1 {
		cellWidth = 1
	}
	cuts, err := OptimalCuts(proj, b.Dy(), cellWidth, layout.FrameCount, opts)
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

// ContentSpans counts the maximal runs of columns with opaque content
// (projection > 0) — i.e. the pose bands separated by fully transparent
// gutters on a keyed strip.
func ContentSpans(proj []int) int {
	spans, inSpan := 0, false
	for _, v := range proj {
		if v > 0 {
			if !inSpan {
				spans++
				inSpan = true
			}
		} else {
			inSpan = false
		}
	}
	return spans
}

// ContentBounds returns the bounding box of opaque content (alpha > 0) in
// [min, max) form within the image's own bounds space; ok is false for a
// fully transparent image.
func ContentBounds(img *image.RGBA) (x0, y0, x1, y1 int, ok bool) {
	if img == nil {
		return 0, 0, 0, 0, false
	}
	b := img.Bounds()
	minX, minY, maxX, maxY := -1, -1, -1, -1
	for y := 0; y < b.Dy(); y++ {
		row := img.Pix[y*img.Stride:]
		for x := 0; x < b.Dx(); x++ {
			if row[x*4+3] > 0 {
				// minX must be the true MINIMUM across all rows: the topmost
				// content row alone decides nothing (a pose's head can start
				// far right of another pose's foot — locking minX to the
				// first scan-order hit amputates whole poses in the
				// letterbox crop, the half-body root cause).
				if minX < 0 || x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if minY < 0 {
					minY = y
				}
				maxY = y
			}
		}
	}
	if minX < 0 {
		return 0, 0, 0, 0, false
	}
	return b.Min.X + minX, b.Min.Y + minY, b.Min.X + maxX + 1, b.Min.Y + maxY + 1, true
}

// CropRGBA extracts the [x0,x1) × [y0,y1) rectangle (intersected with the
// source bounds) as a new image whose origin is (0,0).
func CropRGBA(src *image.RGBA, x0, y0, x1, y1 int) *image.RGBA {
	b := src.Bounds()
	r := image.Rect(x0, y0, x1, y1).Intersect(b)
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		srcRow := src.Pix[(y+r.Min.Y-b.Min.Y)*src.Stride:]
		dstRow := out.Pix[y*out.Stride:]
		copy(dstRow, srcRow[(r.Min.X-b.Min.X)*4:(r.Max.X-b.Min.X)*4])
	}
	return out
}
