package pipeline

import (
	"fmt"
	"strings"
	"time"
)

// ReferenceImageRef is the reference-image input recorded in a prompt snapshot
// (阶段 3: 1 主参考图 + 最多 2 辅助参考图 semantics carried into generation).
// base_sprite is the adopted base character sprite (对齐 perfectpixel：身份图
// 作为头号外发参考图，动画逐姿势复刻其外观与配色).
type ReferenceImageRef struct {
	MaterialID string `json:"materialId"`
	Role       string `json:"role"` // main_reference | auxiliary_reference | base_sprite
	Name       string `json:"name"`
}

// PromptInput is everything the deterministic prompt builder needs. It is the
// phase-3 data contract: the identity definition (description + reference
// images), a PerfectPixel style preset, an action preset, the logical canvas,
// and the frame/direction counts of the planned filmstrip. The full motion
// model (tasks 3.x) will feed the same fields.
type PromptInput struct {
	Description  string
	StylePreset  StylePreset
	ActionPreset ActionPreset
	References   []ReferenceImageRef
	CanvasWidth  int
	CanvasHeight int
	FrameCount   int
	Directions   int
	Feedback     string
}

// PromptSnapshot is the immutable record of a built generation prompt (提示词
// 快照). It captures every input that produced the prompt text so a later
// regeneration or audit can reproduce it exactly, and the generation
// confirmation can show the user exactly what will be sent out.
type PromptSnapshot struct {
	BuiltAt              time.Time           `json:"builtAt"`
	StylePresetID        string              `json:"stylePresetId"`
	ActionPresetID       string              `json:"actionPresetId"`
	Description          string              `json:"description"`
	ReferenceMaterialIDs []string            `json:"referenceMaterialIds"`
	References           []ReferenceImageRef `json:"references"`
	CanvasWidth          int                 `json:"canvasWidth"`
	CanvasHeight         int                 `json:"canvasHeight"`
	FrameCount           int                 `json:"frameCount"`
	Directions           int                 `json:"directions"`
	Feedback             string              `json:"feedback,omitempty"`
	Prompt               string              `json:"prompt"`
}

// spriteDesignContract 与 lowResPixelContract 逐字对齐 perfectpixel
// internal/sprite/prompt.go，对像素类风格注入。
func spriteDesignContract() string {
	return "Game-sprite design contract:\n" +
		"- Interpret the subject as a game-ready character sprite, not an illustration, poster, sticker, mascot logo, or concept-art render.\n" +
		"- Preserve the subject's identity through a strong silhouette, hairstyle, outfit shapes, accessories, weapon or signature prop, and dominant color blocks.\n" +
		"- Simplify anatomy into readable sprite shapes: compact torso, clear head shape, simple arms and legs, minimal joint detail, no tiny anatomy rendering.\n" +
		"- Hair, clothing layers, capes, hats, weapons and accessories should read as distinct hard-edged pixel shapes, not detailed painted textures.\n" +
		"- Keep the face simple at sprite scale: readable eyes and mouth, minimal facial detail, no realistic nose or painted skin texture.\n"
}

func lowResPixelContract() string {
	return "Pixel rendering contract:\n" +
		"- The image must look like a 32-64px game sprite enlarged to the canvas, not newly painted at high resolution.\n" +
		"- Use chunky square pixel blocks, clean 1px outline, solid tone clusters, limited palette, minimal two-step flat shading.\n" +
		"- No dithering, no smooth gradients, no soft shadow, no blur, no airbrush, no texture, no fine hair strands, no tiny jewelry detail that would vanish at 64px.\n" +
		"- Every important shape must remain readable when shrunk to a thumbnail: silhouette first, details second.\n"
}

// injectPixelContracts 按预设 ID 注入（有意不用 perfectpixel 的"文本包含 pixel
// 子串"判断——该判断会让 cartoon 的 "Never use pixelation" 误触发注入）。
func injectPixelContracts(styleID string) string {
	if styleID == "pixel" || styleID == "retro16" {
		return "\n" + spriteDesignContract() + "\n" + lowResPixelContract()
	}
	return ""
}

