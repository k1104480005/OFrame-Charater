// Package service is the GUI/CLI shared application service (设计 D1/D12 单核
// 多端): the thin application layer that both the Wails GUI bindings and the
// oframe CLI call for provider configuration/validation, local call
// statistics, PerfectPixel presets, and the generation confirmation flow
// (阶段 3: GUI/CLI 共享 application service). It composes core/identity,
// core/pipeline, core/provider and core/settings; the filmstrip pipeline
// execution, the full task queue, quality acceptance and export stay in later
// phases.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/oframe/character-workbench/core/pipeline"
	"github.com/oframe/character-workbench/core/provider"
	"github.com/oframe/character-workbench/core/settings"
	"github.com/oframe/character-workbench/core/task"
)

// Options configures a Service.
type Options struct {
	// SettingsDir is the local config directory; empty selects the user
	// config directory (settings.New).
	SettingsDir string
	// HTTPClient is injected so tests use a fake transport; nil → default.
	HTTPClient *http.Client
	// Logger is optional; nil → slog.Default().
	Logger *slog.Logger
}

// Service is the shared application service instance.
type Service struct {
	log      *slog.Logger
	settings *settings.Store
	registry *provider.Registry
	client   *http.Client
	plans    *planRegistry

	// queueStore is the persisted, recoverable task queue (tasks 6.1–6.5):
	// SQLite-backed, survives app restarts, deduplicates identical tasks via
	// the success-result cache.
	queueStore *task.Store

	// candidates retains the filmstrip pipeline candidates per identity
	// package (CandidateSet 保留, task 5.6 / filmstrip 管线接入生成执行链).
	candMu     sync.Mutex
	candidates map[string]*pipeline.CandidateSet
}

// New creates the shared service: it loads (or initializes) the local
// settings, opens the persisted task queue next to them, builds the provider
// registry from the persisted configuration (keys/models), and activates the
// persisted active provider (default Doubao).
func New(opts Options) (*Service, error) {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	st, err := settings.New(opts.SettingsDir)
	if err != nil {
		return nil, err
	}
	q, err := task.Open(filepath.Join(st.Dir(), QueueFileName), log)
	if err != nil {
		return nil, err
	}
	s := &Service{
		log:        log,
		settings:   st,
		registry:   provider.NewRegistry(),
		client:     opts.HTTPClient,
		plans:      newPlanRegistry(),
		queueStore: q,
		candidates: make(map[string]*pipeline.CandidateSet),
	}
	if err := s.rebuildRegistry(); err != nil {
		_ = q.Close()
		return nil, err
	}
	return s, nil
}

// rebuildRegistry constructs all adapters from the persisted local config and
// applies the persisted active provider.
func (s *Service) rebuildRegistry() error {
	ps := s.settings.ProviderSettings()
	for id := range ps.Providers {
		p, err := provider.NewAdapter(id, ps.ConfigFor(id), s.client)
		if err != nil {
			return err
		}
		if err := s.registry.Register(p); err != nil {
			// Registry is fresh; duplicate ids cannot occur, but tolerate a
			// re-register by replacing.
			_ = s.registry.Replace(p)
		}
	}
	if ps.ActiveProvider != "" {
		if err := s.registry.SetActive(ps.ActiveProvider); err != nil {
			return err
		}
	}
	return nil
}

// Logger returns the service logger.
func (s *Service) Logger() *slog.Logger { return s.log }

// SettingsDir returns the local settings directory.
func (s *Service) SettingsDir() string { return s.settings.Dir() }

// Close releases the persisted task queue database (call when the service is
// done, e.g. at CLI exit, so the SQLite file is not left locked).
func (s *Service) Close() error {
	if s.queueStore != nil {
		return s.queueStore.Close()
	}
	return nil
}

// --- provider configuration & validation (模型/密钥配置与验证) ---

