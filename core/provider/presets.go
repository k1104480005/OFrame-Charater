package provider

import "fmt"

// Stable keys of the seven FrameBaker quick presets plus the Agnes specialist
// entry (align-framebaker-providers task 1.3 + 人工验收反馈：agnes 多模态专项
// 也走预设入口). A key is the durable contract between the settings UI, drafts,
// tests and documentation: once shipped it never changes, so providers added
// from a preset remain identifiable across restarts. Every preset key
// coincides with its protocol Type — one lookup namespace instead of two.
const (
	PresetKeyOpenAI     = ProviderOpenAI         // gpt-image 图像 + Chat Completions 文本
	PresetKeyDashscope  = ProviderTypeDashscope  // 百炼 DashScope 原生协议
	PresetKeyGemini     = ProviderTypeGemini     // banana/Gemini generateContent 协议
	PresetKeyMiniMax    = ProviderTypeMiniMax    // MiniMax 图片协议
	PresetKeyVolcengine = ProviderTypeVolcengine // 火山方舟 Ark 原生协议（豆包）
	PresetKeyCLI        = ProviderTypeCLI        // 自定义 CLI（argv 执行）
	PresetKeyCustomAPI  = ProviderTypeAPI        // 自定义通用 OpenAI 兼容接口
	PresetKeyAgnes      = ProviderAgnes          // Agnes 专项（多模态：图像 + 文本）
)

// Default endpoints of the protocol presets whose addresses do not yet have a
// dedicated constant (the OpenAI and 火山方舟/豆包 presets reuse
// DefaultOpenAIBaseURL / DefaultDoubaoBaseURL from config.go so all preset
// endpoints share one source of truth).
const (
	DefaultDashscopeBaseURL = "https://dashscope.aliyuncs.com/api/v1"            // 百炼原生协议根路径
	DefaultGeminiBaseURL    = "https://generativelanguage.googleapis.com/v1beta" // Gemini generateContent 根路径
	DefaultMiniMaxBaseURL   = "https://api.minimax.chat/v1"                      // MiniMax 图片接口根路径
)

// Preset is the default backend description of one FrameBaker quick preset:
// a stable key, display name, protocol Type, default Base URL, per-capability
// model catalogs (Image/Video/Text) and capability metadata. The settings UI
// fills these values into a new provider card draft; nothing here performs
// network calls, and strict validation still happens when the resulting
// config is saved (ValidateForAdd / Validate).
//
// Capability metadata stays truthful (design D1/D2):
//   - Capabilities.Video is false on every preset until a video adapter
//     executes real calls (预留目录不代表可调用).
//   - Model catalogs ship EMPTY on every preset (人工验收反馈：不预填模型) —
//     users type model names manually or pick them from the gateway's
//     获取模型 response, which is the authoritative list.
//   - The CLI preset carries no HTTP endpoint (BaseURL "") because it runs a
//     local executable through argv; the 自定义 API preset ships an empty
//     BaseURL and empty catalogs on purpose — the user provides them.
//   - Accessor functions below always return deep copies: callers cannot
//     mutate the global default snapshot.
type Preset struct {
	Key          string         // 稳定预设标识（与协议 Type 同值）
	Name         string         // 设置页显示名
	Description  string         // 字段说明与能力边界（必要元数据）
	Type         string         // 协议类型（ProviderConfig.Type 接受值）
	BaseURL      string         // 默认接口地址（CLI 为空串：无 HTTP 端点）
	Capabilities Capabilities   // 能力元数据；Video 恒为 false（视频未接入）
	ImageModels  []string       // 默认图片模型目录（可编辑草稿）
	VideoModels  []string       // 默认视频模型目录（预留标注，不代表可调用）
	TextModels   []string       // 默认文本模型目录（可编辑草稿）
	CLIDraft     ProviderConfig // CLI 结构化参数草稿（仅 CLI 预设填充）
}

