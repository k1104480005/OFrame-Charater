package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// geminiConfig builds a banana/Gemini config mirroring what the FrameBaker
// preset produces: explicit protocol Type, vendor BaseURL, catalogs, key.
func geminiConfig(baseURL string) ProviderConfig {
	return ProviderConfig{
		ProviderID:  "nano-banana",
		Type:        ProviderTypeGemini,
		Name:        "My Banana",
		APIKey:      "AIza-SecretTestKeyValue0123456789",
		BaseURL:     baseURL,
		ImageModels: []string{"gemini-2.5-flash-image"},
		TextModels:  []string{"gemini-2.5-flash"},
	}
}

// --- 身份与能力（与 DeclaredCapabilities 保持一致；视频恒为预留） ---

func TestGeminiIdentityAndCapabilities(t *testing.T) {
	cfg := geminiConfig(DefaultGeminiBaseURL)
	p := NewGemini(cfg, nil)
	if p.ID() != "nano-banana" {
		t.Fatalf("ID = %q, want configured provider id", p.ID())
	}
	if p.Name() != "My Banana" {
		t.Fatalf("Name = %q, want display name", p.Name())
	}
	if got := NewGemini(ProviderConfig{ProviderID: "nano-banana", Type: ProviderTypeGemini}, nil).Name(); got != "banana / Gemini" {
		t.Fatalf("fallback Name = %q", got)
	}
	caps := p.Capabilities()
	want := cfg.DeclaredCapabilities()
	if caps != want {
		t.Fatalf("live capabilities %v drift from declared %v", caps, want)
	}
	if caps.Video {
		t.Fatal("gemini adapter claims video before any video adapter exists")
	}
	if !caps.Image || !caps.Text {
		t.Fatalf("image/text capabilities lost: %+v", caps)
	}
	// Catalog-only configs (legacy singular fields empty) resolve their
	// defaults from the effective catalogs, exactly like the other presets.
	if p.DefaultImageModel() != "" || p.DefaultTextModel() != "" {
		t.Fatalf("legacy defaults should stay empty for catalog-only configs: %q/%q", p.DefaultImageModel(), p.DefaultTextModel())
	}
	withLegacy := cfg
	withLegacy.Model = "img-explicit"
	if got := NewGemini(withLegacy, nil).DefaultImageModel(); got != "img-explicit" {
		t.Fatalf("DefaultImageModel with legacy field = %q", got)
	}
}

// TestNewAdapterRoutesGeminiExplicitly pins the additive registry contract
// after task 2.3: every previously-routed type keeps its exact adapter,
// gemini gains its own generateContent adapter (never a silent compatible
// fallback), and unknown types still fail explicitly.
func TestNewAdapterRoutesGeminiExplicitly(t *testing.T) {
	mk := func(typ string) ProviderConfig {
		return ProviderConfig{ProviderID: "p1", Type: typ, Name: "P1"}
	}
	cases := []struct {
		typ  string
		want any // expected concrete adapter type
	}{
		{ProviderDoubao, &Doubao{}},
		{ProviderOpenAI, &OpenAI{}},
		{ProviderAgnes, &Agnes{}},
		{ProviderTypeCompatible, &Compatible{}},
		{ProviderTypeDashscope, &Dashscope{}},
		{ProviderTypeGemini, &Gemini{}},
	}
	for _, tc := range cases {
		p, err := NewAdapter("p1", mk(tc.typ), nil)
		if err != nil {
			t.Fatalf("type %q: unexpected error %v", tc.typ, err)
		}
		// Built-ins keep their hard-coded vendor identity; custom protocol
		// types (compatible/dashscope/gemini) use the configured provider id.
		wantID := "p1"
		if tc.typ == ProviderDoubao || tc.typ == ProviderOpenAI || tc.typ == ProviderAgnes {
			wantID = tc.typ
		}
		if id := p.ID(); id != wantID {
			t.Errorf("type %q: adapter ID = %q, want %q", tc.typ, id, wantID)
		}
		if _, ok := p.(*Gemini); ok && tc.typ != ProviderTypeGemini {
			t.Errorf("type %q silently became Gemini", tc.typ)
		}
		if fmt.Sprintf("%T", p) != fmt.Sprintf("%T", tc.want) {
			t.Errorf("type %q routed to %T, want %T — 协议身份不得被另一协议顶替", tc.typ, p, tc.want)
		}
	}
	if _, err := NewAdapter("p1", ProviderConfig{ProviderID: "p1", Type: "mystery"}, nil); err == nil {
		t.Error("unknown types must still be rejected")
	}
}

