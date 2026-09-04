package provider

import (
	"context"
	"net/http"
)

// Compatible is the generic adapter for user-defined OpenAI-compatible
// providers (any Base URL / model / key): it speaks the same
// images/generations and chat/completions contracts as the built-ins, so any
// vendor exposing an OpenAI-compatible endpoint (火山方舟、百炼、硅基流动、本地
// 服务等) can be added from the settings presets without code changes.
type Compatible struct {
	cfg    ProviderConfig
	client *http.Client
}

// NewCompatible creates the generic OpenAI-compatible adapter.
func NewCompatible(cfg ProviderConfig, client *http.Client) *Compatible {
	return &Compatible{cfg: cfg, client: newClient(client)}
}

func (c *Compatible) ID() string { return c.cfg.ProviderID }

// Name returns the user-defined display name (falls back to the id).
func (c *Compatible) Name() string { return c.cfg.EffectiveName() }

// Capabilities: the compatible adapter attempts both modalities; a provider
// that lacks chat or image endpoints reports its own error at call time.
// Video is never attempted until a video adapter path exists.
func (c *Compatible) Capabilities() Capabilities {
	return Capabilities{Image: true, Video: false, Text: true}
}

func (c *Compatible) DefaultImageModel() string { return c.cfg.EffectiveModel() }

func (c *Compatible) DefaultTextModel() string { return c.cfg.EffectiveTextModel() }

// GenerateImage performs one images/generations call. The configured per-call
// timeout bounds the request (including the CDN fetch when the vendor returns
// a URL) even when the caller passes a bare context.
func (c *Compatible) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	key, err := c.cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	model := ResolveModel(req.Model, c.cfg.EffectiveModel())
	ctx, cancel := applyTimeout(ctx, c.cfg.EffectiveTimeout())
	defer cancel()
	data, mime, err := imagesGenerations(ctx, c.client, c.cfg.EffectiveBaseURL(), key, model, req.Prompt, req.Width, req.Height, req.References)
	if err != nil {
		return nil, err
	}
	return &ImageResult{Data: data, MIME: mime, Provider: c.ID(), Model: model}, nil
}

// GenerateText performs one chat/completions call with the same unified auth,
// timeout and error handling as the image path.
func (c *Compatible) GenerateText(ctx context.Context, req TextRequest) (*TextResult, error) {
	key, err := c.cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	model := ResolveModel(req.Model, c.cfg.EffectiveTextModel())
	ctx, cancel := applyTimeout(ctx, c.cfg.EffectiveTimeout())
	defer cancel()
	var text string
	switch c.cfg.APIProtocol {
	case APIProtocolResponses:
		text, err = responsesCompletionText(ctx, c.client, c.cfg.EffectiveBaseURL(), key, model, req.Prompt)
	case APIProtocolAnthropic:
		text, err = anthropicMessagesText(ctx, c.client, c.cfg.EffectiveBaseURL(), key, model, req.Prompt)
	default:
		text, err = chatCompletionText(ctx, c.client, c.cfg.EffectiveBaseURL(), key, model, req.Prompt, req.ImageDataURL)
	}
	if err != nil {
		return nil, err
	}
	return &TextResult{Text: text, Provider: c.ID(), Model: model}, nil
}
