package pipeline

import "image"

// AlphaProjection returns, for each column x, the summed alpha over all rows
// — the opacity mass per column. Transparent gutters between frames appear as
// valleys, which the DP slicing (task 5.3) uses to place cuts.
func AlphaProjection(img *image.RGBA) []int {
	if img == nil {
		return nil
	}
	b := img.Bounds()
	w := b.Dx()
	proj := make([]int, w)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := img.Pix[(y-b.Min.Y)*img.Stride:]
		for x := 0; x < w; x++ {
			proj[x] += int(row[x*4+3])
		}
	}
	return proj
}

// OpaqueRatio returns the fraction of pixels with alpha > 0 (0..1).
func OpaqueRatio(img *image.RGBA) float64 {
	if img == nil {
		return 0
	}
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return 0
	}
	opaque := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := img.Pix[(y-b.Min.Y)*img.Stride:]
		for x := 0; x < b.Dx(); x++ {
			if row[x*4+3] > 0 {
				opaque++
			}
		}
	}
	return float64(opaque) / float64(b.Dx()*b.Dy())
}

// AlphaCentroid returns the alpha-weighted centroid (cx, cy) of a frame's
// opaque pixels and its foot row — the lowest row containing an opaque pixel
// (the shared-baseline reference; -1 when the frame is empty). All returned
// coordinates are frame-relative: (0, 0) is the frame's origin (bounds.Min),
// so the values are independent of where the frame sits in a larger canvas
// (x and y use the same coordinate space).
func AlphaCentroid(img *image.RGBA) (cx, cy float64, footY int) {
	if img == nil {
		return 0, 0, -1
	}
	b := img.Bounds()
	var sumA, sumAX, sumAY float64
	footY = -1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		relY := y - b.Min.Y
		row := img.Pix[(y-b.Min.Y)*img.Stride:]
		for x := 0; x < b.Dx(); x++ {
			a := float64(row[x*4+3])
			if a == 0 {
				continue
			}
			sumA += a
			sumAX += a * float64(x)
			sumAY += a * float64(relY)
			if relY > footY {
				footY = relY
			}
		}
	}
	if sumA == 0 {
		return 0, 0, -1
	}
	return sumAX / sumA, sumAY / sumA, footY
}
