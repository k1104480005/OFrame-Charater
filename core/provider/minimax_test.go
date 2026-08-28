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

// mmConfig builds a MiniMax config mirroring what the FrameBaker preset
// produces: explicit protocol Type, vendor BaseURL, catalogs, key.
func mmConfig(baseURL string) ProviderConfig {
	return ProviderConfig{
		ProviderID:  "minimax-main",
		Type:        ProviderTypeMiniMax,
		Name:        "我的 MiniMax",
		APIKey:      "mm-secret-key",
		BaseURL:     baseURL,
		ImageModels: []string{"image-01"},
	}
}

// --- 身份与能力（与 DeclaredCapabilities 保持一致；视频/文本恒为未声明） ---

func TestMiniMaxIdentityAndCapabilities(t *testing.T) {
	cfg := mmConfig(DefaultMiniMaxBaseURL)
	p := NewMiniMax(cfg, nil)
	if p.ID() != "minimax-main" {
		t.Fatalf("ID = %q, want configured provider id", p.ID())
	}
	if p.Name() != "我的 MiniMax" {
		t.Fatalf("Name = %q, want display name", p.Name())
	}
	if got := NewMiniMax(ProviderConfig{ProviderID: "minimax-main", Type: ProviderTypeMiniMax}, nil).Name(); got != "MiniMax" {
		t.Fatalf("fallback Name = %q", got)
	}
	caps := p.Capabilities()
	want := cfg.DeclaredCapabilities()
	if caps != want {
		t.Fatalf("live capabilities %v drift from declared %v", caps, want)
	}
	if !caps.Image || caps.Video || caps.Text {
		t.Fatalf("minimax must declare image only, got %+v", caps)
	}
	// Catalog-only configs (legacy singular fields empty) resolve their
	// defaults from the effective catalogs, exactly like the other presets.
	if p.DefaultImageModel() != "" || p.DefaultTextModel() != "" {
		t.Fatalf("legacy defaults should stay empty for catalog-only configs: %q/%q", p.DefaultImageModel(), p.DefaultTextModel())
	}
	withLegacy := cfg
	withLegacy.Model = "image-explicit"
	if got := NewMiniMax(withLegacy, nil).DefaultImageModel(); got != "image-explicit" {
		t.Fatalf("DefaultImageModel with legacy field = %q", got)
	}
	if got := NewMiniMax(cfg, nil).DefaultTextModel(); got != "" {
		t.Fatalf("DefaultTextModel = %q, want empty (image-only preset)", got)
	}
}

// TestNewAdapterRoutesMiniMaxExplicitly pins the additive registry contract
// after task 2.4: every previously-routed type keeps its exact adapter,
// minimax gains its own image_generation adapter (never a silent compatible
// fallback), and unknown types still fail explicitly.
func TestNewAdapterRoutesMiniMaxExplicitly(t *testing.T) {
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
		if fmt.Sprintf("%T", p) != fmt.Sprintf("%T", tc.want) {
			t.Errorf("type %q routed to %T, want %T — 协议身份不得被另一协议顶替", tc.typ, p, tc.want)
		}
	}
	if _, err := NewAdapter("p1", ProviderConfig{ProviderID: "p1", Type: "mystery"}, nil); err == nil {
		t.Error("unknown types must still be rejected")
	}
}

// --- 图片请求面：path/model、认证头、超时、body 形状、默认尺寸 ---

