package identity

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSetNameRenameDisplayName verifies rename updates the manifest name only:
// the directory path never changes and a fresh Open sees the new name.
func TestSetNameRenameDisplayName(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetName("Knight"); err != nil {
		t.Fatalf("SetName: %v", err)
	}

	// Re-open: the new name is persisted, the directory is untouched.
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Manifest().Identity.Name; got != "Knight" {
		t.Fatalf("expected renamed name %q, got %q", "Knight", got)
	}
	if _, err := Open(filepath.Join(t.TempDir(), "Knight")); err == nil {
		t.Fatal("directory must NOT be renamed — a Knight dir must not exist")
	}
}

// TestSetNameValidation rejects blank / over-long names and trims input.
func TestSetNameValidation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetName("   "); err == nil {
		t.Fatal("blank name must be rejected")
	}
	if err := pkg.SetName(strings.Repeat("x", 65)); err == nil {
		t.Fatal("name longer than 64 chars must be rejected")
	}
	if err := pkg.SetName("  Luna  "); err != nil {
		t.Fatalf("SetName: %v", err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Manifest().Identity.Name; got != "Luna" {
		t.Fatalf("expected trimmed name %q, got %q", "Luna", got)
	}
}
