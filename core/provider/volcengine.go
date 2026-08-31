package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Volcengine (火山方舟 / 豆包 Ark) adapter — align-framebaker-providers task 2.5.
//
// A volcengine provider speaks the Ark v3 protocol end to end and never
// borrows another vendor's wire shape (design D1: explicit protocol
// discriminator — 协议身份独立，不静默套用其他协议):
//
//   - Endpoint: Seedream image requests POST {base}/images/generations
//     (default BaseURL is the preset's DefaultDoubaoBaseURL,
//     "https://ark.cn-beijing.volces.com/api/v3" — Ark 自己的 API root, not a
//     DashScope/compatible-mode address). Text requests POST
//     {base}/chat/completions: Ark v3 hosts its Doubao LLMs behind that
//     VENDOR-OWNED chat surface by design — this adapter calls it explicitly
//     under its own identity and fake transport locks the wire; routing the
//     adapter type stays "volcengine", never "compatible".
//   - Auth: "Authorization: Bearer <API key>" on every call through the shared
//     newAuthedRequest/postJSON layer, which also owns the unified timeout,
//     response-size caps, HTTP 401/403 not-retryable classification and
//     redacted non-2xx error detail. No x-goog-api-key header, no
//     X-DashScope-Async header, no other vendor's headers exist here.
//   - Request body carries exactly the Ark contract keys: model/prompt/size
//     ("WxH" — Width/Height map to strings like "1024x1024"), plus
//     response_format pinned to "b64_json" so the answer arrives as inline
//     bytes instead of an expiring CDN URL, and watermark explicitly false —
//     generated strips feed pixel-level slicing/acceptance checks, so the
//     vendor's default watermark must never silently stamp them.
//   - Reference images follow the Seedream i2i convention: the "image" field
//     accepts public URLs or base64 data URLs (string array for multi-image
//     fusion). This adapter keeps local bytes local and always sends data
//     URLs built from ReferenceImage.Data in request order; no role objects
//     are attached because the Ark field is a plain string list. Unknown
//     kinds and empty payloads are rejected BEFORE any outbound call (清晰
//     错误、零外呼).
//   - Response parsing reads the common shapes vendors/gateways return:
//     official data[].b64_json / data[].url item arrays, plus flattened
//     single-object data payloads. Inline bytes win over URLs so no second
//     network hop happens when none is needed; URL downloads reuse the shared
//     fetchGeneratedImage helper (no Authorization toward foreign hosts, same
//     response cap).
//   - Errors stay readable and secret-free: the OpenAI-compatible error
//     envelope is preferred over raw body echoes and every detail passes
//     redactSecret; 401/403 stop immediately as credential failures.
//
// Video stays catalog metadata only: Capabilities().Video is false, video
// requests are gated by ValidateVideoGeneration/ErrCapabilityUnsupported
// BEFORE any external call (预留的视频模型目录不代表可调用). Text generation is a
// real capability of the preset's Doubao LLM catalogs and executes against the
// endpoint above.

const (
	// arkImagesGenerationsPath completes the Seedream endpoint:
	// POST {base}/images/generations.
	arkImagesGenerationsPath = "/images/generations"

	// response_format value requested on every Seedream call: inline base64
	// keeps the result self-contained and deterministic; url-shaped answers
	// are still parsed when a gateway returns them anyway.
	arkResponseFormatB64JSON = "b64_json"
)

// Reference-image kinds this adapter attaches under the Ark "image" field.
// The values mirror identity.MaterialKindReferenceImage / MaterialKindSprite;
// they are spelled locally so the provider layer stays decoupled from
// workspace state. Any other kind is rejected before an outbound call instead
// of silently dropped or mis-encoded.
const (
	arkRefKindReferenceImage = "reference_image"
	arkRefKindSprite         = "sprite"
)

// Volcengine is the 火山方舟/豆包 Ark protocol adapter for providers created
// from the FrameBaker 火山方舟 preset (Type ProviderTypeVolcengine).
type Volcengine struct {
	cfg    ProviderConfig
	client *http.Client
}

// NewVolcengine creates the Ark adapter (nil client → http.DefaultClient).
func NewVolcengine(cfg ProviderConfig, client *http.Client) *Volcengine {
	return &Volcengine{cfg: cfg, client: newClient(client)}
}

// ID returns the configured provider id (volcengine presets create custom,
// user-named providers, unlike the hard-coded built-in identities).
func (v *Volcengine) ID() string { return v.cfg.ProviderID }

// Name returns the display name, falling back to the preset's vendor name.
func (v *Volcengine) Name() string {
	if n := strings.TrimSpace(v.cfg.Name); n != "" {
		return n
	}
	return "火山方舟 / 豆包"
}

