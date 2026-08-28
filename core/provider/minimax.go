package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// MiniMax adapter — align-framebaker-providers task 2.4.
//
// A MiniMax provider speaks the vendor's own image_generation protocol end to
// end and never borrows another protocol's wire shape (design D1: explicit
// protocol discriminator — the OpenAI-compatible surface is NOT reused):
//
//   - Endpoint: POST {base}/image_generation (default BaseURL is
//     DefaultMiniMaxBaseURL from presets.go, "https://api.minimax.chat/v1").
//   - Auth: "Authorization: Bearer <API key>" on every call (bearerAuth in
//     the official contract). Execution reuses the shared postJSON helper,
//     which also owns the unified timeout, response-size cap, the HTTP
//     401/403 not-retryable class and redacted non-2xx error detail.
//   - Request body carries at least model + prompt; n is pinned to 1 (the
//     filmstrip pipeline consumes exactly one strip per call) and
//     response_format is requested as "base64" so the answer arrives as
//     inline bytes instead of a 24h-expiring CDN URL. Size uses ONE explicit
//     field policy: exact width/height integers snapped onto the vendor's
//     pixel contract ([512,2048], multiples of 8), never aspect_ratio — the
//     vendor gives aspect_ratio priority over width/height when both travel
//     in one request, which would silently discard the caller's resolution.
//   - Reference images map to subject_reference[] entries
//     {type:"character", image_file:"data:<mime>;base64,…"} (the official i2i
//     field accepts public URLs or base64 data URLs; this adapter keeps local
//     bytes local and always sends a data URL). The API takes a SINGLE subject
//     reference, so zero or one references attach while more than one is
//     rejected BEFORE any outbound call (清晰错误、零外呼); unknown kinds and
//     empty payloads are rejected pre-flight as well.
//   - Response parsing tolerates every shape vendors/proxies actually return:
//     the official data.image_urls / data.image_base64 string arrays, plus
//     OpenAI-like data[].url / data[].b64_json / data[].image_url items and
//     flattened single-field objects. Inline bytes win over URLs so no second
//     network hop happens when none is needed.
//   - Errors honor MiniMax's NON-standard envelope: answers can carry
//     {"base_resp":{"status_code":…,"status_msg":…}} alongside HTTP 200 where
//     any non-zero status_code means failure; codes 1004/2049 are credential
//     problems and share the HTTP-401/403 not-retryable class. Every detail
//     passes through the package-wide secret redaction.
//
// Video stays catalog metadata only: Capabilities().Video is false, video
// requests are gated by ValidateVideoGeneration/ErrCapabilityUnsupported
// BEFORE any external call (预留的视频模型目录不代表可调用), and text generation
// is explicitly unsupported — the MiniMax preset declares image only.

const (
	// minimaxImageGenerationPath completes the endpoint:
	// POST {base}/image_generation.
	minimaxImageGenerationPath = "/image_generation"

	// Pixel-contract bounds of the image-01 width/height fields ([512,2048],
	// divisible by 8) documented by the vendor's t2i/i2i API.
	minimaxMinDim = 512
	minimaxMaxDim = 2048

	// The subject_reference entry type accepted for image-to-image calls;
	// the vendor currently supports exactly "character" (portrait reference).
	minimaxSubjectTypeCharacter = "character"

	// response_format value requested on every call: inline base64 keeps the
	// result self-contained (vendor URLs expire within 24 hours and would add
	// an extra download hop); URL-shaped responses are still parsed when the
	// vendor sends them anyway.
	minimaxResponseFormatBase64 = "base64"

	// Non-zero base_resp.status_code values treated as credential failures
	// (same retry class as HTTP 401/403): 1004 account authentication failed,
	// 2049 invalid API key. Every other non-zero code stays retryable.
	minimaxStatusAuthFailed    = 1004
	minimaxStatusInvalidAPIKey = 2049
)

// Reference-image kinds this adapter attaches as subject_reference. The values
// mirror identity.MaterialKindReferenceImage / MaterialKindSprite; they are
// spelled locally so the provider layer stays decoupled from workspace state.
// Any other kind is rejected before an outbound call instead of silently
// dropped or mis-encoded.
const (
	minimaxRefKindReferenceImage = "reference_image"
	minimaxRefKindSprite         = "sprite"
)

