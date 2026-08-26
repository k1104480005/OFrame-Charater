package assetexport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	TargetGeneric = "generic"
	TargetGodot   = "godot"
	TargetUnity   = "unity"
	FormatVersion = 1
)

type Anchor struct {
	ID string `json:"id,omitempty"`
	X  int    `json:"x"`
	Y  int    `json:"y"`
}

type Frame struct {
	Index      int      `json:"index"`
	DurationMs int      `json:"durationMs"`
	Anchors    []Anchor `json:"anchors,omitempty"`
	PNG        []byte   `json:"-"`
}

type Animation struct {
	MotionID    string  `json:"motionId"`
	Direction   string  `json:"direction"`
	CandidateID string  `json:"candidateId,omitempty"`
	Frames      []Frame `json:"frames"`
}

type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type FrameManifest struct {
	Index      int      `json:"index"`
	File       string   `json:"file"`
	Rect       Rect     `json:"rect"`
	DurationMs int      `json:"durationMs"`
	Anchors    []Anchor `json:"anchors,omitempty"`
}

type AnimationManifest struct {
	MotionID    string          `json:"motionId"`
	Direction   string          `json:"direction"`
	CandidateID string          `json:"candidateId,omitempty"`
	Frames      []FrameManifest `json:"frames"`
}

type Manifest struct {
	FormatVersion   int                 `json:"formatVersion"`
	Target          string              `json:"target"`
	IdentityVersion string              `json:"identityVersion"`
	GeneratedAt     string              `json:"generatedAt"`
	SpriteSheet     string              `json:"spriteSheet"`
	CellWidth       int                 `json:"cellWidth"`
	CellHeight      int                 `json:"cellHeight"`
	Animations      []AnimationManifest `json:"animations"`
}

type Result struct {
	OutputDir string   `json:"outputDir"`
	Target    string   `json:"target"`
	Manifest  Manifest `json:"manifest"`
}

type HistoryRecord struct {
	Target          string `json:"target"`
	IdentityVersion string `json:"identityVersion"`
	OutputDir       string `json:"outputDir"`
	Result          string `json:"result"`
	Error           string `json:"error,omitempty"`
	CreatedAt       string `json:"createdAt"`
}

func ValidateTarget(target string) error {
	switch target {
	case TargetGeneric, TargetGodot, TargetUnity:
		return nil
	default:
		return fmt.Errorf("export: unsupported target %q", target)
	}
}

func Build(outputDir, target, identityVersion string, animations []Animation) (*Result, error) {
	if err := ValidateTarget(target); err != nil {
		return nil, err
	}
	if strings.TrimSpace(outputDir) == "" || strings.TrimSpace(identityVersion) == "" {
		return nil, fmt.Errorf("export: output directory and identity version are required")
	}
	if len(animations) == 0 {
		return nil, fmt.Errorf("export: no accepted animations to export")
	}
	for i := range animations {
		if animations[i].MotionID == "" || animations[i].Direction == "" || len(animations[i].Frames) == 0 {
			return nil, fmt.Errorf("export: animation %d is incomplete", i)
		}
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("export: create output directory: %w", err)
	}
	cellW, cellH, maxFrames, err := frameDimensions(animations)
	if err != nil {
		return nil, err
	}
	atlas := image.NewRGBA(image.Rect(0, 0, cellW*maxFrames, cellH*len(animations)))
	manifest := Manifest{FormatVersion: FormatVersion, Target: target, IdentityVersion: identityVersion, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), SpriteSheet: "spritesheet.png", CellWidth: cellW, CellHeight: cellH, Animations: make([]AnimationManifest, 0, len(animations))}
	for row, animation := range animations {
		am := AnimationManifest{MotionID: animation.MotionID, Direction: animation.Direction, CandidateID: animation.CandidateID, Frames: make([]FrameManifest, 0, len(animation.Frames))}
		for _, frame := range animation.Frames {
			img, err := decodePNG(frame.PNG)
			if err != nil {
				return nil, fmt.Errorf("export: decode %s/%s frame %d: %w", animation.MotionID, animation.Direction, frame.Index, err)
			}
			if img.Bounds().Dx() != cellW || img.Bounds().Dy() != cellH {
				return nil, fmt.Errorf("export: frame %s/%s/%d is %dx%d, expected %dx%d", animation.MotionID, animation.Direction, frame.Index, img.Bounds().Dx(), img.Bounds().Dy(), cellW, cellH)
			}
			x, y := frame.Index*cellW, row*cellH
			draw.Draw(atlas, image.Rect(x, y, x+cellW, y+cellH), img, image.Point{}, draw.Src)
			rel := filepath.ToSlash(filepath.Join("frames", animation.MotionID, animation.Direction, fmt.Sprintf("frame_%03d.png", frame.Index)))
			path := filepath.Join(outputDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(path, frame.PNG, 0o644); err != nil {
				return nil, fmt.Errorf("export: write frame: %w", err)
			}
			am.Frames = append(am.Frames, FrameManifest{Index: frame.Index, File: rel, Rect: Rect{X: x, Y: y, W: cellW, H: cellH}, DurationMs: frame.DurationMs, Anchors: append([]Anchor(nil), frame.Anchors...)})
		}
		manifest.Animations = append(manifest.Animations, am)
	}
	if err := writePNG(filepath.Join(outputDir, manifest.SpriteSheet), atlas); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("export: encode manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "manifest.json"), data, 0o644); err != nil {
		return nil, fmt.Errorf("export: write manifest: %w", err)
	}
	if err := writeTargetMetadata(outputDir, target, manifest); err != nil {
		return nil, err
	}
	result := &Result{OutputDir: outputDir, Target: target, Manifest: manifest}
	if err := Validate(outputDir); err != nil {
		return nil, err
	}
	return result, nil
}

