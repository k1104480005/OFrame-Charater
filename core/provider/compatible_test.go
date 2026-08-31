package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// fakePNG is an arbitrary base64 payload — the adapter returns it verbatim
// (PNG validity is not checked at the transport layer).
const fakePNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

// --- compatible adapter (自定义 OpenAI 兼容 provider) ---

func TestCompatibleGenerateImage(t *testing.T) {
	var gotURL, gotAuth, gotModel string
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL.String()
		gotAuth = r.Header.Get("Authorization")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		return jsonResp(200, map[string]any{
			"data": []map[string]any{{"b64_json": fakePNG}},
		}), nil
	})
	cfg := ProviderConfig{
		ProviderID: "my-custom", Type: ProviderTypeCompatible, Name: "My Custom",
		APIKey: "sk-x", Model: "m1", TextModel: "t1", BaseURL: "https://example.com/v1",
	}
	p := NewCompatible(cfg, client)
	if p.ID() != "my-custom" || p.Name() != "My Custom" {
		t.Fatalf("id/name = %q/%q", p.ID(), p.Name())
	}
	if p.DefaultImageModel() != "m1" || p.DefaultTextModel() != "t1" {
		t.Fatalf("default models = %q/%q", p.DefaultImageModel(), p.DefaultTextModel())
	}
	res, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotURL, "/images/generations") {
		t.Fatalf("url = %q", gotURL)
	}
	if gotAuth != "Bearer sk-x" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotModel != "m1" {
		t.Fatalf("model = %q", gotModel)
	}
	want, _ := base64.StdEncoding.DecodeString(fakePNG)
	if !bytes.Equal(res.Data, want) {
		t.Fatal("image bytes mismatch")
	}
	if res.Provider != "my-custom" || res.Model != "m1" {
		t.Fatalf("result meta = %q/%q", res.Provider, res.Model)
	}
}

func TestCompatibleGenerateText(t *testing.T) {
	var gotURL, gotModel string
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL.String()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		return jsonResp(200, map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "hi"}}},
		}), nil
	})
	cfg := ProviderConfig{
		ProviderID: "my-custom", Type: ProviderTypeCompatible, Name: "My Custom",
		APIKey: "sk-x", TextModel: "t1", Model: "m1", BaseURL: "https://example.com/v1",
	}
	p := NewCompatible(cfg, client)
	res, err := p.GenerateText(context.Background(), TextRequest{Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotURL, "/chat/completions") {
		t.Fatalf("url = %q", gotURL)
	}
	if gotModel != "t1" || res.Text != "hi" {
		t.Fatalf("model/text = %q/%q", gotModel, res.Text)
	}
}

func TestCompatibleMissingKey(t *testing.T) {
	cfg := ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible, Name: "C1", Model: "m1", BaseURL: "https://example.com"}
	p := NewCompatible(cfg, fakeClient(func(r *http.Request) (*http.Response, error) {
		return jsonResp(200, map[string]any{"data": []map[string]any{}}), nil
	}))
	if _, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "p"}); err == nil {
		t.Fatal("expected key error")
	}
}

// --- structural validation (ProviderAdd 用) ---

func TestValidateForAddCustom(t *testing.T) {
	// Valid custom provider: slug id + name, model/url/key may be empty here.
	ok := ProviderConfig{ProviderID: "my-ark", Type: ProviderTypeCompatible, Name: "My Ark"}
	if err := ok.ValidateForAdd(); err != nil {
		t.Fatalf("valid custom rejected: %v", err)
	}
	// Generated id path (empty id) is filled by the service, not here.
	if err := (ProviderConfig{ProviderID: "", Type: ProviderTypeCompatible, Name: "X"}).ValidateForAdd(); err == nil {
		t.Fatal("expected error for empty id")
	}
	// Uppercase / non-slug id.
	if err := (ProviderConfig{ProviderID: "My Ark!", Type: ProviderTypeCompatible, Name: "X"}).ValidateForAdd(); err == nil {
		t.Fatal("expected error for non-slug id")
	}
	// Custom provider without a display name.
	if err := (ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible}).ValidateForAdd(); err == nil {
		t.Fatal("expected error for missing name")
	}
	// Built-in id with a conflicting type.
	if err := (ProviderConfig{ProviderID: ProviderDoubao, Type: ProviderTypeCompatible}).ValidateForAdd(); err == nil {
		t.Fatal("expected error for built-in type conflict")
	}
	// Built-in id with empty type still passes structural validation.
	if err := (ProviderConfig{ProviderID: ProviderDoubao}).ValidateForAdd(); err != nil {
		t.Fatalf("built-in with empty type rejected: %v", err)
	}
}