// --- 图片请求面：path/model、认证头、无 query 泄露、超时、parts 形状 ---

func TestGeminiGenerateImageRequestShape(t *testing.T) {
	var gotMethod, gotPath, gotRawQuery, gotAuthHeader, gotAPIKeyHeader, gotContentType string
	var gotAuthValuePresent bool
	sawDeadline := false
	var gotBody map[string]any
	var rawBody []byte
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotRawQuery = r.URL.RawQuery
		gotAPIKeyHeader = r.Header.Get(headerGeminiAPIKey)
		gotAuthHeader = r.Header.Get("Authorization")
		gotAuthValuePresent = gotAuthHeader != ""
		gotContentType = r.Header.Get("Content-Type")
		if _, ok := r.Context().Deadline(); ok {
			sawDeadline = true
		}
		rawBody, _ = io.ReadAll(r.Body)
		_ = json.Unmarshal(rawBody, &gotBody)
		return jsonResp(200, map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"role":  "model",
					"parts": []map[string]any{{"inlineData": map[string]any{"mimeType": "image/png", "data": fakePNG}}},
				},
				"finishReason": "STOP",
			}},
		}), nil
	})
	const key = "AIza-SecretTestKeyValue0123456789"
	res, err := NewGemini(geminiConfig(DefaultGeminiBaseURL), client).
		GenerateImage(context.Background(), ImageRequest{Prompt: "赛博朋克人物立绘"})
	if err != nil {
		t.Fatal(err)
	}

	// path/model 锁定：POST {base}/models/{model}:generateContent，模型在路径中。
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %s", gotMethod)
	}
	if gotPath != "/v1beta/models/gemini-2.5-flash-image:generateContent" {
		t.Fatalf("endpoint drifted: %s", gotPath)
	}
	if !sawDeadline {
		t.Error("expected an adapter-applied deadline on the provider call")
	}

	// 认证锁定：仅 x-goog-api-key，不带 Authorization/Bearer。
	if gotAPIKeyHeader != key {
		t.Fatalf("%s header = %q", headerGeminiAPIKey, gotAPIKeyHeader)
	}
	if gotAuthValuePresent {
		t.Fatalf("Authorization header leaked Bearer credential: %q", gotAuthHeader)
	}
	if gotContentType != "application/json" {
		t.Errorf("content-type = %q", gotContentType)
	}

	// 密钥不进 URL（无 ?key=），也不进请求体。
	if gotRawQuery != "" {
		t.Fatalf("query string carried data: %q", gotRawQuery)
	}
	fullURL := DefaultGeminiBaseURL + gotPath
	if strings.Contains(fullURL+"|"+string(rawBody), key) {
		t.Fatalf("key found outside headers: url=%q body=%s", fullURL, truncate(string(rawBody), 200))
	}

	// 请求体锁定：generateContent 自己的键集合——contents/parts/text，
	// 模型与尺寸绝不以 OpenAI 兼容字段混入。
	topKeys := make([]string, 0, len(gotBody))
	for k := range gotBody {
		topKeys = append(topKeys, k)
	}
	if len(topKeys) != 1 || topKeys[0] != "contents" {
		t.Fatalf("request body keys = %v, want [contents]", topKeys)
	}
	if gotBody["model"] != nil || gotBody["size"] != nil || gotBody["response_format"] != nil || gotBody["messages"] != nil {
		t.Fatalf("foreign protocol fields crept into the body: %v", topKeys)
	}
	contents, _ := gotBody["contents"].([]any)
	row, _ := contents[0].(map[string]any)
	if row["role"] != geminiRoleUser {
		t.Errorf("contents[0].role = %v", row["role"])
	}
	parts, _ := row["parts"].([]any)
	first, _ := parts[0].(map[string]any)
	if first["text"] != "赛博朋克人物立绘" {
		t.Errorf("parts[0].text = %v", first["text"])
	}
	for i := 1; i < len(parts); i++ {
		part, _ := parts[i].(map[string]any)
		if part["text"] != nil {
			t.Errorf("unexpected extra text part at %d: %v", i, part)
		}
	}

	// 结果解析：inlineData base64 → PNG 字节 + 结果元数据按本适配器身份返回。
	wantPNG, _ := base64.StdEncoding.DecodeString(fakePNG)
	if !bytes.Equal(res.Data, wantPNG) || res.MIME != "image/png" {
		t.Fatalf("decoded image mismatch: mime=%q len=%d", res.MIME, len(res.Data))
	}
	if res.Provider != "nano-banana" || res.Model != "gemini-2.5-flash-image" {
		t.Fatalf("result meta = %q/%q", res.Provider, res.Model)
	}
}

