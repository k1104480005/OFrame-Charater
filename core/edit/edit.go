package edit

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

// Instruction is an append-only, replayable non-destructive edit.
type Instruction struct {
	Kind       string     `json:"kind"`
	FrameIndex int        `json:"frameIndex,omitempty"`
	X          int        `json:"x,omitempty"`
	Y          int        `json:"y,omitempty"`
	Width      int        `json:"width,omitempty"`
	Height     int        `json:"height,omitempty"`
	Color      color.RGBA `json:"color,omitempty"`
	DurationMs int        `json:"durationMs,omitempty"`
	DeltaX     int        `json:"deltaX,omitempty"`
	DeltaY     int        `json:"deltaY,omitempty"`
	Order      []int      `json:"order,omitempty"`
	FramePNG   []byte     `json:"framePng,omitempty"`
	FrameMeta  FrameMeta  `json:"frameMeta,omitempty"`
}

type FrameMeta struct {
	DurationMs int `json:"durationMs"`
	AnchorX    int `json:"anchorX"`
	AnchorY    int `json:"anchorY"`
}

type Frame struct {
	Image      *image.RGBA `json:"-"`
	DurationMs int         `json:"durationMs"`
	AnchorX    int         `json:"anchorX"`
	AnchorY    int         `json:"anchorY"`
}

type Sequence struct {
	Frames       []Frame       `json:"frames"`
	Instructions []Instruction `json:"instructions"`
}

func CloneSequence(in Sequence) Sequence {
	out := Sequence{Instructions: append([]Instruction(nil), in.Instructions...)}
	out.Frames = make([]Frame, len(in.Frames))
	for i, f := range in.Frames {
		out.Frames[i] = f
		if f.Image != nil {
			out.Frames[i].Image = cloneRGBA(f.Image)
		}
	}
	return out
}

func (s *Sequence) Apply(inst Instruction) error {
	switch inst.Kind {
	case "reorder":
		return s.applyReorder(inst.Order, true)
	case "insert":
		return s.applyInsert(inst.FrameIndex, inst.FramePNG, inst.FrameMeta, true)
	case "delete":
		return s.applyDelete(inst.FrameIndex, true)
	}
	if inst.FrameIndex < 0 || inst.FrameIndex >= len(s.Frames) {
		return fmt.Errorf("edit: frame index %d out of range", inst.FrameIndex)
	}
	f := &s.Frames[inst.FrameIndex]
	switch inst.Kind {
	case "pixel":
		if f.Image == nil || !image.Pt(inst.X, inst.Y).In(f.Image.Bounds()) {
			return fmt.Errorf("edit: pixel outside frame")
		}
		f.Image.SetRGBA(inst.X, inst.Y, inst.Color)
	case "erase":
		if f.Image == nil || !image.Pt(inst.X, inst.Y).In(f.Image.Bounds()) {
			return fmt.Errorf("edit: pixel outside frame")
		}
		f.Image.SetRGBA(inst.X, inst.Y, color.RGBA{})
	case "cleanup":
		if f.Image == nil {
			return fmt.Errorf("edit: cleanup requires an image")
		}
		cleanupEdges(f.Image)
	case "crop":
		if f.Image == nil || inst.Width <= 0 || inst.Height <= 0 {
			return fmt.Errorf("edit: invalid crop")
		}
		r := image.Rect(inst.X, inst.Y, inst.X+inst.Width, inst.Y+inst.Height).Intersect(f.Image.Bounds())
		if r.Empty() || r.Dx() != inst.Width || r.Dy() != inst.Height {
			return fmt.Errorf("edit: crop outside frame")
		}
		f.Image = cropRGBA(f.Image, r)
	case "duration":
		if inst.DurationMs <= 0 {
			return fmt.Errorf("edit: duration must be positive")
		}
		f.DurationMs = inst.DurationMs
	case "anchor-delta":
		f.AnchorX += inst.DeltaX
		f.AnchorY += inst.DeltaY
	default:
		return fmt.Errorf("edit: unknown instruction %q", inst.Kind)
	}
	s.Instructions = append(s.Instructions, inst)
	return nil
}

func (s *Sequence) ApplyBatch(inst Instruction) error {
	for i := range s.Frames {
		copy := inst
		copy.FrameIndex = i
		if err := s.Apply(copy); err != nil {
			return err
		}
	}
	return nil
}

func (s *Sequence) Reorder(order []int) error {
	if err := s.applyReorder(order, false); err != nil {
		return err
	}
	copyOrder := append([]int(nil), order...)
	s.Instructions = append(s.Instructions, Instruction{Kind: "reorder", Order: copyOrder})
	return nil
}

