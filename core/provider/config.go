package provider

import (
	"context"
	"net/url"
	"os"
	"strings"
	"time"
)

// Default generation settings (阶段 3 生成确认: 每方向最多 3 次总尝试).
const (
	// DefaultMaxAttemptsPerDirection is the hard cap agreed in the generation
	// confirmation: every generated direction may be attempted at most 3 times
	// in total (1 initial + 2 retries).
	DefaultMaxAttemptsPerDirection = 3
	// DefaultTimeout is the per-call HTTP timeout. Image generation on free
	// gateways routinely takes 1-3 minutes (queueing + diffusion), so the
	// hang-protection bound is generous; every retry attempt gets a fresh
	// window (adapters apply it per attempt via applyTimeout).
	DefaultTimeout = 300 * time.Second
	// DefaultGenerationSize is the generation resolution used for image calls
	// (image APIs require large square sizes; the filmstrip pipeline later
	// maps the logical canvas onto the generated strip).
	DefaultGenerationSize = 1024
)

// Default endpoint and model values. Exact vendor defaults are confirmed
// during live adapter integration (design Open Questions); the adapter
// contract is fully testable with a fake transport regardless.
const (
	DefaultDoubaoBaseURL    = "https://ark.cn-beijing.volces.com/api/v3"
	DefaultDoubaoModel      = "doubao-seedream-4-0"
	DefaultDoubaoVideoModel = "doubao-seedance-1-0-pro" // video 预留：视频管线接入前仅作目录标注
	DefaultDoubaoTextModel  = "doubao-1-5-pro-32k"

	DefaultOpenAIBaseURL = "https://api.openai.com/v1"
	DefaultOpenAIModel   = "gpt-image-2"

	// Agnes AI 真实网关（快速接入文档：OpenAI 兼容面，Bearer 认证）。
	// 免费多模态：图像走 /images/generations，文本走 /chat/completions。
	// 默认模型取官方文档最新 ID（Agnes 2.5 Flash / Agnes Image 2.1 Flash）；
	// 以「获取模型」返回的列表为准。
	DefaultAgnesBaseURL = "https://apihub.agnes-ai.com/v1"
	DefaultAgnesModel   = "agnes-image-21-flash"
	// DefaultAgnesTextModel per the Agnes docs (Agnes 2.5 Flash).
	DefaultAgnesTextModel = "agnes-2.5-flash"
	// DefaultAgnesVideoModel is the reserved video catalog entry (全模态方向,
	// 人工验收反馈): official naming per the user — still metadata only, the
	// global video gate keeps it non-callable until the video pipeline lands.
	DefaultAgnesVideoModel = "agnes-video-2.5-flash"
)

// Environment variable fallbacks for API keys (key 本地管理，可经环境变量注入).
const (
	EnvKeyDoubao = "OFRAME_DOUBAO_API_KEY"
	EnvKeyOpenAI = "OFRAME_OPENAI_API_KEY"
	EnvKeyAgnes  = "OFRAME_AGNES_API_KEY"
)

func envKeyFor(id string) string {
	switch id {
	case ProviderDoubao:
		return EnvKeyDoubao
	case ProviderOpenAI:
		return EnvKeyOpenAI
	case ProviderAgnes:
		return EnvKeyAgnes
	}
	return "OFRAME_" + strings.ToUpper(id) + "_API_KEY"
}

// Default price estimates per call (预算估算用, 非计费): Doubao/Agnes 以 CNY
// 计, OpenAI 以 USD 计。可在 ProviderConfig.PricePerCall 覆盖。
const (
	DefaultPriceDoubao = 0.05
	DefaultPriceOpenAI = 0.10
	DefaultPriceAgnes  = 0.06
)

// Currency returns the estimate currency of a provider.
func Currency(id string) string {
	if id == ProviderOpenAI {
		return "USD"
	}
	return "CNY"
}

