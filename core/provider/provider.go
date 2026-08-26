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

// DefaultProviderID is the provider used when the user has not chosen one
// (generation spec: 首次生成默认路由到 Doubao).
const DefaultProviderID = ProviderDoubao

// ErrUnsupported reports that a provider does not support the requested
// modality (e.g. gpt-image-2 has no text generation).
var ErrUnsupported = errors.New("provider: modality not supported by this provider")

// ErrNoAPIKey reports that neither a configured key nor the environment
// fallback is available.
var ErrNoAPIKey = errors.New("provider: no API key configured (set it in settings or the OFRAME_*_API_KEY environment variable)")

// Capabilities describes which modalities a provider supports.
type Capabilities struct {
	Image bool `json:"image"`
	Text  bool `json:"text"`
}

// ReferenceImage is an image attached to a generation request (外发素材).
type ReferenceImage struct {
	Kind string `json:"kind"` // reference_image | sprite
	Role string `json:"role"` // main_reference | auxiliary_reference
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

// TextRequest is a text generation request (used by the optional AI-assisted
// consistency score later and by Doubao's chat capability).
type TextRequest struct {
	Prompt string
	Model  string // empty → provider default
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
