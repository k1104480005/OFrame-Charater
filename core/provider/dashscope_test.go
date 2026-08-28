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
	"time"
)

// dashConfig builds a 百炼 (dashscope) config mirroring what the FrameBaker
// preset produces: explicit protocol Type, vendor BaseURL, catalogs, key.
func dashConfig(baseURL string) ProviderConfig {
	return ProviderConfig{
		ProviderID:  "bailian",
		Type:        ProviderTypeDashscope,
		Name:        "阿里百炼",
		APIKey:      "sk-dash",
		BaseURL:     baseURL,
		ImageModels: []string{"wanx2.1-t2i-turbo"},
		TextModels:  []string{"qwen-plus"},
	}
}

// --- 身份与路由（design D1：协议身份绝不被另一协议悄悄顶替） ---

func TestDashscopeIdentityAndCapabilities(t *testing.T) {
	cfg := dashConfig(DefaultDashscopeBaseURL)
	p := NewDashscope(cfg, nil)
	if p.ID() != "bailian" {
		t.Fatalf("ID = %q, want configured provider id", p.ID())
	}
	if p.Name() != "阿里百炼" {
		t.Fatalf("Name = %q, want display name", p.Name())
	}
	if got := NewDashscope(ProviderConfig{ProviderID: "bailian", Type: ProviderTypeDashscope}, nil).Name(); got != "百炼 (DashScope)" {
		t.Fatalf("fallback Name = %q", got)
	}
	if p.Mode() != DashscopeModeNative {
		t.Fatalf("default base url should route native, got %q", p.Mode())
	}
	caps := p.Capabilities()
	want := cfg.DeclaredCapabilities()
	if caps != want {
		t.Fatalf("live capabilities %v drift from declared %v", caps, want)
	}
	if caps.Video {
		t.Fatal("dashscope adapter claims video before any video adapter exists")
	}
	if p.DefaultTextModel() != "" || p.DefaultImageModel() != "" {
		t.Fatalf("legacy defaults should stay empty for catalog-only configs: %q/%q", p.DefaultImageModel(), p.DefaultTextModel())
	}
}

func TestDashscopeRouteModeDecidedByEndpoint(t *testing.T) {
	cases := []struct {
		baseURL string
		want    DashscopeMode
	}{
		{DefaultDashscopeBaseURL, DashscopeModeNative},
		{"https://dashscope-intl.aliyuncs.com/api/v1", DashscopeModeNative},
		{"https://api.example.local/my-proxy", DashscopeModeNative},
		{DefaultDashscopeCompatibleBaseURL, DashscopeModeCompatible},
		{"https://dashscope.aliyuncs.com/compatible-mode/v1/", DashscopeModeCompatible},
	}
	for _, tc := range cases {
		if got := DashscopeRouteMode(tc.baseURL); got != tc.want {
			t.Errorf("DashscopeRouteMode(%q) = %q, want %q", tc.baseURL, got, tc.want)
		}
	}
}

// TestNewAdapterAddsDashscopeWithoutBreakingExistingRoutes pins the additive
// registry contract: every previously-routed type keeps its exact adapter,
// dashscope gains its own, unknown types still fail explicitly.
func TestNewAdapterAddsDashscopeWithoutBreakingExistingRoutes(t *testing.T) {
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
	}
	for _, tc := range cases {
		p, err := NewAdapter("p1", mk(tc.typ), nil)
		if err != nil {
			t.Fatalf("type %q: unexpected error %v", tc.typ, err)
		}
		// Built-ins keep their hard-coded vendor identity; custom types
		// (compatible/dashscope) use the configured provider id.
		wantID := "p1"
		if tc.typ == ProviderDoubao || tc.typ == ProviderOpenAI || tc.typ == ProviderAgnes {
			wantID = tc.typ
		}
		if id := p.ID(); id != wantID {
			t.Errorf("type %q: adapter ID = %q, want %q", tc.typ, id, wantID)
		}
		// 静默套用其他协议会在这里以具体类型变化暴露出来。
		if fmt.Sprintf("%T", p) != fmt.Sprintf("%T", tc.want) {
			t.Errorf("type %q routed to %T, want %T", tc.typ, p, tc.want)
		}
	}
	if _, err := NewAdapter("p1", ProviderConfig{ProviderID: "p1", Type: "mystery"}, nil); err == nil {
		t.Error("unknown types must still be rejected")
	}
}

