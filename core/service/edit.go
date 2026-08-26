package service

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/oframe/character-workbench/core/edit"
	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/motion"
	"github.com/oframe/character-workbench/core/pipeline"
	"github.com/oframe/character-workbench/core/version"
)

// EditResult is the outcome of applying edit instructions to a motion
// direction's current animation assets (editing spec 7.x).
type EditResult struct {
	MotionID    string `json:"motionId"`
	Direction   string `json:"direction"`
	FrameCount  int    `json:"frameCount"`
	DurationsMs []int  `json:"durationsMs"`
	LogSeq      int    `json:"logSeq"`
}

// EditDirection applies replayable, non-destructive edit instructions to the
// accepted animation assets of a motion direction (editing spec 7.1–7.5): it
// loads the current frames, replays the instructions (append-only; the
// original instructions are preserved in the operation log), writes the edited
// frames back into the current version's asset area, updates the motion's
// frame metadata (durations / anchors / order), and appends an operation-log
// entry carrying the instruction set so the edit can be replayed (spec:
// 可回放编辑指令) and rolled back (rollback restores the last immutable
// version).
func (s *Service) EditDirection(pkgPath, motionID, direction string, instructions []edit.Instruction) (*EditResult, error) {
	if len(instructions) == 0 {
		return nil, fmt.Errorf("service: edit requires at least one instruction")
	}
	pkg, err := identity.Open(pkgPath)
	if err != nil {
		return nil, err
	}
	st := motion.NewStore(pkgPath)
	ms, err := st.Load()
	if err != nil {
		return nil, err
	}
	m, err := ms.Get(motionID)
	if err != nil {
		return nil, err
	}
	d := m.Direction(direction)
	if d == nil {
		return nil, fmt.Errorf("service: motion %q has no direction %q", motionID, direction)
	}
	assetsDir, err := version.CurrentAssetsDir(pkg)
	if err != nil {
		return nil, err
	}

	// Build the edit sequence from the current asset frames (frames must exist
	// on disk — an accepted asset), carrying duration and anchor bookkeeping.
	seq := edit.Sequence{}
	anchorLists := make([][]pipeline.AnchorPoint, 0, len(d.Sequence.Frames))
	for _, f := range d.Sequence.Frames {
		data, err := os.ReadFile(frameAssetPath(assetsDir, motionID, direction, f.Index))
		if err != nil {
			return nil, fmt.Errorf("service: read frame %d: %w", f.Index, err)
		}
		img, err := decodeEditPNG(data)
		if err != nil {
			return nil, fmt.Errorf("service: decode frame %d: %w", f.Index, err)
		}
		ax, ay := firstAnchorXY(f.Anchors)
		seq.Frames = append(seq.Frames, edit.Frame{Image: img, DurationMs: f.DurationMs, AnchorX: ax, AnchorY: ay})
		anchorLists = append(anchorLists, f.Anchors)
	}

	// Anchor-delta accumulation per frame (or all frames when no index).
	deltas := map[int][2]int{}
	allDelta := [2]int{}
	for _, inst := range instructions {
		if inst.Kind != "anchor-delta" {
			continue
		}
		if inst.FrameIndex >= 0 && inst.FrameIndex < len(seq.Frames) {
			d := deltas[inst.FrameIndex]
			d[0] += inst.DeltaX
			d[1] += inst.DeltaY
			deltas[inst.FrameIndex] = d
		} else {
			allDelta[0] += inst.DeltaX
			allDelta[1] += inst.DeltaY
		}
	}

	// Apply the instructions, tracking the source anchor list through the
	// structural operations (reorder/delete/insert) so anchors follow frames.
	for _, inst := range instructions {
		if err := seq.Apply(inst); err != nil {
			return nil, fmt.Errorf("service: apply %s: %w", inst.Kind, err)
		}
		if err := applyStructuralToAnchors(&anchorLists, inst); err != nil {
			return nil, fmt.Errorf("service: anchor tracking %s: %w", inst.Kind, err)
		}
	}

	// Persist edited frames (new indices), clean up stale files, and rebuild
	// the motion frame sequence with updated metadata.
	newFrames := make([]motion.Frame, 0, len(seq.Frames))
	for i, f := range seq.Frames {
		data, err := encodeEditPNG(f.Image)
		if err != nil {
			return nil, err
		}
		path := frameAssetPath(assetsDir, motionID, direction, i)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return nil, err
		}
		anchors := translateAnchors(anchorLists[i], allDelta[0]+deltas[i][0], allDelta[1]+deltas[i][1])
		newFrames = append(newFrames, motion.Frame{
			Index:      i,
			AssetRef:   fmt.Sprintf("edit:frame:%d", i),
			DurationMs: f.DurationMs,
			Anchors:    anchors,
		})
	}
	removeStaleFrameFiles(assetsDir, motionID, direction, len(newFrames))

	if err := m.SetDirectionSequence(direction, motion.FrameSequence{Frames: newFrames}, ""); err != nil {
		return nil, err
	}
	if err := st.Save(ms); err != nil {
		return nil, fmt.Errorf("service: persist motions: %w", err)
	}

	// Append the replayable edit to the append-only operation log.
	entry, err := version.Append(pkg, version.ActionEdit, map[string]any{
		"motionId":     motionID,
		"direction":    direction,
		"instructions": instructions,
		"frameCount":   len(newFrames),
	})
	if err != nil {
		s.log.Warn("operation log append failed", "error", err)
	}

	durations := make([]int, len(newFrames))
	for i, f := range newFrames {
		durations[i] = f.DurationMs
	}
	s.log.Info("direction edited", "motion", motionID, "direction", direction,
		"instructions", len(instructions), "frames", len(newFrames), "logSeq", entry.Seq)
	return &EditResult{
		MotionID: motionID, Direction: direction, FrameCount: len(newFrames),
		DurationsMs: durations, LogSeq: entry.Seq,
	}, nil
}

