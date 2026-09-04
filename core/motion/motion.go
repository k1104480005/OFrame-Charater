package motion

import (
	"encoding/json"
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
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	ActionPresetID    string            `json:"actionPresetId,omitempty"`
	ActionDescription string            `json:"actionDescription,omitempty"`
	TargetFrameCount  int               `json:"targetFrameCount,omitempty"`
	ProviderID        string            `json:"providerId,omitempty"` // 动作级 Provider；空 = 跟随全局默认
	Model             string            `json:"model,omitempty"`      // 动作级模型；空 = Provider 默认
	Loop              bool              `json:"loop"`                 // 循环播放（待机/行走等）vs 一次性（死亡/跳跃等）
	Strategy          DirectionStrategy `json:"strategy"`
	Directions        []DirectionSet    `json:"directions"`
	CreatedAt         time.Time         `json:"createdAt"`
	UpdatedAt         time.Time         `json:"updatedAt"`
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
	return &Motion{
		ID: id, Name: name,
		ActionPresetID: "walk", TargetFrameCount: 4,
		Loop:     true, // 新动作默认循环播放（可切一次性）
		Strategy: s, Directions: dirs, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// UnmarshalJSON makes Loop default to true when the stored motion predates the
// loop field (older motions.json has no "loop" key and previews were always
// looping). Explicit false in storage is respected.
func (m *Motion) UnmarshalJSON(data []byte) error {
	type plain Motion
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err == nil {
		if _, ok := probe["loop"]; !ok {
			p.Loop = true // 旧数据：无 loop 键 → 默认循环
		}
	}
	*m = Motion(p)
	return nil
}

// SetName renames the motion (动作卡标题；预设切换时前端用它同步预设名).
func (m *Motion) SetName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("motion: name is required")
	}
	m.Name = strings.TrimSpace(name)
	m.UpdatedAt = time.Now().UTC()
	return nil
}

// SetLoop updates the looping playback flag (循环播放 vs 一次性动作).
func (m *Motion) SetLoop(loop bool) {
	m.Loop = loop
	m.UpdatedAt = time.Now().UTC()
}

// SetGenerationSettings stores the action semantics used to build the filmstrip prompt.
// Empty legacy fields are normalized to the safe defaults used by new motions.
// 生成即定稿：动作卡一旦有任一方向生成了动画，动作预设与动作描述（提示词
// 语义）即锁定 —— 它们决定了已有动画的动作语义，事后修改会让已生成动画与
// 提示词脱节。锁只拦真正的语义变化：预设与描述都未变化时照常放行（帧数仍
// 可调；打开生成确认弹窗前的表单落盘也是无语义变化的重复写，不能被拦）。
func (m *Motion) SetGenerationSettings(actionPresetID, actionDescription string, frameCount int) error {
	if strings.TrimSpace(actionPresetID) == "" {
		actionPresetID = "walk"
	}
	actionDescription = strings.TrimSpace(actionDescription)
	currentPreset := m.ActionPresetID
	if currentPreset == "" {
		currentPreset = "walk" // 旧数据缺省按 walk 口径比较
	}
	// 描述只在自定义预设下是可见语义（预设提示词模式下描述恒为空），比较时
	// 同口径：非自定义预设忽略双方的历史描述漂移。
	semanticChange := actionPresetID != currentPreset ||
		(actionPresetID == "custom" && actionDescription != m.ActionDescription)
	if semanticChange {
		for i := range m.Directions {
			if len(m.Directions[i].Sequence.Frames) > 0 {
				return fmt.Errorf("motion: 动作 %q 已有生成动画 —— 动作预设与动作描述已锁定；如需修改请先删除该动作卡的全部已生成动画", m.Name)
			}
		}
	}
	if actionPresetID != "custom" {
		if _, err := pipeline.ActionPresetByID(actionPresetID); err != nil {
			return err
		}
	}
	if frameCount == 0 {
		frameCount = 4
	}
	if frameCount < 1 || frameCount > 10 {
		return fmt.Errorf("motion: frame count must be between 1 and 10, got %d", frameCount)
	}
	if actionPresetID == "custom" && actionDescription == "" {
		return fmt.Errorf("motion: custom action description is required")
	}
	m.ActionPresetID = actionPresetID
	m.ActionDescription = actionDescription
	m.TargetFrameCount = frameCount
	m.UpdatedAt = time.Now().UTC()
	return nil
}

// SetProviderSettings stores the motion-level provider and model choice.
// Empty values mean "follow the global default".
func (m *Motion) SetProviderSettings(providerID, model string) {
	m.ProviderID = strings.TrimSpace(providerID)
	m.Model = strings.TrimSpace(model)
	m.UpdatedAt = time.Now().UTC()
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

// ClearDirection removes a direction's frame sequence (动作卡九宫格右键"删除"
// 该格动画): the direction slot itself is kept (the 8-direction structure is
// stable), the cell reverts to "not generated" — origin/source cleared so it
// can be lit and generated again. Idempotent for an already-empty direction.
func (m *Motion) ClearDirection(dir string) error {
	d := m.Direction(dir)
	if d == nil {
		return fmt.Errorf("motion: unknown direction %q", dir)
	}
	d.Sequence = FrameSequence{}
	d.Origin = ""
	d.Source = ""
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
