package motion

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/pipeline"
)

// TestMotionConsistsOfDirectionSets verifies task 3.1: a motion consists of
// direction sets and every direction owns one independent frame sequence.
func TestMotionConsistsOfDirectionSets(t *testing.T) {
	m, err := NewMotion("m1", "walk", DirectionStrategy{Count: 4, Mirror: true})
	if err != nil {
		t.Fatal(err)
	}
	// 4 方向: right/up/down 生成 + left 镜像 → 4 个方向集, 每方向一份独立帧序列.
	if len(m.Directions) != 4 {
		t.Fatalf("directions = %d, want 4", len(m.Directions))
	}
	want := []string{DirectionRight, DirectionUp, DirectionDown, DirectionLeft}
	got := make([]string, 0, len(m.Directions))
	for _, d := range m.Directions {
		got = append(got, d.Direction)
		// Every direction gets its own frame sequence object (never shared).
		if d.Sequence.Direction != d.Direction {
			t.Errorf("sequence direction %q != set direction %q", d.Sequence.Direction, d.Direction)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("direction order = %v, want %v", got, want)
	}
	// Basic directions are marked generated; the mirror direction marks its source.
	if m.Direction(DirectionRight).Origin != OriginGenerated {
		t.Errorf("right origin = %q", m.Direction(DirectionRight).Origin)
	}
	if m.Direction(DirectionLeft).Origin != OriginMirrored || m.Direction(DirectionLeft).Source != DirectionRight {
		t.Errorf("left origin/source = %q/%q, want mirrored/right",
			m.Direction(DirectionLeft).Origin, m.Direction(DirectionLeft).Source)
	}
	// Sequences are independent objects: mutating one must not touch the other.
	if err := m.SetDirectionSequence(DirectionRight, FrameSequence{Frames: []Frame{{Index: 0, DurationMs: 120}}}, OriginGenerated); err != nil {
		t.Fatal(err)
	}
	if m.FrameCount(DirectionUp) != 0 {
		t.Error("up sequence shares content with right (must be independent)")
	}
}

// TestSingleDirectionDefault verifies task 3.2: a new motion defaults to a
// single direction — down (south/正面, front-facing).
func TestSingleDirectionDefault(t *testing.T) {
	m, err := NewMotion("m2", "idle", DirectionStrategy{Count: 1, Mirror: true})
	if err != nil {
		t.Fatal(err)
	}
	if m.Strategy.Count != 1 {
		t.Fatalf("strategy count = %d, want 1", m.Strategy.Count)
	}
	if len(m.Directions) != 1 || m.Directions[0].Direction != DefaultDirection {
		t.Fatalf("default directions = %v, want single [%s]", dirNames(m), DefaultDirection)
	}
	if DefaultDirection != DirectionDown {
		t.Fatalf("default direction = %q, want down (south/正面)", DefaultDirection)
	}
	// A single direction has no mirror derivation.
	if got := MirroredDirections(DirectionStrategy{Count: 1, Mirror: true}); len(got) != 0 {
		t.Fatalf("single-direction mirrored set = %v, want empty", got)
	}
}

// TestFourDirectionMirroring verifies task 3.3 (4 方向): 3 generated
// (right/up/down) + 1 mirrored (left), with the correct one-way mapping.
func TestFourDirectionMirroring(t *testing.T) {
	s := DirectionStrategy{Count: 4, Mirror: true}
	if got := BasicDirections(s); !slices.Equal(got, []string{DirectionRight, DirectionUp, DirectionDown}) {
		t.Fatalf("basic(4) = %v", got)
	}
	if got := MirroredDirections(s); !slices.Equal(got, []string{DirectionLeft}) {
		t.Fatalf("mirrored(4) = %v", got)
	}
	if MirroredFrom(DirectionRight) != DirectionLeft {
		t.Fatalf("mirror(right) = %q, want left", MirroredFrom(DirectionRight))
	}
	if MirrorSource(DirectionLeft) != DirectionRight {
		t.Fatalf("source(left) = %q, want right", MirrorSource(DirectionLeft))
	}
}

// TestEightDirectionMirroring verifies task 3.3 (8 方向) with the review
// report's explicit semantics: 5 generated (right/up/down/up-right/down-left)
// + 3 mirrored (left/up-left/down-right); down-left is a source because
// down-right is derived ONE-WAY from down-left; down is self-symmetric.
func TestEightDirectionMirroring(t *testing.T) {
	s := DirectionStrategy{Count: 8, Mirror: true}
	if got := BasicDirections(s); !slices.Equal(got,
		[]string{DirectionRight, DirectionUp, DirectionDown, DirectionUpRight, DirectionDownLeft}) {
		t.Fatalf("basic(8) = %v", got)
	}
	if got := MirroredDirections(s); !slices.Equal(got,
		[]string{DirectionLeft, DirectionUpLeft, DirectionDownRight}) {
		t.Fatalf("mirrored(8) = %v", got)
	}
	// Mapping correctness (motion spec: right mirrors to left, up-right mirrors
	// to up-left; review: down-right 单向派生自 down-left).
	pairs := map[string]string{
		DirectionRight:    DirectionLeft,
		DirectionUpRight:  DirectionUpLeft,
		DirectionDownLeft: DirectionDownRight,
	}
	for src, derived := range pairs {
		if MirroredFrom(src) != derived {
			t.Errorf("mirror(%s) = %q, want %s", src, MirroredFrom(src), derived)
		}
		if MirrorSource(derived) != src {
			t.Errorf("source(%s) = %q, want %s", derived, MirrorSource(derived), src)
		}
	}
	// down (and up) are SELF-SYMMETRIC under horizontal mirroring — they are
	// never a source of a different direction and never mirror-derived.
	if MirroredFrom(DirectionDown) != "" || MirrorSource(DirectionDown) != "" {
		t.Error("down must be self-symmetric under horizontal mirroring")
	}
	if MirroredFrom(DirectionUp) != "" || MirrorSource(DirectionUp) != "" {
		t.Error("up must be self-symmetric under horizontal mirroring")
	}
	if !IsSelfSymmetric(DirectionDown) || !IsSelfSymmetric(DirectionUp) {
		t.Error("up/down must be reported self-symmetric")
	}
	// One-way: derived directions (left/up-left/down-right) are not sources.
	if MirroredFrom(DirectionLeft) != "" {
		t.Error("left is derived — must not be a mirror source")
	}
	// Every mirrored direction has a source inside the basic set.
	for _, md := range MirroredDirections(s) {
		src := MirrorSource(md)
		if !slices.Contains(BasicDirections(s), src) {
			t.Errorf("mirrored %s source %s not in basic set", md, src)
		}
	}
}

// TestMirrorDisabledGeneratesAllDirections verifies 关闭镜像时所有方向独立生成:
// with Mirror=false every direction is in the basic (generated) set and the
// mirrored set is empty.
func TestMirrorDisabledGeneratesAllDirections(t *testing.T) {
	for _, tc := range []struct {
		count int
		want  []string
	}{
		{1, []string{DirectionDown}},
		{4, []string{DirectionRight, DirectionUp, DirectionDown, DirectionLeft}},
		{8, AllDirections},
	} {
		s := DirectionStrategy{Count: tc.count, Mirror: false}
		if got := BasicDirections(s); !slices.Equal(got, tc.want) {
			t.Errorf("basic(%d, no mirror) = %v, want %v", tc.count, got, tc.want)
		}
		if got := MirroredDirections(s); len(got) != 0 {
			t.Errorf("mirrored(%d, no mirror) = %v, want empty", tc.count, got)
		}
		m, err := NewMotion("m", "walk", s)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.Directions) != len(tc.want) {
			t.Errorf("motion directions = %d, want %d", len(m.Directions), len(tc.want))
		}
		for _, d := range m.Directions {
			if d.Origin != OriginGenerated {
				t.Errorf("direction %s origin = %q, want generated (mirror off)", d.Direction, d.Origin)
			}
		}
	}
}