// applyStructuralToAnchors mirrors reorder/delete/insert on the parallel anchor
// list so anchors stay attached to their frames after structural edits.
func applyStructuralToAnchors(lists *[][]pipeline.AnchorPoint, inst edit.Instruction) error {
	switch inst.Kind {
	case "reorder":
		if len(inst.Order) != len(*lists) {
			return fmt.Errorf("reorder length mismatch")
		}
		next := make([][]pipeline.AnchorPoint, len(inst.Order))
		for i, src := range inst.Order {
			if src < 0 || src >= len(*lists) {
				return fmt.Errorf("invalid reorder index %d", src)
			}
			next[i] = (*lists)[src]
		}
		*lists = next
	case "delete":
		if inst.FrameIndex < 0 || inst.FrameIndex >= len(*lists) {
			return fmt.Errorf("delete index out of range")
		}
		*lists = append((*lists)[:inst.FrameIndex], (*lists)[inst.FrameIndex+1:]...)
	case "insert":
		if inst.FrameIndex < 0 || inst.FrameIndex > len(*lists) {
			return fmt.Errorf("insert index out of range")
		}
		meta := inst.FrameMeta
		*lists = append(*lists, nil)
		copy((*lists)[inst.FrameIndex+1:], (*lists)[inst.FrameIndex:])
		(*lists)[inst.FrameIndex] = []pipeline.AnchorPoint{{Name: "anchor", X: meta.AnchorX, Y: meta.AnchorY}}
	}
	return nil
}

// translateAnchors moves every anchor of a frame by (dx, dy).
func translateAnchors(in []pipeline.AnchorPoint, dx, dy int) []pipeline.AnchorPoint {
	if len(in) == 0 {
		return nil
	}
	out := make([]pipeline.AnchorPoint, len(in))
	for i, a := range in {
		out[i] = pipeline.AnchorPoint{Name: a.Name, X: a.X + dx, Y: a.Y + dy}
	}
	return out
}

func firstAnchorXY(anchors []pipeline.AnchorPoint) (int, int) {
	if len(anchors) == 0 {
		return 0, 0
	}
	return anchors[0].X, anchors[0].Y
}

func frameAssetPath(assetsDir, motionID, direction string, index int) string {
	return filepath.Join(assetsDir, motionID, direction, fmt.Sprintf("frame_%03d.png", index))
}

// removeStaleFrameFiles deletes leftover frame files beyond the new count
// (e.g. after a frame delete shrinks the sequence).
func removeStaleFrameFiles(assetsDir, motionID, direction string, newCount int) {
	for idx := newCount; ; idx++ {
		p := frameAssetPath(assetsDir, motionID, direction, idx)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return
		}
		_ = os.Remove(p)
	}
}

func decodeEditPNG(data []byte) (*image.RGBA, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dst.Set(x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst, nil
}

func encodeEditPNG(img *image.RGBA) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("service: encode edited frame: %w", err)
	}
	return buf.Bytes(), nil
}
