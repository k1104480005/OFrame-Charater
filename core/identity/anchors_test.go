package identity

import (
	"path/filepath"
	"testing"
)

// Task 2.5: anchor definitions and presets (脚底/手持点等) — anchors and their
// coordinate range are written to the manifest and referenceable by ID.

func TestAnchorPresets(t *testing.T) {
	ps := AnchorPresets()
	if len(ps) != 7 {
		t.Fatalf("presets = %d, want 7", len(ps))
	}
	names := map[string]string{}
	for _, p := range ps {
		names[p.ID] = p.Name
	}
	if names[PresetFeet.ID] != "脚底" {
		t.Errorf("feet preset name = %q", names[PresetFeet.ID])
	}
	if names[PresetCenter.ID] != "画布中心" {
		t.Errorf("center preset name = %q", names[PresetCenter.ID])
	}
	if names[PresetHandLeft.ID] != "左侧挂载参考点" || names[PresetHandRight.ID] != "右侧挂载参考点" {
		t.Errorf("hand preset names wrong: %v", names)
	}
}

func TestDeleteAnchor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetLogicalCanvas(16, 16); err != nil {
		t.Fatal(err)
	}
	candidate, err := pkg.AddBaseCharacterCandidate("base.png", "test", "test", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.AdoptBaseCharacter(candidate.ID); err != nil {
		t.Fatal(err)
	}
	anchor, err := pkg.AddAnchorPreset(PresetCenter, "中心点")
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.DeleteAnchor(anchor.ID); err != nil {
		t.Fatalf("DeleteAnchor: %v", err)
	}
	if len(pkg.Anchors()) != 0 {
		t.Fatalf("anchors after delete = %d, want 0", len(pkg.Anchors()))
	}
}

func TestAddAnchorPreset(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetLogicalCanvas(16, 16); err != nil {
		t.Fatal(err)
	}
	candidate, err := pkg.AddBaseCharacterCandidate("base.png", "test", "test", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.AdoptBaseCharacter(candidate.ID); err != nil {
		t.Fatal(err)
	}
	a, err := pkg.AddAnchorPreset(PresetFeet, "")
	if err != nil {
		t.Fatalf("AddAnchorPreset: %v", err)
	}
	// 脚底 default = bottom center.
	if a.X != 8 || a.Y != 15 {
		t.Errorf("feet anchor = (%d,%d), want (8,15)", a.X, a.Y)
	}
	if a.Preset != PresetFeet.ID || a.CoordinateRange.Width() != 16 || a.CoordinateRange.Height() != 16 {
		t.Errorf("anchor range/preset wrong: %+v", a)
	}
	// Referenceable by ID after re-open (motions/direction sets reference by ID).
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Anchor(a.ID)
	if err != nil {
		t.Fatalf("Anchor(%q): %v", a.ID, err)
	}
	if got.Name != "脚底" || got.X != 8 || got.Y != 15 {
		t.Errorf("anchor not persisted: %+v", got)
	}
	if len(reopened.Anchors()) != 1 {
		t.Errorf("anchors = %d, want 1", len(reopened.Anchors()))
	}
}

func TestAddAnchorExplicitCoords(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetLogicalCanvas(32, 32); err != nil {
		t.Fatal(err)
	}
	a, err := pkg.AddAnchor("手持点", PresetHandRight, 31, 16)
	if err != nil {
		t.Fatalf("AddAnchor: %v", err)
	}
	if a.X != 31 || a.Y != 16 {
		t.Errorf("anchor = (%d,%d)", a.X, a.Y)
	}
	// Out-of-range coordinate refused.
	if _, err := pkg.AddAnchor("越界", PresetCenter, 32, 16); err == nil {
		t.Fatal("AddAnchor should reject out-of-range coordinates")
	}
}

func TestAddAnchorRequiresCanvas(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pkg.AddAnchorPreset(PresetFeet, ""); err == nil {
		t.Fatal("anchors require the logical canvas to be set")
	}
}

func TestAnchorNotFound(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pkg.Anchor("nope"); err == nil {
		t.Fatal("Anchor should fail for unknown id")
	}
}