// TestMirrorDirectionIndependentSequenceAndAnchors verifies task 3.4: the
// mirrored direction's frame sequence exists independently and its anchors are
// converted by the horizontal mirror rule (X' = width-1-X, Y' = Y), with
// integer pixel exactness.
func TestMirrorDirectionIndependentSequenceAndAnchors(t *testing.T) {
	const width = 32
	src := FrameSequence{
		Direction: DirectionDownLeft,
		Frames: []Frame{
			{Index: 0, AssetRef: "candidate:c1:frame:0", DurationMs: 150,
				Anchors: []pipeline.AnchorPoint{{Name: "feet", X: 20, Y: 31}, {Name: "hand", X: 4, Y: 12}}},
			{Index: 1, AssetRef: "candidate:c1:frame:1", DurationMs: 90,
				Anchors: []pipeline.AnchorPoint{{Name: "feet", X: 16, Y: 31}}},
		},
	}
	seq, err := MirrorSequence(src, width)
	if err != nil {
		t.Fatal(err)
	}
	// Independent sequence: same count, own frames (never aliasing the source).
	if len(seq.Frames) != len(src.Frames) {
		t.Fatalf("mirrored frames = %d, want %d", len(seq.Frames), len(src.Frames))
	}
	for i, f := range seq.Frames {
		if f.Index != i {
			t.Errorf("mirrored frame %d index = %d", i, f.Index)
		}
		if f.AssetRef == src.Frames[i].AssetRef {
			t.Errorf("mirrored frame %d shares the source asset ref (must be independent)", i)
		}
		if f.DurationMs != src.Frames[i].DurationMs {
			t.Errorf("mirrored frame %d duration = %d, want %d (mirroring keeps rhythm)",
				i, f.DurationMs, src.Frames[i].DurationMs)
		}
	}
	// Anchor conversion: X' = width-1-X, Y' = Y.
	got := seq.Frames[0].Anchors
	want := []pipeline.AnchorPoint{{Name: "feet", X: width - 1 - 20, Y: 31}, {Name: "hand", X: width - 1 - 4, Y: 12}}
	if len(got) != len(want) {
		t.Fatalf("mirrored anchors = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("anchor %d = %+v, want %+v (X' = width-1-X, Y' = Y)", i, got[i], want[i])
		}
	}
	// The mirrored direction set entry carries origin mirrored + source.
	m, _ := NewMotion("m", "walk", DirectionStrategy{Count: 8, Mirror: true})
	if err := m.SetDirectionSequence(DirectionDownRight, seq, OriginMirrored); err != nil {
		t.Fatal(err)
	}
	d := m.Direction(DirectionDownRight)
	if d.Origin != OriginMirrored || d.Source != DirectionDownLeft {
		t.Errorf("down-right origin/source = %q/%q, want mirrored/%s",
			d.Origin, d.Source, DirectionDownLeft)
	}
	if m.FrameCount(DirectionDownRight) != len(src.Frames) {
		t.Errorf("down-right frame count = %d, want %d", m.FrameCount(DirectionDownRight), len(src.Frames))
	}
}