func TestMiniMaxGenerateImageRequestShape(t *testing.T) {
	var gotMethod, gotPath, gotRawQuery, gotAuth, gotContentType, gotAccept string
	sawDeadline := false
	var gotBody map[string]any
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotRawQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		if _, ok := r.Context().Deadline(); ok {
			sawDeadline = true
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		return jsonResp(200, map[string]any{
			"id":        "mm-req-1",
			"data":      map[string]any{"image_base64": []string{fakePNG}},
			"base_resp": map[string]any{"status_code": 0, "status_msg": "success"},
		}), nil
	})
	res, err := NewMiniMax(mmConfig(DefaultMiniMaxBaseURL), client).GenerateImage(context.Background(),
		ImageRequest{Prompt: "一只戴宇航头盔的橘猫，赛博朋克霓虹"})
	if err != nil {
		t.Fatal(err)
	}
	// 端点与认证：POST {base}/image_generation + 官方 Bearer（不走兼容协议）。
	if gotMethod != http.MethodPost || gotPath != "/v1"+minimaxImageGenerationPath {
		t.Fatalf("endpoint drifted: %s %s", gotMethod, gotPath)
	}
	if gotRawQuery != "" {
		t.Errorf("unexpected query string %q", gotRawQuery)
	}
	if gotAuth != "Bearer mm-secret-key" {
		t.Fatalf("auth = %q, want official Bearer", gotAuth)
	}
	if gotContentType != "application/json" || gotAccept != "application/json" {
		t.Fatalf("content headers drifted: %q/%q", gotContentType, gotAccept)
	}
	if !sawDeadline {
		t.Error("expected an adapter-applied deadline on the provider call")
	}
	// 请求体：至少 model/prompt；默认尺寸按协议整数像素字段。
	if gotBody["model"] != "image-01" { // catalog head as default model
		t.Errorf("model = %v, want image-01", gotBody["model"])
	}
	if gotBody["prompt"] != "一只戴宇航头盔的橘猫，赛博朋克霓虹" {
		t.Errorf("prompt = %v", gotBody["prompt"])
	}
	if !eqNum(gotBody["width"], 1024) || !eqNum(gotBody["height"], 1024) {
		t.Errorf("size = %v/%v, want 1024/1024 (generation default)", gotBody["width"], gotBody["height"])
	}
	if !eqNum(gotBody["n"], 1) {
		t.Errorf("n = %v, want pinned 1", gotBody["n"])
	}
	if gotBody["response_format"] != "base64" {
		t.Errorf("response_format = %v, want base64", gotBody["response_format"])
	}
	for _, banned := range []string{"aspect_ratio", "size", "reference_images"} {
		if _, ok := gotBody[banned]; ok {
			t.Errorf("foreign protocol field %q leaked into the minimax body", banned)
		}
	}
	if _, ok := gotBody["subject_reference"]; ok {
		t.Errorf("subject_reference must stay absent for zero references")
	}
	// 响应解析（官方 base64 变体）。
	wantPNG, _ := base64.StdEncoding.DecodeString(fakePNG)
	if !stringsEqualBytes(res.Data, wantPNG) || res.MIME != "image/png" {
		t.Fatalf("decoded image mismatch: mime=%q len=%d", res.MIME, len(res.Data))
	}
	if res.Provider != "minimax-main" || res.Model != "image-01" {
		t.Fatalf("result meta = %q/%q", res.Provider, res.Model)
	}
}

func eqNum(v any, want int) bool {
	f, ok := v.(float64)
	return ok && int(f) == want
}

// TestMiniMaxExplicitSizeMapping pins Width/Height onto the vendor pixel
// contract ([512,2048], multiples of 8) and keeps aspect_ratio out.
func TestMiniMaxExplicitSizeMapping(t *testing.T) {
	cases := []struct {
		w, h   int
		wantW  int
		wantH  int
		reason string
	}{
		{800, 600, 800, 600, "in-range aligned dims pass through"},
		{0, -3, 1024, 1024, "unset dims fall back to the generation default"},
		{300, 300, 512, 512, "below-range dims clamp up to 512"},
		{4000, 1020, 2048, 1024, "above-range clamps down; near values round to ×8"},
		{513, 1001, 512, 1000, "values snap to the nearest multiple of 8"},
	}
	for _, tc := range cases {
		var gotW, gotH float64
		var gotAspectRatio bool
		client := fakeClient(func(r *http.Request) (*http.Response, error) {
			var body struct {
				Width       any    `json:"width"`
				Height      any    `json:"height"`
				AspectRatio string `json:"aspect_ratio"`
			}
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &body)
			gotW, _ = body.Width.(float64)
			gotH, _ = body.Height.(float64)
			gotAspectRatio = body.AspectRatio != ""
			return jsonResp(200, map[string]any{"data": map[string]any{"image_base64": []string{fakePNG}}}), nil
		})
		cfg := mmConfig(DefaultMiniMaxBaseURL)
		if _, err := NewMiniMax(cfg, client).GenerateImage(context.Background(), ImageRequest{Prompt: "p", Width: tc.w, Height: tc.h}); err != nil {
			t.Fatalf("w=%d h=%d: %v", tc.w, tc.h, err)
		}
		if int(gotW) != tc.wantW || int(gotH) != tc.wantH {
			t.Errorf("%s: W%d H%d → sent %.0fx%.0f, want %dx%d", tc.reason, tc.w, tc.h, gotW, gotH, tc.wantW, tc.wantH)
		}
		if gotAspectRatio {
			t.Errorf("%s: aspect_ratio must never travel alongside explicit sizes", tc.reason)
		}
	}
}

