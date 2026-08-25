package identity

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// Task 1.4: manifest data structure — format version, identity metadata,
// logical canvas, anchors, asset references, candidate/log references;
// serialization and version field round-trips.

func sampleManifest() *Manifest {
	now := time.Now().UTC()
	return &Manifest{
		FormatVersion: FormatVersion,
		Identity: IdentityMeta{
			ID: "pkg-1", Name: "Hero", Description: "a pixel hero",
			EntryKind: EntryKindReferenceImage, EntryMaterialID: "mat-1",
			CreatedAt: now, UpdatedAt: now,
		},
		LogicalCanvas: &CanvasSpec{UnitWidth: 32, UnitHeight: 32, CoordinateRange: DefaultCanvasRange(32, 32)},
		Anchors: []Anchor{{
			ID: "an-1", Name: "脚底", Preset: PresetFeet.ID, X: 16, Y: 31,
			CoordinateRange: DefaultCanvasRange(32, 32),
		}},
		Materials: []Material{{
			ID: "mat-1", Kind: MaterialKindReferenceImage, Name: "ref.png",
			Path: filepath.ToSlash(filepath.Join(DirMaterials, "mat-1.png")), AddedAt: now,
		}},
		References: DefaultReferences(),
		Versions: Versions{
			Current: InitialVersionID,
			Items:   []VersionRecord{{ID: InitialVersionID, CreatedAt: now, Reason: "initial", Immutable: true}},
		},
	}
}

func TestManifestRoundTrip(t *testing.T) {
	m := sampleManifest()
	data, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest: %v", err)
	}
	if got.FormatVersion != FormatVersion {
		t.Errorf("FormatVersion = %d, want %d", got.FormatVersion, FormatVersion)
	}
	if got.Identity.Name != "Hero" || got.Identity.Description != "a pixel hero" {
		t.Errorf("identity metadata not round-tripped: %+v", got.Identity)
	}
	if got.LogicalCanvas == nil || got.LogicalCanvas.UnitWidth != 32 || got.LogicalCanvas.UnitHeight != 32 {
		t.Errorf("logical canvas not round-tripped: %+v", got.LogicalCanvas)
	}
	if len(got.Anchors) != 1 || got.Anchors[0].Name != "脚底" {
		t.Errorf("anchors not round-tripped: %+v", got.Anchors)
	}
	if len(got.Materials) != 1 || got.Materials[0].Kind != MaterialKindReferenceImage {
		t.Errorf("materials not round-tripped: %+v", got.Materials)
	}
	if got.References.CandidateHistory == "" || got.References.OperationLog == "" {
		t.Errorf("candidate/log references not present: %+v", got.References)
	}
	if got.Versions.Current != InitialVersionID || len(got.Versions.Items) != 1 || !got.Versions.Items[0].Immutable {
		t.Errorf("versions not round-tripped: %+v", got.Versions)
	}
}

func TestManifestVersionFieldReadWrite(t *testing.T) {
	// The format version is the compatibility contract: write v1, read v1.
	m := sampleManifest()
	data, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	var raw struct {
		FormatVersion int `json:"formatVersion"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if raw.FormatVersion != 1 {
		t.Errorf("formatVersion field = %d, want 1", raw.FormatVersion)
	}
}

func TestManifestEncodeIndentedStable(t *testing.T) {
	m := sampleManifest()
	a, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	b, err := m.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if string(a) != string(b) {
		t.Error("manifest encoding is not deterministic")
	}
}

func TestDecodeManifestRejectsGarbage(t *testing.T) {
	if _, err := DecodeManifest([]byte("not json")); err == nil {
		t.Fatal("DecodeManifest should reject invalid JSON")
	}
	if _, err := DecodeManifest([]byte(`{"formatVersion": "x"}`)); err == nil {
		t.Fatal("DecodeManifest should reject wrong-typed fields")
	}
}

func TestCoordinateRange(t *testing.T) {
	r := DefaultCanvasRange(16, 16)
	if !r.Contains(0, 0) || !r.Contains(15, 15) || r.Contains(16, 0) || r.Contains(0, -1) {
		t.Errorf("range %v containment wrong", r)
	}
	if r.Width() != 16 || r.Height() != 16 {
		t.Errorf("range size = %dx%d, want 16x16", r.Width(), r.Height())
	}
}