// MiniMax is the MiniMax image protocol adapter for providers created from the
// FrameBaker MiniMax preset (Type ProviderTypeMiniMax).
type MiniMax struct {
	cfg    ProviderConfig
	client *http.Client
}

// NewMiniMax creates the MiniMax adapter (nil client → http.DefaultClient).
func NewMiniMax(cfg ProviderConfig, client *http.Client) *MiniMax {
	return &MiniMax{cfg: cfg, client: newClient(client)}
}

// ID returns the configured provider id (MiniMax presets create custom,
// user-named providers, unlike the hard-coded built-in identities).
func (m *MiniMax) ID() string { return m.cfg.ProviderID }

// Name returns the display name, falling back to the preset's vendor name.
func (m *MiniMax) Name() string {
	if n := strings.TrimSpace(m.cfg.Name); n != "" {
		return n
	}
	return "MiniMax"
}

// Capabilities mirrors DeclaredCapabilities(ProviderTypeMiniMax): image only.
// Video stays false until a real video adapter exists — the reserved video
// catalog never implies callability — and text endpoints are simply absent
// from this protocol, so Text stays false too.
func (m *MiniMax) Capabilities() Capabilities {
	return Capabilities{Image: true}
}

func (m *MiniMax) DefaultImageModel() string { return m.cfg.EffectiveModel() }

func (m *MiniMax) DefaultTextModel() string { return "" }

// --- Size mapping (single explicit field policy: exact width/height) ---

// minimaxSize snaps a requested W×H onto the image-01 pixel contract:
// unset/non-positive dimensions fall back to DefaultGenerationSize (square
// 1024 like every other adapter in this package), then each dimension clamps
// to [512,2048] and rounds to the nearest multiple of 8. Snap-before-send is
// deterministic and stated up front: the vendor rejects out-of-range or
// non-aligned integers outright, and never receives aspect_ratio here because
// it outranks width/height when both travel in one request.
func minimaxSize(w, h int) (int, int) {
	if w <= 0 {
		w = DefaultGenerationSize
	}
	if h <= 0 {
		h = DefaultGenerationSize
	}
	return minimaxSnapDim(w), minimaxSnapDim(h)
}

func minimaxSnapDim(v int) int {
	if v < minimaxMinDim {
		v = minimaxMinDim
	}
	if v > minimaxMaxDim {
		v = minimaxMaxDim
	}
	v = ((v + 4) / 8) * 8 // round to nearest multiple of 8
	if v < minimaxMinDim {
		v = minimaxMinDim
	}
	if v > minimaxMaxDim {
		v = minimaxMaxDim
	}
	return v
}

// --- Request wire types ---

// minimaxSubjectReference is one subject_reference[] entry. image_file accepts
// a public URL or a base64 data URL; the adapter always sends a data URL built
// from the attached local bytes so reference material never needs to be
// uploaded elsewhere first.
type minimaxSubjectReference struct {
	Type      string `json:"type"`
	ImageFile string `json:"image_file"`
}

// minimaxImageGenerationRequest locks the request envelope to exactly its own
// protocol keys: model/prompt (required), exact pixel size, pinned n=1,
// inline-base64 responses and the optional single-subject reference list.
// Foreign fields (aspect_ratio, "size":"WxH", response_format:"png",
// reference_images…) deliberately do not exist here — that would be another
// protocol's shape wearing this adapter's name.
type minimaxImageGenerationRequest struct {
	Model            string                    `json:"model"`
	Prompt           string                    `json:"prompt"`
	Width            int                       `json:"width,omitempty"`
	Height           int                       `json:"height,omitempty"`
	N                int                       `json:"n,omitempty"`
	ResponseFormat   string                    `json:"response_format,omitempty"`
	SubjectReference []minimaxSubjectReference `json:"subject_reference,omitempty"`
}

// minimaxGenerateURL builds POST {base}/image_generation.
func minimaxGenerateURL(baseURL string) string {
	return baseURL + minimaxImageGenerationPath
}