// --- 单引用图契约：零/一引用按协议支持，超过一个外呼前明确拒绝 ---

func TestMiniMaxReferenceImages(t *testing.T) {
	refs := []ReferenceImage{
		{Kind: "reference_image", Role: "main_reference", MIME: "image/png", Data: []byte("abc")},
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("abc"))

	t.Run("single reference becomes one character subject_reference data URL", func(t *testing.T) {
		var gotRefs []map[string]any
		client := fakeClient(func(r *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			raw, _ := m["subject_reference"].([]any)
			for _, e := range raw {
				gotRefs = append(gotRefs, e.(map[string]any))
			}
			return jsonResp(200, map[string]any{"data": map[string]any{"image_base64": []string{fakePNG}}}), nil
		})
		res, err := NewMiniMax(mmConfig(DefaultMiniMaxBaseURL), client).GenerateImage(context.Background(),
			ImageRequest{Prompt: "同一角色在雨中", References: refs})
		if err != nil {
			t.Fatal(err)
		}
		if len(gotRefs) != 1 {
			t.Fatalf("subject_reference = %v, want exactly one entry", gotRefs)
		}
		if gotRefs[0]["type"] != "character" {
			t.Errorf("subject type = %v, want character", gotRefs[0]["type"])
		}
		if gotRefs[0]["image_file"] != dataURL {
			t.Errorf("image_file = %v, want base64 data URL %q", gotRefs[0]["image_file"], dataURL)
		}
		if res.Model != "image-01" {
			t.Fatalf("result meta = %+v", res)
		}
	})

	t.Run("sprite kind attaches with blank-mime default", func(t *testing.T) {
		var gotFile any
		client := fakeClient(func(r *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			refsOut, _ := m["subject_reference"].([]any)
			first, _ := refsOut[0].(map[string]any)
			gotFile = first["image_file"]
			return jsonResp(200, map[string]any{"data": map[string]any{"image_base64": []string{fakePNG}}}), nil
		})
		sprite := []ReferenceImage{{Kind: "sprite", MIME: "", Data: []byte{0x89, 'S'}}}
		if _, err := NewMiniMax(mmConfig(DefaultMiniMaxBaseURL), client).GenerateImage(context.Background(),
			ImageRequest{Prompt: "p", References: sprite}); err != nil {
			t.Fatal(err)
		}
		want := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte{0x89, 'S'})
		if gotFile != want {
			t.Errorf("sprite image_file = %v, want %q", gotFile, want)
		}
	})

	t.Run("more than one reference rejects before any outbound call", func(t *testing.T) {
		calls := 0
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			calls++
			return jsonResp(200, map[string]any{}), nil
		})
		tooMany := []ReferenceImage{
			{Kind: "reference_image", MIME: "image/png", Data: []byte("a")},
			{Kind: "sprite", MIME: "image/png", Data: []byte("b")},
		}
		cfg := mmConfig(DefaultMiniMaxBaseURL)
		_, err := NewMiniMax(cfg, client).GenerateImage(context.Background(), ImageRequest{Prompt: "p", References: tooMany})
		if err == nil {
			t.Fatal("expected rejection")
		}
		if calls != 0 {
			t.Fatalf("made %d outbound calls before rejecting the multi-reference request", calls)
		}
		for _, part := range []string{"minimax-main", "2 reference images attached", "exactly one"} {
			if !strings.Contains(err.Error(), part) {
				t.Errorf("rejection %q misses %q", err, part)
			}
		}
		if !IsNotRetryable(err) {
			t.Fatalf("multi-reference rejection should not waste retries: %v", err)
		}
	})

	t.Run("unsupported kind rejects pre-flight with zero calls", func(t *testing.T) {
		calls := 0
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			calls++
			return jsonResp(200, map[string]any{}), nil
		})
		bad := []ReferenceImage{{Kind: "banner", MIME: "image/png", Data: []byte("x")}}
		_, err := NewMiniMax(mmConfig(DefaultMiniMaxBaseURL), client).GenerateImage(context.Background(), ImageRequest{Prompt: "p", References: bad})
		if err == nil || !strings.Contains(err.Error(), `unsupported reference image kind "banner"`) {
			t.Fatalf("expected clear kind rejection, got %v", err)
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
		_, err := NewMiniMax(mmConfig(DefaultMiniMaxBaseURL), client).GenerateImage(context.Background(), ImageRequest{Prompt: "p", References: empty})
		if err == nil || !strings.Contains(err.Error(), "carries no data") {
			t.Fatalf("expected empty-payload rejection, got %v", err)
		}
		if calls != 0 {
			t.Fatalf("pre-flight payload failure issued %d outbound calls", calls)
		}
	})

	t.Run("empty prompt rejects pre-flight even with a valid reference", func(t *testing.T) {
		calls := 0
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			calls++
			return jsonResp(200, map[string]any{"data": map[string]any{"image_base64": []string{fakePNG}}}), nil
		})
		_, err := NewMiniMax(mmConfig(DefaultMiniMaxBaseURL), client).GenerateImage(context.Background(),
			ImageRequest{Prompt: "   ", References: refs})
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
}