// --- 引用图：多个引用按序转 inlineData，sprite 类同样支持 ---

func TestGeminiReferenceImagesInlineDataContract(t *testing.T) {
	var gotParts []struct {
		Text       string `json:"text"`
		InlineData *struct {
			MimeType string `json:"mimeType"`
			Data     string `json:"data"`
		} `json:"inlineData"`
	}
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					Text       string `json:"text"`
					InlineData *struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"contents"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		gotParts = body.Contents[0].Parts
		return jsonResp(200, map[string]any{"candidates": []map[string]any{{
			"content": map[string]any{"parts": []map[string]any{{"inlineData": map[string]any{"mimeType": "image/png", "data": fakePNG}}}},
		}}}), nil
	})
	res, err := NewGemini(geminiConfig(DefaultGeminiBaseURL), client).GenerateImage(context.Background(), ImageRequest{
		Prompt: "hero turnaround",
		References: []ReferenceImage{
			{Kind: geminiRefKindReferenceImage, Role: "main_reference", MIME: "", Data: []byte("abc")},
			{Kind: geminiRefKindSprite, Role: "sprite", MIME: "image/jpeg", Data: []byte("de")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(gotParts) != 3 {
		t.Fatalf("parts = %d, want prompt + 2 references in order", len(gotParts))
	}
	if gotParts[0].Text != "hero turnaround" {
		t.Fatalf("prompt part drifted: %+v", gotParts[0])
	}
	refsOut := gotParts[1:]
	wantMimes := []string{"image/png", "image/jpeg"} // 空 MIME 默认 png，与共享契约一致
	wantDatas := [][]byte{[]byte("abc"), []byte("de")}
	for i, p := range refsOut {
		if p.Text != "" {
			t.Errorf("ref part %d unexpectedly carries text %q", i, p.Text)
		}
		if p.InlineData == nil {
			t.Fatalf("ref part %d lost its inlineData entry", i)
		}
		if p.InlineData.MimeType != wantMimes[i] {
			t.Errorf("ref %d mimeType = %q, want %q", i, p.InlineData.MimeType, wantMimes[i])
		}
		if got := p.InlineData.Data; got != base64.StdEncoding.EncodeToString(wantDatas[i]) {
			t.Errorf("ref %d data = %q, want base64(%q)", i, got, wantDatas[i])
		}
	}
	wantPNG, _ := base64.StdEncoding.DecodeString(fakePNG)
	if !bytes.Equal(res.Data, wantPNG) {
		t.Fatal("image bytes mismatch")
	}
}

// --- 引用边界：不支持的类型/空数据在外呼前明确报错（零外呼、不重试） ---

func TestGeminiUnsupportedReferencesPreFlight(t *testing.T) {
	cases := []struct {
		name string
		refs []ReferenceImage
		want []string
	}{
		{
			name: "unknown future material kind",
			refs: []ReferenceImage{{Kind: "video_clip", MIME: "video/mp4", Data: []byte("xyz")}},
			want: []string{"unsupported reference image", "reference_image", "sprite"},
		},
		{
			name: "empty kind rejected too",
			refs: []ReferenceImage{{Kind: "", Data: []byte("xyz")}},
			want: []string{"unsupported reference image", `kind ""`},
		},
		{
			name: "missing payload data",
			refs: []ReferenceImage{{Kind: geminiRefKindReferenceImage, MIME: "image/png", Data: nil}},
			want: []string{"reference image #0", "carries no data"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := fakeClient(func(*http.Request) (*http.Response, error) {
				calls++
				return jsonResp(200, map[string]any{}), nil
			})
			cfg := geminiConfig(DefaultGeminiBaseURL)
			_, err := NewGemini(cfg, client).GenerateImage(context.Background(), ImageRequest{Prompt: "p", References: tc.refs})
			if err == nil {
				t.Fatal("expected a pre-flight rejection")
			}
			for _, part := range tc.want {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("error %q misses %q", err, part)
				}
			}
			if calls != 0 {
				t.Fatalf("rejection issued %d outbound calls before validating references", calls)
			}
			if !IsNotRetryable(err) {
				t.Fatalf("request-shape rejection should not waste retries: %v", err)
			}
		})
	}

	t.Run("valid kinds and sprites stay attachable", func(t *testing.T) {
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			return jsonResp(200, map[string]any{"candidates": []map[string]any{{
				"content": map[string]any{"parts": []map[string]any{{"inlineData": map[string]any{"mimeType": "image/png", "data": fakePNG}}}},
			}}}), nil
		})
		_, err := NewGemini(geminiConfig(DefaultGeminiBaseURL), client).GenerateImage(context.Background(), ImageRequest{
			Prompt: "p",
			References: []ReferenceImage{
				{Kind: geminiRefKindReferenceImage, Data: []byte("a")},
				{Kind: geminiRefKindSprite, Data: []byte("b")},
			},
		})
		if err != nil {
			t.Fatalf("supported kinds were wrongly rejected: %v", err)
		}
	})

	t.Run("fully empty request is a pre-flight failure", func(t *testing.T) {
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			return jsonResp(200, map[string]any{}), nil
		})
		_, err := NewGemini(geminiConfig(DefaultGeminiBaseURL), client).GenerateImage(context.Background(), ImageRequest{})
		if err == nil || !strings.Contains(err.Error(), "empty generation request") {
			t.Fatalf("expected empty-request pre-flight failure, got %v", err)
		}
	})
}

