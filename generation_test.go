package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/pipeline"
)

// fakeRT is an in-process transport; binding tests never call real paid
// services.
type fakeRT struct {
	handler func(r *http.Request) (*http.Response, error)
	calls   atomic.Int64
}

func (f *fakeRT) RoundTrip(r *http.Request) (*http.Response, error) {
	f.calls.Add(1)
	return f.handler(r)
}

func fakeClient(h func(r *http.Request) (*http.Response, error)) *http.Client {
	return &http.Client{Transport: &fakeRT{handler: h}}
}

func b64PNGResp() *http.Response {
	data, _ := json.Marshal(map[string]any{
		"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString([]byte("PNG"))}},
	})
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

// filmstripPNGResp builds a VALID PNG filmstrip response for the wired
// filmstrip pipeline (阶段 5: ConfirmGeneration 解码 provider 字节并跑管线): 4
// frames of 32×32 with magenta technical background and one opaque block each.
func filmstripPNGResp(t *testing.T) *http.Response {
	t.Helper()
	canvas, err := identity.NewCanvasSpec(32, 32)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := pipeline.NormalizeFrameList(*canvas, 4)
	if err != nil {
		t.Fatal(err)
	}
	frames := make([]*image.RGBA, 4)
	for i := range frames {
		frames[i] = blockOnMagentaFrame(10+(i%3), 22)
	}
	strip, err := pipeline.AssembleFilmstrip(frames, layout)
	if err != nil {
		t.Fatal(err)
	}
	data, err := pipeline.EncodeFilmstripPNG(strip)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{
		"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString(data)}},
	})
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(payload)),
	}
}

// blockOnMagentaFrame is a 32×32 magenta frame (洋红仅用于抠图) with a 10×10
// opaque block at (bx, by).
func blockOnMagentaFrame(bx, by int) *image.RGBA {
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

// TestProviderBindings covers provider 配置/验证/切换/统计 through the GUI
// bindings (which delegate to the shared application service).
func TestProviderBindings(t *testing.T) {
	app, _ := newTestApp(t, nil)

	// List: three built-in providers, Doubao active by default.
	list, err := app.ProviderList()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("providers = %d, want 3", len(list))
	}
	byID := map[string]ProviderInfoView{}
	for _, p := range list {
		byID[p.ID] = p
	}
	if !byID["doubao"].Active {
		t.Fatal("doubao should be active by default")
	}
	if byID["doubao"].ImageModel == "" || byID["openai"].ImageModel != "gpt-image-2" {
		t.Fatalf("models: %+v", byID)
	}
	// No key set yet → validation fails offline.
	if _, err := app.ProviderValidate("doubao"); err == nil {
		t.Fatal("expected validation error without a key")
	}

	// Save a key for OpenAI, validate, switch active.
	cfg, err := app.ProviderConfigGet("openai")
	if err != nil {
		t.Fatal(err)
	}
	cfg.APIKey = "sk-test"
	if err := app.ProviderConfigSave("openai", *cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ProviderValidate("openai"); err != nil {
		t.Fatalf("validate openai after save: %v", err)
	}
	if err := app.ProviderSetActive("openai"); err != nil {
		t.Fatal(err)
	}
	list, _ = app.ProviderList()
	for _, p := range list {
		if p.ID == "openai" && !p.Active {
			t.Fatal("openai should be active after switch")
		}
		if p.ID == "doubao" && p.Active {
			t.Fatal("doubao should not be active after switch")
		}
	}
	// The key is reported (hasApiKey) but never returned in listings.
	for _, p := range list {
		if p.ID == "openai" && !p.HasAPIKey {
			t.Fatal("openai key should be reported")
		}
	}
	// Stats start empty.
	stats, err := app.ProviderStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalCalls != 0 {
		t.Fatalf("initial stats: %+v", stats)
	}
}

// TestGenerationPlanBindings covers the generation confirmation flow through
// the GUI bindings: prepare (no calls) → cancel (no calls) → confirm (calls,
// stats updated).
func TestGenerationPlanBindings(t *testing.T) {
	var calls atomic.Int64
	app, _ := newTestApp(t, fakeClient(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return filmstripPNGResp(t), nil
	}))

	if _, err := app.PackageCreate("Hero"); err != nil {
		t.Fatal(err)
	}
	if err := app.IdentitySetCanvas(32, 32); err != nil {
		t.Fatal(err)
	}
	// Reference images with roles (avoid Windows reserved device names).
	main := filepath.Join(t.TempDir(), "ref-main.png")
	extra := filepath.Join(t.TempDir(), "ref-extra.png")
	if err := os.WriteFile(main, []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extra, []byte("extra"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.IdentityImportMaterial("reference_image", main, "主参考图", "main_reference"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.IdentityImportMaterial("reference_image", extra, "辅助", "auxiliary_reference"); err != nil {
		t.Fatal(err)
	}
	// A second main reference must be rejected at the binding too.
	if _, err := app.IdentityImportMaterial("reference_image", extra, "另一个主参考", "main_reference"); err == nil {
		t.Fatal("expected error for a second main reference")
	}

	// Configure a key so execution can reach the fake transport.
	cfg, err := app.ProviderConfigGet("doubao")
	if err != nil {
		t.Fatal(err)
	}
	cfg.APIKey = "test-key"
	if err := app.ProviderConfigSave("doubao", *cfg); err != nil {
		t.Fatal(err)
	}

	// Prepare: 4 directions → plan with 外发素材/方向数/预算/每方向最多 3 次.
	plan, err := app.GenerationPlanPrepare(GenerationRequestView{
		Directions: 4, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatalf("prepare made transport calls: %d", calls.Load())
	}
	if plan.ProviderID != "doubao" || plan.BasicDirections != 3 || plan.MirroredDirections != 1 {
		t.Fatalf("plan: %+v", plan)
	}
	if plan.MaxAttemptsPerDirection != 3 || plan.ExpectedCalls != 3 || len(plan.OutboundMaterials) != 2 {
		t.Fatalf("plan budget/materials: %+v", plan)
	}
	if plan.Prompt.Prompt == "" || plan.Prompt.StylePresetID != "pixel_classic" {
		t.Fatalf("prompt snapshot missing: %+v", plan.Prompt)
	}

	// Cancel → no calls, plan final.
	res, err := app.GenerationPlanConfirm(plan.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Accepted || res.CallsMade != 0 || calls.Load() != 0 {
		t.Fatalf("cancel result: %+v, calls=%d", res, calls.Load())
	}

	// Prepare again and confirm → executes 3 provider calls (right/up/down),
	// stats updated.
	plan2, err := app.GenerationPlanPrepare(GenerationRequestView{
		Directions: 4, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	res2, err := app.GenerationPlanConfirm(plan2.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Accepted || res2.Status != "executed" || res2.CallsMade != 3 || res2.Attempts != 3 {
		t.Fatalf("execute result: %+v", res2)
	}
	if calls.Load() != 3 {
		t.Fatalf("transport calls = %d, want 3", calls.Load())
	}
	if len(res2.Results) != 3 {
		t.Fatalf("direction results = %d", len(res2.Results))
	}
	stats, err := app.ProviderStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalCalls != 3 {
		t.Fatalf("stats total = %d, want 3", stats.TotalCalls)
	}
	// Invalid request: directions must be 1/4/8.
	if _, err := app.GenerationPlanPrepare(GenerationRequestView{Directions: 7}); err == nil {
		t.Fatal("expected error for invalid direction count")
	}
	_ = context.Background()
}
