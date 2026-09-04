package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Shared response-size caps for the OpenAI-compatible endpoints (task 2.1:
// 统一认证、超时、响应大小限制和错误解析). Generated-image payloads arrive as
// base64 inside JSON and are large; model catalogs are small lists. Both caps
// stop runaway vendor responses from exhausting memory instead of dying with
// an unreadable decoder failure. They are variables only so the fake-transport
// tests can exercise the boundary with tiny payloads; production keeps the
// defaults below.
var (
	maxGenerationResponseBytes = int64(64 << 20) // images/generations + chat/completions
	maxModelListBytes          = int64(8 << 20)  // GET /models catalog
)

// The adapters speak the OpenAI-compatible images/generations and
// chat/completions contracts. Reference images are attached as the
// vendor-extension field "reference_images" (base64 data URLs); the exact
// vendor field names are confirmed during live integration (design Open
// Questions) — the fake-transport tests pin the phase-3 contract.

// apiError is the OpenAI-compatible error envelope.
type apiError struct {
	Message string `json:"message"`
}

type imageGenResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
	Error *apiError `json:"error"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *apiError `json:"error"`
}

// apiErrorMessage extracts the vendor-provided error message from an
// OpenAI-compatible "error":{"message":…} envelope when one is present.
func apiErrorMessage(raw []byte) (string, bool) {
	var env struct {
		Error *apiError `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", false
	}
	if env.Error == nil || env.Error.Message == "" {
		return "", false
	}
	return strings.TrimSpace(env.Error.Message), true
}

// decodeJSONResponse decodes a provider response and adds a useful diagnostic
// when an HTTP 200 endpoint returns a web page or another non-JSON payload.
func decodeJSONResponse(raw []byte, target any, kind, apiKey string) error {
	if err := json.Unmarshal(raw, target); err != nil {
		body := strings.TrimSpace(string(raw))
		if body == "" {
			return fmt.Errorf("provider: decode %s response: empty response body", kind)
		}
		preview := truncate(redactSecret(body, apiKey), 300)
		if strings.HasPrefix(body, "<") {
			path := kind
			if kind == "chat" {
				path = "chat/completions"
			}
			return fmt.Errorf("provider: decode %s response: endpoint returned HTML instead of JSON (%s); check that Base URL points to the provider API root, for example http://127.0.0.1:11434/v1, and that /%s is an OpenAI-compatible endpoint", kind, preview, path)
		}
		return fmt.Errorf("provider: decode %s response: invalid JSON (%s): %w", kind, preview, err)
	}
	return nil
}

// redactSecret removes every occurrence of secret from s. Vendor bodies are
// echoed into user-facing errors in truncated form, so a service reflecting
// the Authorization header back must never leak the key through them.
func redactSecret(s, secret string) string {
	if secret == "" || !strings.Contains(s, secret) {
		return s
	}
	return strings.ReplaceAll(s, secret, "***")
}

// non2xxDetail builds the readable detail line of a non-2xx response: the
// vendor's envelope message when present, otherwise a truncated echo of the
// body, or a placeholder when the body is empty. The API key is redacted in
// either case (errors stay readable without leaking credentials).
func non2xxDetail(raw []byte, apiKey string) string {
	if msg, ok := apiErrorMessage(raw); ok {
		return redactSecret(msg, apiKey)
	}
	if s := strings.TrimSpace(string(raw)); s != "" {
		return truncate(redactSecret(s, apiKey), 300)
	}
	return "(no response body)"
}

// isAuthStatus reports whether a status is a credential problem (marked
// not-retryable: retrying cannot fix a wrong or missing key).
func isAuthStatus(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden
}

// readCappedBody reads at most maxBytes of r. Reading one byte beyond the cap
// turns a silently-truncated payload (which would later fail as
// "unexpected end of JSON input") into an immediate, readable size-limit error.
func readCappedBody(r io.Reader, maxBytes int64, kind string) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("%s response too large (over %d MiB)", kind, maxBytes>>20)
	}
	return raw, nil
}

