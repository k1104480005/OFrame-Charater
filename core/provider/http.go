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

// postJSON performs an authenticated JSON POST and returns the raw response
// body. HTTP 401/403 are marked not-retryable.
func postJSON(ctx context.Context, client *http.Client, url, apiKey string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("provider: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("provider: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("provider: read response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, MarkNotRetryable(fmt.Errorf("provider: auth failed (HTTP %d): %s", resp.StatusCode, truncate(string(raw), 200)))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("provider: unexpected status %d: %s", resp.StatusCode, truncate(string(raw), 300))
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
		"model":           model,
		"prompt":          prompt,
		"size":            fmt.Sprintf("%dx%d", width, height),
		"response_format": "png",
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
		data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		if err != nil {
			return nil, "", err
		}
		return data, resp.Header.Get("Content-Type"), nil
	}
	return nil, "", fmt.Errorf("provider: images response item is empty")
}

// chatCompletionText performs one chat/completions call and returns the text.
func chatCompletionText(ctx context.Context, client *http.Client, baseURL, apiKey, model, prompt string) (string, error) {
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	raw, err := postJSON(ctx, client, baseURL+"/chat/completions", apiKey, body)
	if err != nil {
		return "", err
	}
	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("provider: decode chat response: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("provider: chat API error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("provider: chat response has no choices")
	}
	return out.Choices[0].Message.Content, nil
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
