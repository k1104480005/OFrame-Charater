// Phase-3 typed bindings over the shared application service (core/service):
// provider configuration/validation, local call statistics, PerfectPixel
// presets, and the generation confirmation flow (Prepare → review → Confirm).
// The oframe CLI calls the same service type, so GUI and CLI cannot drift
// (design D1/D12, cli spec: 与 GUI 共享同一 Go 核心库).
package main

import (
	"context"
	"time"

	"github.com/oframe/character-workbench/core/provider"
	"github.com/oframe/character-workbench/core/service"
)

// --- view types (frontend models live in the main namespace) ---

// ProviderInfoView is the read view of one registered provider (keys never
// returned). Task 4.1: protocol type, capability flags, per-capability model
// catalogs and reserved video models are exposed for the settings UI.
type ProviderInfoView struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"` // doubao | openai | agnes | compatible | api | dashscope | gemini | minimax | volcengine | cli
	Name         string   `json:"name"`
	Builtin      bool     `json:"builtin"`
	Active       bool     `json:"active"`
	Image        bool     `json:"image"`
	Video        bool     `json:"video"`
	Text         bool     `json:"text"`
	ImageModel   string   `json:"imageModel"`
	VideoModel   string   `json:"videoModel"`
	TextModel    string   `json:"textModel"`
	ImageModels  []string `json:"imageModels"`
	VideoModels  []string `json:"videoModels"` // 预留目录：视频能力接入前不可调用
	TextModels   []string `json:"textModels"`
	BaseURL      string   `json:"baseUrl"`
	HasAPIKey    bool     `json:"hasApiKey"`
	KeySource    string   `json:"keySource"` // settings | env | none
	MaxAttempts  int      `json:"maxAttempts"`
	PricePerCall float64  `json:"pricePerCall"`
	Currency     string   `json:"currency"`
}

// ProviderConfigView is the editable local config of one provider (key
// included: it is stored locally and edited in the settings panel). Task 4.1:
// per-capability model catalogs and the structured CLI fields travel with the
// config so every FrameBaker preset card is one round-trippable shape.
type ProviderConfigView struct {
	ProviderID  string   `json:"providerId"`
	Type        string   `json:"type"` // "" → built-in id or "compatible"
	Name        string   `json:"name"` // display name (custom providers)
	APIKey      string   `json:"apiKey"`
	Model       string   `json:"model"`
	VideoModel  string   `json:"videoModel"`
	TextModel   string   `json:"textModel"`
	ImageModels []string `json:"imageModels,omitempty"`
	VideoModels []string `json:"videoModels,omitempty"`
	TextModels  []string `json:"textModels,omitempty"`
	BaseURL     string   `json:"baseUrl"`
	// DefaultSize is the advisory generation size ("WxH") shown on the card
	// (task 5.2 尺寸); generation itself uses the request's explicit size.
	DefaultSize  string  `json:"defaultSize,omitempty"`
	MaxAttempts  int     `json:"maxAttempts"`
	TimeoutSec   int     `json:"timeoutSec"`
	PricePerCall float64 `json:"pricePerCall"`

	// CLI fields (task 3.1/4.1): used to build an exec argv array, never a
	// shell string.
	CLICommand     string   `json:"cliCommand,omitempty"`
	CLIPromptArg   string   `json:"cliPromptArg,omitempty"`
	CLIOutputArg   string   `json:"cliOutputArg,omitempty"`
	CLIModelArg    string   `json:"cliModelArg,omitempty"`
	CLIRefImageArg string   `json:"cliRefImageArg,omitempty"`
	CLIExtraArgs   []string `json:"cliExtraArgs,omitempty"`
	CLITemplate    string   `json:"cliTemplate,omitempty"` // legacy read-only
}

// ProviderTestView is the live connection-test outcome (settings "测试连接").
type ProviderTestView struct {
	OK        bool     `json:"ok"`
	LatencyMS int64    `json:"latencyMs,omitempty"`
	Models    []string `json:"models,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// ProviderPresetView is the default description of one FrameBaker quick
// preset (task 4.1: 预设元数据 — the settings UI fills provider card drafts
// from this catalog; CLI argument defaults travel inside CLIDraft fields).
type ProviderPresetView struct {
	Key            string   `json:"key"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Type           string   `json:"type"`
	BaseURL        string   `json:"baseUrl"`
	Image          bool     `json:"image"`
	Video          bool     `json:"video"`
	Text           bool     `json:"text"`
	ImageModels    []string `json:"imageModels"`
	VideoModels    []string `json:"videoModels"`
	TextModels     []string `json:"textModels"`
	CLIPromptArg   string   `json:"cliPromptArg,omitempty"`
	CLIOutputArg   string   `json:"cliOutputArg,omitempty"`
	CLIModelArg    string   `json:"cliModelArg,omitempty"`
	CLIRefImageArg string   `json:"cliRefImageArg,omitempty"`
}

