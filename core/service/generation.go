package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/motion"
	"github.com/oframe/character-workbench/core/pipeline"
	"github.com/oframe/character-workbench/core/provider"
	"github.com/oframe/character-workbench/core/task"
	"github.com/oframe/character-workbench/core/version"
)

// Default frame count for the filmstrip prompt when the request does not
// specify one (the motion model supplies the real count when available).
const defaultFrameCount = 4

// Plan statuses.
const (
	PlanPending   = "pending"   // shown in the confirmation, not yet decided
	PlanConfirmed = "confirmed" // user accepted; execution started
	PlanCancelled = "cancelled" // user cancelled; no external call was made
	PlanExecuted  = "executed"  // execution completed
	PlanFailed    = "failed"    // execution failed at the retry cap
)

// Plan kinds (阶段 5: 生成执行链入口).
const (
	PlanKindGenerate      = "generate"       // 新动作/方向集生成: basic 方向 + 镜像派生
	PlanKindReplace       = "replace"        // 验收时手动替换方向 (task 3.5)
	PlanKindRegenerate    = "regenerate"     // 验收未通过后的重新生成 (task 5.6)
	PlanKindBaseCharacter = "base-character" // identity base sprite, not a filmstrip
)

// GenerationRequest is the input for building a generation plan. Phase 3 took
// the direction count directly; phase 5 (motion capability) additionally
// accepts a MotionID so the direction strategy (count + mirror) comes from the
// motion, plus the replacement (3.5) and regeneration (5.6) flows.
type GenerationRequest struct {
	PackagePath             string   `json:"packagePath"`
	BaseCharacter           bool     `json:"baseCharacter,omitempty"`     // text-to-character single-image plan
	MotionID                string   `json:"motionId,omitempty"`          // "" → legacy direction-count mode
	ProviderID              string   `json:"providerId"`                  // "" → active provider (default Doubao)
	Model                   string   `json:"model"`                       // "" → provider default
	Directions              int      `json:"directions"`                  // 1 | 4 | 8 (legacy mode; 0 → 1)
	DisableMirror           bool     `json:"disableMirror,omitempty"`     // legacy mode: 关闭镜像 → 所有方向独立生成
	StylePresetID           string   `json:"stylePresetId"`               // "" → pixel
	StyleCustom             string   `json:"styleCustom,omitempty"`       // 自定义风格提示词（非空时优先于 StylePresetID）
	Description             string   `json:"description,omitempty"`       // 本次生成的提示词覆盖；空 → 身份描述
	ActionPresetID          string   `json:"actionPresetId"`              // "" → walk
	FrameCount              int      `json:"frameCount"`                  // 0 → 4 (或动作已有序列帧数)
	MaxAttemptsPerDirection int      `json:"maxAttemptsPerDirection"`     // 0 → 3
	ReplaceDirections       []string `json:"replaceDirections,omitempty"` // 3.5: 手动替换的方向
	RegenerateOf            string   `json:"regenerateOf,omitempty"`      // 5.6: 上一候选 id (重新生成)
}

// OutboundMaterial is one material that will be sent to the provider (外发素材):
// the identity's reference images with their roles, resolved to absolute paths.
type OutboundMaterial struct {
	MaterialID string `json:"materialId"`
	Kind       string `json:"kind"`
	Role       string `json:"role"`
	Name       string `json:"name"`
	Path       string `json:"path"`
}

// GenerationPlan is the generation confirmation payload: everything the user
// must see before any external call — 外发素材, provider/model, 方向数 (生成/镜像
// 明细), 预计调用量, 预算, 每方向最多 3 次总尝试 — plus the prompt snapshot
// (提示词快照).
type GenerationPlan struct {
	ID                      string                  `json:"id"`
	Kind                    string                  `json:"kind"` // generate | replace | regenerate
	Capability              string                  `json:"capability"`
	MotionID                string                  `json:"motionId,omitempty"`
	PackagePath             string                  `json:"packagePath"`
	ProviderID              string                  `json:"providerId"`
	ProviderType            string                  `json:"providerType,omitempty"` // 协议类型（任务 6.3 确认界面展示）
	Model                   string                  `json:"model"`
	Directions              int                     `json:"directions"`               // 方向数
	BasicDirections         int                     `json:"basicDirections"`          // 需生成的（非镜像）方向
	MirroredDirections      int                     `json:"mirroredDirections"`       // 镜像派生方向
	BasicLabels             []string                `json:"basicLabels,omitempty"`    // 生成方向明细
	MirroredLabels          []string                `json:"mirroredLabels,omitempty"` // 镜像方向明细
	ExpectedCalls           int                     `json:"expectedCalls"`            // 预计调用量
	MaxAttemptsPerDirection int                     `json:"maxAttemptsPerDirection"`  // 每方向最多尝试次数
	MaxTotalAttempts        int                     `json:"maxTotalAttempts"`         // 预算上限（总尝试）
	OutboundMaterials       []OutboundMaterial      `json:"outboundMaterials"`        // 外发素材
	Prompt                  pipeline.PromptSnapshot `json:"prompt"`                   // 提示词快照
	Canvas                  identity.CanvasSpec     `json:"canvas"`
	FrameCount              int                     `json:"frameCount"`
	Anchors                 []pipeline.AnchorPoint  `json:"anchors,omitempty"` // identity-level anchors (校正输入)
	RegenerateOf            string                  `json:"regenerateOf,omitempty"`
	CostPerCall             float64                 `json:"costPerCall"`
	Currency                string                  `json:"currency"`
	ExpectedCost            float64                 `json:"expectedCost"` // 预计费用 = 预计调用量 × 单价
	MaxCost                 float64                 `json:"maxCost"`      // 预算上限费用 = 总尝试 × 单价
	Status                  string                  `json:"status"`
	CreatedAt               string                  `json:"createdAt"`
}