// ProviderInfo is the read view of one registered provider.
type ProviderInfo struct {
	ID           string                `json:"id"`
	Type         string                `json:"type"` // doubao | openai | agnes | compatible | api | dashscope | gemini | minimax | volcengine | cli
	Name         string                `json:"name"`
	Builtin      bool                  `json:"builtin"`
	Active       bool                  `json:"active"`
	Capabilities provider.Capabilities `json:"capabilities"`
	ImageModel   string                `json:"imageModel"`
	VideoModel   string                `json:"videoModel"`
	TextModel    string                `json:"textModel"`
	ImageModels  []string              `json:"imageModels"` // per-capability catalogs (task 4.1)
	VideoModels  []string              `json:"videoModels"` // reserved until video executes
	TextModels   []string              `json:"textModels"`
	BaseURL      string                `json:"baseUrl"`
	HasAPIKey    bool                  `json:"hasApiKey"`
	KeySource    string                `json:"keySource"` // settings | env | none
	MaxAttempts  int                   `json:"maxAttempts"`
	PricePerCall float64               `json:"pricePerCall"`
	Currency     string                `json:"currency"`
}

// ProviderList returns the registered providers with their current local
// configuration status (keys are never returned).
func (s *Service) ProviderList() ([]ProviderInfo, error) {
	ps := s.settings.ProviderSettings()
	out := make([]ProviderInfo, 0, len(ps.Providers))
	for _, p := range s.registry.List() {
		cfg := ps.ConfigFor(p.ID())
		_, keyErr := cfg.ResolveAPIKey()
		info := ProviderInfo{
			ID:           p.ID(),
			Type:         cfg.EffectiveType(),
			Name:         p.Name(),
			Builtin:      provider.IsBuiltin(p.ID()),
			Active:       ps.ActiveProvider == p.ID(),
			Capabilities: p.Capabilities(),
			ImageModel:   cfg.EffectiveModel(),
			VideoModel:   cfg.VideoModel,
			TextModel:    cfg.EffectiveTextModel(),
			ImageModels:  cfg.EffectiveImageModels(),
			VideoModels:  cfg.EffectiveVideoModels(),
			TextModels:   cfg.EffectiveTextModels(),
			BaseURL:      cfg.EffectiveBaseURL(),
			MaxAttempts:  cfg.EffectiveMaxAttempts(),
			PricePerCall: cfg.EffectivePrice(),
			Currency:     provider.Currency(p.ID()),
		}
		switch {
		case keyErr == nil && cfg.APIKey != "":
			info.HasAPIKey, info.KeySource = true, "settings"
		case keyErr == nil:
			info.HasAPIKey, info.KeySource = true, "env"
		default:
			info.KeySource = "none"
		}
		out = append(out, info)
	}
	return out, nil
}

// ProviderConfig returns the persisted local config of a provider, including
// its API key (service layer only; GUI bindings redact before sending out).
func (s *Service) ProviderConfig(id string) (provider.ProviderConfig, error) {
	ps := s.settings.ProviderSettings()
	if _, err := s.registry.Get(id); err != nil {
		return provider.ProviderConfig{}, err
	}
	return ps.ConfigFor(id), nil
}