// TestHorizontalMirrorPixelExact verifies the frame mirror is a left-right
// flip with integer pixel exactness (no interpolation).
func TestHorizontalMirrorPixelExact(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	img.SetRGBA(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	img.SetRGBA(3, 0, color.RGBA{R: 9, G: 8, B: 7, A: 255})
	img.SetRGBA(1, 1, color.RGBA{R: 5, G: 5, B: 5, A: 128})
	mir := HorizontalMirror(img)
	if mir.RGBAAt(3, 0) != img.RGBAAt(0, 0) || mir.RGBAAt(0, 0) != img.RGBAAt(3, 0) {
		t.Error("horizontal mirror did not swap columns")
	}
	if mir.RGBAAt(2, 1) != img.RGBAAt(1, 1) || mir.RGBAAt(1, 1) != img.RGBAAt(2, 1) {
		t.Error("horizontal mirror column mapping wrong at row 1")
	}
	if mir.RGBAAt(0, 1) != img.RGBAAt(3, 1) {
		t.Error("row 1 column 0 mismatch")
	}
	// Mirroring twice returns the original (integer exact, no data loss).
	if twice := HorizontalMirror(mir); !framesEqual(twice, img) {
		t.Error("double mirror must reproduce the original")
	}
}

// TestReplaceDirection verifies task 3.5: replacing a mirrored direction
// updates the direction set with the replacement frames and marks it replaced.
func TestReplaceDirection(t *testing.T) {
	m, _ := NewMotion("m", "walk", DirectionStrategy{Count: 4, Mirror: true})
	repl := FrameSequence{Frames: []Frame{
		{Index: 0, AssetRef: "candidate:r1:frame:0", DurationMs: 100, Anchors: []pipeline.AnchorPoint{{Name: "feet", X: 16, Y: 31}}},
	}}
	if err := m.ReplaceDirection(DirectionLeft, repl); err != nil {
		t.Fatal(err)
	}
	d := m.Direction(DirectionLeft)
	if d.Origin != OriginReplaced {
		t.Errorf("left origin = %q, want replaced", d.Origin)
	}
	if d.Source != "" {
		t.Errorf("left source = %q, want empty after replacement", d.Source)
	}
	if len(d.Sequence.Frames) != 1 || d.Sequence.Frames[0].AssetRef != "candidate:r1:frame:0" {
		t.Errorf("replacement frames not stored: %+v", d.Sequence.Frames)
	}
	if err := m.ReplaceDirection(DirectionUnknown, repl); err == nil {
		t.Error("replacing an unknown direction must fail")
	}
}

const DirectionUnknown = "unknown"

// TestFrameDurationsAndTempo verifies task 3.6: frame durations are saved in
// the frame sequence metadata and preview playback follows the new rhythm.
func TestFrameDurationsAndTempo(t *testing.T) {
	m, _ := NewMotion("m", "walk", DirectionStrategy{Count: 4, Mirror: true})
	if err := m.SetDirectionSequence(DirectionRight, FrameSequence{Frames: []Frame{
		{Index: 0}, {Index: 1}, {Index: 2},
	}}, OriginGenerated); err != nil {
		t.Fatal(err)
	}
	// Defaults before any adjustment.
	tempo, err := m.PlaybackTempo(DirectionRight)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{DefaultFrameDurationMs, DefaultFrameDurationMs, DefaultFrameDurationMs}
	if !slices.Equal(tempo, want) {
		t.Fatalf("default tempo = %v, want %v", tempo, want)
	}
	// Adjust the rhythm (帧时长调整).
	newRhythm := []int{60, 120, 240}
	if err := m.SetFrameDurations(DirectionRight, newRhythm); err != nil {
		t.Fatal(err)
	}
	tempo, err = m.PlaybackTempo(DirectionRight)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tempo, newRhythm) {
		t.Fatalf("adjusted tempo = %v, want %v (预览按新节奏回放)", tempo, newRhythm)
	}
	// Persisted in the sequence metadata.
	if got := m.Direction(DirectionRight).Sequence.Frames[1].DurationMs; got != 120 {
		t.Errorf("frame 1 duration = %d, want 120", got)
	}
	// Length mismatch and non-positive durations are rejected.
	if err := m.SetFrameDurations(DirectionRight, []int{1, 2}); err == nil {
		t.Error("duration count mismatch must fail")
	}
	if err := m.SetFrameDurations(DirectionRight, []int{60, 0, 240}); err == nil {
		t.Error("non-positive duration must fail")
	}
}

