package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Candidate directory layout inside <pkg>/candidates/<candidateID>/:
// the preserved original artifact (原始产物), the immutable prompt snapshot,
// the quality scores, and the processed frames with their anchors — so a
// candidate can be reloaded after an app restart for preview / acceptance.
const (
	fileFilmstrip   = "filmstrip.png" // 原始产物 (raw provider output)
	filePrompt      = "prompt.json"   // 提示词快照 (immutable)
	fileScores      = "scores.json"   // 质量评分
	fileLayout      = "layout.json"   // 规格化帧清单
	fileMeta        = "meta.json"     // candidate meta (direction/status/reason/...)
	fileAnchors     = "anchors.json"  // per-frame corrected anchors
	dirFrames       = "frames"        // processed frames: frames/frame_000.png ...
	fileFramesIndex = "frames/index.json"
)

// SaveCandidate persists the preserved artifacts of a candidate into dir
// (保留原始产物与 prompt 快照): filmstrip.png, prompt.json, scores.json,
// layout.json, meta.json, the processed frames (frames/frame_%03d.png) and
// their anchors (anchors.json). File names are deterministic; the directory is
// created if missing. Frames are encoded losslessly (PNG) so preview rendering
// is pixel-identical to the pipeline output (task 5.5).
func SaveCandidate(dir string, c Candidate) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("pipeline: create candidate directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileFilmstrip), c.FilmstripPNG, 0o644); err != nil {
		return fmt.Errorf("pipeline: write filmstrip artifact: %w", err)
	}
	promptData, err := json.MarshalIndent(c.Prompt, "", "  ")
	if err != nil {
		return fmt.Errorf("pipeline: encode prompt snapshot: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filePrompt), promptData, 0o644); err != nil {
		return fmt.Errorf("pipeline: write prompt snapshot: %w", err)
	}
	scoresData, err := json.MarshalIndent(c.Scores, "", "  ")
	if err != nil {
		return fmt.Errorf("pipeline: encode scores: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileScores), scoresData, 0o644); err != nil {
		return fmt.Errorf("pipeline: write scores: %w", err)
	}
	layoutData, err := json.MarshalIndent(c.Layout, "", "  ")
	if err != nil {
		return fmt.Errorf("pipeline: encode layout: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileLayout), layoutData, 0o644); err != nil {
		return fmt.Errorf("pipeline: write layout: %w", err)
	}
	meta := candidateMeta{
		ID:             c.ID,
		CreatedAt:      c.CreatedAt.Format(time.RFC3339),
		Direction:      c.Direction,
		Status:         c.Status,
		Reason:         c.Reason,
		RegenerationOf: c.RegenerationOf,
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("pipeline: encode candidate meta: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileMeta), metaData, 0o644); err != nil {
		return fmt.Errorf("pipeline: write candidate meta: %w", err)
	}
	if err := saveFrames(dir, c.Frames, c.AnchorSets); err != nil {
		return err
	}
	return nil
}

// saveFrames writes the processed frames (frames/frame_%03d.png) and their
// anchors (frames/index.json: anchors per frame).
func saveFrames(dir string, frames []*image.RGBA, anchorSets [][]AnchorPoint) error {
	if len(frames) == 0 {
		// A failed candidate has no processed frames; nothing to write.
		return nil
	}
	framesDir := filepath.Join(dir, dirFrames)
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return fmt.Errorf("pipeline: create frames dir: %w", err)
	}
	idx := framesIndex{Frames: make([]frameEntry, len(frames))}
	for i, f := range frames {
		if f == nil {
			return fmt.Errorf("pipeline: candidate frame %d is nil", i)
		}
		name := fmt.Sprintf("frame_%03d.png", i)
		if err := os.WriteFile(filepath.Join(framesDir, name), mustEncodePNG(f), 0o644); err != nil {
			return fmt.Errorf("pipeline: write frame %s: %w", name, err)
		}
		idx.Frames[i] = frameEntry{File: name, Width: f.Bounds().Dx(), Height: f.Bounds().Dy()}
		if i < len(anchorSets) {
			idx.Frames[i].Anchors = anchorSets[i]
		}
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("pipeline: encode frames index: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileFramesIndex), data, 0o644); err != nil {
		return fmt.Errorf("pipeline: write frames index: %w", err)
	}
	return nil
}

// LoadCandidate reconstructs a candidate from its persisted directory
// (task 5.5 preview / acceptance after restart).
func LoadCandidate(dir string) (Candidate, error) {
	var c Candidate
	c.ID = filepath.Base(dir)

	if data, err := os.ReadFile(filepath.Join(dir, fileFilmstrip)); err == nil {
		c.FilmstripPNG = data
	} else if !os.IsNotExist(err) {
		return Candidate{}, fmt.Errorf("pipeline: read filmstrip: %w", err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, filePrompt)); err == nil {
		if err := json.Unmarshal(data, &c.Prompt); err != nil {
			return Candidate{}, fmt.Errorf("pipeline: parse prompt snapshot: %w", err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, fileScores)); err == nil {
		if err := json.Unmarshal(data, &c.Scores); err != nil {
			return Candidate{}, fmt.Errorf("pipeline: parse scores: %w", err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, fileLayout)); err == nil {
		if err := json.Unmarshal(data, &c.Layout); err != nil {
			return Candidate{}, fmt.Errorf("pipeline: parse layout: %w", err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, fileMeta)); err == nil {
		var meta candidateMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			return Candidate{}, fmt.Errorf("pipeline: parse candidate meta: %w", err)
		}
		c.Direction = meta.Direction
		c.Status = meta.Status
		c.Reason = meta.Reason
		c.RegenerationOf = meta.RegenerationOf
		if meta.CreatedAt != "" {
			if ts, err := time.Parse(time.RFC3339, meta.CreatedAt); err == nil {
				c.CreatedAt = ts
			}
		}
	}
	if err := loadFrames(dir, &c); err != nil {
		return Candidate{}, err
	}
	return c, nil
}

func loadFrames(dir string, c *Candidate) error {
	idxData, err := os.ReadFile(filepath.Join(dir, fileFramesIndex))
	if os.IsNotExist(err) {
		return nil // failed candidate without processed frames
	}
	if err != nil {
		return fmt.Errorf("pipeline: read frames index: %w", err)
	}
	var idx framesIndex
	if err := json.Unmarshal(idxData, &idx); err != nil {
		return fmt.Errorf("pipeline: parse frames index: %w", err)
	}
	c.Frames = make([]*image.RGBA, 0, len(idx.Frames))
	c.AnchorSets = make([][]AnchorPoint, 0, len(idx.Frames))
	for _, fe := range idx.Frames {
		data, err := os.ReadFile(filepath.Join(dir, dirFrames, fe.File))
		if err != nil {
			return fmt.Errorf("pipeline: read frame %s: %w", fe.File, err)
		}
		img, err := DecodeFrame(data)
		if err != nil {
			return fmt.Errorf("pipeline: decode frame %s: %w", fe.File, err)
		}
		c.Frames = append(c.Frames, img)
		c.AnchorSets = append(c.AnchorSets, fe.Anchors)
	}
	return nil
}

// ListCandidates returns the persisted candidate ids of a candidates root
// directory, sorted by directory modification time (oldest first is
// deterministic enough for listings; ids are unique).
func ListCandidates(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// candidateMeta is the persisted candidate meta payload.
type candidateMeta struct {
	ID             string `json:"id"`
	CreatedAt      string `json:"createdAt"`
	Direction      string `json:"direction,omitempty"`
	Status         string `json:"status,omitempty"`
	Reason         string `json:"reason,omitempty"`
	RegenerationOf string `json:"regenerationOf,omitempty"`
}

// framesIndex maps processed frames to files + per-frame anchors.
type framesIndex struct {
	Frames []frameEntry `json:"frames"`
}

type frameEntry struct {
	File    string        `json:"file"`
	Width   int           `json:"width"`
	Height  int           `json:"height"`
	Anchors []AnchorPoint `json:"anchors,omitempty"`
}

// DecodeFrame decodes one processed frame PNG.
func DecodeFrame(data []byte) (*image.RGBA, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return ToRGBA(img), nil
}

func mustEncodePNG(img *image.RGBA) []byte {
	data, err := EncodeFilmstripPNG(img)
	if err != nil {
		panic(fmt.Sprintf("pipeline: encode frame: %v", err))
	}
	return data
}
