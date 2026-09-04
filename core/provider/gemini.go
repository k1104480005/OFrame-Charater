package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Gemini adapter — align-framebaker-providers task 2.3.
//
// A banana/Gemini provider speaks the Google Generative Language
// "generateContent" protocol end to end and never borrows another vendor's
// wire shape (design D1: explicit protocol discriminator):
//
//   - Endpoint: POST {base}/models/{model}:generateContent (default BaseURL is
//     DefaultGeminiBaseURL from presets.go, "https://generativelanguage.
//     googleapis.com/v1beta"). The model travels in the PATH segment; it is
//     never duplicated into another vendor's body field ("model" belongs to
//     the OpenAI-compatible contracts, not this one).
//   - Auth: the API key goes ONLY in the x-goog-api-key request header. No
//     Authorization/Bearer header is added and no ?key= query fallback exists
//     (a credential embedded in a URL would end up in proxy/referrer/error
//     logs); error echoes keep the package-wide redaction so a reflected key
//     can never surface to the user.
//   - Request body: contents[].parts[] with the prompt as a text part followed
//     by every reference image as an ordered inlineData entry
//     {"mimeType","data"(base64)}. Multiple references are attached in request
//     order. Unsupported reference kinds and empty payloads are rejected
//     BEFORE any external call (清晰错误、零外呼).
//   - Response: the generated image is read from
//     candidates[0].content.parts[].inlineData.data (standard base64) with the
//     vendor's mimeType preserved. Text-only answers, blocked prompts,
//     missing/empty fields, malformed base64, non-2xx statuses (vendor error
//     envelope preferred), oversized bodies and timeouts all fail with
//     readable, secret-free errors under the same retry classes as every other
//     adapter in this package (401/403 → not-retryable).
//
// The generateContent contract has no portable pixel-size parameter, so
// Width/Height are deliberately NOT mapped onto foreign fields (no impostor
// "size" borrowed from another protocol's shape); framing intent travels in
// the prompt text itself. Video stays catalog metadata only: this adapter
// declares no video capability and performs no video external calls — 预留的
// 视频模型目录不代表可调用, matching DeclaredCapabilities(ProviderTypeGemini)
// and the global video gate (ErrCapabilityUnsupported before any network use).

const (
	// headerGeminiAPIKey carries the Gemini API key on every call. It is set
	// as a plain MIME header; HTTP header names are case-insensitive on the
	// wire and Go canonicalizes to X-Goog-Api-Key.
	headerGeminiAPIKey = "x-goog-api-key"

	// geminiGenerateContentAction completes the model resource path:
	// {base}/models/{model}:generateContent.
	geminiGenerateContentAction = ":generateContent"

	// geminiRoleUser labels the single request content turn sent by this
	// adapter (ignored for image generation, conventional for text turns).
	geminiRoleUser = "user"
)

// Reference-image kinds this adapter attaches as inlineData. The values mirror
// identity.MaterialKindReferenceImage / MaterialKindSprite; they are spelled
// locally so the provider layer stays decoupled from workspace state. Any
// other kind is unsupported here and must be rejected before an outbound call
// instead of silently dropped or mis-encoded.
const (
	geminiRefKindReferenceImage = "reference_image"
	geminiRefKindSprite         = "sprite"
)

// Gemini is the banana/Gemini protocol adapter for providers created from the
// FrameBaker banana/Gemini preset (Type ProviderTypeGemini).
type Gemini struct {
	cfg    ProviderConfig
	client *http.Client
}

// NewGemini creates the Gemini adapter (nil client → http.DefaultClient).
func NewGemini(cfg ProviderConfig, client *http.Client) *Gemini {
	return &Gemini{cfg: cfg, client: newClient(client)}
}

// ID returns the configured provider id (Gemini presets create custom,
// user-named providers, unlike the hard-coded built-in identities).
func (g *Gemini) ID() string { return g.cfg.ProviderID }

// Name returns the display name, falling back to the preset's vendor name.
func (g *Gemini) Name() string {
	if n := strings.TrimSpace(g.cfg.Name); n != "" {
		return n
	}
	return "banana / Gemini"
}

// Capabilities mirrors DeclaredCapabilities(ProviderTypeGemini): image + text
// execute today through generateContent, video stays false until a real video
// adapter exists — the reserved video catalog never implies callability.
func (g *Gemini) Capabilities() Capabilities {
	return Capabilities{Image: true, Video: false, Text: true}
}

