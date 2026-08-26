package pipeline

import (
	"fmt"

	"github.com/oframe/character-workbench/core/identity"
)

// FrameSpec is one normalized frame's placement inside the filmstrip: its
// index in sequence order and its integer top-left position and size.
type FrameSpec struct {
	Index  int
	X, Y   int
	Width  int
	Height int
}

// FrameList is the normalized, fixed-length frame list (task 5.1 帧清单规格化):
// a motion's frame sequence reduced to a fixed count, fixed size, and fixed
// order that all conform to the logical canvas specification. The filmstrip
// is exactly FrameCount × UnitWidth wide and UnitHeight tall.
type FrameList struct {
	Canvas      identity.CanvasSpec
	FrameCount  int
	Frames      []FrameSpec
	StripWidth  int
	StripHeight int
}

// NormalizeFrameList normalizes a motion's frame sequence into a fixed-length,
// fixed-order frame list based on the logical canvas (filmstrip-pipeline spec:
// "Frame list normalization"). Frames are laid left to right in sequence
// order, each exactly UnitWidth×UnitHeight, starting at (0,0).
func NormalizeFrameList(canvas identity.CanvasSpec, frameCount int) (FrameList, error) {
	if err := canvas.Validate(); err != nil {
		return FrameList{}, err
	}
	if frameCount <= 0 {
		return FrameList{}, fmt.Errorf("pipeline: frame count must be positive, got %d", frameCount)
	}
	w, h := canvas.UnitWidth, canvas.UnitHeight
	frames := make([]FrameSpec, frameCount)
	for i := 0; i < frameCount; i++ {
		frames[i] = FrameSpec{Index: i, X: i * w, Y: 0, Width: w, Height: h}
	}
	return FrameList{
		Canvas:      canvas,
		FrameCount:  frameCount,
		Frames:      frames,
		StripWidth:  frameCount * w,
		StripHeight: h,
	}, nil
}

// FrameAt returns the normalized placement of frame index i.
func (fl FrameList) FrameAt(i int) (FrameSpec, error) {
	if i < 0 || i >= len(fl.Frames) {
		return FrameSpec{}, fmt.Errorf("pipeline: frame index %d out of range 0..%d", i, len(fl.Frames)-1)
	}
	return fl.Frames[i], nil
}

// ValidateFrames reports whether a produced frame count conforms to the
// normalized list. This is the conformance check referenced by slicing
// (task 5.3: 切片与规格不符时任务失败).
func (fl FrameList) ValidateFrames(count int) error {
	if count != fl.FrameCount {
		return fmt.Errorf("pipeline: frame count %d does not match normalized list of %d", count, fl.FrameCount)
	}
	return nil
}