// --- 原生图片合成（fake transport）---

// TestDashscopeNativeImageSyncPayload covers the synchronous-results variant:
// correct endpoint/auth/async-header/body/size mapping, unified timeout, and
// output.results[].b64_json parsing under the dashscope provider identity.
func TestDashscopeNativeImageSyncPayload(t *testing.T) {
	var gotPath, gotMethod, gotAuth, gotAsyncHeader string
	var gotBody map[string]any
	sawDeadline := false
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAsyncHeader = r.Header.Get(headerDashscopeAsync)
		if _, ok := r.Context().Deadline(); ok {
			sawDeadline = true
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		return jsonResp(200, map[string]any{
			"request_id": "req-1",
			"output": map[string]any{
				"task_status": "SUCCEEDED",
				"results":     []map[string]any{{"b64_json": fakePNG}},
			},
		}), nil
	})
	cfg := dashConfig(DefaultDashscopeBaseURL)
	res, err := NewDashscope(cfg, client).GenerateImage(context.Background(), ImageRequest{Prompt: "赛博朋克人物"})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1"+dashscopeNativeImagePath {
		t.Fatalf("native endpoint drifted: %s %s", gotMethod, gotPath)
	}
	if gotAuth != "Bearer sk-dash" {
		t.Fatalf("auth = %q, want DashScope Bearer", gotAuth)
	}
	if gotAsyncHeader != headerDashscopeAsyncValue {
		t.Fatalf("%s header = %q", headerDashscopeAsync, gotAsyncHeader)
	}
	if !sawDeadline {
		t.Error("expected an adapter-applied deadline on the provider call")
	}
	inp, _ := gotBody["input"].(map[string]any)
	if inp["prompt"] != "赛博朋克人物" {
		t.Errorf("input.prompt = %v", inp["prompt"])
	}
	params, _ := gotBody["parameters"].(map[string]any)
	if params["size"] != "1024*1024" { // 默认尺寸 → DashScope 原生 W*H 分隔符
		t.Errorf("parameters.size = %v, want 1024*1024", params["size"])
	}
	if params["n"].(float64) != 1 {
		t.Errorf("parameters.n = %v", params["n"])
	}
	if gotBody["model"] != "wanx2.1-t2i-turbo" { // catalog head as default model
		t.Errorf("model = %v", gotBody["model"])
	}
	wantPNG, _ := base64.StdEncoding.DecodeString(fakePNG)
	if !stringsEqualBytes(res.Data, wantPNG) || res.MIME != "image/png" {
		t.Fatalf("decoded image mismatch: mime=%q len=%d", res.MIME, len(res.Data))
	}
	if res.Provider != "bailian" || res.Model != "wanx2.1-t2i-turbo" {
		t.Fatalf("result meta = %q/%q", res.Provider, res.Model)
	}
}

func stringsEqualBytes(a, b []byte) bool {
	return len(a) == len(b) && string(a) == string(b)
}

// TestDashscopeNativeExplicitSizeMapping pins Width/Height → parameters.size.
func TestDashscopeNativeExplicitSizeMapping(t *testing.T) {
	cases := []struct {
		w, h int
		want string
	}{
		{800, 600, "800*600"},
		{-5, 0, "1024*1024"}, // unset dims fall back to the generation default
	}
	for _, tc := range cases {
		var gotSize any
		client := fakeClient(func(r *http.Request) (*http.Response, error) {
			var body struct {
				Parameters struct {
					Size string `json:"size"`
				} `json:"parameters"`
			}
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &body)
			gotSize = body.Parameters.Size
			return jsonResp(200, map[string]any{"output": map[string]any{"task_status": "SUCCEEDED", "results": []map[string]any{{"b64_json": fakePNG}}}}), nil
		})
		cfg := dashConfig(DefaultDashscopeBaseURL)
		if _, err := NewDashscope(cfg, client).GenerateImage(context.Background(), ImageRequest{Prompt: "p", Width: tc.w, Height: tc.h}); err != nil {
			t.Fatalf("w=%d h=%d: %v", tc.w, tc.h, err)
		}
		if gotSize != tc.want {
			t.Errorf("W%dH%d size = %v, want %q", tc.w, tc.h, gotSize, tc.want)
		}
	}
}

