package identity

import (
	"path/filepath"
	"testing"
)

// 阶段 3 reference-image semantics: 1 主参考图 + 最多 2 辅助参考图.

// TestReferenceRoleBounds verifies the role bounds: a second main reference is
// rejected and a third auxiliary reference is rejected; the valid 1+2 layout is
// accepted and survives a re-open.
func TestReferenceRoleBounds(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}

	mk := func(name string) string { return writeTempFile(t, name, []byte("png")) }

	main, err := pkg.AddReferenceImage(mk("m.png"), "主参考图", RoleMainReference)
	if err != nil {
		t.Fatalf("add main reference: %v", err)
	}

	// Second main reference must be rejected.
	if _, err := pkg.AddReferenceImage(mk("m2.png"), "第二个主参考", RoleMainReference); err == nil {
		t.Fatal("expected error for a second main reference image")
	}

	// Two auxiliary references are fine…
	aux1, err := pkg.AddReferenceImage(mk("a1.png"), "辅助1", RoleAuxiliaryReference)
	if err != nil {
		t.Fatalf("add aux 1: %v", err)
	}
	aux2, err := pkg.AddReferenceImage(mk("a2.png"), "辅助2", RoleAuxiliaryReference)
	if err != nil {
		t.Fatalf("add aux 2: %v", err)
	}

	// …but a third is rejected.
	if _, err := pkg.AddReferenceImage(mk("a3.png"), "辅助3", RoleAuxiliaryReference); err == nil {
		t.Fatal("expected error for a third auxiliary reference image")
	}

	if err := pkg.ValidateReferenceRoles(); err != nil {
		t.Fatalf("valid role layout rejected: %v", err)
	}

	// Re-open: roles persist in the manifest.
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	mats := reopened.ReferenceImages()
	if len(mats) != 3 {
		t.Fatalf("reference images = %d, want 3", len(mats))
	}
	// Ordered: main first, then auxiliaries.
	if mats[0].ID != main.ID || mats[0].Role != RoleMainReference {
		t.Errorf("first reference not main: %+v", mats[0])
	}
	gotAux := map[string]string{aux1.ID: aux1.Role, aux2.ID: aux2.Role}
	for _, m := range mats[1:] {
		if gotAux[m.ID] != RoleAuxiliaryReference {
			t.Errorf("auxiliary reference role lost: %+v", m)
		}
	}
}

// TestSetMaterialRole verifies re-role: promoting an auxiliary to main fails
// when a main already exists and succeeds after the old main is demoted; a
// sprite cannot carry a reference role.
func TestSetMaterialRole(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	mk := func(name string) string { return writeTempFile(t, name, []byte("png")) }

	main, err := pkg.AddReferenceImage(mk("m.png"), "主", RoleMainReference)
	if err != nil {
		t.Fatal(err)
	}
	aux, err := pkg.AddReferenceImage(mk("a.png"), "辅", RoleAuxiliaryReference)
	if err != nil {
		t.Fatal(err)
	}

	// Promoting aux while a main exists → rejected.
	if _, err := pkg.SetMaterialRole(aux.ID, RoleMainReference); err == nil {
		t.Fatal("expected error promoting aux while main exists")
	}

	// Demote the main first, then promote the aux.
	if _, err := pkg.SetMaterialRole(main.ID, RoleAuxiliaryReference); err != nil {
		t.Fatalf("demote main: %v", err)
	}
	promoted, err := pkg.SetMaterialRole(aux.ID, RoleMainReference)
	if err != nil {
		t.Fatalf("promote aux: %v", err)
	}
	if promoted.Role != RoleMainReference {
		t.Errorf("promoted role = %q", promoted.Role)
	}

	// Sprite cannot be re-rolled.
	sprite, err := pkg.ImportSprite(mk("s.png"), "精灵")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pkg.SetMaterialRole(sprite.ID, RoleMainReference); err == nil {
		t.Fatal("expected error re-roling a sprite material")
	}

	if err := pkg.ValidateReferenceRoles(); err != nil {
		t.Fatalf("layout invalid after re-role: %v", err)
	}
}

// TestAddReferenceImageRejectsInvalidRole verifies the role argument is
// validated at the entry point.
func TestAddReferenceImageRejectsInvalidRole(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pkg.AddReferenceImage(writeTempFile(t, "r.png", []byte("png")), "r", "bogus"); err == nil {
		t.Fatal("expected error for invalid reference role")
	}
}
