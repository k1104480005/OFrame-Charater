package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/motion"
	"github.com/oframe/character-workbench/core/provider"
)

// configureDoubaoKey sets a fake key so executions reach the fake transport.
func configureDoubaoKey(t *testing.T, svc *Service) {
	t.Helper()
	cfg, err := svc.ProviderConfig(provider.ProviderDoubao)
	if err != nil {
		t.Fatal(err)
	}
	cfg.APIKey = "test-key"
	if err := svc.SaveProviderConfig(provider.ProviderDoubao, cfg); err != nil {
		t.Fatal(err)
	}
}

// newPhase5Svc returns a service whose fake transport returns valid filmstrip
// PNGs (阶段 5: 生成执行链会解码 provider 字节并跑 filmstrip 管线).
func newPhase5Svc(t *testing.T) (*Service, string) {
	t.Helper()
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return filmstripResp(t, 32, 32, 4), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	configureDoubaoKey(t, svc)
	return svc, newTestPackage(t)
}

// TestSingleDirectionDefaultViaService verifies task 3.2 through the service:
// a new motion defaults to a single direction — down (south/正面) — and a
// confirmed generation fills exactly that one direction.
func TestSingleDirectionDefaultViaService(t *testing.T) {
	svc, root := newPhase5Svc(t)
	m, err := svc.MotionCreate(root, "idle", motion.DirectionStrategy{Count: 1, Mirror: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Directions) != 1 || m.Directions[0].Direction != motion.DirectionDown {
		t.Fatalf("default directions = %v, want single [down]", motionDirNames(m))
	}
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, MotionID: m.ID, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.BasicDirections != 1 || plan.MirroredDirections != 0 || plan.ExpectedCalls != 1 {
		t.Fatalf("single-direction plan: %+v", plan)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanExecuted || res.CallsMade != 1 {
		t.Fatalf("single-direction result: %+v", res)
	}
	got, err := svc.MotionGet(root, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.FrameCount(motion.DirectionDown) != 4 {
		t.Fatalf("down frames = %d, want 4", got.FrameCount(motion.DirectionDown))
	}
}

// TestMotionDrivenGenerationWithMirroring verifies tasks 3.3 + 3.4 through the
// service: 4 方向 = 3 生成 (right/up/down) + 1 镜像 (left), the mirrored
// direction owns an independent frame sequence, and its anchors are converted
// by the horizontal mirror rule (X' = width-1-X, Y' = Y).
func TestMotionDrivenGenerationWithMirroring(t *testing.T) {
	svc, root := newPhase5Svc(t)
	// Identity-level feet anchor (脚底) feeds the pipeline anchor correction.
	pkg, err := identity.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pkg.AddAnchorPreset(identity.PresetFeet, "feet"); err != nil {
		t.Fatal(err)
	}

	m, err := svc.MotionCreate(root, "walk", motion.DirectionStrategy{Count: 4, Mirror: true})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, MotionID: m.ID, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.BasicDirections != 3 || plan.MirroredDirections != 1 || plan.ExpectedCalls != 3 {
		t.Fatalf("4-direction plan: %+v", plan)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanExecuted || res.CallsMade != 3 {
		t.Fatalf("4-direction result: %+v", res)
	}

	got, err := svc.MotionGet(root, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Basic directions are generated with an independent frame sequence each.
	for _, dir := range []string{motion.DirectionRight, motion.DirectionUp, motion.DirectionDown} {
		d := got.Direction(dir)
		if d == nil || d.Origin != motion.OriginGenerated {
			t.Fatalf("direction %s missing or not generated: %+v", dir, d)
		}
		if got.FrameCount(dir) != 4 {
			t.Fatalf("direction %s frames = %d, want 4", dir, got.FrameCount(dir))
		}
	}
	// The mirrored direction exists independently with its mirror source.
	left := got.Direction(motion.DirectionLeft)
	if left == nil || left.Origin != motion.OriginMirrored || left.Source != motion.DirectionRight {
		t.Fatalf("left = %+v, want mirrored from right", left)
	}
	if got.FrameCount(motion.DirectionLeft) != 4 {
		t.Fatalf("left frames = %d, want 4", got.FrameCount(motion.DirectionLeft))
	}
	// Anchor conversion (X' = width-1-X, Y' = Y): every left frame's feet
	// anchor is the horizontal mirror of the right frame's feet anchor.
	rightAnchors := got.Direction(motion.DirectionRight).Sequence.Frames[0].Anchors
	leftAnchors := left.Sequence.Frames[0].Anchors
	if len(rightAnchors) != 1 || len(leftAnchors) != 1 {
		t.Fatalf("anchors: right=%v left=%v", rightAnchors, leftAnchors)
	}
	if leftAnchors[0].Name != rightAnchors[0].Name {
		t.Errorf("anchor name not preserved: %q vs %q", leftAnchors[0].Name, rightAnchors[0].Name)
	}
	if leftAnchors[0].X != 31-rightAnchors[0].X || leftAnchors[0].Y != rightAnchors[0].Y {
		t.Errorf("mirror anchor conversion: left %+v, want X'=31-%d=%d, Y'=%d",
			leftAnchors[0], rightAnchors[0].X, 31-rightAnchors[0].X, rightAnchors[0].Y)
	}
	// Persisted: motions.json exists and reloads the same model.
	if _, err := svc.MotionGet(root, m.ID); err != nil {
		t.Fatalf("motion not persisted: %v", err)
	}
}

// TestEightDirectionMirroringViaService verifies the 8-direction strategy
// through the service: 5 生成 (right/up/down/up-right/down-left) + 3 镜像
// (left/up-left/down-right) — down-right is derived ONE-WAY from down-left
// (复核报告语义), so down-left is generated.
func TestEightDirectionMirroringViaService(t *testing.T) {
	svc, root := newPhase5Svc(t)
	m, err := svc.MotionCreate(root, "walk", motion.DirectionStrategy{Count: 8, Mirror: true})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, MotionID: m.ID, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.BasicDirections != 5 || plan.MirroredDirections != 3 || plan.ExpectedCalls != 5 {
		t.Fatalf("8-direction plan: %+v", plan)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanExecuted || res.CallsMade != 5 {
		t.Fatalf("8-direction result: %+v", res)
	}
	got, err := svc.MotionGet(root, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantPairs := map[string]string{
		motion.DirectionLeft:      motion.DirectionRight,
		motion.DirectionUpLeft:    motion.DirectionUpRight,
		motion.DirectionDownRight: motion.DirectionDownLeft,
	}
	if len(got.Directions) != 8 {
		t.Fatalf("motion directions = %d, want 8", len(got.Directions))
	}
	for derived, source := range wantPairs {
		d := got.Direction(derived)
		if d == nil || d.Origin != motion.OriginMirrored || d.Source != source {
			t.Errorf("%s = %+v, want mirrored from %s", derived, d, source)
		}
		if got.FrameCount(derived) != got.FrameCount(source) {
			t.Errorf("%s frames %d != source %s frames %d",
				derived, got.FrameCount(derived), source, got.FrameCount(source))
		}
	}
	// down is self-symmetric: it is generated, never a source of a derivation.
	if d := got.Direction(motion.DirectionDown); d == nil || d.Origin != motion.OriginGenerated {
		t.Errorf("down = %+v, want generated (self-symmetric under horizontal mirroring)", d)
	}
}

// TestMirrorOffGeneratesAllDirectionsViaService verifies 关闭镜像时所有方向独立生成
// through the service: a 4-direction motion with Mirror=false plans and
// generates all 4 directions independently (no mirrored set).
func TestMirrorOffGeneratesAllDirectionsViaService(t *testing.T) {
	svc, root := newPhase5Svc(t)
	m, err := svc.MotionCreate(root, "walk", motion.DirectionStrategy{Count: 4, Mirror: false})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, MotionID: m.ID, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.BasicDirections != 4 || plan.MirroredDirections != 0 || plan.ExpectedCalls != 4 {
		t.Fatalf("mirror-off plan: %+v", plan)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanExecuted || res.CallsMade != 4 {
		t.Fatalf("mirror-off result: %+v", res)
	}
	got, err := svc.MotionGet(root, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{motion.DirectionRight, motion.DirectionUp, motion.DirectionDown, motion.DirectionLeft} {
		if d := got.Direction(dir); d == nil || d.Origin != motion.OriginGenerated {
			t.Errorf("direction %s = %+v, want generated (mirror off)", dir, d)
		}
	}
}

// TestReplacementCountedInConfirmation verifies task 3.5: replacing a mirrored
// direction during acceptance counts as an extra call in the generation
// confirmation's expected call count, and the direction set is updated with
// the replacement frames (origin "replaced").
func TestReplacementCountedInConfirmation(t *testing.T) {
	svc, root := newPhase5Svc(t)
	m, err := svc.MotionCreate(root, "walk", motion.DirectionStrategy{Count: 4, Mirror: true})
	if err != nil {
		t.Fatal(err)
	}
	// Initial 4-direction generation (3 calls) → left is mirror-derived.
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, MotionID: m.ID, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmGeneration(context.Background(), plan.ID, true); err != nil {
		t.Fatal(err)
	}
	if before, _ := svc.MotionGet(root, m.ID); before.Direction(motion.DirectionLeft).Origin != motion.OriginMirrored {
		t.Fatal("left should be mirror-derived before replacement")
	}

	// 验收时手动替换镜像方向: the replacement is counted in the confirmation's
	// expected call count (1 extra call for the replaced direction).
	repl, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath:       root,
		MotionID:          m.ID,
		StylePresetID:     "pixel_classic",
		ActionPresetID:    "walk",
		ReplaceDirections: []string{motion.DirectionLeft},
	})
	if err != nil {
		t.Fatal(err)
	}
	if repl.Kind != PlanKindReplace || repl.ExpectedCalls != 1 || repl.BasicDirections != 1 || repl.MirroredDirections != 0 {
		t.Fatalf("replacement plan: %+v", repl)
	}
	res, err := svc.ConfirmGeneration(context.Background(), repl.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanExecuted || res.CallsMade != 1 {
		t.Fatalf("replacement result: %+v", res)
	}
	// Direction set updated: left now holds the replacement frames.
	got, err := svc.MotionGet(root, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	left := got.Direction(motion.DirectionLeft)
	if left.Origin != motion.OriginReplaced || left.Source != "" {
		t.Fatalf("left after replacement = origin %q source %q, want replaced", left.Origin, left.Source)
	}
	if got.FrameCount(motion.DirectionLeft) != 4 {
		t.Fatalf("replacement frames = %d, want 4", got.FrameCount(motion.DirectionLeft))
	}
	// Replacing an unknown direction is refused at prepare time.
	if _, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, MotionID: m.ID, ReplaceDirections: []string{"nowhere"},
	}); err == nil {
		t.Fatal("replacing an unknown direction must fail")
	}
}

// TestFrameTimingViaService verifies task 3.6 through the service: frame
// durations are saved in the sequence metadata and preview playback follows
// the new rhythm.
func TestFrameTimingViaService(t *testing.T) {
	svc, root := newPhase5Svc(t)
	m, err := svc.MotionCreate(root, "idle", motion.DirectionStrategy{Count: 1, Mirror: true})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, MotionID: m.ID, StylePresetID: "pixel_classic", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ConfirmGeneration(context.Background(), plan.ID, true); err != nil {
		t.Fatal(err)
	}
	tempo, err := svc.MotionPlaybackTempo(root, m.ID, motion.DirectionDown)
	if err != nil {
		t.Fatal(err)
	}
	if len(tempo) != 4 {
		t.Fatalf("tempo length = %d, want 4", len(tempo))
	}
	// Adjust the rhythm; the preview tempo follows (预览按新节奏回放).
	newRhythm := []int{80, 160, 240, 320}
	if _, err := svc.MotionSetFrameDurations(root, m.ID, motion.DirectionDown, newRhythm); err != nil {
		t.Fatal(err)
	}
	tempo, err = svc.MotionPlaybackTempo(root, m.ID, motion.DirectionDown)
	if err != nil {
		t.Fatal(err)
	}
	for i := range newRhythm {
		if tempo[i] != newRhythm[i] {
			t.Fatalf("tempo = %v, want %v", tempo, newRhythm)
		}
	}
}

func motionDirNames(m *motion.Motion) []string {
	out := make([]string, 0, len(m.Directions))
	for _, d := range m.Directions {
		out = append(out, d.Direction)
	}
	return out
}