// TestDashscopeNativeAsyncSubmitAndPoll covers the async workflow: submit
// answers PENDING/task_id, GET /tasks/{id} transitions RUNNING → SUCCEEDED,
// the poll carries Bearer auth, and the URL result downloads WITHOUT ever
// sending the API key to the CDN.
func TestDashscopeNativeAsyncSubmitAndPoll(t *testing.T) {
	defer func(d time.Duration, n int) {
		dashscopePollDelay, dashscopePollAttempts = d, n
	}(dashscopePollDelay, dashscopePollAttempts)
	dashscopePollDelay = 0
	dashscopePollAttempts = 10

	polls := 0
	submits := 0
	var gotTaskAuth, lastGotPath, cdnAuth string
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		lastGotPath = r.URL.Path
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, dashscopeNativeImagePath):
			submits++
			gotTaskAuth = r.Header.Get("Authorization")
			return jsonResp(200, map[string]any{
				"output": map[string]any{"task_id": "t-42", "task_status": "PENDING"},
			}), nil
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, dashscopeTaskQueryPath):
			polls++
			if r.Header.Get("Authorization") != "Bearer sk-dash" {
				t.Errorf("poll #%d missing Bearer auth (%q)", polls, r.Header.Get("Authorization"))
			}
			status := "RUNNING"
			results := []map[string]any{}
			if polls >= 2 {
				status = "SUCCEEDED"
				results = []map[string]any{{"url": "http://cdn.example.com/framebaker.png"}}
			}
			return jsonResp(200, map[string]any{
				"output": map[string]any{"task_id": "t-42", "task_status": status, "results": results},
			}), nil
		case r.URL.Host == "cdn.example.com":
			cdnAuth = r.Header.Get("Authorization")
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"image/png"}},
				Body:       io.NopCloser(strings.NewReader("CDN-BYTES")),
			}, nil
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			return jsonResp(404, map[string]any{}), nil
		}
	})
	cfg := dashConfig(DefaultDashscopeBaseURL)
	res, err := NewDashscope(cfg, client).GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if submits != 1 {
		t.Errorf("submits = %d, want 1", submits)
	}
	if polls < 2 {
		t.Errorf("polls = %d, want ≥2", polls)
	}
	if gotTaskAuth != "Bearer sk-dash" {
		t.Errorf("submit auth = %q", gotTaskAuth)
	}
	if cdnAuth != "" {
		t.Errorf("CDN download carried credentials: %q", cdnAuth)
	}
	if string(res.Data) != "CDN-BYTES" || res.MIME != "image/png" {
		t.Fatalf("url result = %q/%q", res.Data, res.MIME)
	}
	if res.Model != "wanx2.1-t2i-turbo" || res.Provider != "bailian" {
		t.Fatalf("result meta = %+v", res)
	}
	if lastGotPath == "" {
		t.Error("no requests recorded?")
	}
}

// --- 错误响应可读、密钥脱敏、重试语义（与 2.1 统一行为一致）---

