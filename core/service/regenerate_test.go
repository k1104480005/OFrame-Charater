package service

import (
	"context"
	"image"
	"image/color"
	"net/http"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/pipeline"
	"github.com/oframe/character-workbench/core/provider"
)

// TestRegenerateCandidateFollowsConfirmationGate verifies task 5.6 / filmstrip
// pipeline spec: regenerating a candidate is a NEW filmstrip generation that
// goes through PrepareGeneration (no provider call) and ConfirmGeneration
// (agreed expected call count), and RegenerateCandidate itself REFUSES to run
// against a plan that was not confirmed — the gate is enforced at the service
// seam, not just documented.
func TestRegenerateCandidateFollowsConfirmationGate(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return filmstripResp(t, 32, 32, 4), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	root := newTestPackage(t)

	// Doubao needs a key to reach the (fake) transport.
	cfg, err := svc.ProviderConfig(provider.ProviderDoubao)
	if err != nil {
		t.Fatal(err)
	}
	cfg.APIKey = "test-key"
	if err := svc.SaveProviderConfig(provider.ProviderDoubao, cfg); err != nil {
		t.Fatal(err)
	}

	// 1. Initial 4-direction generation through the confirmation gate:
	//    3 生成 + 1 镜像, 每方向最多 3 次总尝试. The wired pipeline produces a
	//    retained candidate per generated direction.
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 4, StylePresetID: "pixel", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rt.calls.Load() != 0 {
		t.Fatalf("PrepareGeneration issued %d provider calls, want 0", rt.calls.Load())
	}
	if plan.BasicDirections != 3 || plan.MirroredDirections != 1 ||
		plan.ExpectedCalls != 3 || plan.MaxAttemptsPerDirection != 3 || plan.MaxTotalAttempts != 9 {
		t.Fatalf("confirmation budget not as agreed: %+v", plan)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanExecuted || res.CallsMade != 3 || res.Attempts != 3 || len(res.Results) != 3 {
		t.Fatalf("confirmation result: %+v", res)
	}
	stats := svc.ProviderStats()
	if got := stats.ForProvider(provider.ProviderDoubao); len(got) != 1 || got[0].CallCount != 3 {
		t.Fatalf("call statistics after confirmation: %+v", got)
	}
	prevID := res.Results[0].CandidateID
	if prevID == "" {
		t.Fatal("first direction produced no candidate")
	}

	// 2. Prepare the regeneration plan: ONE new filmstrip call, and NO provider
	//    call at prepare time. The previous candidate must be retained.
	regen, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, RegenerateOf: prevID, StylePresetID: "pixel", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if regen.Kind != PlanKindRegenerate || regen.ExpectedCalls != 1 || regen.BasicDirections != 0 {
		t.Fatalf("regeneration plan: %+v", regen)
	}
	if rt.calls.Load() != 3 {
		t.Fatalf("PrepareGeneration(regenerate) issued provider calls: %d, want 3 (unchanged)", rt.calls.Load())
	}

	// 3. RegenerateCandidate against the NOT-YET-CONFIRMED plan must be refused
	//    and must make NO external call (the gate is enforced, spec 4.5).
	if _, _, err := svc.RegenerateCandidate(context.Background(), regen.ID); err == nil {
		t.Fatal("RegenerateCandidate before ConfirmGeneration must be refused")
	}
	if rt.calls.Load() != 3 {
		t.Fatalf("refused regeneration issued calls: %d, want 3 (no new external call)", rt.calls.Load())
	}

	// 4. Confirm the regeneration: executes exactly 1 call, produces a NEW
	//    candidate linked to the previous one.
	gres, err := svc.ConfirmGeneration(context.Background(), regen.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if gres.Status != PlanExecuted || gres.CallsMade != 1 || len(gres.Results) != 1 {
		t.Fatalf("regeneration result: %+v", gres)
	}
	if rt.calls.Load() != 4 {
		t.Fatalf("transport calls after regeneration = %d, want 4", rt.calls.Load())
	}
	newID := gres.Results[0].CandidateID
	if newID == "" || newID == prevID {
		t.Fatalf("regeneration candidate id = %q (prev %q)", newID, prevID)
	}
	// The new candidate is retained and linked to the previous one
	// (生成结果保留最佳候选而非空手返回).
	cands := svc.CandidateList(root)
	if len(cands) != 4 {
		t.Fatalf("retained candidates = %d, want 4", len(cands))
	}
	var linked bool
	for _, c := range cands {
		if c.ID == newID {
			linked = c.RegenerationOf == prevID
		}
	}
	if !linked {
		t.Fatalf("new candidate %s not linked to %s", newID, prevID)
	}
}

// TestRegenerateCandidateRejectsUnknownAndNonRegenPlans verifies the gate
// rejects unknown plans and plans that are not regeneration plans.
func TestRegenerateCandidateRejectsUnknownAndNonRegenPlans(t *testing.T) {
	svc, _ := newTestService(t, nil)
	root := newTestPackage(t)
	if _, _, err := svc.RegenerateCandidate(context.Background(), "nope"); err == nil {
		t.Fatal("unknown plan must be refused")
	}
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 1, StylePresetID: "pixel", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.RegenerateCandidate(context.Background(), plan.ID); err == nil {
		t.Fatal("non-regeneration plan must be refused by RegenerateCandidate")
	}
}

// buildRegenStrip builds a deterministic 4-frame filmstrip (32×32 per frame)
// with a magenta technical background and a colored block per frame — the
// stand-in for the filmstrip a confirmed generation would return.
func buildRegenStrip(t *testing.T, blocks [][2]int) (*image.RGBA, pipeline.FrameList) {
	t.Helper()
	if len(blocks) != 4 {
		t.Fatalf("buildRegenStrip needs 4 blocks, got %d", len(blocks))
	}
	canvas, err := identity.NewCanvasSpec(32, 32)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := pipeline.NormalizeFrameList(*canvas, 4)
	if err != nil {
		t.Fatal(err)
	}
	frames := make([]*image.RGBA, 0, 4)
	for _, b := range blocks {
		frames = append(frames, magentaBlockFrame(b[0], b[1]))
	}
	strip, err := pipeline.AssembleFilmstrip(frames, layout)
	if err != nil {
		t.Fatal(err)
	}
	return strip, layout
}

// magentaBlockFrame is a 32×32 magenta frame with a 10×10 opaque block at
// (bx, by); the magenta is the keying technical background (洋红仅用于抠图).
func magentaBlockFrame(bx, by int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 255, A: 255})
		}
	}
	for y := by; y < by+10 && y < 32; y++ {
		for x := bx; x < bx+10 && x < 32; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 120, G: 200, B: 80, A: 255})
		}
	}
	return img
}
