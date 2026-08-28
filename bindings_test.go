package main

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/oframe/character-workbench/core/provider"
	"github.com/oframe/character-workbench/core/settings"
)

// newTestApp builds an App pointed at a temp workspace so binding tests run
// headlessly (no Wails window; emit() is nil-safe). The shared application
// service (provider/generation bindings) uses a temp settings dir and, when
// client is non-nil, a fake HTTP transport — real paid services are never
// called.
func newTestApp(t *testing.T, client *http.Client) (*App, string) {
	t.Helper()
	wsRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(wsRoot, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	app := NewApp()
	app.settingsDir = filepath.Join(t.TempDir(), "cfg")
	app.httpClient = client
	// Seed the classic built-in trio for binding tests (人工验收更新: fresh
	// installs start EMPTY — most tests here assume a pre-configured doubao).
	seedAppProviders(t, app.settingsDir)
	info, err := app.WorkspaceOpen(wsRoot)
	if err != nil {
		t.Fatalf("WorkspaceOpen: %v", err)
	}
	if info.Path == "" {
		t.Fatal("workspace path empty")
	}
	t.Cleanup(func() {
		if app.svc != nil {
			_ = app.svc.Close()
		}
	})
	return app, wsRoot
}

// newTestAppSimple is newTestApp with a nil client (no HTTP at all).
func newTestAppSimple(t *testing.T) (*App, string) {
	t.Helper()
	return newTestApp(t, nil)
}

// seedAppProviders writes the classic doubao/openai/agnes trio into the
// settings file BEFORE the app's service is lazily created, so the seeded
// providers are registered on first use.
func seedAppProviders(t *testing.T, dir string) {
	t.Helper()
	store, err := settings.New(dir)
	if err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	ps := store.ProviderSettings()
	ps.Providers = map[string]provider.ProviderConfig{
		provider.ProviderDoubao: provider.DefaultConfig(provider.ProviderDoubao),
		provider.ProviderOpenAI: provider.DefaultConfig(provider.ProviderOpenAI),
		provider.ProviderAgnes:  provider.DefaultConfig(provider.ProviderAgnes),
	}
	ps.ActiveProvider = provider.ProviderDoubao
	if err := store.SaveProviderSettings(ps); err != nil {
		t.Fatalf("seed providers: %v", err)
	}
}

func TestAppInfo(t *testing.T) {
	app, _ := newTestAppSimple(t)
	info := app.AppInfo()
	if info.Name == "" || info.Version == "" || info.Go == "" {
		t.Fatalf("AppInfo incomplete: %+v", info)
	}
	if info.Format < 1 {
		t.Fatalf("format version invalid: %d", info.Format)
	}
}

func TestPackageLifecycle(t *testing.T) {
	app, wsRoot := newTestAppSimple(t)

	// No package open yet → launch page semantics.
	if cur := app.CurrentPackage(); cur != nil {
		t.Fatalf("expected nil current package, got %+v", cur)
	}

	// Create → opens automatically.
	sum, err := app.PackageCreate("Hero", "主角")
	if err != nil {
		t.Fatalf("PackageCreate: %v", err)
	}
	if sum.Name != "Hero" || sum.Path == "" || sum.CurrentVersion == "" {
		t.Fatalf("created summary incomplete: %+v", sum)
	}
	if sum.Category != "主角" {
		t.Fatalf("expected category %q, got %q", "主角", sum.Category)
	}
	if !filepath.IsAbs(sum.Path) {
		t.Fatalf("package path not absolute: %s", sum.Path)
	}
	if _, err := os.Stat(filepath.Join(sum.Path, "manifest.json")); err != nil {
		t.Fatalf("manifest missing after create: %v", err)
	}
	if cur := app.CurrentPackage(); cur == nil || cur.Name != "Hero" {
		t.Fatalf("current package not set after create: %+v", cur)
	}

	// Workspace list reflects the new package (GUI and CLI share core/workspace).
	pkgs, err := app.WorkspaceList()
	if err != nil {
		t.Fatalf("WorkspaceList: %v", err)
	}
	if len(pkgs) != 1 || pkgs[0].Name != "Hero" {
		t.Fatalf("workspace list unexpected: %+v", pkgs)
	}

	// Close → back to launch page.
	if err := app.PackageClose(); err != nil {
		t.Fatalf("PackageClose: %v", err)
	}
	if cur := app.CurrentPackage(); cur != nil {
		t.Fatalf("expected nil after close, got %+v", cur)
	}

	// Re-open by path.
	sum2, err := app.PackageOpen(filepath.Join(wsRoot, "Hero"))
	if err != nil {
		t.Fatalf("PackageOpen: %v", err)
	}
	if sum2.Name != "Hero" {
		t.Fatalf("reopened name = %q", sum2.Name)
	}
}

func TestPackageOpenErrors(t *testing.T) {
	app, wsRoot := newTestAppSimple(t)

	// Missing manifest → refused (task 2.2).
	bad := filepath.Join(wsRoot, "Broken")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := app.PackageOpen(bad); err == nil {
		t.Fatal("expected error opening package without manifest")
	}

	// Empty name / path rejected by bindings.
	if _, err := app.PackageCreate("  ", ""); err == nil {
		t.Fatal("expected error for blank package name")
	}
	if _, err := app.PackageOpen(""); err == nil {
		t.Fatal("expected error for blank path")
	}
}

func TestIdentityBindings(t *testing.T) {
	app, _ := newTestAppSimple(t)
	if _, err := app.PackageCreate("Hero", ""); err != nil {
		t.Fatal(err)
	}

	// Identity sub-page starts empty.
	view, err := app.IdentityGet()
	if err != nil {
		t.Fatalf("IdentityGet: %v", err)
	}
	if view.Name != "Hero" || view.Description != "" {
		t.Fatalf("unexpected initial identity: %+v", view)
	}
	if view.Canvas != nil {
		t.Fatalf("canvas should be unset initially")
	}

	// Text description entry (task 2.3 入口 1).
	if err := app.IdentitySetDescription("a small green hero"); err != nil {
		t.Fatalf("IdentitySetDescription: %v", err)
	}
	view, _ = app.IdentityGet()
	if view.Description != "a small green hero" || view.EntryKind != "text" {
		t.Fatalf("description not persisted: %+v", view)
	}

	// Logical canvas (task 2.4).
	if err := app.IdentitySetCanvas(32, 32); err != nil {
		t.Fatalf("IdentitySetCanvas: %v", err)
	}
	view, _ = app.IdentityGet()
	if view.Canvas == nil || view.Canvas.UnitWidth != 32 || view.Canvas.UnitHeight != 32 {
		t.Fatalf("canvas not persisted: %+v", view.Canvas)
	}

	// Anchor presets + add (task 2.5).
	presets := app.IdentityAnchorPresets()
	if len(presets) == 0 {
		t.Fatal("expected built-in anchor presets")
	}
	anchor, err := app.IdentityAddAnchorPreset("feet", "脚底")
	if err != nil {
		t.Fatalf("IdentityAddAnchorPreset: %v", err)
	}
	if anchor.X != 16 || anchor.Y != 31 { // bottom-center on 32×32
		t.Fatalf("feet anchor default position wrong: (%d,%d)", anchor.X, anchor.Y)
	}
	view, _ = app.IdentityGet()
	if len(view.Anchors) != 1 {
		t.Fatalf("anchors not persisted: %+v", view.Anchors)
	}

	// Version history (task 9.1 model is present from creation).
	if len(view.Versions) != 1 || view.Versions[0].ID != "v1" {
		t.Fatalf("versions unexpected: %+v", view.Versions)
	}
	if view.CurrentVersion != "v1" {
		t.Fatalf("current version = %q", view.CurrentVersion)
	}
}

func TestIdentityWithoutPackage(t *testing.T) {
	app, _ := newTestAppSimple(t)
	if _, err := app.IdentityGet(); err == nil {
		t.Fatal("expected error calling IdentityGet with no package open")
	}
	if _, err := app.IdentityAddAnchorPreset("feet", ""); err == nil {
		t.Fatal("expected error adding anchor with no package open")
	}
	if err := app.IdentitySetCanvas(16, 16); err == nil {
		t.Fatal("expected error setting canvas with no package open")
	}
}
