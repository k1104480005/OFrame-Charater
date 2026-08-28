package provider

import (
	"errors"
	"fmt"
	"strings"
)

// Modality is a requested generation capability (image | video | text). It is
// the shared vocabulary between capability declarations (Capabilities), the
// per-capability model catalogs (ProviderConfig) and the validation boundary
// that gates every external call.
type Modality string

// The three supported modalities.
const (
	ModalityImage Modality = "image"
	ModalityVideo Modality = "video"
	ModalityText  Modality = "text"
)

// String returns the canonical modality name.
func (m Modality) String() string { return string(m) }

// Has reports whether caps declare support for the requested modality. An
// unknown modality is never declared.
func (c Capabilities) Has(m Modality) bool {
	switch m {
	case ModalityImage:
		return c.Image
	case ModalityVideo:
		return c.Video
	case ModalityText:
		return c.Text
	}
	return false
}

// Capability/model boundary errors (align-framebaker-providers task 1.4). All
// validation failures wrap exactly one of these sentinels so callers can
// branch with errors.Is:
//
//	errors.Is(err, provider.ErrCapabilityUnsupported) — the provider does not
//	  declare the requested modality at all (e.g. OpenAI text, or video before
//	  any video adapter exists);
//	errors.Is(err, provider.ErrModelNotConfigured) — the modality is declared,
//	  but the provider has no model in its effective catalog for it;
//	errors.Is(err, provider.ErrModelInvalid) — an explicit model was named but
//	  it is not part of the provider's effective catalog for the modality.
//
// Every error message carries the provider id, modality and offending model
// where applicable. Validation never touches the network: it reads only the
// capability declaration and the local model catalog, so mismatched requests
// fail BEFORE any external call.
var (
	ErrCapabilityUnsupported = errors.New("capability not supported by this provider")
	ErrModelNotConfigured    = errors.New("model not configured for this capability")
	ErrModelInvalid          = errors.New("model is not in the provider's capability catalog")
)

func capabilityUnsupportedf(providerID string, m Modality, detail string) error {
	if detail == "" {
		detail = "the adapter does not implement it"
	}
	return fmt.Errorf("provider %s does not support %s generation (%s): %w", providerID, m, detail, ErrCapabilityUnsupported)
}

func modelNotConfiguredf(providerID string, m Modality) error {
	return fmt.Errorf("provider %s: no %s model configured (add a %s model to this provider first): %w", providerID, m, m, ErrModelNotConfigured)
}

func modelInvalidf(providerID string, m Modality, model string, catalog []string) error {
	return fmt.Errorf("provider %s: model %q is not in its %s model catalog [%s]: %w", providerID, model, m, strings.Join(catalog, ", "), ErrModelInvalid)
}

// containsModel reports whether the normalized catalog holds model exactly
// (catalog entries are already whitespace-trimmed by normalizeModelList).
func containsModel(catalog []string, model string) bool {
	for _, entry := range catalog {
		if entry == model {
			return true
		}
	}
	return false
}

// DeclaredCapabilities maps a protocol adapter type to the capabilities its
// CURRENT adapters actually declare (task 1.4: 校验使用能力声明 — the table must
// stay truthful about what executes today, not what a preset markets):
//
//   - doubao: image + text. Video stays FALSE even though the Doubao config
//     presets a Seedance video-model catalog entry — 预留目录不代表可调用, and no
//     video adapter performs external calls yet.
//   - openai (gpt-image-2): image only; refuses text today.
//   - agnes: MULTIMODAL (人工验收反馈：agnes 是多模态模型) — image + text both
//     execute through its OpenAI-compatible surface; video false.
//   - compatible / api (自定义 OpenAI 兼容): image + text endpoints exist in the
//     shared adapter; video is never attempted.
//   - dashscope / gemini / volcengine: image + text declared by their FrameBaker
//     presets (concrete protocol adapters land in tasks 2.x); video false.
//   - minimax / cli: image only.
//
// ok=false for unknown types (callers treat them as fully unsupported).
func DeclaredCapabilities(adapterType string) (Capabilities, bool) {
	switch adapterType {
	case ProviderDoubao:
		return Capabilities{Image: true, Video: false, Text: true}, true
	case ProviderOpenAI:
		return Capabilities{Image: true}, true
	case ProviderAgnes:
		return Capabilities{Image: true, Text: true}, true
	case ProviderTypeCompatible, ProviderTypeAPI:
		return Capabilities{Image: true, Text: true}, true
	case ProviderTypeDashscope, ProviderTypeGemini, ProviderTypeVolcengine:
		return Capabilities{Image: true, Text: true}, true
	case ProviderTypeMiniMax, ProviderTypeCLI:
		return Capabilities{Image: true}, true
	}
	return Capabilities{}, false
}

