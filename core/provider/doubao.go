package provider

import (
	"context"
	"net/http"
)

// newClient returns the injected client or the default one.
func newClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return http.DefaultClient
}

// Doubao is the default primary adapter (豆包 / Doubao, Ark API — OpenAI
// compatible). Supports image generation (Seedream) and text generation.
type Doubao struct {
	cfg    ProviderConfig
	client *http.Client
}

// NewDoubao creates the Doubao adapter with the given config and HTTP client
// (nil → http.DefaultClient). The config is the local contract; the key is
// resolved per call (explicit key or OFRAME_DOUBAO_API_KEY).
func NewDoubao(cfg ProviderConfig, client *http.Client) *Doubao {
	return &Doubao{cfg: cfg, client: newClient(client)}
}

func (d *Doubao) ID() string { return ProviderDoubao }

func (d *Doubao) Name() string { return "豆包 (Doubao)" }

func (d *Doubao) Capabilities() Capabilities { return Capabilities{Image: true, Text: true} }

func (d *Doubao) DefaultImageModel() string { return DefaultDoubaoModel }

func (d *Doubao) DefaultTextModel() string { return DefaultDoubaoTextModel }

// GenerateImage performs one text-to-image call (默认主力路径).
func (d *Doubao) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	key, err := d.cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	model := ResolveModel(req.Model, d.cfg.EffectiveModel())
	data, mime, err := imagesGenerations(ctx, d.client, d.cfg.EffectiveBaseURL(), key, model, req.Prompt, req.Width, req.Height, req.References)
	if err != nil {
		return nil, err
	}
	return &ImageResult{Data: data, MIME: mime, Provider: d.ID(), Model: model}, nil
}

// GenerateText performs one chat completion (text generation).
func (d *Doubao) GenerateText(ctx context.Context, req TextRequest) (*TextResult, error) {
	key, err := d.cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	model := ResolveModel(req.Model, d.cfg.EffectiveTextModel())
	text, err := chatCompletionText(ctx, d.client, d.cfg.EffectiveBaseURL(), key, model, req.Prompt)
	if err != nil {
		return nil, err
	}
	return &TextResult{Text: text, Provider: d.ID(), Model: model}, nil
}
