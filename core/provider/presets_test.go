package provider

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// wantPresetOrder locks the fixed order of the quick presets (align-framebaker-
// providers task 1.3 + 人工验收反馈：七项 FrameBaker 预设 + Agnes 专项)：
// OpenAI、百炼、banana/Gemini、MiniMax、火山方舟/豆包、自定义 CLI、自定义 API、Agnes.
var wantPresetOrder = []string{
	PresetKeyOpenAI,
	PresetKeyDashscope,
	PresetKeyGemini,
	PresetKeyMiniMax,
	PresetKeyVolcengine,
	PresetKeyCLI,
	PresetKeyCustomAPI,
	PresetKeyAgnes,
}

// wantPresets is the locked content snapshot of every preset description. It
// pins keys, display names, protocol types, default endpoints, capability
// flags and model catalogs; a deliberate preset change requires updating this
// table consciously instead of drifting silently.
//
// Endpoints and 豆包/OpenAI models deliberately reference the existing config.go
// constants so the snapshots and DefaultConfig share one source of truth.
var wantPresets = map[string]Preset{
	PresetKeyOpenAI: {
		Key:         PresetKeyOpenAI,
		Name:        "OpenAI",
		Description: "官方 OpenAI Images 与 Chat Completions 协议；填写 API Key 后即可用于图像与文本生成。模型需手动输入或获取模型后点选。",
		Type:        ProviderOpenAI,
		BaseURL:     DefaultOpenAIBaseURL, // https://api.openai.com/v1
		Capabilities: Capabilities{
			Image: true,
			Text:  true,
		},
		// 模型目录留空（人工验收反馈：不预填模型）—— 手动输入或获取模型后点选。
	},
	PresetKeyDashscope: {
		Key:         PresetKeyDashscope,
		Name:        "百炼（DashScope）",
		Description: "阿里云百炼原生协议：通义万相 Text2Image 生成图像、Qwen 文本生成；视频目录预留（管线接入前不可调用）。模型需手动输入或获取模型后点选。",
		Type:        ProviderTypeDashscope,
		BaseURL:     DefaultDashscopeBaseURL, // https://dashscope.aliyuncs.com/api/v1
		Capabilities: Capabilities{
			Image: true,
			Text:  true,
		},
	},
	PresetKeyGemini: {
		Key:         PresetKeyGemini,
		Name:        "banana / Gemini",
		Description: "Google Gemini generateContent 协议（nano banana）：inlineData 支持参考图输入，图像与文本共用同一密钥。模型需手动输入或获取模型后点选。",
		Type:        ProviderTypeGemini,
		BaseURL:     DefaultGeminiBaseURL, // https://generativelanguage.googleapis.com/v1beta
		Capabilities: Capabilities{
			Image: true,
			Text:  true,
		},
	},
	PresetKeyMiniMax: {
		Key:         PresetKeyMiniMax,
		Name:        "MiniMax",
		Description: "MiniMax 图片生成协议；仅声明图片能力（视频能力接入前不开放调用，保留视频模型配置位）。模型需手动输入或获取模型后点选。",
		Type:        ProviderTypeMiniMax,
		BaseURL:     DefaultMiniMaxBaseURL, // https://api.minimax.chat/v1
		Capabilities: Capabilities{
			Image: true,
		},
	},
	PresetKeyVolcengine: {
		Key:         PresetKeyVolcengine,
		Name:        "火山方舟 / 豆包",
		Description: "火山方舟 Ark 原生协议（豆包）：Seedream 图像、Doubao 文本；视频目录预留（管线接入前不可调用）。模型需手动输入或获取模型后点选。",
		Type:        ProviderTypeVolcengine,
		BaseURL:     DefaultDoubaoBaseURL, // https://ark.cn-beijing.volces.com/api/v3
		Capabilities: Capabilities{
			Image: true,
			Text:  true,
		},
	},
	PresetKeyCLI: {
		Key:         PresetKeyCLI,
		Name:        "自定义 CLI",
		Description: "自定义命令行工具：命令与参数按 argv 数组执行（不经 shell），输出文件由应用校验；无固定接口地址，模型目录按所用工具自行填写。",
		Type:        ProviderTypeCLI,
		BaseURL:     "", // CLI 无 HTTP 端点
		Capabilities: Capabilities{
			Image: true,
		},
		// 模型目录留空：CLI 工具由用户完全定义。
		// 参数旗标为可编辑的常见默认值；命令留空由用户填写（任务 3.1）。
		CLIDraft: ProviderConfig{
			Type:           ProviderTypeCLI,
			CLIPromptArg:   "--prompt",
			CLIOutputArg:   "--output",
			CLIModelArg:    "--model",
			CLIRefImageArg: "--ref",
		},
	},
	PresetKeyCustomAPI: {
		Key:         PresetKeyCustomAPI,
		Name:        "自定义 API",
		Description: "自定义 OpenAI 兼容接口：填入 Base URL 与密钥后测试连接、获取模型并归类到图片/文本目录；默认不预置地址与模型。",
		Type:        ProviderTypeAPI,
		BaseURL:     "", // 用户填写后保存时严格校验
		Capabilities: Capabilities{
			Image: true,
			Text:  true,
		},
		// 模型目录留空：默认草稿不预置模型。
	},
	PresetKeyAgnes: {
		Key:         PresetKeyAgnes,
		Name:        "Agnes（免费多模态）",
		Description: "Agnes AI 免费多模态网关（OpenAI 兼容）：图像走 /images/generations，文本走 /chat/completions；视频目录预留，管线接入前不可调用。填入 API Key 后即可测试连接与获取模型。",
		Type:        ProviderAgnes,
		BaseURL:     DefaultAgnesBaseURL, // https://apihub.agnes-ai.com/v1（真实网关）
		Capabilities: Capabilities{
			Image: true,
			Text:  true,
		},
		// 模型目录留空（人工验收反馈：不预填模型）。
	},
}

