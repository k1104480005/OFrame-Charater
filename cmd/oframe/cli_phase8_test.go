package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oframe/character-workbench/core/assetexport"
	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/motion"
	"github.com/oframe/character-workbench/core/provider"
	"github.com/oframe/character-workbench/core/service"
	"github.com/oframe/character-workbench/core/task"
)

// cliPhase8FilmstripRT returns a fake transport answering filmstrip PNGs so
// CLI phase-8 tests (generation run/export) never call real services.
func cliPhase8FilmstripRT(t *testing.T) (*fakeRT, *http.Client) {
	t.Helper()
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return cliFilmstripResp(t), nil
	}}
	return rt, &http.Client{Transport: rt}
}

// acceptBestCandidate accepts the highest-scoring persisted candidate of a
// package through a fresh service instance (candidates survive restarts via
// the candidate directory), making the assets exportable (task 9.2).
func acceptBestCandidate(t *testing.T, settingsDir, pkgPath string, rt http.RoundTripper) {
	t.Helper()
	svc, err := service.New(service.Options{SettingsDir: settingsDir, HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	cands := svc.CandidateList(pkgPath)
	if len(cands) == 0 {
		t.Fatal("no candidates generated for acceptance")
	}
	best := cands[0]
	for _, c := range cands {
		if c.Scores.Overall > best.Scores.Overall {
			best = c
		}
	}
	dec, err := svc.CandidateDecide(context.Background(), pkgPath, best.ID, true, "")
	if err != nil {
		t.Fatalf("accept best candidate: %v", err)
	}
	if dec.Decision != identity.CandidateAccepted {
		t.Fatalf("acceptance failed: %+v", dec)
	}
}

// createCLIMotion creates a motion in the package through a fresh service
// instance so CLI batch generation can target it (--motion) — the assets
// become exportable (export requires motion-linked accepted assets).
func createCLIMotion(t *testing.T, settingsDir, pkgPath string, rt http.RoundTripper, count int) string {
	t.Helper()
	svc, err := service.New(service.Options{SettingsDir: settingsDir, HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	m, err := svc.MotionCreate(pkgPath, "walk", motion.DirectionStrategy{Count: count, Mirror: true})
	if err != nil {
		t.Fatal(err)
	}
	return m.ID
}

// TestIdentityCanvasCommand verifies the `identity canvas` subcommand: a
// CLI-created package has no logical canvas, generation is refused until the
// canvas is set, and after `identity canvas` generation planning succeeds.
func TestIdentityCanvasCommand(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "ws")
	if _, _, err := runCLI(t, "workspace", "init", ws); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCLI(t, "identity", "create", "--workspace", ws, "--name", "Hero", "--json")
	if err != nil {
		t.Fatalf("identity create: %v\n%s", err, out)
	}
	var created map[string]any
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatal(err)
	}
	pkg := created["path"].(string)

	// 人工验收更新: generation planning requires a configured provider — make
	// the test hermetic with a temp settings dir + doubao instead of relying
	// on the developer's real settings.
	settingsDir := filepath.Join(t.TempDir(), "cfg")
	if _, _, err := runCLI(t, "provider", "config", "set", "--key", "ark-canvas", "--settings-dir", settingsDir, "doubao"); err != nil {
		t.Fatal(err)
	}

	// Without a canvas, generation planning is refused (前置条件).
	if _, _, err := runCLI(t, "generation", "plan", "--directions", "1", "--settings-dir", settingsDir, pkg, "--json"); err == nil ||
		!strings.Contains(err.Error(), "logical canvas must be set") {
		t.Fatalf("generation plan without canvas should fail with the canvas prerequisite, got %v", err)
	}

	// Set the canvas, then planning succeeds (3 生成 + 1 镜像 for 4 directions).
	out, _, err = runCLI(t, "identity", "canvas", "--width", "32", "--height", "32", pkg, "--json")
	if err != nil {
		t.Fatalf("identity canvas: %v\n%s", err, out)
	}
	var cres map[string]any
	if err := json.Unmarshal([]byte(out), &cres); err != nil || cres["ok"] != true {
		t.Fatalf("canvas result: %v\n%s", cres, out)
	}
	out, _, err = runCLI(t, "generation", "plan", "--directions", "4", "--settings-dir", settingsDir, pkg, "--json")
	if err != nil {
		t.Fatalf("generation plan after canvas: %v\n%s", err, out)
	}
	var plan map[string]any
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	p := plan["plan"].(map[string]any)
	if p["basicDirections"].(float64) != 3 || p["mirroredDirections"].(float64) != 1 {
		t.Fatalf("plan direction split: %+v", p)
	}
}

// TestGenerationRunLinksTaskQueue verifies task 12.2: the CLI batch generation
// command executes through the SAME persisted task queue as the GUI — a
// `generation run --yes` leaves a succeeded task row in the queue database.
func TestGenerationRunLinksTaskQueue(t *testing.T) {
	pkg := setupCLIPackage(t)
	settingsDir := filepath.Join(t.TempDir(), "cfg")
	_, client := cliPhase8FilmstripRT(t)
	httpClientOverride = client
	defer func() { httpClientOverride = nil }()

	if _, _, err := runCLI(t, "provider", "config", "set", "--key", "ark-test", "--settings-dir", settingsDir, "doubao"); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCLI(t, "generation", "run", "--directions", "1", "--settings-dir", settingsDir, "--yes", pkg, "--json")
	if err != nil {
		t.Fatalf("generation run --yes: %v\n%s", err, out)
	}

	// The task row lives in the shared queue database (settings dir), exactly
	// as the GUI drawer reads it (task 6.1: 本地持久化).
	q, err := task.Open(filepath.Join(settingsDir, "queue.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = q.Close() }()
	all, err := q.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("task rows = %d, want 1", len(all))
	}
	row := all[0]
	if row.Kind != "generate" || row.Status != task.StatusSucceeded || row.Progress != 1 || row.ExpectedCalls != 1 {
		t.Fatalf("task row: %+v", row)
	}
	if row.Result == "" {
		t.Fatal("succeeded task must carry its cached result (4.8 idempotency key)")
	}
}

// TestValidateCommandIdentityAndExport verifies task 12.3: `oframe validate`
// accepts both identity packages and export packages, and fails on a bad path.
func TestValidateCommandIdentityAndExport(t *testing.T) {
	pkg := setupCLIPackage(t)

	out, _, err := runCLI(t, "validate", pkg, "--json")
	if err != nil {
		t.Fatalf("validate identity: %v\n%s", err, out)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if res["ok"] != true || res["kind"] != "identityPackage" {
		t.Fatalf("identity validation result: %v", res)
	}

	// A bare directory with no manifest is neither an identity package nor an
	// export package → validation fails.
	empty := filepath.Join(t.TempDir(), "nothing")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, "validate", empty); err == nil {
		t.Fatal("validate of an empty dir must fail")
	}
	// The export-package validation branch (kind=exportPackage) is exercised
	// in TestExportCreateValidateHistoryCLI below.
}

// TestExportCreateValidateHistoryCLI verifies tasks 12.3/12.4: the CLI export
// commands build a validated engine-target package, validate it, and record
// the export in the package's export history.
func TestExportCreateValidateHistoryCLI(t *testing.T) {
	pkg := setupCLIPackage(t)
	settingsDir := filepath.Join(t.TempDir(), "cfg")
	rt, client := cliPhase8FilmstripRT(t)
	httpClientOverride = client
	defer func() { httpClientOverride = nil }()

	// Generate a candidate into a motion and accept it (assets exportable).
	if _, _, err := runCLI(t, "provider", "config", "set", "--key", "ark-test", "--settings-dir", settingsDir, "doubao"); err != nil {
		t.Fatal(err)
	}
	mID := createCLIMotion(t, settingsDir, pkg, rt, 1)
	out, _, err := runCLI(t, "generation", "run", "--motion", mID, "--settings-dir", settingsDir, "--yes", pkg, "--json")
	if err != nil {
		t.Fatalf("generation run: %v\n%s", err, out)
	}
	acceptBestCandidate(t, settingsDir, pkg, rt)

	// export create → godot
	outDir := filepath.Join(t.TempDir(), "out")
	out, _, err = runCLI(t, "export", "create", "--output", outDir, "--target", "godot", "--settings-dir", settingsDir, pkg, "--json")
	if err != nil {
		t.Fatalf("export create: %v\n%s", err, out)
	}
	var exp map[string]any
	if err := json.Unmarshal([]byte(out), &exp); err != nil {
		t.Fatal(err)
	}
	if exp["ok"] != true || exp["target"] != "godot" {
		t.Fatalf("export create result: %v", exp)
	}

	// Package contents: manifest.json + spritesheet.png + per-frame PNGs +
	// target metadata.
	for _, f := range []string{"manifest.json", "spritesheet.png", "godot.json"} {
		if !fileExistsCLI(filepath.Join(outDir, f)) {
			t.Fatalf("export missing %s", f)
		}
	}
	manifest := exp["manifest"].(map[string]any)
	anims := manifest["animations"].([]any)
	if len(anims) != 1 || manifest["cellWidth"].(float64) <= 0 {
		t.Fatalf("export manifest: %v", manifest)
	}

	// export validate passes (the package was auto-validated at build time).
	if _, _, err := runCLI(t, "export", "validate", outDir); err != nil {
		t.Fatalf("export validate: %v", err)
	}
	// And `oframe validate` also accepts the export package (task 12.3).
	out, _, err = runCLI(t, "validate", outDir, "--json")
	if err != nil {
		t.Fatalf("validate export package: %v", err)
	}
	var vres map[string]any
	_ = json.Unmarshal([]byte(out), &vres)
	if vres["ok"] != true || vres["kind"] != "exportPackage" {
		t.Fatalf("export package validation: %v", vres)
	}

	// export history records the succeeded export (task 11.4/12.4).
	out, _, err = runCLI(t, "export", "history", "--settings-dir", settingsDir, pkg, "--json")
	if err != nil {
		t.Fatalf("export history: %v", err)
	}
	var hist map[string]any
	if err := json.Unmarshal([]byte(out), &hist); err != nil {
		t.Fatal(err)
	}
	items, ok := hist["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("export history items: %v", hist)
	}
	rec := items[0].(map[string]any)
	if rec["target"] != "godot" || rec["result"] != "succeeded" {
		t.Fatalf("export history record: %v", rec)
	}
}

// acceptAllCandidates accepts every candidate that passes the quality gate
// (confirm=true). Identical generation on both ends → the same candidate
// directions pass, so both export packages carry the same animation set.
func acceptAllCandidates(t *testing.T, settingsDir, pkgPath string, rt http.RoundTripper) {
	t.Helper()
	svc, err := service.New(service.Options{SettingsDir: settingsDir, HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	cands := svc.CandidateList(pkgPath)
	if len(cands) == 0 {
		t.Fatal("no candidates generated for acceptance")
	}
	accepted := 0
	for _, c := range cands {
		dec, err := svc.CandidateDecide(context.Background(), pkgPath, c.ID, true, "")
		if err == nil && dec.Decision == identity.CandidateAccepted {
			accepted++
		}
	}
	if accepted == 0 {
		t.Fatal("no candidate passed the acceptance gate")
	}
}

// TestCLIAndServiceConsistency verifies task 12.5: the SAME workflow run
// through the CLI (`oframe generation run` + `oframe export create`) and
// through the GUI application service (core/service) produces identical
// results — the shared-core guarantee (design D1/D12).
func TestCLIAndServiceConsistency(t *testing.T) {
	rt, client := cliPhase8FilmstripRT(t)

	// Two identically-prepared packages.
	pkgCLI := setupCLIPackage(t)
	pkgSvc := setupCLIPackage(t)
	dirCLI := filepath.Join(t.TempDir(), "cfg-cli")
	dirSvc := filepath.Join(t.TempDir(), "cfg-svc")

	// --- End A: CLI path (generation run --motion + export create) ---
	httpClientOverride = client
	defer func() { httpClientOverride = nil }()
	if _, _, err := runCLI(t, "provider", "config", "set", "--key", "ark-test", "--settings-dir", dirCLI, "doubao"); err != nil {
		t.Fatal(err)
	}
	cliMotion := createCLIMotion(t, dirCLI, pkgCLI, rt, 4)
	out, _, err := runCLI(t, "generation", "run", "--motion", cliMotion, "--settings-dir", dirCLI, "--yes", pkgCLI, "--json")
	if err != nil {
		t.Fatalf("CLI generation: %v\n%s", err, out)
	}
	acceptAllCandidates(t, dirCLI, pkgCLI, rt)
	outCLI := filepath.Join(t.TempDir(), "out-cli")
	_, _, err = runCLI(t, "export", "create", "--output", outCLI, "--target", "generic", "--settings-dir", dirCLI, pkgCLI, "--json")
	if err != nil {
		t.Fatalf("CLI export: %v", err)
	}

	// --- End B: GUI service path (same shared core, direct calls) ---
	httpClientOverride = nil
	svc, err := service.New(service.Options{SettingsDir: dirSvc, HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	// Fresh stores carry no provider cards (人工验收更新) — seed doubao from
	// its built-in defaults; SaveProviderConfig registers it on demand.
	cfg := provider.DefaultConfig(provider.ProviderDoubao)
	cfg.APIKey = "ark-test"
	if err := svc.SaveProviderConfig(provider.ProviderDoubao, cfg); err != nil {
		t.Fatal(err)
	}
	svcMotion := createCLIMotion(t, dirSvc, pkgSvc, rt, 4)
	plan, err := svc.PrepareGeneration(context.Background(), service.GenerationRequest{
		PackagePath: pkgSvc, MotionID: svcMotion, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "executed" || len(res.Results) != 3 {
		t.Fatalf("service generation result: %+v", res)
	}
	acceptAllCandidates(t, dirSvc, pkgSvc, rt)
	outSvc := filepath.Join(t.TempDir(), "out-svc")
	_, err = svc.ExportPackage(pkgSvc, outSvc, assetexport.TargetGeneric, "")
	if err != nil {
		t.Fatalf("service export: %v", err)
	}

	// --- Compare: both ends must produce identical candidate/motion/export
	// structure (单核多端, 行为一致). ---
	cliCands := candidatesOf(t, dirCLI, pkgCLI, rt)
	svcCands := candidatesOf(t, dirSvc, pkgSvc, rt)
	if len(cliCands) != len(svcCands) {
		t.Fatalf("candidate count differs: CLI=%d service=%d", len(cliCands), len(svcCands))
	}
	cliDirs := map[string]int{}
	for _, c := range cliCands {
		cliDirs[c.Direction] = c.Frames
	}
	for _, c := range svcCands {
		if n, ok := cliDirs[c.Direction]; !ok || n != c.Frames {
			t.Fatalf("candidate %s: CLI frames=%v service frames=%d — dual-end drift", c.Direction, cliDirs, c.Frames)
		}
	}

	cliManifest := manifestOf(t, outCLI)
	svcManifest := manifestOf(t, outSvc)
	if cliManifest.Target != svcManifest.Target || cliManifest.CellWidth != svcManifest.CellWidth ||
		cliManifest.CellHeight != svcManifest.CellHeight || len(cliManifest.Animations) != len(svcManifest.Animations) {
		t.Fatalf("export manifest drift: CLI=%+v service=%+v", cliManifest, svcManifest)
	}
	// Same accepted direction set (identical generation → identical candidates
	// pass the gate on both ends), same per-frame geometry and rhythm.
	byDir := map[string]assetexport.AnimationManifest{}
	for _, a := range svcManifest.Animations {
		byDir[a.Direction] = a
	}
	for _, a := range cliManifest.Animations {
		b, ok := byDir[a.Direction]
		if !ok || len(a.Frames) != len(b.Frames) {
			t.Fatalf("animation %s drift: CLI=%d frames service=%d", a.Direction, len(a.Frames), len(b.Frames))
		}
		for j := range a.Frames {
			if a.Frames[j].Rect != b.Frames[j].Rect || a.Frames[j].DurationMs != b.Frames[j].DurationMs {
				t.Fatalf("frame %d of %s drift: CLI=%+v service=%+v", j, a.Direction, a.Frames[j], b.Frames[j])
			}
		}
	}
}

// candidatesOf lists the persisted candidates of a package through a fresh
// service (restart continuity: candidates load from the candidate directory).
func candidatesOf(t *testing.T, settingsDir, pkgPath string, rt http.RoundTripper) []struct {
	Direction string
	Frames    int
} {
	t.Helper()
	svc, err := service.New(service.Options{SettingsDir: settingsDir, HTTPClient: &http.Client{Transport: rt}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	cands := svc.CandidateList(pkgPath)
	out := make([]struct {
		Direction string
		Frames    int
	}, 0, len(cands))
	for _, c := range cands {
		out = append(out, struct {
			Direction string
			Frames    int
		}{c.Direction, len(c.Frames)})
	}
	return out
}

// manifestOf parses an export package's manifest.json.
func manifestOf(t *testing.T, outDir string) assetexport.Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(outDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m assetexport.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}
