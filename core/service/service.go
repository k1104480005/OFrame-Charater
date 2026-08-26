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
	"log/slog"
	"net/http"
	"path/filepath"
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
	Name         string                `json:"name"`
	Active       bool                  `json:"active"`
	Capabilities provider.Capabilities `json:"capabilities"`
	ImageModel   string                `json:"imageModel"`
	TextModel    string                `json:"textModel"`
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
			Name:         p.Name(),
			Active:       ps.ActiveProvider == p.ID(),
			Capabilities: p.Capabilities(),
			ImageModel:   cfg.EffectiveModel(),
			TextModel:    cfg.EffectiveTextModel(),
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
// change is effective immediately (runtime switch without restart).
func (s *Service) SaveProviderConfig(id string, cfg provider.ProviderConfig) error {
	cfg.ProviderID = id
	if _, err := s.registry.Get(id); err != nil {
		return err
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
	return s.registry.Replace(p)
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

// ProviderStats returns the local call statistics ledger.
func (s *Service) ProviderStats() provider.Stats {
	return s.settings.Stats()
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
