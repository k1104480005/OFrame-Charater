package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// TestGenerationRetriesWithinBudgetOnPipelineFailure verifies that a filmstrip
// pipeline failure (模型返回全幅不透明图、不符合条带布局契约 —— 实测 agnes 的
// 待机/行走返回) consumes the agreed attempt budget and retries: garbage,
// garbage, then a valid strip → the direction succeeds with 3 recorded
// attempts and exactly one billed call sequence.
func TestGenerationRetriesWithinBudgetOnPipelineFailure(t *testing.T) {
	var calls int64
	rt := &fakeRT{}
	rt.handler = func(r *http.Request) (*http.Response, error) {
		if atomic.AddInt64(&calls, 1) <= 2 {
			return fullBleedResp(), nil
		}
		return filmstripResp(t, 32, 32, 4), nil
	}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	configureDoubaoKey(t, svc)
	root := newTestPackage(t)

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 1, StylePresetID: "pixel", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanExecuted || res.CallsMade != 1 {
		t.Fatalf("result: %+v", res)
	}
	if res.Attempts != 3 || len(res.Results) != 1 || res.Results[0].Attempts != 3 {
		t.Fatalf("attempts = %d/%+v, want 3 (2 pipeline failures + 1 success)", res.Attempts, res.Results)
	}
}

// TestGenerationPipelineFailureAtBudgetCap verifies the failure side: a model
// that NEVER returns a compliant strip fails after the budget is exhausted,
// keeps exactly one retained failed candidate, and reports the pipeline reason.
func TestGenerationPipelineFailureAtBudgetCap(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return fullBleedResp(), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	configureDoubaoKey(t, svc)
	root := newTestPackage(t)

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 1, StylePresetID: "pixel", ActionPresetID: "walk",
		MaxAttemptsPerDirection: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanFailed || res.CallsMade != 0 {
		t.Fatalf("result: %+v, want failed without executed calls", res)
	}
	// 2 次调用 = 尝试预算耗尽（2 次全幅不透明返回，重试后仍失败）。
	if rt.calls.Load() != 2 {
		t.Fatalf("transport calls = %d, want 2 (budget exhausted)", rt.calls.Load())
	}
	if !strings.Contains(res.Error, "filmstrip pipeline") {
		t.Fatalf("error = %q, want pipeline reason", res.Error)
	}
}

// TestGenerationAcceptsJPEGFilmstrip verifies filmstrip format normalization:
// providers like Doubao/Seedream return JPEG bytes — the pipeline must decode
// them (DecodeImageAny) instead of failing with "not a PNG file".
func TestGenerationAcceptsJPEGFilmstrip(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return b64ImageResp(t, jpegStrip(t)), nil
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
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanExecuted || res.CallsMade != 1 {
		t.Fatalf("jpeg filmstrip result = %+v, want executed", res)
	}
}

// jpegStrip builds a magenta-background 4-pose strip and encodes it as JPEG
// (the observed Doubao/Seedream response format).
func jpegStrip(t *testing.T) []byte {
	t.Helper()
	strip := image.NewRGBA(image.Rect(0, 0, 128, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 128; x++ {
			strip.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 255, A: 255})
		}
	}
	for i := 0; i < 4; i++ {
		for y := 8; y < 24; y++ {
			for x := i*32 + 8; x < i*32+24; x++ {
				strip.SetRGBA(x, y, color.RGBA{R: 120, G: 200, B: 80, A: 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, strip, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fullBleedResp returns a fully opaque square PNG (no transparency, no strip
// band) — the observed model failure mode.
func fullBleedResp() *http.Response {
	img := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 60, G: 120, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return jsonResp(200, map[string]any{
		"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString(buf.Bytes())}},
	})
}
