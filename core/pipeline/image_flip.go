package pipeline

import "image"

// FlipHorizontal mirrors an RGBA image along its vertical center axis
// (水平翻转，左右镜像). It returns a NEW image and never mutates the input.
// The operation is its own inverse: applying it twice restores the original
// pixels exactly, so callers can flip an image back without keeping the
// original bytes.
func FlipHorizontal(img *image.RGBA) *image.RGBA {
	if img == nil {
		return nil
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.SetRGBA(x, y, img.RGBAAt(b.Min.X+b.Dx()-1-x, b.Min.Y+y))
		}
	}
	return out
}
