package pipeline

import (
	"image"
	"image/color"
	"math"
)

// QualityInput is everything ScoreCandidate needs: the (pre-quantization)
// corrected frames, the expected layout, the alignment transforms (their
// clipping produces the out-of-bounds ratio), per-frame corrected anchors
// with their normalized targets, the shared palette, and an optional
// cross-direction mirror pair for symmetry scoring.
type QualityInput struct {
	Frames        []*image.RGBA
	Layout        FrameList
	Transforms    []FrameTransform
	AnchorSets    [][]AnchorPoint
	TargetAnchors []AnchorPoint
	Palette       []color.RGBA
	MirrorPair    [2]*image.RGBA
}

// QualityScores is the quantified metric report for one generated candidate
// (task 8.1): structural metrics (切片完整性、锚点偏差、帧间面积/重心抖动) and
// rule metrics (色数/调色板一致性、镜像对称性、越界像素占比). Every value is
// deterministic for identical input.
type QualityScores struct {
	SliceCompleteness  float64 `json:"sliceCompleteness"`  // 切片完整性 0..1
	AnchorDeviationPx  float64 `json:"anchorDeviationPx"`  // 锚点偏差 (px, 均值)
	AnchorDeviation    float64 `json:"anchorDeviation"`    // 锚点偏差 0..1 (归一化)
	AreaJitter         float64 `json:"areaJitter"`         // 帧间面积抖动 0..1
	CentroidJitter     float64 `json:"centroidJitter"`     // 帧间重心抖动 0..1
	ColorCount         int     `json:"colorCount"`         // 色数
	PaletteConsistency float64 `json:"paletteConsistency"` // 调色板一致性 0..1
	MirrorSymmetry     float64 `json:"mirrorSymmetry"`     // 镜像对称性 0..1
	OutOfBoundsRatio   float64 `json:"outOfBoundsRatio"`   // 越界像素占比 0..1
	Overall            float64 `json:"overall"`            // 综合评分 0..1
}

// Overall weights (sum = 1.0). Documented in the phase-4 design: the four
// phase-4 dimensions map to 帧数→SliceCompleteness, 锚点→AnchorDeviation,
// 运动→Area/CentroidJitter, 身份→PaletteConsistency; 镜像对称性与越界像素占比 are
// rule metrics.
const (
	weightSliceCompleteness  = 0.25
	weightAnchorDeviation    = 0.20
	weightAreaJitter         = 0.15
	weightCentroidJitter     = 0.10
	weightPaletteConsistency = 0.10
	weightMirrorSymmetry     = 0.10
	weightOutOfBounds        = 0.10
)

// ScoreCandidate computes the quantified quality metrics for a candidate
// (task 8.1). Scores are advisory — the acceptance gate (task 8.3) combines
// thresholds with user confirmation.
func ScoreCandidate(in QualityInput) QualityScores {
	s := QualityScores{}

	// 结构指标: 切片完整性 (帧数 + 尺寸).
	countOK := len(in.Frames) == in.Layout.FrameCount
	sizeOK := true
	for _, f := range in.Frames {
		if f == nil || f.Bounds().Dx() != in.Layout.Canvas.UnitWidth || f.Bounds().Dy() != in.Layout.Canvas.UnitHeight {
			sizeOK = false
			break
		}
	}
	if countOK && sizeOK {
		s.SliceCompleteness = 1
	}

	// 结构指标: 锚点偏差 (corrected anchors vs normalized targets, 均值 px).
	totalDev, pairs := 0.0, 0
	for fi := 0; fi < len(in.AnchorSets) && fi < len(in.Frames); fi++ {
		set := in.AnchorSets[fi]
		n := min(len(set), len(in.TargetAnchors))
		for i := 0; i < n; i++ {
			totalDev += math.Hypot(float64(set[i].X-in.TargetAnchors[i].X), float64(set[i].Y-in.TargetAnchors[i].Y))
			pairs++
		}
	}
	if pairs > 0 {
		s.AnchorDeviationPx = totalDev / float64(pairs)
		if diag := math.Hypot(float64(in.Layout.Canvas.UnitWidth), float64(in.Layout.Canvas.UnitHeight)); diag > 0 {
			s.AnchorDeviation = math.Min(1, s.AnchorDeviationPx/diag)
		}
	}

	// 结构指标: 帧间面积抖动 (opaque-area coefficient of variation, 0..1).
	areas := make([]float64, 0, len(in.Frames))
	for _, f := range in.Frames {
		areas = append(areas, opaqueArea(f))
	}
	s.AreaJitter = coefficientOfVariation(areas)

	// 结构指标: 帧间重心抖动 (mean centroid distance from the sequence mean,
	// normalized by the canvas diagonal, 0..1).
	s.CentroidJitter = centroidJitter(in.Frames, in.Layout.Canvas.UnitWidth, in.Layout.Canvas.UnitHeight)

	// 规则指标: 色数 + 调色板一致性 (fraction of opaque pixels covered by the
	// shared palette; 身份一致性维度).
	distinct := map[color.RGBA]struct{}{}
	opaqueTotal, inPalette := 0, 0
	for _, f := range in.Frames {
		if f == nil {
			continue
		}
		b := f.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := f.RGBAAt(x, y)
				if c.A == 0 {
					continue
				}
				opaqueTotal++
				distinct[c] = struct{}{}
				if colorInPalette(c, in.Palette) {
					inPalette++
				}
			}
		}
	}
	s.ColorCount = len(distinct)
	if opaqueTotal > 0 {
		if len(in.Palette) == 0 {
			s.PaletteConsistency = 1 // not evaluated
		} else {
			s.PaletteConsistency = float64(inPalette) / float64(opaqueTotal)
		}
	}

	// 规则指标: 镜像对称性 (right/left pair pixel match after horizontal
	// flip; 1 when no pair is provided).
	s.MirrorSymmetry = mirrorSymmetry(in.MirrorPair)

	// 规则指标: 越界像素占比 (opaque pixels clipped by alignment / total).
	clipped, total := 0, 0
	for _, t := range in.Transforms {
		clipped += t.ClippedOpaque
		total += t.TotalOpaque
	}
	if total > 0 {
		s.OutOfBoundsRatio = float64(clipped) / float64(total)
	}

	// 综合评分.
	s.Overall = weightSliceCompleteness*s.SliceCompleteness +
		weightAnchorDeviation*(1-s.AnchorDeviation) +
		weightAreaJitter*(1-s.AreaJitter) +
		weightCentroidJitter*(1-s.CentroidJitter) +
		weightPaletteConsistency*s.PaletteConsistency +
		weightMirrorSymmetry*s.MirrorSymmetry +
		weightOutOfBounds*(1-s.OutOfBoundsRatio)
	return s
}