func Validate(outputDir string) error {
	data, err := os.ReadFile(filepath.Join(outputDir, "manifest.json"))
	if err != nil {
		return fmt.Errorf("export: manifest missing: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("export: invalid manifest: %w", err)
	}
	if m.FormatVersion != FormatVersion {
		return fmt.Errorf("export: unsupported manifest version %d", m.FormatVersion)
	}
	if err := ValidateTarget(m.Target); err != nil {
		return err
	}
	if m.CellWidth <= 0 || m.CellHeight <= 0 || len(m.Animations) == 0 {
		return fmt.Errorf("export: manifest is incomplete")
	}
	if _, err := os.Stat(filepath.Join(outputDir, filepath.FromSlash(m.SpriteSheet))); err != nil {
		return fmt.Errorf("export: sprite sheet missing: %w", err)
	}
	for _, animation := range m.Animations {
		if len(animation.Frames) == 0 {
			return fmt.Errorf("export: animation %s/%s has no frames", animation.MotionID, animation.Direction)
		}
		for _, frame := range animation.Frames {
			if frame.DurationMs <= 0 {
				return fmt.Errorf("export: frame %s/%s/%d has invalid duration", animation.MotionID, animation.Direction, frame.Index)
			}
			if _, err := os.Stat(filepath.Join(outputDir, filepath.FromSlash(frame.File))); err != nil {
				return fmt.Errorf("export: missing frame %s: %w", frame.File, err)
			}
		}
	}
	return nil
}

func RecordHistory(path string, record HistoryRecord) error {
	if record.CreatedAt == "" {
		record.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("export: open history: %w", err)
	}
	defer f.Close()
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

func ReadHistory(path string) ([]HistoryRecord, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return []HistoryRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []HistoryRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var r HistoryRecord
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("export: invalid history: %w", err)
		}
		out = append(out, r)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func frameDimensions(animations []Animation) (int, int, int, error) {
	var w, h, maxFrames int
	for _, animation := range animations {
		if len(animation.Frames) > maxFrames {
			maxFrames = len(animation.Frames)
		}
		for _, frame := range animation.Frames {
			img, err := decodePNG(frame.PNG)
			if err != nil {
				return 0, 0, 0, err
			}
			if w == 0 {
				w, h = img.Bounds().Dx(), img.Bounds().Dy()
			} else if img.Bounds().Dx() != w || img.Bounds().Dy() != h {
				return 0, 0, 0, fmt.Errorf("export: inconsistent frame dimensions")
			}
		}
	}
	return w, h, maxFrames, nil
}

func decodePNG(data []byte) (image.Image, error) {
	return png.Decode(bytes.NewReader(data))
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("export: create %s: %w", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("export: encode %s: %w", path, err)
	}
	return nil
}

func writeTargetMetadata(dir, target string, manifest Manifest) error {
	name := target + ".json"
	payload := map[string]any{"target": target, "manifest": "manifest.json", "spriteSheet": manifest.SpriteSheet}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), data, 0o644)
}

func SortAnimations(animations []Animation) {
	sort.SliceStable(animations, func(i, j int) bool {
		if animations[i].MotionID != animations[j].MotionID {
			return animations[i].MotionID < animations[j].MotionID
		}
		return animations[i].Direction < animations[j].Direction
	})
}