// ProviderOptionView is one capability-filtered provider choice for the
// generation forms (task 4.4): Models is only populated when the provider is
// usable for the requested capability; Reason explains rejections readably.
type ProviderOptionView struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Models []string `json:"models"`
	Reason string   `json:"reason,omitempty"`
}

// EnhanceSettingsView is the prompt-enhancement association (task 5.5): which
// provider's text model is used for prompt enhancement; an empty providerId
// means "follow the active provider".
type EnhanceSettingsView struct {
	ProviderID string `json:"providerId"`
	Model      string `json:"model"`
}

// configFromView maps the wire view onto the domain config (single mapping so
// GET/SAVE/ADD cannot drift apart — the VideoModel omission bug class).
func configFromView(cfg ProviderConfigView) provider.ProviderConfig {
	return provider.ProviderConfig{
		ProviderID: cfg.ProviderID, Type: cfg.Type, Name: cfg.Name,
		APIKey: cfg.APIKey, Model: cfg.Model, VideoModel: cfg.VideoModel, TextModel: cfg.TextModel,
		ImageModels: cfg.ImageModels, VideoModels: cfg.VideoModels, TextModels: cfg.TextModels,
		BaseURL: cfg.BaseURL, DefaultSize: cfg.DefaultSize, MaxAttempts: cfg.MaxAttempts,
		TimeoutSec: cfg.TimeoutSec, PricePerCall: cfg.PricePerCall,
		CLICommand: cfg.CLICommand, CLIPromptArg: cfg.CLIPromptArg,
		CLIOutputArg: cfg.CLIOutputArg, CLIModelArg: cfg.CLIModelArg,
		CLIRefImageArg: cfg.CLIRefImageArg, CLIExtraArgs: cfg.CLIExtraArgs,
		CLITemplate: cfg.CLITemplate,
	}
}

// viewFromConfig maps the domain config onto the wire view.
func viewFromConfig(cfg provider.ProviderConfig) ProviderConfigView {
	return ProviderConfigView{
		ProviderID: cfg.ProviderID, Type: cfg.Type, Name: cfg.Name,
		APIKey: cfg.APIKey, Model: cfg.Model, VideoModel: cfg.VideoModel, TextModel: cfg.TextModel,
		ImageModels: cfg.ImageModels, VideoModels: cfg.VideoModels, TextModels: cfg.TextModels,
		BaseURL: cfg.BaseURL, DefaultSize: cfg.DefaultSize, MaxAttempts: cfg.MaxAttempts,
		TimeoutSec: cfg.TimeoutSec, PricePerCall: cfg.PricePerCall,
		CLICommand: cfg.CLICommand, CLIPromptArg: cfg.CLIPromptArg,
		CLIOutputArg: cfg.CLIOutputArg, CLIModelArg: cfg.CLIModelArg,
		CLIRefImageArg: cfg.CLIRefImageArg, CLIExtraArgs: cfg.CLIExtraArgs,
		CLITemplate: cfg.CLITemplate,
	}
}

// StatView is one provider/model statistics row.
type StatView struct {
	ProviderID    string  `json:"providerId"`
	Model         string  `json:"model"`
	CallCount     int     `json:"callCount"`
	EstimatedCost float64 `json:"estimatedCost"`
	Currency      string  `json:"currency"`
	LastCallAt    string  `json:"lastCallAt"`
}

// StatsView is the call statistics overview (spec 4.6: 次数与费用估算).
type StatsView struct {
	TotalCalls int        `json:"totalCalls"`
	Items      []StatView `json:"items"`
}

// StylePresetView mirrors pipeline.StylePreset.
type StylePresetView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ActionPresetView mirrors pipeline.ActionPreset.
type ActionPresetView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PresetCatalogView is the PerfectPixel presets catalog (四个风格预设 + 动作预设).
type PresetCatalogView struct {
	Styles  []StylePresetView  `json:"styles"`
	Actions []ActionPresetView `json:"actions"`
}

