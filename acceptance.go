// Phase-6 typed bindings over the quality acceptance gate, candidate history,
// versioning (acceptance/operation log/rollback), the optional AI consistency
// score, and the PixelPerfect direction preview. All methods delegate to the
// shared application service (core/service) — the same code the oframe CLI
// calls, so GUI and CLI cannot drift (design D1/D12).
package main

import (
	"github.com/oframe/character-workbench/core/pipeline"
)

// CandidateHistoryView mirrors service.CandidateHistoryView.
type CandidateHistoryView struct {
	ID             string  `json:"id"`
	MotionID       string  `json:"motionId,omitempty"`
	Direction      string  `json:"direction,omitempty"`
	CreatedAt      string  `json:"createdAt"`
	Status         string  `json:"status"`
	Overall        float64 `json:"overall"`
	AcceptanceNote string  `json:"acceptanceNote,omitempty"`
	RegenerationOf string  `json:"regenerationOf,omitempty"`
}

// AcceptanceDecisionView mirrors service.AcceptanceDecisionView.
type AcceptanceDecisionView struct {
	CandidateID string  `json:"candidateId"`
	Decision    string  `json:"decision"`
	Note        string  `json:"note"`
	Threshold   float64 `json:"threshold"`
}

// OperationLogEntryView mirrors service.OperationLogEntryView.
type OperationLogEntryView struct {
	Seq     int    `json:"seq"`
	At      string `json:"at"`
	Action  string `json:"action"`
	Payload any    `json:"payload,omitempty"`
}

// AcceptedAssetView mirrors service.AcceptedAssetView.
type AcceptedAssetView struct {
	MotionID    string `json:"motionId,omitempty"`
	Direction   string `json:"direction"`
	CandidateID string `json:"candidateId"`
	AcceptedAt  string `json:"acceptedAt"`
	FrameCount  int    `json:"frameCount"`
}

// ConsistencyScoreView mirrors service.ConsistencyScoreView.
type ConsistencyScoreView struct {
	Score  float64 `json:"score"`
	Source string  `json:"source"`
	Detail string  `json:"detail,omitempty"`
}

// PreviewFrameView mirrors service.PreviewFrameView.
type PreviewFrameView struct {
	Index      int                    `json:"index"`
	PNG        string                 `json:"png"` // base64 PNG
	DurationMs int                    `json:"durationMs"`
	Anchors    []pipeline.AnchorPoint `json:"anchors,omitempty"`
}

// CandidatePreviewView is the PixelPerfect preview payload of a direction:
// the playable frames plus the motion's direction origin and the logical
// canvas size.
type CandidatePreviewView struct {
	MotionID     string             `json:"motionId"`
	Direction    string             `json:"direction"`
	Origin       string             `json:"origin"`
	Source       string             `json:"source,omitempty"`
	CanvasWidth  int                `json:"canvasWidth"`
	CanvasHeight int                `json:"canvasHeight"`
	Frames       []PreviewFrameView `json:"frames"`
}

// CandidateHistory returns the candidate history of the session package
// (task 8.4: 未接受候选连同评分与验收结果保留在候选历史中).
func (a *App) CandidateHistory() ([]CandidateHistoryView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	items, err := svc.CandidateHistory(pkg.Root())
	if err != nil {
		return nil, err
	}
	out := make([]CandidateHistoryView, 0, len(items))
	for _, it := range items {
		out = append(out, CandidateHistoryView{
			ID: it.ID, MotionID: it.MotionID, Direction: it.Direction,
			CreatedAt: it.CreatedAt, Status: it.Status, Overall: it.Overall,
			AcceptanceNote: it.AcceptanceNote, RegenerationOf: it.RegenerationOf,
		})
	}
	return out, nil
}

// CandidateDecide applies the quality acceptance gate to a candidate
// (task 8.3/9.2): pass requires scores ≥ threshold AND user confirmation in
// the PixelPerfect preview. confirm=false → rejected; true → the gate decides.
func (a *App) CandidateDecide(candidateID string, confirm bool, note string) (*AcceptanceDecisionView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	d, err := svc.CandidateDecide(a.bgCtx(), pkg.Root(), candidateID, confirm, note)
	if err != nil {
		return nil, err
	}
	return &AcceptanceDecisionView{CandidateID: d.CandidateID, Decision: d.Decision, Note: d.Note, Threshold: d.Threshold}, nil
}

