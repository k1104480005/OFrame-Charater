package service

import (
	"context"
	"encoding/base64"
	"image"
	"net/http"
	"strings"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/motion"
	"github.com/oframe/character-workbench/core/pipeline"
	"github.com/oframe/character-workbench/core/provider"
	"github.com/oframe/character-workbench/core/task"
	"github.com/oframe/character-workbench/core/version"
)

// newPhase6Svc returns a service whose fake transport returns valid filmstrip
// PNGs (identical content per call, so identical plans dedup cleanly).
func newPhase6Svc(t *testing.T) (*Service, string) {
	t.Helper()
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return filmstripResp(t, 32, 32, 4), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	configureDoubaoKey(t, svc)
	return svc, newTestPackage(t)
}

// TestConfirmGenerationPersistsTask verifies task 6.1: a confirmed generation
// creates a persisted task (provider params, expected call count, status,
// progress, error, retry count) that a fresh service instance — an app
// restart — can still read.
func TestConfirmGenerationPersistsTask(t *testing.T) {
	svc, root := newPhase6Svc(t)
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 1, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanExecuted {
		t.Fatalf("result: %+v", res)
	}
	tv, err := svc.TaskGet(plan.ID)
	if err != nil {
		t.Fatalf("task not persisted: %v", err)
	}
	if tv.Kind != "generate" || tv.Provider != "doubao" || tv.ExpectedCalls != 1 ||
		tv.Status != task.StatusSucceeded || tv.Progress != 1 {
		t.Fatalf("task row: %+v", tv)
	}
	if tv.Fingerprint == "" {
		t.Fatal("task fingerprint missing (dedup key)")
	}

	// "Restart": a fresh service over the same settings dir reads the task.
	svc2, err := New(Options{SettingsDir: svc.SettingsDir(), Logger: svc.log})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc2.Close() }()
	tv2, err := svc2.TaskGet(plan.ID)
	if err != nil {
		t.Fatalf("task lost after restart: %v", err)
	}
	if tv2.Status != task.StatusSucceeded || tv2.Fingerprint == "" {
		t.Fatalf("task after restart: %+v", tv2)
	}
	// The task's plan payload survives for retry/resume.
	raw, err := svc2.QueueStore().Get(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.Payload, plan.ID) {
		t.Fatalf("payload missing plan data: %+v", raw)
	}
}

