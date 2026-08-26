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
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Strategy   StrategyView    `json:"strategy"`
	Directions []DirectionView `json:"directions"`
	CreatedAt  string          `json:"createdAt"`
	UpdatedAt  string          `json:"updatedAt"`
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

func motionToView(m *motion.Motion) *MotionView {
	view := &MotionView{
		ID:         m.ID,
		Name:       m.Name,
		Strategy:   StrategyView{Count: m.Strategy.Count, Mirror: m.Strategy.Mirror},
		Directions: []DirectionView{},
		CreatedAt:  m.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  m.UpdatedAt.Format(time.RFC3339),
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