// OperationLog returns the append-only operation log of the session package
// (task 9.3: 生成、编辑、接受、镜像替换等所有变更逐条追加).
func (a *App) OperationLog() ([]OperationLogEntryView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	entries, err := svc.OperationLog(pkg.Root())
	if err != nil {
		return nil, err
	}
	out := make([]OperationLogEntryView, 0, len(entries))
	for _, e := range entries {
		payload := any(nil)
		if len(e.Payload) > 0 {
			payload = string(e.Payload)
		}
		out = append(out, OperationLogEntryView{Seq: e.Seq, At: e.At, Action: e.Action, Payload: payload})
	}
	return out, nil
}

// RollbackTo restores the identity package content to the historical point at
// log seq; later log entries are preserved (task 9.4).
func (a *App) RollbackTo(seq int) ([]OperationLogEntryView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	entries, err := svc.RollbackTo(pkg.Root(), seq)
	if err != nil {
		return nil, err
	}
	out := make([]OperationLogEntryView, 0, len(entries))
	for _, e := range entries {
		payload := any(nil)
		if len(e.Payload) > 0 {
			payload = string(e.Payload)
		}
		out = append(out, OperationLogEntryView{Seq: e.Seq, At: e.At, Action: e.Action, Payload: payload})
	}
	return out, nil
}

// CurrentAssets returns the accepted animation assets of the current identity
// version (task 9.2 read view).
func (a *App) CurrentAssets() ([]AcceptedAssetView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	items, err := svc.CurrentAssets(pkg.Root())
	if err != nil {
		return nil, err
	}
	out := make([]AcceptedAssetView, 0, len(items))
	for _, it := range items {
		out = append(out, AcceptedAssetView{
			MotionID: it.MotionID, Direction: it.Direction, CandidateID: it.CandidateID,
			AcceptedAt: it.AcceptedAt, FrameCount: it.FrameCount,
		})
	}
	return out, nil
}

// CandidateConsistency computes the coarse same-character consistency score of
// the session package's candidates (task 8.2: 仅参考展示、不阻塞流程). When
// useAI is true, the active provider's text capability is used when available;
// otherwise (and on any failure) the local heuristic is returned.
func (a *App) CandidateConsistency(useAI bool) (ConsistencyScoreView, error) {
	svc, err := a.service()
	if err != nil {
		return ConsistencyScoreView{}, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return ConsistencyScoreView{}, err
	}
	v, err := svc.CandidateConsistency(a.bgCtx(), pkg.Root(), useAI)
	if err != nil {
		return ConsistencyScoreView{}, err
	}
	return ConsistencyScoreView{Score: v.Score, Source: v.Source, Detail: v.Detail}, nil
}

// DirectionPreview returns the PixelPerfect preview frames of a motion
// direction (task 5.5: 最近邻采样回放、网格叠加与锚点可视化; 预览渲染与切片结果
// 逐像素一致; 镜像方向渲染为源帧水平翻转).
func (a *App) DirectionPreview(motionID, direction string) (*CandidatePreviewView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	frames, err := svc.DirectionPreviewFrames(pkg.Root(), motionID, direction)
	if err != nil {
		return nil, err
	}
	viewFrames := make([]PreviewFrameView, 0, len(frames))
	for _, f := range frames {
		viewFrames = append(viewFrames, PreviewFrameView{
			Index: f.Index, PNG: f.PNG, DurationMs: f.DurationMs, Anchors: f.Anchors,
		})
	}
	mv, err := a.motionView(motionID)
	if err != nil {
		return nil, err
	}
	dir := motionViewDirection(mv, direction)
	view := &CandidatePreviewView{MotionID: motionID, Direction: direction, Frames: viewFrames}
	if c := pkg.LogicalCanvas(); c != nil {
		view.CanvasWidth = c.UnitWidth
		view.CanvasHeight = c.UnitHeight
	}
	if dir != nil {
		view.Origin = dir.Origin
		view.Source = dir.Source
	}
	return view, nil
}

// motionView loads one motion view (helper over the motion binding).
func (a *App) motionView(id string) (*MotionView, error) {
	return a.MotionGet(id)
}

// motionViewDirection finds a direction view by name.
func motionViewDirection(mv *MotionView, direction string) *DirectionView {
	if mv == nil {
		return nil
	}
	for i := range mv.Directions {
		if mv.Directions[i].Direction == direction {
			return &mv.Directions[i]
		}
	}
	return nil
}