// ProviderConfig is the local configuration of one provider adapter: key,
// model, endpoint, retry cap and cost estimate (generation spec 4.6: 密钥本地
// 管理; 模型/密钥配置与验证).
//
// Type discriminates the adapter implementation: the three built-in ids
// ("doubao" / "openai" / "agnes") use their own type and preserve their
// vendor-specific behavior; user-defined providers use Type "compatible"
// (OpenAI-compatible images/generations + chat/completions). Name is the
// user-editable display name of custom providers (built-ins keep their
// adapter's hard-coded name). The protocol preset types (cli / api /
// dashscope / gemini / minimax / volcengine) are accepted values for future
// protocol adapters.
//
// Model catalogs are stored per capability: ImageModels/VideoModels/
// TextModels are the persisted lists (design D2); the singular Model /
// VideoModel / TextModel fields remain as the legacy compatibility fallback
// (and the legacy "effective default" used by existing call sites).
type ProviderConfig struct {
	ProviderID  string   `json:"providerId"`
	Type        string   `json:"type,omitempty"` // "" → built-in id or "compatible"
	Name        string   `json:"name,omitempty"` // display name (custom providers)
	APIKey      string   `json:"apiKey,omitempty"`
	Model       string   `json:"model,omitempty"`       // legacy image model field (fallback for ImageModels)
	VideoModel  string   `json:"videoModel,omitempty"`  // legacy video model field (fallback for VideoModels)
	TextModel   string   `json:"textModel,omitempty"`   // legacy text model field (fallback for TextModels)
	ImageModels []string `json:"imageModels,omitempty"` // image model catalog (preferred over Model)
	VideoModels []string `json:"videoModels,omitempty"` // video model catalog (reserved capability)
	TextModels  []string `json:"textModels,omitempty"`  // text model catalog (preferred over TextModel)
	BaseURL     string   `json:"baseUrl,omitempty"`
	// APIProtocol selects the text API used by custom HTTP providers. Empty and
	// APIProtocolCompletions use /chat/completions; APIProtocolResponses uses
	// /responses; APIProtocolAnthropic uses /messages.
	APIProtocol string `json:"apiProtocol,omitempty"`
	// DefaultSize is the advisory generation size ("WxH", e.g. "1024x1024").
	// It is card metadata for the settings UI (task 1.1/5.2); adapters keep
	// using the request's explicit Width/Height.
	DefaultSize  string  `json:"defaultSize,omitempty"`
	MaxAttempts  int     `json:"maxAttempts,omitempty"`  // 0 → DefaultMaxAttemptsPerDirection
	TimeoutSec   int     `json:"timeoutSec,omitempty"`   // 0 → 300
	PricePerCall float64 `json:"pricePerCall,omitempty"` // 0 → provider default estimate

	// CLI fields are used only to build an exec argv array (task 3.2); user
	// values are never interpolated into a shell command string.
	CLICommand     string   `json:"cliCommand,omitempty"`     // executable path
	CLIPromptArg   string   `json:"cliPromptArg,omitempty"`   // prompt flag, e.g. --prompt
	CLIOutputArg   string   `json:"cliOutputArg,omitempty"`   // output-file flag, e.g. --output
	CLIModelArg    string   `json:"cliModelArg,omitempty"`    // model flag, e.g. --model
	CLIRefImageArg string   `json:"cliRefImageArg,omitempty"` // reference-image flag, repeated per image
	CLIExtraArgs   []string `json:"cliExtraArgs,omitempty"`   // fixed arguments passed in order
	// CLITemplate is a legacy read-only compatibility field. Migration semantics
	// are handled by task 3.4; execution must not use this template string.
	CLITemplate string `json:"cliTemplate,omitempty"`
}

