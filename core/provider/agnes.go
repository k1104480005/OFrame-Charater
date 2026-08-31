package provider

import (
	"context"
	"encoding/base64"
	"fmt"
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
	data, mime, err := agnesImagesGenerate(ctx, a.client, a.cfg.EffectiveBaseURL(), key, model, req.Prompt, req.Width, req.Height, req.References)
	if err != nil {
		return nil, err
	}
	return &ImageResult{Data: data, MIME: mime, Provider: a.ID(), Model: model}, nil
}

// agnesImagesGenerate performs one images/generations call using the Agnes
// gateway's OWN request contract, which deliberately differs from the generic
// OpenAI-compatible shape (官方文档实测，2026-08-31):
//   - `response_format` 只能放在 `extra_body` 内部；顶层形状（含历史 "png"）会让
//     网关把请求吞进永不返回的队列或直接 400；
//   - `return_base64` 顶层开关实测同样挂起 → 统一走 `extra_body.response_format:
//     "url"`（实测 11s 出图），响应 data[0].url 由共享解析器下载（不带凭证）；
//   - 图生图/多图合成的参考图放 `extra_body.image`（Data URI Base64 字符串数组，
//     无 role 字段）；
//   - 不传 `n`（未在文档中定义）。
func agnesImagesGenerate(ctx context.Context, client *http.Client, baseURL, apiKey, model, prompt string, width, height int, refs []ReferenceImage) ([]byte, string, error) {
	if width <= 0 {
		width = DefaultGenerationSize
	}
	if height <= 0 {
		height = DefaultGenerationSize
	}
	extra := map[string]any{"response_format": "url"}
	if len(refs) > 0 {
		imgs := make([]string, 0, len(refs))
		for _, r := range refs {
			imgs = append(imgs, "data:"+mimeOr(r.MIME)+";base64,"+base64.StdEncoding.EncodeToString(r.Data))
		}
		extra["image"] = imgs
	}
	body := map[string]any{
		"model":      model,
		"prompt":     prompt,
		"size":       fmt.Sprintf("%dx%d", width, height),
		"extra_body": extra,
	}
	raw, err := postJSON(ctx, client, baseURL+"/images/generations", apiKey, body)
	if err != nil {
		return nil, "", err
	}
	return parseImageGenResponse(ctx, client, raw)
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
