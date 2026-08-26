package pipeline

import (
	"image"
	"image/color"
	"path/filepath"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
)

// TestSaveLoadCandidatePixelIdentity verifies task 5.5's backend guarantee:
// the processed frames persisted for preview/acceptance survive a
// save → load round trip pixel-identically (lossless PNG), so the preview
// renders exactly what the pipeline sliced.
func TestSaveLoadCandidatePixelIdentity(t *testing.T) {
	canvas, err := identity.NewCanvasSpec(16, 16)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := NormalizeFrameList(*canvas, 3)
	if err != nil {
		t.Fatal(err)
	}
	frames := []*image.RGBA{
		patternFrame(16, 16, 0),
		patternFrame(16, 16, 1),
		patternFrame(16, 16, 2),
	}
	anchors := [][]AnchorPoint{
		{{Name: "feet", X: 8, Y: 15}},
		{{Name: "feet", X: 8, Y: 15}},
		{{Name: "feet", X: 8, Y: 15}},
	}
	strip, err := AssembleFilmstrip(frames, layout)
	if err != nil {
		t.Fatal(err)
	}
	pngData, err := EncodeFilmstripPNG(strip)
	if err != nil {
		t.Fatal(err)
	}
	c := Candidate{
		ID:           "cand-1",
		Direction:    "down",
		FilmstripPNG: pngData,
		Frames:       frames,
		AnchorSets:   anchors,
		Layout:       layout,
		Status:       CandidatePending,
	}
	dir := filepath.Join(t.TempDir(), "cand-1")
	if err := SaveCandidate(dir, c); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCandidate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "cand-1" || got.Direction != "down" || got.Status != CandidatePending {
		t.Fatalf("candidate meta lost: %+v", got)
	}
	if len(got.Frames) != 3 || len(got.AnchorSets) != 3 {
		t.Fatalf("frames/anchors lost: %d/%d", len(got.Frames), len(got.AnchorSets))
	}
	for i := range frames {
		if !pixelEqual(got.Frames[i], frames[i]) {
			t.Fatalf("frame %d changed across the PNG round trip (PixelPerfect violated)", i)
		}
		if got.AnchorSets[i][0] != anchors[i][0] {
			t.Fatalf("anchor %d lost: %+v", i, got.AnchorSets[i])
		}
	}
	if len(got.FilmstripPNG) != len(pngData) {
		t.Fatalf("filmstrip artifact changed")
	}
}

// patternFrame is a deterministic 16×16 frame: checkerboard alpha + a colored
// 8×8 block whose color depends on the frame index.
func patternFrame(w, h, seed int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	col := color.RGBA{R: uint8(40 + seed*50), G: 90, B: 200, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (x+y+seed)%2 == 0 {
				img.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 255, A: 255}) // magenta key bg
			} else {
				img.SetRGBA(x, y, color.RGBA{}) // transparent
			}
		}
	}
	for y := 4; y < 12; y++ {
		for x := 4; x < 12; x++ {
			img.SetRGBA(x, y, col)
		}
	}
	return img
}

func pixelEqual(a, b *image.RGBA) bool {
	if a == nil || b == nil || a.Bounds() != b.Bounds() {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}
