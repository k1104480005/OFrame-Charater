package provider

import (
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

// volcConfig builds a 火山方舟 (volcengine/Ark) config mirroring what the
// FrameBaker preset produces: explicit protocol Type, Ark BaseURL, catalogs,
// key. Catalog-only (no legacy singular fields) like a real preset draft.
func volcConfig(baseURL string) ProviderConfig {
	return ProviderConfig{
		ProviderID:  "volcengine-main",
		Type:        ProviderTypeVolcengine,
		Name:        "我的火山方舟",
		APIKey:      "ark-secret-key",
		BaseURL:     baseURL,
		ImageModels: []string{DefaultDoubaoModel},      // doubao-seedream-4-0
		VideoModels: []string{DefaultDoubaoVideoModel}, // doubao-seedance-1-0-pro：预留目录
		TextModels:  []string{DefaultDoubaoTextModel},  // doubao-1-5-pro-32k
	}
}

// --- 身份与能力（与 DeclaredCapabilities 保持一致；视频恒为未声明） ---

func TestVolcengineIdentityAndCapabilities(t *testing.T) {
	cfg := volcConfig(DefaultDoubaoBaseURL)
	p := NewVolcengine(cfg, nil)
	if p.ID() != "volcengine-main" {
		t.Fatalf("ID = %q, want configured provider id", p.ID())
	}
	if p.Name() != "我的火山方舟" {
		t.Fatalf("Name = %q, want display name", p.Name())
	}
	if got := NewVolcengine(ProviderConfig{ProviderID: "volcengine-main", Type: ProviderTypeVolcengine}, nil).Name(); got != "火山方舟 / 豆包" {
		t.Fatalf("fallback Name = %q", got)
	}
	caps := p.Capabilities()
	want := cfg.DeclaredCapabilities()
	if caps != want {
		t.Fatalf("live capabilities %v drift from declared %v", caps, want)
	}
	if !caps.Image || !caps.Text || caps.Video {
		t.Fatalf("volcengine must declare image+text and never video, got %+v", caps)
	}
	// Catalog-only configs (legacy singular fields empty) resolve their
	// defaults from the legacy fields only — they stay empty here.
	if p.DefaultImageModel() != "" || p.DefaultTextModel() != "" {
		t.Fatalf("legacy defaults should stay empty for catalog-only configs: %q/%q", p.DefaultImageModel(), p.DefaultTextModel())
	}
	withLegacy := cfg
	withLegacy.Model = "legacy-seedream"
	withLegacy.TextModel = "legacy-pro"
	if got := NewVolcengine(withLegacy, nil).DefaultImageModel(); got != "legacy-seedream" {
		t.Fatalf("DefaultImageModel with legacy field = %q", got)
	}
	if got := NewVolcengine(withLegacy, nil).DefaultTextModel(); got != "legacy-pro" {
		t.Fatalf("DefaultTextModel with legacy field = %q", got)
	}
}

// TestNewAdapterRoutesVolcengineExplicitly pins the additive registry contract
// after task 2.5: every previously-routed type keeps its exact adapter,
// volcengine gains its own Ark adapter (never a silent compatible or DashScope
// fallback), and unknown types still fail explicitly.
func TestNewAdapterRoutesVolcengineExplicitly(t *testing.T) {
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
		{ProviderTypeMiniMax, &MiniMax{}},
		{ProviderTypeVolcengine, &Volcengine{}},
	}
	for _, tc := range cases {
		p, err := NewAdapter("p1", mk(tc.typ), nil)
		if err != nil {
			t.Fatalf("type %q: unexpected error %v", tc.typ, err)
		}
		// Built-ins keep their hard-coded vendor identity; custom protocol
		// types use the configured provider id.
		wantID := "p1"
		if tc.typ == ProviderDoubao || tc.typ == ProviderOpenAI || tc.typ == ProviderAgnes {
			wantID = tc.typ
		}
		if id := p.ID(); id != wantID {
			t.Errorf("type %q: adapter ID = %q, want %q", tc.typ, id, wantID)
		}
		// 静默套用其他协议会在这里以具体类型变化暴露出来：volcengine 绝不允许
		// 落到 *Compatible 或 *Dashscope。
		if fmt.Sprintf("%T", p) != fmt.Sprintf("%T", tc.want) {
			t.Errorf("type %q routed to %T, want %T — 协议身份不得被另一协议顶替", tc.typ, p, tc.want)
		}
	}
	vp, err := NewAdapter("p1", mk(ProviderTypeVolcengine), nil)
	if err != nil {
		t.Fatal(err)
	}
	switch vp.(type) {
	case *Compatible, *Dashscope:
		t.Fatal("volcengine leaked into another protocol adapter")
	}
	if _, err := NewAdapter("p1", ProviderConfig{ProviderID: "p1", Type: "mystery"}, nil); err == nil {
		t.Error("unknown types must still be rejected")
	}
}

