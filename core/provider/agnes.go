package provider

import (
	"context"
	"net/http"
)

// Agnes is the specialized supplementary adapter backed by the REAL Agnes AI
// free multimodal gateway (人工验收反馈 + 快速接入文档): an OpenAI-compatible
// surface at https://apihub.agnes-ai.com/v1 with Bearer auth — images via
// /images/generations, text via /chat/completions. The adapter is fully
// registered and switchable, and never affects the default Doubao path.
//
// 多模态（人工验收反馈）：Agnes 声明图像 + 文本两种能力；视频保持未声明
// （目录预留，全局视频门禁拦截）。
type Agnes struct {
	cfg    ProviderConfig
	client *http.Client
}

// NewAgnes creates the Agnes adapter.
func NewAgnes(cfg ProviderConfig, client *http.Client) *Agnes {
	return &Agnes{cfg: cfg, client: newClient(client)}
}

func (a *Agnes) ID() string { return ProviderAgnes }

func (a *Agnes) Name() string { return "Agnes（免费多模态）" }

// Capabilities mirrors the multimodal declaration: image + text execute
// through the OpenAI-compatible surface; video stays undeclared (全局视频门禁).
func (a *Agnes) Capabilities() Capabilities {
	return Capabilities{Image: true, Video: false, Text: true}
}

func (a *Agnes) DefaultImageModel() string { return a.cfg.EffectiveModel() }

func (a *Agnes) DefaultTextModel() string { return a.cfg.EffectiveTextModel() }

// GenerateImage performs one images/generations call through the Agnes adapter.
func (a *Agnes) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	// 占位端点预检：给出可读的接入提示，而不是裸 DNS 错误（零外呼）。
	if err := a.cfg.CheckPlaceholderEndpoint(); err != nil {
		return nil, err
	}
	key, err := a.cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	model := ResolveModel(req.Model, a.cfg.EffectiveModel())
	ctx, cancel := applyTimeout(ctx, a.cfg.EffectiveTimeout())
	defer cancel()
	data, mime, err := imagesGenerations(ctx, a.client, a.cfg.EffectiveBaseURL(), key, model, req.Prompt, req.Width, req.Height, req.References)
	if err != nil {
		return nil, err
	}
	return &ImageResult{Data: data, MIME: mime, Provider: a.ID(), Model: model}, nil
}

// GenerateText performs one chat/completions call — the multimodal text half
// of the Agnes surface, sharing the unified auth/timeout/error layer with the
// image path.
func (a *Agnes) GenerateText(ctx context.Context, req TextRequest) (*TextResult, error) {
	// 占位端点预检（零外呼）。
	if err := a.cfg.CheckPlaceholderEndpoint(); err != nil {
		return nil, err
	}
	key, err := a.cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	model := ResolveModel(req.Model, a.cfg.EffectiveTextModel())
	ctx, cancel := applyTimeout(ctx, a.cfg.EffectiveTimeout())
	defer cancel()
	text, err := chatCompletionText(ctx, a.client, a.cfg.EffectiveBaseURL(), key, model, req.Prompt)
	if err != nil {
		return nil, err
	}
	return &TextResult{Text: text, Provider: a.ID(), Model: model}, nil
}