// presetDefaults is the canonical preset table in the fixed FrameBaker order
// (OpenAI、百炼、banana/Gemini、MiniMax、火山方舟/豆包、自定义 CLI、自定义 API).
// It is unexported: every read goes through the cloning accessors below so
// the package's global state can never be modified through a returned value.
//
// 人工验收反馈：预设不预填任何模型 —— 所有目录默认留空，用户手动输入或
// 「获取模型」点选（权威列表永远来自网关的 /models 返回）。能力徽标仍然
// 声明该协议可执行的模态；端点/密钥为各预设的真实默认值。
var presetDefaults = []Preset{
	{
		Key:         PresetKeyOpenAI,
		Name:        "OpenAI",
		Description: "官方 OpenAI Images 与 Chat Completions 协议；填写 API Key 后即可用于图像与文本生成。模型需手动输入或获取模型后点选。",
		Type:        ProviderOpenAI,
		BaseURL:     DefaultOpenAIBaseURL,
		Capabilities: Capabilities{
			Image: true,
			Text:  true,
		},
	},
	{
		Key:         PresetKeyDashscope,
		Name:        "百炼（DashScope）",
		Description: "阿里云百炼原生协议：通义万相 Text2Image 生成图像、Qwen 文本生成；视频目录预留（管线接入前不可调用）。模型需手动输入或获取模型后点选。",
		Type:        ProviderTypeDashscope,
		BaseURL:     DefaultDashscopeBaseURL,
		Capabilities: Capabilities{
			Image: true,
			Text:  true,
		},
	},
	{
		Key:         PresetKeyGemini,
		Name:        "banana / Gemini",
		Description: "Google Gemini generateContent 协议（nano banana）：inlineData 支持参考图输入，图像与文本共用同一密钥。模型需手动输入或获取模型后点选。",
		Type:        ProviderTypeGemini,
		BaseURL:     DefaultGeminiBaseURL,
		Capabilities: Capabilities{
			Image: true,
			Text:  true,
		},
	},
	{
		Key:         PresetKeyMiniMax,
		Name:        "MiniMax",
		Description: "MiniMax 图片生成协议；仅声明图片能力（视频能力接入前不开放调用，保留视频模型配置位）。模型需手动输入或获取模型后点选。",
		Type:        ProviderTypeMiniMax,
		BaseURL:     DefaultMiniMaxBaseURL,
		Capabilities: Capabilities{
			Image: true,
		},
	},
	{
		Key:         PresetKeyVolcengine,
		Name:        "火山方舟 / 豆包",
		Description: "火山方舟 Ark 原生协议（豆包）：Seedream 图像、Doubao 文本；视频目录预留（管线接入前不可调用）。模型需手动输入或获取模型后点选。",
		Type:        ProviderTypeVolcengine,
		BaseURL:     DefaultDoubaoBaseURL,
		Capabilities: Capabilities{
			Image: true,
			Text:  true,
		},
	},
	{
		Key:         PresetKeyCLI,
		Name:        "自定义 CLI",
		Description: "自定义命令行工具：命令与参数按 argv 数组执行（不经 shell），输出文件由应用校验；无固定接口地址，模型目录按所用工具自行填写。",
		Type:        ProviderTypeCLI,
		Capabilities: Capabilities{
			Image: true, // CLI 以图片资产产出为主；其余能力在任务支持落地后再声明
		},
		// 无预置地址与模型：CLI 工具完全由用户定义（设计 D5 的结构化草稿）。
		// 参数旗标给出常见默认值（可编辑）；命令本身留空由用户填写。
		CLIDraft: ProviderConfig{
			Type:           ProviderTypeCLI,
			CLIPromptArg:   "--prompt",
			CLIOutputArg:   "--output",
			CLIModelArg:    "--model",
			CLIRefImageArg: "--ref",
		},
	},
	{
		Key:         PresetKeyCustomAPI,
		Name:        "自定义 API",
		Description: "自定义 OpenAI 兼容接口：填入 Base URL 与密钥后测试连接、获取模型并归类到图片/文本目录；默认不预置地址与模型。",
		Type:        ProviderTypeAPI,
		Capabilities: Capabilities{
			Image: true,
			Text:  true, // OpenAI 兼容接口通常同时提供 chat/completions
		},
		// 地址与模型留空：由用户填写（默认草稿，保存时严格校验）。
	},
	{
		// 额外需求（人工验收反馈）：Agnes AI 真实网关接入（免费多模态）。
		// 全模态方向：图像 + 文本可配置，视频目录预留（管线接入前不可调用）。
		Key:         PresetKeyAgnes,
		Name:        "Agnes（免费多模态）",
		Description: "Agnes AI 免费多模态网关（OpenAI 兼容）：图像走 /images/generations，文本走 /chat/completions；视频目录预留，管线接入前不可调用。填入 API Key 后即可测试连接与获取模型。",
		Type:        ProviderAgnes,
		BaseURL:     DefaultAgnesBaseURL,
		Capabilities: Capabilities{
			Image: true,
			Text:  true,
		},
	},
}