// BuildCharacterPrompt deterministically renders a single-character prompt.
// It reuses the same identity/style semantics as filmstrip generation while
// keeping the output contract explicitly single-image.
func BuildCharacterPrompt(description string, style StylePreset, width, height int, refs []ReferenceImageRef) (PromptSnapshot, error) {
	if strings.TrimSpace(style.ID) == "" {
		return PromptSnapshot{}, fmt.Errorf("pipeline: style preset is required")
	}
	if width <= 0 || height <= 0 {
		return PromptSnapshot{}, fmt.Errorf("pipeline: canvas size must be positive, got %dx%d", width, height)
	}
	refIDs := make([]string, 0, len(refs))
	for _, r := range refs {
		if r.Role != "main_reference" && r.Role != "auxiliary_reference" {
			return PromptSnapshot{}, fmt.Errorf("pipeline: invalid reference role %q", r.Role)
		}
		refIDs = append(refIDs, r.MaterialID)
	}
	var b strings.Builder
	b.WriteString("2D pixel art game character base sprite. ")
	if d := strings.TrimSpace(description); d != "" {
		b.WriteString("Character: ")
		b.WriteString(d)
		b.WriteString(". ")
	}
	b.WriteString(style.PromptSuffix)
	b.WriteString(injectPixelContracts(style.ID))
	// 角色生成契约 —— 对齐 perfectpixel 的 BuildCharacterPrompt：不要求透明
	// 背景（图像模型产不出 alpha，豆包等会画成白底），而是要求整幅纯洋红
	// #FF00FF 技术底，由管线 chroma key 抠掉；角色本身严禁洋红/粉/紫。
	fmt.Fprintf(&b, ". Render one centered character on a %dx%d pixel canvas, head to feet, vertically centered, occupying about three quarters of the canvas height with generous breathing room on every side. ", width, height)
	b.WriteString("BACKGROUND COLOR MANDATE (read this before drawing anything): fill the ENTIRE canvas background, edge to edge, with solid pure magenta #FF00FF (R=255, G=0, B=255) — one single flat color touching all four image borders. Do not use white, gray, black, or any other background color. No gradient, texture, scenery, floor, shadow, contact patch, panel, frame, or border of any kind. The character must avoid magenta, pink and purple entirely — clothing, props, highlights and effects included. No text, no borders, no watermark.")
	return PromptSnapshot{
		BuiltAt: time.Now().UTC(), StylePresetID: style.ID, Description: strings.TrimSpace(description),
		ReferenceMaterialIDs: refIDs, References: append([]ReferenceImageRef(nil), refs...),
		CanvasWidth: width, CanvasHeight: height, FrameCount: 1, Directions: 1, Prompt: b.String(),
	}, nil
}