// SaveProviderConfig validates and persists a provider's local config (key /
// model / endpoint / retry cap / price), then rebuilds that adapter so the
// change is effective immediately (runtime switch without restart). An id that
// is not registered yet (e.g. a CLI `provider config set` before any GUI add)
// is validated, persisted and REGISTERED in one step.
func (s *Service) SaveProviderConfig(id string, cfg provider.ProviderConfig) error {
	cfg.ProviderID = id
	if _, err := s.registry.Get(id); err != nil {
		// Unregistered id: structural validation with the same rules as
		// ProviderAdd (an empty type defaults to compatible, custom ids need a
		// display name), then persist + register.
		if cfg.Type == "" && !provider.IsBuiltin(id) {
			cfg.Type = provider.ProviderTypeCompatible
		}
		if strings.TrimSpace(cfg.Name) == "" {
			cfg.Name = id
		}
		if err := cfg.ValidateForAdd(); err != nil {
			return err
		}
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	ps := s.settings.ProviderSettings()
	if ps.Providers == nil {
		ps.Providers = make(map[string]provider.ProviderConfig)
	}
	ps.Providers[id] = cfg
	if err := s.settings.SaveProviderSettings(ps); err != nil {
		return err
	}
	p, err := provider.NewAdapter(id, cfg, s.client)
	if err != nil {
		return err
	}
	if err := s.registry.Replace(p); err != nil {
		// Not registered yet — register it so the provider becomes selectable
		// without a restart.
		return s.registry.Register(p)
	}
	return nil
}

// SetActiveProvider switches the active provider at runtime and persists the
// choice (generation spec 4.1: 运行时切换生效, 原 provider 配置保留).
func (s *Service) SetActiveProvider(id string) error {
	if _, err := s.registry.Get(id); err != nil {
		return err
	}
	ps := s.settings.ProviderSettings()
	ps.ActiveProvider = id
	if err := s.settings.SaveProviderSettings(ps); err != nil {
		return err
	}
	return s.registry.SetActive(id)
}

// ValidateProvider runs the offline configuration validation of a provider
// (模型/密钥配置与验证 — no network call).
func (s *Service) ValidateProvider(id string) error {
	ps := s.settings.ProviderSettings()
	return ps.ConfigFor(id).Validate()
}

// providerAndConfig resolves a registered adapter together with its persisted
// local configuration (shared by the modality validation entry points).
func (s *Service) providerAndConfig(id string) (provider.Provider, provider.ProviderConfig, error) {
	p, err := s.registry.Get(id)
	if err != nil {
		return nil, provider.ProviderConfig{}, err
	}
	return p, s.settings.ProviderSettings().ConfigFor(id), nil
}

// ValidateImageGeneration and ValidateTextGeneration are the external,
// pre-call capability boundary of the generation service (align-framebaker-
// providers task 1.4): they answer whether a call for the requested modality
// may proceed for this provider/model BEFORE anything external happens — no
// network request, no silent provider or model substitution. Errors wrap
// provider.ErrCapabilityUnsupported / ErrModelNotConfigured / ErrModelInvalid.
func (s *Service) ValidateImageGeneration(id, model string) error {
	p, cfg, err := s.providerAndConfig(id)
	if err != nil {
		return err
	}
	if _, verr := provider.ResolveValidatedModel(p.Capabilities(), cfg, provider.ModalityImage, model); verr != nil {
		return fmt.Errorf("service: %w", verr)
	}
	return nil
}

func (s *Service) ValidateTextGeneration(id, model string) error {
	p, cfg, err := s.providerAndConfig(id)
	if err != nil {
		return err
	}
	if _, verr := provider.ResolveValidatedModel(p.Capabilities(), cfg, provider.ModalityText, model); verr != nil {
		return fmt.Errorf("service: %w", verr)
	}
	return nil
}

// ValidateVideoGeneration is the explicit video pre-flight entry point (任务
// 1.4, generation spec: 视频能力在外部调用前校验、不支持时给出明确错误). Until a
// video execution pipeline exists (tasks 2.x / 6.x) every configuration answers
// ErrCapabilityUnsupported — including Doubao, whose config presets a Seedance
// video-model catalog entry that stays metadata-only (预留目录不代表可调用).
// The check performs zero network requests.
func (s *Service) ValidateVideoGeneration(id string) error {
	p, cfg, err := s.providerAndConfig(id)
	if err != nil {
		return err
	}
	if _, verr := provider.ResolveValidatedModel(p.Capabilities(), cfg, provider.ModalityVideo, ""); verr != nil {
		return fmt.Errorf("service: %w", verr)
	}
	return nil
}

// newProviderID derives a stable slug from a display name, disambiguating
// collisions against the existing provider ids.
func newProviderID(name string, existing map[string]bool) string {
	base := strings.ToLower(strings.TrimSpace(name))
	runes := []rune{}
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			runes = append(runes, r)
		case r == '-' || r == '_' || r == ' ':
			runes = append(runes, '-')
		}
	}
	slug := strings.Trim(string(runes), "-")
	if slug == "" {
		slug = "custom-provider"
	}
	if len(slug) > 24 {
		slug = slug[:24]
	}
	candidate := slug
	for i := 2; existing[candidate]; i++ {
		suffix := fmt.Sprintf("-%d", i)
		trim := 24 - len(suffix)
		if trim < 1 {
			trim = 1
		}
		candidate = slug[:min(len(slug), trim)] + suffix
	}
	return candidate
}