// --- 图片请求面：path/model、认证头、超时、body 形状、默认尺寸、外协议字段零泄漏 ---

func TestVolcengineGenerateImageRequestShape(t *testing.T) {
	var gotMethod, gotPath, gotRawQuery, gotAuth, gotContentType, gotAccept string
	var googKey, dashAsync string
	sawDeadline := false
	var gotBody map[string]any
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotRawQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		googKey = r.Header.Get("x-goog-api-key")
		dashAsync = r.Header.Get(headerDashscopeAsync)
		if _, ok := r.Context().Deadline(); ok {
			sawDeadline = true
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		return jsonResp(200, map[string]any{
			"model":   DefaultDoubaoModel,
			"created": 1725000000,
			"data":    []map[string]any{{"b64_json": fakePNG}},
		}), nil
	})
	res, err := NewVolcengine(volcConfig(DefaultDoubaoBaseURL), client).GenerateImage(context.Background(),
		ImageRequest{Prompt: "一只戴宇航头盔的橘猫，赛博朋克霓虹"})
	if err != nil {
		t.Fatal(err)
	}
	// 端点与认证：POST {base}/images/generations + 官方 Bearer（Ark 风格），
	// 不带 Gemini 的 key 头，也不带 DashScope 的异步头。
	if gotMethod != http.MethodPost || gotPath != "/api/v3"+arkImagesGenerationsPath {
		t.Fatalf("endpoint drifted: %s %s", gotMethod, gotPath)
	}
	if gotRawQuery != "" {
		t.Errorf("unexpected query string %q", gotRawQuery)
	}
	if gotAuth != "Bearer ark-secret-key" {
		t.Fatalf("auth = %q, want official Bearer", gotAuth)
	}
	if googKey != "" || dashAsync != "" {
		t.Errorf("foreign protocol headers leaked: x-goog-api-key=%q X-DashScope-Async=%q", googKey, dashAsync)
	}
	if gotContentType != "application/json" || gotAccept != "application/json" {
		t.Fatalf("content headers drifted: %q/%q", gotContentType, gotAccept)
	}
	if !sawDeadline {
		t.Error("expected an adapter-applied deadline on the provider call")
	}
	// 请求体：明确携带 model/prompt/size；response_format 固定 b64_json；
	// watermark 显式 false（分镜条要做像素级切片验收）。
	if gotBody["model"] != DefaultDoubaoModel { // catalog head as default model
		t.Errorf("model = %v, want %s", gotBody["model"], DefaultDoubaoModel)
	}
	if gotBody["prompt"] != "一只戴宇航头盔的橘猫，赛博朋克霓虹" {
		t.Errorf("prompt = %v", gotBody["prompt"])
	}
	if gotBody["size"] != "1024x1024" { // 默认尺寸映射为 WxH 字符串
		t.Errorf("size = %v, want 1024x1024 (WxH)", gotBody["size"])
	}
	if gotBody["response_format"] != arkResponseFormatB64JSON {
		t.Errorf("response_format = %v, want b64_json", gotBody["response_format"])
	}
	if wm, ok := gotBody["watermark"]; !ok || wm != false {
		t.Errorf("watermark = %v, want explicit false in the body", gotBody["watermark"])
	}
	for _, banned := range []string{
		"aspect_ratio",      // 其他适配器的尺寸替代字段
		"reference_images",  // 兼容表面的引用图字段
		"subject_reference", // MiniMax 的引用图字段
		"input",             // DashScope 原生包装
		"parameters",        // DashScope 原生包装
		"contents",          // Gemini generateContent 包装
		"inlineData",        // Gemini inline 数据
	} {
		if _, ok := gotBody[banned]; ok {
			t.Errorf("foreign protocol field %q leaked into the ark body", banned)
		}
	}
	_, hasImage := gotBody["image"]
	if hasImage {
		t.Errorf("image field must stay absent for zero references")
	}
	// 响应解析（data[].b64_json 官方内联变体）。
	wantPNG, _ := base64.StdEncoding.DecodeString(fakePNG)
	if !stringsEqualBytes(res.Data, wantPNG) || res.MIME != "image/png" {
		t.Fatalf("decoded image mismatch: mime=%q len=%d", res.MIME, len(res.Data))
	}
	if res.Provider != "volcengine-main" || res.Model != DefaultDoubaoModel {
		t.Fatalf("result meta = %q/%q", res.Provider, res.Model)
	}
}