// --- 响应解析：成功变体与可读失败（文本回答/缺字段/坏 b64/拦截） ---

func TestGeminiImageResponseParsingVariants(t *testing.T) {
	mkCandidates := func(parts []map[string]any) []map[string]any {
		return []map[string]any{{"content": map[string]any{"parts": parts}, "finishReason": "STOP"}}
	}
	imgPart := func(mime, b64 string) map[string]any {
		return map[string]any{"inlineData": map[string]any{"mimeType": mime, "data": b64}}
	}
	successCases := []struct {
		name       string
		body       map[string]any
		wantMIME   string
		wantPrefix string // decoded-bytes prefix; empty prefix asserts length only
	}{
		{
			name:     "text part before image keeps image",
			body:     map[string]any{"candidates": mkCandidates([]map[string]any{{"text": "Here is your image."}, imgPart("image/png", fakePNG)})},
			wantMIME: "image/png",
		},
		{
			name:     "vendor mimeType preserved",
			body:     map[string]any{"candidates": mkCandidates([]map[string]any{imgPart("image/webp", fakePNG)})},
			wantMIME: "image/webp",
		},
		{
			name:     "blank result mime defaults to png",
			body:     map[string]any{"candidates": mkCandidates([]map[string]any{imgPart("", fakePNG)})},
			wantMIME: "image/png",
		},
		{
			name:       "first usable image wins over later ones",
			body:       map[string]any{"candidates": mkCandidates([]map[string]any{imgPart("image/png", base64markerA), imgPart("image/png", base64markerB)})},
			wantMIME:   "image/png",
			wantPrefix: markerA,
		},
		{
			name:     "empty-data part skipped, later image still used",
			body:     map[string]any{"candidates": mkCandidates([]map[string]any{imgPart("image/png", "  "), imgPart("image/png", fakePNG)})},
			wantMIME: "image/png",
		},
	}
	fakePNGBytes, _ := base64.StdEncoding.DecodeString(fakePNG)
	for _, tc := range successCases {
		t.Run(tc.name, func(t *testing.T) {
			client := fakeClient(func(*http.Request) (*http.Response, error) { return jsonResp(200, tc.body), nil })
			res, err := NewGemini(geminiConfig(DefaultGeminiBaseURL), client).GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
			if err != nil {
				t.Fatalf("expected success, got %v", err)
			}
			if res.MIME != tc.wantMIME {
				t.Errorf("mime = %q, want %q", res.MIME, tc.wantMIME)
			}
			if tc.wantPrefix != "" {
				if string(res.Data) != tc.wantPrefix {
					t.Errorf("picked wrong image part: %q", string(res.Data))
				}
			} else if !bytes.Equal(res.Data, fakePNGBytes) {
				t.Errorf("decoded bytes mismatch (len %d)", len(res.Data))
			}
			if res.Provider != "nano-banana" || res.Model != "gemini-2.5-flash-image" {
				t.Errorf("result meta = %+v", res)
			}
		})
	}

	failCases := []struct {
		name     string
		body     map[string]any
		want     []string
		notIn    []string
		notRetry bool
	}{
		{
			name: "no candidates",
			body: map[string]any{},
			want: []string{"gemini response has no candidates"},
		},
		{
			name: "candidate without parts",
			body: map[string]any{"candidates": []map[string]any{{"content": map[string]any{}, "finishReason": "SAFETY"}}},
			want: []string{"candidate has no parts", `finishReason "SAFETY"`},
		},
		{
			name:  "text-only answer is a readable failure",
			body:  map[string]any{"candidates": mkCandidates([]map[string]any{{"text": "I cannot render images."}})},
			want:  []string{"text-only response without image data"},
			notIn: []string{fakePNG},
		},
		{
			name: "part objects carry neither text nor inlineData",
			body: map[string]any{"candidates": mkCandidates([]map[string]any{{}, {}})},
			want: []string{"carried no inlineData image"},
		},
		{
			name: "malformed base64 fails readable",
			body: map[string]any{"candidates": mkCandidates([]map[string]any{imgPart("image/png", "@@not-base64@@")})},
			want: []string{"decode gemini inlineData"},
		},
		{
			name: "blocked prompt reported instead of no-candidates noise",
			body: map[string]any{"promptFeedback": map[string]any{"blockReason": "PROHIBITED_CONTENT"}},
			want: []string{"blocked the prompt", "PROHIBITED_CONTENT"},
		},
		{
			name:  "in-band error envelope at HTTP 200 surfaced redacted",
			body:  map[string]any{"error": map[string]any{"code": 429, "message": "Resource exhausted AIza-SecretTestKeyValue0123456789"}},
			want:  []string{"gemini API error", "***"},
			notIn: []string{"AIza-SecretTestKeyValue0123456789"},
		},
	}
	for _, tc := range failCases {
		t.Run(tc.name, func(t *testing.T) {
			client := fakeClient(func(*http.Request) (*http.Response, error) { return jsonResp(200, tc.body), nil })
			res, err := NewGemini(geminiConfig(DefaultGeminiBaseURL), client).GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
			if err == nil {
				t.Fatalf("expected a readable failure, got result %#v", res)
			}
			msg := err.Error()
			for _, part := range tc.want {
				if !strings.Contains(msg, part) {
					t.Errorf("error %q misses %q", msg, part)
				}
			}
			for _, banned := range tc.notIn {
				if strings.Contains(msg, banned) {
					t.Errorf("error leaked %q: %q", banned, msg)
				}
			}
			if IsNotRetryable(err) != tc.notRetry {
				t.Errorf("IsNotRetryable = %v, want %v (%q)", IsNotRetryable(err), tc.notRetry, msg)
			}
		})
	}
}

