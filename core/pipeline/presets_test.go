package pipeline

import (
	"strings"
	"testing"
)

// 阶段 3: PerfectPixel 四个风格预设 + 动作预设数据结构 + 提示词快照.

func TestStylePresetsCount(t *testing.T) {
	presets := StylePresets()
	if len(presets) != 4 {
		t.Fatalf("style presets = %d, want 4 (PerfectPixel 四个风格预设)", len(presets))
	}
	seen := map[string]bool{}
	for _, p := range presets {
		if p.ID == "" || p.Name == "" || strings.TrimSpace(p.PromptSuffix) == "" {
			t.Errorf("incomplete style preset: %+v", p)
		}
		if seen[p.ID] {
			t.Errorf("duplicate style preset id %q", p.ID)
		}
		seen[p.ID] = true
	}
}

func TestPaletteSizeForStyle(t *testing.T) {
	cases := map[string]int{"retro16": 16, "pixel": 32, "chibi": 0, "cartoon": 0, "custom": 0}
	for style, want := range cases {
		if got := PaletteSizeForStyle(style); got != want {
			t.Errorf("PaletteSizeForStyle(%q) = %d, want %d", style, got, want)
		}
	}
}

func TestActionPresets(t *testing.T) {
	presets := ActionPresets()
	if len(presets) < 4 {
		t.Fatalf("action presets = %d, want at least 4", len(presets))
	}
	for _, p := range presets {
		if p.ID == "" || p.Name == "" || strings.TrimSpace(p.PromptText) == "" {
			t.Errorf("incomplete action preset: %+v", p)
		}
	}
	if _, err := ActionPresetByID("walk"); err != nil {
		t.Errorf("walk preset missing: %v", err)
	}
	if _, err := ActionPresetByID("nope"); err == nil {
		t.Error("expected error for unknown action preset")
	}
}

func TestPixelContractsByStyle(t *testing.T) {
	action := ActionIdle
	for _, id := range []string{"pixel", "retro16"} {
		style, err := StylePresetByID(id)
		if err != nil {
			t.Fatal(err)
		}
		character, err := BuildCharacterPrompt("hero", style, 32, 32, nil)
		if err != nil {
			t.Fatal(err)
		}
		filmstrip, err := BuildPrompt(PromptInput{Description: "hero", StylePreset: style, ActionPreset: action, CanvasWidth: 32, CanvasHeight: 32, FrameCount: 4, Directions: 1})
		if err != nil {
			t.Fatal(err)
		}
		for _, prompt := range []string{character.Prompt, filmstrip.Prompt} {
			if !strings.Contains(prompt, "Game-sprite design contract:") || !strings.Contains(prompt, "Pixel rendering contract:") {
				t.Errorf("%s prompt misses pixel contracts: %s", id, prompt)
			}
		}
	}
	for _, id := range []string{"chibi", "cartoon"} {
		style, err := StylePresetByID(id)
		if err != nil {
			t.Fatal(err)
		}
		character, err := BuildCharacterPrompt("hero", style, 32, 32, nil)
		if err != nil {
			t.Fatal(err)
		}
		filmstrip, err := BuildPrompt(PromptInput{Description: "hero", StylePreset: style, ActionPreset: action, CanvasWidth: 32, CanvasHeight: 32, FrameCount: 4, Directions: 1})
		if err != nil {
			t.Fatal(err)
		}
		for _, prompt := range []string{character.Prompt, filmstrip.Prompt} {
			if strings.Contains(prompt, "Game-sprite design contract:") || strings.Contains(prompt, "Pixel rendering contract:") {
				t.Errorf("%s prompt unexpectedly contains pixel contracts: %s", id, prompt)
			}
		}
	}
}

// TestBuildPromptDeterministic verifies the prompt is deterministic for the
// same inputs (two builds produce identical prompt text).
func TestBuildPromptDeterministic(t *testing.T) {
	style, _ := StylePresetByID("pixel")
	action, _ := ActionPresetByID("walk")
	in := PromptInput{
		Description:  "a red-haired pixel hero",
		StylePreset:  style,
		ActionPreset: action,
		References: []ReferenceImageRef{
			{MaterialID: "m1", Role: "main_reference", Name: "正面"},
			{MaterialID: "a1", Role: "auxiliary_reference", Name: "配色"},
		},
		CanvasWidth:  32,
		CanvasHeight: 32,
		FrameCount:   4,
		Directions:   4,
	}
	s1, err := BuildPrompt(in)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := BuildPrompt(in)
	if err != nil {
		t.Fatal(err)
	}
	if s1.Prompt != s2.Prompt {
		t.Errorf("prompt not deterministic:\n%s\n---\n%s", s1.Prompt, s2.Prompt)
	}
	if !strings.Contains(s1.Prompt, "4 frames") || !strings.Contains(s1.Prompt, "32x32") {
		t.Errorf("prompt misses canvas/frame details: %s", s1.Prompt)
	}
	if !strings.Contains(s1.Prompt, "main reference '正面'") {
		t.Errorf("prompt misses main reference semantics: %s", s1.Prompt)
	}
	if len(s1.ReferenceMaterialIDs) != 2 || s1.StylePresetID != "pixel" || s1.ActionPresetID != "walk" {
		t.Errorf("snapshot fields wrong: %+v", s1)
	}
}

// TestBuildPromptValidation verifies invalid inputs are rejected.
func TestBuildPromptValidation(t *testing.T) {
	style, _ := StylePresetByID("pixel")
	action, _ := ActionPresetByID("idle")
	base := PromptInput{
		Description:  "hero",
		StylePreset:  style,
		ActionPreset: action,
		CanvasWidth:  16,
		CanvasHeight: 16,
		FrameCount:   4,
		Directions:   1,
	}
	cases := []struct {
		name string
		mut  func(*PromptInput)
	}{
		{"missing style", func(in *PromptInput) { in.StylePreset = StylePreset{} }},
		{"missing action", func(in *PromptInput) { in.ActionPreset = ActionPreset{} }},
		{"bad canvas", func(in *PromptInput) { in.CanvasWidth = 0 }},
		{"bad frames", func(in *PromptInput) { in.FrameCount = 0 }},
		{"bad directions", func(in *PromptInput) { in.Directions = 3 }},
		{"two mains", func(in *PromptInput) {
			in.References = []ReferenceImageRef{{MaterialID: "m1", Role: "main_reference"}, {MaterialID: "m2", Role: "main_reference"}}
		}},
		{"three auxes", func(in *PromptInput) {
			in.References = []ReferenceImageRef{
				{MaterialID: "a1", Role: "auxiliary_reference"},
				{MaterialID: "a2", Role: "auxiliary_reference"},
				{MaterialID: "a3", Role: "auxiliary_reference"},
			}
		}},
	}
	for _, tc := range cases {
		in := base
		tc.mut(&in)
		if _, err := BuildPrompt(in); err == nil {
			t.Errorf("%s: expected error", tc.name)
		}
	}
}
