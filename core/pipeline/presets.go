package pipeline

import "fmt"

// StylePreset is a PerfectPixel generation style preset (阶段 3: PerfectPixel
// 四个风格预设). Each preset carries a deterministic English prompt fragment
// that is appended to the base filmstrip prompt so generation style is
// reproducible and auditable via the prompt snapshot.
type StylePreset struct {
	ID           string
	Name         string
	Description  string
	PromptSuffix string
}

// Built-in PerfectPixel style presets (四个风格预设).
var (
	StylePresetClassic = StylePreset{
		ID:           "pixel_classic",
		Name:         "经典像素",
		Description:  "复古像素风：有限调色板、硬边、无抗锯齿",
		PromptSuffix: "classic pixel art style, limited color palette, hard pixel edges, no anti-aliasing, no gradients",
	}
	StylePresetModern = StylePreset{
		ID:           "pixel_modern",
		Name:         "现代像素",
		Description:  "现代像素风：更多色彩层次与精细明暗",
		PromptSuffix: "modern pixel art style, rich palette, clean shading, crisp pixel edges, no anti-aliasing",
	}
	StylePresetMinimal = StylePreset{
		ID:           "pixel_minimal",
		Name:         "极简像素",
		Description:  "极简像素风：最少用色、强剪影、高辨识度",
		PromptSuffix: "minimal pixel art style, very few colors, strong silhouette, flat shading, hard pixel edges",
	}
	StylePresetRetroArcade = StylePreset{
		ID:           "pixel_retro",
		Name:         "粗颗粒复古",
		Description:  "粗颗粒复古风：大像素、高对比、街机质感",
		PromptSuffix: "chunky retro arcade pixel art style, large pixels, high contrast, bold outlines, hard edges",
	}
)

// StylePresets returns the built-in style presets in definition order.
func StylePresets() []StylePreset {
	return []StylePreset{StylePresetClassic, StylePresetModern, StylePresetMinimal, StylePresetRetroArcade}
}

// StylePresetByID looks up a style preset by id.
func StylePresetByID(id string) (StylePreset, error) {
	for _, p := range StylePresets() {
		if p.ID == id {
			return p, nil
		}
	}
	return StylePreset{}, fmt.Errorf("pipeline: unknown style preset %q", id)
}

// ActionPreset is a motion action preset (动作预设) that supplies the
// deterministic animation instruction inside the filmstrip prompt.
type ActionPreset struct {
	ID          string
	Name        string
	Description string
	PromptText  string
}

// Built-in action presets.
var (
	ActionIdle = ActionPreset{
		ID:          "idle",
		Name:        "待机",
		Description: "站立待机，轻微呼吸起伏",
		PromptText:  "idle animation, standing upright with a subtle breathing motion",
	}
	ActionWalk = ActionPreset{
		ID:          "walk",
		Name:        "行走",
		Description: "四方向行走循环",
		PromptText:  "walking animation, steady stride cycle",
	}
	ActionRun = ActionPreset{
		ID:          "run",
		Name:        "奔跑",
		Description: "快速奔跑循环",
		PromptText:  "running animation, fast stride cycle with body lean",
	}
	ActionAttack = ActionPreset{
		ID:          "attack",
		Name:        "攻击",
		Description: "单次攻击动作",
		PromptText:  "attack animation, one quick strike with anticipation and recovery",
	}
	ActionJump = ActionPreset{
		ID:          "jump",
		Name:        "跳跃",
		Description: "起跳-上升-下落-落地循环",
		PromptText:  "jump animation, crouch-anticipation, rising, falling, landing cycle",
	}
)

// ActionPresets returns the built-in action presets in definition order.
func ActionPresets() []ActionPreset {
	return []ActionPreset{ActionIdle, ActionWalk, ActionRun, ActionAttack, ActionJump}
}

// ActionPresetByID looks up an action preset by id.
func ActionPresetByID(id string) (ActionPreset, error) {
	for _, p := range ActionPresets() {
		if p.ID == id {
			return p, nil
		}
	}
	return ActionPreset{}, fmt.Errorf("pipeline: unknown action preset %q", id)
}
