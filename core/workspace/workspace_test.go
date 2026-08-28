package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
)

// Workspace directory: organizes identity packages; listings and copy-restore
// (复制恢复) of a package directory are verified here.

func TestInitAndOpen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ws")
	ws, err := Init(root)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if ws.Root() != root {
		t.Errorf("Root() = %q, want %q", ws.Root(), root)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("workspace dir missing: %v", err)
	}
	// Init is idempotent.
	if _, err := Init(root); err != nil {
		t.Errorf("re-Init: %v", err)
	}
	if _, err := Open(root); err != nil {
		t.Errorf("Open: %v", err)
	}
	if _, err := Open(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("Open should fail for a missing workspace")
	}
}

func TestListFindsPackagesOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ws")
	ws, err := Init(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Create(filepath.Join(root, "Hero"), "Hero"); err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Create(filepath.Join(root, "Slime"), "Slime"); err != nil {
		t.Fatal(err)
	}
	// A non-package directory must be ignored.
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	pkgs, err := ws.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("packages = %d, want 2 (%+v)", len(pkgs), pkgs)
	}
	if pkgs[0].Name != "Hero" || pkgs[1].Name != "Slime" {
		t.Errorf("packages not sorted by name: %+v", pkgs)
	}
	if pkgs[0].FormatVersion != identity.FormatVersion || pkgs[0].CurrentVersion != identity.InitialVersionID {
		t.Errorf("package info wrong: %+v", pkgs[0])
	}
}

func TestResolve(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ws")
	ws, err := Init(root)
	if err != nil {
		t.Fatal(err)
	}
	byName, err := ws.Resolve("Hero")
	if err != nil {
		t.Fatalf("Resolve(name): %v", err)
	}
	if want := filepath.Join(root, "Hero"); byName != want {
		t.Errorf("Resolve(name) = %q, want %q", byName, want)
	}
	byAbs, err := ws.Resolve(filepath.Join(root, "Hero"))
	if err != nil {
		t.Fatalf("Resolve(abs): %v", err)
	}
	if byAbs != byName {
		t.Errorf("Resolve(abs) = %q, want %q", byAbs, byName)
	}
	if _, err := ws.Resolve(filepath.Join(t.TempDir(), "outside")); err == nil {
		t.Error("Resolve should refuse paths outside the workspace")
	}
	if _, err := ws.Resolve("..\\evil"); err == nil {
		t.Error("Resolve should refuse traversal names")
	}
}