// OutboundMaterialView is one material that will be sent to the provider (外发素材).
type OutboundMaterialView struct {
	MaterialID string `json:"materialId"`
	Kind       string `json:"kind"`
	Role       string `json:"role"`
	Name       string `json:"name"`
	Path       string `json:"path"`
}

// PromptSnapshotView mirrors pipeline.PromptSnapshot for the confirmation UI.
type PromptSnapshotView struct {
	StylePresetID        string   `json:"stylePresetId"`
	ActionPresetID       string   `json:"actionPresetId"`
	Description          string   `json:"description"`
	ReferenceMaterialIDs []string `json:"referenceMaterialIds"`
	CanvasWidth          int      `json:"canvasWidth"`
	CanvasHeight         int      `json:"canvasHeight"`
	FrameCount           int      `json:"frameCount"`
	Directions           int      `json:"directions"`
	Prompt               string   `json:"prompt"`
	BuiltAt              string   `json:"builtAt"`
}

// GenerationRequestView is the generation-confirmation plan request.
type GenerationRequestView struct {
	PackagePath             string   `json:"packagePath"`
	MotionID                string   `json:"motionId,omitempty"`      // "" → 方向数模式 (legacy)
	ProviderID              string   `json:"providerId"`              // "" → 当前 provider（默认 Doubao）
	Model                   string   `json:"model"`                   // "" → provider 默认
	Directions              int      `json:"directions"`              // 1 | 4 | 8 (legacy 模式)
	DisableMirror           bool     `json:"disableMirror,omitempty"` // legacy 模式: 关闭镜像 → 全方向独立生成
	StylePresetID           string   `json:"stylePresetId"`           // "" → pixel_classic
	ActionPresetID          string   `json:"actionPresetId"`          // "" → walk
	FrameCount              int      `json:"frameCount"`
	MaxAttemptsPerDirection int      `json:"maxAttemptsPerDirection"`     // 0 → 3
	ReplaceDirections       []string `json:"replaceDirections,omitempty"` // 3.5: 验收时手动替换的方向
	RegenerateOf            string   `json:"regenerateOf,omitempty"`      // 5.6: 上一候选 id (重新生成)
}

// GenerationPlanView is the confirmation payload shown before any external call.
type GenerationPlanView struct {
	ID                      string                 `json:"id"`
	Kind                    string                 `json:"kind"` // generate | replace | regenerate
	MotionID                string                 `json:"motionId,omitempty"`
	ProviderID              string                 `json:"providerId"`
	ProviderType            string                 `json:"providerType,omitempty"` // 协议（任务 6.3 确认界面展示）
	Model                   string                 `json:"model"`
	Capability              string                 `json:"capability"` // image | video | text（任务 4.5/6.3）
	Directions              int                    `json:"directions"`
	BasicDirections         int                    `json:"basicDirections"`
	MirroredDirections      int                    `json:"mirroredDirections"`
	BasicLabels             []string               `json:"basicLabels,omitempty"`
	MirroredLabels          []string               `json:"mirroredLabels,omitempty"`
	ExpectedCalls           int                    `json:"expectedCalls"`
	MaxAttemptsPerDirection int                    `json:"maxAttemptsPerDirection"`
	MaxTotalAttempts        int                    `json:"maxTotalAttempts"`
	OutboundMaterials       []OutboundMaterialView `json:"outboundMaterials"`
	Prompt                  PromptSnapshotView     `json:"prompt"`
	CostPerCall             float64                `json:"costPerCall"`
	Currency                string                 `json:"currency"`
	ExpectedCost            float64                `json:"expectedCost"`
	MaxCost                 float64                `json:"maxCost"`
	Status                  string                 `json:"status"`
	CreatedAt               string                 `json:"createdAt"`
}

// DirectionResultView is one generated direction's outcome.
type DirectionResultView struct {
	Direction string `json:"direction"`
	Attempts  int    `json:"attempts"`
	Bytes     int    `json:"bytes"`
	Model     string `json:"model"`
}

// GenerationResultView is returned after the confirmation decision.
type GenerationResultView struct {
	PlanID    string                `json:"planId"`
	Accepted  bool                  `json:"accepted"`
	Status    string                `json:"status"`
	CallsMade int                   `json:"callsMade"`
	Attempts  int                   `json:"attempts"`
	Results   []DirectionResultView `json:"results"`
	Error     string                `json:"error"`
}

// --- provider configuration & validation (模型/密钥配置与验证) ---