// ProviderAdd registers a new custom provider from the settings presets: it
// validates the structural parts (an empty API key is allowed here — the key
// is filled in afterwards), persists the config and registers the adapter so
// the provider is immediately selectable and survives restarts (rebuildRegistry
// re-registers it). 人工验收更新：no id is special-cased any more — a removed
// built-in identity (e.g. "doubao") may be added again like any other id.
func (s *Service) ProviderAdd(cfg provider.ProviderConfig) (ProviderInfo, error) {
	ps := s.settings.ProviderSettings()
	if ps.Providers == nil {
		ps.Providers = make(map[string]provider.ProviderConfig)
	}
	if strings.TrimSpace(cfg.ProviderID) == "" {
		existing := make(map[string]bool, len(ps.Providers))
		for id := range ps.Providers {
			existing[id] = true
		}
		cfg.ProviderID = newProviderID(cfg.Name, existing)
	} else {
		cfg.ProviderID = strings.ToLower(strings.TrimSpace(cfg.ProviderID))
	}
	if _, dup := ps.Providers[cfg.ProviderID]; dup {
		return ProviderInfo{}, &provider.ConfigError{Msg: fmt.Sprintf("provider %q already exists", cfg.ProviderID)}
	}
	// Task 2.7/人工验收更新: keep the caller's explicit protocol type
	// (dashscope / gemini / minimax / volcengine / api / compatible); an empty
	// type defaults to the GENERIC compatible adapter — except for a built-in
	// identity id (doubao/openai/agnes), which restores its own adapter so a
	// removed built-in can be re-added under the same id.
	if cfg.Type == "" {
		if provider.IsBuiltin(cfg.ProviderID) {
			cfg.Type = cfg.ProviderID
		} else {
			cfg.Type = provider.ProviderTypeCompatible
		}
	}
	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = cfg.ProviderID
	}
	if err := cfg.ValidateForAdd(); err != nil {
		return ProviderInfo{}, err
	}
	ps.Providers[cfg.ProviderID] = cfg
	if err := s.settings.SaveProviderSettings(ps); err != nil {
		return ProviderInfo{}, err
	}
	adapter, err := provider.NewAdapter(cfg.ProviderID, cfg, s.client)
	if err != nil {
		return ProviderInfo{}, err
	}
	if err := s.registry.Register(adapter); err != nil {
		return ProviderInfo{}, err
	}
	infos, err := s.ProviderList()
	if err != nil {
		return ProviderInfo{}, err
	}
	for _, info := range infos {
		if info.ID == cfg.ProviderID {
			return info, nil
		}
	}
	return ProviderInfo{}, &provider.ConfigError{Msg: fmt.Sprintf("provider %q added but not listed", cfg.ProviderID)}
}

// ProviderRemove deletes a provider: 人工验收更新 — NO provider is protected,
// including the doubao/openai/agnes identities (they can be re-added from the
// presets or the CLI afterwards). Removing the active provider falls back to
// the first remaining one (or none when the map empties).
func (s *Service) ProviderRemove(id string) error {
	ps := s.settings.ProviderSettings()
	if _, ok := ps.Providers[id]; !ok {
		return &provider.ConfigError{Msg: fmt.Sprintf("provider %q is not configured", id)}
	}
	delete(ps.Providers, id)
	if ps.ActiveProvider == id {
		ps.ActiveProvider = ""
		for _, builtin := range []string{provider.ProviderDoubao, provider.ProviderOpenAI, provider.ProviderAgnes} {
			if _, ok := ps.Providers[builtin]; ok {
				ps.ActiveProvider = builtin
				break
			}
		}
		if ps.ActiveProvider == "" {
			for other := range ps.Providers {
				if ps.ActiveProvider == "" || other < ps.ActiveProvider {
					ps.ActiveProvider = other
				}
			}
		}
	}
	if err := s.settings.SaveProviderSettings(ps); err != nil {
		return err
	}
	if err := s.registry.Remove(id); err != nil {
		return err
	}
	if ps.ActiveProvider == "" {
		return nil
	}
	return s.registry.SetActive(ps.ActiveProvider)
}