func TestValidateCustomStrict(t *testing.T) {
	cfg := ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible, Name: "C1", Model: "m1", BaseURL: "https://example.com/v1"}
	if err := cfg.Validate(); !strings.Contains(err.Error(), "no API key") {
		t.Fatalf("expected key error, got %v", err)
	}
	cfg.APIKey = "sk-x"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("strict validate with key: %v", err)
	}
	bad := cfg
	bad.BaseURL = "not-a-url"
	if err := bad.Validate(); err == nil {
		t.Fatal("expected error for bad base url")
	}
}

// --- 测试连接 / 模型列表 (GET /models) ---

func TestTestConnectionOK(t *testing.T) {
	var gotURL, gotAuth string
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL.String()
		gotAuth = r.Header.Get("Authorization")
		return jsonResp(200, map[string]any{
			"data": []map[string]any{{"id": "m1"}, {"id": "m2"}},
		}), nil
	})
	cfg := ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible, Name: "C1", Model: "m1", BaseURL: "https://example.com/v1", APIKey: "sk-x"}
	res := TestConnection(context.Background(), client, cfg)
	if !res.OK {
		t.Fatalf("expected ok, got %+v", res)
	}
	if len(res.Models) != 2 || res.Models[0] != "m1" {
		t.Fatalf("models = %v", res.Models)
	}
	if !strings.HasSuffix(gotURL, "/models") || gotAuth != "Bearer sk-x" {
		t.Fatalf("url/auth = %q/%q", gotURL, gotAuth)
	}
	if res.LatencyMS < 0 {
		t.Fatalf("negative latency %d", res.LatencyMS)
	}
}