// opaqueArea returns the number of opaque pixels in a frame.
func opaqueArea(img *image.RGBA) float64 {
	if img == nil {
		return 0
	}
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := img.Pix[(y-b.Min.Y)*img.Stride:]
		for x := 0; x < b.Dx(); x++ {
			if row[x*4+3] > 0 {
				n++
			}
		}
	}
	return float64(n)
}

// coefficientOfVariation returns stddev/mean clamped to [0,1]. A degenerate
// all-empty sequence reports 1 (max jitter) to flag emptiness.
func coefficientOfVariation(v []float64) float64 {
	n := len(v)
	if n == 0 {
		return 0
	}
	mean := 0.0
	for _, x := range v {
		mean += x
	}
	mean /= float64(n)
	if mean == 0 {
		return 1
	}
	variance := 0.0
	for _, x := range v {
		d := x - mean
		variance += d * d
	}
	variance /= float64(n)
	return math.Min(1, math.Sqrt(variance)/mean)
}

// centroidJitter returns the mean distance of each frame's alpha-weighted
// centroid from the sequence-mean centroid, normalized by the canvas diagonal
// and clamped to [0,1].
func centroidJitter(frames []*image.RGBA, w, h int) float64 {
	n := len(frames)
	if n == 0 {
		return 0
	}
	diag := math.Hypot(float64(w), float64(h))
	if diag == 0 {
		return 0
	}
	centroids := make([][2]float64, n)
	meanX, meanY := 0.0, 0.0
	for i, f := range frames {
		cx, cy, _ := AlphaCentroid(f)
		centroids[i] = [2]float64{cx, cy}
		meanX += cx
		meanY += cy
	}
	meanX /= float64(n)
	meanY /= float64(n)
	total := 0.0
	for _, c := range centroids {
		total += math.Hypot(c[0]-meanX, c[1]-meanY)
	}
	meanDist := total / float64(n)
	return math.Min(1, meanDist/diag)
}

// colorInPalette reports whether c matches a palette entry exactly.
func colorInPalette(c color.RGBA, palette []color.RGBA) bool {
	for _, p := range palette {
		if c.R == p.R && c.G == p.G && c.B == p.B {
			return true
		}
	}
	return false
}

// mirrorSymmetry compares a generated frame with its mirrored counterpart
// (e.g., right vs left): the fraction of pixels where the frame equals the
// horizontal flip of the counterpart, counting alpha presence and color.
// Returns 1 (not penalized) when no pair is provided.
func mirrorSymmetry(pair [2]*image.RGBA) float64 {
	a, b := pair[0], pair[1]
	if a == nil || b == nil {
		return 1
	}
	w, h := a.Bounds().Dx(), a.Bounds().Dy()
	if b.Bounds().Dx() != w || b.Bounds().Dy() != h {
		return 0
	}
	total := w * h
	if total == 0 {
		return 1
	}
	matches := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ca := a.RGBAAt(x, y)
			cb := b.RGBAAt(w-1-x, y) // horizontal flip of the counterpart
			same := (ca.A == 0 && cb.A == 0) ||
				(ca.A > 0 && cb.A > 0 && ca.R == cb.R && ca.G == cb.G && ca.B == cb.B)
			if same {
				matches++
			}
		}
	}
	return float64(matches) / float64(total)
}
