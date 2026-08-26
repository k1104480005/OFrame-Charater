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
// returned).
type ProviderInfoView struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Active       bool    `json:"active"`
	Image        bool    `json:"image"`
	Text         bool    `json:"text"`
	ImageModel   string  `json:"imageModel"`
	TextModel    string  `json:"textModel"`
	BaseURL      string  `json:"baseUrl"`
	HasAPIKey    bool    `json:"hasApiKey"`
	KeySource    string  `json:"keySource"` // settings | env | none
	MaxAttempts  int     `json:"maxAttempts"`
	PricePerCall float64 `json:"pricePerCall"`
	Currency     string  `json:"currency"`
}

// ProviderConfigView is the editable local config of one provider (key
// included: it is stored locally and edited in the settings panel).
type ProviderConfigView struct {
	ProviderID   string  `json:"providerId"`
	APIKey       string  `json:"apiKey"`
	Model        string  `json:"model"`
	TextModel    string  `json:"textModel"`
	BaseURL      string  `json:"baseUrl"`
	MaxAttempts  int     `json:"maxAttempts"`
	TimeoutSec   int     `json:"timeoutSec"`
	PricePerCall float64 `json:"pricePerCall"`
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
	Model                   string                 `json:"model"`
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
			ID: p.ID, Name: p.Name, Active: p.Active,
			Image: p.Capabilities.Image, Text: p.Capabilities.Text,
			ImageModel: p.ImageModel, TextModel: p.TextModel, BaseURL: p.BaseURL,
			HasAPIKey: p.HasAPIKey, KeySource: p.KeySource,
			MaxAttempts: p.MaxAttempts, PricePerCall: p.PricePerCall, Currency: p.Currency,
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
	return &ProviderConfigView{
		ProviderID: cfg.ProviderID, APIKey: cfg.APIKey,
		Model: cfg.Model, TextModel: cfg.TextModel, BaseURL: cfg.BaseURL,
		MaxAttempts: cfg.MaxAttempts, TimeoutSec: cfg.TimeoutSec, PricePerCall: cfg.PricePerCall,
	}, nil
}

// ProviderConfigSave validates and persists a provider's local config, and
// rebuilds the adapter so the change takes effect immediately (runtime switch).
func (a *App) ProviderConfigSave(id string, cfg ProviderConfigView) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.SaveProviderConfig(id, provider.ProviderConfig{
		ProviderID: id, APIKey: cfg.APIKey, Model: cfg.Model, TextModel: cfg.TextModel,
		BaseURL: cfg.BaseURL, MaxAttempts: cfg.MaxAttempts,
		TimeoutSec: cfg.TimeoutSec, PricePerCall: cfg.PricePerCall,
	})
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
		ProviderID: plan.ProviderID, Model: plan.Model,
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
