package pipeline

import (
	"image"
)

// DirectionConsistency is one direction's evidence for the coarse
// same-character consistency score (task 8.2): its processed frames and its
// rule-level quality scores.
type DirectionConsistency struct {
	Direction string
	Frames    []*image.RGBA
	Scores    QualityScores
}

// ConsistencyInput is everything CoarseConsistencyScore needs: the directions
// of a motion/package and the mirror pairs (source, derived) available for
// symmetry evidence.
type ConsistencyInput struct {
	Directions  []DirectionConsistency
	MirrorPairs [][2]*image.RGBA // (source frame, derived frame) pairs
}

// CoarseConsistencyScore returns a deterministic 0..1 reference score for the
// same-character consistency of a direction set (task 8.2: AI 辅助一致性粗评分
// 仅参考、不阻塞流程). This is the local-model heuristic — always available
// offline — built from rule-level evidence:
//
//   - mirror symmetry of derived directions vs their sources (镜像对称性),
//   - palette agreement across directions (调色板一致性, from each direction's
//     rule scores),
//   - frame-geometry consistency (all frames share the canvas size).
//
// The optional provider-based AI score is layered on top by the service layer;
// this score is advisory only and NEVER blocks the acceptance gate.
func CoarseConsistencyScore(in ConsistencyInput) float64 {
	// 1. Mirror symmetry evidence.
	mirrorSym := 1.0
	if len(in.MirrorPairs) > 0 {
		total := 0.0
		for _, pair := range in.MirrorPairs {
			total += mirrorSymmetry(pair)
		}
		mirrorSym = total / float64(len(in.MirrorPairs))
	}

	// 2. Palette agreement across directions.
	palette := 1.0
	if len(in.Directions) > 0 {
		total := 0.0
		for _, d := range in.Directions {
			total += d.Scores.PaletteConsistency
		}
		palette = total / float64(len(in.Directions))
	}

	// 3. Frame-geometry consistency: every frame of every direction shares the
	// same size (conforms to the logical canvas).
	geometry := 1.0
	var refW, refH int
	first := true
outer:
	for _, d := range in.Directions {
		for _, f := range d.Frames {
			if f == nil {
				geometry = 0
				break outer
			}
			w, h := f.Bounds().Dx(), f.Bounds().Dy()
			if first {
				refW, refH = w, h
				first = false
				continue
			}
			if w != refW || h != refH {
				geometry = 0
				break outer
			}
		}
	}

	// Reference-only composite (weights sum to 1).
	return 0.5*mirrorSym + 0.3*palette + 0.2*geometry
}
