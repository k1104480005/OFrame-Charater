package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/oframe/character-workbench/core/assetexport"
)

// TestExampleGenProducesValidPackage verifies the example generator (13.1):
// running it produces a valid identity package and validated generic/godot
// export packages with the expected accepted animation set (right/up/down for
// a 4-direction mirrored walk). Runs entirely offline on synthetic filmstrips.
func TestExampleGenProducesValidPackage(t *testing.T) {
	out := filepath.Join(t.TempDir(), "Hero")
	exports := filepath.Join(t.TempDir(), "exports")
	if err := run(out, "Hero", exports, true); err != nil {
		t.Fatalf("examplegen run: %v", err)
	}

	// Identity package: manifest exists and is a real identity package.
	manifestPath := filepath.Join(out, "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("identity manifest missing: %v", err)
	}

	// Exports validate (frames, anchors, manifest completeness).
	for _, target := range []string{assetexport.TargetGeneric, assetexport.TargetGodot} {
		dir := filepath.Join(exports, target)
		if err := assetexport.Validate(dir); err != nil {
			t.Fatalf("%s export invalid: %v", target, err)
		}
	}

	// The generic manifest carries the 3 accepted basic directions, each with
	// the example frame count (4), 32×32 cell.
	data, err := os.ReadFile(filepath.Join(exports, assetexport.TargetGeneric, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m assetexport.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Animations) != 3 || m.CellWidth != 32 || m.CellHeight != 32 {
		t.Fatalf("export manifest: %+v", m)
	}
	dirs := map[string]int{}
	for _, a := range m.Animations {
		dirs[a.Direction] = len(a.Frames)
	}
	for _, d := range []string{"right", "up", "down"} {
		if dirs[d] != exampleFrames {
			t.Fatalf("animation %s frames = %d, want %d (dirs: %v)", d, dirs[d], exampleFrames, dirs)
		}
	}
}
