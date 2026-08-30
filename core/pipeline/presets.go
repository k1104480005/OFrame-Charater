package pipeline

import (
	"fmt"
	"strings"
)

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

// 契约文本来源：perfectpixel-studio internal/sprite/prompt.go（逐字对齐，勿改写）
const (
	ContractPixel = "true low-resolution pixel-art game sprite, like a 32-64px sprite enlarged on the canvas, " +
		"chunky readable silhouette, clean dark 1px outline, visible square pixel blocks, " +
		"grid-aligned hard pixel edges, limited shared palette, solid tone clusters, " +
		"flat color shading with at most one highlight step and one shadow step, " +
		"simple readable face and clearly separated limbs. " +
		"Never use painterly rendering, smooth gradients, airbrush shading, glossy lighting, " +
		"anti-aliased fine detail, high-definition pixel art, fine-grained pixel art, anime illustration, concept art, or 3D rendering."
	ContractChibi = "cute chibi game sprite with oversized head and small body, " +
		"bold dark outline, flat bright colors, minimal shading, large expressive eyes, " +
		"clean cartoon shapes readable at small size. " +
		"Never use realistic proportions, gradients, or painterly detail."
	ContractCartoon = "clean 2D cartoon game sprite, bold uniform outline, flat vivid colors, " +
		"simple two-tone cel shading, smooth rounded shapes, expressive but simple face. " +
		"Never use pixelation, gradients, photo textures, or 3D rendering."
	ContractRetro16 = "16-bit retro console era game sprite, restrained palette of 16-24 colors, " +
		"dark outline, dithering only where needed, compact proportions, " +
		"crisp hard pixel edges like a classic arcade fighter sprite. " +
		"Never use modern smooth shading or high-resolution detail."
)

var (
	StylePresetClassic = StylePreset{ID: "pixel", Name: "低分辨率像素", Description: "游戏精灵像素风：按 32-64px 低分辨率结构设计，强调清晰轮廓、硬边色块和简化细节；启用调色板量化，生成后最多保留 32 色", PromptSuffix: ContractPixel}
	StylePresetChibi   = StylePreset{ID: "chibi", Name: "Q版", Description: "Q版游戏精灵：大头小身、粗描边、平涂亮色和大眼睛；不启用调色板量化，保留生成颜色", PromptSuffix: ContractChibi}
	StylePresetCartoon = StylePreset{ID: "cartoon", Name: "卡通（非像素）", Description: "干净 2D 卡通：圆润造型、统一描边和双色赛璐璐明暗，明确不做像素化；不启用调色板量化，保留生成颜色", PromptSuffix: ContractCartoon}
	StylePresetRetro16 = StylePreset{ID: "retro16", Name: "复古 16-bit", Description: "复古 16-bit 游戏精灵：克制色板、深色描边和硬边造型；启用调色板量化，生成后最多保留 16 色", PromptSuffix: ContractRetro16}
)

func StylePresets() []StylePreset {
	return []StylePreset{StylePresetClassic, StylePresetChibi, StylePresetCartoon, StylePresetRetro16}
}

// PaletteSizeForStyle 对齐 perfectpixel pixelize.go：retro16→16，pixel→32，
// 其余（chibi/cartoon/custom）返回 0 表示跳过量化强制。
func PaletteSizeForStyle(styleID string) int {
	switch styleID {
	case "retro16":
		return 16
	case "pixel":
		return 32
	default:
		return 0
	}
}

// NegativePromptForStyle returns the built-in negative prompt portion of a style.
func NegativePromptForStyle(styleID string) string {
	style, err := StylePresetByID(styleID)
	if err != nil {
		return ""
	}
	const marker = "Never use "
	idx := strings.Index(style.PromptSuffix, marker)
	if idx < 0 {
		return ""
	}
	return style.PromptSuffix[idx:]
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