// TestVolcengineExplicitSizeMapping pins Width/Height → size "WxH"。
func TestVolcengineExplicitSizeMapping(t *testing.T) {
	cases := []struct {
		w, h int
		want string
	}{
		{800, 600, "800x600"},
		{1920, 1080, "1920x1080"},
		{-5, 0, "1024x1024"}, // 未设置的方向回退到生成默认尺寸
	}
	for _, tc := range cases {
		var gotSize any
		client := fakeClient(func(r *http.Request) (*http.Response, error) {
			var body struct {
				Size string `json:"size"`
			}
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &body)
			gotSize = body.Size
			return jsonResp(200, map[string]any{"data": []map[string]any{{"b64_json": fakePNG}}}), nil
		})
		cfg := volcConfig(DefaultDoubaoBaseURL)
		if _, err := NewVolcengine(cfg, client).GenerateImage(context.Background(), ImageRequest{Prompt: "p", Width: tc.w, Height: tc.h}); err != nil {
			t.Fatalf("w=%d h=%d: %v", tc.w, tc.h, err)
		}
		if gotSize != tc.want {
			t.Errorf("W%d H%d size = %v, want %q", tc.w, tc.h, gotSize, tc.want)
		}
	}
}

// --- 引用图按 Seedream 约定进 "image" 数组：多引用有序、纯字符串条目 ---

func TestVolcengineReferenceImages(t *testing.T) {
	refs := []ReferenceImage{
		{Kind: "reference_image", Role: "main_reference", MIME: "image/jpeg", Data: []byte("JPGDATA")},
		{Kind: "sprite", Role: "auxiliary_reference", MIME: "", Data: []byte{0x89, 'S'}},
	}

	t.Run("multiple references attach as ordered base64 data URLs without role wrappers", func(t *testing.T) {
		var gotImage []any
		client := fakeClient(func(r *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			raw, _ := m["image"].([]any)
			for _, e := range raw {
				gotImage = append(gotImage, e)
			}
			return jsonResp(200, map[string]any{"data": []map[string]any{{"b64_json": fakePNG}}}), nil
		})
		cfg := volcConfig(DefaultDoubaoBaseURL)
		res, err := NewVolcengine(cfg, client).GenerateImage(context.Background(),
			ImageRequest{Prompt: "同一角色在雨中", References: refs})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{
			"data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte("JPGDATA")),
			"data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte{0x89, 'S'}), // 空 MIME 默认 png
		}
		if len(gotImage) != len(want) {
			t.Fatalf("image = %v, want %d ordered entries", gotImage, len(want))
		}
		for i, w := range want {
			if gotImage[i] != w {
				t.Errorf("image[%d] = %v, want %q", i, gotImage[i], w)
			}
		}
		if res.Provider != "volcengine-main" {
			t.Fatalf("result meta = %+v", res)
		}
	})

	t.Run("unsupported kind rejects pre-flight with zero calls", func(t *testing.T) {
		calls := 0
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			calls++
			return jsonResp(200, map[string]any{}), nil
		})
		bad := []ReferenceImage{{Kind: "banner", MIME: "image/png", Data: []byte("x")}}
		_, err := NewVolcengine(volcConfig(DefaultDoubaoBaseURL), client).GenerateImage(context.Background(), ImageRequest{Prompt: "p", References: bad})
		if err == nil || !strings.Contains(err.Error(), `unsupported reference image kind "banner"`) {
			t.Fatalf("expected clear kind rejection, got %v", err)
		}
		if !strings.Contains(err.Error(), "volcengine-main") {
			t.Errorf("rejection should name the provider id: %v", err)
		}
		if calls != 0 {
			t.Fatalf("pre-flight kind failure issued %d outbound calls", calls)
		}
		if !IsNotRetryable(err) {
			t.Fatalf("kind rejection must be final: %v", err)
		}
	})

	t.Run("empty payload rejects pre-flight with zero calls", func(t *testing.T) {
		calls := 0
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			calls++
			return jsonResp(200, map[string]any{}), nil
		})
		empty := []ReferenceImage{{Kind: "reference_image", MIME: "image/png"}}
		_, err := NewVolcengine(volcConfig(DefaultDoubaoBaseURL), client).GenerateImage(context.Background(), ImageRequest{Prompt: "p", References: empty})
		if err == nil || !strings.Contains(err.Error(), "carries no data") {
			t.Fatalf("expected empty-payload rejection, got %v", err)
		}
		if calls != 0 {
			t.Fatalf("pre-flight payload failure issued %d outbound calls", calls)
		}
	})
}