// Capabilities mirrors DeclaredCapabilities(ProviderTypeVolcengine): image +
// text execute today (Seedream images via /images/generations, Doubao text
// via Ark's own /chat/completions), video stays false until a real video
// adapter exists — the reserved Seedance catalog never implies callability.
func (v *Volcengine) Capabilities() Capabilities {
	return Capabilities{Image: true, Video: false, Text: true}
}

func (v *Volcengine) DefaultImageModel() string { return v.cfg.EffectiveModel() }

func (v *Volcengine) DefaultTextModel() string { return v.cfg.EffectiveTextModel() }

// --- Request wire types (task 2.5: model/prompt/size + Seedream 引用图) ---

// arkImageGenerationRequest locks the request envelope to exactly the Ark
// images/generations contract keys. Foreign fields (the compatible surface's
// reference_images objects with role wrappers, DashScope-native
// input.parameters blocks, MiniMax subject_reference entries, aspect_ratio…)
// deliberately do not exist here — that would be another protocol's shape
// wearing this adapter's name.
type arkImageGenerationRequest struct {
	Model  string `json:"model"`  // required by the vendor contract
	Prompt string `json:"prompt"` // required by the vendor contract
	Size   string `json:"size"`   // "WxH" (custom size), e.g. "1024x1024"

	// Image lists the Seedream i2i references: public URLs or base64 data
	// URLs, fused in order. Absent for pure text-to-image calls.
	Image []string `json:"image,omitempty"`

	ResponseFormat string `json:"response_format,omitempty"`
	Watermark      bool   `json:"watermark"`
}

// arkGenerateURL builds POST {base}/images/generations.
func arkGenerateURL(baseURL string) string {
	return baseURL + arkImagesGenerationsPath
}

// arkImageSize maps Width/Height onto the Ark "size":"WxH" convention:
// unset/non-positive dimensions fall back to DefaultGenerationSize (square
// 1024 like every adapter in this package); otherwise the caller's exact
// integers travel verbatim — no silent clamping on either axis.
func arkImageSize(w, h int) string {
	if w <= 0 {
		w = DefaultGenerationSize
	}
	if h <= 0 {
		h = DefaultGenerationSize
	}
	return fmt.Sprintf("%dx%d", w, h)
}

// arkReferenceImages validates the attached reference images against the
// Seedream i2i rules and builds the "image" string list. Zero references
// yield nil (纯文生图); otherwise every reference becomes one base64 data URL
// entry in request order. The Ark field accepts URL strings too, but local
// ReferenceImage input only ever carries Data bytes — keeping them local
// means no extra upload hop and no third-party host involved. Unknown kinds
// and empty payloads fail with marked-not-retryable ConfigErrors BEFORE any
// external call; multiple references are allowed (Seedream multi-image
// fusion — unlike MiniMax's single-subject rule).
func arkReferenceImages(providerID string, refs []ReferenceImage) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	imgs := make([]string, 0, len(refs))
	for _, r := range refs {
		switch r.Kind {
		case arkRefKindReferenceImage, arkRefKindSprite:
			// attachable below
		default:
			return nil, MarkNotRetryable(configErrf(
				"provider %s: unsupported reference image kind %q (only %q/%q images attach to the Ark image field)",
				providerID, r.Kind, arkRefKindReferenceImage, arkRefKindSprite))
		}
		if len(r.Data) == 0 {
			return nil, MarkNotRetryable(configErrf(
				"provider %s: reference image (%s) carries no data", providerID, r.Kind))
		}
		imgs = append(imgs, "data:"+mimeOr(r.MIME)+";base64,"+base64.StdEncoding.EncodeToString(r.Data))
	}
	return imgs, nil
}

// --- Response types and parsing ---

// arkImageItem is one tolerated data[] entry of the images response: inline
// b64_json bytes or a CDN url. size mirrors what the vendor reports.
type arkImageItem struct {
	URL     string `json:"url"`
	B64JSON string `json:"b64_json"`
	Size    string `json:"size"`
}

// arkImagesEnvelope covers the success envelope plus the error surface some
// gateways inject into otherwise-normal 200 bodies.
type arkImagesEnvelope struct {
	Model string          `json:"model"`
	Data  json.RawMessage `json:"data"`
	Error *apiError       `json:"error"`
}

