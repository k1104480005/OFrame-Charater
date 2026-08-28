package provider

import (
	"net/url"
	"sort"
	"strings"
)

// builtinIDOrder is the deterministic recovery order used when the stored
// active provider is unusable: it matches DefaultSettings' precedence, so a
// repaired store always lands on Doubao first (首次生成默认路由到 Doubao).
var builtinIDOrder = []string{ProviderDoubao, ProviderOpenAI, ProviderAgnes}

// NormalizeSettings migrates a loaded provider-settings payload to the current
// persisted shape (align-framebaker-providers task 1.2, updated by the 人工验收
// decision "不需要固定显示 3 个内置 Provider"). It is a pure function over a
// snapshot; the caller decides what to persist:
//
//   - Missing or empty Providers map → stays EMPTY: a fresh install shows no
//     provider cards until the user adds one from the seven presets (不再补种
//     三个内置 Provider). EXISTING entries are always preserved.
//   - Non-empty map: every entry is carried over verbatim: 保留现有
//     Doubao/OpenAI/Agnes/compatible 配置，包括旧单模型 Model/VideoModel/
//     TextModel 字段（继续经 EffectiveImageModels/EffectiveVideoModels/
//     EffectiveTextModels 回退访问）、缺失或带空白/重复的数组字段（读取归一化、
//     存储不篡改）以及 API key。未知/自定义条目原样保留。
//   - A custom provider's explicit Type is never rewritten: cli / api /
//     dashscope / gemini / minimax / volcengine / compatible all survive
//     normalization intact (显式 Type 绝不静默改成 compatible).
//   - ActiveProvider: surrounding whitespace is trimmed; an empty active or one
//     pointing at an id absent from Providers is recovered to the first
//     built-in still present (doubao → openai → agnes), then to the
//     lexicographically first remaining id — 激活选择在重启后总能落在一个已配置的
//     Provider 上。With NO providers at all the active stays empty and
//     generation reports the readable "no provider configured" error.
//
// No API keys are validated and no network calls are made. Normalization does
// not add missing built-in entries to a non-empty map (existing configurations
// and deletions are preserved exactly as stored — deleted providers stay
// deleted). The function is idempotent:
// NormalizeSettings(NormalizeSettings(s)) equals NormalizeSettings(s).
func NormalizeSettings(in Settings) Settings {
	out := Settings{
		ActiveProvider:    strings.TrimSpace(in.ActiveProvider),
		EnhanceProviderID: in.EnhanceProviderID,
		EnhanceModel:      in.EnhanceModel,
	}
	if len(in.Providers) == 0 {
		out.Providers = map[string]ProviderConfig{}
		out.ActiveProvider = ""
		out.EnhanceProviderID, out.EnhanceModel = "", ""
		return out
	}
	out.Providers = make(map[string]ProviderConfig, len(in.Providers))
	for id, c := range in.Providers {
		// Agnes 真实接入迁移（人工验收反馈）：旧版占位地址 api.agnes.local 是
		// 本应用独家历史默认值，无歧义 —— 仅对 Agnes 类型条目改写为真实网关，
		// 用户已保存的 Agnes 卡片在下次加载后即可用（design D7: 无歧义旧值允许迁移）。
		if c.EffectiveType() == ProviderAgnes {
			if u, err := url.Parse(c.EffectiveBaseURL()); err == nil &&
				strings.EqualFold(u.Hostname(), PlaceholderAgnesHost) {
				c.BaseURL = DefaultAgnesBaseURL
			}
			// 旧占位模型名 → 官方文档 ID（同样是本应用独家发明值，无歧义）：
			// agnes-image-v1 → agnes-image-21-flash，agnes-text-v1 → agnes-2.5-flash，
			// agnes-video-v1 → agnes-video-2.5-flash（人工验收反馈）。
			remapAgnes := func(list []string) []string {
				out := make([]string, len(list))
				for i, m := range list {
					switch strings.TrimSpace(m) {
					case "agnes-image-v1":
						out[i] = DefaultAgnesModel
					case "agnes-text-v1":
						out[i] = DefaultAgnesTextModel
					case "agnes-video-v1":
						out[i] = DefaultAgnesVideoModel
					default:
						out[i] = m
					}
				}
				return out
			}
			if len(c.ImageModels) > 0 {
				c.ImageModels = remapAgnes(c.ImageModels)
			}
			if len(c.TextModels) > 0 {
				c.TextModels = remapAgnes(c.TextModels)
			}
			if len(c.VideoModels) > 0 {
				c.VideoModels = remapAgnes(c.VideoModels)
			}
			if strings.TrimSpace(c.Model) == "agnes-image-v1" {
				c.Model = DefaultAgnesModel
			}
			if strings.TrimSpace(c.TextModel) == "agnes-text-v1" {
				c.TextModel = DefaultAgnesTextModel
			}
		}
		out.Providers[id] = c
	}

	active := out.ActiveProvider
	if _, ok := out.Providers[active]; !ok {
		for _, id := range builtinIDOrder {
			if _, ok := out.Providers[id]; ok {
				active = id
				break
			}
		}
		if _, ok := out.Providers[active]; !ok {
			ids := make([]string, 0, len(out.Providers))
			for id := range out.Providers {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			active = ids[0]
		}
	}
	out.ActiveProvider = active

	// Prompt-enhancement association (task 5.5): a dangling provider reference
	// (deleted or unknown id) is cleared so the association falls back to the
	// active provider's text model instead of blocking generation. A dangling
	// MODEL reference is kept for display but resolves to the provider default
	// at use time (NormalizeSettings never invents a model).
	if ref := strings.TrimSpace(in.EnhanceProviderID); ref != "" {
		if _, ok := out.Providers[ref]; ok {
			out.EnhanceProviderID = ref
			out.EnhanceModel = strings.TrimSpace(in.EnhanceModel)
		} else {
			out.EnhanceProviderID, out.EnhanceModel = "", ""
		}
	}
	return out
}