// --- 成功响应变体：官方数组、扁平对象、坏条目回退、URL 下载不带凭证 ---

func TestVolcengineSuccessVariants(t *testing.T) {
	type variant struct {
		name       string
		resp       func(*testing.T) map[string]any // 图像端点返回体（nil 表示原始 rawBody）
		rawBody    string                          // 与 resp 二选一
		cdnBody    string                          // CDN 响应字节（空 → 不应有下载）
		cdnType    string
		wantData   string
		wantMIME   string
		wantErr    []string
		notIn      []string
		notRtry    bool
		wantCDNHit bool
	}
	cases := []variant{
		{
			name: "official data[].b64_json stays inline",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": []map[string]any{{"b64_json": fakePNG}}}
			},
			wantData: fakePNGDecode(t),
			wantMIME: "image/png",
		},
		{
			name: "official data[].url downloads without credentials",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": []map[string]any{{"url": "http://cdn.ark.test/one.png"}}}
			},
			cdnBody:    "URL-BYTES",
			cdnType:    "image/png",
			wantData:   "URL-BYTES",
			wantMIME:   "image/png",
			wantCDNHit: true,
		},
		{
			name: "url item keeps the vendor-reported mime",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": []map[string]any{{"size": "1024x1024", "url": "http://cdn.ark.test/two.jpg"}}}
			},
			cdnBody:    "JPEG-BYTES",
			cdnType:    "image/jpeg",
			wantData:   "JPEG-BYTES",
			wantMIME:   "image/jpeg",
			wantCDNHit: true,
		},
		{
			name: "flattened data.b64_json object tolerated",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": map[string]any{"b64_json": fakePNG}}
			},
			wantData: fakePNGDecode(t),
			wantMIME: "image/png",
		},
		{
			name: "flattened data.url object tolerated",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": map[string]any{"url": "http://cdn.ark.test/three.webp"}}
			},
			cdnBody:    "FLAT-BYTES",
			cdnType:    "image/webp",
			wantData:   "FLAT-BYTES",
			wantMIME:   "image/webp",
			wantCDNHit: true,
		},
		{
			name: "broken inline entry falls through to later url candidate",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": []map[string]any{{"b64_json": "@@not-base64@@"}, {"url": "http://cdn.ark.test/four.png"}}}
			},
			cdnBody:    "FALLBACK-BYTES",
			cdnType:    "image/png",
			wantData:   "FALLBACK-BYTES",
			wantMIME:   "image/png",
			wantCDNHit: true,
		},
		{
			name: "only broken inline entries fails readably without extra network use",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": []map[string]any{{"b64_json": "!!!"}}}
			},
			wantErr: []string{"decode ark b64 image"},
		},
		{
			name:    "missing data entirely fails readably",
			rawBody: `{"model":"doubao-seedream-4-0","created":1725000000}`,
			wantErr: []string{"has no image data"},
		},
		{
			name: "usable-shaped but empty item fails readably",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": []map[string]any{{}}}
			},
			wantErr: []string{"no usable image"},
		},
		{
			name:    "gateway 200 with gateway-style error envelope surfaces vendor reason",
			rawBody: `{"error":{"code":"InternalError","message":"upstream exploded: ark-secret-key"}}`,
			wantErr: []string{"ark images API error", "upstream exploded", "***"},
			notIn:   []string{"ark-secret-key"},
		},
	}
	for i := range cases {
		tc := cases[i]
		t.Run(tc.name, func(t *testing.T) {
			var cdnHits int
			var cdnAuth string
			client := fakeClient(func(r *http.Request) (*http.Response, error) {
				switch {
				case strings.HasSuffix(r.URL.Path, arkImagesGenerationsPath):
					if tc.resp != nil {
						return jsonResp(200, tc.resp(t)), nil
					}
					return rawJSONResp(200, tc.rawBody), nil
				default:
					cdnHits++
					cdnAuth = r.Header.Get("Authorization")
					return &http.Response{
						StatusCode: 200,
						Header:     http.Header{"Content-Type": []string{tc.cdnType}},
						Body:       io.NopCloser(strings.NewReader(tc.cdnBody)),
					}, nil
				}
			})
			p := NewVolcengine(volcConfig(DefaultDoubaoBaseURL), client)
			res, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
			if len(tc.wantErr) > 0 {
				if err == nil {
					t.Fatalf("expected error containing %v", tc.wantErr)
				}
				msg := err.Error()
				for _, part := range tc.wantErr {
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
					t.Errorf("IsNotRetryable = %v, want %v", IsNotRetryable(err), tc.notRtry)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantCDNHit {
				if cdnHits != 1 {
					t.Fatalf("CDN hits = %d, want 1", cdnHits)
				}
				if cdnAuth != "" {
					t.Errorf("CDN download carried credentials: %q", cdnAuth)
				}
			} else if cdnHits != 0 {
				t.Fatalf("unexpected CDN hit (%d): inline payload should be used directly", cdnHits)
			}
			if string(res.Data) != tc.wantData {
				t.Fatalf("bytes mismatch: len=%d want=%q", len(res.Data), tc.wantData)
			}
			if res.MIME != tc.wantMIME {
				t.Fatalf("mime = %q, want %q", res.MIME, tc.wantMIME)
			}
		})
	}
}

// --- 错误响应可读、密钥脱敏、401/403 不重试、上限与预检边界 ---

func TestVolcengineErrorHandling(t *testing.T) {
	origCap := maxGenerationResponseBytes
	defer func() { maxGenerationResponseBytes = origCap }()

	type run struct {
		name      string
		setup     func(*testing.T) (*http.Client, int) // client + maxGen override (0 = keep)
		wantParts []string
		notIn     []string
		notRtry   bool
	}
	cases := []run{
		{
			// 共享层契约：错误里回显的是调用者自己的配置密钥，必须脱敏。
			name: "HTTP 401 stops as auth failure, key redacted, not retryable",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawJSONResp(http.StatusUnauthorized, `{"error":{"message":"invalid bearer token ark-secret-key"}}`), nil
				}), 0
			},
			wantParts: []string{"auth failed", "HTTP 401", "bearer token", "***"},
			notIn:     []string{"ark-secret-key"},
			notRtry:   true,
		},
		{
			name: "HTTP 403 forbidden is also a credential failure",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawJSONResp(http.StatusForbidden, `{"error":{"message":"permission denied for key ark-secret-key"}}`), nil
				}), 0
			},
			wantParts: []string{"auth failed", "HTTP 403", "***"},
			notIn:     []string{"ark-secret-key"},
			notRtry:   true,
		},
		{
			name: "HTTP 401 with unparseable body echoes a truncated secret-free detail",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawResp(http.StatusUnauthorized, "text/plain", "denied: ark-secret-key"), nil
				}), 0
			},
			wantParts: []string{"auth failed", "HTTP 401", "denied:", "***"},
			notIn:     []string{"ark-secret-key"},
			notRtry:   true,
		},
		{
			name: "HTTP 500 html echo stays retryable without leaking the key",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawResp(http.StatusInternalServerError, "text/html", `<html>boom ark-secret-key</html>`), nil
				}), 0
			},
			wantParts: []string{"unexpected status 500", "boom"},
			notIn:     []string{"ark-secret-key"},
			notRtry:   false,
		},
		{
			name: "oversize 200 hits the shared response-size cap",
			setup: func(t *testing.T) (*http.Client, int) {
				body := strings.Repeat("x", (1<<20)+16)
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawResp(200, "application/json", body), nil
				}), 1 << 20
			},
			wantParts: []string{"too large"},
			notRtry:   false,
		},
		{
			name: "malformed 200 body decodes into a readable failure",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawResp(200, "application/json", "{not-json"), nil
				}), 0
			},
			wantParts: []string{"decode ark images response"},
			notRtry:   false,
		},
	}
	for i := range cases {
		tc := cases[i]
		t.Run(tc.name, func(t *testing.T) {
			client, genCap := tc.setup(t)
			if genCap > 0 {
				maxGenerationResponseBytes = int64(genCap)
				defer func() { maxGenerationResponseBytes = origCap }()
			}
			p := NewVolcengine(volcConfig(DefaultDoubaoBaseURL), client)
			_, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
			maxGenerationResponseBytes = origCap
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
		})
	}
}

