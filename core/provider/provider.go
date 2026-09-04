package provider

import (
	"context"
	"errors"
	"fmt"
)

// Provider identifiers. Doubao is the default primary provider.
const (
	ProviderDoubao = "doubao"
	ProviderOpenAI = "openai" // gpt-image-2
	ProviderAgnes  = "agnes"
)

// ProviderTypeCompatible is the adapter type for user-defined OpenAI-compatible
// providers (any custom Base URL / model / key). Built-in providers use their
// own id as the type; the compatible type powers the settings presets.
const ProviderTypeCompatible = "compatible"

// API protocol variants supported by the custom API adapter.
const (
	APIProtocolCompletions = "openai-completions"
	APIProtocolResponses   = "openai-responses"
	APIProtocolAnthropic   = "anthropic-messages"
)

// Protocol adapter types for the FrameBaker presets (design D1: explicit
// protocol discriminator — every preset maps to exactly one protocol so a
// vendor's request shape is never silently replaced by another vendor's).
// Config/capability metadata accept them from now on; the concrete protocol
// adapters are wired into NewAdapter in the adapter tasks.
const (
	ProviderTypeCLI        = "cli"        // 自定义 CLI provider（argv 执行，不用 shell）
	ProviderTypeAPI        = "api"        // 自定义通用 API provider
	ProviderTypeDashscope  = "dashscope"  // 百炼 DashScope 原生协议
	ProviderTypeGemini     = "gemini"     // banana / Gemini generateContent 协议
	ProviderTypeMiniMax    = "minimax"    // MiniMax 图片 API 协议
	ProviderTypeVolcengine = "volcengine" // 火山方舟/豆包 Ark 原生协议
)

// DefaultProviderID is the provider used when the user has not chosen one
// (generation spec: 首次生成默认路由到 Doubao).
const DefaultProviderID = ProviderDoubao

// IsBuiltin reports whether id is one of the three built-in providers. The
// built-ins are protected from removal in the settings UI; custom providers
// are user-defined and freely removable.
func IsBuiltin(id string) bool {
	switch id {
	case ProviderDoubao, ProviderOpenAI, ProviderAgnes:
		return true
	}
	return false
}

// ErrUnsupported reports that a provider does not support the requested
// modality (e.g. gpt-image-2 has no text generation).
var ErrUnsupported = errors.New("provider: modality not supported by this provider")

// ErrNoAPIKey reports that neither a configured key nor the environment
// fallback is available.
var ErrNoAPIKey = errors.New("provider: no API key configured (set it in settings or the OFRAME_*_API_KEY environment variable)")

// Capabilities describes which modalities a provider supports. Video is
// reserved metadata for the future video generation/filmstrip-extraction
// pipeline: while no adapter implements video calls, no adapter may report
// Video (capability declarations stay truthful before any external call).
type Capabilities struct {
	Image bool `json:"image"`
	Video bool `json:"video"`
	Text  bool `json:"text"`
}

// ReferenceImage kinds. reference_image/sprite are the identity package's
// reference materials; base_sprite is the adopted base character sprite sent
// as the canonical identity reference (对齐 perfectpixel：身份图外发).
const (
	RefKindReferenceImage = "reference_image"
	RefKindSprite         = "sprite"
	RefKindBaseSprite     = "base_sprite"
)

// ReferenceImage is an image attached to a generation request (外发素材).
type ReferenceImage struct {
	Kind string `json:"kind"` // reference_image | sprite | base_sprite
	Role string `json:"role"` // main_reference | auxiliary_reference | base_sprite
	MIME string `json:"mime"`
	Data []byte `json:"-"`
}

// ImageRequest is a text-to-image generation request.
type ImageRequest struct {
	Prompt     string
	Model      string // empty → provider default
	Width      int    // generation resolution (the filmstrip pipeline maps canvas → strip later)
	Height     int
	References []ReferenceImage
}

// ImageResult is the raw generated image (PNG bytes in phase 3; slicing and
// correction belong to the filmstrip pipeline, tasks 5.x).
type ImageResult struct {
	Data     []byte
	MIME     string
	Provider string
	Model    string
}

// TextRequest is a text generation request (prompt enhancement, AI image
// captioning 识图生成描述, and chat-capable providers).
type TextRequest struct {
	Prompt string
	Model  string // empty → provider default
	// ImageDataURL optionally attaches one image (data URL, e.g.
	// "data:image/png;base64,...") for vision-capable text models. Empty =
	// plain text call; providers without vision support will fail the call.
	ImageDataURL string
}

// TextResult is a text generation result.
type TextResult struct {
	Text     string
	Provider string
	Model    string
}

// Provider is the unified generation provider interface (generation spec 4.1:
// 文本/图像生成统一接口, runtime switchable adapters).
type Provider interface {
	ID() string
	// Name is the human-readable display name (e.g. 豆包 / Doubao).
	Name() string
	Capabilities() Capabilities
	DefaultImageModel() string
	DefaultTextModel() string
	GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error)
	GenerateText(ctx context.Context, req TextRequest) (*TextResult, error)
}

// ResolveModel picks req.Model, falling back to the provider default.
func ResolveModel(reqModel, fallback string) string {
	if reqModel != "" {
		return reqModel
	}
	return fallback
}

// notRetryable wraps an error so the retry executor stops immediately.
type notRetryableError struct{ err error }

func (e *notRetryableError) Error() string { return e.err.Error() }
func (e *notRetryableError) Unwrap() error { return e.err }

// MarkNotRetryable wraps err so retry logic treats it as final.
func MarkNotRetryable(err error) error {
	if err == nil {
		return nil
	}
	return &notRetryableError{err: err}
}

// IsNotRetryable reports whether err (or its chain) was marked final.
func IsNotRetryable(err error) bool {
	var n *notRetryableError
	return errors.As(err, &n)
}

// ConfigError is a configuration/validation error.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return fmt.Sprintf("provider: %s", e.Msg) }

func configErrf(format string, args ...any) error {
	return &ConfigError{Msg: fmt.Sprintf(format, args...)}
}
