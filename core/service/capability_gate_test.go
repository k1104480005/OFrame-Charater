package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/oframe/character-workbench/core/motion"
	"github.com/oframe/character-workbench/core/provider"
)

// guardRT is a transport that PANICS on any request: every zero-outbound-call
// assertion below relies on this transport never being reached — a panicking
// RoundTrip inside a passing test is the strongest possible "no external call"
// proof.
type guardRT struct{}

func (g *guardRT) RoundTrip(r *http.Request) (*http.Response, error) {
	panic("OUTBOUND CALL DURING ZERO-CALL TEST: " + r.URL.String())
}

// newGuardService builds a service whose HTTP transport panics on use.
func newGuardService(t *testing.T) *Service {
	t.Helper()
	svc, _ := newTestService(t, &http.Client{Transport: &guardRT{}})
	return svc
}

// TestValidateVideoGenerationUnsupportedBeforeAnyCall covers task 1.4's
// explicit video entry point: until a video pipeline exists every configuration
// — including Doubao with its preset Seedance video model — returns
// ErrCapabilityUnsupported without any network activity.
func TestValidateVideoGenerationUnsupportedBeforeAnyCall(t *testing.T) {
	svc := newGuardService(t)
	for _, id := range []string{provider.ProviderDoubao, provider.ProviderOpenAI, provider.ProviderAgnes} {
		err := svc.ValidateVideoGeneration(id)
		if !errors.Is(err, provider.ErrCapabilityUnsupported) {
			t.Errorf("provider %s video gate = %v, want ErrCapabilityUnsupported", id, err)
		}
	}
	// Unknown providers fail structurally (offline), never as capability errors.
	if err := svc.ValidateVideoGeneration("no-such-provider"); err == nil || errors.Is(err, provider.ErrCapabilityUnsupported) {
		t.Errorf("unknown provider video check = %v, want a structural not-found error", err)
	}
}

// TestImageTextValidationEntries covers the shared validation surface the UI/CLI
// call BEFORE any external action: image membership, text catalogs and the
// exact sentinel identities — all with a panicking transport.
func TestImageTextValidationEntries(t *testing.T) {
	svc := newGuardService(t)

	// Image: preset default and member model pass; stranger model rejected.
	if err := svc.ValidateImageGeneration(provider.ProviderDoubao, ""); err != nil {
		t.Fatalf("doubao default image model must validate: %v", err)
	}
	if err := svc.ValidateImageGeneration(provider.ProviderDoubao, "ghost-image-model"); !errors.Is(err, provider.ErrModelInvalid) {
		t.Fatalf("unknown image model = %v, want ErrModelInvalid", err)
	}
	if !strings.Contains(errString(svc.ValidateImageGeneration(provider.ProviderDoubao, "ghost-image-model")), "ghost-image-model") {
		t.Error("readable error should echo the offending model name")
	}

	// Text: doubao default passes; image-only built-ins are unsupported.
	if err := svc.ValidateTextGeneration(provider.ProviderDoubao, ""); err != nil {
		t.Fatalf("doubao text default must validate: %v", err)
	}
	if err := svc.ValidateTextGeneration(provider.ProviderOpenAI, ""); !errors.Is(err, provider.ErrCapabilityUnsupported) {
		t.Fatalf("openai text = %v, want ErrCapabilityUnsupported", err)
	}

	// Custom compatible provider WITH an image model but NO text models:
	// image selection resolves, text reports model-not-configured (with or
	// without an explicit name — nothing is verifiable against an empty catalog).
	info, err := svc.ProviderAdd(provider.ProviderConfig{Name: "Img Only", Model: "img-m1", BaseURL: "https://x.example.com/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ValidateImageGeneration(info.ID, ""); err != nil {
		t.Fatalf("configured image model must validate: %v", err)
	}
	if err := svc.ValidateImageGeneration(info.ID, "m999"); !errors.Is(err, provider.ErrModelInvalid) {
		t.Fatalf("stray image model on filled catalog = %v, want ErrModelInvalid", err)
	}
	for _, attempt := range []string{"", "some-chat-model"} {
		err := svc.ValidateTextGeneration(info.ID, attempt)
		if !errors.Is(err, provider.ErrModelNotConfigured) {
			t.Fatalf("empty text catalog (model=%q) = %v, want ErrModelNotConfigured", attempt, err)
		}
	}
}

// TestPrepareGenerationRejectsUnmatchedModelOffline proves the prepare-time
// capability gate: an image request whose model is absent from the provider's
// catalog is refused BEFORE the plan is stored and before ANY external call.
func TestPrepareGenerationRejectsUnmatchedModelOffline(t *testing.T) {
	svc := newGuardService(t)
	root := newTestPackage(t)
	_, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root,
		Model:       "unknown-image-model",
	})
	if !errors.Is(err, provider.ErrModelInvalid) {
		t.Fatalf("prepare err = %v, want ErrModelInvalid", err)
	}
	assertNoPlansStored(t, svc)

	// A reserved VIDEO model cannot sneak into the image pipeline either:
	// Doubao carries seedance in its video catalog, and that catalog never
	// makes it a legal IMAGE selection.
	videoModel := provider.DefaultConfig(provider.ProviderDoubao).VideoModels[0]
	_, err = svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root,
		Model:       videoModel,
	})
	if !errors.Is(err, provider.ErrModelInvalid) {
		t.Fatalf("video model %q in image pipeline = %v, want ErrModelInvalid", videoModel, err)
	}
	assertNoPlansStored(t, svc)
}