// --- 预检拒绝全部零外呼：空 prompt、模型目录为空、密钥缺失 ---

func TestVolcenginePreFlightRejectionsZeroNetwork(t *testing.T) {
	t.Run("empty prompt rejected before any outbound call", func(t *testing.T) {
		calls := 0
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			calls++
			return jsonResp(200, map[string]any{"data": []map[string]any{{"b64_json": fakePNG}}}), nil
		})
		_, err := NewVolcengine(volcConfig(DefaultDoubaoBaseURL), client).GenerateImage(context.Background(), ImageRequest{Prompt: "   "})
		if err == nil || !strings.Contains(err.Error(), "requires a prompt") {
			t.Fatalf("expected prompt rejection, got %v", err)
		}
		if calls != 0 {
			t.Fatalf("pre-flight prompt failure issued %d outbound calls", calls)
		}
		if !IsNotRetryable(err) {
			t.Fatalf("empty-prompt rejection must be final: %v", err)
		}
	})

	t.Run("empty image catalog rejected before any outbound call", func(t *testing.T) {
		calls := 0
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			calls++
			return jsonResp(200, map[string]any{}), nil
		})
		cfg := volcConfig(DefaultDoubaoBaseURL)
		cfg.ImageModels = nil
		cfg.Model = "" // 明确清掉旧字段回退
		_, err := NewVolcengine(cfg, client).GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
		if err == nil || !strings.Contains(err.Error(), "no image model configured") {
			t.Fatalf("expected pre-flight model error, got %v", err)
		}
		if calls != 0 {
			t.Fatalf("pre-flight model failure issued %d outbound calls", calls)
		}
		if !IsNotRetryable(err) {
			t.Fatalf("no-model rejection must be final: %v", err)
		}
	})

	t.Run("missing key fails both modalities with zero calls", func(t *testing.T) {
		t.Setenv("OFRAME_VOLCENGINE-MAIN_API_KEY", "")
		calls := 0
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			calls++
			return jsonResp(200, map[string]any{}), nil
		})
		cfg := volcConfig(DefaultDoubaoBaseURL)
		cfg.APIKey = ""
		p := NewVolcengine(cfg, client)
		if _, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "p"}); !errors.Is(err, ErrNoAPIKey) {
			t.Fatalf("image: want ErrNoAPIKey, got %v", err)
		}
		if _, err := p.GenerateText(context.Background(), TextRequest{Prompt: "p"}); !errors.Is(err, ErrNoAPIKey) {
			t.Fatalf("text: want ErrNoAPIKey, got %v", err)
		}
		if calls != 0 {
			t.Fatalf("missing key produced %d outbound calls", calls)
		}
	})
}