func TestDashscopeNativeErrorHandling(t *testing.T) {
	origCap := maxGenerationResponseBytes
	defer func() { maxGenerationResponseBytes = origCap }()

	type run struct {
		name      string
		setup     func(*testing.T) (*http.Client, int) // client + maxGen override (0 = keep)
		attempts  int                                  // poll override (0 = keep)
		wantParts []string
		notIn     []string
		notRtry   bool
	}
	cases := []run{
		{
			// Shared-helper contract: the credential redacted from vendor
			// echoes is the CALLER'S OWN configured key (sk-dash here).
			name: "submit 401 vendor envelope, not retryable, key redacted",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawJSONResp(http.StatusUnauthorized, `{"code":"InvalidApiKey","message":"key sk-dash is invalid","request_id":"r"}`), nil
				}), 0
			},
			wantParts: []string{"auth failed", "HTTP 401", "is invalid", "***"},
			notIn:     []string{"sk-dash"},
			notRtry:   true,
		},
		{
			name: "submit 500 html echo stays retryable without leaking key",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return rawResp(http.StatusInternalServerError, "text/html", `<html>boom sk-dash</html>`), nil
				}), 0
			},
			wantParts: []string{"unexpected status 500", "boom"},
			notIn:     []string{"sk-dash"},
			notRtry:   false,
		},
		{
			name: "sync payload declares task FAILED with reason",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return jsonResp(200, map[string]any{
						"output": map[string]any{"task_status": "FAILED", "code": "IPInfringementSuspected", "message": "输入内容涉嫌侵权，请调整后重试"},
					}), nil
				}), 0
			},
			wantParts: []string{"FAILED", "IPInfringementSuspected", "涉嫌侵权"},
		},
		{
			name: "polling gives up boundedly naming the task",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(r *http.Request) (*http.Response, error) {
					if r.Method == http.MethodPost {
						return jsonResp(200, map[string]any{"output": map[string]any{"task_id": "t-stuck", "task_status": "PENDING"}}), nil
					}
					return jsonResp(200, map[string]any{"output": map[string]any{"task_id": "t-stuck", "task_status": "RUNNING"}}), nil
				}), 0
			},
			attempts:  3,
			wantParts: []string{"t-stuck", "did not finish after 3 polls", `last status "RUNNING"`},
		},
		{
			name: "poll 401 stops as auth failure",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(r *http.Request) (*http.Response, error) {
					if r.Method == http.MethodPost {
						return jsonResp(200, map[string]any{"output": map[string]any{"task_id": "t-auth", "task_status": "PENDING"}}), nil
					}
					return jsonResp(http.StatusUnauthorized, map[string]any{"code": "InvalidApiKey", "message": "expired"}), nil
				}), 0
			},
			wantParts: []string{"auth failed", "HTTP 401"},
			notRtry:   true,
		},
		{
			name: "submit 200 without task_id or results is a readable failure",
			setup: func(t *testing.T) (*http.Client, int) {
				return fakeClient(func(*http.Request) (*http.Response, error) {
					return jsonResp(200, map[string]any{"request_id": "r-empty"}), nil
				}), 0
			},
			wantParts: []string{"no task_id"},
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
		},
	}
	for i := range cases {
		tc := cases[i]
		t.Run(tc.name, func(t *testing.T) {
			if tc.attempts > 0 {
				oldA, oldD := dashscopePollAttempts, dashscopePollDelay
				dashscopePollAttempts, dashscopePollDelay = tc.attempts, 0
				defer func() { dashscopePollAttempts, dashscopePollDelay = oldA, oldD }()
			}
			if tc.setup == nil {
				t.Fatal("missing setup")
			}
			client, genCap := tc.setup(t)
			if genCap > 0 {
				maxGenerationResponseBytes = int64(genCap)
			}
			p := NewDashscope(dashConfig(DefaultDashscopeBaseURL), client)
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

// rawResp/rawJSONResp build non-2xx fixtures without the shared jsonResp's
// implicit success framing.
func rawResp(code int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func rawJSONResp(code int, body string) *http.Response {
	return rawResp(code, "application/json", body)
}

// --- 引用图按模式分流：兼容模式透传约定字段，原生模式外呼前明确拒绝 ---

func TestDashscopeReferenceImagesPerMode(t *testing.T) {
	refs := []ReferenceImage{
		{Kind: "reference_image", Role: "main_reference", MIME: "image/png", Data: []byte("abc")},
	}

	t.Run("native rejects references before any outbound call", func(t *testing.T) {
		calls := 0
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			calls++
			return jsonResp(200, map[string]any{}), nil
		})
		cfg := dashConfig(DefaultDashscopeBaseURL)
		_, err := NewDashscope(cfg, client).GenerateImage(context.Background(), ImageRequest{Prompt: "p", References: refs})
		if err == nil {
			t.Fatal("expected rejection")
		}
		if calls != 0 {
			t.Fatalf("native mode made %d outbound calls before rejecting references", calls)
		}
		if !strings.Contains(err.Error(), "reference images") || !strings.Contains(err.Error(), "bailian") {
			t.Fatalf("rejection not readable: %v", err)
		}
		if !IsNotRetryable(err) {
			t.Fatalf("reference rejection should not waste retries: %v", err)
		}
	})

	t.Run("compatible mode keeps the shared reference_images contract", func(t *testing.T) {
		var gotBody map[string]any
		var gotAsync string
		client := fakeClient(func(r *http.Request) (*http.Response, error) {
			gotAsync = r.Header.Get(headerDashscopeAsync)
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			return jsonResp(200, map[string]any{"data": []map[string]any{{"b64_json": fakePNG}}}), nil
		})
		cfg := dashConfig(DefaultDashscopeCompatibleBaseURL)
		res, err := NewDashscope(cfg, client).GenerateImage(context.Background(), ImageRequest{Prompt: "p", References: refs, Width: 512, Height: 512})
		if err != nil {
			t.Fatal(err)
		}
		if gotAsync != "" {
			t.Errorf("compatible surface must not carry the native async header, got %q", gotAsync)
		}
		refsOut, _ := gotBody["reference_images"].([]any)
		if len(refsOut) != 1 {
			t.Fatalf("reference_images = %v", gotBody["reference_images"])
		}
		first, _ := refsOut[0].(map[string]any)
		if first["role"] != "main_reference" ||
			first["image"] != "data:image/png;base64,"+base64.StdEncoding.EncodeToString([]byte("abc")) {
			t.Fatalf("ref entry drifted: %v", first)
		}
		if gotBody["size"] != "512x512" { // 兼容表面沿用共享契约的 x 分隔符
			t.Errorf("compatible size = %v", gotBody["size"])
		}
		wantPNG, _ := base64.StdEncoding.DecodeString(fakePNG)
		if !stringsEqualBytes(res.Data, wantPNG) {
			t.Fatal("image bytes mismatch")
		}
	})
}

// --- 兼容模式请求面与原生端点互不串扰 ---

func TestDashscopeCompatibleModeRoutesOpenAIEndpoints(t *testing.T) {
	imgURL, txtURL, imgModel := "", "", ""
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/images/generations") {
			imgURL = r.URL.String()
			if r.Header.Get("Authorization") != "Bearer sk-dash" {
				t.Errorf("compat image auth = %q", r.Header.Get("Authorization"))
			}
			var body map[string]any
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &body)
			imgModel, _ = body["model"].(string)
			return jsonResp(200, map[string]any{"data": []map[string]any{{"b64_json": fakePNG}}}), nil
		}
		txtURL = r.URL.String()
		return jsonResp(200, map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": "你好"}}}}), nil
	})
	cfg := dashConfig(DefaultDashscopeCompatibleBaseURL)
	p := NewDashscope(cfg, client)
	if p.Mode() != DashscopeModeCompatible {
		t.Fatalf("mode = %q", p.Mode())
	}
	res, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Provider != "bailian" || res.Model != "wanx2.1-t2i-turbo" {
		t.Errorf("compat image result meta = %+v", res)
	}
	if !strings.HasSuffix(imgURL, "/compatible-mode/v1/images/generations") {
		t.Errorf("compat image url = %q", imgURL)
	}
	if imgModel != "wanx2.1-t2i-turbo" {
		t.Errorf("compat image model = %q", imgModel)
	}
	txt, err := p.GenerateText(context.Background(), TextRequest{Prompt: "问好"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(txtURL, "/compatible-mode/v1/chat/completions") {
		t.Errorf("compat text url = %q", txtURL)
	}
	if txt.Text != "你好" || txt.Provider != "bailian" || txt.Model != "qwen-plus" {
		t.Fatalf("text result = %+v", txt)
	}
}

// --- 原生文本生成（output.text 与 choices 变体；错误脱敏）---

func TestDashscopeNativeTextGeneration(t *testing.T) {
	t.Run("legacy output.text variant", func(t *testing.T) {
		var gotPath, gotAuth string
		var gotBody map[string]any
		client := fakeClient(func(r *http.Request) (*http.Response, error) {
			gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &gotBody)
			return jsonResp(200, map[string]any{
				"output": map[string]any{"text": "好的，开始绘制。", "finish_reason": "stop"},
			}), nil
		})
		res, err := NewDashscope(dashConfig(DefaultDashscopeBaseURL), client).GenerateText(context.Background(), TextRequest{Prompt: "规划"})
		if err != nil {
			t.Fatal(err)
		}
		if gotPath != "/api/v1"+dashscopeNativeTextPath {
			t.Errorf("path = %s", gotPath)
		}
		if gotAuth != "Bearer sk-dash" {
			t.Errorf("auth = %q", gotAuth)
		}
		input, _ := gotBody["input"].(map[string]any)
		msgs, _ := input["messages"].([]any)
		first, _ := msgs[0].(map[string]any)
		if gotBody["model"] != "qwen-plus" || first["content"] != "规划" {
			t.Fatalf("request body drifted: %v", gotBody)
		}
		if res.Text != "好的，开始绘制。" || res.Model != "qwen-plus" {
			t.Fatalf("result = %+v", res)
		}
	})

	t.Run("choices message.content variant", func(t *testing.T) {
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			return jsonResp(200, map[string]any{
				"output": map[string]any{"choices": []map[string]any{{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": "已就绪"}}}},
			}), nil
		})
		res, err := NewDashscope(dashConfig(DefaultDashscopeBaseURL), client).GenerateText(context.Background(), TextRequest{})
		if err != nil {
			t.Fatal(err)
		}
		if res.Text != "已就绪" {
			t.Fatalf("result = %+v", res)
		}
	})

	t.Run("vendor code/message envelope surfaced with redaction", func(t *testing.T) {
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			return jsonResp(200, map[string]any{
				"code": "Throttling.RateQuota", "message": "quota exceeded for sk-dash", "request_id": "r",
			}), nil
		})
		_, err := NewDashscope(dashConfig(DefaultDashscopeBaseURL), client).GenerateText(context.Background(), TextRequest{Prompt: "hi"})
		if err == nil || !strings.Contains(err.Error(), "Throttling.RateQuota") {
			t.Fatalf("expected readable vendor error, got %v", err)
		}
		if strings.Contains(err.Error(), "sk-dash") {
			t.Fatalf("error leaked the key: %v", err)
		}
	})

	t.Run("empty output is a clear failure", func(t *testing.T) {
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			return jsonResp(200, map[string]any{"output": map[string]any{}}), nil
		})
		_, err := NewDashscope(dashConfig(DefaultDashscopeBaseURL), client).GenerateText(context.Background(), TextRequest{Prompt: "hi"})
		if err == nil || !strings.Contains(err.Error(), "no content") {
			t.Fatalf("expected no-content error, got %v", err)
		}
	})

	t.Run("no models anywhere fails before outbound", func(t *testing.T) {
		calls := 0
		client := fakeClient(func(*http.Request) (*http.Response, error) {
			calls++
			return jsonResp(200, map[string]any{}), nil
		})
		cfg := dashConfig(DefaultDashscopeBaseURL)
		cfg.TextModels = nil
		_, err := NewDashscope(cfg, client).GenerateText(context.Background(), TextRequest{Prompt: "x"})
		if err == nil || !strings.Contains(err.Error(), "no text model configured") {
			t.Fatalf("expected pre-flight model error, got %v", err)
		}
		if calls != 0 {
			t.Fatalf("pre-flight model failure issued %d outbound calls", calls)
		}
	})
}