func (s *Sequence) applyReorder(order []int, record bool) error {
	if len(order) != len(s.Frames) {
		return fmt.Errorf("edit: order length mismatch")
	}
	seen := make([]bool, len(order))
	frames := make([]Frame, len(order))
	for i, src := range order {
		if src < 0 || src >= len(s.Frames) || seen[src] {
			return fmt.Errorf("edit: invalid frame order")
		}
		seen[src] = true
		frames[i] = s.Frames[src]
		if frames[i].Image != nil {
			frames[i].Image = cloneRGBA(frames[i].Image)
		}
	}
	s.Frames = frames
	if record {
		s.Instructions = append(s.Instructions, Instruction{Kind: "reorder", Order: append([]int(nil), order...)})
	}
	return nil
}

func (s *Sequence) Insert(index int, frame Frame) error {
	data, err := encodePNG(frame.Image)
	if err != nil {
		return err
	}
	return s.applyInsert(index, data, FrameMeta{DurationMs: frame.DurationMs, AnchorX: frame.AnchorX, AnchorY: frame.AnchorY}, true)
}

func (s *Sequence) applyInsert(index int, data []byte, meta FrameMeta, record bool) error {
	if index < 0 || index > len(s.Frames) {
		return fmt.Errorf("edit: insert index out of range")
	}
	frame, err := decodePNG(data)
	if err != nil {
		return err
	}
	s.Frames = append(s.Frames, Frame{})
	copy(s.Frames[index+1:], s.Frames[index:])
	s.Frames[index] = Frame{Image: frame, DurationMs: meta.DurationMs, AnchorX: meta.AnchorX, AnchorY: meta.AnchorY}
	if record {
		s.Instructions = append(s.Instructions, Instruction{Kind: "insert", FrameIndex: index, FramePNG: append([]byte(nil), data...), FrameMeta: meta})
	}
	return nil
}

func (s *Sequence) Delete(index int) error {
	if err := s.applyDelete(index, false); err != nil {
		return err
	}
	s.Instructions = append(s.Instructions, Instruction{Kind: "delete", FrameIndex: index})
	return nil
}

func (s *Sequence) applyDelete(index int, record bool) error {
	if index < 0 || index >= len(s.Frames) {
		return fmt.Errorf("edit: delete index out of range")
	}
	s.Frames = append(s.Frames[:index], s.Frames[index+1:]...)
	if record {
		s.Instructions = append(s.Instructions, Instruction{Kind: "delete", FrameIndex: index})
	}
	return nil
}

func (s *Sequence) ApplyAnchorDelta(deltaX, deltaY int, all bool, frameIndex int) error {
	if all {
		return s.ApplyBatch(Instruction{Kind: "anchor-delta", DeltaX: deltaX, DeltaY: deltaY})
	}
	return s.Apply(Instruction{Kind: "anchor-delta", FrameIndex: frameIndex, DeltaX: deltaX, DeltaY: deltaY})
}

func Replay(base Sequence, instructions []Instruction) (Sequence, error) {
	out := CloneSequence(base)
	out.Instructions = nil
	for _, inst := range instructions {
		if err := out.Apply(inst); err != nil {
			return Sequence{}, err
		}
	}
	return out, nil
}

func encodePNG(src *image.RGBA) ([]byte, error) {
	if src == nil {
		return nil, fmt.Errorf("edit: inserted frame image is required")
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		return nil, fmt.Errorf("edit: encode inserted frame: %w", err)
	}
	return buf.Bytes(), nil
}

func decodePNG(data []byte) (*image.RGBA, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("edit: decode inserted frame: %w", err)
	}
	return toRGBA(img), nil
}

func cloneRGBA(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}

func toRGBA(src image.Image) *image.RGBA {
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			dst.Set(x, y, src.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return dst
}

func cropRGBA(src *image.RGBA, r image.Rectangle) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			dst.SetRGBA(x, y, src.RGBAAt(r.Min.X+x, r.Min.Y+y))
		}
	}
	return dst
}

func cleanupEdges(img *image.RGBA) {
	bounds := img.Bounds()
	seen := make(map[image.Point]bool)
	queue := make([]image.Point, 0)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		queue = append(queue, image.Pt(bounds.Min.X, y), image.Pt(bounds.Max.X-1, y))
	}
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		queue = append(queue, image.Pt(x, bounds.Min.Y), image.Pt(x, bounds.Max.Y-1))
	}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if !p.In(bounds) || seen[p] || img.RGBAAt(p.X, p.Y).A >= 128 {
			continue
		}
		seen[p] = true
		img.SetRGBA(p.X, p.Y, color.RGBA{})
		queue = append(queue, image.Pt(p.X-1, p.Y), image.Pt(p.X+1, p.Y), image.Pt(p.X, p.Y-1), image.Pt(p.X, p.Y+1))
	}
}