func (g *Gemini) DefaultImageModel() string { return g.cfg.EffectiveModel() }

func (g *Gemini) DefaultTextModel() string { return g.cfg.EffectiveTextModel() }

// --- Request wire types (task 2.3: contents/parts/text + ordered inlineData) ---

// geminiInlineData is one reference-image attachment or result payload:
// standard-base64 bytes with an explicit MIME type.
type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

// geminiPart is one part of a request contents turn. Exactly one of Text /
// InlineData is populated per constructed part; omitempty keeps a text-only
// part free of an empty inlineData object.
type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inlineData,omitempty"`
}

// geminiContentRow is one contents[] turn ({role, parts}).
type geminiContentRow struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

// geminiGenerateContentRequest locks the generateContent request envelope to
// exactly its own protocol keys (no foreign size/model/response_format fields
// may creep in from the OpenAI-compatible shapes).
type geminiGenerateContentRequest struct {
	Contents []geminiContentRow `json:"contents"`
}

// geminiGenerateContentURL builds POST {base}/models/{model}:generateContent.
// No query parameters are ever appended: the key travels in the header only.
func geminiGenerateContentURL(baseURL, model string) string {
	return baseURL + "/models/" + model + geminiGenerateContentAction
}

// geminiRequestParts builds the ordered parts payload AND acts as the
// pre-flight validation boundary: the prompt becomes the leading text part,
// then every reference image follows as inlineData in request order. An
// unsupported reference kind or an empty data payload returns a marked
// not-retryable ConfigError with no outbound call having been attempted.
func geminiRequestParts(providerID, prompt string, refs []ReferenceImage) ([]geminiPart, error) {
	parts := make([]geminiPart, 0, len(refs)+1)
	if strings.TrimSpace(prompt) != "" {
		parts = append(parts, geminiPart{Text: prompt})
	}
	for i, r := range refs {
		switch r.Kind {
		case geminiRefKindReferenceImage, geminiRefKindSprite, RefKindBaseSprite:
			// attachable below
		default:
			return nil, MarkNotRetryable(configErrf(
				"provider %s: unsupported reference image #%d kind %q (only %q/%q/%q images attach as inlineData)",
				providerID, i, r.Kind, geminiRefKindReferenceImage, geminiRefKindSprite, RefKindBaseSprite))
		}
		if len(r.Data) == 0 {
			return nil, MarkNotRetryable(configErrf(
				"provider %s: reference image #%d (%s) carries no data", providerID, i, r.Kind))
		}
		parts = append(parts, geminiPart{InlineData: &geminiInlineData{
			MimeType: mimeOr(r.MIME), // blank → image/png, like the shared surface
			Data:     base64.StdEncoding.EncodeToString(r.Data),
		}})
	}
	if len(parts) == 0 {
		return nil, MarkNotRetryable(configErrf(
			"provider %s: empty generation request (no prompt text and no reference images)", providerID))
	}
	return parts, nil
}

// --- Wire execution (auth/redaction/caps/retry semantics shared with 2.1/2.2) ---