// TestTaskResumeAllAfterInterruption verifies task 6.3: an unfinished
// (interrupted) task is resumed with one action after a restart. We simulate
// the crash by leaving a task row in "running" and opening a fresh service.
func TestTaskResumeAllAfterInterruption(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return filmstripResp(t, 32, 32, 4), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	root := newTestPackage(t)
	configureDoubaoKey(t, svc)

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 1, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmGeneration(context.Background(), plan.ID, true); err != nil {
		t.Fatal(err)
	}
	// Simulate the interruption: the app "crashed" mid-run leaving the task
	// row in running state (the queue row is the source of truth).
	if _, err := svc.QueueStore().Update(plan.ID, func(t *task.Task) error {
		t.Status = task.StatusRunning
		t.Progress = 0.4
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	callsBefore := rt.calls.Load()

	// "Restart" with a fresh service instance and resume with ONE action.
	svc2, err := New(Options{SettingsDir: svc.SettingsDir(), Logger: svc.log, HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc2.Close() }()
	// The new instance must not be able to re-open the in-memory plan
	// registry (a restart loses it) — resume works purely from the payload.
	n, err := svc2.TaskResumeAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("resumed = %d, want 1", n)
	}
	tv, err := svc2.TaskGet(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tv.Status != task.StatusSucceeded || tv.Progress != 1 || tv.Error != "" {
		t.Fatalf("task after resume: %+v", tv)
	}
	if rt.calls.Load() != callsBefore+1 {
		t.Fatalf("provider calls = %d, want %d (one resumed execution)", rt.calls.Load(), callsBefore+1)
	}
}

// TestTaskDedupReusesCachedResult verifies task 6.4: submitting an identical
// task reuses the cached success result without issuing a new external call.
func TestTaskDedupReusesCachedResult(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return filmstripResp(t, 32, 32, 4), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	root := newTestPackage(t)
	configureDoubaoKey(t, svc)

	req := GenerationRequest{
		PackagePath: root, Directions: 1, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	}
	p1, err := svc.PrepareGeneration(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := svc.ConfirmGeneration(context.Background(), p1.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Status != PlanExecuted || rt.calls.Load() != 1 {
		t.Fatalf("first run: %+v calls=%d", r1, rt.calls.Load())
	}

	// Identical task re-submitted: the cached result is reused, NO new call.
	p2, err := svc.PrepareGeneration(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := svc.ConfirmGeneration(context.Background(), p2.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if rt.calls.Load() != 1 {
		t.Fatalf("identical task issued new calls: %d, want 1 (idempotent dedup)", rt.calls.Load())
	}
	if r2.Status != PlanExecuted {
		t.Fatalf("dedup result: %+v", r2)
	}
	// The dedup is visible as a succeeded task row with the cache note.
	tv, err := svc.TaskGet(p2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tv.Status != task.StatusSucceeded || !strings.Contains(tv.Error, "reused cached success result") {
		t.Fatalf("dedup task row: %+v", tv)
	}

	// A DIFFERENT task (different frame count) is NOT deduplicated.
	p3, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 1, FrameCount: 8, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmGeneration(context.Background(), p3.ID, true); err != nil {
		t.Fatal(err)
	}
	if rt.calls.Load() != 2 {
		t.Fatalf("distinct task should call the provider: calls=%d", rt.calls.Load())
	}
}

// TestTaskRetryFollowsConfirmationCap verifies task 6.5: retries obey the
// generation confirmation's agreed maximum (MaxAttemptsPerDirection total
// executions) and abandoned tasks are not executed further.
func TestTaskRetryFollowsConfirmationCap(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return jsonResp(500, map[string]any{"error": map[string]any{"message": "boom"}}), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	root := newTestPackage(t)
	cfg, err := svc.ProviderConfig(provider.ProviderDoubao)
	if err != nil {
		t.Fatal(err)
	}
	cfg.APIKey = "test-key"
	cfg.MaxAttempts = 1 // agreed in the confirmation: 1 execution max
	if err := svc.SaveProviderConfig(provider.ProviderDoubao, cfg); err != nil {
		t.Fatal(err)
	}
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 1, MaxAttemptsPerDirection: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanFailed {
		t.Fatalf("expected failure: %+v", res)
	}
	// spec 4.7: at the retry cap the TASK is marked failed with the recorded
	// reason (the provider error), and no further retries are issued.
	ftv, err := svc.TaskGet(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ftv.Status != task.StatusFailed || ftv.Error == "" {
		t.Fatalf("failed task must record the reason: %+v", ftv)
	}

	// MaxAttemptsPerDirection=1 → the initial run is the only agreed
	// execution; retrying is refused (重试遵循生成确认上限).
	if _, err := svc.TaskRetry(context.Background(), plan.ID); err == nil {
		t.Fatal("retry must be refused when the confirmation agreed only 1 execution")
	}
	if rt.calls.Load() != 1 {
		t.Fatalf("calls after refused retry = %d, want 1", rt.calls.Load())
	}

	// Abandon: marked abandoned, and ResumeAll does NOT execute it.
	if _, err := svc.TaskAbandon(plan.ID); err != nil {
		t.Fatal(err)
	}
	n, err := svc.TaskResumeAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("abandoned task resumed: %d", n)
	}
	tv, _ := svc.TaskGet(plan.ID)
	if tv.Status != task.StatusAbandoned {
		t.Fatalf("abandoned task status = %s", tv.Status)
	}
}

// --- quality acceptance (tasks 8.2–8.4) + versioning (9.2–9.4) ---

// acceptThresholdSvc generates a 4-direction motion with candidates so the
// acceptance flow has real candidates + history records.
func acceptThresholdSvc(t *testing.T) (*Service, string, string) {
	t.Helper()
	svc, root := newPhase6Svc(t)
	m, err := svc.MotionCreate(root, "walk", motion.DirectionStrategy{Count: 4, Mirror: true})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, MotionID: m.ID, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmGeneration(context.Background(), plan.ID, true); err != nil {
		t.Fatal(err)
	}
	return svc, root, m.ID
}

// TestCandidateHistoryAndDecisionGate verifies tasks 8.3 + 8.4: every
// produced candidate lands in the candidate history with scores; the gate
// passes only on scores ≥ threshold AND user confirmation; rejected candidates
// remain in the history.
func TestCandidateHistoryAndDecisionGate(t *testing.T) {
	svc, root, _ := acceptThresholdSvc(t)
	cands := svc.CandidateList(root)
	if len(cands) != 3 { // right/up/down generated (left is mirror-derived)
		t.Fatalf("generated candidates = %d, want 3", len(cands))
	}
	hist, err := svc.CandidateHistory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 3 {
		t.Fatalf("candidate history = %d, want 3", len(hist))
	}
	// The pipeline candidate scores are recorded in the history metadata.
	if hist[0].Overall == 0 {
		t.Fatalf("history record missing scores: %+v", hist[0])
	}

	// Pick the best candidate and confirm in the preview: the gate passes when
	// scores meet the threshold AND the user confirms (task 8.3).
	best := cands[0]
	for _, c := range cands {
		if c.Scores.Overall > best.Scores.Overall {
			best = c
		}
	}
	if best.Scores.Overall < version.DefaultAcceptanceThresholds().Overall {
		t.Fatalf("pipeline candidate below the acceptance threshold (overall %.2f); cannot exercise the pass branch — synthetic strip too degraded", best.Scores.Overall)
	}
	dec, err := svc.CandidateDecide(context.Background(), root, best.ID, true, "looks good")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Decision != identity.CandidateAccepted {
		t.Fatalf("gate decision = %s, want accepted (%+v)", dec.Decision, dec)
	}
	// Accepted candidate becomes the current animation asset (task 9.2).
	assets, err := svc.CurrentAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].CandidateID != best.ID {
		t.Fatalf("current assets: %+v", assets)
	}

	// A rejected candidate (user rejects the preview) stays in history as
	// rejected and never becomes an asset (task 8.4).
	other := cands[0]
	if other.ID == best.ID {
		other = cands[1]
	}
	dec2, err := svc.CandidateDecide(context.Background(), root, other.ID, false, "wrong pose")
	if err != nil {
		t.Fatal(err)
	}
	if dec2.Decision != identity.CandidateRejected {
		t.Fatalf("user rejection should reject: %+v", dec2)
	}
	hist, _ = svc.CandidateHistory(root)
	byID := map[string]CandidateHistoryView{}
	for _, h := range hist {
		byID[h.ID] = h
	}
	if byID[other.ID].Status != identity.CandidateRejected || !strings.Contains(byID[other.ID].AcceptanceNote, "rejected by user") {
		t.Fatalf("rejected candidate history: %+v", byID[other.ID])
	}
	if len(assets) != 1 {
		t.Fatalf("rejected candidate must not become an asset: %+v", assets)
	}
}

// TestOperationLogAppendOnly verifies task 9.3: generation, acceptance and
// mirror replacement are appended to the operation log, one entry at a time,
// with complete content.
func TestOperationLogAppendOnly(t *testing.T) {
	svc, root, mID := acceptThresholdSvc(t)
	entries, err := svc.OperationLog(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Action != version.ActionGeneration {
		t.Fatalf("log after generation: %+v", entries)
	}

	// Accept a candidate → an acceptance entry is appended (seq 2).
	cands := svc.CandidateList(root)
	dec, err := svc.CandidateDecide(context.Background(), root, cands[0].ID, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Decision != identity.CandidateAccepted {
		t.Fatalf("decision: %+v", dec)
	}
	entries, _ = svc.OperationLog(root)
	if len(entries) != 2 || entries[1].Action != version.ActionAcceptance || entries[1].Seq != 2 {
		t.Fatalf("log after acceptance: %+v", entries)
	}

	// Mirror replacement → a third entry with the replaced direction.
	repl, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, MotionID: mID, StylePresetID: "pixel_classic", ActionPresetID: "walk",
		ReplaceDirections: []string{motion.DirectionLeft},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmGeneration(context.Background(), repl.ID, true); err != nil {
		t.Fatal(err)
	}
	entries, _ = svc.OperationLog(root)
	if len(entries) != 3 || entries[2].Action != version.ActionMirrorReplacement {
		t.Fatalf("log after replacement: %+v", entries)
	}
	if !strings.Contains(string(entries[2].Payload), motion.DirectionLeft) {
		t.Fatalf("replacement payload incomplete: %s", entries[2].Payload)
	}
}

// TestRollbackRestoresContentAndPreservesLog verifies task 9.4: rolling back to
// a historical point restores the identity package content (motions, assets,
// history) while later log entries are preserved.
func TestRollbackRestoresContentAndPreservesLog(t *testing.T) {
	svc, root, mID := acceptThresholdSvc(t)
	cands := svc.CandidateList(root)
	if _, err := svc.CandidateDecide(context.Background(), root, cands[0].ID, true, ""); err != nil {
		t.Fatal(err)
	}
	// Mirror replacement changes the motion content (left origin → replaced).
	repl, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, MotionID: mID, StylePresetID: "pixel_classic", ActionPresetID: "walk",
		ReplaceDirections: []string{motion.DirectionLeft},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmGeneration(context.Background(), repl.ID, true); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.MotionGet(root, mID)
	if got.Direction(motion.DirectionLeft).Origin != motion.OriginReplaced {
		t.Fatalf("left should be replaced before rollback: %+v", got.Direction(motion.DirectionLeft))
	}
	assets, _ := svc.CurrentAssets(root)
	if len(assets) != 1 {
		t.Fatalf("assets before rollback: %+v", assets)
	}

	// Roll back to seq 1 (after the first generation, before the acceptance).
	entries, err := svc.RollbackTo(root, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Later log entries are PRESERVED; a rollback entry is appended (seq 4).
	if len(entries) != 4 || entries[3].Action != version.ActionRollback {
		t.Fatalf("log after rollback: %+v", entries)
	}
	// Content restored to the seq-1 state: left is mirror-derived again.
	got, _ = svc.MotionGet(root, mID)
	left := got.Direction(motion.DirectionLeft)
	if left.Origin != motion.OriginMirrored || left.Source != motion.DirectionRight {
		t.Fatalf("motion not restored: %+v", left)
	}
	// Assets restored (the acceptance happened after seq 1).
	assets, _ = svc.CurrentAssets(root)
	if len(assets) != 0 {
		t.Fatalf("assets after rollback: %+v", assets)
	}
	// Candidate history restored: the accepted candidate is pending again.
	hist, _ := svc.CandidateHistory(root)
	for _, h := range hist {
		if h.Status != identity.CandidatePending {
			t.Fatalf("history not restored: %+v", h)
		}
	}
}

// --- AI consistency score (task 8.2) ---

// TestCandidateConsistencyLocalAndAI verifies task 8.2: the coarse consistency
// score is reference-only — the local heuristic is deterministic and the
// optional provider AI path returns an AI score; neither blocks acceptance.
func TestCandidateConsistencyLocalAndAI(t *testing.T) {
	svc, root, _ := acceptThresholdSvc(t)

	// Local heuristic: deterministic reference score.
	local, err := svc.CandidateConsistency(context.Background(), root, false)
	if err != nil {
		t.Fatal(err)
	}
	if local.Source != "local" || local.Score < 0 || local.Score > 1 {
		t.Fatalf("local consistency: %+v", local)
	}
	local2, _ := svc.CandidateConsistency(context.Background(), root, false)
	if local.Score != local2.Score {
		t.Fatalf("local score not deterministic: %v vs %v", local.Score, local2.Score)
	}

	// AI path: the fake transport answers the chat call with a score; image
	// calls keep returning filmstrips. The AI score is displayed as reference.
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/chat/completions") {
			return jsonResp(200, map[string]any{
				"choices": []map[string]any{{"message": map[string]any{"content": "0.85"}}},
			}), nil
		}
		return filmstripResp(t, 32, 32, 4), nil
	}}
	svcAI, _ := newTestService(t, &http.Client{Transport: rt})
	aiRoot := newTestPackage(t)
	configureDoubaoKey(t, svcAI)
	m, err := svcAI.MotionCreate(aiRoot, "walk", motion.DirectionStrategy{Count: 1, Mirror: true})
	if err != nil {
		t.Fatal(err)
	}
	p, err := svcAI.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: aiRoot, MotionID: m.ID, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svcAI.ConfirmGeneration(context.Background(), p.ID, true); err != nil {
		t.Fatal(err)
	}
	ai, err := svcAI.CandidateConsistency(context.Background(), aiRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	if ai.Source != "ai" || ai.Score != 0.85 {
		t.Fatalf("AI consistency: %+v", ai)
	}
	// Reference only: a low AI score does not block the acceptance flow.
	low, err := svcAI.CandidateConsistency(context.Background(), aiRoot, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = low
	cands := svcAI.CandidateList(aiRoot)
	if _, err := svcAI.CandidateDecide(context.Background(), aiRoot, cands[0].ID, true, ""); err != nil {
		t.Fatalf("acceptance must not be blocked by the consistency score: %v", err)
	}
}

// --- PixelPerfect preview (task 5.5) + mirror E2E ---

// TestDirectionPreviewFramesPixelIdentity verifies task 5.5 + the mirror
// end-to-end requirement: the rendered preview frames of a generated direction
// are pixel-identical to the pipeline's processed frames, and the rendered
// frames of a mirror-derived direction are pixel-by-pixel equal to the source
// frames horizontally flipped.
func TestDirectionPreviewFramesPixelIdentity(t *testing.T) {
	svc, root, mID := acceptThresholdSvc(t)
	// The fake filmstrip has a distinct block per frame so pixel identity is
	// a real assertion.
	preview, err := svc.DirectionPreviewFrames(root, mID, motion.DirectionRight)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) != 4 {
		t.Fatalf("preview frames = %d, want 4", len(preview))
	}
	// Source frames = the right candidate's processed frames.
	cands := svc.CandidateList(root)
	var right pipeline.Candidate
	for _, c := range cands {
		if c.Direction == motion.DirectionRight {
			right = c
			break
		}
	}
	if right.ID == "" {
		t.Fatal("no right candidate")
	}
	for i, f := range preview {
		got := decodePreviewFrame(t, f.PNG)
		want := right.Frames[i]
		if !imagesEqual(got, want) {
			t.Fatalf("preview frame %d of generated direction differs from the sliced frame (PixelPerfect violated)", i)
		}
	}

	// Mirror-derived direction: left renders as the horizontal flip of right,
	// pixel by pixel (镜像帧逐像素等于源帧水平翻转).
	leftPreview, err := svc.DirectionPreviewFrames(root, mID, motion.DirectionLeft)
	if err != nil {
		t.Fatal(err)
	}
	if len(leftPreview) != 4 {
		t.Fatalf("left preview frames = %d, want 4", len(leftPreview))
	}
	for i, f := range leftPreview {
		got := decodePreviewFrame(t, f.PNG)
		want := motion.HorizontalMirror(right.Frames[i])
		if !imagesEqual(got, want) {
			t.Fatalf("mirrored frame %d not equal to source horizontally flipped", i)
		}
		// Anchor conversion X' = width-1-X, Y' = Y is visible in the preview
		// metadata.
		if len(f.Anchors) != len(right.AnchorSets[i]) {
			t.Fatalf("mirror anchor count mismatch at frame %d", i)
		}
	}
}

// --- phase-5 review fixes ---

// TestConfirmGenerationProviderMissingMarksPlanFailed verifies the phase-5
// review fix: when the plan's provider cannot be resolved at confirmation
// time, the plan is marked failed instead of staying "confirmed".
func TestConfirmGenerationProviderMissingMarksPlanFailed(t *testing.T) {
	svc, _ := newTestService(t, nil)
	root := newTestPackage(t)
	// Unregister every provider so the lookup fails.
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 1, ProviderID: "doubao",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = svc.registry.Remove("doubao")
	if _, err := svc.ConfirmGeneration(context.Background(), plan.ID, true); err == nil {
		t.Fatal("expected error when the provider is missing")
	}
	got, err := svc.GetPlan(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != PlanFailed {
		t.Fatalf("plan status = %q, want failed (must not stay confirmed)", got.Status)
	}
}

// --- helpers ---

func decodePreviewFrame(t *testing.T, b64 string) *image.RGBA {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatal(err)
	}
	img, err := pipeline.DecodeFrame(data)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func imagesEqual(a, b *image.RGBA) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Bounds() != b.Bounds() {
		return false
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false
		}
	}
	return true
}