func TestTestConnectionAuthFailure(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"bad key"}}`))}, nil
	})
	cfg := ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible, Name: "C1", Model: "m1", BaseURL: "https://example.com/v1", APIKey: "sk-x"}
	res := TestConnection(context.Background(), client, cfg)
	if res.OK {
		t.Fatal("expected failure")
	}
	if !strings.Contains(res.Error, "认证失败") {
		t.Fatalf("error = %q", res.Error)
	}
}

func TestTestConnectionNoKey(t *testing.T) {
	cfg := ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible, Name: "C1", Model: "m1", BaseURL: "https://example.com/v1"}
	res := TestConnection(context.Background(), nil, cfg)
	if res.OK {
		t.Fatal("expected failure without key")
	}
}

func TestTestConnectionTimeout(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})
	cfg := ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible, Name: "C1", Model: "m1", BaseURL: "https://example.com/v1", APIKey: "sk-x"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res := TestConnection(ctx, client, cfg)
	if res.OK {
		t.Fatal("expected failure on timeout")
	}
}

func TestListModels(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		return jsonResp(200, map[string]any{
			"data": []map[string]any{{"id": "a"}, {"id": "b"}, {}},
		}), nil
	})
	models, err := ListModels(context.Background(), client, "https://example.com/v1", "sk-x")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0] != "a" {
		t.Fatalf("models = %v", models)
	}
}

// --- 兼容适配器统一 HTTP 行为 (task 2.1: 认证/超时/响应大小/错误解析) ---

// TestCompatibleReferenceImagesContract pins the fake-transport request shape
// for reference images: roles preserved in order, empty MIME defaults to
// image/png, payloads are base64 data URLs, and the default size/format stay
// intact alongside the reference block.
func TestCompatibleReferenceImagesContract(t *testing.T) {
	var gotBody map[string]any
	var gotAuth string
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		return jsonResp(200, map[string]any{"data": []map[string]any{{"b64_json": fakePNG}}}), nil
	})
	cfg := ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible, Name: "C1",
		APIKey: "sk-x", Model: "m1", BaseURL: "https://example.com/v1"}
	p := NewCompatible(cfg, client)
	res, err := p.GenerateImage(context.Background(), ImageRequest{
		Prompt: "hero",
		References: []ReferenceImage{
			{Kind: "reference_image", Role: "main_reference", MIME: "", Data: []byte("abc")},
			{Kind: "reference_image", Role: "auxiliary_reference", MIME: "image/jpeg", Data: []byte("de")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer sk-x" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotBody["n"].(float64) != 1 || gotBody["response_format"] != "b64_json" || gotBody["size"] != "1024x1024" {
		t.Fatalf("request basics drifted: %v", gotBody)
	}
	refs, _ := gotBody["reference_images"].([]any)
	if len(refs) != 2 {
		t.Fatalf("reference_images = %v", gotBody["reference_images"])
	}
	first, _ := refs[0].(map[string]any)
	if first["role"] != "main_reference" ||
		first["image"] != "data:image/png;base64,"+base64.StdEncoding.EncodeToString([]byte("abc")) {
		t.Fatalf("ref[0] = %v", first)
	}
	second, _ := refs[1].(map[string]any)
	if second["role"] != "auxiliary_reference" ||
		second["image"] != "data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString([]byte("de")) {
		t.Fatalf("ref[1] = %v", second)
	}
	want, _ := base64.StdEncoding.DecodeString(fakePNG)
	if !bytes.Equal(res.Data, want) {
		t.Fatal("image bytes mismatch")
	}
}

// TestCompatibleAppliesConfiguredTimeout verifies the adapter bounds external
// calls with the configured per-call timeout even when the caller passes a
// bare background context: the transport must observe a live deadline on both
// the image and text paths (task 2.1 统一超时).
func TestCompatibleAppliesConfiguredTimeout(t *testing.T) {
	checkDeadline := func(r *http.Request) {
		if _, ok := r.Context().Deadline(); !ok {
			t.Error("expected an adapter-applied deadline on the provider call")
		}
	}
	cfg := ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible, Name: "C1",
		APIKey: "sk-x", Model: "m1", TextModel: "t1", BaseURL: "https://example.com/v1"}

	imgClient := fakeClient(func(r *http.Request) (*http.Response, error) {
		checkDeadline(r)
		return jsonResp(200, map[string]any{"data": []map[string]any{{"b64_json": fakePNG}}}), nil
	})
	if _, err := NewCompatible(cfg, imgClient).GenerateImage(context.Background(), ImageRequest{Prompt: "p"}); err != nil {
		t.Fatal(err)
	}

	txtClient := fakeClient(func(r *http.Request) (*http.Response, error) {
		checkDeadline(r)
		return jsonResp(200, map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "ok"}}},
		}), nil
	})
	if _, err := NewCompatible(cfg, txtClient).GenerateText(context.Background(), TextRequest{Prompt: "p"}); err != nil {
		t.Fatal(err)
	}
}

// TestCompatibleURLFetchedImage covers the url response variant: bytes come
// from the CDN download with the vendor's content type, and — critically — the
// Bearer key must never be attached to the arbitrary CDN fetch.
func TestCompatibleURLFetchedImage(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost {
			return jsonResp(200, map[string]any{
				"data": []map[string]any{{"url": "http://cdn.example.com/img.png"}},
			}), nil
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("CDN fetch must not carry the API key")
		}
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(strings.NewReader("CDN-BYTES")),
		}, nil
	})
	cfg := ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible, Name: "C1",
		APIKey: "sk-secret-cdn", Model: "m1", BaseURL: "https://example.com/v1"}
	res, err := NewCompatible(cfg, client).GenerateImage(context.Background(), ImageRequest{Prompt: "hero"})
	if err != nil {
		t.Fatal(err)
	}
	if res.MIME != "image/png" || string(res.Data) != "CDN-BYTES" {
		t.Fatalf("fetched result mime/data = %q/%q", res.MIME, res.Data)
	}
}

// TestCompatibleOversizeResponseReadable verifies a vendor response past the
// shared size cap fails fast with a readable limit error instead of a silent
// truncation that would later die as a decoder error. The package-level cap is
// lowered so the fixture stays tiny.
func TestCompatibleOversizeResponseReadable(t *testing.T) {
	defer func(orig int64) { maxGenerationResponseBytes = orig }(maxGenerationResponseBytes)
	maxGenerationResponseBytes = 1 << 20 // shrink cap for the test only

	body := strings.Repeat("x", (1<<20)+16)
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	cfg := ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible, Name: "C1",
		APIKey: "sk-x", Model: "m1", BaseURL: "https://example.com/v1"}
	_, err := NewCompatible(cfg, client).GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected readable size-limit error, got %v", err)
	}
}

// TestCompatibleNon2xxErrorsWithoutKeyLeak covers non-2xx error parsing: the
// vendor envelope wins over a raw body dump, 401 stays marked not-retryable,
// other statuses remain retryable, and reflected keys never reach the user.
func TestCompatibleNon2xxErrorsWithoutKeyLeak(t *testing.T) {
	const leakyKey = "sk-leaky-12345"
	cases := []struct {
		name         string
		status       int
		contentType  string
		respBody     string
		wantContains string
		notRetryable bool
	}{
		{
			name:         "401 envelope marked not retryable",
			status:       401,
			contentType:  "application/json",
			respBody:     `{"error":{"message":"invalid_api_key ` + leakyKey + `"}}`,
			wantContains: "invalid_api_key ***",
			notRetryable: true,
		},
		{
			name:         "500 plain body echoed without the key",
			status:       500,
			contentType:  "text/html",
			respBody:     `<html>oops ` + leakyKey + ` boom</html>`,
			wantContains: "unexpected status 500",
			notRetryable: false,
		},
		{
			name:         "429 rate limit keeps vendor message",
			status:       429,
			contentType:  "application/json",
			respBody:     `{"error":{"message":"rate limited"}}`,
			wantContains: "rate limited",
			notRetryable: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := fakeClient(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tc.status,
					Header:     http.Header{"Content-Type": []string{tc.contentType}},
					Body:       io.NopCloser(strings.NewReader(tc.respBody)),
				}, nil
			})
			cfg := ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible, Name: "C1",
				APIKey: leakyKey, Model: "m1", BaseURL: "https://example.com/v1"}
			p := NewCompatible(cfg, client)
			_, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
			if err == nil {
				t.Fatal("expected an error")
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.wantContains) {
				t.Fatalf("error %q does not contain %q", msg, tc.wantContains)
			}
			if strings.Contains(msg, leakyKey) {
				t.Fatalf("error leaked the API key: %q", msg)
			}
			if IsNotRetryable(err) != tc.notRetryable {
				t.Fatalf("IsNotRetryable = %v, want %v (err %q)", IsNotRetryable(err), tc.notRetryable, msg)
			}
		})
	}
}

// TestCompatibleMissingFieldsReadable pins readable failures for malformed
// 200 responses instead of empty results or cryptic decoder noise.
func TestCompatibleMissingFieldsReadable(t *testing.T) {
	const respOK = `{"status":"ok"}`
	cases := []struct {
		name    string
		textual bool
		body    string
		want    string
	}{
		{name: "images no data", body: `{"data":[]}`, want: "images response has no data"},
		{name: "images empty item", body: `{"data":[{}]}`, want: "item is empty"},
		{name: "images bad b64", body: `{"data":[{"b64_json":"not-base64!!"}]}`, want: "decode b64 image"},
		{name: "images in-envelope error", body: `{"error":{"message":"prompt flagged"},"data":[]}`, want: "images API error: prompt flagged"},
		{name: "chat no choices", textual: true, body: respOK, want: "chat response has no choices"},
		{name: "chat no content", textual: true, body: `{"choices":[{"message":{"role":"assistant"}}]}`, want: "choice has no content"},
		{name: "chat in-envelope error", textual: true, body: `{"choices":[{"message":{"content":""}}],"error":{"message":"quota exceeded"}}`, want: "chat API error: quota exceeded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := fakeClient(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(tc.body)),
				}, nil
			})
			cfg := ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible, Name: "C1",
				APIKey: "sk-x", Model: "m1", TextModel: "t1", BaseURL: "https://example.com/v1"}
			p := NewCompatible(cfg, client)
			var err error
			if tc.textual {
				_, err = p.GenerateText(context.Background(), TextRequest{Prompt: "p"})
			} else {
				_, err = p.GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected readable error containing %q, got %v", tc.want, err)
			}
		})
	}
}