// minimaxSubjectReferences validates the attached reference images against the
// vendor's single-subject contract and builds the request entries. Zero
// references yield nil (纯文生图); exactly one yields one character entry as a
// base64 data URL. More than one reference, unknown kinds and empty payloads
// all fail with marked-not-retryable ConfigErrors BEFORE any external call.
func minimaxSubjectReferences(providerID string, refs []ReferenceImage) ([]minimaxSubjectReference, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	if len(refs) > 1 {
		return nil, MarkNotRetryable(configErrf(
			"provider %s (minimax): %d reference images attached but the MiniMax subject_reference API carries exactly one; send only the primary reference image",
			providerID, len(refs)))
	}
	r := refs[0]
	switch r.Kind {
	case minimaxRefKindReferenceImage, minimaxRefKindSprite:
		// attachable below
	default:
		return nil, MarkNotRetryable(configErrf(
			"provider %s: unsupported reference image kind %q (only %q/%q images attach as subject_reference)",
			providerID, r.Kind, minimaxRefKindReferenceImage, minimaxRefKindSprite))
	}
	if len(r.Data) == 0 {
		return nil, MarkNotRetryable(configErrf(
			"provider %s: reference image (%s) carries no data", providerID, r.Kind))
	}
	return []minimaxSubjectReference{{
		Type:      minimaxSubjectTypeCharacter,
		ImageFile: "data:" + mimeOr(r.MIME) + ";base64," + base64.StdEncoding.EncodeToString(r.Data),
	}}, nil
}

// --- Response types and parsing ---

// minimaxBaseResp is MiniMax's non-standard status envelope. Unlike most
// vendors, failures arrive INSIDE otherwise-normal JSON (typically alongside
// HTTP 200): any non-zero status_code means failure and status_msg explains
// why. A missing base_resp decodes to zero-value success — absent means OK.
type minimaxBaseResp struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

// minimaxDataItem is one tolerated array entry under data[]: the OpenAI-like
// shapes reshaped by gateways/proxies that sit in front of the vendor API.
type minimaxDataItem struct {
	URL        string `json:"url"`
	B64JSON    string `json:"b64_json"`
	ImageURL   string `json:"image_url"`
	ImageB64   string `json:"image_base64"`
	B64Alt     string `json:"b64"`            // 另一种常见别名（宽容解析）
	ImageB64AJ string `json:"image_b64_json"` // 再一种内联别名（宽容解析）
}

// minimaxDataObject is the OFFICIAL data object plus its tolerated flattened
// variants: image_urls/image_base64 string arrays, or single url/b64_json/
// image_url string fields on the object itself.
type minimaxDataObject struct {
	ImageURLs   []string `json:"image_urls"`
	ImageBase64 []string `json:"image_base64"`
	URL         string   `json:"url"`
	B64JSON     string   `json:"b64_json"`
	ImageURLOne string   `json:"image_url"`
}

// minimaxGenerationResponse covers the success envelope plus both failure
// surfaces: the vendor's base_resp{status_code,status_msg} and an
// OpenAI-compatible-shaped error.message some gateways inject instead.
type minimaxGenerationResponse struct {
	Data     json.RawMessage  `json:"data"`
	BaseResp *minimaxBaseResp `json:"base_resp"`
	Error    *apiError        `json:"error"`
	ID       string           `json:"id"`
}

// decodeMiniMaxResponse unmarshals a response body once and applies the
// checks shared by every call path: decode health, in-band error.message,
// then the base_resp status classes (1004/2049 → not-retryable credential
// failures mirroring the HTTP 401/403 class; every other non-zero code →
// readable retryable failure). All details stay secret-free via redactSecret.
func decodeMiniMaxResponse(raw []byte, apiKey string) (*minimaxGenerationResponse, error) {
	var out minimaxGenerationResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("provider: decode minimax response: %w", err)
	}
	if out.Error != nil && strings.TrimSpace(out.Error.Message) != "" {
		return nil, fmt.Errorf("provider: minimax API error: %s",
			redactSecret(strings.TrimSpace(out.Error.Message), apiKey))
	}
	code := 0
	msg := ""
	if out.BaseResp != nil {
		code = out.BaseResp.StatusCode
		msg = strings.TrimSpace(out.BaseResp.StatusMsg)
	}
	if code == 0 {
		return &out, nil
	}
	detail := msg
	if detail == "" {
		detail = "(vendor gave no reason)"
	} else {
		detail = redactSecret(detail, apiKey)
	}
	err := fmt.Errorf("provider: minimax API error (status_code %d): %s",
		code, truncate(detail, 300))
	switch code {
	case minimaxStatusAuthFailed, minimaxStatusInvalidAPIKey:
		return nil, MarkNotRetryable(err)
	}
	return nil, err
}