// ProviderList returns the registered providers with their local config status.
func (a *App) ProviderList() ([]ProviderInfoView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	infos, err := svc.ProviderList()
	if err != nil {
		return nil, err
	}
	out := make([]ProviderInfoView, 0, len(infos))
	for _, p := range infos {
		out = append(out, ProviderInfoView{
			ID: p.ID, Type: p.Type, Name: p.Name, Builtin: p.Builtin, Active: p.Active,
			Image: p.Capabilities.Image, Video: p.Capabilities.Video, Text: p.Capabilities.Text,
			ImageModel: p.ImageModel, VideoModel: p.VideoModel, TextModel: p.TextModel,
			ImageModels: p.ImageModels, VideoModels: p.VideoModels, TextModels: p.TextModels,
			BaseURL:   p.BaseURL,
			HasAPIKey: p.HasAPIKey, KeySource: p.KeySource,
			MaxAttempts: p.MaxAttempts, PricePerCall: p.PricePerCall, Currency: p.Currency,
		})
	}
	return out, nil
}

// ProviderPresets returns the seven FrameBaker quick-preset descriptions
// (task 4.1: 预设元数据) — keys, protocol types, default endpoints, capability
// flags, model catalogs and CLI argument defaults.
func (a *App) ProviderPresets() ([]ProviderPresetView, error) {
	presets := provider.Presets()
	out := make([]ProviderPresetView, 0, len(presets))
	for _, p := range presets {
		out = append(out, ProviderPresetView{
			Key: p.Key, Name: p.Name, Description: p.Description, Type: p.Type, BaseURL: p.BaseURL,
			Image: p.Capabilities.Image, Video: p.Capabilities.Video, Text: p.Capabilities.Text,
			ImageModels: p.ImageModels, VideoModels: p.VideoModels, TextModels: p.TextModels,
			CLIPromptArg: p.CLIDraft.CLIPromptArg, CLIOutputArg: p.CLIDraft.CLIOutputArg,
			CLIModelArg: p.CLIDraft.CLIModelArg, CLIRefImageArg: p.CLIDraft.CLIRefImageArg,
		})
	}
	return out, nil
}

// ProviderConfigGet returns the editable local config of a provider (key included).
func (a *App) ProviderConfigGet(id string) (*ProviderConfigView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	cfg, err := svc.ProviderConfig(id)
	if err != nil {
		return nil, err
	}
	view := viewFromConfig(cfg)
	return &view, nil
}

// ProviderConfigSave validates and persists a provider's local config, and
// rebuilds the adapter so the change takes effect immediately (runtime switch).
func (a *App) ProviderConfigSave(id string, cfg ProviderConfigView) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	domain := configFromView(cfg)
	domain.ProviderID = id
	return svc.SaveProviderConfig(id, domain)
}

// ProviderAdd registers a new custom provider from the settings presets
// (id may be empty → generated from the display name; explicit protocol types
// are preserved — task 2.7/4.1).
func (a *App) ProviderAdd(cfg ProviderConfigView) (*ProviderInfoView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	info, err := svc.ProviderAdd(configFromView(cfg))
	if err != nil {
		return nil, err
	}
	return &ProviderInfoView{
		ID: info.ID, Type: info.Type, Name: info.Name, Builtin: info.Builtin, Active: info.Active,
		Image: info.Capabilities.Image, Video: info.Capabilities.Video, Text: info.Capabilities.Text,
		ImageModel: info.ImageModel, VideoModel: info.VideoModel, TextModel: info.TextModel,
		ImageModels: info.ImageModels, VideoModels: info.VideoModels, TextModels: info.TextModels,
		BaseURL:   info.BaseURL,
		HasAPIKey: info.HasAPIKey, KeySource: info.KeySource,
		MaxAttempts: info.MaxAttempts, PricePerCall: info.PricePerCall, Currency: info.Currency,
	}, nil
}

// ProviderRemove deletes a custom provider (built-ins are refused).
func (a *App) ProviderRemove(id string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.ProviderRemove(id)
}

// ProviderTest runs a live connection test against the provider's /models
// endpoint (settings "测试连接").
func (a *App) ProviderTest(id string) (*ProviderTestView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	res := svc.TestProvider(id)
	return &ProviderTestView{OK: res.OK, LatencyMS: res.LatencyMS, Models: res.Models, Error: res.Error}, nil
}

// ProviderModels fetches the model ids exposed by the provider's persisted
// configuration (settings "获取模型").
func (a *App) ProviderModels(id string) ([]string, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	return svc.ListProviderModels(id)
}