// --- 文本生成：Ark 自有 chat 表面（路径/认证/body 锁定），能力真实可用 ---

func TestVolcengineTextGeneration(t *testing.T) {
	t.Run("chat completion succeeds under the ark identity", func(t *testing.T) {
		var gotPath, gotAuth string
		var gotBody map[string]any
		client := fakeClient(func(r *http.Request) (*http.Response, error) {
			gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			return jsonResp(200, map[string]any{
				"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "好的，开始规划分镜。"}}},
			}), nil
		})
		res, err := NewVolcengine(volcConfig(DefaultDoubaoBaseURL), client).GenerateText(context.Background(), TextRequest{Prompt: "规划"})
		if err != nil {
			t.Fatal(err)
		}
		if gotPath != "/api/v3/chat/completions" {
			t.Errorf("path = %s, want Ark chat endpoint", gotPath)
		}
		if gotAuth != "Bearer ark-secret-key" {
			t.Errorf("auth = %q, want Bearer", gotAuth)
		}
		msgs, _ := gotBody["messages"].([]any)
		first, _ := msgs[0].(map[string]any)
		if gotBody["model"] != DefaultDoubaoTextModel || first["content"] != "规划" {
			t.Fatalf("request body drifted: %v", gotBody)
		}
		if res.Text != "好的，开始规划分镜。" || res.Provider != "volcengine-main" || res.Model != DefaultDoubaoTextModel {
			t.Fatalf("result = %+v", res)
		}
	})

	t.Run("in-band chat error surfaces readably", func(t *testing.T) {
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			return jsonResp(200, map[string]any{"error": map[string]any{"message": "rate limit exceeded"}}), nil
		})
		_, err := NewVolcengine(volcConfig(DefaultDoubaoBaseURL), client).GenerateText(context.Background(), TextRequest{Prompt: "hi"})
		if err == nil || !strings.Contains(err.Error(), "rate limit exceeded") {
			t.Fatalf("expected readable chat error, got %v", err)
		}
	})

	t.Run("no text model anywhere fails before outbound", func(t *testing.T) {
		calls := 0
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			calls++
			return jsonResp(200, map[string]any{}), nil
		})
		cfg := volcConfig(DefaultDoubaoBaseURL)
		cfg.TextModels = nil
		cfg.TextModel = "" // 明确清掉旧字段回退
		_, err := NewVolcengine(cfg, client).GenerateText(context.Background(), TextRequest{Prompt: "x"})
		if err == nil || !strings.Contains(err.Error(), "no text model configured") {
			t.Fatalf("expected pre-flight model error, got %v", err)
		}
		if calls != 0 {
			t.Fatalf("pre-flight model failure issued %d outbound calls", calls)
		}
		if !IsNotRetryable(err) {
			t.Fatalf("no-model rejection must be final: %v", err)
		}
	})
}

