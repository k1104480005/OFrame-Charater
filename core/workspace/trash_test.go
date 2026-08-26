package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
)

// TestTrashPackageMovesToDotTrash verifies a deleted package is moved into
// <root>/.trash (recoverable), disappears from List(), and non-package or
// out-of-workspace paths are refused.
func TestTrashPackageMovesToDotTrash(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ws")
	ws, err := Init(root)
	if err != nil {
		t.Fatal(err)
	}
	pkgDir := filepath.Join(root, "Hero")
	if _, err := identity.Create(pkgDir, "Hero"); err != nil {
		t.Fatal(err)
	}

	dst, err := ws.TrashPackage(pkgDir)
	if err != nil {
		t.Fatalf("TrashPackage: %v", err)
	}
	if filepath.Dir(dst) != filepath.Join(root, DirTrash) {
		t.Fatalf("expected trash under %s, got %s", DirTrash, dst)
	}
	if _, err := os.Stat(pkgDir); !os.IsNotExist(err) {
		t.Fatalf("original package dir should be gone, stat err=%v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("trashed package should exist: %v", err)
	}

	// The package no longer shows up in the workspace list.
	list, err := ws.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty package list after trash, got %d", len(list))
	}

	// Refusals: not a package / outside workspace.
	if _, err := ws.TrashPackage(filepath.Join(root, "not-a-package")); err == nil {
		t.Fatal("trashing a non-package directory must fail")
	}
	outside := filepath.Join(t.TempDir(), "elsewhere")
	if _, err := ws.TrashPackage(outside); err == nil {
		t.Fatal("trashing a path outside the workspace must fail")
	}
}