// TestSetStrategyPreservesExistingSequences verifies strategy switching keeps
// the independent sequences of directions that still exist.
func TestSetStrategyPreservesExistingSequences(t *testing.T) {
	m, _ := NewMotion("m", "walk", DirectionStrategy{Count: 4, Mirror: true})
	seq := FrameSequence{Frames: []Frame{{Index: 0, DurationMs: 100}, {Index: 1, DurationMs: 100}}}
	if err := m.SetDirectionSequence(DirectionRight, seq, OriginGenerated); err != nil {
		t.Fatal(err)
	}
	if err := m.SetStrategy(DirectionStrategy{Count: 8, Mirror: true}); err != nil {
		t.Fatal(err)
	}
	if m.FrameCount(DirectionRight) != 2 {
		t.Errorf("right frames lost after strategy change: %d", m.FrameCount(DirectionRight))
	}
	// New 8-direction directions exist with the correct origins.
	if m.Direction(DirectionDownRight) == nil || m.Direction(DirectionDownRight).Origin != OriginMirrored {
		t.Error("down-right missing or not mirrored after strategy change")
	}
	if m.Direction(DirectionDownLeft) == nil || m.Direction(DirectionDownLeft).Origin != OriginGenerated {
		t.Error("down-left missing or not generated after strategy change")
	}
	if err := m.SetStrategy(DirectionStrategy{Count: 4, Mirror: true}); err != nil {
		t.Fatal(err)
	}
	if m.FrameCount(DirectionRight) != 2 {
		t.Errorf("right frames lost after switching back: %d", m.FrameCount(DirectionRight))
	}
}

