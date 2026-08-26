package provider

import (
	"context"
	"net/http"
)

// Agnes is the specialized supplementary adapter (专项备选). Its live
// integration is validated during P1 (generation spec 4.4: 按实际接入情况可插拔、
// 不影响默认路径) — the adapter is fully registered and switchable, and never
// affects the default Doubao path. The endpoint/model below are placeholders
// confirmed at integration time.
type Agnes struct {
	cfg    ProviderConfig
	client *http.Client
}

// NewAgnes creates the Agnes adapter.
func NewAgnes(cfg ProviderConfig, client *http.Client) *Agnes {
	return &Agnes{cfg: cfg, client: newClient(client)}
}

func (a *Agnes) ID() string { return ProviderAgnes }

func (a *Agnes) Name() string { return "Agnes (专项)" }

func (a *Agnes) Capabilities() Capabilities { return Capabilities{Image: true} }

func (a *Agnes) DefaultImageModel() string { return DefaultAgnesModel }

func (a *Agnes) DefaultTextModel() string { return "" }

// GenerateImage performs one images/generations call through the Agnes adapter.
func (a *Agnes) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	key, err := a.cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	model := ResolveModel(req.Model, a.cfg.EffectiveModel())
	data, mime, err := imagesGenerations(ctx, a.client, a.cfg.EffectiveBaseURL(), key, model, req.Prompt, req.Width, req.Height, req.References)
	if err != nil {
		return nil, err
	}
	return &ImageResult{Data: data, MIME: mime, Provider: a.ID(), Model: model}, nil
}

// GenerateText is unsupported for Agnes in phase 3.
func (a *Agnes) GenerateText(context.Context, TextRequest) (*TextResult, error) {
	return nil, ErrUnsupported
}
