package motion

import (
	"fmt"
	"strings"
	"time"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/pipeline"
)

// Direction labels (spec vocabulary). "down" is the front-facing direction
// (正面, compass south) and the single-direction default (task 3.2).
const (
	DirectionRight     = "right"
	DirectionUp        = "up"
	DirectionDown      = "down" // 正面 (front-facing / compass south)
	DirectionLeft      = "left"
	DirectionUpRight   = "up-right"
	DirectionUpLeft    = "up-left"
	DirectionDownLeft  = "down-left"
	DirectionDownRight = "down-right"
)

// DefaultDirection is the single-direction default: down (south/正面).
const DefaultDirection = DirectionDown

// AllDirections is the full 8-direction vocabulary. Order matters for
// deterministic output: clockwise compass order starting at east/right, with
// the mirror pairs adjacent (right/left, up-right/up-left, down-left/down-right)
// and the self-symmetric cardinals (up/down) in between. Used when mirroring
// is disabled and every direction must be generated independently.
var AllDirections = []string{
	DirectionRight, DirectionUpRight, DirectionUp, DirectionUpLeft,
	DirectionLeft, DirectionDownLeft, DirectionDown, DirectionDownRight,
}

// DirectionStrategy declares how many directions a motion has (1 | 4 | 8) and
// whether automatic horizontal mirroring derives the remaining directions
// (task 3.3). When Mirror is false, ALL directions are generated
// independently (关闭镜像时所有方向独立生成).
type DirectionStrategy struct {
	Count  int  `json:"count"`  // 1 | 4 | 8
	Mirror bool `json:"mirror"` // 自动镜像 on/off
}

// ValidateStrategy checks a direction strategy.
func ValidateStrategy(s DirectionStrategy) error {
	if s.Count != 1 && s.Count != 4 && s.Count != 8 {
		return fmt.Errorf("motion: direction count must be 1, 4 or 8, got %d", s.Count)
	}
	return nil
}

// Direction origins: how a direction's frame sequence came to be.
const (
	OriginGenerated = "generated" // 基本方向: provider 生成
	OriginMirrored  = "mirrored"  // 镜像方向: 水平镜像派生 (independent sequence)
	OriginReplaced  = "replaced"  // 手动替换方向: 验收时以额外调用替换 (task 3.5)
)

// BasicDirections returns the directions that require generation calls for a
// strategy (自动镜像口径, task 3.3):
//
//   - single:            [down]
//   - 4 + mirror:        [right, up, down]                    (left 镜像派生)
//   - 8 + mirror:        [right, up, down, up-right, down-left] (left / up-left /
//     down-right 镜像派生 — 复核报告语义: down-right 单向派生自 down-left, 因此
//     down-left 是生成方向)
//   - mirror off:        every direction is generated independently.
func BasicDirections(s DirectionStrategy) []string {
	switch s.Count {
	case 1:
		return []string{DefaultDirection}
	case 4:
		if !s.Mirror {
			return []string{DirectionRight, DirectionUp, DirectionDown, DirectionLeft}
		}
		return []string{DirectionRight, DirectionUp, DirectionDown}
	case 8:
		if !s.Mirror {
			return append([]string(nil), AllDirections...)
		}
		return []string{DirectionRight, DirectionUp, DirectionDown, DirectionUpRight, DirectionDownLeft}
	default:
		return nil
	}
}

// MirroredDirections returns the directions derived by horizontal mirroring
// for a strategy (empty when mirroring is disabled). Each returned direction
// has a defined mirror source (MirrorSource) in the basic set.
func MirroredDirections(s DirectionStrategy) []string {
	if !s.Mirror {
		return nil
	}
	switch s.Count {
	case 4:
		return []string{DirectionLeft}
	case 8:
		return []string{DirectionLeft, DirectionUpLeft, DirectionDownRight}
	default:
		return nil
	}
}

// Frame is one frame of a frame sequence with its rhythm metadata (帧时长,
// task 3.6). AssetRef references the frame's asset/candidate content
// (deterministic reference; the pixel content lives in the pipeline
// candidate / asset store). Anchors are the frame-local anchor points (锚点).
type Frame struct {
	Index      int                    `json:"index"`
	AssetRef   string                 `json:"assetRef,omitempty"`
	DurationMs int                    `json:"durationMs"`
	Anchors    []pipeline.AnchorPoint `json:"anchors,omitempty"`
}

// DefaultFrameDurationMs is the default per-frame display duration (rhythm)
// when a frame has no explicit duration. The concrete default is a product
// detail (design Open Questions); 100ms is the sensible initial value.
const DefaultFrameDurationMs = 100

