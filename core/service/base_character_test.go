package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/pipeline"
)

// b64ImageResp returns the Doubao-shaped response wrapping an arbitrary image
// payload ({"data":[{"b64_json": ...}]}), the same shape the filmstrip fakes
// use.
func b64ImageResp(t *testing.T, data []byte) *http.Response {
	t.Helper()
	return jsonResp(200, map[string]any{
		"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString(data)}},
	})
}

// TestBaseCharacterPlanWithoutProviders verifies the no-provider boundary
// (character-creation-workflow spec): with zero configured providers the plan
// is refused offline with a readable, actionable error and NO transport hit.
func TestBaseCharacterPlanWithoutProviders(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	svc, err := New(Options{
		SettingsDir: dir,
		HTTPClient:  &http.Client{Transport: &guardRT{}}, // panics on any request
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	root := newTestPackage(t)
	_, err = svc.PrepareGeneration(context.Background(), GenerationRequest{PackagePath: root, BaseCharacter: true})
	if err == nil || !strings.Contains(err.Error(), "尚未配置任何 Provider") {
		t.Fatalf("no-provider error = %v, want 尚未配置任何 Provider", err)
	}
}

// TestBaseCharacterPlanPreparesWithoutCalls verifies the base-character plan
// carries the single-image contract (1 call, 1 frame, character prompt,
// outbound materials) while performing ZERO external calls at prepare time.
func TestBaseCharacterPlanPreparesWithoutCalls(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return jsonResp(500, map[string]string{"error": "must not be called at prepare time"}), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	configureDoubaoKey(t, svc)
	root := newTestPackage(t)

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{PackagePath: root, BaseCharacter: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Kind != PlanKindBaseCharacter {
		t.Fatalf("plan kind = %q, want %q", plan.Kind, PlanKindBaseCharacter)
	}
	if plan.ExpectedCalls != 1 || plan.FrameCount != 1 || plan.Directions != 1 {
		t.Fatalf("single-image contract violated: %+v", plan)
	}
	if plan.MaxTotalAttempts != plan.MaxAttemptsPerDirection {
		t.Fatalf("budget = %d total attempts, want 1×%d", plan.MaxTotalAttempts, plan.MaxAttemptsPerDirection)
	}
	if !strings.Contains(plan.Prompt.Prompt, "a small green pixel hero") {
		t.Fatalf("prompt missing identity description: %q", plan.Prompt.Prompt)
	}
	if len(plan.OutboundMaterials) != 2 {
		t.Fatalf("outbound materials = %d, want 2 (main + auxiliary reference)", len(plan.OutboundMaterials))
	}
	if plan.Canvas.UnitWidth != 32 || plan.Canvas.UnitHeight != 32 {
		t.Fatalf("canvas = %dx%d, want 32x32", plan.Canvas.UnitWidth, plan.Canvas.UnitHeight)
	}
	if n := rt.calls.Load(); n != 0 {
		t.Fatalf("prepare made %d external calls, want 0", n)
	}
}

// TestBaseCharacterCancelMakesNoExternalCall verifies the confirmation gate:
// accepting=false aborts a base-character plan without any provider call and
// without recording any candidate.
func TestBaseCharacterCancelMakesNoExternalCall(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return jsonResp(500, map[string]string{"error": "cancelled plan must not be executed"}), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	configureDoubaoKey(t, svc)
	root := newTestPackage(t)

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{PackagePath: root, BaseCharacter: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Accepted || res.Status != PlanCancelled {
		t.Fatalf("cancel result = %+v, want cancelled", res)
	}
	if n := rt.calls.Load(); n != 0 {
		t.Fatalf("cancel made %d external calls, want 0", n)
	}
	pkg, err := identity.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := pkg.BaseCharacterCandidates(); len(got) != 0 {
		t.Fatalf("cancel recorded candidates: %+v", got)
	}
}

// TestBaseCharacterExecutesAndRecordsCandidate verifies the happy path: one
// provider call, a TRUE PNG artifact persisted under the candidate area, and a
// pending base-character candidate recorded in the manifest (adoption stays a
// separate explicit decision).
func TestBaseCharacterExecutesAndRecordsCandidate(t *testing.T) {
	pngBytes, err := pipeline.EncodeFilmstripPNG(blockFrame(32, 32, 10, 22))
	if err != nil {
		t.Fatal(err)
	}
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return b64ImageResp(t, pngBytes), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	configureDoubaoKey(t, svc)
	root := newTestPackage(t)

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{PackagePath: root, BaseCharacter: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanExecuted || res.CallsMade != 1 {
		t.Fatalf("execute result = %+v, want executed with 1 call", res)
	}
	if len(res.Results) != 1 || res.Results[0].CandidateID == "" {
		t.Fatalf("result missing candidate id: %+v", res.Results)
	}

	pkg, err := identity.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	candidates := pkg.BaseCharacterCandidates()
	if len(candidates) != 1 {
		t.Fatalf("manifest candidates = %+v, want exactly one", candidates)
	}
	cand := candidates[0]
	if cand.ID != res.Results[0].CandidateID || cand.Status != "pending" {
		t.Fatalf("candidate = %+v, want pending %s", cand, res.Results[0].CandidateID)
	}
	if cand.Provider != "doubao" || cand.Model != plan.Model {
		t.Fatalf("candidate provenance = %s/%s, want doubao/%s", cand.Provider, cand.Model, plan.Model)
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(cand.ImagePath)))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("persisted artifact is not a true PNG: %v", err)
	}
	if img.Bounds().Dx() != 32 || img.Bounds().Dy() != 32 {
		t.Fatalf("persisted artifact = %v, want 32x32", img.Bounds())
	}
}

// TestBaseCharacterReencodesJPEGToPNG verifies the format normalization
// contract: a provider returning JPEG/WebP-style bytes still produces a true
// PNG artifact on disk (never a renamed foreign-format payload), and the
// artifact is normalized to the identity's logical canvas size so candidates
// never drift from the canvas contract.
func TestBaseCharacterReencodesJPEGToPNG(t *testing.T) {
	// 夹具遵循洋红键控契约：满幅洋红技术底 + 中央角色块（JPEG 有损压缩后的
	// 洋红仍在键控容差内）。
	src := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 255, A: 255})
		}
	}
	for y := 3; y < 13; y++ {
		for x := 3; x < 13; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 200, G: 80, B: 40, A: 255})
		}
	}
	var jpegBuf bytes.Buffer
	if err := jpeg.Encode(&jpegBuf, src, nil); err != nil {
		t.Fatal(err)
	}
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return b64ImageResp(t, jpegBuf.Bytes()), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	configureDoubaoKey(t, svc)
	root := newTestPackage(t)

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{PackagePath: root, BaseCharacter: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanExecuted {
		t.Fatalf("jpeg input result = %+v, want executed", res)
	}
	pkg, err := identity.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	candidates := pkg.BaseCharacterCandidates()
	if len(candidates) != 1 {
		t.Fatalf("manifest candidates = %+v, want exactly one", candidates)
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(candidates[0].ImagePath)))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("jpeg input was not re-encoded as PNG: %v", err)
	}
	if img.Bounds().Dx() != 32 || img.Bounds().Dy() != 32 {
		t.Fatalf("re-encoded artifact = %v, want normalized to 32x32 logical canvas", img.Bounds())
	}
}

// TestBaseCharacterFailsOnGarbageWithoutCandidate verifies the failure path: an
// undecodable payload fails the plan, records the error, and leaves the
// manifest candidate list untouched.
func TestBaseCharacterFailsOnGarbageWithoutCandidate(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return b64ImageResp(t, []byte("this is not an image")), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	configureDoubaoKey(t, svc)
	root := newTestPackage(t)

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{PackagePath: root, BaseCharacter: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanFailed || res.Error == "" {
		t.Fatalf("garbage payload result = %+v, want failed with error", res)
	}
	got, err := svc.GetPlan(plan.ID)
	if err != nil || got.Status != PlanFailed {
		t.Fatalf("plan status = %+v err=%v, want failed", got, err)
	}
	pkg, err := identity.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if cands := pkg.BaseCharacterCandidates(); len(cands) != 0 {
		t.Fatalf("failed run recorded candidates: %+v", cands)
	}
	entries, err := os.ReadDir(filepath.Join(root, identity.DirCandidates))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "base-") {
			t.Fatalf("failed run left artifact behind: %s", e.Name())
		}
	}
}