// TestProvider performs a live connectivity check against the provider's
// persisted configuration (settings "测试连接").
func (s *Service) TestProvider(id string) provider.TestResult {
	cfg := s.settings.ProviderSettings().ConfigFor(id)
	return s.TestProviderDraft(cfg)
}

// ListProviderModels fetches models from the persisted provider configuration.
func (s *Service) ListProviderModels(id string) ([]string, error) {
	cfg := s.settings.ProviderSettings().ConfigFor(id)
	return s.ListProviderModelsDraft(cfg)
}

// TestProviderDraft performs a live connectivity check using the supplied
// unsaved configuration. Draft operations are deliberately independent of the
// registry and settings store: they neither persist nor activate anything.
func (s *Service) TestProviderDraft(cfg provider.ProviderConfig) provider.TestResult {
	return provider.TestConnection(context.Background(), s.client, cfg)
}

// ListProviderModelsDraft discovers models using the supplied unsaved
// configuration without touching persisted settings or the active provider.
func (s *Service) ListProviderModelsDraft(cfg provider.ProviderConfig) ([]string, error) {
	return provider.DiscoverModels(context.Background(), s.client, cfg)
}

// ProviderOption is a capability-aware provider choice for generation forms.
type ProviderOption struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Models []string `json:"models"`
	Reason string   `json:"reason"`
}

// ProviderOptions returns every registered provider, with models only when the
// adapter declares the requested capability and its local catalog is usable.
// This is entirely offline and never probes a provider endpoint.
func (s *Service) ProviderOptions(capability string) ([]ProviderOption, error) {
	modality := provider.Modality(strings.ToLower(strings.TrimSpace(capability)))
	switch modality {
	case provider.ModalityImage, provider.ModalityVideo, provider.ModalityText:
	default:
		return nil, fmt.Errorf("service: unknown provider capability %q", capability)
	}
	ps := s.settings.ProviderSettings()
	out := make([]ProviderOption, 0, len(s.registry.List()))
	for _, p := range s.registry.List() {
		cfg := ps.ConfigFor(p.ID())
		option := ProviderOption{ID: p.ID(), Name: p.Name(), Type: cfg.EffectiveType()}
		catalog := cfg.EffectiveCatalog(modality)
		switch {
		case !p.Capabilities().Has(modality):
			if modality == provider.ModalityVideo && len(catalog) > 0 {
				// 视频目录是预留元数据：明确说明为什么当前不可选，而不是让
				// 用户误以为已配置的 Seedance/Wan 模型立刻可执行。
				option.Reason = "视频模型目录为预留配置，视频能力接入前不可调用"
			} else {
				option.Reason = fmt.Sprintf("%s capability is not declared by this provider", modality)
			}
		case len(catalog) == 0:
			option.Reason = fmt.Sprintf("no %s models are configured", modality)
		default:
			if _, err := provider.ResolveValidatedModel(p.Capabilities(), cfg, modality, ""); err != nil {
				option.Reason = err.Error()
			} else {
				option.Models = append([]string(nil), catalog...)
			}
		}
		out = append(out, option)
	}
	return out, nil
}

// ProviderStats returns the local call statistics ledger.
func (s *Service) ProviderStats() provider.Stats {
	return s.settings.Stats()
}

// EnhanceSettings is the prompt-enhancement association (task 5.5): the
// provider and text model used for prompt enhancement. An empty ProviderID
// means "follow the active provider's text model" — there are no independent
// enhancement credentials, so the association always reuses an existing
// provider configuration.
type EnhanceSettings struct {
	ProviderID string `json:"providerId"`
	Model      string `json:"model"`
}