// --- 成功响应变体：官方数组、OpenAI 风格条目、扁平对象、坏条目回退 ---

func TestMiniMaxSuccessVariants(t *testing.T) {
	type variant struct {
		name       string
		resp       func(t *testing.T) map[string]any // 图像端点返回体（nil 表示自定义）
		rawBody    string                            // 与 resp 二选一：原始响应体
		cdnBody    string                            // CDN 响应字节（空 → 不应有下载）
		cdnType    string
		wantData   string
		wantMIME   string
		wantErr    []string
		notRtry    bool
		wantCDNHit bool
	}
	cases := []variant{
		{
			name: "official data.image_base64 array",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": map[string]any{"image_base64": []string{fakePNG}}}
			},
			wantData: fakePNGDecode(t),
			wantMIME: "image/png",
		},
		{
			name: "official data.image_urls downloads without credentials",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": map[string]any{"image_urls": []string{"http://cdn.mm.test/one.png"}}}
			},
			cdnBody:    "URL-BYTES",
			cdnType:    "image/png",
			wantData:   "URL-BYTES",
			wantMIME:   "image/png",
			wantCDNHit: true,
		},
		{
			name: "OpenAI-style data[].url item",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": []map[string]any{{"url": "http://cdn.mm.test/two.jpg"}}}
			},
			cdnBody:    "ARRAYURL-BYTES",
			cdnType:    "image/jpeg",
			wantData:   "ARRAYURL-BYTES",
			wantMIME:   "image/jpeg",
			wantCDNHit: true,
		},
		{
			name: "OpenAI-style data[].b64_json item",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": []map[string]any{{"b64_json": fakePNG}}}
			},
			wantData: fakePNGDecode(t),
			wantMIME: "image/png",
		},
		{
			name: "proxy-style data[].image_url item",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": []map[string]any{{"image_url": "http://cdn.mm.test/three.webp"}}}
			},
			cdnBody:    "IMAGEURL-BYTES",
			cdnType:    "image/webp",
			wantData:   "IMAGEURL-BYTES",
			wantMIME:   "image/webp",
			wantCDNHit: true,
		},
		{
			name: "flattened data.url object",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": map[string]any{"url": "http://cdn.mm.test/four.png"}}
			},
			cdnBody:    "FLAT-BYTES",
			cdnType:    "image/png",
			wantData:   "FLAT-BYTES",
			wantMIME:   "image/png",
			wantCDNHit: true,
		},
		{
			name:     "flattened data.b64_json object",
			resp:     func(*testing.T) map[string]any { return map[string]any{"data": map[string]any{"b64_json": fakePNG}} },
			wantData: fakePNGDecode(t),
			wantMIME: "image/png",
		},
		{
			name: "broken inline entry falls through to later URL candidate",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": []map[string]any{{"b64_json": "@@not-base64@@"}, {"url": "http://cdn.mm.test/five.png"}}}
			},
			cdnBody:    "FALLBACK-BYTES",
			cdnType:    "image/png",
			wantData:   "FALLBACK-BYTES",
			wantMIME:   "image/png",
			wantCDNHit: true,
		},
		{
			name: "only broken inline entries fails readably without a network call",
			resp: func(*testing.T) map[string]any {
				return map[string]any{"data": []map[string]any{{"b64_json": "@@"}, {"image_base64": "!!!"}}}
			},
			wantErr: []string{"decode minimax base64 image"},
		},
		{
			name:    "usable-shaped but empty data fails readably",
			resp:    func(*testing.T) map[string]any { return map[string]any{"data": []map[string]any{{}}} },
			wantErr: []string{"no usable image"},
		},
		{
			name:    "missing data entirely fails readably",
			rawBody: `{"id":"mm-none","base_resp":{"status_code":0,"status_msg":"success"}}`,
			wantErr: []string{"has no image data"},
		},
	}
	for i := range cases {
		tc := cases[i]
		t.Run(tc.name, func(t *testing.T) {
			var cdnHits int
			var cdnAuth string
			client := fakeClient(func(r *http.Request) (*http.Response, error) {
				switch {
				case strings.HasSuffix(r.URL.Path, minimaxImageGenerationPath):
					if tc.resp != nil {
						return jsonResp(200, map[string]any{
							"id":        "mm-v",
							"data":      tc.resp(t)["data"],
							"base_resp": map[string]any{"status_code": 0, "status_msg": "success"},
						}), nil
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
			p := NewMiniMax(mmConfig(DefaultMiniMaxBaseURL), client)
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

func fakePNGDecode(t *testing.T) string {
	t.Helper()
	return string(mustDecode(t, fakePNG))
}

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("fixture decode failed: %v", err)
	}
	return b
}

// --- 错误与非标准 envelope：base_resp/status_code/status_msg、脱敏、重试分类 ---

func TestMiniMaxNonStandardErrorEnvelope(t *testing.T) {
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
			// HTTP 200 但 base_resp.status_code=1004：官方鉴权失败码，
			// 与 401/403 同类不重试；错误里回显的密钥必须脱敏。
			name: "200 body carries base_resp auth failure 1004, key redacted, not retryable",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawJSONResp(200, `{"id":"e1","base_resp":{"status_code":1004,"status_msg":"account authentication failed for key mm-secret-key"}}`), nil
				}), 0
			},
			wantParts: []string{"status_code 1004", "authentication failed", "***"},
			notIn:     []string{"mm-secret-key"},
			notRtry:   true,
		},
		{
			name: "200 body carries invalid API key 2049 without status_msg",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawJSONResp(200, `{"base_resp":{"status_code":2049,"status_msg":""}}`), nil
				}), 0
			},
			wantParts: []string{"status_code 2049", "(vendor gave no reason)"},
			notRtry:   true,
		},
		{
			name: "rate-limit code 1002 stays retryable and readable",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawJSONResp(200, `{"base_resp":{"status_code":1002,"status_msg":"rate limit triggered, please try again later"}}`), nil
				}), 0
			},
			wantParts: []string{"status_code 1002", "rate limit"},
			notRtry:   false,
		},
		{
			name: "parameter error 2013 surfaces the vendor reason",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawJSONResp(200, `{"base_resp":{"status_code":2013,"status_msg":"invalid input parameters"}}`), nil
				}), 0
			},
			wantParts: []string{"status_code 2013", "invalid input parameters"},
			notRtry:   false,
		},
		{
			name: "gateway-injected error.message envelope stays readable and redacted",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawJSONResp(200, `{"error":{"message":"upstream exploded: mm-secret-key"}}`), nil
				}), 0
			},
			wantParts: []string{"minimax API error", "upstream exploded", "***"},
			notIn:     []string{"mm-secret-key"},
			notRtry:   false,
		},
		{
			name: "HTTP 401 stops as auth failure with redaction (shared helper)",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawJSONResp(http.StatusUnauthorized, `{"base_resp":{"status_code":1004,"status_msg":"bad bearer token mm-secret-key"}}`), nil
				}), 0
			},
			wantParts: []string{"auth failed", "HTTP 401", "***"},
			notIn:     []string{"mm-secret-key"},
			notRtry:   true,
		},
		{
			name: "HTTP 500 html echo stays retryable without leaking the key",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawResp(http.StatusInternalServerError, "text/html", `<html>boom mm-secret-key</html>`), nil
				}), 0
			},
			wantParts: []string{"unexpected status 500", "boom"},
			notIn:     []string{"mm-secret-key"},
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
	}
	for i := range cases {
		tc := cases[i]
		t.Run(tc.name, func(t *testing.T) {
			client, genCap := tc.setup(t)
			if genCap > 0 {
				maxGenerationResponseBytes = int64(genCap)
				defer func() { maxGenerationResponseBytes = origCap }()
			}
			p := NewMiniMax(mmConfig(DefaultMiniMaxBaseURL), client)
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

// --- 密钥缺失：零外呼 ---

func TestMiniMaxMissingKeyPreFlightZeroNetwork(t *testing.T) {
	t.Setenv("OFRAME_MINIMAX-MAIN_API_KEY", "")
	calls := 0
	client := fakeClient(func(*http.Request) (*http.Response, error) {
		calls++
		return jsonResp(200, map[string]any{}), nil
	})
	cfg := mmConfig(DefaultMiniMaxBaseURL)
	cfg.APIKey = ""
	p := NewMiniMax(cfg, client)
	if _, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "p"}); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("image: want ErrNoAPIKey, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("missing key produced %d outbound calls", calls)
	}
}

// --- 视频仅保留配置接口、文本显式不支持：均零外呼 ---

func TestMiniMaxVideoAndTextStayZeroCall(t *testing.T) {
	calls := 0
	prevDefault := http.DefaultClient
	http.DefaultClient = fakeClient(func(r *http.Request) (*http.Response, error) {
		calls++
		t.Errorf("gated request reached the network: %s%s", r.URL.Host, r.URL.Path)
		return jsonResp(200, map[string]any{}), nil
	})
	t.Cleanup(func() { http.DefaultClient = prevDefault })

	cfg := mmConfig(DefaultMiniMaxBaseURL)
	cfg.VideoModels = []string{"T2V-01"} // 预留的视频目录：只作元数据
	p := NewMiniMax(cfg, nil)

	if caps := p.Capabilities(); caps.Video {
		t.Fatal("adapter reports video capability though no video adapter executes calls")
	}
	if err := cfg.ValidateVideoGeneration(); !errors.Is(err, ErrCapabilityUnsupported) {
		t.Fatalf("ValidateVideoGeneration = %v, want ErrCapabilityUnsupported", err)
	}
	if _, err := cfg.ResolveValidatedModel(ModalityVideo, "T2V-01"); !errors.Is(err, ErrCapabilityUnsupported) {
		t.Fatalf("explicit reserved video model resolved instead of gated: %v", err)
	}
	if err := cfg.ValidateCapability(ModalityText, ""); !errors.Is(err, ErrCapabilityUnsupported) {
		t.Fatalf("text capability validation = %v, want ErrCapabilityUnsupported", err)
	}
	_, terr := p.GenerateText(context.Background(), TextRequest{Prompt: "写一句话"})
	if terr == nil {
		t.Fatal("text generation must refuse explicitly")
	}
	if !errors.Is(terr, ErrUnsupported) || !errors.Is(terr, ErrCapabilityUnsupported) {
		t.Fatalf("text refusal lost the capability sentinels: %v", terr)
	}
	if !IsNotRetryable(terr) {
		t.Fatalf("text refusal should not waste retries: %v", terr)
	}
	if calls != 0 {
		t.Fatalf("gated modalities made %d outbound calls", calls)
	}
}