// DefaultConfig returns the built-in defaults for a provider id. Besides the
// legacy singular model fields it fills the per-capability default catalogs
// (task 1.1: Doubao image+video+text, OpenAI image, Agnes image); unknown ids
// stay uncategorized. The singular EffectiveModel/EffectiveTextModel behavior
// is unchanged.
func DefaultConfig(id string) ProviderConfig {
	c := ProviderConfig{ProviderID: id}
	switch id {
	case ProviderDoubao:
		c.Type = ProviderDoubao
		c.BaseURL = DefaultDoubaoBaseURL
		c.Model = DefaultDoubaoModel
		c.TextModel = DefaultDoubaoTextModel
		c.ImageModels = []string{DefaultDoubaoModel}
		c.VideoModels = []string{DefaultDoubaoVideoModel} // 预留：视频能力未接入
		c.TextModels = []string{DefaultDoubaoTextModel}
	case ProviderOpenAI:
		c.Type = ProviderOpenAI
		c.BaseURL = DefaultOpenAIBaseURL
		c.Model = DefaultOpenAIModel
		c.ImageModels = []string{DefaultOpenAIModel}
	case ProviderAgnes:
		c.Type = ProviderAgnes
		c.BaseURL = DefaultAgnesBaseURL
		c.Model = DefaultAgnesModel
		c.ImageModels = []string{DefaultAgnesModel}
		// 多模态（人工验收反馈）：文本目录与图像并存，端点接入前不可真实调用。
		c.TextModel = DefaultAgnesTextModel
		c.TextModels = []string{DefaultAgnesTextModel}
		// 全模态方向：视频目录仅作预留元数据（能力声明仍为 false）。
		c.VideoModels = []string{DefaultAgnesVideoModel}
	default:
		c.Type = ProviderTypeCompatible
	}
	return c
}

// EffectiveType returns the adapter type: the explicit Type, or the built-in
// id when it is one of the three built-ins, or "compatible" otherwise.
func (c ProviderConfig) EffectiveType() string {
	if c.Type != "" {
		return c.Type
	}
	return DefaultConfig(c.ProviderID).Type
}

// EffectiveName returns the display name of a custom provider (empty → the
// provider id), used by the compatible adapter's Name().
func (c ProviderConfig) EffectiveName() string {
	if strings.TrimSpace(c.Name) != "" {
		return strings.TrimSpace(c.Name)
	}
	return c.ProviderID
}

// EffectiveModel returns the configured image model or the built-in default.
func (c ProviderConfig) EffectiveModel() string {
	if c.Model != "" {
		return c.Model
	}
	return DefaultConfig(c.ProviderID).Model
}

// EffectiveTextModel returns the configured text model or the built-in default.
func (c ProviderConfig) EffectiveTextModel() string {
	if c.TextModel != "" {
		return c.TextModel
	}
	return DefaultConfig(c.ProviderID).TextModel
}

// normalizeModelList trims whitespace around entries, drops blank entries and
// removes duplicates (first occurrence wins, order preserved). An empty result
// comes back as nil so callers treat it as "not configured".
func normalizeModelList(items []string) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, raw := range items {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// EffectiveImageModels returns the image models of a provider: the persisted
// imageModels catalog when it holds any non-blank entry; otherwise the legacy
// singular field (or built-in default — the same value EffectiveModel has
// always returned) wrapped as a one-entry list. The result is always freshly
// allocated and normalized (去空白、去重).
func (c ProviderConfig) EffectiveImageModels() []string {
	if m := normalizeModelList(c.ImageModels); len(m) > 0 {
		return m
	}
	if s := strings.TrimSpace(c.EffectiveModel()); s != "" {
		return []string{s}
	}
	return nil
}

// EffectiveVideoModels returns the video models of a provider: the persisted
// videoModels catalog when set, otherwise the legacy singular VideoModel
// field. There is no built-in video default until video support lands;
// unset configurations yield nil (视频目录为预留，不代表可调用).
func (c ProviderConfig) EffectiveVideoModels() []string {
	if m := normalizeModelList(c.VideoModels); len(m) > 0 {
		return m
	}
	if s := strings.TrimSpace(c.VideoModel); s != "" {
		return []string{s}
	}
	return nil
}

// EffectiveTextModels returns the text models of a provider: the persisted
// textModels catalog when it holds any non-blank entry; otherwise the legacy
// singular field (or built-in default — matching EffectiveTextModel) wrapped
// as a one-entry list.
func (c ProviderConfig) EffectiveTextModels() []string {
	if m := normalizeModelList(c.TextModels); len(m) > 0 {
		return m
	}
	if s := strings.TrimSpace(c.EffectiveTextModel()); s != "" {
		return []string{s}
	}
	return nil
}

// EffectiveBaseURL returns the configured endpoint or the built-in default.
func (c ProviderConfig) EffectiveBaseURL() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return strings.TrimRight(DefaultConfig(c.ProviderID).BaseURL, "/")
}

