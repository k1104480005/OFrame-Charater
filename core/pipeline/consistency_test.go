package pipeline

import (
	"image"
	"image/color"
	"testing"
)

// TestCoarseConsistencyScore verifies task 8.2's local heuristic: the coarse
// same-character consistency score is deterministic, bounded 0..1, and reacts
// to geometry and mirror-symmetry evidence.
func TestCoarseConsistencyScore(t *testing.T) {
	frames := func(size int) []*image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, size, size))
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				img.SetRGBA(x, y, color.RGBA{R: 10, G: 20, B: 30, A: 255})
			}
		}
		return []*image.RGBA{img, img}
	}
	dir := DirectionConsistency{Direction: "down", Frames: frames(16), Scores: QualityScores{PaletteConsistency: 1}}

	// Single consistent direction, no mirror pair: high score.
	high := CoarseConsistencyScore(ConsistencyInput{Directions: []DirectionConsistency{dir}})
	if high != 1 {
		t.Fatalf("consistent input scored %v, want 1", high)
	}

	// Geometry mismatch (different frame sizes) lowers the score.
	mixed := DirectionConsistency{Direction: "up", Frames: frames(8), Scores: QualityScores{PaletteConsistency: 1}}
	low := CoarseConsistencyScore(ConsistencyInput{Directions: []DirectionConsistency{dir, mixed}})
	if low >= high {
		t.Fatalf("geometry mismatch should lower the score: %v vs %v", low, high)
	}

	// Deterministic: same input → same output.
	again := CoarseConsistencyScore(ConsistencyInput{Directions: []DirectionConsistency{dir}})
	if again != high {
		t.Fatalf("score not deterministic: %v vs %v", again, high)
	}

	// A mismatched mirror pair (source vs a wildly different derived frame)
	// is penalized.
	a := image.NewRGBA(image.Rect(0, 0, 8, 8))
	b := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			a.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			b.SetRGBA(x, y, color.RGBA{R: 1, G: 2, B: 3, A: 255})
		}
	}
	bad := CoarseConsistencyScore(ConsistencyInput{
		Directions:  []DirectionConsistency{dir},
		MirrorPairs: [][2]*image.RGBA{{a, b}},
	})
	if bad >= high {
		t.Fatalf("bad mirror symmetry should lower the score: %v vs %v", bad, high)
	}
	// Empty input is "not evaluated" (1).
	if empty := CoarseConsistencyScore(ConsistencyInput{}); empty != 1 {
		t.Fatalf("empty input scored %v, want 1", empty)
	}
}
