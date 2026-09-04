// Phase-5 typed bindings over the motion capability (core/motion via the
// shared application service): 动作/方向集/帧序列 model, direction strategy
// (单方向默认 / 4/8 方向自动镜像 / 关闭镜像全生成), frame timing (帧时长/节奏).
// The oframe CLI calls the same service type, so GUI and CLI cannot drift
// (design D1/D12).
package main

import (
	"time"

	"github.com/oframe/character-workbench/core/motion"
)

// StrategyView mirrors motion.DirectionStrategy for the frontend.
type StrategyView struct {
	Count  int  `json:"count"`  // 1 | 4 | 8
	Mirror bool `json:"mirror"` // 自动镜像 on/off (off → 所有方向独立生成)
}

// AnchorPointView mirrors pipeline.AnchorPoint for the frontend.
type AnchorPointView struct {
	Name string `json:"name"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
}

// FrameView is one frame of a direction's frame sequence (帧时长/锚点).
type FrameView struct {
	Index      int               `json:"index"`
	AssetRef   string            `json:"assetRef,omitempty"`
	DurationMs int               `json:"durationMs"`
	Anchors    []AnchorPointView `json:"anchors,omitempty"`
}

// DirectionView is one direction set of a motion (独立帧序列 + 来源).
type DirectionView struct {
	Direction string      `json:"direction"`
	Origin    string      `json:"origin"` // generated | mirrored | replaced
	Source    string      `json:"source,omitempty"`
	Frames    []FrameView `json:"frames"`
}

// MotionView is the full motion payload for the Motion sub-page.
type MotionView struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	ActionPresetID    string          `json:"actionPresetId"`
	ActionDescription string          `json:"actionDescription"`
	FrameCount        int             `json:"frameCount"`
	ProviderID        string          `json:"providerId"`
	Model             string          `json:"model"`
	Loop              bool            `json:"loop"` // 循环播放 vs 一次性
	Strategy          StrategyView    `json:"strategy"`
	Directions        []DirectionView `json:"directions"`
	CreatedAt         string          `json:"createdAt"`
	UpdatedAt         string          `json:"updatedAt"`
}

// MotionCreate creates a motion with the chosen direction strategy
// (tasks 3.1/3.2: 动作由方向集构成; 单方向默认 down/正面; 3.3: 4/8 方向自动镜像).
func (a *App) MotionCreate(name string, count int, mirror bool) (*MotionView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	m, err := svc.MotionCreate(pkg.Root(), name, motion.DirectionStrategy{Count: count, Mirror: mirror})
	if err != nil {
		return nil, err
	}
	return motionToView(m), nil
}

// MotionList returns all motions of the session package (insertion order).
func (a *App) MotionList() ([]*MotionView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	list, err := svc.MotionList(pkg.Root())
	if err != nil {
		return nil, err
	}
	out := make([]*MotionView, 0, len(list))
	for _, m := range list {
		out = append(out, motionToView(m))
	}
	return out, nil
}

// MotionGet returns one motion of the session package.
func (a *App) MotionGet(id string) (*MotionView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	m, err := svc.MotionGet(pkg.Root(), id)
	if err != nil {
		return nil, err
	}
	return motionToView(m), nil
}

// MotionSetStrategy re-derives a motion's direction set for a new strategy
// (3.2/3.3: 方向策略调整; 关闭镜像 → 所有方向独立生成).
// MotionSetGenerationSettings updates the action prompt and target frame count.
func (a *App) MotionSetGenerationSettings(id, actionPresetID, actionDescription string, frameCount int) (*MotionView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	m, err := svc.MotionSetGenerationSettings(pkg.Root(), id, actionPresetID, actionDescription, frameCount)
	if err != nil {
		return nil, err
	}
	return motionToView(m), nil
}

// MotionDelete removes a motion from the session package.
func (a *App) MotionDelete(id string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return err
	}
	return svc.MotionDelete(pkg.Root(), id)
}

func (a *App) MotionSetStrategy(id string, count int, mirror bool) (*MotionView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	m, err := svc.MotionSetStrategy(pkg.Root(), id, motion.DirectionStrategy{Count: count, Mirror: mirror})
	if err != nil {
		return nil, err
	}
	return motionToView(m), nil
}

// MotionSetFrameDurations sets the per-frame display durations (rhythm) of a
// direction (task 3.6: 帧时长调整; 预览按新节奏回放).
func (a *App) MotionSetFrameDurations(id, direction string, durationsMs []int) (*MotionView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	m, err := svc.MotionSetFrameDurations(pkg.Root(), id, direction, durationsMs)
	if err != nil {
		return nil, err
	}
	return motionToView(m), nil
}

// MotionPlaybackTempo returns the playback rhythm of a direction's sequence
// (task 3.6: 调整后预览按新节奏回放).
func (a *App) MotionPlaybackTempo(id, direction string) ([]int, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	return svc.MotionPlaybackTempo(pkg.Root(), id, direction)
}

// MotionSetProviderSettings persists the motion-level provider/model choice
// (per-action provider configuration; empty values follow the global default).
func (a *App) MotionSetProviderSettings(id, providerID, model string) (*MotionView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	m, err := svc.MotionSetProviderSettings(pkg.Root(), id, providerID, model)
	if err != nil {
		return nil, err
	}
	return motionToView(m), nil
}

// MotionSetLoop persists whether the motion plays as a seamless loop or as a
// one-shot action (循环播放开关：待机/行走循环 vs 死亡/跳跃一次性).
func (a *App) MotionSetLoop(id string, loop bool) (*MotionView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	m, err := svc.MotionSetLoop(pkg.Root(), id, loop)
	if err != nil {
		return nil, err
	}
	return motionToView(m), nil
}

// MotionRename updates the motion card title (动作卡标题跟随所选动作预设名).
func (a *App) MotionRename(id, name string) (*MotionView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	m, err := svc.MotionRename(pkg.Root(), id, name)
	if err != nil {
		return nil, err
	}
	return motionToView(m), nil
}

// MotionClearDirection removes one direction's frame sequence (动作卡九宫格
// 右键"删除"该格动画): the cell reverts to "not generated" and can be lit and
// generated again. Idempotent.
func (a *App) MotionClearDirection(id, direction string) (*MotionView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	m, err := svc.MotionClearDirection(pkg.Root(), id, direction)
	if err != nil {
		return nil, err
	}
	return motionToView(m), nil
}

// MotionDirectionRawStrip returns a direction's raw filmstrip (九宫格右键
// "预览原图": 大模型返回、未切分的原始条带图, base64 PNG). Mirrored
// directions resolve their source direction's candidate.
func (a *App) MotionDirectionRawStrip(id, direction string) (string, error) {
	svc, err := a.service()
	if err != nil {
		return "", err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return "", err
	}
	return svc.DirectionRawStrip(pkg.Root(), id, direction)
}

// MotionFlipDirection mirrors one generated direction's animation horizontally
// (九宫格右键"水平翻转"). A mirror-derived pair flips together (the pair shares
// the source candidate's pixels); the flip is its own inverse. Accepted asset
// snapshots are synced so a later export stays coherent. Returns a human-
// readable confirmation message for the UI toast.
func (a *App) MotionFlipDirection(id, direction string) (string, error) {
	svc, err := a.service()
	if err != nil {
		return "", err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return "", err
	}
	return svc.MotionFlipDirection(pkg.Root(), id, direction)
}

func motionToView(m *motion.Motion) *MotionView {
	view := &MotionView{
		ID:                m.ID,
		Name:              m.Name,
		ActionPresetID:    m.ActionPresetID,
		ActionDescription: m.ActionDescription,
		FrameCount:        m.TargetFrameCount,
		ProviderID:        m.ProviderID,
		Model:             m.Model,
		Loop:              m.Loop,
		Strategy:          StrategyView{Count: m.Strategy.Count, Mirror: m.Strategy.Mirror},
		Directions:        []DirectionView{},
		CreatedAt:         m.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         m.UpdatedAt.Format(time.RFC3339),
	}
	for _, d := range m.Directions {
		dv := DirectionView{Direction: d.Direction, Origin: d.Origin, Source: d.Source, Frames: []FrameView{}}
		for _, f := range d.Sequence.Frames {
			fv := FrameView{Index: f.Index, AssetRef: f.AssetRef, DurationMs: f.DurationMs, Anchors: []AnchorPointView{}}
			for _, an := range f.Anchors {
				fv.Anchors = append(fv.Anchors, AnchorPointView{Name: an.Name, X: an.X, Y: an.Y})
			}
			dv.Frames = append(dv.Frames, fv)
		}
		view.Directions = append(view.Directions, dv)
	}
	return view
}
