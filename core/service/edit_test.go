package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/oframe/character-workbench/core/edit"
	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/version"
)

// acceptedDirectionSvc returns a service + package + motion where the best
// candidate of the motion has been ACCEPTED, so its frames exist as current
// animation assets on disk (the edit target of EditDirection).
func acceptedDirectionSvc(t *testing.T) (*Service, string, string, string) {
	t.Helper()
	svc, root, mID := acceptThresholdSvc(t)
	cands := svc.CandidateList(root)
	best := cands[0]
	for _, c := range cands {
		if c.Scores.Overall > best.Scores.Overall {
			best = c
		}
	}
	dec, err := svc.CandidateDecide(context.Background(), root, best.ID, true, "accept for edit test")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Decision != identity.CandidateAccepted {
		t.Fatalf("acceptance failed: %+v", dec)
	}
	return svc, root, mID, best.Direction
}

func frameFile(t *testing.T, root, mID, dir string, index int) []byte {
	t.Helper()
	pkg, err := identity.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	assets, err := version.CurrentAssetsDir(pkg)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(assets, mID, dir, frameAssetName(index)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestEditDirectionSequenceMetadataAndReplay verifies the service-level edit
// wiring (editing spec 7.x): duration, reorder, delete, anchor-delta and batch
// cleanup all persist to the motion + asset files, stale files are removed,
// and every edit is appended to the operation log as a replayable instruction.
func TestEditDirectionSequenceMetadataAndReplay(t *testing.T) {
	svc, root, mID, dir := acceptedDirectionSvc(t)

	mv, err := svc.MotionGet(root, mID)
	if err != nil {
		t.Fatal(err)
	}
	before := len(mv.Direction(dir).Sequence.Frames)
	if before < 3 {
		t.Fatalf("expected at least 3 frames to edit, got %d", before)
	}

	// snapshot original frame files for byte-level reorder verification.
	orig := map[int][]byte{}
	for i := 0; i < before; i++ {
		orig[i] = frameFile(t, root, mID, dir, i)
	}

	// 1) duration edit on frame 0 (rhythm).
	res, err := svc.EditDirection(root, mID, dir, []edit.Instruction{{Kind: "duration", FrameIndex: 0, DurationMs: 150}})
	if err != nil {
		t.Fatalf("duration edit: %v", err)
	}
	if res.FrameCount != before || res.DurationsMs[0] != 150 {
		t.Fatalf("duration result: %+v", res)
	}
	mv, _ = svc.MotionGet(root, mID)
	if got := mv.Direction(dir).Sequence.Frames[0].DurationMs; got != 150 {
		t.Fatalf("duration not persisted: %d", got)
	}

	// 2) reorder: [2,0,1,...] — frame files must follow the new order.
	order := make([]int, before)
	for i := range order {
		order[i] = i
	}
	order[0], order[1], order[2] = 2, 0, 1
	if _, err := svc.EditDirection(root, mID, dir, []edit.Instruction{{Kind: "reorder", Order: order}}); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	if !bytes.Equal(frameFile(t, root, mID, dir, 0), orig[2]) {
		t.Fatal("reorder: frame_000.png should equal the old frame_002.png")
	}
	if !bytes.Equal(frameFile(t, root, mID, dir, 1), orig[0]) {
		t.Fatal("reorder: frame_001.png should equal the old frame_000.png")
	}

	// 3) delete frame 1 → count shrinks, stale file removed.
	res3, err := svc.EditDirection(root, mID, dir, []edit.Instruction{{Kind: "delete", FrameIndex: 1}})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res3.FrameCount != before-1 {
		t.Fatalf("delete frame count = %d, want %d", res3.FrameCount, before-1)
	}
	// the stale file (old index before-1, now out of range) must be gone
	pkg, err := identity.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	assets, _ := version.CurrentAssetsDir(pkg)
	stale := filepath.Join(assets, mID, dir, frameAssetName(before-1))
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale frame file still exists: %s", stale)
	}

	// 4) anchor-delta applied to all frames of the direction set. The core
	// behavior is covered by core/edit tests; here we verify the service path:
	// it runs without error and, when the synthetic pipeline attached anchors
	// (identity-level anchors present), they are translated by the delta.
	beforeAnchors := mv.Direction(dir).Sequence.Frames[0].Anchors
	if _, err := svc.EditDirection(root, mID, dir, []edit.Instruction{{Kind: "anchor-delta", DeltaX: 2, DeltaY: 3}}); err != nil {
		t.Fatalf("anchor-delta: %v", err)
	}
	if len(beforeAnchors) > 0 {
		mv, _ = svc.MotionGet(root, mID)
		for _, f := range mv.Direction(dir).Sequence.Frames {
			for _, a := range f.Anchors {
				if a.X != beforeAnchors[0].X+2 || a.Y != beforeAnchors[0].Y+3 {
					t.Fatalf("anchor not translated: %+v", a)
				}
			}
		}
	}

	// 5) batch cleanup (uniform background removal across the direction set)
	// runs without error (frames are fully opaque, so pixels are unchanged,
	// but the instruction is recorded).
	if _, err := svc.EditDirection(root, mID, dir, []edit.Instruction{{Kind: "cleanup"}}); err != nil {
		t.Fatalf("batch cleanup: %v", err)
	}

	// 6) operation log carries the edit instructions (replayable).
	entries, err := svc.OperationLog(root)
	if err != nil {
		t.Fatal(err)
	}
	editCount := 0
	for _, e := range entries {
		if e.Action == version.ActionEdit {
			editCount++
		}
	}
	if editCount < 5 {
		t.Fatalf("expected at least 5 edit log entries, got %d", editCount)
	}

	// 7) the edited direction still exports and validates.
	out := filepath.Join(t.TempDir(), "out")
	if _, err := svc.ExportPackage(root, out, "generic", ""); err != nil {
		t.Fatalf("export after edit: %v", err)
	}
}

// TestEditDirectionRejectsBadInstructions verifies instruction validation:
// out-of-range frames, bad order, and unknown kinds fail without corrupting.
func TestEditDirectionRejectsBadInstructions(t *testing.T) {
	svc, root, mID, dir := acceptedDirectionSvc(t)
	mv, _ := svc.MotionGet(root, mID)
	before := len(mv.Direction(dir).Sequence.Frames)

	if _, err := svc.EditDirection(root, mID, dir, []edit.Instruction{{Kind: "delete", FrameIndex: before}}); err == nil {
		t.Fatal("delete out of range should fail")
	}
	if _, err := svc.EditDirection(root, mID, dir, []edit.Instruction{{Kind: "reorder", Order: []int{0, 0, 1}}}); err == nil {
		t.Fatal("invalid reorder should fail")
	}
	if _, err := svc.EditDirection(root, mID, dir, []edit.Instruction{{Kind: "bogus"}}); err == nil {
		t.Fatal("unknown instruction should fail")
	}
	if _, err := svc.EditDirection(root, mID, dir, nil); err == nil {
		t.Fatal("empty instruction list should fail")
	}

	// Nothing changed.
	mv, _ = svc.MotionGet(root, mID)
	if got := len(mv.Direction(dir).Sequence.Frames); got != before {
		t.Fatalf("frames changed after rejected edits: %d != %d", got, before)
	}
}

func frameAssetName(index int) string {
	return fmt.Sprintf("frame_%03d.png", index)
}