// geminiPostJSON performs ONE authenticated generateContent call and returns
// the size-capped raw body. It reuses the shared building blocks exactly like
// postJSON/postJSONWithHeaders (readCappedBody cap, isAuthStatus classes,
// non2xxDetail with redactSecret) while keeping THIS protocol's auth shape:
// x-goog-api-key header, no Bearer duplication, no query-string credentials.
func geminiPostJSON(ctx context.Context, client *http.Client, url, apiKey string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("provider: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("provider: build request: %w", err)
	}
	req.Header.Set(headerGeminiAPIKey, apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := readCappedBody(resp.Body, maxGenerationResponseBytes, "generation")
	if err != nil {
		return nil, fmt.Errorf("provider: read generation response: %w", err)
	}
	if isAuthStatus(resp.StatusCode) {
		return nil, MarkNotRetryable(fmt.Errorf(
			"provider: auth failed (HTTP %d): %s", resp.StatusCode, non2xxDetail(raw, apiKey)))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("provider: unexpected status %d: %s", resp.StatusCode, non2xxDetail(raw, apiKey))
	}
	return raw, nil
}

// --- Response types and parsing (candidates.content.parts) ---

// geminiRespInline is one response inlineData payload (camelCase on the wire).
type geminiRespInline struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

// geminiRespPart is one candidate content part. A generated image arrives as
// InlineData{MimeType,Data}; reasoning/refusal answers arrive as text parts.
type geminiRespPart struct {
	Text       string            `json:"text"`
	InlineData *geminiRespInline `json:"inlineData"`
}

type geminiRespContent struct {
	Parts []geminiRespPart `json:"parts"`
}

type geminiRespCandidate struct {
	Content      geminiRespContent `json:"content"`
	FinishReason string            `json:"finishReason"`
}

type geminiPromptFeedback struct {
	BlockReason string `json:"blockReason"`
}

// geminiResponse covers the generateContent success envelope plus the two
// failure surfaces that can appear inside otherwise-valid payloads: the
// vendor error object ("error":{"message":…}) and prompt blocking feedback.
type geminiResponse struct {
	Candidates     []geminiRespCandidate `json:"candidates"`
	PromptFeedback *geminiPromptFeedback `json:"promptFeedback"`
	Error          *apiError             `json:"error"` // shared OpenAI-compatible-shape message envelope
}

// decodeGeminiResponse unmarshals a 200 body once and applies the checks both
// image and text parsing share: decode health, in-band error envelope and
// prompt-block reporting (all with secret redaction).
func decodeGeminiResponse(raw []byte, apiKey string) (*geminiResponse, error) {
	var out geminiResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("provider: decode gemini response: %w", err)
	}
	if out.Error != nil && strings.TrimSpace(out.Error.Message) != "" {
		return nil, fmt.Errorf("provider: gemini API error: %s",
			redactSecret(strings.TrimSpace(out.Error.Message), apiKey))
	}
	if out.PromptFeedback != nil {
		if block := strings.TrimSpace(out.PromptFeedback.BlockReason); block != "" {
			return nil, fmt.Errorf("provider: gemini blocked the prompt (blockReason %q)", redactSecret(block, apiKey))
		}
	}
	return &out, nil
}

// parseGeminiImage extracts the first usable inlineData image from a
// generateContent answer: candidates[0].content.parts walk in order, skipping
// empty-data entries, decoding standard base64, and preserving the vendor's
// mimeType (blank → image/png). Every degenerate outcome reports a readable
// secret-free failure instead of returning an empty result.
func parseGeminiImage(raw []byte, apiKey string) ([]byte, string, error) {
	out, err := decodeGeminiResponse(raw, apiKey)
	if err != nil {
		return nil, "", err
	}
	if len(out.Candidates) == 0 {
		return nil, "", fmt.Errorf("provider: gemini response has no candidates")
	}
	candidate := out.Candidates[0]
	if len(candidate.Content.Parts) == 0 {
		return nil, "", fmt.Errorf("provider: gemini response candidate has no parts (finishReason %q)",
			strings.TrimSpace(candidate.FinishReason))
	}
	hasText := false
	for _, p := range candidate.Content.Parts {
		if strings.TrimSpace(p.Text) != "" {
			hasText = true
		}
		if p.InlineData == nil || strings.TrimSpace(p.InlineData.Data) == "" {
			continue // 缺字段/空数据部分不能当作成功，也不能中断对后续可用图的查找
		}
		data, derr := base64.StdEncoding.DecodeString(strings.TrimSpace(p.InlineData.Data))
		if derr != nil {
			return nil, "", fmt.Errorf("provider: decode gemini inlineData image: %w", derr)
		}
		mime := strings.TrimSpace(p.InlineData.MimeType)
		if mime == "" {
			mime = mimeOr("")
		}
		return data, mime, nil
	}
	switch {
	case hasText:
		return nil, "", fmt.Errorf(
			"provider: gemini returned a text-only response without image data (finishReason %q)",
			strings.TrimSpace(candidate.FinishReason))
	default:
		return nil, "", fmt.Errorf(
			"provider: gemini response carried no inlineData image (finishReason %q)",
			strings.TrimSpace(candidate.FinishReason))
	}
}