// EffectiveMaxAttempts returns the retry cap (每方向最多 3 次总尝试 by default).
func (c ProviderConfig) EffectiveMaxAttempts() int {
	if c.MaxAttempts > 0 {
		return c.MaxAttempts
	}
	return DefaultMaxAttemptsPerDirection
}

// EffectiveTimeout returns the per-call HTTP timeout.
func (c ProviderConfig) EffectiveTimeout() time.Duration {
	if c.TimeoutSec > 0 {
		return time.Duration(c.TimeoutSec) * time.Second
	}
	return DefaultTimeout
}

// applyTimeout bounds one external provider call by d (task 2.1: the adapter
// layer enforces the configured timeout, so a caller passing context.Background
// is still bounded). It nests safely under any caller deadline already present:
// the child deadline never extends past the parent's. The returned cancel must
// be called when the call finishes.
func applyTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

// EffectivePrice returns the per-call cost estimate used for the budget.
// Custom compatible providers default to the Doubao estimate (CNY) so the
// budget confirmation still shows a sensible number; users may override it.
func (c ProviderConfig) EffectivePrice() float64 {
	if c.PricePerCall > 0 {
		return c.PricePerCall
	}
	switch c.ProviderID {
	case ProviderDoubao:
		return DefaultPriceDoubao
	case ProviderOpenAI:
		return DefaultPriceOpenAI
	case ProviderAgnes:
		return DefaultPriceAgnes
	}
	return DefaultPriceDoubao
}

// ResolveAPIKey returns the configured key, falling back to the environment
// variable OFRAME_<ID>_API_KEY. ErrNoAPIKey when neither is set.
func (c ProviderConfig) ResolveAPIKey() (string, error) {
	if strings.TrimSpace(c.APIKey) != "" {
		return strings.TrimSpace(c.APIKey), nil
	}
	if v := strings.TrimSpace(os.Getenv(envKeyFor(c.ProviderID))); v != "" {
		return v, nil
	}
	return "", ErrNoAPIKey
}

// PlaceholderAgnesHost is the LEGACY placeholder host from before the real
// Agnes AI integration (apihub.agnes-ai.com). Configs still carrying it are
// migrated by NormalizeSettings; the preflight below answers late reads with
// an actionable message instead of a raw DNS failure.
const PlaceholderAgnesHost = "api.agnes.local"

// CheckPlaceholderEndpoint returns a readable, not-retryable error when the
// config still points at the LEGACY Agnes placeholder endpoint (zero network
// calls): the message tells the user the new URL. nil for real endpoints.
func (c ProviderConfig) CheckPlaceholderEndpoint() error {
	if c.EffectiveType() != ProviderAgnes {
		return nil
	}
	u, err := url.Parse(c.EffectiveBaseURL())
	if err == nil && strings.EqualFold(u.Hostname(), PlaceholderAgnesHost) {
		return MarkNotRetryable(configErrf(
			"provider %s: 该 Base URL 是旧版 Agnes 占位地址（%s）— 请改为 https://apihub.agnes-ai.com/v1 后保存重试（重启应用也会自动迁移）",
			c.ProviderID, u.Host))
	}
	return nil
}

// KnownModels lists the built-in default models per provider (image, text).
func KnownModels(id string) (image, text string) {
	d := DefaultConfig(id)
	return d.Model, d.TextModel
}

// validSlug reports whether s is a safe provider id: 1-40 chars of lowercase
// letters, digits and hyphens (user-defined custom provider ids).
func validSlug(s string) bool {
	if len(s) == 0 || len(s) > 40 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return true
}

