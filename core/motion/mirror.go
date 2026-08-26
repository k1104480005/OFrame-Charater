package motion

import (
	"fmt"
	"image"

	"github.com/oframe/character-workbench/core/pipeline"
)

// Horizontal mirror geometry (水平镜像), the review report's semantic points
// made explicit:
//
//   - ONLY horizontal (left-right) mirroring derives directions.
//   - down (正面/south) is SELF-SYMMETRIC: mirror(down) = down. up is likewise
//     self-symmetric: mirror(up) = up. Neither is a mirror source for another
//     direction (仅水平镜像时 down 应自对称).
//   - The mirror pairs are ONE-WAY source → derived:
//       right → left, up-right → up-left, down-left → down-right
//     (down-right 单向派生自 down-left; left / up-left / down-right are
//     themselves derived and are never mirror sources).
//   - A mirror-derived direction owns an INDEPENDENT frame sequence, and its
//     anchors are converted by the horizontal mirror rule: X' = width-1-X,
//     Y' = Y, integer pixel exact, no interpolation (task 3.4).

// mirrorPairs is the one-way source → derived mapping (review semantics).
var mirrorPairs = map[string]string{
	DirectionRight:    DirectionLeft,
	DirectionUpRight:  DirectionUpLeft,
	DirectionDownLeft: DirectionDownRight,
}

// mirrorSources is the reverse lookup derived → source.
var mirrorSources = map[string]string{
	DirectionLeft:      DirectionRight,
	DirectionUpLeft:    DirectionUpRight,
	DirectionDownRight: DirectionDownLeft,
}

// IsSelfSymmetric reports whether a direction is invariant under horizontal
// mirroring (up and down).
func IsSelfSymmetric(dir string) bool {
	return dir == DirectionUp || dir == DirectionDown
}

// MirroredFrom returns the horizontally-mirrored derived direction of a
// source, or "" when the source has no mirror derivative (self-symmetric
// directions and derived directions are not sources — one-way mapping).
func MirroredFrom(source string) string { return mirrorPairs[source] }

// MirrorSource returns the source a direction is mirrored from, or "" when the
// direction is not mirror-derived.
func MirrorSource(derived string) string { return mirrorSources[derived] }

// HorizontalMirror flips an image left-right with integer pixel exactness
// (no interpolation) — the deterministic mirror operation for frame content
// (task 3.4: 镜像方向独立帧序列).
func HorizontalMirror(src *image.RGBA) *image.RGBA {
	b := src.Bounds()
	w := b.Dx()
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.SetRGBA(b.Min.X+w-1-x, y, src.RGBAAt(x, y))
		}
	}
	return out
}

// MirrorAnchors converts anchors by the horizontal mirror rule for a canvas
// unit width: X' = width-1-X, Y' = Y (task 3.4: 锚点坐标按水平镜像规则换算).
func MirrorAnchors(anchors []pipeline.AnchorPoint, width int) []pipeline.AnchorPoint {
	out := make([]pipeline.AnchorPoint, len(anchors))
	for i, a := range anchors {
		out[i] = pipeline.AnchorPoint{Name: a.Name, X: width - 1 - a.X, Y: a.Y}
	}
	return out
}

// MirrorSequence builds the INDEPENDENT frame sequence of a mirrored direction
// from the source direction's sequence (task 3.4): same frame count, anchors
// converted by the horizontal mirror rule, display rhythm copied (mirroring
// does not change timing), and a deterministic asset reference marking the
// content as the horizontal mirror of the source frame. The caller stores the
// resulting sequence on the mirrored direction via SetDirectionSequence with
// OriginMirrored (the sequence Direction field is set there).
func MirrorSequence(src FrameSequence, width int) (FrameSequence, error) {
	if width <= 0 {
		return FrameSequence{}, fmt.Errorf("motion: mirror needs a positive canvas width, got %d", width)
	}
	seq := FrameSequence{Frames: make([]Frame, len(src.Frames))}
	for i, sf := range src.Frames {
		seq.Frames[i] = Frame{
			Index:      i,
			AssetRef:   fmt.Sprintf("mirror:%s:%d", src.Direction, i),
			DurationMs: sf.DurationMs,
			Anchors:    MirrorAnchors(sf.Anchors, width),
		}
	}
	return seq, nil
}
