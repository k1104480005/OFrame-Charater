package identity

import (
	"path/filepath"
	"testing"
)

// Task 2.4: logical canvas specification — set, write to manifest, and be the
// reference for frame conformance validation used by motions/frame sequences.

func TestSetLogicalCanvas(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.LogicalCanvas() != nil {
		t.Fatal("canvas should be unset initially")
	}
	if err := pkg.SetLogicalCanvas(16, 32); err != nil {
		t.Fatalf("SetLogicalCanvas: %v", err)
	}
	c := pkg.LogicalCanvas()
	if c == nil || c.UnitWidth != 16 || c.UnitHeight != 32 {
		t.Fatalf("canvas = %+v", c)
	}
	if !c.Contains(0, 0) || !c.Contains(15, 31) || c.Contains(16, 0) {
		t.Errorf("canvas range wrong: %+v", c.CoordinateRange)
	}
	// Written to manifest and readable after re-open.
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	m := reopened.Manifest()
	if m.LogicalCanvas == nil || m.LogicalCanvas.UnitWidth != 16 || m.LogicalCanvas.UnitHeight != 32 {
		t.Errorf("canvas not persisted: %+v", m.LogicalCanvas)
	}
}

func TestSetLogicalCanvasRejectsInvalid(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetLogicalCanvas(0, 32); err == nil {
		t.Fatal("SetLogicalCanvas should reject non-positive size")
	}
	if pkg.LogicalCanvas() != nil {
		t.Fatal("failed SetLogicalCanvas must not leave a canvas behind")
	}
}

func TestCanvasValidateFrame(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	// Conformance check without a canvas set: refuse.
	if err := pkg.ValidateFrame(16, 16); err == nil {
		t.Fatal("ValidateFrame should fail when canvas is unset")
	}
	if err := pkg.SetLogicalCanvas(16, 16); err != nil {
		t.Fatal(err)
	}
	if err := pkg.ValidateFrame(16, 16); err != nil {
		t.Errorf("conforming frame rejected: %v", err)
	}
	if err := pkg.ValidateFrame(32, 16); err == nil {
		t.Error("non-conforming frame must be rejected")
	}
	if err := pkg.ValidateFrame(16, 32); err == nil {
		t.Error("non-conforming frame must be rejected")
	}
}

func TestNewCanvasSpec(t *testing.T) {
	if _, err := NewCanvasSpec(0, 0); err == nil {
		t.Fatal("NewCanvasSpec should reject zero size")
	}
	c, err := NewCanvasSpec(8, 8)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
	bad := &CanvasSpec{UnitWidth: 8, UnitHeight: 8, CoordinateRange: CoordinateRange{XMin: 1, YMin: 0, XMax: 7, YMax: 7}}
	if err := bad.Validate(); err == nil {
		t.Error("Validate should reject mismatched range")
	}
}