// FrameSequence is the independent frame sequence of one direction (每个方向
// 一份独立帧序列, task 3.1). Mirrored directions own their own sequence — never
// a shared reference to the source direction's frames (task 3.4).
type FrameSequence struct {
	Direction string  `json:"direction"`
	Frames    []Frame `json:"frames"`
}

// DirectionSet is one direction of a motion: the direction label, how its
// sequence came to be (generated / mirrored / replaced), the mirror source for
// mirrored directions, and the independent frame sequence.
type DirectionSet struct {
	Direction string        `json:"direction"`
	Origin    string        `json:"origin"`
	Source    string        `json:"source,omitempty"` // mirror source when Origin == mirrored
	Sequence  FrameSequence `json:"sequence"`
}

// Motion is one action of a character (动作): it consists of direction sets,
// one independent frame sequence per direction (task 3.1), organized by the
// chosen direction strategy (task 3.2/3.3).
type Motion struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Strategy   DirectionStrategy `json:"strategy"`
	Directions []DirectionSet    `json:"directions"`
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

// NewMotion creates a motion with the strategy's directions initialized as
// empty frame sequences (task 3.1: 动作由方向集构成、每方向一份独立帧序列; task
// 3.2: 单方向默认 down/正面).
func NewMotion(id, name string, s DirectionStrategy) (*Motion, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("motion: id and name are required")
	}
	if err := ValidateStrategy(s); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	dirs := make([]DirectionSet, 0, len(BasicDirections(s))+len(MirroredDirections(s)))
	for _, b := range BasicDirections(s) {
		dirs = append(dirs, newDirection(b, OriginGenerated, ""))
	}
	for _, m := range MirroredDirections(s) {
		dirs = append(dirs, newDirection(m, OriginMirrored, MirrorSource(m)))
	}
	return &Motion{ID: id, Name: name, Strategy: s, Directions: dirs, CreatedAt: now, UpdatedAt: now}, nil
}

func newDirection(dir, origin, source string) DirectionSet {
	return DirectionSet{
		Direction: dir,
		Origin:    origin,
		Source:    source,
		Sequence:  FrameSequence{Direction: dir, Frames: []Frame{}},
	}
}

// Direction returns the direction set by label, or nil when the motion has no
// such direction.
func (m *Motion) Direction(dir string) *DirectionSet {
	for i := range m.Directions {
		if m.Directions[i].Direction == dir {
			return &m.Directions[i]
		}
	}
	return nil
}

// FrameCount returns the number of frames in a direction's sequence (0 when
// the direction is unknown or empty).
func (m *Motion) FrameCount(dir string) int {
	d := m.Direction(dir)
	if d == nil {
		return 0
	}
	return len(d.Sequence.Frames)
}

// SetStrategy re-derives the direction set for a new strategy (task 3.3/3.2:
// 方向策略可调整). Frame content of directions that still exist is preserved
// (each direction keeps its independent sequence); new directions start
// empty; removed directions drop.
func (m *Motion) SetStrategy(s DirectionStrategy) error {
	if err := ValidateStrategy(s); err != nil {
		return err
	}
	if s == m.Strategy {
		return nil
	}
	existing := make(map[string]DirectionSet, len(m.Directions))
	for _, d := range m.Directions {
		existing[d.Direction] = d
	}
	dirs := make([]DirectionSet, 0, len(BasicDirections(s))+len(MirroredDirections(s)))
	for _, b := range BasicDirections(s) {
		if prev, ok := existing[b]; ok {
			prev.Origin, prev.Source = OriginGenerated, ""
			dirs = append(dirs, prev)
			continue
		}
		dirs = append(dirs, newDirection(b, OriginGenerated, ""))
	}
	for _, mm := range MirroredDirections(s) {
		src := MirrorSource(mm)
		if prev, ok := existing[mm]; ok {
			prev.Origin, prev.Source = OriginMirrored, src
			dirs = append(dirs, prev)
			continue
		}
		dirs = append(dirs, newDirection(mm, OriginMirrored, src))
	}
	m.Directions = dirs
	m.Strategy = s
	m.UpdatedAt = time.Now().UTC()
	return nil
}