// parseArkImage extracts the generated image bytes from an images response:
// it understands the official data[] item arrays (b64_json preferred over url
// per item, broken entries never abort the search) and flattened single-field
// data objects. Base64 candidates decode directly; URLs download through the
// SHARED fetch helper (no Authorization toward foreign hosts, same
// response-size cap). Anything else fails with a readable secret-free error.
func parseArkImage(raw []byte, apiKey string, ctx context.Context, client *http.Client) ([]byte, string, error) {
	var env arkImagesEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, "", fmt.Errorf("provider: decode ark images response: %w", err)
	}
	if env.Error != nil && strings.TrimSpace(env.Error.Message) != "" {
		return nil, "", fmt.Errorf("provider: ark images API error: %s",
			redactSecret(strings.TrimSpace(env.Error.Message), apiKey))
	}
	data := strings.TrimSpace(string(env.Data))
	if data == "" || data == "null" {
		return nil, "", fmt.Errorf("provider: ark response has no image data")
	}

	var items []arkImageItem
	switch data[0] {
	case '[':
		if err := json.Unmarshal(env.Data, &items); err != nil {
			return nil, "", fmt.Errorf("provider: decode ark data array: %w", err)
		}
	case '{':
		var one arkImageItem
		if err := json.Unmarshal(env.Data, &one); err != nil {
			return nil, "", fmt.Errorf("provider: decode ark data object: %w", err)
		}
		items = []arkImageItem{one}
	default:
		return nil, "", fmt.Errorf("provider: ark data payload has unexpected shape (%s…)", truncate(data, 24))
	}

	var lastDecodeErr error
	for _, it := range items {
		switch {
		case strings.TrimSpace(it.B64JSON) != "":
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(it.B64JSON))
			if err != nil {
				lastDecodeErr = err
				continue // 坏条目不终止查找，向后继续找可用候选
			}
			return decoded, mimeOr(""), nil
		case strings.TrimSpace(it.URL) != "":
			fetched, fmime, ferr := fetchGeneratedImage(ctx, client, strings.TrimSpace(it.URL))
			if ferr != nil {
				return nil, "", ferr
			}
			return fetched, mimeOr(fmime), nil
		}
	}
	if lastDecodeErr != nil {
		return nil, "", fmt.Errorf("provider: decode ark b64 image: %w", lastDecodeErr)
	}
	return nil, "", fmt.Errorf("provider: ark response carried no usable image")
}

// --- Public generation operations ---

// GenerateImage generates one Seedream image through the Ark
// images/generations protocol. All pre-flight validation (key presence, model
// catalog, prompt, reference-image rules) happens BEFORE any external call;
// execution shares the unified auth/timeout/response-cap/error layer through
// postJSON.
func (v *Volcengine) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	key, err := v.cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	model, ok := resolveCatalogHead(req.Model, v.cfg.EffectiveImageModels())
	if !ok {
		return nil, MarkNotRetryable(configErrf("provider %s: no image model configured", v.ID()))
	}
	prompt := req.Prompt
	if strings.TrimSpace(prompt) == "" {
		return nil, MarkNotRetryable(configErrf(
			"provider %s: empty generation request (the Ark images/generations protocol requires a prompt)", v.ID()))
	}
	imgs, err := arkReferenceImages(v.ID(), req.References)
	if err != nil {
		return nil, err // already MarkNotRetryable-wrapped by the boundary
	}
	ctx, cancel := applyTimeout(ctx, v.cfg.EffectiveTimeout())
	defer cancel()

	raw, err := postJSON(ctx, v.client, arkGenerateURL(v.cfg.EffectiveBaseURL()), key,
		arkImageGenerationRequest{
			Model:          model,
			Prompt:         prompt,
			Size:           arkImageSize(req.Width, req.Height),
			Image:          imgs,
			ResponseFormat: arkResponseFormatB64JSON,
			Watermark:      false, // 生成的分镜条要进像素级切片与验收，厂商默认水印必须显式关闭
		})
	if err != nil {
		return nil, err
	}
	data, mime, perr := parseArkImage(raw, key, ctx, v.client)
	if perr != nil {
		return nil, perr
	}
	return &ImageResult{Data: data, MIME: mime, Provider: v.ID(), Model: model}, nil
}

// GenerateText runs one text completion through Ark's own chat surface: the
// doubao text models shipped with the preset live behind POST
// {base}/chat/completions on Ark v3, so calling THAT path is this protocol's
// native behavior — not another provider type being applied. The shared
// chatCompletionText helper supplies the identical auth/response-cap/error
// classes every other call in this package uses.
func (v *Volcengine) GenerateText(ctx context.Context, req TextRequest) (*TextResult, error) {
	key, err := v.cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	model, ok := resolveCatalogHead(req.Model, v.cfg.EffectiveTextModels())
	if !ok {
		return nil, MarkNotRetryable(configErrf("provider %s: no text model configured", v.ID()))
	}
	ctx, cancel := applyTimeout(ctx, v.cfg.EffectiveTimeout())
	defer cancel()
	// chatCompletionText posts {"model","messages":[user]} to
	// {base}/chat/completions — exactly Ark's own Doubao-LLM endpoint — with
	// the shared auth/response-cap/error layer underneath.
	text, err := chatCompletionText(ctx, v.client, v.cfg.EffectiveBaseURL(), key, model, req.Prompt, req.ImageDataURL)
	if err != nil {
		return nil, err
	}
	return &TextResult{Text: text, Provider: v.ID(), Model: model}, nil
}
