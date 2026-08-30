package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/pipeline"
	"github.com/oframe/character-workbench/core/task"
)

// QueueFileName is the local SQLite task queue database file, stored next to
// the settings file so it survives app restarts (task 6.1: 本地持久化).
const QueueFileName = "queue.db"

// QueueStore exposes the persisted task queue store (used by the GUI drawer
// bindings and by tests to simulate interrupted tasks).
func (s *Service) QueueStore() *task.Store { return s.queueStore }

// SetTasksChangedHook installs the task:changed event hook (the GUI task
// drawer stays live; nil-safe in headless tests).
func (s *Service) SetTasksChangedHook(fn func()) { s.queueStore.SetOnChange(fn) }

// TaskView is the persisted task row exposed to the drawer. It carries the
// same shape as the legacy in-memory TaskSummary plus provider/expected-call
// info (tasks spec 6.1).
type TaskView struct {
	ID            string  `json:"id"`
	Kind          string  `json:"kind"`
	Status        string  `json:"status"`
	Progress      float64 `json:"progress"`
	Error         string  `json:"error"`
	RetryCount    int     `json:"retryCount"`
	Provider      string  `json:"provider"`
	ExpectedCalls int     `json:"expectedCalls"`
	Fingerprint   string  `json:"fingerprint,omitempty"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}

func taskView(t task.Task) TaskView {
	return TaskView{
		ID: t.ID, Kind: t.Kind, Status: t.Status, Progress: t.Progress,
		Error: t.Error, RetryCount: t.RetryCount, Provider: t.Provider,
		ExpectedCalls: t.ExpectedCalls, Fingerprint: t.Fingerprint,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

// TaskList returns all persisted tasks for the global task drawer.
func (s *Service) TaskList() ([]TaskView, error) {
	all, err := s.queueStore.List()
	if err != nil {
		return nil, err
	}
	out := make([]TaskView, 0, len(all))
	for _, t := range all {
		out = append(out, taskView(t))
	}
	return out, nil
}

// TaskGet returns one persisted task (失败可查看原因).
func (s *Service) TaskGet(id string) (TaskView, error) {
	t, err := s.queueStore.Get(id)
	if err != nil {
		return TaskView{}, err
	}
	return taskView(t), nil
}

// TaskRetry re-queues and immediately re-executes a failed or abandoned task
// (task 6.5: 失败可查看原因、重试或放弃). Retries obey the generation
// confirmation's agreed maximum: a task may be executed at most
// MaxAttemptsPerDirection times in total (1 initial run + retries), so a
// retry is refused once RetryCount reaches the agreed per-direction attempt
// cap minus one.
func (s *Service) TaskRetry(ctx context.Context, id string) (TaskView, error) {
	t, err := s.queueStore.Get(id)
	if err != nil {
		return TaskView{}, err
	}
	if !t.IsRetryable() {
		return TaskView{}, fmt.Errorf("service: task %s is not retryable in status %s", id, t.Status)
	}
	plan, err := decodePlanPayload(t)
	if err != nil {
		return TaskView{}, err
	}
	cap := plan.MaxAttemptsPerDirection - 1
	if cap < 0 {
		cap = 0
	}
	if t.RetryCount >= cap {
		return TaskView{}, fmt.Errorf(
			"service: task %s retry refused: retry cap %d reached (generation confirmation agreed max %d attempts per direction)",
			id, t.RetryCount, plan.MaxAttemptsPerDirection)
	}
	rt, err := s.queueStore.Retry(id)
	if err != nil {
		return TaskView{}, err
	}
	s.log.Info("task retried", "id", id, "retryCount", rt.RetryCount)
	s.executeTask(ctx, rt)
	got, err := s.queueStore.Get(id)
	if err != nil {
		return TaskView{}, err
	}
	return taskView(got), nil
}

// TaskAbandon marks a failed task abandoned; it is not executed further
// (task 6.5: 放弃后不再执行).
func (s *Service) TaskAbandon(id string) (TaskView, error) {
	ab, err := s.queueStore.Abandon(id)
	if err != nil {
		return TaskView{}, err
	}
	s.log.Info("task abandoned", "id", id)
	return taskView(ab), nil
}

// TaskResumeAll resumes every unfinished task (queued/running) with one action
// (task 6.3: 中断（崩溃/关机/网络失败）后重启可一键续跑未完成任务). Returns the
// number of resumed tasks.
func (s *Service) TaskResumeAll(ctx context.Context) (int, error) {
	unf, err := s.queueStore.Unfinished()
	if err != nil {
		return 0, err
	}
	for _, t := range unf {
		s.executeTask(ctx, t)
	}
	s.log.Info("unfinished tasks resumed", "count", len(unf))
	return len(unf), nil
}

// createTaskForPlan persists a queued task for a confirmed plan (the plan is
// the payload so retry/resume can re-execute it after a restart). The task id
// equals the plan id (one confirmed plan = one task).
func (s *Service) createTaskForPlan(plan *GenerationPlan, fp string) (task.Task, error) {
	payload, err := task.MarshalPayload(plan)
	if err != nil {
		return task.Task{}, err
	}
	providerParams, err := task.MarshalPayload(map[string]any{
		"providerId": plan.ProviderID,
		"model":      plan.Model,
		"prompt":     plan.Prompt,
	})
	if err != nil {
		return task.Task{}, err
	}
	return s.queueStore.Create(task.Task{
		ID:             plan.ID,
		Kind:           plan.Kind,
		Provider:       plan.ProviderID,
		ProviderParams: string(providerParams),
		ExpectedCalls:  plan.ExpectedCalls,
		Status:         task.StatusQueued,
		Payload:        payload,
		Fingerprint:    fp,
	})
}

// executeTask runs one persisted task to completion (or failure), updating the
// task row as progress advances (task 6.2: 状态与进度展示). The task must have
// a decodable plan payload.
func (s *Service) executeTask(ctx context.Context, t task.Task) {
	plan, err := decodePlanPayload(t)
	if err != nil {
		s.queueStore.Update(t.ID, func(tt *task.Task) error {
			tt.Status = task.StatusFailed
			tt.Error = fmt.Sprintf("task payload unreadable: %v", err)
			return nil
		})
		return
	}
	s.queueStore.Update(t.ID, func(tt *task.Task) error {
		tt.Status = task.StatusRunning
		tt.Progress = 0
		return nil
	})
	done := func(completed, total int) {
		if total <= 0 {
			return
		}
		p := float64(completed) / float64(total)
		if p > 1 {
			p = 1
		}
		s.queueStore.Update(t.ID, func(tt *task.Task) error {
			tt.Progress = p
			return nil
		})
	}
	res := s.executePlanTask(ctx, plan, done)
	switch res.Status {
	case PlanExecuted:
		data, err := json.Marshal(res)
		if err != nil {
			data = []byte(`{"status":"executed"}`)
		}
		if planFingerprint(plan) != "" {
			_ = s.queueStore.CachePut(planFingerprint(plan), string(data))
		}
		s.queueStore.Update(t.ID, func(tt *task.Task) error {
			tt.Status = task.StatusSucceeded
			tt.Progress = 1
			tt.Result = string(data)
			return nil
		})
		s.log.Info("task succeeded", "id", t.ID, "kind", t.Kind)
	case PlanFailed:
		s.queueStore.Update(t.ID, func(tt *task.Task) error {
			tt.Status = task.StatusFailed
			tt.Error = res.Error
			return nil
		})
		s.log.Warn("task failed", "id", t.ID, "error", res.Error)
	}
}

// executePlanTask runs a confirmed plan through the execution chain with a
// progress callback (completed/total directions).
func (s *Service) executePlanTask(ctx context.Context, plan *GenerationPlan, progress func(completed, total int)) *GenerationResult {
	prov, err := s.registry.Get(plan.ProviderID)
	if err != nil {
		return s.failPlan(plan, &GenerationResult{PlanID: plan.ID, Accepted: true, Status: PlanExecuted}, fmt.Sprintf("provider %s unavailable: %v", plan.ProviderID, err))
	}
	cfg := s.settings.ProviderSettings().ConfigFor(plan.ProviderID)
	refs, err := s.loadOutboundMaterials(plan)
	if err != nil {
		return s.failPlan(plan, &GenerationResult{PlanID: plan.ID, Accepted: true, Status: PlanExecuted}, err.Error())
	}
	switch plan.Kind {
	case PlanKindBaseCharacter:
		return s.runBaseCharacter(ctx, plan, prov, cfg, refs, progress)
	case PlanKindRegenerate:
		return s.runRegeneration(ctx, plan, prov, cfg, refs)
	case PlanKindReplace:
		return s.runReplacement(ctx, plan, prov, cfg, refs, progress)
	default:
		return s.runGeneration(ctx, plan, prov, cfg, refs, progress)
	}
}

// decodePlanPayload reconstructs the generation plan persisted as a task
// payload (persistent session: retry/resume re-execute from the payload).
func decodePlanPayload(t task.Task) (*GenerationPlan, error) {
	if t.Payload == "" {
		return nil, fmt.Errorf("service: task %s has no plan payload", t.ID)
	}
	var plan GenerationPlan
	if err := json.Unmarshal([]byte(t.Payload), &plan); err != nil {
		return nil, fmt.Errorf("service: task %s plan payload unreadable: %w", t.ID, err)
	}
	if plan.ID == "" || plan.PackagePath == "" {
		return nil, fmt.Errorf("service: task %s plan payload incomplete", t.ID)
	}
	return &plan, nil
}

// planFingerprint is the deterministic dedup key of a plan (task 6.4: 幂等去重
// 成功结果缓存): identical plans share a fingerprint, so a re-submitted
// identical task reuses the cached success result without a new external call.
// Volatile snapshot fields (BuiltAt, status, ids) are excluded, and nil slices
// are normalized to empty so a JSON round-trip (payload persistence) never
// changes the fingerprint (omitempty drops empty slices on marshal, which
// decodes back as nil).
func planFingerprint(plan *GenerationPlan) string {
	prompt := plan.Prompt
	prompt.BuiltAt = time.Time{}
	anchors := plan.Anchors
	if anchors == nil {
		anchors = []pipeline.AnchorPoint{}
	}
	basic := plan.BasicLabels
	if basic == nil {
		basic = []string{}
	}
	mirrored := plan.MirroredLabels
	if mirrored == nil {
		mirrored = []string{}
	}
	canonical := struct {
		Kind                    string
		PackagePath             string
		MotionID                string
		ProviderID              string
		Model                   string
		Directions              int
		ExpectedCalls           int
		MaxAttemptsPerDirection int
		BasicLabels             []string
		MirroredLabels          []string
		RegenerateOf            string
		Prompt                  pipeline.PromptSnapshot
		Canvas                  identity.CanvasSpec
		FrameCount              int
		Anchors                 []pipeline.AnchorPoint
	}{
		Kind: plan.Kind, PackagePath: plan.PackagePath, MotionID: plan.MotionID,
		ProviderID: plan.ProviderID, Model: plan.Model, Directions: plan.Directions,
		ExpectedCalls: plan.ExpectedCalls, MaxAttemptsPerDirection: plan.MaxAttemptsPerDirection,
		BasicLabels: basic, MirroredLabels: mirrored,
		RegenerateOf: plan.RegenerateOf,
		Prompt:       prompt, Canvas: plan.Canvas, FrameCount: plan.FrameCount,
		Anchors: anchors,
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}