// --- 密钥缺失：零外呼 ---

func TestDashscopeMissingKeyPreFlightZeroNetwork(t *testing.T) {
	t.Setenv("OFRAME_BAILIAN_API_KEY", "")
	calls := 0
	client := fakeClient(func(*http.Request) (*http.Response, error) {
		calls++
		return jsonResp(200, map[string]any{}), nil
	})
	cfg := dashConfig(DefaultDashscopeBaseURL)
	cfg.APIKey = ""
	p := NewDashscope(cfg, client)
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

// --- 视频模型只保留目录，不产生外呼（能力校验在外呼前拦截）---

func TestDashscopeVideoModelsStayReserved(t *testing.T) {
	calls := 0
	prevDefault := http.DefaultClient
	http.DefaultClient = fakeClient(func(r *http.Request) (*http.Response, error) {
		calls++
		t.Errorf("video gate reached the network: %s%s", r.URL.Host, r.URL.Path)
		return jsonResp(200, map[string]any{}), nil
	})
	t.Cleanup(func() { http.DefaultClient = prevDefault })

	cfg := dashConfig(DefaultDashscopeBaseURL)
	cfg.VideoModels = []string{"wan2.2-t2v-flash"} // 百炼预设预留的视频目录
	if caps := NewDashscope(cfg, nil).Capabilities(); caps.Video {
		t.Fatal("adapter reports video capability though no video adapter executes calls")
	}
	err := cfg.ValidateVideoGeneration()
	if !errors.Is(err, ErrCapabilityUnsupported) {
		t.Fatalf("ValidateVideoGeneration = %v, want ErrCapabilityUnsupported", err)
	}
	if _, err := cfg.ResolveValidatedModel(ModalityVideo, "wan2.2-t2v-flash"); !errors.Is(err, ErrCapabilityUnsupported) {
		t.Fatalf("explicit reserved video model resolved instead of gated: %v", err)
	}
	if calls != 0 {
		t.Fatalf("video gating made %d outbound calls", calls)
	}
}
