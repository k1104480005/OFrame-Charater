package assetexport

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func pngFrame(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestBuildValidateAndHistory(t *testing.T) {
	dir := t.TempDir()
	animations := []Animation{
		{MotionID: "walk", Direction: "down", CandidateID: "cand-1", Frames: []Frame{
			{Index: 0, DurationMs: 100, Anchors: []Anchor{{ID: "feet", X: 1, Y: 1}}, PNG: pngFrame(t, color.RGBA{R: 255, A: 255})},
			{Index: 1, DurationMs: 120, Anchors: []Anchor{{ID: "feet", X: 1, Y: 1}}, PNG: pngFrame(t, color.RGBA{G: 255, A: 255})},
		}},
	}
	result, err := Build(filepath.Join(dir, "out"), TargetGeneric, "v1", animations)
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.CellWidth != 2 || len(result.Manifest.Animations) != 1 {
		t.Fatalf("unexpected manifest: %#v", result.Manifest)
	}
	for _, name := range []string{"manifest.json", "spritesheet.png", "generic.json", "frames/walk/down/frame_000.png"} {
		if _, err := os.Stat(filepath.Join(dir, "out", filepath.FromSlash(name))); err != nil {
			t.Fatalf("missing export file %s: %v", name, err)
		}
	}
	if err := Validate(filepath.Join(dir, "out")); err != nil {
		t.Fatal(err)
	}
	history := filepath.Join(dir, "history.jsonl")
	if err := RecordHistory(history, HistoryRecord{Target: TargetGeneric, IdentityVersion: "v1", OutputDir: "out", Result: "succeeded"}); err != nil {
		t.Fatal(err)
	}
	items, err := ReadHistory(history)
	if err != nil || len(items) != 1 || items[0].Target != TargetGeneric {
		t.Fatalf("history = %#v, err=%v", items, err)
	}
}

func TestRejectsInvalidExport(t *testing.T) {
	if err := ValidateTarget("unknown"); err == nil {
		t.Fatal("unknown target accepted")
	}
	if _, err := Build(t.TempDir(), TargetGodot, "v1", nil); err == nil {
		t.Fatal("empty animation export accepted")
	}
}