// base64markerA/B give the "which part wins" assertions distinct payloads.
var (
	base64markerA = base64.StdEncoding.EncodeToString([]byte("MARKER-A"))
	base64markerB = base64.StdEncoding.EncodeToString([]byte("MARKER-B"))
)

const markerA = "MARKER-A"

// --- 非 2xx 与响应上限：脱敏、错误信封、retry 分类统一沿用 2.1 语义 ---

func TestGeminiNon2xxErrorsWithoutKeyLeak(t *testing.T) {
	origCap := maxGenerationResponseBytes
	defer func() { maxGenerationResponseBytes = origCap }()
	const leakyKey = "AIza-SecretTestKeyValue0123456789"

	type run struct {
		name      string
		setup     func(*testing.T) (*http.Client, int) // client + maxGen override (0 = keep)
		wantParts []string
		notIn     []string
		notRtry   bool
	}
	cases := []run{
		{
			name: "401 envelope marked not retryable with redaction",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawJSONResp(http.StatusUnauthorized, `{"error":{"code":401,"message":"API key expired AIza-SecretTestKeyValue0123456789","status":"UNAUTHENTICATED"}}`), nil
				}), 0
			},
			wantParts: []string{"auth failed", "HTTP 401", "***", "API key expired"},
			notIn:     []string{leakyKey},
			notRtry:   true,
		},
		{
			name: "403 stays an auth-class failure",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawJSONResp(http.StatusForbidden, `{"error":{"message":"PERMISSION_DENIED for AIza-SecretTestKeyValue0123456789"}}`), nil
				}), 0
			},
			wantParts: []string{"auth failed", "HTTP 403", "***"},
			notIn:     []string{leakyKey},
			notRtry:   true,
		},
		{
			name: "500 html echo stays retryable without leaking key",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawResp(http.StatusInternalServerError, "text/html", `<html>boom AIza-SecretTestKeyValue0123456789</html>`), nil
				}), 0
			},
			wantParts: []string{"unexpected status 500", "boom"},
			notIn:     []string{leakyKey},
			notRtry:   false,
		},
		{
			name: "400 quota-style envelope remains readable and retryable",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawJSONResp(http.StatusBadRequest, `{"error":{"code":400,"message":"Resource has been exhausted","status":"RESOURCE_EXHAUSTED"}}`), nil
				}), 0
			},
			wantParts: []string{"unexpected status 400", "exhausted"},
			notRtry:   false,
		},
		{
			name: "oversize 200 hits the shared response-size cap",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawResp(200, "application/json", strings.Repeat("x", (1<<20)+16)), nil
				}), 1 << 20
			},
			wantParts: []string{"too large"},
		},
		{
			name: "transport timeout wraps readable",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return nil, context.DeadlineExceeded
				}), 0
			},
			wantParts: []string{"provider:", "deadline exceeded"},
		},
	}
	for i := range cases {
		tc := cases[i]
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup == nil {
				t.Fatal("missing setup")
			}
			client, genCap := tc.setup(t)
			if genCap > 0 {
				maxGenerationResponseBytes = int64(genCap)
			}
			p := NewGemini(geminiConfig(DefaultGeminiBaseURL), client)
			_, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
			if err == nil {
				t.Fatal("expected an error")
			}
			msg := err.Error()
			for _, part := range tc.wantParts {
				if !strings.Contains(msg, part) {
					t.Errorf("error %q misses %q", msg, part)
				}
			}
			for _, banned := range tc.notIn {
				if strings.Contains(msg, banned) {
					t.Errorf("error leaked %q: %q", banned, msg)
				}
			}
			if IsNotRetryable(err) != tc.notRtry {
				t.Errorf("IsNotRetryable = %v, want %v (msg %q)", IsNotRetryable(err), tc.notRtry, msg)
			}
			// Restore the shared cap so later subtests keep production limits.
			maxGenerationResponseBytes = origCap
		})
	}
}

