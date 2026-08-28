package provider

import (
	"context"
	"net/http"
)

// OpenAI is the gpt-image-2 high-quality fallback adapter (高质量备选).
// Image generation only.
type OpenAI struct {
	cfg    ProviderConfig
	client *http.Client
}

// NewOpenAI creates the gpt-image-2 adapter.
func NewOpenAI(cfg ProviderConfig, client *http.Client) *OpenAI {
	return &OpenAI{cfg: cfg, client: newClient(client)}
}

func (o *OpenAI) ID() string { return ProviderOpenAI }

func (o *OpenAI) Name() string { return "OpenAI gpt-image-2" }

func (o *OpenAI) Capabilities() Capabilities { return Capabilities{Image: true, Video: false} }

func (o *OpenAI) DefaultImageModel() string { return DefaultOpenAIModel }

func (o *OpenAI) DefaultTextModel() string { return "" }

// GenerateImage performs one images/generations call with gpt-image-2.
func (o *OpenAI) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	key, err := o.cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	model := ResolveModel(req.Model, o.cfg.EffectiveModel())
	data, mime, err := imagesGenerations(ctx, o.client, o.cfg.EffectiveBaseURL(), key, model, req.Prompt, req.Width, req.Height, req.References)
	if err != nil {
		return nil, err
	}
	return &ImageResult{Data: data, MIME: mime, Provider: o.ID(), Model: model}, nil
}

// GenerateText is unsupported for gpt-image-2.
func (o *OpenAI) GenerateText(context.Context, TextRequest) (*TextResult, error) {
	return nil, ErrUnsupported
}