// splitMiniMaxDataURL recognizes a strict "data:<mime>;base64,<payload>" data
// URL and splits off its parts. Everything else (raw base64 payloads,
// http(s) links, non-base64 data URLs) yields ok=false.
func splitMiniMaxDataURL(s string) (mime, payload string, ok bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(strings.ToLower(s), "data:") {
		return "", "", false
	}
	body := s[len("data:"):]
	comma := strings.Index(body, ",")
	if comma < 0 {
		return "", "", false
	}
	header := body[:comma]
	idx := strings.LastIndex(strings.ToLower(header), ";base64")
	if idx < 0 || strings.TrimSpace(body[comma+1:]) == "" {
		return "", "", false
	}
	return strings.TrimSpace(header[:idx]), body[comma+1:], true
}

// minimaxInlinePayload classifies one vendor-supplied string that is SUPPOSED
// to carry inline bytes: strict base64 data-URL prefixes win their MIME,
// anything else is treated as raw standard base64.
func minimaxInlinePayload(s string) (mime, payload string, ok bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", "", false
	}
	if m, p, isData := splitMiniMaxDataURL(trimmed); isData {
		return m, p, true
	}
	return "", trimmed, true
}

// minimaxCandidate is one usable image candidate collected from whatever
// shape the answer arrived in. Classification follows SOURCE FIELD NAMES:
// base64-named fields always hold inline bytes; url-named fields hold links,
// except explicit "data:<mime>;base64,…" payloads, which stay inline wherever
// they surface.
type minimaxCandidate struct {
	mime   string
	base64 string
	url    string
}

// collectCandidates walks a parsed data payload across its tolerated shapes —
// the official string-array members, OpenAI-like item-array members and
// flattened single fields — collecting EVERY inline-base64 candidate BEFORE
// any URL candidate, so mixed answers prefer direct bytes and never make a
// second network hop when none is needed. Order inside each group follows
// field order deterministically.
func collectCandidates(items []minimaxDataItem, obj minimaxDataObject, isArray bool) []minimaxCandidate {
	var b64s, urls []minimaxCandidate

	pushInline := func(v string) {
		if m, p, ok := minimaxInlinePayload(v); ok {
			b64s = append(b64s, minimaxCandidate{base64: p, mime: m})
		}
	}
	pushURLish := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if m, p, ok := splitMiniMaxDataURL(v); ok {
			b64s = append(b64s, minimaxCandidate{base64: p, mime: m})
			return
		}
		urls = append(urls, minimaxCandidate{url: v})
	}

	if isArray {
		for _, it := range items {
			for _, v := range []string{it.B64JSON, it.B64Alt, it.ImageB64, it.ImageB64AJ} {
				if strings.TrimSpace(v) != "" {
					pushInline(v)
					break // 第一个命中的内联字段即代表该条目
				}
			}
			for _, v := range []string{it.URL, it.ImageURL} {
				if strings.TrimSpace(v) != "" {
					pushURLish(v)
					break
				}
			}
		}
		return append(b64s, urls...)
	}

	for _, v := range obj.ImageBase64 {
		pushInline(v)
	}
	pushInline(obj.B64JSON)
	for _, v := range obj.ImageURLs {
		pushURLish(v)
	}
	pushURLish(obj.ImageURLOne)
	pushURLish(obj.URL)
	return append(b64s, urls...)
}