// BuildPrompt deterministically renders the filmstrip generation prompt from a
// PromptInput and returns the immutable snapshot. No external calls are made.
func BuildPrompt(in PromptInput) (PromptSnapshot, error) {
	if strings.TrimSpace(in.StylePreset.ID) == "" {
		return PromptSnapshot{}, fmt.Errorf("pipeline: style preset is required")
	}
	if strings.TrimSpace(in.ActionPreset.ID) == "" {
		return PromptSnapshot{}, fmt.Errorf("pipeline: action preset is required")
	}
	if in.CanvasWidth <= 0 || in.CanvasHeight <= 0 {
		return PromptSnapshot{}, fmt.Errorf("pipeline: canvas size must be positive, got %dx%d", in.CanvasWidth, in.CanvasHeight)
	}
	if in.FrameCount <= 0 {
		return PromptSnapshot{}, fmt.Errorf("pipeline: frame count must be positive, got %d", in.FrameCount)
	}
	if in.Directions != 1 && in.Directions != 4 && in.Directions != 8 {
		return PromptSnapshot{}, fmt.Errorf("pipeline: direction count must be 1, 4 or 8, got %d", in.Directions)
	}
	main := 0
	aux := 0
	base := 0
	refIDs := make([]string, 0, len(in.References))
	for _, r := range in.References {
		switch r.Role {
		case "main_reference":
			main++
		case "auxiliary_reference":
			aux++
		case "base_sprite":
			base++
		default:
			return PromptSnapshot{}, fmt.Errorf("pipeline: invalid reference role %q", r.Role)
		}
		refIDs = append(refIDs, r.MaterialID)
	}
	if main > 1 || aux > 2 || base > 1 {
		return PromptSnapshot{}, fmt.Errorf("pipeline: reference role bounds violated (%d main, %d auxiliary, %d base sprite)", main, aux, base)
	}

	var b strings.Builder
	b.WriteString("2D pixel art game character animation asset. ")
	if d := strings.TrimSpace(in.Description); d != "" {
		b.WriteString("Character: ")
		b.WriteString(d)
		b.WriteString(". ")
	}
	// Subject lock —— 逐字对齐 perfectpixel BuildStripPrompt 的身份锁段落：
	// 已采纳的基础角色精灵图作为第一张参考图外发时，身份一致性优先级最高，
	// 调色板按参考图逐区域取样，全行固定机位与朝向。
	for _, r := range in.References {
		if r.Role == "base_sprite" {
			b.WriteString("Subject lock (top priority): the attached base sprite is the canonical character. Match it exactly across every pose: face, hairstyle, build, outfit, accessories, weapon or signature prop. ")
			b.WriteString("Palette is binding: re-sample each region's hue, saturation and value from the reference — skin, hair, every garment, every piece of gear. Do not re-tint, re-light, brighten, darken, or substitute a similar shade. ")
			b.WriteString("Hold one fixed camera and facing: the figure never rotates, mirrors, ages, or restyles between poses — only the body moves. ")
			break
		}
	}
	b.WriteString("Animation: ")
	b.WriteString(in.ActionPreset.PromptText)
	b.WriteString(". ")
	// 编排细节 —— 对齐 perfectpixel 的 "Choreography: {MotionHint}"：逐帧
	// 动作分解（身体各部位怎么动、脚步是否固定、起势-爆发-收势的节奏）。
	if c := strings.TrimSpace(in.ActionPreset.Choreography); c != "" {
		b.WriteString("Choreography: ")
		b.WriteString(c)
		b.WriteString(". ")
	}
	if in.ActionPreset.Loop {
		// 循环型动作：强调首尾无缝衔接，帧序列可无限循环。
		b.WriteString("The motion must loop seamlessly (the last frame flows back into the first frame). ")
	} else {
		// 一次性动作：强调清晰起止，不做成无限循环。
		b.WriteString("The motion plays once as a one-shot action with a clear start and a clear end pose. ")
	}
	b.WriteString(in.StylePreset.PromptSuffix)
	b.WriteString(injectPixelContracts(in.StylePreset.ID))
	if feedback := strings.TrimSpace(in.Feedback); feedback != "" {
		b.WriteString(" User feedback to address: ")
		b.WriteString(feedback)
		b.WriteString(". ")
	}
	// 条带布局与抠图契约 —— 对齐 perfectpixel 的关键做法：不要求透明背景
	// （图像模型产不出 alpha，要求透明只会得到满幅不透明图），而是要求整幅
	// 纯洋红 #FF00FF 技术底，由管线 chroma key 抠掉；姿势之间留宽幅洋红间隙
	// 供投影分割，姿势本身严禁接触洋红以外的构图元素。
	b.WriteString("BACKGROUND COLOR MANDATE: fill every pixel that is not part of a character pose with solid pure magenta #FF00FF (R=255, G=0, B=255), edge to edge — one single flat color touching all four image borders. No gradient, texture, scenery, floor, shadow, contact patch, panel, frame, or border of any kind. The character must avoid magenta, pink and purple entirely — clothing, props, highlights and effects included. ")
	fmt.Fprintf(&b, "Render the animation as one single horizontal row of exactly %d poses of the same character, ordered left to right, evenly spaced — %d poses, no more and no fewer; count them before finishing. ", in.FrameCount, in.FrameCount)
	// 对齐 perfectpixel 的行布局句：同一比例 + 约占格高 70-85% + 等拍连续动作。
	b.WriteString("Every pose is one whole connected body at one shared scale, each filling about 70-85% of the cell height, standing on one common ground line, centered in its share of the row — no pose may be noticeably smaller, larger, or set further back than the others. ")
	fmt.Fprintf(&b, "Treat the %d poses as evenly timed beats of one continuous motion — pose k is phase k of %d, and neighbours read as smooth in-betweens, never unrelated stances. ", in.FrameCount, in.FrameCount)
	b.WriteString("Leave a generous band of the flat magenta background between every pair of poses — poses never touch, overlap, or bridge into the neighbour, and no pose is clipped by the canvas edge. ")
	fmt.Fprintf(&b, "Each pose cell represents %dx%d pixels of the sprite sheet. ", in.CanvasWidth, in.CanvasHeight)
	b.WriteString("No film-strip sprocket holes or perforations, no panel dividers, no outline boxes, no vignette, no motion streaks, speed lines, blur or after-images, no free-floating sparkles or symbols, no text, no watermarks.")
	if len(in.References) > 0 {
		b.WriteString(" Reference images (the canonical base sprite defines the character appearance; follow the main reference for details): ")
		parts := make([]string, 0, len(in.References))
		for _, r := range in.References {
			label := "auxiliary"
			switch r.Role {
			case "main_reference":
				label = "main"
			case "base_sprite":
				label = "canonical base sprite"
			}
			name := strings.TrimSpace(r.Name)
			if name == "" {
				name = r.MaterialID
			}
			parts = append(parts, fmt.Sprintf("%s reference '%s'", label, name))
		}
		b.WriteString(strings.Join(parts, ", "))
		b.WriteString(".")
	}

	return PromptSnapshot{
		BuiltAt:              time.Now().UTC(),
		StylePresetID:        in.StylePreset.ID,
		ActionPresetID:       in.ActionPreset.ID,
		Description:          strings.TrimSpace(in.Description),
		ReferenceMaterialIDs: refIDs,
		References:           append([]ReferenceImageRef(nil), in.References...),
		CanvasWidth:          in.CanvasWidth,
		CanvasHeight:         in.CanvasHeight,
		FrameCount:           in.FrameCount,
		Directions:           in.Directions,
		Prompt:               b.String(),
	}, nil
}
