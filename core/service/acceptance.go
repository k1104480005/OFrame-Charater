package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/motion"
	"github.com/oframe/character-workbench/core/pipeline"
	"github.com/oframe/character-workbench/core/provider"
	"github.com/oframe/character-workbench/core/version"
)

// encodeBase64 encodes bytes for the frontend preview.
func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// --- quality acceptance (tasks 8.2–8.4, 9.2–9.4) ---

// AcceptanceDecisionView is the outcome of a candidate decision (task 8.3:
// 通过/拒绝两分支).
type AcceptanceDecisionView struct {
	CandidateID string  `json:"candidateId"`
	Decision    string  `json:"decision"` // accepted | rejected
	Note        string  `json:"note"`
	Threshold   float64 `json:"threshold"`
}

// CandidateHistoryView mirrors identity.CandidateHistoryRecord for the UI.
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

// CandidateHistory returns the candidate history of the session package
// (task 8.4: 未接受候选连同评分与验收结果保留在候选历史中).
func (s *Service) CandidateHistory(pkgPath string) ([]CandidateHistoryView, error) {
	pkg, err := identity.Open(pkgPath)
	if err != nil {
		return nil, err
	}
	h, err := pkg.LoadCandidateHistory()
	if err != nil {
		return nil, err
	}
	out := make([]CandidateHistoryView, 0, len(h.Items))
	for _, rec := range h.Items {
		out = append(out, CandidateHistoryView{
			ID: rec.ID, MotionID: rec.MotionID, Direction: rec.Direction,
			CreatedAt: rec.CreatedAt, Status: rec.Status, Overall: rec.Overall,
			AcceptanceNote: rec.AcceptanceNote, RegenerationOf: rec.RegenerationOf,
		})
	}
	return out, nil
}

// CandidateDecide applies the full quality acceptance gate to a candidate
// (task 8.3): it passes only when the scores meet the thresholds AND the user
// confirmed the PixelPerfect preview. Passing candidates become the current
// animation asset of their motion direction (task 9.2: 候选接受成为当前动画资产并
// 记入操作日志); rejected candidates remain in the candidate history
// (task 8.4). confirm=false means the user rejected the preview.
func (s *Service) CandidateDecide(ctx context.Context, pkgPath, candidateID string, confirm bool, note string) (*AcceptanceDecisionView, error) {
	pkg, err := identity.Open(pkgPath)
	if err != nil {
		return nil, err
	}
	thresholds := version.DefaultAcceptanceThresholds()
	decision, err := version.RecordDecision(pkg, candidateID, confirm, note, thresholds)
	if err != nil {
		return nil, err
	}
	rec, err := candidateHistoryRecord(pkg, candidateID)
	if err != nil {
		return nil, err
	}
	if decision == identity.CandidateAccepted {
		cand, err := s.findCandidate(pkgPath, candidateID)
		if err != nil {
			return nil, err
		}
		if err := s.acceptAssets(pkg, rec, cand); err != nil {
			return nil, err
		}
	}
	return &AcceptanceDecisionView{
		CandidateID: candidateID,
		Decision:    decision,
		Note:        rec.AcceptanceNote,
		Threshold:   thresholds.Overall,
	}, nil
}

// acceptAssets writes the accepted candidate's frames into the current
// version's asset area (task 9.2) and marks the history record accepted
// (acceptance timestamps are recorded).
func (s *Service) acceptAssets(pkg *identity.Package, rec identity.CandidateHistoryRecord, cand pipeline.Candidate) error {
	frames := make([]version.AssetFrame, 0, len(cand.Frames))
	for i, f := range cand.Frames {
		if f == nil {
			continue
		}
		pngData, err := pipeline.EncodeFilmstripPNG(f)
		if err != nil {
			return err
		}
		frames = append(frames, version.AssetFrame{Index: i, PNG: pngData})
	}
	if err := version.AcceptAssets(pkg, rec.MotionID, rec.Direction, cand.ID, frames); err != nil {
		return err
	}
	return pkg.UpdateCandidateHistory(cand.ID, func(r *identity.CandidateHistoryRecord) error {
		r.Status = identity.CandidateAccepted
		r.AcceptanceAt = time.Now().UTC().Format(time.RFC3339)
		return nil
	})
}

