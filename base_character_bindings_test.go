// Binding-level acceptance of the base-character workflow
// (character-creation-workflow spec): prepare → cancel (zero external calls) →
// prepare → confirm (one call, candidate recorded) → preview → adopt →
// identity basis visible through IdentityGet.
package main

import (
	"net/http"
	"testing"

	"github.com/oframe/character-workbench/core/pipeline"
)

func TestBaseCharacterBindings(t *testing.T) {
	rt := newFakeRTForBaseCharacter(t)
	app, _ := newTestApp(t, fakeClient(rt.handler))

	// Give the seeded doubao a key so execution reaches the fake transport.
	cfg, err := app.ProviderConfigGet("doubao")
	if err != nil {
		t.Fatal(err)
	}
	cfg.APIKey = "test-key"
	if err := app.ProviderConfigSave("doubao", *cfg); err != nil {
		t.Fatal(err)
	}

	if _, err := app.PackageCreate("Hero", ""); err != nil {
		t.Fatal(err)
	}
	if err := app.IdentitySetDescription("a small green pixel hero"); err != nil {
		t.Fatal(err)
	}
	if err := app.IdentitySetCanvas(32, 32); err != nil {
		t.Fatal(err)
	}

	// Prepare: single-image plan contract, no external call.
	plan, err := app.GenerationPlanPrepare(GenerationRequestView{BaseCharacter: true})
	if err != nil {
		t.Fatalf("GenerationPlanPrepare: %v", err)
	}
	if plan.Kind != "base-character" || plan.ExpectedCalls != 1 || plan.Prompt.FrameCount != 1 {
		t.Fatalf("plan contract: %+v", plan)
	}
	if n := rt.calls.Load(); n != 0 {
		t.Fatalf("prepare made %d external calls, want 0", n)
	}

	// Cancel: no provider call, no candidate.
	cancelled, err := app.GenerationPlanConfirm(plan.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Accepted || cancelled.Status != "cancelled" {
		t.Fatalf("cancel result = %+v", cancelled)
	}
	if n := rt.calls.Load(); n != 0 {
		t.Fatalf("cancel made %d external calls, want 0", n)
	}

	// Confirm: exactly one call, candidate recorded and returned.
	plan2, err := app.GenerationPlanPrepare(GenerationRequestView{BaseCharacter: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := app.GenerationPlanConfirm(plan2.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "executed" || res.CallsMade != 1 {
		t.Fatalf("confirm result = %+v", res)
	}
	if len(res.Results) != 1 || res.Results[0].CandidateID == "" {
		t.Fatalf("result missing candidate id: %+v", res.Results)
	}

	// Preview: candidates endpoint returns the inline PNG.
	candidates, err := app.BaseCharacterCandidatesGet()
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].PNG == "" || candidates[0].Status != "pending" {
		t.Fatalf("candidates = %+v", candidates)
	}

	// Adopt: identity basis visible through IdentityGet.
	if err := app.BaseCharacterAdopt(candidates[0].ID); err != nil {
		t.Fatal(err)
	}
	view, err := app.IdentityGet()
	if err != nil {
		t.Fatal(err)
	}
	if view.BaseCharacterID != candidates[0].ID {
		t.Fatalf("identity base character = %q, want %q", view.BaseCharacterID, candidates[0].ID)
	}
}

// newFakeRTForBaseCharacter returns a transport whose counter is test-visible
// and whose handler serves one valid 32×32 PNG (the doubao b64_json shape).
func newFakeRTForBaseCharacter(t *testing.T) *fakeRT {
	t.Helper()
	pngBytes, err := pipeline.EncodeFilmstripPNG(blockOnMagentaFrame(10, 22))
	if err != nil {
		t.Fatal(err)
	}
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return b64ImagePNGResp(pngBytes), nil
	}}
	return rt
}