// --- 文本生成：同一端点/密钥，文本目录选择模型；多段 text 合并 ---

func TestGeminiGenerateTextShapeAndParsing(t *testing.T) {
	var gotPath, gotAPIKey string
	sawDeadline := false
	sawInline := false
	var gotPrompt string
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get(headerGeminiAPIKey)
		if _, ok := r.Context().Deadline(); ok {
			sawDeadline = true
		}
		var body struct {
			Contents []struct {
				Parts []struct {
					Text       string `json:"text"`
					InlineData *struct {
						Data string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"contents"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		gotPrompt = body.Contents[0].Parts[0].Text
		sawInline = len(body.Contents[0].Parts) > 1 || body.Contents[0].Parts[0].InlineData != nil
		return jsonResp(200, map[string]any{"candidates": []map[string]any{{
			"content": map[string]any{"parts": []map[string]any{{"text": "Hello"}, {"text": "World"}}},
		}}}), nil
	})
	cfg := geminiConfig(DefaultGeminiBaseURL)
	res, err := NewGemini(cfg, client).GenerateText(context.Background(), TextRequest{Prompt: "写一句开场白"})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1beta/models/gemini-2.5-flash:generateContent" {
		t.Fatalf("text endpoint used the wrong catalog model: %s", gotPath)
	}
	if gotAPIKey != cfg.APIKey {
		t.Fatalf("auth header = %q", gotAPIKey)
	}
	if !sawDeadline {
		t.Error("expected an adapter-applied deadline on the text call")
	}
	if gotPrompt != "写一句开场白" || sawInline {
		t.Fatalf("text request parts drifted: prompt=%q inline=%v", gotPrompt, sawInline)
	}
	if res.Text != "Hello\nWorld" {
		t.Fatalf("multi-part text join = %q", res.Text)
	}
	if res.Provider != "nano-banana" || res.Model != "gemini-2.5-flash" {
		t.Fatalf("result meta = %+v", res)
	}

	t.Run("explicit model override reaches the path", func(t *testing.T) {
		var p string
		c := fakeClient(func(r *http.Request) (*http.Response, error) {
			p = r.URL.Path
			return jsonResp(200, map[string]any{"candidates": []map[string]any{{
				"content": map[string]any{"parts": []map[string]any{{"text": "ok"}}},
			}}}), nil
		})
		if _, err := NewGemini(cfg, c).GenerateText(context.Background(), TextRequest{Prompt: "hi", Model: "custom-chat-x"}); err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(p, "/models/custom-chat-x:generateContent") {
			t.Fatalf("path = %s", p)
		}
	})

	t.Run("readable failures mirror the image parser", func(t *testing.T) {
		cases := []struct {
			name string
			body string
			want string
		}{
			{name: "no text anywhere", body: `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"eA=="}}]}}]}`, want: "has no text content"},
			{name: "error envelope", body: `{"error":{"message":"permission denied AIza-SecretTestKeyValue0123456789"}}`, want: "gemini API error"},
			{name: "blocked", body: `{"promptFeedback":{"blockReason":"SAFETY"}}`, want: "SAFETY"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				c := fakeClient(func(*http.Request) (*http.Response, error) {
					return jsonResp(200, json.RawMessage(tc.body)), nil
				})
				_, err := NewGemini(cfg, c).GenerateText(context.Background(), TextRequest{Prompt: "hi"})
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("expected error containing %q, got %v", tc.want, err)
				}
				if strings.Contains(err.Error(), "AIza-SecretTestKeyValue0123456789") {
					t.Fatalf("error leaked the key: %v", err)
				}
			})
		}
	})

	t.Run("no text models configured fails before outbound", func(t *testing.T) {
		calls := 0
		c := fakeClient(func(*http.Request) (*http.Response, error) {
			calls++
			return jsonResp(200, map[string]any{}), nil
		})
		textless := geminiConfig(DefaultGeminiBaseURL)
		textless.TextModels = nil
		textless.TextModel = ""
		_, err := NewGemini(textless, c).GenerateText(context.Background(), TextRequest{Prompt: "x"})
		if err == nil || !strings.Contains(err.Error(), "no text model configured") {
			t.Fatalf("expected pre-flight model error, got %v", err)
		}
		if calls != 0 {
			t.Fatalf("pre-flight failure issued %d outbound calls", calls)
		}
	})

	t.Run("empty prompt fails before outbound", func(t *testing.T) {
		calls := 0
		c := fakeClient(func(*http.Request) (*http.Response, error) {
			calls++
			return jsonResp(200, map[string]any{}), nil
		})
		_, err := NewGemini(cfg, c).GenerateText(context.Background(), TextRequest{Prompt: "   "})
		if err == nil || !strings.Contains(err.Error(), "empty text prompt") {
			t.Fatalf("expected empty-prompt pre-flight failure, got %v", err)
		}
		if calls != 0 {
			t.Fatalf("pre-flight failure issued %d outbound calls", calls)
		}
	})
}

