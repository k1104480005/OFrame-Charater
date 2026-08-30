package pipeline

import (
	"fmt"
	"strings"
	"time"
)

// ReferenceImageRef is the reference-image input recorded in a prompt snapshot
// (阶段 3: 1 主参考图 + 最多 2 辅助参考图 semantics carried into generation).
type ReferenceImageRef struct {
	MaterialID string `json:"materialId"`
	Role       string `json:"role"` // main_reference | auxiliary_reference
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
	fmt.Fprintf(&b, ". Render one centered character on a transparent %dx%d pixel canvas, no text, no borders, no watermark.", width, height)
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
	refIDs := make([]string, 0, len(in.References))
	for _, r := range in.References {
		switch r.Role {
		case "main_reference":
			main++
		case "auxiliary_reference":
			aux++
		default:
			return PromptSnapshot{}, fmt.Errorf("pipeline: invalid reference role %q", r.Role)
		}
		refIDs = append(refIDs, r.MaterialID)
	}
	if main > 1 || aux > 2 {
		return PromptSnapshot{}, fmt.Errorf("pipeline: reference role bounds violated (%d main, %d auxiliary)", main, aux)
	}

	var b strings.Builder
	b.WriteString("2D pixel art game character animation asset. ")
	if d := strings.TrimSpace(in.Description); d != "" {
		b.WriteString("Character: ")
		b.WriteString(d)
		b.WriteString(". ")
	}
	b.WriteString("Animation: ")
	b.WriteString(in.ActionPreset.PromptText)
	b.WriteString(". ")
	b.WriteString(in.StylePreset.PromptSuffix)
	b.WriteString(injectPixelContracts(in.StylePreset.ID))
	b.WriteString(". Render the animation as one single horizontal filmstrip containing ")
	fmt.Fprintf(&b, "%d frames", in.FrameCount)
	fmt.Fprintf(&b, " of %dx%d pixels each, frames in left-to-right order, ", in.CanvasWidth, in.CanvasHeight)
	b.WriteString("character centered in every frame, transparent background, no borders, no text, no watermarks.")
	if len(in.References) > 0 {
		b.WriteString(" Reference images (follow the main reference for the character appearance): ")
		parts := make([]string, 0, len(in.References))
		for _, r := range in.References {
			label := "auxiliary"
			if r.Role == "main_reference" {
				label = "main"
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