// EnhanceSettingsGet returns the persisted association (never a secret).
func (s *Service) EnhanceSettingsGet() EnhanceSettings {
	ps := s.settings.ProviderSettings()
	return EnhanceSettings{ProviderID: ps.EnhanceProviderID, Model: ps.EnhanceModel}
}

// EnhanceSettingsSet validates and persists the association: the provider must
// exist and declare text capability, and a named model must belong to its text
// catalog. An empty providerID resets to "follow the active provider".
func (s *Service) EnhanceSettingsSet(providerID, model string) error {
	providerID = strings.TrimSpace(providerID)
	model = strings.TrimSpace(model)
	ps := s.settings.ProviderSettings()
	if providerID == "" {
		ps.EnhanceProviderID, ps.EnhanceModel = "", ""
	} else {
		p, err := s.registry.Get(providerID)
		if err != nil {
			return fmt.Errorf("service: enhance provider %q: %w", providerID, err)
		}
		if !p.Capabilities().Has(provider.ModalityText) {
			return fmt.Errorf("service: provider %q does not declare text capability (enhancement needs a text model)", providerID)
		}
		if model != "" {
			if _, err := provider.ResolveValidatedModel(p.Capabilities(), ps.ConfigFor(providerID), provider.ModalityText, model); err != nil {
				return fmt.Errorf("service: enhance model: %w", err)
			}
		}
		ps.EnhanceProviderID = providerID
		ps.EnhanceModel = model
	}
	return s.settings.SaveProviderSettings(ps)
}

// VideoExtractionConfig is the read-only video-model configuration view for
// the future filmstrip-extraction pipeline (task 6.2). The persisted video
// catalogs are readable and survive restarts, but Supported is ALWAYS false
// until a video execution pipeline exists: 视频模型目录是预留配置，明确阻止
// 实际视频调用而不是静默失败。The check is purely local (zero external calls);
// callers that ignore Supported and try a video call are stopped again by the
// capability gate (ErrCapabilityUnsupported) before any network activity.
type VideoExtractionConfig struct {
	ProviderID  string   `json:"providerId"`
	Type        string   `json:"type"`
	VideoModels []string `json:"videoModels"`
	Supported   bool     `json:"supported"`
	Reason      string   `json:"reason"`
}

// VideoExtractionConfig returns the video-model configuration of one provider
// together with the current execution boundary.
func (s *Service) VideoExtractionConfig(id string) (VideoExtractionConfig, error) {
	p, cfg, err := s.providerAndConfig(id)
	if err != nil {
		return VideoExtractionConfig{}, err
	}
	out := VideoExtractionConfig{
		ProviderID:  p.ID(),
		Type:        cfg.EffectiveType(),
		VideoModels: cfg.EffectiveVideoModels(),
	}
	if _, verr := provider.ResolveValidatedModel(p.Capabilities(), cfg, provider.ModalityVideo, ""); verr != nil {
		if len(out.VideoModels) > 0 {
			out.Reason = "视频模型目录为预留配置，视频执行管线尚未完成前不可调用"
		} else {
			out.Reason = "该 Provider 未配置视频模型，且视频执行管线尚未完成"
		}
		return out, nil
	}
	// Defense in depth: even a hypothetical video-capable declaration stays
	// gated until the pipeline lands (任务 6.2: 明确阻止实际视频调用).
	out.Reason = "视频执行管线尚未完成，实际视频调用被阻止"
	return out, nil
}

// --- PerfectPixel presets (阶段 3: 四个风格预设 + 动作预设) ---

// PresetCatalog is the read view of the PerfectPixel presets.
type PresetCatalog struct {
	Styles  []pipeline.StylePreset  `json:"styles"`
	Actions []pipeline.ActionPreset `json:"actions"`
}

// PresetCatalog returns the built-in style and action presets.
func (s *Service) PresetCatalog() PresetCatalog {
	return PresetCatalog{Styles: pipeline.StylePresets(), Actions: pipeline.ActionPresets()}
}