// TestCopyRestore verifies the workspace copy-restore property (复制恢复): an
// identity package directory can be copied elsewhere and re-opened with its
// manifest and materials intact.
func TestCopyRestore(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ws")
	ws, err := Init(root)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "Hero")
	pkg, err := identity.Create(src, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	refSrc := filepath.Join(t.TempDir(), "ref.png")
	if err := os.WriteFile(refSrc, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	mat, err := pkg.AddReferenceImage(refSrc, "ref.png", identity.RoleMainReference)
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetLogicalCanvas(16, 16); err != nil {
		t.Fatal(err)
	}

	// Copy the whole package directory to a backup location.
	backup := filepath.Join(t.TempDir(), "backup", "Hero-copy")
	if err := copyTree(src, backup); err != nil {
		t.Fatalf("copyTree: %v", err)
	}

	// Restore: open the copy as a fresh package.
	restored, err := identity.Open(backup)
	if err != nil {
		t.Fatalf("open restored copy: %v", err)
	}
	if restored.Identity().Name != "Hero" {
		t.Errorf("restored name = %q", restored.Identity().Name)
	}
	mats := restored.Materials()
	if len(mats) != 1 || mats[0].ID != mat.ID {
		t.Fatalf("restored materials = %+v", mats)
	}
	abs, err := restored.MaterialPath(mats[0])
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(abs); err != nil || string(data) != "png" {
		t.Fatalf("restored material content missing: %v", err)
	}
	if c := restored.LogicalCanvas(); c == nil || c.UnitWidth != 16 {
		t.Errorf("restored canvas = %+v", c)
	}

	// The copy also shows up as a package of the backup workspace.
	backupWS, err := Open(filepath.Dir(backup))
	if err != nil {
		t.Fatal(err)
	}
	pkgs, err := backupWS.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || pkgs[0].Name != "Hero" {
		t.Errorf("backup workspace packages = %+v", pkgs)
	}
	_ = ws
}

func TestConfigPersist(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "workspace.json")
	prev := configPathResolver
	configPathResolver = func() (string, error) { return cfgFile, nil }
	defer func() { configPathResolver = prev }()

	if _, err := LoadConfig(); err != nil {
		t.Fatalf("LoadConfig missing file: %v", err)
	}
	if err := SaveConfig(Config{Path: `D:\OFrameWorkspace`}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got.Path != `D:\OFrameWorkspace` {
		t.Errorf("persisted path = %q, want D:\\OFrameWorkspace", got.Path)
	}
	// Overwriting with an empty choice persists (used to re-seed defaults).
	if err := SaveConfig(Config{}); err != nil {
		t.Fatalf("SaveConfig empty: %v", err)
	}
	got, _ = LoadConfig()
	if got.Path != "" {
		t.Errorf("cleared path = %q, want empty", got.Path)
	}
}

func TestPreferredDefaultPath(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "workspace.json")
	prev := configPathResolver
	configPathResolver = func() (string, error) { return cfgFile, nil }
	defer func() { configPathResolver = prev }()

	// When a choice is persisted, it wins regardless of drives.
	chosen := filepath.Join(t.TempDir(), "chosen-ws")
	if err := os.MkdirAll(chosen, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(Config{Path: chosen}); err != nil {
		t.Fatal(err)
	}
	p, err := PreferredDefaultPath()
	if err != nil {
		t.Fatalf("PreferredDefaultPath: %v", err)
	}
	if p != chosen {
		t.Errorf("PreferredDefaultPath = %q, want %q", p, chosen)
	}

	// No choice: falls back to a path ending in the default workspace name.
	if err := SaveConfig(Config{}); err != nil {
		t.Fatal(err)
	}
	p, err = PreferredDefaultPath()
	if err != nil {
		t.Fatalf("PreferredDefaultPath: %v", err)
	}
	if filepath.Base(p) != DefaultWorkspaceName {
		t.Errorf("PreferredDefaultPath = %q, want to end with %q", p, DefaultWorkspaceName)
	}
}

func TestMigrateCopy(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ws")
	ws, err := Init(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Create(filepath.Join(root, "Hero"), "Hero"); err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Create(filepath.Join(root, "Slime"), "Slime"); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "dst")
	if err := ws.Migrate(dst, false); err != nil {
		t.Fatalf("Migrate(copy): %v", err)
	}

	dstWS, err := Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	pkgs, err := dstWS.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("dst packages = %d, want 2", len(pkgs))
	}
	// Source is preserved on copy.
	srcWS, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if pkgs, _ := srcWS.List(); len(pkgs) != 2 {
		t.Fatalf("src packages after copy = %d, want 2", len(pkgs))
	}
}

func TestMigrateMove(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ws")
	ws, err := Init(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Create(filepath.Join(root, "Hero"), "Hero"); err != nil {
		t.Fatal(err)
	}
	if _, err := identity.Create(filepath.Join(root, "Slime"), "Slime"); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "dst")
	if err := ws.Migrate(dst, true); err != nil {
		t.Fatalf("Migrate(move): %v", err)
	}

	dstWS, err := Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	if pkgs, _ := dstWS.List(); len(pkgs) != 2 {
		t.Fatalf("dst packages = %d, want 2", len(pkgs))
	}
	// Source packages are removed on move.
	srcWS, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if pkgs, _ := srcWS.List(); len(pkgs) != 0 {
		t.Fatalf("src packages after move = %d, want 0", len(pkgs))
	}
}