// ProviderTestDraft runs a connection test against the UNSAVED form values
// (tasks 4.2/5.3): nothing is persisted, registered or activated.
func (a *App) ProviderTestDraft(cfg ProviderConfigView) (*ProviderTestView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	res := svc.TestProviderDraft(configFromView(cfg))
	return &ProviderTestView{OK: res.OK, LatencyMS: res.LatencyMS, Models: res.Models, Error: res.Error}, nil
}

// ProviderModelsDraft discovers models from the UNSAVED form values
// (tasks 4.3/5.3) without persisting or switching anything.
func (a *App) ProviderModelsDraft(cfg ProviderConfigView) ([]string, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	return svc.ListProviderModelsDraft(configFromView(cfg))
}

// ProviderOptions returns the capability-filtered provider/model choices for
// generation forms (task 4.4): capability is "image" | "video" | "text";
// unusable entries carry a readable Reason and never expose models.
func (a *App) ProviderOptions(capability string) ([]ProviderOptionView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	options, err := svc.ProviderOptions(capability)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderOptionView, 0, len(options))
	for _, opt := range options {
		out = append(out, ProviderOptionView{
			ID: opt.ID, Name: opt.Name, Type: opt.Type, Models: opt.Models, Reason: opt.Reason,
		})
	}
	return out, nil
}

// EnhanceSettingsGet returns the prompt-enhancement association (task 5.5).
func (a *App) EnhanceSettingsGet() (*EnhanceSettingsView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	es := svc.EnhanceSettingsGet()
	return &EnhanceSettingsView{ProviderID: es.ProviderID, Model: es.Model}, nil
}

// EnhanceSettingsSet validates and persists the prompt-enhancement
// association (task 5.5): the provider must exist and declare text
// capability; a named model must belong to its text catalog.
func (a *App) EnhanceSettingsSet(providerID, model string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.EnhanceSettingsSet(providerID, model)
}

// VideoExtractionConfigView is the read-only video-model configuration for
// the future filmstrip-extraction pipeline (task 6.2): catalogs are readable
// and restart-safe, while Supported stays false until video executes.
type VideoExtractionConfigView struct {
	ProviderID  string   `json:"providerId"`
	Type        string   `json:"type"`
	VideoModels []string `json:"videoModels"`
	Supported   bool     `json:"supported"`
	Reason      string   `json:"reason"`
}

// VideoExtractionConfig returns the video-model configuration of a provider
// (task 6.2). It performs zero external calls.
func (a *App) VideoExtractionConfig(id string) (*VideoExtractionConfigView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	cfg, err := svc.VideoExtractionConfig(id)
	if err != nil {
		return nil, err
	}
	return &VideoExtractionConfigView{
		ProviderID: cfg.ProviderID, Type: cfg.Type, VideoModels: cfg.VideoModels,
		Supported: cfg.Supported, Reason: cfg.Reason,
	}, nil
}

// ProviderSetActive switches the active provider at runtime (spec 4.1).
func (a *App) ProviderSetActive(id string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.SetActiveProvider(id)
}

// ProviderValidate runs the offline configuration validation of a provider.
// It returns "ok" on success.
func (a *App) ProviderValidate(id string) (string, error) {
	svc, err := a.service()
	if err != nil {
		return "", err
	}
	if err := svc.ValidateProvider(id); err != nil {
		return "", err
	}
	return "ok", nil
}

// ProviderStats returns the local call statistics (spec 4.6).
func (a *App) ProviderStats() (*StatsView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	stats := svc.ProviderStats()
	items := make([]StatView, 0, len(stats.Items))
	for _, st := range stats.Items {
		items = append(items, StatView{
			ProviderID: st.ProviderID, Model: st.Model, CallCount: st.CallCount,
			EstimatedCost: st.EstimatedCost, Currency: st.Currency, LastCallAt: st.LastCallAt,
		})
	}
	return &StatsView{TotalCalls: stats.TotalCalls(), Items: items}, nil
}

// --- PerfectPixel presets (四个风格预设 + 动作预设) ---

// PresetCatalog returns the built-in PerfectPixel presets.
func (a *App) PresetCatalog() (*PresetCatalogView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	cat := svc.PresetCatalog()
	out := &PresetCatalogView{Styles: []StylePresetView{}, Actions: []ActionPresetView{}}
	for _, s := range cat.Styles {
		out.Styles = append(out.Styles, StylePresetView{ID: s.ID, Name: s.Name, Description: s.Description})
	}
	for _, ac := range cat.Actions {
		out.Actions = append(out.Actions, ActionPresetView{ID: ac.ID, Name: ac.Name, Description: ac.Description})
	}
	return out, nil
}