// TestValidateFrameSequence verifies sequence order invariants against the
// logical canvas (identity spec: 规格被后续动作/帧序列校验引用).
func TestValidateFrameSequence(t *testing.T) {
	canvas, err := identity.NewCanvasSpec(32, 32)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := NewMotion("m", "walk", DirectionStrategy{Count: 4, Mirror: true})
	if err := m.ValidateFrameSequence(DirectionRight, *canvas); err == nil {
		t.Error("empty sequence must fail validation")
	}
	if err := m.SetDirectionSequence(DirectionRight, FrameSequence{Frames: []Frame{
		{Index: 0}, {Index: 2}, // order broken
	}}, OriginGenerated); err != nil {
		t.Fatal(err)
	}
	if err := m.ValidateFrameSequence(DirectionRight, *canvas); err == nil {
		t.Error("broken frame order must fail validation")
	}
	if err := m.SetDirectionSequence(DirectionRight, FrameSequence{Frames: []Frame{
		{Index: 0}, {Index: 1}, {Index: 2}, {Index: 3},
	}}, OriginGenerated); err != nil {
		t.Fatal(err)
	}
	if err := m.ValidateFrameSequence(DirectionRight, *canvas); err != nil {
		t.Errorf("valid sequence rejected: %v", err)
	}
}

// TestMotionSetStoreRoundTrip verifies the motions.json persistence: save,
// reload, and list in insertion order.
func TestMotionSetStoreRoundTrip(t *testing.T) {
	root := t.TempDir()
	st := NewStore(root)
	ms := NewMotionSet()
	m1, _ := NewMotion("m1", "walk", DirectionStrategy{Count: 4, Mirror: true})
	m2, _ := NewMotion("m2", "idle", DirectionStrategy{Count: 1, Mirror: true})
	if err := ms.Add(m1); err != nil {
		t.Fatal(err)
	}
	if err := ms.Add(m2); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(ms); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, FileName)); err != nil {
		t.Fatalf("motions.json not written: %v", err)
	}
	loaded, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Len() != 2 {
		t.Fatalf("loaded motions = %d, want 2", loaded.Len())
	}
	got := loaded.List()
	if got[0].ID != "m1" || got[1].ID != "m2" {
		t.Errorf("insertion order lost: %v", []string{got[0].ID, got[1].ID})
	}
	if got[0].Strategy.Count != 4 || !got[0].Strategy.Mirror {
		t.Errorf("m1 strategy lost: %+v", got[0].Strategy)
	}
	// A fresh store over a missing file yields an empty set.
	empty, err := NewStore(filepath.Join(t.TempDir(), "nope")).Load()
	if err != nil {
		t.Fatal(err)
	}
	if empty.Len() != 0 {
		t.Fatal("missing file must yield an empty motion set")
	}
}

func dirNames(m *Motion) []string {
	out := make([]string, 0, len(m.Directions))
	for _, d := range m.Directions {
		out = append(out, d.Direction)
	}
	return out
}

func framesEqual(a, b *image.RGBA) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return false
			}
		}
	}
	return true
}