// newAuthedRequest builds an authenticated provider request (统一认证): the
// Bearer header on every call, JSON content type only when a body is sent,
// and Accept: application/json always.
func newAuthedRequest(ctx context.Context, method, url, apiKey string, jsonBody []byte) (*http.Request, error) {
	var rdr io.Reader
	if jsonBody != nil {
		rdr = bytes.NewReader(jsonBody)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, fmt.Errorf("provider: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	if jsonBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// sendAPI performs one authenticated provider request and returns the
// size-capped body plus the status code. Transport failures, response-size
// enforcement and header handling live here so every OpenAI-compatible call
// behaves identically (design D4: shared helpers own auth/response caps;
// adapters own endpoints, bodies and parsing).
func sendAPI(ctx context.Context, client *http.Client, method, url, apiKey string, jsonBody []byte, maxBytes int64, kind string) ([]byte, int, error) {
	req, err := newAuthedRequest(ctx, method, url, apiKey, jsonBody)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("provider: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := readCappedBody(resp.Body, maxBytes, kind)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("provider: read %s response: %w", kind, err)
	}
	return raw, resp.StatusCode, nil
}

// postJSON performs an authenticated JSON POST and returns the raw response
// body. HTTP 401/403 are marked not-retryable; other non-2xx responses carry
// the vendor error message (envelope preferred over a body dump).
func postJSON(ctx context.Context, client *http.Client, url, apiKey string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("provider: encode request: %w", err)
	}
	raw, status, err := sendAPI(ctx, client, http.MethodPost, url, apiKey, data, maxGenerationResponseBytes, "generation")
	if err != nil {
		return nil, err
	}
	if isAuthStatus(status) {
		return nil, MarkNotRetryable(fmt.Errorf("provider: auth failed (HTTP %d): %s", status, non2xxDetail(raw, apiKey)))
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("provider: unexpected status %d: %s", status, non2xxDetail(raw, apiKey))
	}
	return raw, nil
}

// imagesGenerations performs one images/generations call and decodes the
// resulting image bytes (b64_json, or url fetched via GET).
func imagesGenerations(ctx context.Context, client *http.Client, baseURL, apiKey, model, prompt string, width, height int, refs []ReferenceImage) ([]byte, string, error) {
	if width <= 0 {
		width = DefaultGenerationSize
	}
	if height <= 0 {
		height = DefaultGenerationSize
	}
	body := map[string]any{
		"model":  model,
		"prompt": prompt,
		"size":   fmt.Sprintf("%dx%d", width, height),
		// b64_json 让结果内联自包含（与 Ark/MiniMax 适配器同一策略）；Ark 系
		// 网关只接受 url|b64_json，"png" 这类值会被直接 400 拒绝。
		"response_format": "b64_json",
		"n":               1,
	}
	if len(refs) > 0 {
		imgs := make([]map[string]string, 0, len(refs))
		for _, r := range refs {
			imgs = append(imgs, map[string]string{
				"role":  r.Role,
				"image": "data:" + mimeOr(r.MIME) + ";base64," + base64.StdEncoding.EncodeToString(r.Data),
			})
		}
		body["reference_images"] = imgs
	}
	raw, err := postJSON(ctx, client, baseURL+"/images/generations", apiKey, body)
	if err != nil {
		return nil, "", err
	}
	return parseImageGenResponse(ctx, client, raw)
}

// parseImageGenResponse decodes an images/generations payload (b64_json, or
// url fetched via GET) into image bytes. Shared by the OpenAI-compatible
// surface and the Agnes-specific surface (same response envelope).
func parseImageGenResponse(ctx context.Context, client *http.Client, raw []byte) ([]byte, string, error) {
	var out imageGenResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, "", fmt.Errorf("provider: decode images response: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return nil, "", fmt.Errorf("provider: images API error: %s", out.Error.Message)
	}
	if len(out.Data) == 0 {
		return nil, "", fmt.Errorf("provider: images response has no data")
	}
	item := out.Data[0]
	switch {
	case item.B64JSON != "":
		data, err := base64.StdEncoding.DecodeString(item.B64JSON)
		if err != nil {
			return nil, "", fmt.Errorf("provider: decode b64 image: %w", err)
		}
		return data, "image/png", nil
	case item.URL != "":
		// Fetch the generated image from the vendor's CDN. The Bearer key is
		// deliberately NOT attached: arbitrary URLs must never receive the
		// credential. The download honors the same size cap.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
		if err != nil {
			return nil, "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("provider: fetch image url: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("provider: fetch image url: HTTP %d", resp.StatusCode)
		}
		data, err := readCappedBody(resp.Body, maxGenerationResponseBytes, "fetched image")
		if err != nil {
			return nil, "", fmt.Errorf("provider: fetch image url: %w", err)
		}
		return data, resp.Header.Get("Content-Type"), nil
	}
	return nil, "", fmt.Errorf("provider: images response item is empty")
}

// chatCompletionText performs one chat/completions call and returns the text.
// imageURI is an optional data URL: when set the user message becomes
// multimodal content parts (text + image_url) for vision-capable models;
// text-only requests keep the exact plain-string body as before.
func chatCompletionText(ctx context.Context, client *http.Client, baseURL, apiKey, model, prompt, imageURI string) (string, error) {
	var content any = prompt
	if imageURI != "" {
		content = []map[string]any{
			{"type": "text", "text": prompt},
			{"type": "image_url", "image_url": map[string]string{"url": imageURI}},
		}
	}
	body := map[string]any{
		"model":    model,
		"messages": []map[string]any{{"role": "user", "content": content}},
	}
	raw, err := postJSON(ctx, client, baseURL+"/chat/completions", apiKey, body)
	if err != nil {
		return "", err
	}
	var out chatResponse
	if err := decodeJSONResponse(raw, &out, "chat", apiKey); err != nil {
		return "", err
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("provider: chat API error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("provider: chat response has no choices")
	}
	reply := out.Choices[0].Message.Content
	if reply == "" {
		return "", fmt.Errorf("provider: chat response choice has no content")
	}
	return reply, nil
}

// responsesCompletionText performs an OpenAI Responses API call. Responses
// returns generated text inside output[].content[].text rather than choices.
func responsesCompletionText(ctx context.Context, client *http.Client, baseURL, apiKey, model, prompt string) (string, error) {
	body := map[string]any{
		"model": model,
		"input": prompt,
	}
	raw, err := postJSON(ctx, client, baseURL+"/responses", apiKey, body)
	if err != nil {
		return "", err
	}
	var out struct {
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Error *apiError `json:"error"`
	}
	if err := decodeJSONResponse(raw, &out, "responses", apiKey); err != nil {
		return "", err
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("provider: responses API error: %s", redactSecret(out.Error.Message, apiKey))
	}
	for _, item := range out.Output {
		for _, content := range item.Content {
			if strings.TrimSpace(content.Text) != "" {
				return content.Text, nil
			}
		}
	}
	return "", fmt.Errorf("provider: responses response has no output text")
}

// anthropicMessagesText performs an Anthropic Messages API call for custom
// text providers. Anthropic uses x-api-key and content blocks instead of the
// OpenAI Bearer/choices envelope.
func anthropicMessagesText(ctx context.Context, client *http.Client, baseURL, apiKey, model, prompt string) (string, error) {
	body := map[string]any{
		"model":      model,
		"max_tokens": 1024,
		"messages":   []map[string]any{{"role": "user", "content": prompt}},
	}
	raw, status, err := postJSONWithHeaders(ctx, client, baseURL+"/messages", apiKey,
		map[string]string{"Authorization": "", "x-api-key": apiKey, "anthropic-version": "2023-06-01"}, body)
	if err != nil {
		return "", err
	}
	if isAuthStatus(status) {
		return "", MarkNotRetryable(fmt.Errorf("provider: auth failed (HTTP %d): %s", status, non2xxDetail(raw, apiKey)))
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("provider: unexpected status %d: %s", status, non2xxDetail(raw, apiKey))
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *apiError `json:"error"`
	}
	if err := decodeJSONResponse(raw, &out, "messages", apiKey); err != nil {
		return "", err
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("provider: messages API error: %s", redactSecret(out.Error.Message, apiKey))
	}
	for _, block := range out.Content {
		if strings.TrimSpace(block.Text) != "" {
			return block.Text, nil
		}
	}
	return "", fmt.Errorf("provider: messages response has no text content")
}

func mimeOr(m string) string {
	if m == "" {
		return "image/png"
	}
	return m
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "…"
}