// Presets returns the preset descriptions in their fixed order (the seven
// FrameBaker quick presets plus the Agnes specialist entry). Each call returns
// fresh deep copies: mutating the result (including its model catalogs) never
// affects subsequent reads.
func Presets() []Preset {
	out := make([]Preset, len(presetDefaults))
	for i := range presetDefaults {
		out[i] = presetDefaults[i].snapshot()
	}
	return out
}

// PresetByKey looks up a preset by its stable key and returns an independent
// copy. An unknown key yields an error (never a silently empty preset).
func PresetByKey(key string) (Preset, error) {
	for i := range presetDefaults {
		if presetDefaults[i].Key == key {
			return presetDefaults[i].snapshot(), nil
		}
	}
	return Preset{}, fmt.Errorf("provider: unknown provider preset %q", key)
}

// snapshot returns a deep copy of the preset: the scalar fields are copied by
// value and each nested slice (model catalogs and the CLI draft's extra args)
// is re-sliced so the copy shares no backing array with the global table.
func (p Preset) snapshot() Preset {
	out := p
	out.ImageModels = cloneList(p.ImageModels)
	out.VideoModels = cloneList(p.VideoModels)
	out.TextModels = cloneList(p.TextModels)
	out.CLIDraft.CLIExtraArgs = cloneList(p.CLIDraft.CLIExtraArgs)
	out.CLIDraft.ImageModels = cloneList(p.CLIDraft.ImageModels)
	out.CLIDraft.VideoModels = cloneList(p.CLIDraft.VideoModels)
	out.CLIDraft.TextModels = cloneList(p.CLIDraft.TextModels)
	return out
}

// PresetConfigDraft returns a provider-config draft for one preset key: the
// protocol Type, display name, default Base URL, per-capability model catalogs
// and — for the CLI preset — the structured CLI argument defaults. ProviderID
// stays empty (the service derives the slug on add) and the API key is never
// part of a draft. The result is an independent deep copy.
func PresetConfigDraft(key string) (ProviderConfig, error) {
	p, err := PresetByKey(key)
	if err != nil {
		return ProviderConfig{}, err
	}
	cfg := ProviderConfig{
		Type:           p.Type,
		Name:           p.Name,
		BaseURL:        p.BaseURL,
		ImageModels:    p.ImageModels,
		VideoModels:    p.VideoModels,
		TextModels:     p.TextModels,
		CLICommand:     p.CLIDraft.CLICommand,
		CLIPromptArg:   p.CLIDraft.CLIPromptArg,
		CLIOutputArg:   p.CLIDraft.CLIOutputArg,
		CLIModelArg:    p.CLIDraft.CLIModelArg,
		CLIRefImageArg: p.CLIDraft.CLIRefImageArg,
		CLIExtraArgs:   p.CLIDraft.CLIExtraArgs,
	}
	return cfg, nil
}

// cloneList copies a string slice (nil stays nil so "not configured" remains
// distinguishable from an empty list).
func cloneList(items []string) []string {
	if items == nil {
		return nil
	}
	out := make([]string, len(items))
	copy(out, items)
	return out
}
