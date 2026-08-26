package edit

import (
	"image"
	"image/color"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/pipeline"
)

func testFrame(w, h int, c color.RGBA) Frame {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return Frame{Image: img, DurationMs: 100, AnchorX: 2, AnchorY: 3}
}

func TestPixelEraseCropAndReplayPreserveBase(t *testing.T) {
	base := Sequence{Frames: []Frame{testFrame(4, 4, color.RGBA{R: 10, A: 255})}}
	original := CloneSequence(base)
	if err := base.Apply(Instruction{Kind: "pixel", FrameIndex: 0, X: 1, Y: 1, Color: color.RGBA{G: 255, A: 255}}); err != nil {
		t.Fatal(err)
	}
	if err := base.Apply(Instruction{Kind: "erase", FrameIndex: 0, X: 0, Y: 0}); err != nil {
		t.Fatal(err)
	}
	if err := base.Apply(Instruction{Kind: "crop", FrameIndex: 0, X: 1, Y: 1, Width: 2, Height: 2}); err != nil {
		t.Fatal(err)
	}
	replayed, err := Replay(original, base.Instructions)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Frames[0].Image.Bounds().Dx() != 2 || replayed.Frames[0].Image.Bounds().Dy() != 2 {
		t.Fatalf("replay crop bounds = %v", replayed.Frames[0].Image.Bounds())
	}
	if original.Frames[0].Image.RGBAAt(0, 0) != (color.RGBA{R: 10, A: 255}) {
		t.Fatal("base frame was mutated")
	}
}

func TestCleanupRemovesEdgeBackgroundKeepsInterior(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 5, 5))
	for y := 0; y < 5; y++ {
		for x := 0; x < 5; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, A: 0})
		}
	}
	img.SetRGBA(2, 2, color.RGBA{R: 255, A: 255})
	img.SetRGBA(2, 1, color.RGBA{R: 255, A: 0})
	s := Sequence{Frames: []Frame{{Image: img}}}
	if err := s.Apply(Instruction{Kind: "cleanup", FrameIndex: 0}); err != nil {
		t.Fatal(err)
	}
	if s.Frames[0].Image.RGBAAt(2, 2).A != 255 {
		t.Fatal("cleanup removed opaque subject")
	}
	if s.Frames[0].Image.RGBAAt(0, 0).A != 0 {
		t.Fatal("edge background remains")
	}
}

func TestSequenceOperationsAreReplayable(t *testing.T) {
	base := Sequence{Frames: []Frame{
		testFrame(2, 2, color.RGBA{R: 1, A: 255}),
		testFrame(2, 2, color.RGBA{G: 2, A: 255}),
	}}
	if err := base.Reorder([]int{1, 0}); err != nil {
		t.Fatal(err)
	}
	if err := base.Insert(1, testFrame(2, 2, color.RGBA{B: 3, A: 255})); err != nil {
		t.Fatal(err)
	}
	if err := base.Delete(0); err != nil {
		t.Fatal(err)
	}
	if err := base.Apply(Instruction{Kind: "duration", FrameIndex: 0, DurationMs: 240}); err != nil {
		t.Fatal(err)
	}
	if err := base.ApplyAnchorDelta(4, -1, true, 0); err != nil {
		t.Fatal(err)
	}
	replayed, err := Replay(Sequence{Frames: []Frame{
		testFrame(2, 2, color.RGBA{R: 1, A: 255}),
		testFrame(2, 2, color.RGBA{G: 2, A: 255}),
	}}, base.Instructions)
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed.Frames) != len(base.Frames) || len(replayed.Instructions) != len(base.Instructions) {
		t.Fatalf("replayed sequence differs: frames %d/%d instructions %d/%d", len(replayed.Frames), len(base.Frames), len(replayed.Instructions), len(base.Instructions))
	}
	for i := range base.Frames {
		if replayed.Frames[i].DurationMs != base.Frames[i].DurationMs || replayed.Frames[i].AnchorX != base.Frames[i].AnchorX || replayed.Frames[i].AnchorY != base.Frames[i].AnchorY {
			t.Fatalf("frame %d metadata differs: %#v vs %#v", i, replayed.Frames[i], base.Frames[i])
		}
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				if replayed.Frames[i].Image.RGBAAt(x, y) != base.Frames[i].Image.RGBAAt(x, y) {
					t.Fatalf("frame %d pixel %d,%d differs", i, x, y)
				}
			}
		}
	}
}

func TestBatchAnchorDelta(t *testing.T) {
	s := Sequence{Frames: []Frame{testFrame(2, 2, color.RGBA{A: 255}), testFrame(2, 2, color.RGBA{A: 255})}}
	if err := s.ApplyAnchorDelta(3, 5, true, 0); err != nil {
		t.Fatal(err)
	}
	for i, f := range s.Frames {
		if f.AnchorX != 5 || f.AnchorY != 8 {
			t.Fatalf("frame %d anchor = (%d,%d)", i, f.AnchorX, f.AnchorY)
		}
	}
	if len(s.Instructions) != 2 {
		t.Fatalf("batch recorded %d instructions, want 2", len(s.Instructions))
	}
}

func TestEditedFramesCanReenterQualityScoring(t *testing.T) {
	s := Sequence{Frames: []Frame{testFrame(2, 2, color.RGBA{R: 40, A: 255})}}
	if err := s.Apply(Instruction{Kind: "pixel", FrameIndex: 0, X: 0, Y: 0, Color: color.RGBA{G: 80, A: 255}}); err != nil {
		t.Fatal(err)
	}
	canvas, err := identity.NewCanvasSpec(2, 2)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := pipeline.NormalizeFrameList(*canvas, len(s.Frames))
	if err != nil {
		t.Fatal(err)
	}
	frames := make([]*image.RGBA, len(s.Frames))
	for i, f := range s.Frames {
		frames[i] = f.Image
	}
	scores := pipeline.ScoreCandidate(pipeline.QualityInput{Frames: frames, Layout: layout})
	if scores.SliceCompleteness != 1 {
		t.Fatalf("edited frames were not accepted by quality scorer: %#v", scores)
	}
}
