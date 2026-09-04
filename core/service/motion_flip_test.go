// Service-level acceptance of motion direction horizontal flip (九宫格右键
// 水平翻转): the owning candidate's filmstrip/frames/anchors flip, the mirror
// pair's sequence anchors re-derive coherently, flipping a DERIVED direction
// redirects to the same source pixels, a second flip restores everything, and
// accepted asset snapshots flip together with the motion sequences.
package service

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/motion"
	"github.com/oframe/character-workbench/core/pipeline"
	"github.com/oframe/character-workbench/core/version"
)

func flipTestPNG(t *testing.T, img *image.RGBA) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// flipTestFrame builds a 32×32 frame with a red band on the left (x 2..10)
// and a blue band on the right (x 21..29) — asymmetric, so a flip is visible.
func flipTestFrame(t *testing.T) *image.RGBA {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	for y := 10; y <= 20; y++ {
		for x := 2; x <= 10; x++ {
			img.SetRGBA(x, y, red)
		}
		for x := 21; x <= 29; x++ {
			img.SetRGBA(x, y, blue)
		}
	}
	return img
}

// flipTestPackage wires one motion (4 directions + mirror) whose right
// direction holds a generated candidate and whose left direction is mirror-
// derived from it. Returns the pkg path and motion id.
func flipTestPackage(t *testing.T, svc *Service) (string, string) {
	t.Helper()
	pkgPath := newTestPackage(t)
	m, err := svc.MotionCreate(pkgPath, "walk-flip", motion.DirectionStrategy{Count: 4, Mirror: true})
	if err != nil {
		t.Fatal(err)
	}

	frame := flipTestFrame(t)
	strip := image.NewRGBA(image.Rect(0, 0, 64, 32))
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	for y := 8; y <= 24; y++ {
		for x := 4; x <= 12; x++ {
			strip.SetRGBA(x, y, red)
		}
		for x := 50; x <= 58; x++ {
			strip.SetRGBA(x, y, blue)
		}
	}
	cand := pipeline.Candidate{
		ID: "cand-flip-1", FilmstripPNG: flipTestPNG(t, strip),
		Frames:     []*image.RGBA{frame},
		AnchorSets: [][]pipeline.AnchorPoint{{{Name: "foot", X: 6, Y: 30}}},
		Direction:  motion.DirectionRight, Status: pipeline.CandidatePending,
	}
	if err := pipeline.SaveCandidate(filepath.Join(pkgPath, identity.DirCandidates, cand.ID), cand); err != nil {
		t.Fatal(err)
	}

	seq := motion.FrameSequence{Direction: motion.DirectionRight, Frames: []motion.Frame{{
		Index: 0, AssetRef: "candidate:cand-flip-1:frame:0", DurationMs: 100,
		Anchors: []pipeline.AnchorPoint{{Name: "foot", X: 6, Y: 30}},
	}}}
	st := motion.NewStore(pkgPath)
	ms, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	mm, err := ms.Get(m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := mm.SetDirectionSequence(motion.DirectionRight, seq, motion.OriginGenerated); err != nil {
		t.Fatal(err)
	}
	mseq, err := motion.MirrorSequence(seq, 32)
	if err != nil {
		t.Fatal(err)
	}
	if err := mm.SetDirectionSequence(motion.DirectionLeft, mseq, motion.OriginMirrored); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(ms); err != nil {
		t.Fatal(err)
	}
	return pkgPath, m.ID
}

// flipAnchors reads the stored anchors of a direction from motions.json.
func flipAnchors(t *testing.T, pkgPath, motionID, dir string) pipeline.AnchorPoint {
	t.Helper()
	ms, err := motion.NewStore(pkgPath).Load()
	if err != nil {
		t.Fatal(err)
	}
	m, err := ms.Get(motionID)
	if err != nil {
		t.Fatal(err)
	}
	d := m.Direction(dir)
	if d == nil || len(d.Sequence.Frames) == 0 || len(d.Sequence.Frames[0].Anchors) == 0 {
		t.Fatalf("direction %s has no anchored frame", dir)
	}
	return d.Sequence.Frames[0].Anchors[0]
}

func TestMotionFlipDirectionFlipsCandidateAndPairAnchors(t *testing.T) {
	svc, _ := newTestService(t, nil)
	pkgPath, motionID := flipTestPackage(t, svc)

	// Flip the SOURCE direction (right).
	msg, err := svc.MotionFlipDirection(pkgPath, motionID, motion.DirectionRight)
	if err != nil {
		t.Fatalf("flip right: %v", err)
	}
	if msg == "" {
		t.Fatal("empty confirmation message")
	}

	// Candidate on disk: frames and filmstrip mirrored.
	cand, err := pipeline.LoadCandidate(filepath.Join(pkgPath, identity.DirCandidates, "cand-flip-1"))
	if err != nil {
		t.Fatal(err)
	}
	blue := color.RGBA{B: 255, A: 255}
	red := color.RGBA{R: 255, A: 255}
	if got := cand.Frames[0].RGBAAt(4, 15); got != blue {
		t.Fatalf("frame left band after flip = %v, want blue (mirrored)", got)
	}
	if got := cand.Frames[0].RGBAAt(25, 15); got != red {
		t.Fatalf("frame right band after flip = %v, want red (mirrored)", got)
	}
	strip, err := pipeline.DecodeImageAny(cand.FilmstripPNG)
	if err != nil {
		t.Fatal(err)
	}
	if got := strip.RGBAAt(6, 16); got != blue {
		t.Fatalf("filmstrip left band after flip = %v, want blue (mirrored)", got)
	}
	if got := cand.AnchorSets[0][0]; cand.AnchorSets[0][0].X != 25 || got.Y != 30 || got.Name != "foot" {
		t.Fatalf("candidate anchors after flip = %+v, want foot(25,30)", got)
	}

	// In-memory cache serves the flipped pixels too (no stale reads).
	cached, err := svc.findCandidate(pkgPath, "cand-flip-1")
	if err != nil {
		t.Fatal(err)
	}
	if cached.Frames[0].RGBAAt(4, 15) != blue {
		t.Fatal("in-memory candidate cache kept stale pre-flip pixels")
	}

	// Motion anchors: owner mirrored, derived sibling re-derived to the
	// ORIGINAL owner anchors (it now displays the pre-flip pixels).
	if a := flipAnchors(t, pkgPath, motionID, motion.DirectionRight); a.X != 25 || a.Y != 30 {
		t.Fatalf("right anchors after flip = (%d,%d), want (25,30)", a.X, a.Y)
	}
	if a := flipAnchors(t, pkgPath, motionID, motion.DirectionLeft); a.X != 6 || a.Y != 30 {
		t.Fatalf("left anchors after flip = (%d,%d), want (6,30)", a.X, a.Y)
	}

	// Flip the DERIVED direction (left) — same source pixels, everything
	// reverts (the flip is its own inverse).
	if _, err := svc.MotionFlipDirection(pkgPath, motionID, motion.DirectionLeft); err != nil {
		t.Fatalf("flip left: %v", err)
	}
	cand, err = pipeline.LoadCandidate(filepath.Join(pkgPath, identity.DirCandidates, "cand-flip-1"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cand.Frames[0].RGBAAt(4, 15); got != red {
		t.Fatalf("double flip did not restore frame: %v", got)
	}
	if a := flipAnchors(t, pkgPath, motionID, motion.DirectionRight); a.X != 6 || a.Y != 30 {
		t.Fatalf("right anchors after double flip = (%d,%d), want (6,30)", a.X, a.Y)
	}
	if a := flipAnchors(t, pkgPath, motionID, motion.DirectionLeft); a.X != 25 || a.Y != 30 {
		t.Fatalf("left anchors after double flip = (%d,%d), want (25,30)", a.X, a.Y)
	}

	// Rhythm is untouched by a flip.
	ms, err := motion.NewStore(pkgPath).Load()
	if err != nil {
		t.Fatal(err)
	}
	m, _ := ms.Get(motionID)
	if got := m.Direction(motion.DirectionRight).Sequence.Frames[0].DurationMs; got != 100 {
		t.Fatalf("duration changed by flip: %d", got)
	}

	// A never-generated direction must be refused.
	if _, err := svc.MotionFlipDirection(pkgPath, motionID, motion.DirectionUp); err == nil {
		t.Fatal("flip of a non-generated direction must fail")
	}
}

func TestMotionFlipDirectionSyncsAcceptedSnapshots(t *testing.T) {
	svc, _ := newTestService(t, nil)
	pkgPath, motionID := flipTestPackage(t, svc)

	pkg, err := identity.Open(pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	assetsDir, err := version.CurrentAssetsDir(pkg)
	if err != nil {
		t.Skipf("package has no current assets area: %v", err)
	}
	framePNG := flipTestPNG(t, flipTestFrame(t))
	for _, dir := range []string{motion.DirectionRight, motion.DirectionLeft} {
		if err := version.AcceptAssets(pkg, motionID, dir, "cand-flip-1", []version.AssetFrame{{Index: 0, PNG: framePNG}}); err != nil {
			t.Fatal(err)
		}
	}

	blue := color.RGBA{B: 255, A: 255}
	red := color.RGBA{R: 255, A: 255}
	snapshotPixel := func(t *testing.T, dir string) (color.RGBA, color.RGBA) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(assetsDir, motionID, dir, "frame_000.png"))
		if err != nil {
			t.Fatal(err)
		}
		img, err := pipeline.DecodeImageAny(data)
		if err != nil {
			t.Fatal(err)
		}
		return img.RGBAAt(4, 15), img.RGBAAt(25, 15)
	}

	// Flip: BOTH snapshots flip (each member of the pair visually flips once).
	if _, err := svc.MotionFlipDirection(pkgPath, motionID, motion.DirectionRight); err != nil {
		t.Fatal(err)
	}
	if left, right := snapshotPixel(t, motion.DirectionRight); left != blue || right != red {
		t.Fatalf("right snapshot not flipped: %v / %v", left, right)
	}
	if left, right := snapshotPixel(t, motion.DirectionLeft); left != blue || right != red {
		t.Fatalf("left snapshot not flipped: %v / %v", left, right)
	}

	// Flip back: snapshots restored.
	if _, err := svc.MotionFlipDirection(pkgPath, motionID, motion.DirectionLeft); err != nil {
		t.Fatal(err)
	}
	if left, right := snapshotPixel(t, motion.DirectionRight); left != red || right != blue {
		t.Fatalf("right snapshot not restored: %v / %v", left, right)
	}
	if left, right := snapshotPixel(t, motion.DirectionLeft); left != red || right != blue {
		t.Fatalf("left snapshot not restored: %v / %v", left, right)
	}
}