func TestProviderPresetsOrderAndKeys(t *testing.T) {
	ps := Presets()
	if len(ps) != len(wantPresetOrder) {
		t.Fatalf("len(Presets()) = %d, want %d", len(ps), len(wantPresetOrder))
	}
	for i, want := range wantPresetOrder {
		if ps[i].Key != want {
			t.Fatalf("presets[%d].Key = %q, want %q", i, ps[i].Key, want)
		}
		if ps[i].Type != ps[i].Key {
			t.Fatalf("preset %q: Type = %q, want it to equal the Key", ps[i].Key, ps[i].Type)
		}
		if strings.TrimSpace(ps[i].Name) == "" || strings.TrimSpace(ps[i].Description) == "" {
			t.Fatalf("preset %q: display name and description must be non-empty", ps[i].Key)
		}
	}
}

// TestProviderPresetsSnapshot locks every field of every preset against the
// expected default descriptions (类型 / 默认地址 / 模型分类).
func TestProviderPresetsSnapshot(t *testing.T) {
	ps := Presets()
	for i, got := range ps {
		want, ok := wantPresets[got.Key]
		if !ok {
			t.Fatalf("presets[%d]: unexpected preset key %q", i, got.Key)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("presets[%d] (%s):\n got %#v\nwant %#v", i, got.Key, got, want)
		}
	}
}

func TestPresetByKey(t *testing.T) {
	for _, key := range wantPresetOrder {
		got, err := PresetByKey(key)
		if err != nil {
			t.Fatalf("PresetByKey(%q): %v", key, err)
		}
		if !reflect.DeepEqual(got, wantPresets[key]) {
			t.Fatalf("PresetByKey(%q):\n got %#v\nwant %#v", key, got, wantPresets[key])
		}
	}
	// Unknown keys fail loudly instead of returning a silently empty preset.
	if got, err := PresetByKey("nope"); err == nil {
		t.Fatalf("PresetByKey(\"nope\") = %+v, want an error", got)
	} else if !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("error should mention the unknown key, got %v", err)
	}
}

