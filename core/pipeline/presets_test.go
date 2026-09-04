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
	seen := map[string]bool{}
	for _, p := range presets {
		if p.ID == "" || p.Name == "" || strings.TrimSpace(p.PromptText) == "" {
			t.Errorf("incomplete action preset: %+v", p)
		}
		if p.Category == "" {
			t.Errorf("action preset %q missing category", p.ID)
		}
		if p.Frames <= 0 {
			t.Errorf("action preset %q missing suggested frames", p.ID)
		}
		if seen[p.ID] {
			t.Errorf("duplicate action preset id %q", p.ID)
		}
		seen[p.ID] = true
	}
	if _, err := ActionPresetByID("walk"); err != nil {
		t.Errorf("walk preset missing: %v", err)
	}
	if _, err := ActionPresetByID("death"); err != nil {
		t.Errorf("death preset missing: %v", err)
	}
	if _, err := ActionPresetByID("roll"); err != nil {
		t.Errorf("roll preset missing: %v", err)
	}
	if _, err := ActionPresetByID("nope"); err == nil {
		t.Error("expected error for unknown action preset")
	}
	if got := ActionPresetFrames("death"); got <= 0 {
		t.Errorf("ActionPresetFrames(death) = %d, want > 0", got)
	}
	// 循环语义：行走是循环型、死亡是一次性。
	walk, _ := ActionPresetByID("walk")
	if !walk.Loop {
		t.Error("walk preset should be a looping motion")
	}
	death, _ := ActionPresetByID("death")
	if death.Loop {
		t.Error("death preset should be a one-shot motion")
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

// TestBuildPromptMagentaKeyingContract verifies the filmstrip prompt mandates
// the magenta keying background (对齐 perfectpixel：图像模型产不出透明背景，
// 管线靠抠洋红分离姿势) and never asks for transparency on the strip.
func TestBuildPromptMagentaKeyingContract(t *testing.T) {
	style, _ := StylePresetByID("pixel")
	action, _ := ActionPresetByID("walk")
	s, err := BuildPrompt(PromptInput{Description: "hero", StylePreset: style, ActionPreset: action, CanvasWidth: 256, CanvasHeight: 256, FrameCount: 4, Directions: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.Prompt, "#FF00FF") || !strings.Contains(s.Prompt, "BACKGROUND COLOR MANDATE") {
		t.Errorf("filmstrip prompt misses the magenta keying mandate: %s", s.Prompt)
	}
	if strings.Contains(s.Prompt, "transparent background") {
		t.Errorf("filmstrip prompt still asks for a transparent background (models cannot produce alpha): %s", s.Prompt)
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
	if !strings.Contains(s1.Prompt, "exactly 4 poses") || !strings.Contains(s1.Prompt, "32x32") {
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

// TestBuildPromptSubjectLock verifies the perfectpixel-aligned identity lock:
// a base_sprite reference injects the Subject lock section, is listed as the
// canonical reference, and duplicate base sprites violate the role bounds.
func TestBuildPromptSubjectLock(t *testing.T) {
	style, err := StylePresetByID("pixel")
	if err != nil {
		t.Fatal(err)
	}
	in := PromptInput{
		Description:  "hero",
		StylePreset:  style,
		ActionPreset: ActionIdle,
		CanvasWidth:  256, CanvasHeight: 256, FrameCount: 4, Directions: 1,
	}
	// 无 base_sprite：提示词不得包含 Subject lock。
	plain, err := BuildPrompt(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.Prompt, "Subject lock") {
		t.Errorf("prompt contains Subject lock without a base sprite: %s", plain.Prompt)
	}
	// 有 base_sprite：Subject lock 段落 + canonical 列表。
	in.References = []ReferenceImageRef{
		{MaterialID: "base-1", Role: "base_sprite", Name: "已采纳基础角色"},
		{MaterialID: "m1", Role: "main_reference", Name: "main"},
	}
	locked, err := BuildPrompt(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(locked.Prompt, "Subject lock (top priority)") ||
		!strings.Contains(locked.Prompt, "Palette is binding") ||
		!strings.Contains(locked.Prompt, "canonical base sprite reference") {
		t.Errorf("prompt misses the identity subject lock: %s", locked.Prompt)
	}
	// 两个 base_sprite：违反角色边界。
	in.References = append(in.References, ReferenceImageRef{MaterialID: "base-2", Role: "base_sprite"})
	if _, err := BuildPrompt(in); err == nil {
		t.Error("duplicate base_sprite references: expected error")
	}
}