// parseMiniMaxImage extracts the generated image bytes from a decoded
// response: it understands the official data.image_urls/image_base64 string
// arrays, OpenAI-like data[].url/b64_json(/image_url) item arrays and
// flattened single-field objects. Base64 candidates decode directly (optional
// data-URL prefix preserved into the MIME; broken entries never abort the
// search); URLs download through the SHARED fetch helper (no Authorization
// toward foreign hosts, same response-size cap). Anything else fails with a
// readable secret-free error.
func parseMiniMaxImage(env *minimaxGenerationResponse, ctx context.Context, client *http.Client) ([]byte, string, error) {
	data := strings.TrimSpace(string(env.Data))
	if data == "" || data == "null" {
		return nil, "", fmt.Errorf("provider: minimax response has no image data")
	}
	var (
		items   []minimaxDataItem
		obj     minimaxDataObject
		isArray bool
	)
	switch data[0] {
	case '[':
		isArray = true
		if err := json.Unmarshal(env.Data, &items); err != nil {
			return nil, "", fmt.Errorf("provider: decode minimax data array: %w", err)
		}
	case '{':
		if err := json.Unmarshal(env.Data, &obj); err != nil {
			return nil, "", fmt.Errorf("provider: decode minimax data object: %w", err)
		}
	default:
		return nil, "", fmt.Errorf("provider: minimax data payload has unexpected shape (%s…)", truncate(data, 24))
	}

	var lastDecodeErr error
	for _, c := range collectCandidates(items, obj, isArray) {
		switch {
		case c.base64 != "":
			decoded, err := base64.StdEncoding.DecodeString(c.base64)
			if err != nil {
				lastDecodeErr = err
				continue // 坏条目不终止查找，向后继续找可用候选
			}
			return decoded, mimeOr(c.mime), nil
		case c.url != "":
			fetched, fmime, ferr := fetchGeneratedImage(ctx, client, c.url)
			if ferr != nil {
				return nil, "", ferr
			}
			return fetched, mimeOr(fmime), nil
		}
	}
	if lastDecodeErr != nil {
		return nil, "", fmt.Errorf("provider: decode minimax base64 image: %w", lastDecodeErr)
	}
	return nil, "", fmt.Errorf("provider: minimax response carried no usable image")
}

// --- Public generation operations ---

// GenerateImage generates one image through the MiniMax image_generation
// protocol. All pre-flight validation (key presence, model catalog, prompt,
// reference-image rules) happens BEFORE any external call; execution shares
// the unified auth/timeout/response-cap/error layer through postJSON.
func (m *MiniMax) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	key, err := m.cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	model, ok := resolveCatalogHead(req.Model, m.cfg.EffectiveImageModels())
	if !ok {
		return nil, MarkNotRetryable(configErrf("provider %s: no image model configured", m.ID()))
	}
	prompt := req.Prompt
	if strings.TrimSpace(prompt) == "" {
		return nil, MarkNotRetryable(configErrf(
			"provider %s: empty generation request (the minimax image protocol requires a prompt)", m.ID()))
	}
	refs, err := minimaxSubjectReferences(m.ID(), req.References)
	if err != nil {
		return nil, err // already MarkNotRetryable-wrapped by the boundary
	}
	width, height := minimaxSize(req.Width, req.Height)
	ctx, cancel := applyTimeout(ctx, m.cfg.EffectiveTimeout())
	defer cancel()

	raw, err := postJSON(ctx, m.client, minimaxGenerateURL(m.cfg.EffectiveBaseURL()), key,
		minimaxImageGenerationRequest{
			Model:            model,
			Prompt:           prompt,
			Width:            width,
			Height:           height,
			N:                1,
			ResponseFormat:   minimaxResponseFormatBase64,
			SubjectReference: refs,
		})
	if err != nil {
		return nil, err
	}
	env, perr := decodeMiniMaxResponse(raw, key)
	if perr != nil {
		return nil, perr
	}
	data, mime, perr := parseMiniMaxImage(env, ctx, m.client)
	if perr != nil {
		return nil, perr
	}
	return &ImageResult{Data: data, MIME: mime, Provider: m.ID(), Model: model}, nil
}

// GenerateText is unsupported: DeclaredCapabilities(ProviderTypeMiniMax)
// declares image only, the preset ships no text model, and inventing a chat
// surface would silently apply another protocol to this vendor. The refusal
// is purely local (zero external calls) and remains discoverable through both
// capability sentinels used across this package.
func (m *MiniMax) GenerateText(context.Context, TextRequest) (*TextResult, error) {
	return nil, MarkNotRetryable(fmt.Errorf(
		"provider %s does not support text generation (the minimax adapter implements the image_generation protocol only): %w; %w",
		m.ID(), ErrCapabilityUnsupported, ErrUnsupported))
}