// parseGeminiText extracts the text answer from a generateContent completion:
// all non-empty text parts join in order (the model may split a sentence
// across several parts). Failures mirror the image parser exactly.
func parseGeminiText(raw []byte, apiKey string) (string, error) {
	out, err := decodeGeminiResponse(raw, apiKey)
	if err != nil {
		return "", err
	}
	if len(out.Candidates) == 0 {
		return "", fmt.Errorf("provider: gemini response has no candidates")
	}
	var texts []string
	for _, p := range out.Candidates[0].Content.Parts {
		if s := strings.TrimSpace(p.Text); s != "" {
			texts = append(texts, s)
		}
	}
	if len(texts) == 0 {
		return "", fmt.Errorf("provider: gemini text response has no text content")
	}
	return strings.Join(texts, "\n"), nil
}

// --- Public generation operations ---

// resolveModelFromCatalog resolves req.Model over the modality catalog head,
// failing before any external call when neither is present (same contract as
// the dashscope adapter; zero substitution beyond the documented default).
func resolveCatalogHead(reqModel string, catalog []string) (string, bool) {
	model := ResolveModel(reqModel, "")
	if model == "" && len(catalog) > 0 {
		model = catalog[0]
	}
	return model, model != ""
}

// GenerateImage generates one image through the generateContent protocol: the
// validated prompt/reference parts POST to {base}/models/{model}:
// generateContent and the first usable inlineData part becomes the result.
func (g *Gemini) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	key, err := g.cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	model, ok := resolveCatalogHead(req.Model, g.cfg.EffectiveImageModels())
	if !ok {
		return nil, MarkNotRetryable(configErrf("provider %s: no image model configured", g.ID()))
	}
	parts, err := geminiRequestParts(g.ID(), req.Prompt, req.References)
	if err != nil {
		return nil, err // already MarkNotRetryable-wrapped by the boundary
	}
	ctx, cancel := applyTimeout(ctx, g.cfg.EffectiveTimeout())
	defer cancel()

	raw, err := geminiPostJSON(ctx, g.client,
		geminiGenerateContentURL(g.cfg.EffectiveBaseURL(), model), key,
		geminiGenerateContentRequest{Contents: []geminiContentRow{{Role: geminiRoleUser, Parts: parts}}})
	if err != nil {
		return nil, err
	}
	data, mime, perr := parseGeminiImage(raw, key)
	if perr != nil {
		return nil, perr
	}
	return &ImageResult{Data: data, MIME: mime, Provider: g.ID(), Model: model}, nil
}

// GenerateText runs one text completion through the same generateContent
// endpoint: Gemini serves image and text generation from one address with one
// key (the preset ships both capabilities and shares this transport, keeping
// DeclaredCapabilities(ProviderTypeGemini) truthful end to end).
func (g *Gemini) GenerateText(ctx context.Context, req TextRequest) (*TextResult, error) {
	key, err := g.cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	model, ok := resolveCatalogHead(req.Model, g.cfg.EffectiveTextModels())
	if !ok {
		return nil, MarkNotRetryable(configErrf("provider %s: no text model configured", g.ID()))
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, MarkNotRetryable(configErrf("provider %s: empty text prompt", g.ID()))
	}
	ctx, cancel := applyTimeout(ctx, g.cfg.EffectiveTimeout())
	defer cancel()

	parts := []geminiPart{{Text: req.Prompt}}
	// Vision captioning (识图生成描述): attach the image as one inlineData
	// part after the text — same request shape the image path uses for
	// reference images.
	if uri := strings.TrimSpace(req.ImageDataURL); uri != "" {
		mime, b64, ok := strings.Cut(strings.TrimPrefix(uri, "data:"), ";base64,")
		if !ok || mime == "" || b64 == "" {
			return nil, MarkNotRetryable(configErrf("provider %s: invalid image data URL", g.ID()))
		}
		parts = append(parts, geminiPart{InlineData: &geminiInlineData{MimeType: mime, Data: b64}})
	}
	raw, err := geminiPostJSON(ctx, g.client,
		geminiGenerateContentURL(g.cfg.EffectiveBaseURL(), model), key,
		geminiGenerateContentRequest{Contents: []geminiContentRow{{Role: geminiRoleUser, Parts: parts}}})
	if err != nil {
		return nil, err
	}
	text, perr := parseGeminiText(raw, key)
	if perr != nil {
		return nil, perr
	}
	return &TextResult{Text: text, Provider: g.ID(), Model: model}, nil
}