// --- generation confirmation (生成确认) ---

// GenerationPlanPrepare builds the generation confirmation plan for the current
// identity package. No external call is made.
func (a *App) GenerationPlanPrepare(req GenerationRequestView) (*GenerationPlanView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	plan, err := svc.PrepareGeneration(context.Background(), service.GenerationRequest{
		PackagePath:             pkg.Root(),
		MotionID:                req.MotionID,
		ProviderID:              req.ProviderID,
		Model:                   req.Model,
		Directions:              req.Directions,
		DisableMirror:           req.DisableMirror,
		StylePresetID:           req.StylePresetID,
		ActionPresetID:          req.ActionPresetID,
		FrameCount:              req.FrameCount,
		MaxAttemptsPerDirection: req.MaxAttemptsPerDirection,
		ReplaceDirections:       req.ReplaceDirections,
		RegenerateOf:            req.RegenerateOf,
	})
	if err != nil {
		return nil, err
	}
	return planToView(plan), nil
}

// GenerationPlanConfirm decides a prepared plan: accept=true executes the
// provider calls (with the agreed retry cap); accept=false aborts with no call.
func (a *App) GenerationPlanConfirm(planID string, accept bool) (*GenerationResultView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	res, err := svc.ConfirmGeneration(context.Background(), planID, accept)
	if err != nil {
		return nil, err
	}
	out := &GenerationResultView{
		PlanID: res.PlanID, Accepted: res.Accepted, Status: res.Status,
		CallsMade: res.CallsMade, Attempts: res.Attempts, Error: res.Error,
		Results: []DirectionResultView{},
	}
	for _, r := range res.Results {
		out.Results = append(out.Results, DirectionResultView{Direction: r.Direction, Attempts: r.Attempts, Bytes: r.Bytes, Model: r.Model})
	}
	return out, nil
}

// GenerationPlanGet returns a prepared plan by id (for re-render after an
// event or a cancelled confirmation).
func (a *App) GenerationPlanGet(planID string) (*GenerationPlanView, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	plan, err := svc.GetPlan(planID)
	if err != nil {
		return nil, err
	}
	return planToView(plan), nil
}

func planToView(plan *service.GenerationPlan) *GenerationPlanView {
	mats := make([]OutboundMaterialView, 0, len(plan.OutboundMaterials))
	for _, o := range plan.OutboundMaterials {
		mats = append(mats, OutboundMaterialView{MaterialID: o.MaterialID, Kind: o.Kind, Role: o.Role, Name: o.Name, Path: o.Path})
	}
	refIDs := append([]string(nil), plan.Prompt.ReferenceMaterialIDs...)
	return &GenerationPlanView{
		ID: plan.ID, Kind: plan.Kind, MotionID: plan.MotionID,
		ProviderID: plan.ProviderID, ProviderType: plan.ProviderType, Model: plan.Model,
		Capability: plan.Capability,
		Directions: plan.Directions, BasicDirections: plan.BasicDirections,
		MirroredDirections:      plan.MirroredDirections,
		BasicLabels:             plan.BasicLabels,
		MirroredLabels:          plan.MirroredLabels,
		ExpectedCalls:           plan.ExpectedCalls,
		MaxAttemptsPerDirection: plan.MaxAttemptsPerDirection,
		MaxTotalAttempts:        plan.MaxTotalAttempts,
		OutboundMaterials:       mats,
		Prompt: PromptSnapshotView{
			StylePresetID:        plan.Prompt.StylePresetID,
			ActionPresetID:       plan.Prompt.ActionPresetID,
			Description:          plan.Prompt.Description,
			ReferenceMaterialIDs: refIDs,
			CanvasWidth:          plan.Prompt.CanvasWidth,
			CanvasHeight:         plan.Prompt.CanvasHeight,
			FrameCount:           plan.Prompt.FrameCount,
			Directions:           plan.Prompt.Directions,
			Prompt:               plan.Prompt.Prompt,
			BuiltAt:              plan.Prompt.BuiltAt.Format(time.RFC3339),
		},
		CostPerCall: plan.CostPerCall, Currency: plan.Currency,
		ExpectedCost: plan.ExpectedCost, MaxCost: plan.MaxCost,
		Status: plan.Status, CreatedAt: plan.CreatedAt,
	}
}
