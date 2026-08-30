package identity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDraftSaveLoadClear(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hero.oframe")
	p, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}

	// Fresh package: zero draft, no file.
	d, err := p.LoadDraft()
	if err != nil || !d.Empty() {
		t.Fatalf("fresh draft = %+v err=%v, want empty", d, err)
	}

	mirror := false
	if err := p.SaveDraft(Draft{Description: "未保存的描述", MotionName: "walk", MotionCount: 4, MotionMirror: &mirror}); err != nil {
		t.Fatal(err)
	}

	// Restart continuity: a fresh Open sees the same draft.
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	d, err = reopened.LoadDraft()
	if err != nil {
		t.Fatal(err)
	}
	if d.Description != "未保存的描述" || d.MotionName != "walk" || d.MotionCount != 4 || d.MotionMirror == nil || *d.MotionMirror != false {
		t.Fatalf("restored draft = %+v", d)
	}

	// The draft sidecar must never leak into the manifest.
	m, _ := json.Marshal(reopened.Manifest())
	if len(m) == 0 {
		t.Fatal("manifest empty")
	}

	// Clear removes the file; loading afterwards is empty again.
	if err := reopened.ClearDraft(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, DraftFileName)); !os.IsNotExist(err) {
		t.Fatalf("draft file still present after clear: %v", err)
	}
	d, err = reopened.LoadDraft()
	if err != nil || !d.Empty() {
		t.Fatalf("post-clear draft = %+v err=%v, want empty", d, err)
	}
}

func TestCorruptDraftDropsSilently(t *testing.T) {
	root := filepath.Join(t.TempDir(), "hero.oframe")
	p, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, DraftFileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := p.LoadDraft()
	if err != nil || !d.Empty() {
		t.Fatalf("corrupt draft = %+v err=%v, want empty without error", d, err)
	}
	if _, err := os.Stat(filepath.Join(root, DraftFileName)); !os.IsNotExist(err) {
		t.Fatal("corrupt draft file was not removed")
	}
	// The package itself stays openable and unaffected.
	if _, err := Open(root); err != nil {
		t.Fatalf("package unreadable after corrupt draft: %v", err)
	}
}