// assertNoPlansStored fails when the service holds prepared plans (a rejected
// request must leave nothing behind).
func assertNoPlansStored(t *testing.T, svc *Service) {
	t.Helper()
	svc.plans.mu.Lock()
	defer svc.plans.mu.Unlock()
	if n := len(svc.plans.plans); n != 0 {
		t.Fatalf("%d plans stored after rejection, want 0", n)
	}
}

// TestConfirmGenerationDriftBlockedBeforeExternalCall verifies the
// confirmation-time re-validation (task 1.4 + generation spec: 选择固定、执行不漂移):
// when the saved catalog changes between Prepare and Confirm so that the fixed
// plan model no longer belongs to it, confirming FAILS OFFLINE and marks the
// plan failed instead of issuing calls.
func TestConfirmGenerationDriftBlockedBeforeExternalCall(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return filmstripResp(t, 32, 32, 4), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	root := newTestPackage(t)
	m, err := svc.MotionCreate(root, "walk", motion.DirectionStrategy{Count: 1, Mirror: false})
	if err != nil {
		t.Fatal(err)
	}
	configureDoubaoKey(t, svc)
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, MotionID: m.ID, Directions: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	baseCalls := rt.calls.Load()

	// Drift the catalog behind the plan's back: Doubao now only knows another
	// image model (array catalogs take precedence over the legacy singular
	// field), so the confirmed plan's model left its effective catalog.
	cfg, err := svc.ProviderConfig(provider.ProviderDoubao)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Model = "drifted-away-model"
	cfg.ImageModels = []string{"drifted-away-model"}
	if err := svc.SaveProviderConfig(provider.ProviderDoubao, cfg); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.ConfirmGeneration(context.Background(), plan.ID, true); !errors.Is(err, provider.ErrModelInvalid) {
		t.Fatalf("confirm err = %v, want ErrModelInvalid surfaced", err)
	}
	if got := rt.calls.Load(); got != baseCalls {
		t.Fatalf("drifted confirm issued %d provider calls, want 0", got-baseCalls)
	}
	if p, _ := svc.plans.get(plan.ID); p.Status != PlanFailed {
		t.Fatalf("plan status after drift refusal = %s, want failed", p.Status)
	}
}

// TestCandidateConsistencyTextGateCoversTask14 wires the TEXT boundary into its
// real entry point: the AI consistency path validates the text capability/model
// pre-call — missing text model → local heuristic with zero transport hits;
// valid text model → exactly one chat call reaches the fake transport.
func TestCandidateConsistencyTextGateCoversTask14(t *testing.T) {
	// (a) custom provider with NO text model, active: gated locally, offline.
	// The key is present so the check reaches the model gate (after the earlier
	// key check) instead of stopping there.
	guard := newGuardService(t)
	pkgGuard := newTestPackage(t)
	info, err := guard.ProviderAdd(provider.ProviderConfig{Name: "Img Only", Model: "img-x", BaseURL: "https://g.example.com/v1", APIKey: "k-img-only"})
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.SetActiveProvider(info.ID); err != nil {
		t.Fatal(err)
	}
	view, err := guard.CandidateConsistency(context.Background(), pkgGuard, true)
	if err != nil {
		t.Fatal(err)
	}
	if view.Source != "local" || !strings.Contains(view.Detail, "text capability/model unavailable") {
		t.Fatalf("AI score should degrade to local on missing text model: %+v", view)
	}

	// (b) valid text model: the gate lets exactly one chat/completions through.
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/chat/completions") {
			return jsonResp(200, map[string]any{
				"choices": []map[string]any{{"message": map[string]any{"content": "0.9"}}},
			}), nil
		}
		return filmstripResp(t, 32, 32, 4), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	root := newTestPackage(t)
	m, err := svc.MotionCreate(root, "walk", motion.DirectionStrategy{Count: 1, Mirror: false})
	if err != nil {
		t.Fatal(err)
	}
	configureDoubaoKey(t, svc)
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, MotionID: m.ID, Directions: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmGeneration(context.Background(), plan.ID, true); err != nil {
		t.Fatal(err)
	}
	before := rt.calls.Load()
	ai, err := svc.CandidateConsistency(context.Background(), root, true)
	if err != nil {
		t.Fatal(err)
	}
	if ai.Source != "ai" || ai.Score < 0.89 || ai.Score > 0.91 {
		t.Fatalf("valid text model AI score: %+v", ai)
	}
	if got := rt.calls.Load() - before; got != 1 {
		t.Fatalf("text gate let %d extra calls through, want exactly 1 chat call", got)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
