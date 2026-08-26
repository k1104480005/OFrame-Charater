package pipeline

import (
	"image"
	"image/color"
	"testing"
)

// TestAlphaCentroidFrameRelativeCoordinates verifies AlphaCentroid returns
// frame-relative coordinates (x/y/foot in one coordinate space): the same
// opaque content produces identical centroid and foot values regardless of
// where the frame sits in a larger canvas (bounds.Min != (0,0)).
func TestAlphaCentroidFrameRelativeCoordinates(t *testing.T) {
	base := image.NewRGBA(image.Rect(0, 0, 32, 32))
	offset := image.NewRGBA(image.Rect(7, 3, 39, 35))
	for y := 10; y < 20; y++ {
		for x := 5; x < 15; x++ {
			c := color.RGBA{R: 120, G: 200, B: 80, A: 255}
			base.SetRGBA(x, y, c)
			offset.SetRGBA(x+7, y+3, c) // same frame-relative position (5..14, 10..19)
		}
	}

	cx1, cy1, foot1 := AlphaCentroid(base)
	cx2, cy2, foot2 := AlphaCentroid(offset)
	if cx1 != cx2 || cy1 != cy2 || foot1 != foot2 {
		t.Fatalf("frame-relative centroid/foot depend on canvas origin: base=(%.3f,%.3f,%d) offset=(%.3f,%.3f,%d)",
			cx1, cy1, foot1, cx2, cy2, foot2)
	}
	// Sanity: the values are those of the content inside its own frame.
	if cx1 != 9.5 || cy1 != 14.5 || foot1 != 19 {
		t.Errorf("frame-relative centroid/foot = (%.3f,%.3f,%d), want (9.5,14.5,19)", cx1, cy1, foot1)
	}
}
