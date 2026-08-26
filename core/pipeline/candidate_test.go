package pipeline

import (
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func cand(id string, overall float64) Candidate {
	return Candidate{
		ID:     id,
		Status: CandidatePending,
		Scores: QualityScores{Overall: overall},
	}
}

func TestCandidateSetBest(t *testing.T) {
	cs := NewCandidateSet()
	cs.Add(cand("a", 0.8))
	cs.Add(cand("b", 0.9))
	cs.Add(cand("c", 0.7))
	best := cs.Best()
	if best == nil || best.ID != "b" {
		t.Fatalf("Best() = %+v, want candidate b", best)
	}
	if cs.Count() != 3 {
		t.Fatalf("Count = %d, want 3", cs.Count())
	}
	if len(cs.All()) != 3 {
		t.Fatalf("All() = %d, want 3", len(cs.All()))
	}
}

func TestCandidateSetTieKeepsFirst(t *testing.T) {
	cs := NewCandidateSet()
	cs.Add(cand("a", 0.8))
	cs.Add(cand("b", 0.8))
	if best := cs.Best(); best.ID != "a" {
		t.Fatalf("tie: Best() = %q, want a (first, deterministic)", best.ID)
	}
}

func TestCandidateSetNeverEmpty(t *testing.T) {
	cs := NewCandidateSet()
	if cs.Best() != nil {
		t.Fatal("empty set: Best() must be nil")
	}
	// A failed candidate is still retained — 生成结果保留最佳候选而非空手返回.
	failed := cand("f", 0.2)
	failed.Status = CandidateFailed
	failed.Reason = "slicing mismatch"
	cs.Add(failed)
	if cs.Best() == nil || cs.Best().ID != "f" {
		t.Fatal("failed candidate must be retained as the best-so-far")
	}
}

func TestSaveCandidate(t *testing.T) {
	dir := t.TempDir()
	prompt, err := BuildPrompt(PromptInput{
		StylePreset:  StylePresetClassic,
		ActionPreset: ActionWalk,
		CanvasWidth:  32,
		CanvasHeight: 32,
		FrameCount:   4,
		Directions:   1,
	})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	strip := stripWithFrames(1, 16, 16, 0, color.RGBA{R: 255, G: 0, B: 255, A: 255})
	pngData, err := EncodeFilmstripPNG(strip)
	if err != nil {
		t.Fatalf("EncodeFilmstripPNG: %v", err)
	}
	c := Candidate{
		ID:           "cand-1",
		Prompt:       prompt,
		FilmstripPNG: pngData,
		Scores:       QualityScores{Overall: 0.99},
		Status:       CandidatePending,
	}
	if err := SaveCandidate(dir, c); err != nil {
		t.Fatalf("SaveCandidate: %v", err)
	}
	for _, name := range []string{"filmstrip.png", "prompt.json", "scores.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing saved artifact %s: %v", name, err)
		}
	}
	// The prompt snapshot round-trips (保留 prompt 快照).
	data, err := os.ReadFile(filepath.Join(dir, "prompt.json"))
	if err != nil {
		t.Fatalf("read prompt.json: %v", err)
	}
	var back PromptSnapshot
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("parse prompt.json: %v", err)
	}
	if back.Prompt != prompt.Prompt || back.StylePresetID != prompt.StylePresetID {
		t.Errorf("prompt snapshot round trip mismatch: %q vs %q", back.Prompt, prompt.Prompt)
	}
	// The original filmstrip round-trips (保留原始产物).
	raw, err := os.ReadFile(filepath.Join(dir, "filmstrip.png"))
	if err != nil {
		t.Fatalf("read filmstrip.png: %v", err)
	}
	decoded, err := DecodeFilmstrip(raw)
	if err != nil {
		t.Fatalf("decode saved filmstrip: %v", err)
	}
	if decoded.Bounds().Dx() != 16 || decoded.Bounds().Dy() != 16 {
		t.Errorf("saved filmstrip = %dx%d, want 16x16", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}
