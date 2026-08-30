package service

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
)

// TestPrepareGenerationUsesTaskDescriptionOverride verifies the task-level
// prompt override contract: a non-empty request Description replaces the saved
// identity description in the prompt snapshot (与基础角色路径同语义).
func TestPrepareGenerationUsesTaskDescriptionOverride(t *testing.T) {
	svc, _ := newTestService(t, nil)
	root := newTestPackage(t)

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 1, StylePresetID: "pixel", ActionPresetID: "walk",
		Description: "task override description",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Prompt.Description != "task override description" {
		t.Fatalf("prompt description = %q, want task override", plan.Prompt.Description)
	}

	fallback, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 1, StylePresetID: "pixel", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fallback.Prompt.Description != "a small green pixel hero" {
		t.Fatalf("prompt description = %q, want saved identity description", fallback.Prompt.Description)
	}
}

// TestPersistFailureFailsPlan verifies the no-fake-success contract: when the
// candidate cannot be persisted to the package, the plan FAILS with a readable
// reason instead of reporting success without any candidate artifact on disk.
func TestPersistFailureFailsPlan(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return filmstripResp(t, 32, 32, 4), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	configureDoubaoKey(t, svc)
	root := newTestPackage(t)

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 1, StylePresetID: "pixel", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Sabotage the candidate area: a regular FILE where the directory must be.
	if err := os.RemoveAll(filepath.Join(root, identity.DirCandidates)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, identity.DirCandidates), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanFailed || !strings.Contains(res.Error, "persist candidate") {
		t.Fatalf("result = %+v, want failed with persist candidate reason", res)
	}
	if cands := svc.CandidateList(root); len(cands) != 0 {
		t.Fatalf("candidates retained despite persist failure: %+v", cands)
	}
}

// TestRegenerateAttemptsCounted verifies regeneration reports the REAL attempt
// count (provider failed once, succeeded on retry) instead of a hardcoded 1 —
// result rows, logs and budget statistics must not under-report retries.
func TestRegenerateAttemptsCounted(t *testing.T) {
	var rt *fakeRT
	rt = &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		// Transport call #2 is the regeneration's FIRST attempt (the initial
		// 1-direction generation consumed call #1): fail it once, succeed after.
		if rt.calls.Load() == 2 {
			return jsonResp(500, map[string]any{"error": map[string]any{"message": "transient boom"}}), nil
		}
		return filmstripResp(t, 32, 32, 4), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	configureDoubaoKey(t, svc)
	root := newTestPackage(t)

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 1, StylePresetID: "pixel", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil || first.Status != PlanExecuted || len(first.Results) != 1 {
		t.Fatalf("initial generation: res=%+v err=%v", first, err)
	}
	prevID := first.Results[0].CandidateID

	regen, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, RegenerateOf: prevID, StylePresetID: "pixel", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	gres, err := svc.ConfirmGeneration(context.Background(), regen.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if gres.Status != PlanExecuted {
		t.Fatalf("regeneration result: %+v", gres)
	}
	if gres.Attempts != 2 || gres.Results[0].Attempts != 2 {
		t.Fatalf("regeneration attempts = %d/%d, want 2 (1 failure + 1 success)", gres.Attempts, gres.Results[0].Attempts)
	}
	if gres.CallsMade != 1 {
		t.Fatalf("regeneration callsMade = %d, want 1 (one billed call sequence)", gres.CallsMade)
	}
	if got := rt.calls.Load(); got != 3 {
		t.Fatalf("transport calls = %d, want 3 (1 initial + 2 regen)", got)
	}
}

// TestConfirmGenerationDecisionIsSingleShot verifies the confirmation gate is
// one-shot: after a decision (cancel here) the plan can never be confirmed
// again, so a duplicate submission cannot trigger a second external call.
func TestConfirmGenerationDecisionIsSingleShot(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return jsonResp(500, map[string]string{"error": "must not be called"}), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	configureDoubaoKey(t, svc)
	root := newTestPackage(t)

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{PackagePath: root, BaseCharacter: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmGeneration(context.Background(), plan.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmGeneration(context.Background(), plan.ID, true); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("second decision error = %v, want already-decided refusal", err)
	}
}

func candidatesOf(t *testing.T, root string) []identity.BaseCharacterCandidate {
	t.Helper()
	pkg, err := identity.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return pkg.BaseCharacterCandidates()
}

func artifactOf(t *testing.T, root, rel string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// multiColorPNG builds a 32x32 PNG carrying more distinct opaque colors than
// the retro16 palette cap, for quantization assertions.
func multiColorPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for i := 0; i < 20; i++ {
		c := color.RGBA{R: uint8(i * 12), G: uint8(255 - i*12), B: uint8(i * 5), A: 255}
		for y := i; y < 32; y += 20 {
			for x := i; x < 32; x += 20 {
				img.SetRGBA(x, y, c)
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func opaqueDistinctColors(t *testing.T, raw []byte) int {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[color.RGBA]struct{}{}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			seen[color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: uint8(a >> 8)}] = struct{}{}
		}
	}
	return len(seen)
}

// TestBaseCharacterQuantizesByStyle verifies the style→palette contract on the
// single-image base-character path: retro16 quantizes to ≤16 colors while
// chibi keeps the generated colors untouched.
func TestBaseCharacterQuantizesByStyle(t *testing.T) {
	payload := multiColorPNG(t)
	for _, tc := range []struct {
		style    string
		maxColor int
	}{
		{style: "retro16", maxColor: 16},
		{style: "chibi", maxColor: 1 << 30},
	} {
		t.Run(tc.style, func(t *testing.T) {
			rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
				return b64ImageResp(t, payload), nil
			}}
			svc, _ := newTestService(t, &http.Client{Transport: rt})
			configureDoubaoKey(t, svc)
			root := newTestPackage(t)

			plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
				PackagePath: root, BaseCharacter: true, StylePresetID: tc.style,
			})
			if err != nil {
				t.Fatal(err)
			}
			res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != PlanExecuted {
				t.Fatalf("result = %+v, want executed", res)
			}
			cands := candidatesOf(t, root)
			if len(cands) != 1 {
				t.Fatalf("candidates = %+v, want exactly one", cands)
			}
			raw := artifactOf(t, root, cands[0].ImagePath)
			if got := opaqueDistinctColors(t, raw); got > tc.maxColor {
				t.Fatalf("%s artifact has %d opaque colors, want <= %d", tc.style, got, tc.maxColor)
			}
		})
	}
}
