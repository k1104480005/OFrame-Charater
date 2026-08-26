package provider

import (
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
	// DefaultTimeout is the per-call HTTP timeout.
	DefaultTimeout = 60 * time.Second
	// DefaultGenerationSize is the generation resolution used for image calls
	// (image APIs require large square sizes; the filmstrip pipeline later
	// maps the logical canvas onto the generated strip).
	DefaultGenerationSize = 1024
)

// Default endpoint and model values. Exact vendor defaults are confirmed
// during live adapter integration (design Open Questions); the adapter
// contract is fully testable with a fake transport regardless.
const (
	DefaultDoubaoBaseURL   = "https://ark.cn-beijing.volces.com/api/v3"
	DefaultDoubaoModel     = "doubao-seedream-4-0"
	DefaultDoubaoTextModel = "doubao-1-5-pro-32k"

	DefaultOpenAIBaseURL = "https://api.openai.com/v1"
	DefaultOpenAIModel   = "gpt-image-2"

	DefaultAgnesBaseURL = "https://api.agnes.local/v1" // 占位端点，P1 接入验证时确认
	DefaultAgnesModel   = "agnes-image-v1"
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
type ProviderConfig struct {
	ProviderID   string  `json:"providerId"`
	APIKey       string  `json:"apiKey,omitempty"`
	Model        string  `json:"model,omitempty"`     // image model
	TextModel    string  `json:"textModel,omitempty"` // text model (Doubao)
	BaseURL      string  `json:"baseUrl,omitempty"`
	MaxAttempts  int     `json:"maxAttempts,omitempty"`  // 0 → DefaultMaxAttemptsPerDirection
	TimeoutSec   int     `json:"timeoutSec,omitempty"`   // 0 → 60
	PricePerCall float64 `json:"pricePerCall,omitempty"` // 0 → provider default estimate
}

// DefaultConfig returns the built-in defaults for a provider id.
func DefaultConfig(id string) ProviderConfig {
	c := ProviderConfig{ProviderID: id}
	switch id {
	case ProviderDoubao:
		c.BaseURL = DefaultDoubaoBaseURL
		c.Model = DefaultDoubaoModel
		c.TextModel = DefaultDoubaoTextModel
	case ProviderOpenAI:
		c.BaseURL = DefaultOpenAIBaseURL
		c.Model = DefaultOpenAIModel
	case ProviderAgnes:
		c.BaseURL = DefaultAgnesBaseURL
		c.Model = DefaultAgnesModel
	}
	return c
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

// EffectivePrice returns the per-call cost estimate used for the budget.
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
	return 0
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

// KnownModels lists the built-in default models per provider (image, text).
func KnownModels(id string) (image, text string) {
	d := DefaultConfig(id)
	return d.Model, d.TextModel
}

// Validate checks the local configuration without any network call (模型/密钥
// 配置与验证): provider id known, key present (direct or via env), model
// non-empty, endpoint parseable, retry cap within a sane bound.
func (c ProviderConfig) Validate() error {
	switch c.ProviderID {
	case ProviderDoubao, ProviderOpenAI, ProviderAgnes:
	default:
		return configErrf("unknown provider %q", c.ProviderID)
	}
	if _, err := c.ResolveAPIKey(); err != nil {
		return err
	}
	if strings.TrimSpace(c.EffectiveModel()) == "" {
		return configErrf("provider %s: image model is required", c.ProviderID)
	}
	if u, err := url.Parse(c.EffectiveBaseURL()); err != nil || u.Scheme == "" || u.Host == "" {
		return configErrf("provider %s: invalid base url %q", c.ProviderID, c.EffectiveBaseURL())
	}
	if c.MaxAttempts < 0 || c.EffectiveMaxAttempts() > 10 {
		return configErrf("provider %s: max attempts %d out of range [1,10]", c.ProviderID, c.EffectiveMaxAttempts())
	}
	return nil
}

// Settings is the persisted local provider configuration (active provider +
// per-provider configs). Keys are stored locally (design D6).
type Settings struct {
	ActiveProvider string                    `json:"activeProvider"`
	Providers      map[string]ProviderConfig `json:"providers"`
}

// DefaultSettings returns the default active provider (Doubao) with the three
// built-in configs.
func DefaultSettings() Settings {
	return Settings{
		ActiveProvider: DefaultProviderID,
		Providers: map[string]ProviderConfig{
			ProviderDoubao: DefaultConfig(ProviderDoubao),
			ProviderOpenAI: DefaultConfig(ProviderOpenAI),
			ProviderAgnes:  DefaultConfig(ProviderAgnes),
		},
	}
}

// ConfigFor returns the config of a provider, defaulting missing entries.
func (s Settings) ConfigFor(id string) ProviderConfig {
	if c, ok := s.Providers[id]; ok {
		return c
	}
	return DefaultConfig(id)
}