// SetDirectionSequence writes the frame sequence of a direction (task 3.3:
// 生成方向帧序列; task 3.4: 镜像方向独立帧序列; task 3.5: 替换方向帧序列). For
// OriginMirrored the mirror source is derived from the mapping; the caller
// supplies the mirrored content (mirror.go). origin empty keeps the current
// origin.
func (m *Motion) SetDirectionSequence(dir string, seq FrameSequence, origin string) error {
	d := m.Direction(dir)
	if d == nil {
		return fmt.Errorf("motion: unknown direction %q", dir)
	}
	if origin == "" {
		origin = d.Origin
	}
	switch origin {
	case OriginGenerated, OriginMirrored, OriginReplaced:
	default:
		return fmt.Errorf("motion: invalid origin %q", origin)
	}
	if seq.Direction == "" {
		seq.Direction = dir
	}
	if seq.Direction != dir {
		return fmt.Errorf("motion: sequence direction %q does not match direction %q", seq.Direction, dir)
	}
	d.Sequence = seq
	d.Origin = origin
	if origin == OriginMirrored {
		d.Source = MirrorSource(dir)
	} else {
		d.Source = ""
	}
	m.UpdatedAt = time.Now().UTC()
	return nil
}

// ReplaceDirection replaces a direction's frame sequence with manually
// generated content during acceptance (task 3.5: 镜像方向手动替换, 验收时以额外
// 调用替换). The direction's origin becomes "replaced"; the direction set is
// updated with the replacement frames. The replacement is counted in the
// generation confirmation's expected call count by the caller (service layer).
func (m *Motion) ReplaceDirection(dir string, seq FrameSequence) error {
	d := m.Direction(dir)
	if d == nil {
		return fmt.Errorf("motion: unknown direction %q", dir)
	}
	if seq.Direction == "" {
		seq.Direction = dir
	}
	if seq.Direction != dir {
		return fmt.Errorf("motion: replacement sequence direction %q does not match direction %q", seq.Direction, dir)
	}
	d.Sequence = seq
	d.Origin = OriginReplaced
	d.Source = ""
	m.UpdatedAt = time.Now().UTC()
	return nil
}

// SetFrameDurations sets the per-frame display durations (rhythm) of a
// direction's sequence (task 3.6: 帧时长调整). The number of durations must
// match the sequence length and every duration must be positive.
func (m *Motion) SetFrameDurations(dir string, durationsMs []int) error {
	d := m.Direction(dir)
	if d == nil {
		return fmt.Errorf("motion: unknown direction %q", dir)
	}
	if len(durationsMs) != len(d.Sequence.Frames) {
		return fmt.Errorf("motion: %d durations for %d frames of direction %q", len(durationsMs), len(d.Sequence.Frames), dir)
	}
	for i, ms := range durationsMs {
		if ms <= 0 {
			return fmt.Errorf("motion: frame duration must be positive, got %d at index %d", ms, i)
		}
		d.Sequence.Frames[i].DurationMs = ms
	}
	m.UpdatedAt = time.Now().UTC()
	return nil
}

// PlaybackTempo returns the per-frame display durations of a direction's
// sequence in order — the playback rhythm the preview must follow (task 3.6:
// 调整后预览按新节奏回放). Unset (0) durations fall back to
// DefaultFrameDurationMs.
func (m *Motion) PlaybackTempo(dir string) ([]int, error) {
	d := m.Direction(dir)
	if d == nil {
		return nil, fmt.Errorf("motion: unknown direction %q", dir)
	}
	out := make([]int, len(d.Sequence.Frames))
	for i, f := range d.Sequence.Frames {
		if f.DurationMs <= 0 {
			out[i] = DefaultFrameDurationMs
		} else {
			out[i] = f.DurationMs
		}
	}
	return out, nil
}

// ValidateFrameSequence checks a direction's sequence invariants against the
// logical canvas: the sequence has a defined count and a defined order
// (motion spec: "Frame sequence specification"; identity spec: 规格被后续动作/
// 帧序列校验引用). Frame pixel-size conformance is enforced by the filmstrip
// pipeline (NormalizeFrameList + slicing), which produces the frames.
func (m *Motion) ValidateFrameSequence(dir string, canvas identity.CanvasSpec) error {
	if err := canvas.Validate(); err != nil {
		return err
	}
	d := m.Direction(dir)
	if d == nil {
		return fmt.Errorf("motion: unknown direction %q", dir)
	}
	if len(d.Sequence.Frames) == 0 {
		return fmt.Errorf("motion: direction %q has no frames", dir)
	}
	for i, f := range d.Sequence.Frames {
		if f.Index != i {
			return fmt.Errorf("motion: direction %q frame %d has index %d (order broken)", dir, i, f.Index)
		}
	}
	return nil
}