// TestProviderPresetsCapabilityInvariants checks that model classifications
// stay consistent with declared capabilities (模型分类非空且与协议能力一致),
// including the core truthfulness rule: 视频模型可作为预配置但不能宣称已可执行.
func TestProviderPresetsCapabilityInvariants(t *testing.T) {
	ps := Presets()

	wantTextCaps := map[string]bool{
		PresetKeyOpenAI:     true,
		PresetKeyDashscope:  true,
		PresetKeyGemini:     true,
		PresetKeyMiniMax:    false,
		PresetKeyVolcengine: true,
		PresetKeyCLI:        false,
		PresetKeyCustomAPI:  true,
		PresetKeyAgnes:      true, // 多模态（人工验收反馈）
	}

	// 人工验收反馈：所有预设的模型目录默认留空 —— 手动输入或获取模型后点选。
	for _, p := range ps {
		// No preset may claim executable video while no adapter implements it.
		if p.Capabilities.Video {
			t.Errorf("preset %q: Capabilities.Video must stay false before a video adapter exists (不能宣称已可执行)", p.Key)
		}
		if p.Capabilities.Image != true {
			t.Errorf("preset %q: image capability mismatch, got %v, want true", p.Key, p.Capabilities.Image)
		}
		if p.Capabilities.Text != wantTextCaps[p.Key] {
			t.Errorf("preset %q: text capability = %v, want %v", p.Key, p.Capabilities.Text, wantTextCaps[p.Key])
		}

		// All catalogs ship EMPTY on every preset (不预填模型).
		if p.ImageModels != nil || p.VideoModels != nil || p.TextModels != nil {
			t.Errorf("preset %q: catalogs must ship empty, got %v/%v/%v", p.Key, p.ImageModels, p.VideoModels, p.TextModels)
		}

		// Endpoint rules: custom CLI carries no HTTP BaseURL and 自定义 API
		// ships an empty one until the user fills it in; every preset that
		// does carry a default must be a real https endpoint.
		if p.BaseURL != "" || (p.Key != PresetKeyCLI && p.Key != PresetKeyCustomAPI) {
			u, err := url.Parse(p.BaseURL)
			if err != nil || u.Scheme != "https" || u.Host == "" {
				t.Errorf("preset %q: BaseURL %q is not a valid https endpoint", p.Key, p.BaseURL)
			}
		}
	}
}

// checkModelList rejects blank or whitespace-padded entries and duplicates:
// preset catalogs follow the same normalization contract as stored configs.
func checkModelList(t *testing.T, preset, kind string, list []string) {
	t.Helper()
	seen := map[string]struct{}{}
	for i, m := range list {
		s := strings.TrimSpace(m)
		if s == "" {
			t.Errorf("preset %q: %s catalog entry %d is blank", preset, kind, i)
			continue
		}
		if s != m {
			t.Errorf("preset %q: %s catalog entry %d is padded: %q", preset, kind, i, m)
		}
		if _, dup := seen[s]; dup {
			t.Errorf("preset %q: %s catalog has duplicate entry %q", preset, kind, s)
		}
		seen[s] = struct{}{}
	}
}

// TestProviderPresetsReturnIndependentCopies guarantees callers cannot mutate
// the global default snapshot through any returned value (返回副本).
func TestProviderPresetsReturnIndependentCopies(t *testing.T) {
	first := Presets()
	// Mutate every mutable surface of the first copy. Catalogs ship empty, so
	// the append path is exercised too.
	first[0].Key = "tampered-key"
	first[0].Type = "tampered-type"
	first[0].Capabilities.Image = false
	first[0].ImageModels = append(first[0].ImageModels, "hijacked-image-model")
	first[0].VideoModels = append(first[0].VideoModels, "extra-video")
	first[0].TextModels = append(first[0].TextModels, "hijacked-text-model")

	again := Presets()
	if len(again) != len(wantPresetOrder) {
		t.Fatalf("len(Presets()) after tampering = %d, want %d", len(again), len(wantPresetOrder))
	}
	for i, got := range again {
		want, ok := wantPresets[got.Key]
		if !ok {
			t.Fatalf("presets[%d]: unexpected preset key %q after tampering", i, got.Key)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("global snapshot leaked mutations at %s:\n got %#v\nwant %#v", got.Key, got, want)
		}
	}

	// Lookup copies are equally isolated, across all three catalogs.
	one, err := PresetByKey(PresetKeyVolcengine)
	if err != nil {
		t.Fatal(err)
	}
	one.BaseURL = "https://mutated.example.com"
	one.ImageModels = append(one.ImageModels, "m1")
	one.VideoModels = append(one.VideoModels, "m2")
	one.TextModels = append(one.TextModels, "m3")
	fresh, err := PresetByKey(PresetKeyVolcengine)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fresh, wantPresets[PresetKeyVolcengine]) {
		t.Fatalf("PresetByKey result shares state with the global table:\n got %#v\nwant %#v", fresh, wantPresets[PresetKeyVolcengine])
	}

	// Two successive list reads never alias each other either (append on the
	// empty catalog must not leak into the other read's backing array).
	a, b := Presets(), Presets()
	a[4].VideoModels = append(a[4].VideoModels, "overwritten")
	if len(b[4].VideoModels) != 0 {
		t.Fatalf("two Presets() calls share a backing array: %v vs %v", a[4].VideoModels, b[4].VideoModels)
	}
}
