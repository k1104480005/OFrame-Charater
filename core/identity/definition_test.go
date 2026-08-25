package identity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// Task 2.3: identity definition entry points — text description, reference
// image, existing sprite — written into identity package metadata / material
// area; saved content is readable after re-opening.

func TestTextDescriptionEntry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetTextDescription("一个红发像素勇者"); err != nil {
		t.Fatalf("SetTextDescription: %v", err)
	}
	if pkg.Description() != "一个红发像素勇者" {
		t.Errorf("Description = %q", pkg.Description())
	}
	// Re-open reads it back from metadata.
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	m := reopened.Manifest()
	if m.Identity.Description != "一个红发像素勇者" || m.Identity.EntryKind != EntryKindText {
		t.Errorf("description entry not persisted: %+v", m.Identity)
	}
}

func TestReferenceImageEntry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	src := writeTempFile(t, "ref.png", []byte("png-bytes"))
	mat, err := pkg.AddReferenceImage(src, "参考图")
	if err != nil {
		t.Fatalf("AddReferenceImage: %v", err)
	}
	if mat.Kind != MaterialKindReferenceImage {
		t.Errorf("material kind = %q", mat.Kind)
	}
	// The file exists inside the package material area.
	abs, err := pkg.MaterialPath(*mat)
	if err != nil {
		t.Fatalf("MaterialPath: %v", err)
	}
	if !strings.Contains(abs, string(filepath.Separator)+DirMaterials+string(filepath.Separator)) {
		t.Errorf("material stored outside material area: %s", abs)
	}
	if !fileExists(t, abs) {
		t.Fatalf("material file missing: %s", abs)
	}
	if data, _ := os.ReadFile(abs); string(data) != "png-bytes" {
		t.Errorf("material content mismatch")
	}
	// Re-open reads the reference.
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	mats := reopened.Materials()
	if len(mats) != 1 || mats[0].ID != mat.ID || mats[0].Kind != MaterialKindReferenceImage {
		t.Errorf("material reference not persisted: %+v", mats)
	}
	if m := reopened.Manifest(); m.Identity.EntryKind != EntryKindReferenceImage || m.Identity.EntryMaterialID != mat.ID {
		t.Errorf("entry reference not persisted: %+v", m.Identity)
	}
}

func TestImportSpriteEntry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	src := writeTempFile(t, "old.png", []byte("sprite-bytes"))
	mat, err := pkg.ImportSprite(src, "既有精灵")
	if err != nil {
		t.Fatalf("ImportSprite: %v", err)
	}
	if mat.Kind != MaterialKindSprite {
		t.Errorf("material kind = %q, want sprite", mat.Kind)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	m := reopened.Manifest()
	if m.Identity.EntryKind != EntryKindSprite || m.Identity.EntryMaterialID != mat.ID {
		t.Errorf("sprite entry not persisted: %+v", m.Identity)
	}
	if len(reopened.Materials()) != 1 {
		t.Errorf("materials = %d, want 1", len(reopened.Materials()))
	}
}

func TestAddMaterialRejectsMissingSource(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pkg.AddReferenceImage(filepath.Join(t.TempDir(), "nope.png"), "x"); err == nil {
		t.Fatal("AddReferenceImage should fail for a missing source")
	}
	if len(pkg.Materials()) != 0 {
		t.Error("failed material add must not leave references")
	}
}

func TestMaterialLookup(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	mat, err := pkg.ImportSprite(writeTempFile(t, "s.png", []byte("x")), "s")
	if err != nil {
		t.Fatal(err)
	}
	got, err := pkg.Material(mat.ID)
	if err != nil || got.ID != mat.ID {
		t.Fatalf("Material(%q) = %+v, %v", mat.ID, got, err)
	}
	if _, err := pkg.Material("missing"); err == nil {
		t.Fatal("Material should fail for unknown id")
	}
}