// DeclaredCapabilities returns the capabilities declared for this config's
// effective adapter type. Config-derived declarations mirror the live adapters
// (guard-tested to match Capabilities() of every registered adapter type), so
// drafts and CLI callers can run the same validation without a registry.
func (c ProviderConfig) DeclaredCapabilities() Capabilities {
	caps, _ := DeclaredCapabilities(c.EffectiveType())
	return caps
}

// EffectiveCatalog returns the normalized effective model catalog of the
// config for the requested modality (EffectiveImageModels /
// EffectiveVideoModels / EffectiveTextModels). Unknown modalities yield nil.
func (c ProviderConfig) EffectiveCatalog(m Modality) []string {
	switch m {
	case ModalityImage:
		return c.EffectiveImageModels()
	case ModalityVideo:
		return c.EffectiveVideoModels()
	case ModalityText:
		return c.EffectiveTextModels()
	}
	return nil
}

// resolveValidatedModel is the single implementation behind ResolveValidatedModel
// and ValidateCapability: it resolves explicit=="" → catalog default and checks
// membership against catalog (both capability declaration and catalog are used;
// the capability check comes FIRST so a reserved video model on a non-video
// adapter reports unsupported-capability, not unknown-model).
func resolveValidatedModel(caps Capabilities, providerID string, m Modality, catalog []string, explicit string) (string, error) {
	if !caps.Has(m) {
		detail := ""
		if len(catalog) > 0 {
			detail = fmt.Sprintf("the %s model catalog is metadata-only until the adapter supports it", m)
		}
		return "", capabilityUnsupportedf(providerID, m, detail)
	}
	model := strings.TrimSpace(explicit)
	if len(catalog) == 0 {
		return "", modelNotConfiguredf(providerID, m)
	}
	if model == "" {
		return catalog[0], nil
	}
	if !containsModel(catalog, model) {
		return "", modelInvalidf(providerID, m, model, catalog)
	}
	return model, nil
}

// ResolveValidatedModel resolves and validates the model for one requested
// modality BEFORE any external call (generation spec: 模型选择与任务能力匹配):
//
//  1. capability check — caps must declare the modality;
//  2. catalog check — cfg's effective catalog for the modality must be
//     non-empty (ErrModelNotConfigured otherwise);
//  3. membership check — a non-empty explicit model must be a member of that
//     catalog (ErrModelInvalid otherwise); empty explicit → catalog default
//     (first entry).
//
// No model substitution beyond the documented default resolution happens:
// callers pass the user's explicit selection through unchanged or receive an
// error. Zero network calls.
func ResolveValidatedModel(caps Capabilities, cfg ProviderConfig, m Modality, explicit string) (string, error) {
	return resolveValidatedModel(caps, cfg.ProviderID, m, cfg.EffectiveCatalog(m), explicit)
}

// ValidateCapability validates without needing the resolved model back:
// ErrCapabilityUnsupported / ErrModelNotConfigured / ErrModelInvalid when the
// request cannot be served, nil when the call may proceed.
func ValidateCapability(caps Capabilities, cfg ProviderConfig, m Modality, explicit string) error {
	_, err := resolveValidatedModel(caps, cfg.ProviderID, m, cfg.EffectiveCatalog(m), explicit)
	return err
}

// ValidateCapability (method form) derives the capabilities from the config's
// own adapter type instead of taking them from a live adapter instance.
func (c ProviderConfig) ValidateCapability(m Modality, explicit string) error {
	return ValidateCapability(c.DeclaredCapabilities(), c, m, explicit)
}

// ResolveValidatedModel (method form) uses config-derived capabilities.
func (c ProviderConfig) ResolveValidatedModel(m Modality, explicit string) (string, error) {
	return resolveValidatedModel(c.DeclaredCapabilities(), c.ProviderID, m, c.EffectiveCatalog(m), explicit)
}

// ValidateVideoGeneration is the explicit video pre-flight entry point (task
// 1.4 / spec generation: 视频生成能力在外部调用前校验). Until a video adapter
// lands (tasks 2.x/6.x) EVERY configuration answers ErrCapabilityUnsupported —
// including Doubao with its preset Seedance video model — and the function is
// purely local: it never issues network requests.
func (c ProviderConfig) ValidateVideoGeneration() error {
	return c.ValidateCapability(ModalityVideo, "")
}
