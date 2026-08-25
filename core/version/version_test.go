package version

import (
	"path/filepath"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
)

// Task 9.1 (phase-1 core): immutable identity versions — an explicit appearance
// revision forms an immutable version, older version assets stay preserved but
// no longer represent the current identity; old and new versions are
// accessible in parallel and the current pointer is correct.

func newTestPackage(t *testing.T) *identity.Package {
	t.Helper()
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := identity.Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func TestInitialVersionState(t *testing.T) {
	pkg := newTestPackage(t)
	cur, err := Current(pkg)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if cur.ID != identity.InitialVersionID || !cur.Immutable {
		t.Errorf("initial version = %+v", cur)
	}
	items := List(pkg)
	if len(items) != 1 {
		t.Errorf("initial versions = %d, want 1", len(items))
	}
}

func TestCommitAppearanceRevision(t *testing.T) {
	pkg := newTestPackage(t)

	v2, err := CommitAppearanceRevision(pkg, "redesign head")
	if err != nil {
		t.Fatalf("CommitAppearanceRevision: %v", err)
	}
	if v2.ID != "v2" || v2.Immutable {
		t.Errorf("new version = %+v, want v2 non-immutable", v2)
	}

	// New version represents the current identity by default.
	cur, err := Current(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if cur.ID != "v2" {
		t.Errorf("current = %q, want v2", cur.ID)
	}

	// Old version preserved and immutable; both accessible in parallel.
	v1, err := Get(pkg, "v1")
	if err != nil {
		t.Fatalf("Get(v1): %v", err)
	}
	if !v1.Immutable {
		t.Error("v1 must be immutable after a revision")
	}
	if v1.ID == cur.ID {
		t.Error("v1 must not be the current version")
	}
	items := List(pkg)
	if len(items) != 2 {
		t.Errorf("versions = %d, want 2", len(items))
	}

	// Second revision seals v2 and creates v3.
	v3, err := CommitAppearanceRevision(pkg, "new palette")
	if err != nil {
		t.Fatal(err)
	}
	if v3.ID != "v3" {
		t.Errorf("third version = %q, want v3", v3.ID)
	}
	v2again, _ := Get(pkg, "v2")
	if !v2again.Immutable {
		t.Error("v2 must become immutable after the second revision")
	}
}

func TestCommitPersistsAcrossReopen(t *testing.T) {
	pkg := newTestPackage(t)
	if _, err := CommitAppearanceRevision(pkg, "new look"); err != nil {
		t.Fatal(err)
	}
	reopened, err := identity.Open(pkg.Root())
	if err != nil {
		t.Fatal(err)
	}
	cur, err := Current(reopened)
	if err != nil {
		t.Fatal(err)
	}
	if cur.ID != "v2" {
		t.Errorf("reopened current = %q, want v2", cur.ID)
	}
	if len(List(reopened)) != 2 {
		t.Errorf("reopened versions = %d, want 2", len(List(reopened)))
	}
}

func TestCommitRequiresReason(t *testing.T) {
	pkg := newTestPackage(t)
	if _, err := CommitAppearanceRevision(pkg, "  "); err == nil {
		t.Fatal("CommitAppearanceRevision should require a reason")
	}
}

func TestVersionAssetsRef(t *testing.T) {
	pkg := newTestPackage(t)
	_, err := CommitAppearanceRevision(pkg, "v2 assets")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := Get(pkg, "v2")
	if err != nil {
		t.Fatal(err)
	}
	if v2.AssetsRef == "" {
		t.Error("version should carry an assets reference")
	}
}

func TestGetUnknownVersion(t *testing.T) {
	pkg := newTestPackage(t)
	if _, err := Get(pkg, "v99"); err == nil {
		t.Fatal("Get should fail for unknown version")
	}
}
