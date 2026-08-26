package identity

import (
	"path/filepath"
	"testing"
)

// TestSetCategoryPersistAndClear verifies category is persisted, trimmed, and
// cleared when set to a blank value.
func TestSetCategoryPersistAndClear(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetCategory("  角色/怪物  "); err != nil {
		t.Fatalf("SetCategory: %v", err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Manifest().Identity.Category; got != "角色/怪物" {
		t.Fatalf("expected trimmed category %q, got %q", "角色/怪物", got)
	}

	// blank clears the category
	if err := reopened.SetCategory("  "); err != nil {
		t.Fatalf("SetCategory(blank): %v", err)
	}
	reopened2, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened2.Manifest().Identity.Category; got != "" {
		t.Fatalf("expected cleared category, got %q", got)
	}

	// over-long rejected
	if err := reopened2.SetCategory(stringWithLen(33)); err == nil {
		t.Fatal("category longer than 32 chars must be rejected")
	}
}

func stringWithLen(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}