// DirectionResult is the outcome of one generated direction's provider calls.
type DirectionResult struct {
	Direction   string `json:"direction"`
	Attempts    int    `json:"attempts"`
	Bytes       int    `json:"bytes"`
	Model       string `json:"model"`
	CandidateID string `json:"candidateId,omitempty"`
}

// GenerationResult is returned after the confirmation decision.
type GenerationResult struct {
	PlanID    string            `json:"planId"`
	Accepted  bool              `json:"accepted"`
	Status    string            `json:"status"`
	CallsMade int               `json:"callsMade"`
	Attempts  int               `json:"attempts"`
	Results   []DirectionResult `json:"results"`
	Error     string            `json:"error"`
}

// planRegistry holds the in-memory plan registry (阶段 3; the recoverable task
// queue of tasks 6.x will persist plans across restarts).
type planRegistry struct {
	mu    sync.Mutex
	plans map[string]*GenerationPlan
}

func newPlanRegistry() *planRegistry {
	return &planRegistry{plans: make(map[string]*GenerationPlan)}
}

func (r *planRegistry) put(p *GenerationPlan) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plans[p.ID] = p
}

func (r *planRegistry) get(id string) (*GenerationPlan, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.plans[id]
	return p, ok
}

func (r *planRegistry) setStatus(id, status string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.plans[id]
	if !ok {
		return false
	}
	p.Status = status
	return true
}

func (r *planRegistry) claimPending(id string, accept bool) (*GenerationPlan, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.plans[id]
	if !ok || p.Status != PlanPending {
		return p, false
	}
	if accept {
		p.Status = PlanConfirmed
	} else {
		p.Status = PlanCancelled
	}
	return p, true
}