// --- 视频仅保留配置接口：能力恒未声明、预留目录不可解析、零外呼 ---

func TestVolcengineVideoCatalogStaysZeroCall(t *testing.T) {
	calls := 0
	prevDefault := http.DefaultClient
	http.DefaultClient = fakeClient(func(r *http.Request) (*http.Response, error) {
		calls++
		t.Errorf("gated request reached the network: %s%s", r.URL.Host, r.URL.Path)
		return jsonResp(200, map[string]any{}), nil
	})
	t.Cleanup(func() { http.DefaultClient = prevDefault })

	cfg := volcConfig(DefaultDoubaoBaseURL)
	cfg.VideoModels = []string{DefaultDoubaoVideoModel} // Seedance 预留目录：只作元数据
	p := NewVolcengine(cfg, nil)

	if caps := p.Capabilities(); caps.Video {
		t.Fatal("adapter reports video capability though no video adapter executes calls")
	}
	if err := cfg.ValidateVideoGeneration(); !errors.Is(err, ErrCapabilityUnsupported) {
		t.Fatalf("ValidateVideoGeneration = %v, want ErrCapabilityUnsupported", err)
	}
	if _, err := cfg.ResolveValidatedModel(ModalityVideo, DefaultDoubaoVideoModel); !errors.Is(err, ErrCapabilityUnsupported) {
		t.Fatalf("explicit reserved video model resolved instead of gated: %v", err)
	}
	if calls != 0 {
		t.Fatalf("video gating made %d outbound calls", calls)
	}
}

// --- 协议身份证明：Volcengine ≠ Doubao/兼容表面，也 ≠ DashScope 原生 ---
//
// 同一请求分别走三个适配器并记录各自的线协议。Ark 与 OpenAI 兼容表面共享
// /images/generations 路径家族——所以协议差异必须由 body/字段契约证明：
// 引用图字段（image vs reference_images）、尺寸字段位置、watermark/
// response_format 取值互斥；而 DashScope 原生更是完全不同的端点与包装。

