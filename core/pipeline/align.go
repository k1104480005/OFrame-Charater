package pipeline

import (
	"image"
	"math"
)

// AlignOptions controls the deterministic alpha-weighted centroid alignment
// and shared baseline (task 5.4: alpha-weighted centroid 对齐与共享基线).
type AlignOptions struct {
	// CenterX is the shared horizontal center column every frame's
	// alpha-weighted centroid x is aligned to (default canvas width/2).
	CenterX int
	// BaselineY is the shared baseline row every frame's foot (lowest opaque
	// row) is aligned to (default canvas height-1, the bottom row).
	BaselineY int
}

// FrameTransform is the integer pixel displacement applied to one frame,
// plus the opaque-pixel clipping it caused (the source of the out-of-bounds
// quality metric).
type FrameTransform struct {
	Dx            int
	Dy            int
	ClippedOpaque int
	TotalOpaque   int
}

// AnchorPoint is a named anchor coordinate on a frame/canvas (锚点).
type AnchorPoint struct {
	Name string
	X, Y int
}

// AlignSequence aligns every frame so its alpha-weighted centroid x lands on
// CenterX and its foot row lands on BaselineY, using integer pixel
// displacements only (task 5.4). All frames therefore share the same baseline
// and horizontal axis. Returns the aligned frames and their transforms.
func AlignSequence(frames []*image.RGBA, opts AlignOptions) ([]*image.RGBA, []FrameTransform, error) {
	if len(frames) == 0 {
		return nil, nil, nil
	}
	w := frames[0].Bounds().Dx()
	h := frames[0].Bounds().Dy()
	cx := opts.CenterX
	if cx == 0 {
		cx = w / 2
	}
	by := opts.BaselineY
	if by == 0 {
		by = h - 1
	}
	out := make([]*image.RGBA, 0, len(frames))
	transforms := make([]FrameTransform, 0, len(frames))
	for _, f := range frames {
		centroidX, _, foot := AlphaCentroid(f)
		var dx, dy int
		if foot >= 0 {
			dx = cx - int(math.Round(centroidX))
			dy = by - foot
		}
		shifted, clipped, total := TranslateFrame(f, dx, dy)
		out = append(out, shifted)
		transforms = append(transforms, FrameTransform{Dx: dx, Dy: dy, ClippedOpaque: clipped, TotalOpaque: total})
	}
	return out, transforms, nil
}

// TranslateFrame shifts content by the integer (dx, dy), clipping at the
// canvas edges. Returns the shifted frame and the clipped/total opaque pixel
// counts.
func TranslateFrame(src *image.RGBA, dx, dy int) (*image.RGBA, int, int) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewRGBA(b)
	clipped, total := 0, 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := src.RGBAAt(x, y)
			if c.A == 0 {
				continue
			}
			total++
			nx, ny := x+dx, y+dy
			if nx < 0 || nx >= w || ny < 0 || ny >= h {
				clipped++
				continue
			}
			out.SetRGBA(nx, ny, c)
		}
	}
	return out, clipped, total
}

// CorrectAnchors applies a frame's alignment transform to identity anchor
// positions (task 5.4: 锚点校正). Anchors follow the sprite's integer
// displacement, so corrected coordinates remain integer pixel positions.
func CorrectAnchors(anchors []AnchorPoint, t FrameTransform) []AnchorPoint {
	out := make([]AnchorPoint, len(anchors))
	for i, a := range anchors {
		out[i] = AnchorPoint{Name: a.Name, X: a.X + t.Dx, Y: a.Y + t.Dy}
	}
	return out
}