// --- 密钥缺失：零外呼 ---

func TestGeminiMissingKeyZeroNetwork(t *testing.T) {
	t.Setenv("OFRAME_NANO-BANANA_API_KEY", "")
	calls := 0
	client := fakeClient(func(*http.Request) (*http.Response, error) {
		calls++
		return jsonResp(200, map[string]any{}), nil
	})
	cfg := geminiConfig(DefaultGeminiBaseURL)
	cfg.APIKey = ""
	p := NewGemini(cfg, client)
	if _, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "p"}); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("image: want ErrNoAPIKey, got %v", err)
	}
	if _, err := p.GenerateText(context.Background(), TextRequest{Prompt: "p"}); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("text: want ErrNoAPIKey, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("missing key produced %d outbound calls", calls)
	}
}

// --- 视频模型只保留目录，不产生外呼（能力校验在外呼前拦截） ---

func TestGeminiVideoCatalogStaysReserved(t *testing.T) {
	calls := 0
	prevDefault := http.DefaultClient
	http.DefaultClient = fakeClient(func(r *http.Request) (*http.Response, error) {
		calls++
		t.Errorf("video gate reached the network: %s%s", r.URL.Host, r.URL.Path)
		return jsonResp(200, map[string]any{}), nil
	})
	t.Cleanup(func() { http.DefaultClient = prevDefault })

	cfg := geminiConfig(DefaultGeminiBaseURL)
	cfg.VideoModels = []string{"veo-3.0-generate-preview"} // 预留视频目录：不代表可调用
	if caps := NewGemini(cfg, nil).Capabilities(); caps.Video {
		t.Fatal("adapter reports video capability though no video adapter executes calls")
	}
	err := cfg.ValidateVideoGeneration()
	if !errors.Is(err, ErrCapabilityUnsupported) {
		t.Fatalf("ValidateVideoGeneration = %v, want ErrCapabilityUnsupported", err)
	}
	if _, err := cfg.ResolveValidatedModel(ModalityVideo, "veo-3.0-generate-preview"); !errors.Is(err, ErrCapabilityUnsupported) {
		t.Fatalf("explicit reserved video model resolved instead of gated: %v", err)
	}
	if calls != 0 {
		t.Fatalf("video gating made %d outbound calls", calls)
	}
}
