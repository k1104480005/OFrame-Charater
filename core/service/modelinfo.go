package service

import "github.com/oframe/character-workbench/core/provider"

// ModelInfo describes the models behind the current UI actions so interfaces
// can show "which model am I about to call" before any spend happens.
type ModelInfo struct {
	ProviderID   string `json:"providerId"`
	ProviderName string `json:"providerName"`
	ImageModel   string `json:"imageModel,omitempty"` // active provider's resolved image model
	// Enhancement target: the configured enhance association when set,
	// otherwise the active provider's text model.
	EnhanceProviderID string `json:"enhanceProviderId,omitempty"`
	EnhanceModel      string `json:"enhanceModel,omitempty"`
	EnhanceSupported  bool   `json:"enhanceSupported"`
}

// CurrentModelInfo resolves the effective models without any network call:
// capability flags and locally configured/preset catalogs only.
func (s *Service) CurrentModelInfo() *ModelInfo {
	ps := s.settings.ProviderSettings()
	out := &ModelInfo{}
	providerID := ps.ActiveProvider
	if providerID == "" {
		providerID = provider.DefaultProviderID
	}
	p, cfg, err := s.providerAndConfig(providerID)
	if err != nil {
		return out
	}
	out.ProviderID = p.ID()
	out.ProviderName = p.Name()
	caps := p.Capabilities()
	if caps.Has(provider.ModalityImage) {
		if m, err := provider.ResolveValidatedModel(caps, cfg, provider.ModalityImage, ""); err == nil {
			out.ImageModel = m
		}
	}
	// Enhancement target: association first, then the active provider.
	enhanceID := ps.EnhanceProviderID
	enhanceExplicit := ps.EnhanceModel
	if enhanceID == "" {
		enhanceID = providerID
	} else {
		enhanceExplicit = ps.EnhanceModel
	}
	tp, tcfg, err := s.providerAndConfig(enhanceID)
	if err != nil {
		return out
	}
	tcaps := tp.Capabilities()
	if !tcaps.Has(provider.ModalityText) {
		return out
	}
	if m, err := provider.ResolveValidatedModel(tcaps, tcfg, provider.ModalityText, enhanceExplicit); err == nil {
		out.EnhanceProviderID = tp.ID()
		out.EnhanceModel = m
		out.EnhanceSupported = true
	}
	return out
}