func TestVolcengineWireIdentityDiffersFromCompatibleAndDashscope(t *testing.T) {
	const prompt = "同一角色在雨中"
	refRefs := []ReferenceImage{
		{Kind: "reference_image", Role: "main_reference", MIME: "image/png", Data: []byte("abc")},
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("abc"))

	var (
		method                  string
		arkPath, dbPath, dsPath string
		arkBody, dbBody, dsBody map[string]any
	)
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		method = r.Method
		body, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, arkImagesGenerationsPath):
			// 共享的请求路径家族被 Ark 与兼容表面先后使用——按 body 里的
			// watermark 键区分（Ark 恒发、兼容表面从不发）。
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			if _, seen := m["watermark"]; seen {
				arkPath, arkBody = r.URL.Path, m
			} else {
				dbPath, dbBody = r.URL.Path, m
			}
			return jsonResp(200, map[string]any{"data": []map[string]any{{"b64_json": fakePNG}}}), nil
		case strings.HasSuffix(r.URL.Path, dashscopeNativeImagePath):
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			dsPath, dsBody = r.URL.Path, m
			return jsonResp(200, map[string]any{
				"output": map[string]any{"task_status": "SUCCEEDED", "results": []map[string]any{{"b64_json": fakePNG}}},
			}), nil
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			return jsonResp(404, map[string]any{}), nil
		}
	})

	// Leg 1: Volcengine（带引用图）
	_, err := NewVolcengine(volcConfig(DefaultDoubaoBaseURL), client).GenerateImage(context.Background(),
		ImageRequest{Prompt: prompt, Width: 512, Height: 512, References: refRefs})
	if err != nil {
		t.Fatalf("ark leg: %v", err)
	}
	// Leg 2: 内置 Doubao（OpenAI 兼容表面，同样带引用图）
	dcfg := DefaultConfig(ProviderDoubao)
	dcfg.APIKey = "db-secret-key"
	_, err = NewDoubao(dcfg, client).GenerateImage(context.Background(),
		ImageRequest{Prompt: prompt, Width: 512, Height: 512, References: refRefs})
	if err != nil {
		t.Fatalf("doubao leg: %v", err)
	}
	// Leg 3: DashScope 原生（无引用图——原生表面对引用图本就前置拒绝）
	_, err = NewDashscope(dashConfig(DefaultDashscopeBaseURL), client).GenerateImage(context.Background(),
		ImageRequest{Prompt: prompt, Width: 512, Height: 512})
	if err != nil {
		t.Fatalf("dashscope leg: %v", err)
	}

	if method != http.MethodPost {
		t.Fatalf("method = %s", method)
	}
	// 路径证据：三条线各走各的路——DashScope 原生完全不同的端点；
	// Ark 与兼容表面虽同族（/images/generations），但身份由下述字段区分。
	if arkPath != "/api/v3/images/generations" {
		t.Errorf("ark path = %q", arkPath)
	}
	if dbPath != "/api/v3/images/generations" {
		t.Errorf("compatible-family path = %q", dbPath)
	}
	if dsPath != "/api/v1/services/aigc/text2image/image-synthesis" {
		t.Errorf("dashscope path = %q, want the native synthesis endpoint", dsPath)
	}
	if dsBody != nil && dsBody["input"] == nil {
		t.Errorf("dashscope native body lost the input wrapper: %v", dsBody)
	}
	if params, ok := dsBody["parameters"].(map[string]any); ok {
		if params["size"] != "512*512" {
			t.Errorf("dashscope size separator drifted: %v (want 512*512, NOT ark's 512x512)", params["size"])
		}
	} else {
		t.Errorf("dashscope native body lost the parameters block: %v", dsBody)
	}

	// Body 证据 ①：Ark 的引用图走 "image" 纯字符串数组，绝无 role 包装对象。
	imgList, ok := arkBody["image"].([]any)
	if !ok || len(imgList) != 1 || imgList[0] != dataURL {
		t.Fatalf("ark image refs = %v, want [%s]", arkBody["image"], dataURL)
	}
	if _, leaked := arkBody["reference_images"]; leaked {
		t.Error("ark body leaked the compatible surface's reference_images contract")
	}
	// Body 证据 ②：兼容表面走它自己的 reference_images 对象数组 + response_format
	// png + n=1，且从不发 image/watermark —— 两套契约互斥，谁顶替谁都立刻现形。
	dbRefs, ok := dbBody["reference_images"].([]any)
	if !ok || len(dbRefs) != 1 {
		t.Fatalf("compatible reference_images = %v", dbBody["reference_images"])
	}
	if first, mok := dbRefs[0].(map[string]any); !mok || first["role"] != "main_reference" {
		t.Errorf("compatible refs keep role objects, got %v", dbRefs[0])
	}
	if got := dbBody["response_format"]; got != "png" {
		t.Errorf("compatible response_format = %v, want png", got)
	}
	if !eqNum(dbBody["n"], 1) {
		t.Errorf("compatible n = %v, want 1", dbBody["n"])
	}
	if _, leaked := dbBody["image"]; leaked {
		t.Error("compatible body leaked the ark image field")
	}
	if _, leaked := dbBody["watermark"]; leaked {
		t.Error("compatible body leaked the ark watermark field")
	}
	// Body 证据 ③：Ark 固定 b64_json/watermark:false/无 n；三个同时成立才合法。
	if arkBody["response_format"] != arkResponseFormatB64JSON {
		t.Errorf("ark response_format = %v, want b64_json", arkBody["response_format"])
	}
	if wm, present := arkBody["watermark"]; !present || wm != false {
		t.Errorf("ark watermark = %v, want explicit false", arkBody["watermark"])
	}
	if _, leaked := arkBody["n"]; leaked {
		t.Error("ark body must not carry the compatible surface's pinned n field")
	}
	if arkBody["size"] != "512x512" || dbBody["size"] != "512x512" {
		t.Errorf("size strings drifted: ark %v / compatible %v", arkBody["size"], dbBody["size"])
	}
}