// Validate checks the local configuration without any network call (模型/密钥
// 配置与验证): provider id known (built-in or valid slug), key present (direct
// or via env), model non-empty, endpoint parseable, retry cap within a sane
// bound. It is the strict check used when saving a provider config.
func (c ProviderConfig) Validate() error {
	if err := c.ValidateForAdd(); err != nil {
		return err
	}
	if _, err := c.ResolveAPIKey(); err != nil {
		return err
	}
	if strings.TrimSpace(c.EffectiveModel()) == "" && len(c.EffectiveImageModels()) == 0 {
		// Either the legacy singular field or the image catalog must name at
		// least one image model (task 5.2: 目录化保存 — a provider whose user
		// only filled imageModels stays valid).
		return configErrf("provider %s: image model is required", c.ProviderID)
	}
	// CLI providers run a local executable through argv and have no HTTP
	// endpoint, so the URL check only applies to API protocols.
	if c.EffectiveType() != ProviderTypeCLI {
		if u, err := url.Parse(c.EffectiveBaseURL()); err != nil || u.Scheme == "" || u.Host == "" {
			return configErrf("provider %s: invalid base url %q", c.ProviderID, c.EffectiveBaseURL())
		}
	}
	if c.MaxAttempts < 0 || c.EffectiveMaxAttempts() > 10 {
		return configErrf("provider %s: max attempts %d out of range [1,10]", c.ProviderID, c.EffectiveMaxAttempts())
	}
	return nil
}

// validCustomType reports whether t is a protocol type a user-defined custom
// provider may use. Every entry must have a concrete adapter in NewAdapter —
// a stored type without an adapter would make the provider unbuildable after
// a restart.
func validCustomType(t string) bool {
	switch t {
	case ProviderTypeCompatible, ProviderTypeAPI, ProviderTypeDashscope,
		ProviderTypeGemini, ProviderTypeMiniMax, ProviderTypeVolcengine, ProviderTypeCLI:
		return true
	}
	return false
}

// ValidateForAdd checks only the structural parts (id / type / display name)
// without requiring a model, endpoint or API key. It is used when a user adds
// a provider from a preset: the fields are filled in and strictly validated
// when the config is saved. Custom providers may carry any known protocol
// type (task 2.7: 显式协议路由) — the type is never rewritten to compatible.
func (c ProviderConfig) ValidateForAdd() error {
	switch c.ProviderID {
	case ProviderDoubao, ProviderOpenAI, ProviderAgnes:
		if c.Type != "" && c.Type != c.ProviderID {
			return configErrf("provider %q: type %q does not match the built-in type %q", c.ProviderID, c.Type, c.ProviderID)
		}
	default:
		if !validSlug(c.ProviderID) {
			return configErrf("invalid provider id %q (use 1-40 lowercase letters, digits or '-')", c.ProviderID)
		}
		if t := c.EffectiveType(); !validCustomType(t) {
			return configErrf("provider %q: unsupported type %q (custom providers use compatible, api, dashscope, gemini, minimax, volcengine or cli)", c.ProviderID, t)
		}
		if strings.TrimSpace(c.Name) == "" {
			return configErrf("provider %q: a display name is required for custom providers", c.ProviderID)
		}
	}
	return nil
}

// Settings is the persisted local provider configuration (active provider +
// per-provider configs). Keys are stored locally (design D6). The prompt-
// enhancement association (task 5.5) points at an EXISTING provider's text
// model — no independent credentials; an empty EnhanceProviderID means
// "follow the active provider's text model".
type Settings struct {
	ActiveProvider    string                    `json:"activeProvider"`
	Providers         map[string]ProviderConfig `json:"providers"`
	EnhanceProviderID string                    `json:"enhanceProviderId,omitempty"`
	EnhanceModel      string                    `json:"enhanceModel,omitempty"`
}

// DefaultSettings returns the FRESH-INSTALL settings: no provider cards are
// pre-seeded (人工验收反馈：不需要固定显示 3 个内置 Provider). Users add
// providers from the seven FrameBaker presets; every stored provider —
// including the doubao/openai/agnes identities — can be removed. The
// doubao/openai/agnes ADAPTERS remain shipped (NewAdapter builds them), so
// those ids can still be added again after a removal.
func DefaultSettings() Settings {
	return Settings{
		ActiveProvider: "",
		Providers:      map[string]ProviderConfig{},
	}
}

// ConfigFor returns the config of a provider, defaulting missing entries.
func (s Settings) ConfigFor(id string) ProviderConfig {
	if c, ok := s.Providers[id]; ok {
		return c
	}
	return DefaultConfig(id)
}
