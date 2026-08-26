package pipeline

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// blockFrame builds a 32×32 frame with a 10×10 opaque block at (bx,by).
func blockFrame(bx, by int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := by; y < by+10; y++ {
		for x := bx; x < bx+10; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 120, G: 200, B: 80, A: 255})
		}
	}
	return img
}

func TestAlignSequenceSharedBaseline(t *testing.T) {
	// Frame A content at x=5..14, y=10..19; frame B at x=12..21, y=2..11.
	frames := []*image.RGBA{blockFrame(5, 10), blockFrame(12, 2)}
	aligned, transforms, err := AlignSequence(frames, AlignOptions{})
	if err != nil {
		t.Fatalf("AlignSequence: %v", err)
	}
	if len(aligned) != 2 || len(transforms) != 2 {
		t.Fatalf("aligned=%d transforms=%d, want 2/2", len(aligned), len(transforms))
	}
	// Both frames must share the baseline (foot row = 31 = bottom).
	for i, f := range aligned {
		_, _, foot := AlphaCentroid(f)
		if foot != 31 {
			t.Errorf("frame %d foot = %d, want 31 (shared baseline)", i, foot)
		}
	}
	// Both centroids must land on the shared horizontal axis (CenterX = 16).
	for i, f := range aligned {
		cx, _, _ := AlphaCentroid(f)
		if math.Round(cx) != 16 {
			t.Errorf("frame %d centroid x = %v, want ~16", i, cx)
		}
	}
	// Expected integer transforms: A dx=6 dy=12, B dx=-1 dy=20.
	want := []FrameTransform{{Dx: 6, Dy: 12}, {Dx: -1, Dy: 20}}
	for i, tr := range transforms {
		if tr.Dx != want[i].Dx || tr.Dy != want[i].Dy {
			t.Errorf("frame %d transform = (%d,%d), want (%d,%d)", i, tr.Dx, tr.Dy, want[i].Dx, want[i].Dy)
		}
	}
	// After alignment both frames hold identical content (same region).
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if aligned[0].RGBAAt(x, y) != aligned[1].RGBAAt(x, y) {
				t.Fatalf("aligned frames differ at (%d,%d): %v vs %v", x, y, aligned[0].RGBAAt(x, y), aligned[1].RGBAAt(x, y))
			}
		}
	}
}

func TestAlignSequenceEmptyFrame(t *testing.T) {
	empty := image.NewRGBA(image.Rect(0, 0, 32, 32))
	frames := []*image.RGBA{empty, blockFrame(5, 10)}
	aligned, transforms, err := AlignSequence(frames, AlignOptions{})
	if err != nil {
		t.Fatalf("AlignSequence: %v", err)
	}
	if transforms[0].Dx != 0 || transforms[0].Dy != 0 {
		t.Errorf("empty frame transform = (%d,%d), want (0,0)", transforms[0].Dx, transforms[0].Dy)
	}
	if _, _, foot := AlphaCentroid(aligned[0]); foot != -1 {
		t.Errorf("empty frame foot = %d, want -1", foot)
	}
}

func TestTranslateFrameClipping(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 4; y++ {
		for x := 28; x < 32; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	shifted, clipped, total := TranslateFrame(img, 4, 0)
	if total != 16 {
		t.Fatalf("total opaque = %d, want 16", total)
	}
	if clipped != 16 {
		t.Errorf("clipped = %d, want 16 (all content pushed out of bounds)", clipped)
	}
	if _, _, foot := AlphaCentroid(shifted); foot != -1 {
		t.Errorf("shifted frame should be empty, foot = %d", foot)
	}
}

func TestCorrectAnchors(t *testing.T) {
	anchors := []AnchorPoint{{Name: "feet", X: 5, Y: 10}, {Name: "hand", X: 30, Y: 15}}
	corrected := CorrectAnchors(anchors, FrameTransform{Dx: 6, Dy: 12})
	want := []AnchorPoint{{Name: "feet", X: 11, Y: 22}, {Name: "hand", X: 36, Y: 27}}
	for i := range want {
		if corrected[i].X != want[i].X || corrected[i].Y != want[i].Y {
			t.Errorf("anchor %d = (%d,%d), want (%d,%d)", i, corrected[i].X, corrected[i].Y, want[i].X, want[i].Y)
		}
	}
}