// candidateHistoryRecord fetches one history record by candidate id.
func candidateHistoryRecord(pkg *identity.Package, id string) (identity.CandidateHistoryRecord, error) {
	h, err := pkg.LoadCandidateHistory()
	if err != nil {
		return identity.CandidateHistoryRecord{}, err
	}
	for _, rec := range h.Items {
		if rec.ID == id {
			return rec, nil
		}
	}
	return identity.CandidateHistoryRecord{}, fmt.Errorf("service: candidate %q not in history", id)
}

// --- operation log (task 9.3) ---

// OperationLogEntryView mirrors version.LogEntry for the UI.
type OperationLogEntryView struct {
	Seq     int             `json:"seq"`
	At      string          `json:"at"`
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// OperationLog returns the append-only operation log of the session package
// (task 9.3: 生成、编辑、接受、镜像替换等所有变更逐条追加).
func (s *Service) OperationLog(pkgPath string) ([]OperationLogEntryView, error) {
	pkg, err := identity.Open(pkgPath)
	if err != nil {
		return nil, err
	}
	entries, err := version.Entries(pkg)
	if err != nil {
		return nil, err
	}
	out := make([]OperationLogEntryView, 0, len(entries))
	for _, e := range entries {
		out = append(out, OperationLogEntryView{Seq: e.Seq, At: e.At, Action: e.Action, Payload: e.Payload})
	}
	return out, nil
}

// --- rollback (task 9.4) ---

// RollbackTo restores the identity package content to the historical point at
// log seq (task 9.4: 回退到任一历史点后身份包内容恢复该点状态、后续日志保留) and
// returns the resulting log (which now carries the appended rollback entry).
func (s *Service) RollbackTo(pkgPath string, seq int) ([]OperationLogEntryView, error) {
	pkg, err := identity.Open(pkgPath)
	if err != nil {
		return nil, err
	}
	if err := version.Rollback(pkg, seq); err != nil {
		return nil, err
	}
	return s.OperationLog(pkgPath)
}

// --- current assets (task 9.2 read view) ---

// AcceptedAssetView is one accepted animation asset of the current version.
type AcceptedAssetView struct {
	MotionID    string `json:"motionId,omitempty"`
	Direction   string `json:"direction"`
	CandidateID string `json:"candidateId"`
	AcceptedAt  string `json:"acceptedAt"`
	FrameCount  int    `json:"frameCount"`
}

// CurrentAssets returns the accepted animation assets of the current identity
// version (task 9.2: 候选接受成为当前动画资产).
func (s *Service) CurrentAssets(pkgPath string) ([]AcceptedAssetView, error) {
	pkg, err := identity.Open(pkgPath)
	if err != nil {
		return nil, err
	}
	idx, err := version.CurrentAssetsIndex(pkg)
	if err != nil {
		return nil, err
	}
	out := make([]AcceptedAssetView, 0, len(idx.Items))
	for _, rec := range idx.Items {
		out = append(out, AcceptedAssetView{
			MotionID: rec.MotionID, Direction: rec.Direction, CandidateID: rec.CandidateID,
			AcceptedAt: rec.AcceptedAt, FrameCount: rec.FrameCount,
		})
	}
	return out, nil
}

// --- AI-assisted consistency score (task 8.2) ---

// ConsistencyScoreView is the reference-only coarse consistency score.
type ConsistencyScoreView struct {
	Score  float64 `json:"score"`  // 0..1
	Source string  `json:"source"` // "local" | "ai"
	Detail string  `json:"detail,omitempty"`
}

// aiScorePattern extracts a 0..1 number from a provider text answer.
var aiScorePattern = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)`)

// CandidateConsistency computes the coarse same-character consistency score of
// the package's candidates (task 8.2: AI 辅助一致性粗评分 — provider 或本地模型,
// 仅参考展示、不阻塞流程). When useAI is true and the active provider supports
// text generation with a configured key, a provider text call produces the
// score; on any failure the local heuristic is returned with source "local"
// (the score NEVER blocks the acceptance flow).
func (s *Service) CandidateConsistency(ctx context.Context, pkgPath string, useAI bool) (ConsistencyScoreView, error) {
	cands := s.CandidateList(pkgPath)
	local := consistencyScoreOf(cands)
	if !useAI {
		return ConsistencyScoreView{Score: local, Source: "local", Detail: fmt.Sprintf("%d 个候选方向", len(cands))}, nil
	}
	ps := s.settings.ProviderSettings()
	provID := ps.ActiveProvider
	if provID == "" {
		provID = "doubao"
	}
	prov, err := s.registry.Get(provID)
	if err != nil || !prov.Capabilities().Text {
		return ConsistencyScoreView{Score: local, Source: "local", Detail: "active provider has no text capability; local heuristic"}, nil
	}
	cfg := ps.ConfigFor(provID)
	if _, err := cfg.ResolveAPIKey(); err != nil {
		return ConsistencyScoreView{Score: local, Source: "local", Detail: "no provider key; local heuristic"}, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, cfg.EffectiveTimeout())
	defer cancel()
	res, err := prov.GenerateText(callCtx, providerTextRequest(cfg, cands))
	if err != nil {
		return ConsistencyScoreView{Score: local, Source: "local", Detail: "AI call failed (" + err.Error() + "); local heuristic"}, nil
	}
	score, ok := parseAIScore(res.Text)
	if !ok {
		return ConsistencyScoreView{Score: local, Source: "local", Detail: "AI answer not parseable; local heuristic"}, nil
	}
	return ConsistencyScoreView{Score: score, Source: "ai", Detail: fmt.Sprintf("provider %s (%d 个候选方向)", provID, len(cands))}, nil
}

// consistencyScoreOf builds the consistency input from candidates and computes
// the deterministic local heuristic.
func consistencyScoreOf(cands []pipeline.Candidate) float64 {
	input := pipeline.ConsistencyInput{}
	byDir := map[string]pipeline.DirectionConsistency{}
	for _, c := range cands {
		if c.Direction == "" || len(c.Frames) == 0 {
			continue
		}
		byDir[c.Direction] = pipeline.DirectionConsistency{Direction: c.Direction, Frames: c.Frames, Scores: c.Scores}
	}
	for _, d := range byDir {
		input.Directions = append(input.Directions, d)
	}
	for _, d := range byDir {
		if src := motion.MirrorSource(d.Direction); src != "" {
			if srcC, ok := byDir[src]; ok && len(srcC.Frames) > 0 && len(d.Frames) > 0 {
				input.MirrorPairs = append(input.MirrorPairs, [2]*image.RGBA{srcC.Frames[0], d.Frames[0]})
			}
		}
	}
	return pipeline.CoarseConsistencyScore(input)
}

// providerTextRequest builds the consistency-evaluation prompt.
func providerTextRequest(cfg providerConfigLike, cands []pipeline.Candidate) provider.TextRequest {
	dirs := make([]string, 0, len(cands))
	for _, c := range cands {
		if c.Direction != "" {
			dirs = append(dirs, c.Direction)
		}
	}
	return provider.TextRequest{
		Model: cfg.EffectiveTextModel(),
		Prompt: "你是像素动画质量评估器。对以下角色动画方向集的一致性（同角色、同体型、同配色、同节奏）给出 0 到 1 的粗评分，只输出数字：" +
			strings.Join(dirs, ", "),
	}
}

func parseAIScore(text string) (float64, bool) {
	m := aiScorePattern.FindString(text)
	if m == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return 0, false
	}
	if v > 1 {
		v = v / 100 // providers sometimes answer 0-100
	}
	if v < 0 || v > 1 {
		return 0, false
	}
	return v, true
}

// providerConfigLike narrows the config surface used by the AI scorer.
type providerConfigLike interface {
	EffectiveTextModel() string
}

// --- direction preview rendering (task 5.5 + mirror E2E) ---

// PreviewFrameView is one playable preview frame (lossless PNG data URL is
// built on the frontend; the backend returns raw PNG bytes).
type PreviewFrameView struct {
	Index      int                    `json:"index"`
	PNG        string                 `json:"png"` // base64 PNG (pixel-perfect preview)
	DurationMs int                    `json:"durationMs"`
	Anchors    []pipeline.AnchorPoint `json:"anchors,omitempty"`
}

// DirectionPreviewFrames resolves a motion direction's frame sequence to
// playable frames (task 5.5: PixelPerfect 预览与切片逐像素一致): generated /
// replaced directions render the candidate's processed frames; mirror-derived
// directions render the horizontal flip of their source frames (pixel-by-pixel
// identical to the source flipped — 镜像帧逐像素等于源帧水平翻转).
func (s *Service) DirectionPreviewFrames(pkgPath, motionID, direction string) ([]PreviewFrameView, error) {
	ms, err := motion.NewStore(pkgPath).Load()
	if err != nil {
		return nil, err
	}
	m, err := ms.Get(motionID)
	if err != nil {
		return nil, err
	}
	dir := m.Direction(direction)
	if dir == nil {
		return nil, fmt.Errorf("service: motion %s has no direction %s", motionID, direction)
	}
	seq := dir.Sequence
	src := motion.MirrorSource(direction)
	out := make([]PreviewFrameView, 0, len(seq.Frames))
	for i, f := range seq.Frames {
		var frame *image.RGBA
		var err error
		switch {
		case src != "" && dir.Origin == motion.OriginMirrored:
			srcFrame, e := s.resolveSourceFrame(pkgPath, motionID, src, i)
			if e != nil {
				return nil, e
			}
			frame = motion.HorizontalMirror(srcFrame)
		default:
			frame, err = s.resolveCandidateFrame(pkgPath, f.AssetRef)
			if err != nil {
				return nil, err
			}
		}
		pngData, err := pipeline.EncodeFilmstripPNG(frame)
		if err != nil {
			return nil, err
		}
		out = append(out, PreviewFrameView{
			Index:      i,
			PNG:        encodeBase64(pngData),
			DurationMs: f.DurationMs,
			Anchors:    f.Anchors,
		})
	}
	return out, nil
}

// resolveSourceFrame finds the source direction's rendered frame i.
func (s *Service) resolveSourceFrame(pkgPath, motionID, source string, i int) (*image.RGBA, error) {
	ms, err := motion.NewStore(pkgPath).Load()
	if err != nil {
		return nil, err
	}
	m, err := ms.Get(motionID)
	if err != nil {
		return nil, err
	}
	srcDir := m.Direction(source)
	if srcDir == nil || i >= len(srcDir.Sequence.Frames) {
		return nil, fmt.Errorf("service: mirror source %s frame %d unavailable", source, i)
	}
	return s.resolveCandidateFrame(pkgPath, srcDir.Sequence.Frames[i].AssetRef)
}

// resolveCandidateFrame resolves "candidate:<id>:frame:<n>" (or a direct
// candidate asset path) to the processed frame pixels.
func (s *Service) resolveCandidateFrame(pkgPath, assetRef string) (*image.RGBA, error) {
	candID, frameIdx, ok := parseCandidateRef(assetRef)
	if !ok {
		return nil, fmt.Errorf("service: cannot resolve asset ref %q", assetRef)
	}
	cand, err := s.findCandidate(pkgPath, candID)
	if err != nil {
		return nil, err
	}
	if frameIdx < 0 || frameIdx >= len(cand.Frames) {
		return nil, fmt.Errorf("service: candidate %s has no frame %d", candID, frameIdx)
	}
	return cand.Frames[frameIdx], nil
}

// parseCandidateRef parses "candidate:<id>:frame:<n>".
func parseCandidateRef(ref string) (string, int, bool) {
	parts := strings.Split(ref, ":")
	if len(parts) != 4 || parts[0] != "candidate" || parts[2] != "frame" {
		return "", 0, false
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil {
		return "", 0, false
	}
	return parts[1], n, true
}