// newPlanID returns a random UUIDv4 for plan identifiers.
func newPlanID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("service: crypto/rand failed: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// PrepareGeneration builds the generation confirmation plan for a request. It
// performs NO external calls: it opens the identity package, resolves the
// provider/model, collects the outbound materials (reference images with
// roles), computes the direction/budget numbers (每方向最多 3 次总尝试) and
// renders the prompt snapshot (提示词快照). The user reviews this plan and
// confirms or cancels via ConfirmGeneration.
//
// Plan kinds (阶段 5):
//   - generate:    basic directions per the motion's strategy (or legacy count)
//     plus mirrored derivation; ExpectedCalls = len(basic).
//   - replace:     manual replacement of the listed directions (task 3.5);
//     ExpectedCalls = len(replaceDirections) — the replacement is counted in
//     the confirmation's expected call count.
//   - regenerate:  new filmstrip for a failed candidate (task 5.6);
//     ExpectedCalls = 1.
func (s *Service) PrepareGeneration(ctx context.Context, req GenerationRequest) (*GenerationPlan, error) {
	if strings.TrimSpace(req.PackagePath) == "" {
		return nil, fmt.Errorf("service: package path is required")
	}
	pkg, err := identity.Open(req.PackagePath)
	if err != nil {
		return nil, err
	}
	if err := pkg.ValidateReferenceRoles(); err != nil {
		return nil, err
	}
	if req.BaseCharacter {
		return s.prepareBaseCharacter(pkg, req)
	}

	// Provider + model (first generation defaults to Doubao).
	ps := s.settings.ProviderSettings()
	if len(ps.Providers) == 0 {
		// 人工验收更新：fresh installs start with NO provider cards — the user
		// adds one from the seven presets first. Fail with a readable,
		// actionable error instead of "unknown provider doubao".
		return nil, fmt.Errorf("service: 尚未配置任何 Provider — 请先在设置的七预设中添加并填写密钥")
	}
	providerID := req.ProviderID
	if providerID == "" {
		providerID = ps.ActiveProvider
	}
	if providerID == "" {
		providerID = provider.DefaultProviderID
	}
	prov, err := s.registry.Get(providerID)
	if err != nil {
		return nil, err
	}
	cfg := ps.ConfigFor(providerID)
	// Capability gate (align-framebaker-providers 1.4): resolve and validate the
	// image model against the provider's capability declaration AND its model
	// catalog BEFORE building the confirmation plan. A mismatched request fails
	// here — offline, readable, errors.Is-branchable — and neither provider nor
	// model is ever silently substituted.
	model, err := provider.ResolveValidatedModel(prov.Capabilities(), cfg, provider.ModalityImage, req.Model)
	if err != nil {
		return nil, fmt.Errorf("service: %w", err)
	}

	// Presets.
	styleID := req.StylePresetID
	if styleID == "" {
		styleID = pipeline.StylePresetClassic.ID
	}
	style, err := pipeline.StylePresetByID(styleID)
	if err != nil {
		return nil, err
	}
	actionID := req.ActionPresetID
	if actionID == "" {
		actionID = pipeline.ActionWalk.ID
	}
	action, err := pipeline.ActionPresetByID(actionID)
	if err != nil {
		return nil, err
	}

	// Logical canvas.
	canvas := pkg.LogicalCanvas()
	if canvas == nil {
		return nil, fmt.Errorf("service: logical canvas must be set before planning generation")
	}

	// Direction strategy (阶段 5: 动作与方向集驱动方向策略; legacy count mode kept).
	strategy, motionID, err := s.resolveStrategy(pkg, req)
	if err != nil {
		return nil, err
	}
	kind := PlanKindGenerate
	basic := motion.BasicDirections(strategy)
	mirrored := motion.MirroredDirections(strategy)
	expectedCalls := len(basic)
	switch {
	case req.RegenerateOf != "":
		kind = PlanKindRegenerate
		basic, mirrored = nil, nil
		expectedCalls = 1
		if _, err := s.findCandidate(pkg.Root(), req.RegenerateOf); err != nil {
			return nil, err
		}
	case len(req.ReplaceDirections) > 0:
		kind = PlanKindReplace
		basic, mirrored = req.ReplaceDirections, nil
		expectedCalls = len(basic)
		if motionID != "" {
			ms, err := motion.NewStore(pkg.Root()).Load()
			if err != nil {
				return nil, err
			}
			m, err := ms.Get(motionID)
			if err != nil {
				return nil, err
			}
			for _, dir := range basic {
				if m.Direction(dir) == nil {
					return nil, fmt.Errorf("service: motion %q has no direction %q to replace", motionID, dir)
				}
			}
		}
	}

	// Outbound materials: the identity's reference images (主 + 辅助), resolved
	// to absolute paths. Sprites are the identity basis and are not sent out.
	var outbound []OutboundMaterial
	for _, m := range pkg.ReferenceImages() {
		if m.Role != identity.RoleMainReference && m.Role != identity.RoleAuxiliaryReference {
			continue
		}
		abs, err := pkg.MaterialPath(m)
		if err != nil {
			return nil, err
		}
		outbound = append(outbound, OutboundMaterial{
			MaterialID: m.ID, Kind: m.Kind, Role: m.Role, Name: m.Name, Path: abs,
		})
	}

	// Prompt snapshot. The frame count follows the motion's existing sequence
	// length for replacement/regeneration (帧数与节奏沿用的基础); otherwise the
	// request value or the default.
	frameCount := req.FrameCount
	if frameCount <= 0 {
		frameCount = defaultFrameCount
	}
	if motionID != "" && kind != PlanKindGenerate {
		if ms, err := motion.NewStore(pkg.Root()).Load(); err == nil {
			if m, err := ms.Get(motionID); err == nil {
				for _, d := range m.Directions {
					if len(d.Sequence.Frames) > 0 {
						frameCount = len(d.Sequence.Frames)
						break
					}
				}
			}
		}
	}
	refs := make([]pipeline.ReferenceImageRef, 0, len(outbound))
	for _, o := range outbound {
		refs = append(refs, pipeline.ReferenceImageRef{MaterialID: o.MaterialID, Role: o.Role, Name: o.Name})
	}
	description := pkg.Description()
	if strings.TrimSpace(req.Description) != "" {
		description = req.Description
	}
	prompt, err := pipeline.BuildPrompt(pipeline.PromptInput{
		Description:  description,
		StylePreset:  style,
		ActionPreset: action,
		References:   refs,
		CanvasWidth:  canvas.UnitWidth,
		CanvasHeight: canvas.UnitHeight,
		FrameCount:   frameCount,
		Directions:   strategy.Count,
	})
	if err != nil {
		return nil, err
	}

	// Budget (生成确认: 预计调用量与预算; 每方向最多 3 次总尝试).
	maxAttempts := req.MaxAttemptsPerDirection
	if maxAttempts <= 0 {
		maxAttempts = cfg.EffectiveMaxAttempts()
	}
	maxTotalAttempts := expectedCalls * maxAttempts
	price := cfg.EffectivePrice()

	plan := &GenerationPlan{
		ID:                      newPlanID(),
		Kind:                    kind,
		Capability:              provider.ModalityImage.String(),
		MotionID:                motionID,
		PackagePath:             pkg.Root(),
		ProviderID:              providerID,
		ProviderType:            cfg.EffectiveType(),
		Model:                   model,
		Directions:              strategy.Count,
		BasicDirections:         len(basic),
		MirroredDirections:      len(mirrored),
		BasicLabels:             basic,
		MirroredLabels:          mirrored,
		ExpectedCalls:           expectedCalls,
		MaxAttemptsPerDirection: maxAttempts,
		MaxTotalAttempts:        maxTotalAttempts,
		OutboundMaterials:       outbound,
		Prompt:                  prompt,
		Canvas:                  *canvas,
		FrameCount:              frameCount,
		Anchors:                 anchorsFromPkg(pkg),
		RegenerateOf:            req.RegenerateOf,
		CostPerCall:             price,
		Currency:                provider.Currency(providerID),
		ExpectedCost:            round2(float64(expectedCalls) * price),
		MaxCost:                 round2(float64(maxTotalAttempts) * price),
		Status:                  PlanPending,
		CreatedAt:               time.Now().UTC().Format(time.RFC3339),
	}
	s.plans.put(plan)
	s.log.Info("generation plan prepared",
		"plan", plan.ID, "kind", kind, "provider", providerID, "model", model,
		"motion", motionID, "directions", strategy.Count, "mirror", strategy.Mirror,
		"basic", len(basic), "mirrored", len(mirrored), "expectedCalls", expectedCalls,
		"maxAttemptsPerDirection", maxAttempts, "outboundMaterials", len(outbound),
		"regenerateOf", req.RegenerateOf, "prompt", prompt.Prompt)
	return plan, nil
}

// resolveStrategy derives the direction strategy from the motion when a
// MotionID is given (阶段 5), else from the legacy request count (mirror
// defaults on; DisableMirror turns it off → 所有方向独立生成).
func (s *Service) resolveStrategy(pkg *identity.Package, req GenerationRequest) (motion.DirectionStrategy, string, error) {
	if req.MotionID != "" {
		ms, err := motion.NewStore(pkg.Root()).Load()
		if err != nil {
			return motion.DirectionStrategy{}, "", err
		}
		m, err := ms.Get(req.MotionID)
		if err != nil {
			return motion.DirectionStrategy{}, "", err
		}
		if err := motion.ValidateStrategy(m.Strategy); err != nil {
			return motion.DirectionStrategy{}, "", fmt.Errorf("service: motion %q: %w", req.MotionID, err)
		}
		return m.Strategy, req.MotionID, nil
	}
	dirs := req.Directions
	if dirs == 0 {
		dirs = 1
	}
	if dirs != 1 && dirs != 4 && dirs != 8 {
		return motion.DirectionStrategy{}, "", fmt.Errorf("service: direction count must be 1, 4 or 8, got %d", dirs)
	}
	return motion.DirectionStrategy{Count: dirs, Mirror: !req.DisableMirror}, "", nil
}

// GetPlan returns a prepared plan by id.
func (s *Service) GetPlan(id string) (*GenerationPlan, error) {
	p, ok := s.plans.get(id)
	if !ok {
		return nil, fmt.Errorf("service: unknown generation plan %q", id)
	}
	return p, nil
}

// ConfirmGeneration implements the confirmation gate (generation spec 4.5):
// accept=true executes the plan's provider calls (with the agreed retry cap),
// running the filmstrip pipeline for every generated direction; accept=false
// aborts without issuing any external call.
//
// Execution is a persisted queue task (tasks 6.1–6.5): the plan becomes a
// task row (payload = the plan), progress is updated during execution, the
// success result is cached under the plan fingerprint, and an IDENTICAL
// re-submitted task reuses the cached result without any new external call
// (task 6.4: 幂等去重).
func (s *Service) ConfirmGeneration(ctx context.Context, planID string, accept bool) (*GenerationResult, error) {
	plan, claimed := s.plans.claimPending(planID, accept)
	if plan == nil {
		return nil, fmt.Errorf("service: unknown generation plan %q", planID)
	}
	if !claimed {
		return nil, fmt.Errorf("service: generation plan %q is already %s", planID, plan.Status)
	}
	if !accept {
		s.log.Info("generation cancelled", "plan", planID)
		return &GenerationResult{PlanID: planID, Accepted: false, Status: PlanCancelled}, nil
	}

	prov, err := s.registry.Get(plan.ProviderID)
	if err != nil {
		// The plan must not stay "confirmed": the decision is final and the
		// failure reason is recorded (阶段 5 复核: provider 查询失败应置 PlanFailed).
		s.plans.setStatus(planID, PlanFailed)
		return nil, fmt.Errorf("service: provider %s unavailable: %w", plan.ProviderID, err)
	}
	// Pre-flight capability check at confirmation time (task 1.4): the plan
	// snapshot fixed provider+model before the user confirmed; re-validate them
	// against the CURRENT capability declaration and model catalog so a config
	// change between prepare and confirm cannot send an unvalidated request.
	// On failure no external call is made and the plan is marked failed.
	ps := s.settings.ProviderSettings()
	if _, verr := provider.ResolveValidatedModel(prov.Capabilities(), ps.ConfigFor(plan.ProviderID), provider.ModalityImage, plan.Model); verr != nil {
		s.plans.setStatus(planID, PlanFailed)
		s.log.Error("generation refused by pre-flight capability check", "plan", planID, "provider", plan.ProviderID, "model", plan.Model, "error", verr)
		return nil, fmt.Errorf("service: plan %s pre-flight check failed: %w", planID, verr)
	}

	// Idempotent dedup (task 6.4 / 4.8): an identical already-successful task
	// reuses the cached result without a new external call.
	fp := planFingerprint(plan)
	if cached, hit, err := s.queueStore.CacheGet(fp); err == nil && hit && fp != "" {
		var cachedRes GenerationResult
		if json.Unmarshal([]byte(cached), &cachedRes) == nil {
			// Persist the dedup as a succeeded task row so the drawer shows it.
			t, terr := s.createTaskForPlan(plan, fp)
			if terr == nil {
				_, _ = s.queueStore.Update(t.ID, func(tt *task.Task) error {
					tt.Status = task.StatusSucceeded
					tt.Progress = 1
					tt.Result = cached
					tt.Error = "reused cached success result (idempotent dedup, no external call)"
					return nil
				})
			}
			s.plans.setStatus(planID, PlanExecuted)
			s.log.Info("generation deduplicated", "plan", planID, "fingerprint", fp, "callsMade", cachedRes.CallsMade)
			cachedRes.PlanID = planID
			return &cachedRes, nil
		}
	}

	t, err := s.createTaskForPlan(plan, fp)
	if err != nil {
		return nil, err
	}
	s.log.Info("generation task enqueued", "task", t.ID, "kind", t.Kind, "expectedCalls", t.ExpectedCalls)

	// Execute (the queue row tracks status/progress; a crash mid-run leaves
	// the task resumable — task 6.3).
	s.executeTask(ctx, t)
	res, err := s.GenerationResultOf(planID)
	if err != nil {
		// The task row holds the outcome; surface what we can.
		if tv, terr := s.TaskGet(planID); terr == nil {
			return &GenerationResult{PlanID: planID, Accepted: true, Status: mapTaskStatusToPlan(tv.Status), Error: tv.Error}, nil
		}
		return nil, err
	}
	return res, nil
}

// GenerationResultOf returns the plan's final result: executed / failed /
// cancelled, mirroring the old synchronous contract.
func (s *Service) GenerationResultOf(planID string) (*GenerationResult, error) {
	p, ok := s.plans.get(planID)
	if !ok {
		return nil, fmt.Errorf("service: unknown generation plan %q", planID)
	}
	switch p.Status {
	case PlanCancelled:
		return &GenerationResult{PlanID: planID, Accepted: false, Status: PlanCancelled}, nil
	case PlanExecuted:
		res := &GenerationResult{PlanID: planID, Accepted: true, Status: PlanExecuted, Results: []DirectionResult{}}
		// Reconstruct from the task's cached result when available.
		if t, err := s.queueStore.Get(planID); err == nil && t.Result != "" {
			var cached GenerationResult
			if json.Unmarshal([]byte(t.Result), &cached) == nil {
				return &cached, nil
			}
		}
		return res, nil
	case PlanFailed:
		res := &GenerationResult{PlanID: planID, Accepted: true, Status: PlanFailed, Results: []DirectionResult{}}
		if t, err := s.queueStore.Get(planID); err == nil {
			res.Error = t.Error
		}
		return res, nil
	default:
		return nil, fmt.Errorf("service: generation plan %q is %s (not settled)", planID, p.Status)
	}
}

// mapTaskStatusToPlan maps a task row status back to a plan status.
func mapTaskStatusToPlan(status string) string {
	switch status {
	case task.StatusSucceeded:
		return PlanExecuted
	case task.StatusFailed, task.StatusAbandoned:
		return PlanFailed
	default:
		return status
	}
}

// runGeneration executes a generate plan: one provider call + filmstrip
// pipeline per basic direction (provider 原始字节 → 解码 → ProcessFilmstrip →
// 候选落盘 → CandidateSet 保留), then derives the mirrored directions as
// independent frame sequences with horizontally-mirrored anchors (task 3.4),
// and updates the motion's direction set. Progress advances per direction and
// the outcome is appended to the operation log (task 9.3: 生成计入操作日志).
func (s *Service) runGeneration(ctx context.Context, plan *GenerationPlan, prov provider.Provider, cfg provider.ProviderConfig, refs []provider.ReferenceImage, progress func(completed, total int)) *GenerationResult {
	res := &GenerationResult{PlanID: plan.ID, Accepted: true, Status: PlanExecuted, Results: []DirectionResult{}}

	st, ms, mv, err := s.motionFor(plan)
	if err != nil {
		return s.failPlan(plan, res, err.Error())
	}

	total := len(plan.BasicLabels)
	for i, dir := range plan.BasicLabels {
		dr, err := s.generateDirectionResult(ctx, prov, cfg, plan, refs, dir)
		if err != nil {
			return s.failPlan(plan, res, fmt.Sprintf("direction %s: %v", dir, err))
		}
		res.Results = append(res.Results, dr)
		res.Attempts += dr.Attempts
		res.CallsMade++
		if mv != nil {
			cand, err := s.findCandidate(plan.PackagePath, dr.CandidateID)
			if err != nil {
				return s.failPlan(plan, res, err.Error())
			}
			if err := mv.SetDirectionSequence(dir, sequenceFromCandidate(cand), motion.OriginGenerated); err != nil {
				return s.failPlan(plan, res, err.Error())
			}
		}
		if progress != nil {
			progress(i+1, total)
		}
	}

	// 镜像派生 (task 3.3/3.4): mirrored directions get their own independent
	// frame sequence with anchors converted by the horizontal mirror rule.
	if mv != nil {
		for _, mdir := range plan.MirroredLabels {
			src := motion.MirrorSource(mdir)
			srcDir := mv.Direction(src)
			if srcDir == nil || len(srcDir.Sequence.Frames) == 0 {
				return s.failPlan(plan, res, fmt.Sprintf("mirrored direction %s: source %s has no frames", mdir, src))
			}
			seq, err := motion.MirrorSequence(srcDir.Sequence, plan.Canvas.UnitWidth)
			if err != nil {
				return s.failPlan(plan, res, err.Error())
			}
			if err := mv.SetDirectionSequence(mdir, seq, motion.OriginMirrored); err != nil {
				return s.failPlan(plan, res, err.Error())
			}
		}
		if err := st.Save(ms); err != nil {
			return s.failPlan(plan, res, fmt.Sprintf("persist motions: %v", err))
		}
	}

	s.plans.setStatus(plan.ID, PlanExecuted)
	s.log.Info("generation executed", "plan", plan.ID, "calls", res.CallsMade, "attempts", res.Attempts)
	s.logGeneration(plan, res, version.ActionGeneration)
	return res
}

// runReplacement executes a replace plan (task 3.5: 验收时手动替换镜像方向, 以
// 额外调用替换): one provider call + filmstrip pipeline per replaced direction,
// and the direction set is updated with the replacement frames (origin
// "replaced"). The replacement was already counted in the plan's expected call
// count at prepare time. The outcome is appended to the operation log
// (task 9.3: 镜像替换计入操作日志).
func (s *Service) runReplacement(ctx context.Context, plan *GenerationPlan, prov provider.Provider, cfg provider.ProviderConfig, refs []provider.ReferenceImage, progress func(completed, total int)) *GenerationResult {
	res := &GenerationResult{PlanID: plan.ID, Accepted: true, Status: PlanExecuted, Results: []DirectionResult{}}

	st, ms, mv, err := s.motionFor(plan)
	if err != nil {
		return s.failPlan(plan, res, err.Error())
	}

	total := len(plan.BasicLabels) // BasicLabels == replaceDirections for replace plans
	for i, dir := range plan.BasicLabels {
		dr, err := s.generateDirectionResult(ctx, prov, cfg, plan, refs, dir)
		if err != nil {
			return s.failPlan(plan, res, fmt.Sprintf("direction %s: %v", dir, err))
		}
		res.Results = append(res.Results, dr)
		res.Attempts += dr.Attempts
		res.CallsMade++
		if mv != nil {
			cand, err := s.findCandidate(plan.PackagePath, dr.CandidateID)
			if err != nil {
				return s.failPlan(plan, res, err.Error())
			}
			if err := mv.ReplaceDirection(dir, sequenceFromCandidate(cand)); err != nil {
				return s.failPlan(plan, res, err.Error())
			}
		}
		if progress != nil {
			progress(i+1, total)
		}
	}
	if mv != nil {
		if err := st.Save(ms); err != nil {
			return s.failPlan(plan, res, fmt.Sprintf("persist motions: %v", err))
		}
	}

	s.plans.setStatus(plan.ID, PlanExecuted)
	s.log.Info("direction replacement executed", "plan", plan.ID, "calls", res.CallsMade, "attempts", res.Attempts)
	s.logGeneration(plan, res, version.ActionMirrorReplacement)
	return res
}

// runRegeneration executes a regenerate plan through the RegenerateCandidate
// seam (task 5.6: 验收未通过后的重新生成, 遵循生成确认规则). The plan is already
// confirmed here; RegenerateCandidate enforces the gate and performs the
// provider call + pipeline regeneration. The outcome is appended to the
// operation log (task 9.3).
func (s *Service) runRegeneration(ctx context.Context, plan *GenerationPlan, prov provider.Provider, cfg provider.ProviderConfig, refs []provider.ReferenceImage) *GenerationResult {
	res, attempts, err := s.RegenerateCandidate(ctx, plan.ID)
	if err != nil {
		msg := err.Error()
		s.plans.setStatus(plan.ID, PlanFailed)
		s.log.Error("regeneration failed", "plan", plan.ID, "error", err)
		return &GenerationResult{PlanID: plan.ID, Accepted: true, Status: PlanFailed, Error: msg}
	}
	s.plans.setStatus(plan.ID, PlanExecuted)
	dr := DirectionResult{
		Direction:   res.Candidate.Direction,
		Attempts:    attempts,
		Bytes:       len(res.Candidate.FilmstripPNG),
		Model:       plan.Model,
		CandidateID: res.Candidate.ID,
	}
	out := &GenerationResult{PlanID: plan.ID, Accepted: true, Status: PlanExecuted, CallsMade: 1, Attempts: attempts, Results: []DirectionResult{dr}}
	s.log.Info("regeneration executed", "plan", plan.ID, "candidate", res.Candidate.ID, "of", plan.RegenerateOf)
	s.logGeneration(plan, out, version.ActionRegeneration)
	return out
}

// RegenerateCandidate regenerates a failed candidate as a new filmstrip under
// the generation confirmation gate (filmstrip-pipeline spec: "Regeneration
// after failed acceptance", task 5.6). It REFUSES to run unless the plan was
// prepared via PrepareGeneration and confirmed via ConfirmGeneration: the plan
// status must be "confirmed" — direct calls against pending / cancelled /
// unknown plans make NO external call and return an error.
//
// Execution chain: one provider call for the new filmstrip → 解码 →
// pipeline.Regenerate (linked to the previous candidate) → 候选落盘 →
// CandidateSet 保留. Pipeline options derive from the plan (identity-level
// anchors), so callers need no opts argument (阶段 5 复核: 未使用的 opts 已移除).
func (s *Service) RegenerateCandidate(ctx context.Context, planID string) (pipeline.ProcessResult, int, error) {
	plan, ok := s.plans.get(planID)
	if !ok {
		return pipeline.ProcessResult{}, 0, fmt.Errorf("service: unknown generation plan %q", planID)
	}
	if plan.Status != PlanConfirmed {
		return pipeline.ProcessResult{}, 0, fmt.Errorf(
			"service: regeneration requires a confirmed plan (PrepareGeneration → ConfirmGeneration); plan %q is %s",
			planID, plan.Status)
	}
	if plan.RegenerateOf == "" {
		return pipeline.ProcessResult{}, 0, fmt.Errorf("service: plan %q is not a regeneration plan", planID)
	}
	prev, err := s.findCandidate(plan.PackagePath, plan.RegenerateOf)
	if err != nil {
		return pipeline.ProcessResult{}, 0, err
	}

	prov, err := s.registry.Get(plan.ProviderID)
	if err != nil {
		return pipeline.ProcessResult{}, 0, err
	}
	cfg := s.settings.ProviderSettings().ConfigFor(plan.ProviderID)
	refs, err := s.loadOutboundMaterials(plan)
	if err != nil {
		return pipeline.ProcessResult{}, 0, err
	}

	raw, attempts, err := s.callProviderOnce(ctx, prov, cfg, plan, refs)
	if err != nil {
		return pipeline.ProcessResult{}, attempts, err
	}
	strip, err := pipeline.DecodeFilmstrip(raw)
	if err != nil {
		return pipeline.ProcessResult{}, attempts, fmt.Errorf("service: decode regenerated filmstrip: %w", err)
	}
	res, err := pipeline.Regenerate(prev, strip, plan.Prompt, prev.Layout, s.processOptions(plan))
	if err != nil {
		// 切片/校正失败: 失败候选仍保留 (生成结果保留最佳候选而非空手返回).
		res.Candidate.Direction = prev.Direction
		if perr := s.persistCandidate(plan, res.Candidate); perr != nil {
			return res, attempts, fmt.Errorf("service: filmstrip pipeline: %v (and candidate persist failed: %v)", err, perr)
		}
		s.candidatesFor(plan.PackagePath).Add(res.Candidate)
		return res, attempts, err
	}
	res.Candidate.Direction = prev.Direction
	if err := s.persistCandidate(plan, res.Candidate); err != nil {
		return pipeline.ProcessResult{}, attempts, fmt.Errorf("service: persist candidate: %w", err)
	}
	s.candidatesFor(plan.PackagePath).Add(res.Candidate)
	return res, attempts, nil
}

// generateDirectionResult runs the provider call + filmstrip pipeline for one
// direction: provider 原始字节 → 解码 → ProcessFilmstrip → 候选落盘 →
// CandidateSet 保留, returning the direction result carrying the candidate id.
func (s *Service) generateDirectionResult(ctx context.Context, prov provider.Provider, cfg provider.ProviderConfig, plan *GenerationPlan, refs []provider.ReferenceImage, dir string) (DirectionResult, error) {
	raw, attempts, err := s.callProviderOnce(ctx, prov, cfg, plan, refs)
	if err != nil {
		return DirectionResult{Direction: dir, Attempts: attempts}, err
	}
	cand, err := s.processFilmstrip(plan, raw, dir)
	if err != nil {
		return DirectionResult{Direction: dir, Attempts: attempts, Bytes: len(raw), CandidateID: cand.ID, Model: plan.Model}, err
	}
	return DirectionResult{Direction: dir, Attempts: attempts, Bytes: len(raw), CandidateID: cand.ID, Model: plan.Model}, nil
}

// callProviderOnce runs ONE provider call under the plan's retry policy (每方向
// 最多 3 次总尝试) and records call statistics. Returns the raw image bytes and
// the attempt count.
func (s *Service) callProviderOnce(ctx context.Context, prov provider.Provider, cfg provider.ProviderConfig, plan *GenerationPlan, refs []provider.ReferenceImage) ([]byte, int, error) {
	// Pre-flight capability gate (task 1.4): every retry attempt runs through
	// here, so an image request is only issued when the adapter's capability
	// declaration supports images AND the plan's model belongs to the
	// provider's image catalog. Not-retryable: no config error heals by retry.
	if _, err := provider.ResolveValidatedModel(prov.Capabilities(), cfg, provider.ModalityImage, plan.Model); err != nil {
		return nil, 0, provider.MarkNotRetryable(fmt.Errorf("service: pre-flight capability check failed for %s/%s: %w", plan.ProviderID, plan.Model, err))
	}
	req := provider.ImageRequest{
		Prompt:     plan.Prompt.Prompt,
		Model:      plan.Model,
		Width:      provider.DefaultGenerationSize,
		Height:     provider.DefaultGenerationSize,
		References: refs,
	}
	policy := provider.PolicyFromConfig(cfg)
	attempts := 0
	var result *provider.ImageResult
	callCtx, cancel := context.WithTimeout(ctx, cfg.EffectiveTimeout())
	defer cancel()
	err := provider.CallWithRetry(callCtx, policy, func(ctx context.Context) error {
		attempts++
		r, err := prov.GenerateImage(ctx, req)
		if err != nil {
			return err
		}
		result = r
		return nil
	})
	if err != nil {
		return nil, attempts, err
	}
	if err := s.settings.RecordCall(plan.ProviderID, plan.Model, plan.CostPerCall); err != nil {
		return nil, attempts, err
	}
	return result.Data, attempts, nil
}

// processFilmstrip decodes the provider's raw bytes into the filmstrip, runs
// the deterministic pipeline, persists the candidate into the package's
// candidate area and retains it in the package's CandidateSet (filmstrip 管线
// 正式接入生成执行链). The candidate carries the direction it was generated for.
// On pipeline failure the FAILED candidate is still persisted and retained
// (保留最佳候选而非空手返回) and the error is returned.
func (s *Service) processFilmstrip(plan *GenerationPlan, raw []byte, dir string) (pipeline.Candidate, error) {
	strip, err := pipeline.DecodeFilmstrip(raw)
	if err != nil {
		return pipeline.Candidate{}, fmt.Errorf("service: decode filmstrip: %w", err)
	}
	layout, err := pipeline.NormalizeFrameList(plan.Canvas, plan.FrameCount)
	if err != nil {
		return pipeline.Candidate{}, err
	}
	res, err := pipeline.ProcessFilmstrip(strip, plan.Prompt, layout, s.processOptions(plan))
	res.Candidate.Direction = dir
	if err != nil {
		// 失败候选仍尽力保留（保留最佳候选而非空手返回）；保留失败时把原因带出，不吞错。
		if perr := s.persistCandidate(plan, res.Candidate); perr != nil {
			return res.Candidate, fmt.Errorf("service: filmstrip pipeline: %v (and candidate persist failed: %v)", err, perr)
		}
		s.candidatesFor(plan.PackagePath).Add(res.Candidate)
		return res.Candidate, fmt.Errorf("service: filmstrip pipeline: %v", err)
	}
	if err := s.persistCandidate(plan, res.Candidate); err != nil {
		// 成功路径落盘失败必须让任务失败，否则会出现"看似成功却无候选文件"的假成功。
		return pipeline.Candidate{}, fmt.Errorf("service: persist candidate: %w", err)
	}
	s.candidatesFor(plan.PackagePath).Add(res.Candidate)
	return res.Candidate, nil
}

// processOptions assembles the pipeline options for a plan: the identity-level
// anchors are the anchor-correction input (锚点校正), targets default to the
// feet baseline.
func (s *Service) processOptions(plan *GenerationPlan) pipeline.ProcessOptions {
	opts := pipeline.ProcessOptions{}
	if len(plan.Anchors) > 0 {
		opts.Anchors = plan.Anchors
	}
	if pkg, err := identity.Open(plan.PackagePath); err == nil {
		opts.PerfectPixelStandard = pkg.PerfectPixelStandard()
	}
	// 风格→调色板对齐 perfectpixel：plan.Prompt.StylePresetID（已核对字段名）
	// 决定量化策略。
	if size := pipeline.PaletteSizeForStyle(plan.Prompt.StylePresetID); size > 0 {
		opts.Palette.MaxColors = size
	} else {
		opts.Palette.Skip = true
	}
	return opts
}

// persistCandidate writes the candidate's preserved artifacts into
// <package>/candidates/<candidateID>/ (候选落盘) and records it in the package's
// candidate history index (task 8.4: 评分与验收结果写入身份包元数据 — every
// produced candidate enters the history as pending).
func (s *Service) persistCandidate(plan *GenerationPlan, c pipeline.Candidate) error {
	dir := filepath.Join(plan.PackagePath, identity.DirCandidates, c.ID)
	if err := pipeline.SaveCandidate(dir, c); err != nil {
		return err
	}
	return s.recordCandidateHistory(plan, c)
}

// recordCandidateHistory appends (or replaces) the candidate's history record
// in the identity package metadata (task 8.4).
func (s *Service) recordCandidateHistory(plan *GenerationPlan, c pipeline.Candidate) error {
	pkg, err := identity.Open(plan.PackagePath)
	if err != nil {
		return err
	}
	scoresData, err := json.Marshal(c.Scores)
	if err != nil {
		return err
	}
	rec := identity.CandidateHistoryRecord{
		ID:             c.ID,
		MotionID:       plan.MotionID,
		Direction:      c.Direction,
		CreatedAt:      c.CreatedAt.Format(time.RFC3339),
		Status:         identity.CandidatePending,
		Overall:        c.Scores.Overall,
		Scores:         scoresData,
		RegenerationOf: c.RegenerationOf,
	}
	return pkg.AppendCandidateHistory(rec)
}

// logGeneration appends a generation-family outcome to the append-only
// operation log (task 9.3: 生成/重新生成/镜像替换计入操作日志).
func (s *Service) logGeneration(plan *GenerationPlan, res *GenerationResult, action string) {
	pkg, err := identity.Open(plan.PackagePath)
	if err != nil {
		s.log.Warn("operation log: cannot open package", "package", plan.PackagePath, "error", err)
		return
	}
	candidateIDs := make([]string, 0, len(res.Results))
	for _, r := range res.Results {
		candidateIDs = append(candidateIDs, r.CandidateID)
	}
	if _, err := version.Append(pkg, action, map[string]any{
		"planId":       plan.ID,
		"kind":         plan.Kind,
		"motionId":     plan.MotionID,
		"direction":    planKindDirection(plan),
		"candidateIds": candidateIDs,
		"callsMade":    res.CallsMade,
		"attempts":     res.Attempts,
	}); err != nil {
		s.log.Warn("operation log append failed", "error", err)
	}
}

// planKindDirection returns the single direction of replace/regenerate plans
// (generate plans cover all basic directions; "" is fine for them).
func planKindDirection(plan *GenerationPlan) string {
	if len(plan.BasicLabels) == 1 {
		return plan.BasicLabels[0]
	}
	return ""
}

// candidatesFor returns the CandidateSet retained for a package, creating it
// on first use (CandidateSet 保留).
func (s *Service) candidatesFor(pkgPath string) *pipeline.CandidateSet {
	s.candMu.Lock()
	defer s.candMu.Unlock()
	cs, ok := s.candidates[pkgPath]
	if !ok {
		cs = pipeline.NewCandidateSet()
		s.candidates[pkgPath] = cs
	}
	return cs
}

// findCandidate looks up a retained candidate by id in a package's set,
// falling back to the persisted candidate directory (restart continuity: the
// in-memory CandidateSet only holds this session's candidates, but the
// candidate files survive restarts).
func (s *Service) findCandidate(pkgPath, id string) (pipeline.Candidate, error) {
	if id == "" {
		return pipeline.Candidate{}, fmt.Errorf("service: candidate id is required")
	}
	for _, c := range s.candidatesFor(pkgPath).All() {
		if c.ID == id {
			return c, nil
		}
	}
	if c, err := pipeline.LoadCandidate(filepath.Join(pkgPath, identity.DirCandidates, id)); err == nil {
		s.candidatesFor(pkgPath).Add(c)
		return c, nil
	}
	return pipeline.Candidate{}, fmt.Errorf("service: unknown candidate %q in package %s", id, pkgPath)
}

// motionFor loads the motion store, set and the plan's motion for a
// motion-driven plan; returns (nil, nil, nil, nil) for legacy direction-count
// plans.
func (s *Service) motionFor(plan *GenerationPlan) (*motion.Store, *motion.MotionSet, *motion.Motion, error) {
	if plan.MotionID == "" {
		return nil, nil, nil, nil
	}
	st := motion.NewStore(plan.PackagePath)
	ms, err := st.Load()
	if err != nil {
		return nil, nil, nil, err
	}
	mv, err := ms.Get(plan.MotionID)
	if err != nil {
		return nil, nil, nil, err
	}
	return st, ms, mv, nil
}

// loadOutboundMaterials reads the plan's outbound materials (外发素材) into
// provider request references.
func (s *Service) loadOutboundMaterials(plan *GenerationPlan) ([]provider.ReferenceImage, error) {
	refs := make([]provider.ReferenceImage, 0, len(plan.OutboundMaterials))
	for _, o := range plan.OutboundMaterials {
		data, err := os.ReadFile(o.Path)
		if err != nil {
			return nil, fmt.Errorf("service: read outbound material %q: %w", o.Path, err)
		}
		refs = append(refs, provider.ReferenceImage{Kind: o.Kind, Role: o.Role, MIME: mimeFor(o.Path), Data: data})
	}
	return refs, nil
}

// anchorsFromPkg converts the identity-level anchors into pipeline anchor
// points (the anchor-correction input for the filmstrip pipeline).
func anchorsFromPkg(pkg *identity.Package) []pipeline.AnchorPoint {
	m := pkg.Manifest()
	out := make([]pipeline.AnchorPoint, 0, len(m.Anchors))
	for _, a := range m.Anchors {
		out = append(out, pipeline.AnchorPoint{Name: a.Name, X: a.X, Y: a.Y})
	}
	return out
}

// sequenceFromCandidate builds the motion frame sequence of a direction from a
// generated candidate: one frame per pipeline frame, the per-frame corrected
// anchors attached, default rhythm.
func sequenceFromCandidate(c pipeline.Candidate) motion.FrameSequence {
	seq := motion.FrameSequence{Frames: make([]motion.Frame, len(c.Frames))}
	for i := range c.Frames {
		var anchors []pipeline.AnchorPoint
		if i < len(c.AnchorSets) {
			anchors = c.AnchorSets[i]
		}
		seq.Frames[i] = motion.Frame{
			Index:      i,
			AssetRef:   fmt.Sprintf("candidate:%s:frame:%d", c.ID, i),
			DurationMs: motion.DefaultFrameDurationMs,
			Anchors:    anchors,
		}
	}
	return seq
}

// failPlan marks the plan failed with the recorded reason and returns the
// failed result.
func (s *Service) failPlan(plan *GenerationPlan, res *GenerationResult, msg string) *GenerationResult {
	res.Status = PlanFailed
	res.Error = msg
	s.plans.setStatus(plan.ID, PlanFailed)
	s.log.Error("generation failed", "plan", plan.ID, "error", msg)
	return res
}

func mimeFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	default:
		return "image/png"
	}
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
