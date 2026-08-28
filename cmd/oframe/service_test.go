package main

import (
	"bytes"
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

// fakeRT is an in-process transport; CLI tests never call real paid services.
type fakeRT struct {
	handler func(r *http.Request) (*http.Response, error)
	calls   atomic.Int64
}

func (f *fakeRT) RoundTrip(r *http.Request) (*http.Response, error) {
	f.calls.Add(1)
	return f.handler(r)
}

func cliPNGResp() *http.Response {
	data, _ := json.Marshal(map[string]any{
		"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString([]byte("PNG"))}},
	})
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

// cliFilmstripResp builds a VALID PNG filmstrip response for the wired
// filmstrip pipeline (阶段 5: ConfirmGeneration 解码 provider 字节并跑管线): 4
// frames of 32×32 with magenta technical background and one opaque block each.
func cliFilmstripResp(t *testing.T) *http.Response {
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
		frames[i] = cliBlockFrame(10+(i%3), 22)
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

// cliBlockFrame is a 32×32 magenta frame (洋红仅用于抠图) with a 10×10 opaque
// block at (bx, by).
func cliBlockFrame(bx, by int) *image.RGBA {
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

// TestProviderCommands covers provider list/config set/validate/stats through
// the CLI (shared application service).
func TestProviderCommands(t *testing.T) {
	settingsDir := filepath.Join(t.TempDir(), "cfg")
	httpClientOverride = nil
	defer func() { httpClientOverride = nil }()

	out, _, err := runCLI(t, "provider", "list", "--settings-dir", settingsDir, "--json")
	if err != nil {
		t.Fatalf("provider list: %v", err)
	}
	var list map[string]any
	if err := json.Unmarshal([]byte(out), &list); err != nil || list["ok"] != true {
		t.Fatalf("provider list json: %v\n%s", err, out)
	}
	providers := list["providers"].([]any)
	// 人工验收更新: a fresh install has NO provider cards — the CLI `config
	// set` below registers doubao on demand.
	if len(providers) != 0 {
		t.Fatalf("providers = %d, want 0 before any configuration", len(providers))
	}

	// Set a key on doubao: the id is unknown to the fresh store, so
	// SaveProviderConfig validates + persists + registers it in one step.
	out, _, err = runCLI(t, "provider", "config", "set", "--key", "ark-test", "--settings-dir", settingsDir, "doubao", "--json")
	if err != nil {
		t.Fatalf("config set: %v\n%s", err, out)
	}
	out, _, err = runCLI(t, "provider", "list", "--settings-dir", settingsDir, "--json")
	if err != nil {
		t.Fatalf("provider list after set: %v", err)
	}
	_ = json.Unmarshal([]byte(out), &list)
	providers = list["providers"].([]any)
	if len(providers) != 1 {
		t.Fatalf("providers after config set = %d, want 1 (doubao auto-registered)", len(providers))
	}

	// Validate without key fails (offline).
	emptyDir := filepath.Join(t.TempDir(), "cfg-empty")
	if _, _, err := runCLI(t, "provider", "validate", "--settings-dir", emptyDir, "doubao"); err == nil {
		t.Fatal("expected validation error without key")
	}

	if _, _, err := runCLI(t, "provider", "validate", "--settings-dir", settingsDir, "doubao"); err != nil {
		t.Fatalf("validate after set: %v", err)
	}

	// The key is redacted in human output.
	out, _, err = runCLI(t, "provider", "config", "get", "--settings-dir", settingsDir, "doubao")
	if err != nil {
		t.Fatalf("config get: %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("****")) || bytes.Contains([]byte(out), []byte("ark-test")) {
		t.Errorf("key not redacted in output: %s", out)
	}

	// Stats start empty.
	out, _, err = runCLI(t, "provider", "stats", "--settings-dir", settingsDir, "--json")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	var stats map[string]any
	_ = json.Unmarshal([]byte(out), &stats)
	items, _ := stats["stats"].(map[string]any)["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("stats should be empty: %s", out)
	}
}

// setupCLIPackage creates a ready identity package on disk (canvas + two
// reference images) so the CLI generation commands can plan against it.
func setupCLIPackage(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := identity.Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetTextDescription("a small green pixel hero"); err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetLogicalCanvas(32, 32); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(t.TempDir(), "ref-main.png")
	extra := filepath.Join(t.TempDir(), "ref-extra.png")
	if err := os.WriteFile(main, []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extra, []byte("extra"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := pkg.AddReferenceImage(main, "main ref", identity.RoleMainReference); err != nil {
		t.Fatal(err)
	}
	if _, err := pkg.AddReferenceImage(extra, "aux ref", identity.RoleAuxiliaryReference); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestGenerationPlanCommand verifies `oframe generation plan` builds the
// confirmation payload without any external call.
func TestGenerationPlanCommand(t *testing.T) {
	pkg := setupCLIPackage(t)
	settingsDir := filepath.Join(t.TempDir(), "cfg")
	httpClientOverride = nil
	defer func() { httpClientOverride = nil }()

	// 人工验收更新: fresh stores have no providers — configure doubao first
	// (the CLI registers it on demand).
	if _, _, err := runCLI(t, "provider", "config", "set", "--key", "ark-plan", "--settings-dir", settingsDir, "doubao"); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "generation", "plan", "--directions", "4", "--settings-dir", settingsDir, pkg, "--json")
	if err != nil {
		t.Fatalf("generation plan: %v\n%s", err, out)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	plan := res["plan"].(map[string]any)
	if plan["providerId"] != "doubao" || plan["basicDirections"].(float64) != 3 || plan["mirroredDirections"].(float64) != 1 {
		t.Fatalf("plan: %+v", plan)
	}
	if plan["maxAttemptsPerDirection"].(float64) != 3 || plan["expectedCalls"].(float64) != 3 {
		t.Fatalf("plan budget: %+v", plan)
	}
	prompt := plan["prompt"].(map[string]any)
	if prompt["prompt"] == "" {
		t.Fatal("prompt snapshot missing")
	}
}

// TestGenerationRunRequiresConfirmation verifies `generation run` without
// --yes never calls the provider (fake transport counter stays 0).
func TestGenerationRunRequiresConfirmation(t *testing.T) {
	pkg := setupCLIPackage(t)
	settingsDir := filepath.Join(t.TempDir(), "cfg")
	var calls atomic.Int64
	httpClientOverride = &http.Client{Transport: &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return cliFilmstripResp(t), nil
	}}}
	defer func() { httpClientOverride = nil }()

	// A provider must be configured before a plan can be built.
	if _, _, err := runCLI(t, "provider", "config", "set", "--key", "ark-test", "--settings-dir", settingsDir, "doubao"); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "generation", "run", "--directions", "1", "--settings-dir", settingsDir, pkg, "--json")
	if err != nil {
		t.Fatalf("run without --yes should not error, got %v\n%s", err, out)
	}
	if calls.Load() != 0 {
		t.Fatalf("transport calls without --yes = %d, want 0", calls.Load())
	}
	var res map[string]any
	_ = json.Unmarshal([]byte(out), &res)
	if res["confirmed"] != false {
		t.Fatalf("expected confirmed=false: %s", out)
	}
}

// TestGenerationRunConfirmed verifies `generation run --yes` executes the
// provider calls (fake transport) and records statistics.
func TestGenerationRunConfirmed(t *testing.T) {
	pkg := setupCLIPackage(t)
	settingsDir := filepath.Join(t.TempDir(), "cfg")
	var calls atomic.Int64
	httpClientOverride = &http.Client{Transport: &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return cliFilmstripResp(t), nil
	}}}
	defer func() { httpClientOverride = nil }()

	// Configure a key first so execution can reach the transport.
	if _, _, err := runCLI(t, "provider", "config", "set", "--key", "ark-test", "--settings-dir", settingsDir, "doubao"); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "generation", "run", "--directions", "4", "--settings-dir", settingsDir, "--yes", pkg, "--json")
	if err != nil {
		t.Fatalf("run --yes: %v\n%s", err, out)
	}
	if calls.Load() != 3 {
		t.Fatalf("transport calls = %d, want 3 (right/up/down)", calls.Load())
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	result := res["result"].(map[string]any)
	if result["status"] != "executed" || result["callsMade"].(float64) != 3 {
		t.Fatalf("result: %+v", result)
	}

	// Statistics persisted through the shared service.
	out, _, err = runCLI(t, "provider", "stats", "--settings-dir", settingsDir, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var stats map[string]any
	_ = json.Unmarshal([]byte(out), &stats)
	items, _ := stats["stats"].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["callCount"].(float64) != 3 {
		t.Fatalf("stats after run: %s", out)
	}
}
