package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// fakeRT is an in-process RoundTripper: tests never touch real paid services.
type fakeRT struct {
	handler func(r *http.Request) (*http.Response, error)
}

func (f *fakeRT) RoundTrip(r *http.Request) (*http.Response, error) {
	return f.handler(r)
}

func fakeClient(h func(r *http.Request) (*http.Response, error)) *http.Client {
	return &http.Client{Transport: &fakeRT{handler: h}}
}

func jsonResp(code int, v any) *http.Response {
	data, _ := json.Marshal(v)
	return &http.Response{
		StatusCode: code,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

// --- registry (spec 4.1: 可注册多适配器且运行时切换生效) ---

func TestRegistrySwitch(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(NewDoubao(DefaultConfig(ProviderDoubao), nil))
	_ = r.Register(NewOpenAI(DefaultConfig(ProviderOpenAI), nil))
	_ = r.Register(NewAgnes(DefaultConfig(ProviderAgnes), nil))

	// First registered wins the initial active slot; default registry is doubao.
	act, err := r.Active()
	if err != nil {
		t.Fatal(err)
	}
	if act.ID() != ProviderDoubao {
		t.Fatalf("initial active = %q, want doubao", act.ID())
	}

	// Runtime switch to gpt-image-2 (高质量备选).
	if err := r.SetActive(ProviderOpenAI); err != nil {
		t.Fatal(err)
	}
	act, _ = r.Active()
	if act.ID() != ProviderOpenAI {
		t.Fatalf("active after switch = %q", act.ID())
	}
	// Doubao stays registered (previous config preserved).
	if _, err := r.Get(ProviderDoubao); err != nil {
		t.Fatalf("doubao lost after switch: %v", err)
	}

	if err := r.SetActive("nope"); err == nil {
		t.Fatal("expected error switching to unknown provider")
	}
	if err := r.Register(NewDoubao(DefaultConfig(ProviderDoubao), nil)); err == nil {
		t.Fatal("expected duplicate registration error")
	}
}

// --- config validation (模型/密钥配置与验证, offline) ---

func TestProviderConfigValidate(t *testing.T) {
	t.Setenv(EnvKeyDoubao, "")
	// Missing key and no env → error.
	if err := (DefaultConfig(ProviderDoubao)).Validate(); !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("expected ErrNoAPIKey, got %v", err)
	}
	// Env fallback makes the same config valid.
	t.Setenv(EnvKeyDoubao, "ark-test-key")
	if err := (DefaultConfig(ProviderDoubao)).Validate(); err != nil {
		t.Fatalf("validate with env key: %v", err)
	}
	// Direct key wins.
	cfg := DefaultConfig(ProviderOpenAI)
	cfg.APIKey = "sk-test"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate openai with key: %v", err)
	}
	// Unknown provider.
	bad := DefaultConfig(ProviderDoubao)
	bad.ProviderID = "bogus"
	if err := bad.Validate(); err == nil {
		t.Fatal("expected error for unknown provider id")
	}
	// Bad endpoint.
	bad2 := DefaultConfig(ProviderDoubao)
	bad2.APIKey = "k"
	bad2.BaseURL = "://nope"
	if err := bad2.Validate(); err == nil {
		t.Fatal("expected error for invalid base url")
	}
	// Out-of-range attempts.
	bad3 := DefaultConfig(ProviderDoubao)
	bad3.APIKey = "k"
	bad3.MaxAttempts = 99
	if err := bad3.Validate(); err == nil {
		t.Fatal("expected error for excessive max attempts")
	}
}

func TestResolveAPIKeyPrecedence(t *testing.T) {
	t.Setenv(EnvKeyDoubao, "env-key")
	cfg := DefaultConfig(ProviderDoubao)
	key, err := cfg.ResolveAPIKey()
	if err != nil || key != "env-key" {
		t.Fatalf("env fallback: %q, %v", key, err)
	}
	cfg.APIKey = "direct-key"
	key, _ = cfg.ResolveAPIKey()
	if key != "direct-key" {
		t.Fatalf("direct key should win, got %q", key)
	}
}

// --- retry (spec 4.7: 自动重试退避 + 硬上限; 每方向最多 3 次总尝试) ---

func TestCallWithRetrySucceedsOnRetry(t *testing.T) {
	attempts := 0
	err := CallWithRetry(context.Background(), RetryPolicy{MaxAttempts: 3, Backoff: time.Millisecond}, func(ctx context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestCallWithRetryStopsAtCap(t *testing.T) {
	attempts := 0
	err := CallWithRetry(context.Background(), RetryPolicy{MaxAttempts: 3, Backoff: time.Millisecond}, func(ctx context.Context) error {
		attempts++
		return errors.New("always fails")
	})
	var re *RetryError
	if !errors.As(err, &re) {
		t.Fatalf("expected RetryError, got %v", err)
	}
	if re.Attempts != 3 || attempts != 3 {
		t.Fatalf("attempts = %d, RetryError.Attempts = %d, want 3/3", attempts, re.Attempts)
	}
}

func TestCallWithRetryStopsOnNotRetryable(t *testing.T) {
	attempts := 0
	err := CallWithRetry(context.Background(), RetryPolicy{MaxAttempts: 3, Backoff: time.Millisecond}, func(ctx context.Context) error {
		attempts++
		return MarkNotRetryable(errors.New("auth failed"))
	})
	if err == nil || attempts != 1 {
		t.Fatalf("not-retryable should stop after 1 attempt, attempts = %d, err = %v", attempts, err)
	}
}

// --- Doubao adapter (默认主力) with fake transport ---

func TestDoubaoGenerateImage(t *testing.T) {
	wantPNG := []byte("fake-png-bytes")
	var gotBody map[string]any
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v3/images/generations" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("auth header = %q", auth)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		return jsonResp(200, map[string]any{
			"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString(wantPNG)}},
		}), nil
	})
	cfg := DefaultConfig(ProviderDoubao)
	cfg.APIKey = "test-key"
	d := NewDoubao(cfg, client)
	res, err := d.GenerateImage(context.Background(), ImageRequest{
		Prompt: "hero", Width: 1024, Height: 1024,
		References: []ReferenceImage{{Kind: "reference_image", Role: "main_reference", MIME: "image/png", Data: []byte("img")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(res.Data, wantPNG) || res.Provider != ProviderDoubao {
		t.Fatalf("result mismatch: %+v", res)
	}
	if gotBody["model"] != DefaultDoubaoModel {
		t.Errorf("model = %v", gotBody["model"])
	}
	if gotBody["size"] != "1024x1024" {
		t.Errorf("size = %v", gotBody["size"])
	}
	refs, _ := gotBody["reference_images"].([]any)
	if len(refs) != 1 {
		t.Fatalf("reference_images = %v", gotBody["reference_images"])
	}
}

func TestDoubaoGenerateText(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v3/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		return jsonResp(200, map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "hello"}}},
		}), nil
	})
	cfg := DefaultConfig(ProviderDoubao)
	cfg.APIKey = "k"
	d := NewDoubao(cfg, client)
	res, err := d.GenerateText(context.Background(), TextRequest{Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "hello" || res.Model != DefaultDoubaoTextModel {
		t.Fatalf("text result: %+v", res)
	}
}

func TestDoubaoAuthFailureNotRetryable(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		return jsonResp(401, map[string]any{"error": map[string]any{"message": "unauthorized"}}), nil
	})
	cfg := DefaultConfig(ProviderDoubao)
	cfg.APIKey = "bad"
	d := NewDoubao(cfg, client)
	_, err := d.GenerateImage(context.Background(), ImageRequest{Prompt: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNotRetryable(err) {
		t.Fatalf("auth failure must be not-retryable, got %v", err)
	}
}

// --- OpenAI gpt-image-2 (高质量备选) ---

func TestOpenAIGenerateImageRoutesCorrectly(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.openai.com" || r.URL.Path != "/v1/images/generations" {
			t.Errorf("request url = %s", r.URL)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"model":"gpt-image-2"`) {
			t.Errorf("body = %s", body)
		}
		return jsonResp(200, map[string]any{
			"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString([]byte("png"))}},
		}), nil
	})
	cfg := DefaultConfig(ProviderOpenAI)
	cfg.APIKey = "sk-test"
	o := NewOpenAI(cfg, client)
	res, err := o.GenerateImage(context.Background(), ImageRequest{Prompt: "hero"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Provider != ProviderOpenAI || res.Model != DefaultOpenAIModel {
		t.Fatalf("result: %+v", res)
	}
	// gpt-image-2 has no text capability.
	if _, err := o.GenerateText(context.Background(), TextRequest{Prompt: "x"}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported, got %v", err)
	}
}

// --- Agnes (专项备选, 可插拔不影响默认路径) ---

func TestAgnesGenerateImage(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/images/generations" {
			t.Errorf("path = %s", r.URL.Path)
		}
		return jsonResp(200, map[string]any{"data": []map[string]any{{"b64_json": "aGk="}}}), nil
	})
	cfg := DefaultConfig(ProviderAgnes)
	cfg.APIKey = "ag-key"
	// 占位端点会被预检拦截 —— 网络路径测试用真实形状的假域名。
	cfg.BaseURL = "https://agnes.example.com/v1"
	a := NewAgnes(cfg, client)
	res, err := a.GenerateImage(context.Background(), ImageRequest{Prompt: "hero"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Provider != ProviderAgnes {
		t.Fatalf("result: %+v", res)
	}
}

// TestAgnesGenerateText covers the multimodal text half (人工验收反馈：agnes
// 是多模态模型): chat/completions through the same OpenAI-compatible surface,
// defaulting to the placeholder text catalog entry.
func TestAgnesGenerateText(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer ag-key" {
			t.Errorf("auth = %q", auth)
		}
		return jsonResp(200, map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "agnes-text"}}},
		}), nil
	})
	cfg := DefaultConfig(ProviderAgnes)
	cfg.APIKey = "ag-key"
	cfg.BaseURL = "https://agnes.example.com/v1"
	a := NewAgnes(cfg, client)
	res, err := a.GenerateText(context.Background(), TextRequest{Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "agnes-text" || res.Model != DefaultAgnesTextModel {
		t.Fatalf("text result: %+v", res)
	}
	if a.Capabilities().Text != true || a.Capabilities().Image != true || a.Capabilities().Video != false {
		t.Fatalf("agnes capabilities = %+v, want multimodal image+text", a.Capabilities())
	}
}

// TestAgnesPlaceholderEndpointPreflight (人工验收反馈): a config still carrying
// the LEGACY placeholder host answers with the actionable migration message —
// not a raw DNS error — and never touches the network. The REAL default
// endpoint is unaffected.
func TestAgnesPlaceholderEndpointPreflight(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		t.Fatal("legacy placeholder endpoint must fail before any network request")
		return nil, nil
	})
	cfg := DefaultConfig(ProviderAgnes)
	cfg.BaseURL = "https://api.agnes.local/v1" // 旧版占位地址
	cfg.APIKey = "ag-key"
	a := NewAgnes(cfg, client)

	if _, err := a.GenerateImage(context.Background(), ImageRequest{Prompt: "x"}); err == nil || !strings.Contains(err.Error(), "apihub.agnes-ai.com/v1") {
		t.Fatalf("GenerateImage err = %v, want the migration message with the real URL", err)
	}
	if _, err := DiscoverModels(context.Background(), client, cfg); err == nil || !strings.Contains(err.Error(), "旧版 Agnes 占位地址") {
		t.Fatalf("DiscoverModels err = %v, want the migration message", err)
	}

	// The REAL default endpoint is not affected by the preflight.
	real := DefaultConfig(ProviderAgnes)
	real.APIKey = "ag-key"
	models, err := DiscoverModels(context.Background(), fakeClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "apihub.agnes-ai.com" {
			t.Errorf("discovery host = %s", r.URL.Host)
		}
		return jsonResp(200, map[string]any{"data": []map[string]any{{"id": "m"}}}), nil
	}), real)
	if err != nil || len(models) != 1 {
		t.Fatalf("real endpoint discovery = %v, %v", models, err)
	}
}

// --- stats (spec 4.6: 每次调用后统计次数与费用估算) ---

func TestStatsRecordCall(t *testing.T) {
	var s Stats
	s.RecordCall(ProviderDoubao, "m1", 0.05)
	s.RecordCall(ProviderDoubao, "m1", 0.05)
	s.RecordCall(ProviderOpenAI, "gpt-image-2", 0.10)
	if s.TotalCalls() != 3 {
		t.Fatalf("total calls = %d", s.TotalCalls())
	}
	doubao := s.ForProvider(ProviderDoubao)
	if len(doubao) != 1 || doubao[0].CallCount != 2 || doubao[0].EstimatedCost != 0.10 {
		t.Fatalf("doubao stats: %+v", doubao)
	}
	if doubao[0].Currency != "CNY" {
		t.Errorf("doubao currency = %q", doubao[0].Currency)
	}
	if got := s.ForProvider(ProviderOpenAI); got[0].Currency != "USD" {
		t.Errorf("openai currency = %q", got[0].Currency)
	}
}

// --- default settings ---

func TestDefaultSettings(t *testing.T) {
	s := DefaultSettings()
	// 人工验收更新: a fresh install pre-seeds NO provider cards — users add
	// providers from the seven presets. The built-in identities remain
	// reachable through ConfigFor's default fallback (adapters still shipped).
	if s.ActiveProvider != "" {
		t.Fatalf("default active = %q, want empty", s.ActiveProvider)
	}
	if len(s.Providers) != 0 {
		t.Fatalf("default providers = %d, want 0", len(s.Providers))
	}
	if got := s.ConfigFor(ProviderDoubao).EffectiveModel(); got != DefaultDoubaoModel {
		t.Fatalf("doubao default model via fallback = %q", got)
	}
	if got := s.ConfigFor("nope").EffectiveMaxAttempts(); got != DefaultMaxAttemptsPerDirection {
		t.Fatalf("default max attempts = %d", got)
	}
}

// --- protocol types & capability metadata (align-framebaker-providers 1.1) ---

// equalStrings reports whether two string slices are identical element-wise
// (nil and empty are treated as distinct so "unset" stays observable).
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestProtocolTypeConstants pins the protocol discriminator values shared by
// presets, config payloads and future adapters.
func TestProtocolTypeConstants(t *testing.T) {
	cases := [][2]string{
		{ProviderTypeCLI, "cli"},
		{ProviderTypeAPI, "api"},
		{ProviderTypeDashscope, "dashscope"},
		{ProviderTypeGemini, "gemini"},
		{ProviderTypeMiniMax, "minimax"},
		{ProviderTypeVolcengine, "volcengine"},
		{ProviderTypeCompatible, "compatible"},
	}
	for _, c := range cases {
		if c[0] != c[1] {
			t.Errorf("protocol constant drifted: got %q, want %q", c[0], c[1])
		}
	}
}

// TestNoAdapterClaimsVideoYet guards capability truthfulness: no current
// adapter performs video calls, so none may report Video support even though
// Doubao carries a reserved video model catalog entry.
func TestNoAdapterClaimsVideoYet(t *testing.T) {
	custom := ProviderConfig{ProviderID: "custom", Type: ProviderTypeCompatible, Name: "Custom"}
	caps := map[string]Capabilities{
		ProviderDoubao: NewDoubao(DefaultConfig(ProviderDoubao), nil).Capabilities(),
		ProviderOpenAI: NewOpenAI(DefaultConfig(ProviderOpenAI), nil).Capabilities(),
		ProviderAgnes:  NewAgnes(DefaultConfig(ProviderAgnes), nil).Capabilities(),
		"custom":       NewCompatible(custom, nil).Capabilities(),
	}
	wantImageText := map[string][2]bool{
		// [image, text] — verified together with Video == false so adding new
		// adapters does not silently drift the existing declarations either.
		ProviderDoubao: {true, true},
		ProviderOpenAI: {true, false},
		ProviderAgnes:  {true, true}, // 多模态（人工验收反馈）
		"custom":       {true, true},
	}
	for id, got := range caps {
		if got.Video {
			t.Errorf("provider %s claims video capability before video support exists", id)
		}
		want := wantImageText[id]
		if got.Image != want[0] || got.Text != want[1] {
			t.Errorf("provider %s capabilities = {image:%v text:%v}, want {image:%v text:%v}", id, got.Image, got.Text, want[0], want[1])
		}
	}
}

// TestLegacySingularFieldsFeedEffectiveLists covers the old-shape JSON payload
// (no imageModels/videoModels/textModels keys): unmarshalling must keep the
// singular fields working as the one-entry effective lists.
func TestLegacySingularFieldsFeedEffectiveLists(t *testing.T) {
	raw := `{"providerId":"my-custom","type":"compatible","name":"My","apiKey":"k",
	         "model":"legacy-image","videoModel":"legacy-video","textModel":"legacy-text",
	         "baseUrl":"https://x.example.com/v1"}`
	var cfg ProviderConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.EffectiveImageModels(); !equalStrings(got, []string{"legacy-image"}) {
		t.Errorf("image list = %v, want [legacy-image]", got)
	}
	if got := cfg.EffectiveVideoModels(); !equalStrings(got, []string{"legacy-video"}) {
		t.Errorf("video list = %v, want [legacy-video]", got)
	}
	if got := cfg.EffectiveTextModels(); !equalStrings(got, []string{"legacy-text"}) {
		t.Errorf("text list = %v, want [legacy-text]", got)
	}
}

// TestArrayCatalogWinsOverLegacySingles verifies arrays take precedence over
// the legacy singular fields and that entries are whitespace-trimmed, blanks
// dropped and duplicates removed in first-seen order.
func TestArrayCatalogWinsOverLegacySingles(t *testing.T) {
	raw := `{"providerId":"doubao","type":"doubao","apiKey":"k",
	         "model":"old-image","textModel":"old-text",
	         "imageModels":[" img-a ","img-b","img-a","","img-c"],
	         "videoModels":[" vid-1 ","vid-1",""],
	         "textModels":[" txt-a ","txt-a"]} `
	var cfg ProviderConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.EffectiveImageModels(); !equalStrings(got, []string{"img-a", "img-b", "img-c"}) {
		t.Errorf("image list = %v, want [img-a img-b img-c]", got)
	}
	if got := cfg.EffectiveVideoModels(); !equalStrings(got, []string{"vid-1"}) {
		t.Errorf("video list = %v, want [vid-1]", got)
	}
	if got := cfg.EffectiveTextModels(); !equalStrings(got, []string{"txt-a"}) {
		t.Errorf("text list = %v, want [txt-a]", got)
	}
}

// TestBlankArraysFallBackToSingularFieldsAndDefaults covers the boundary where
// arrays exist but are blank-only, plus the fully-unset unknown provider.
func TestBlankArraysFallBackToSingularFieldsAndDefaults(t *testing.T) {
	cfg := DefaultConfig(ProviderDoubao)
	cfg.Model = "kept-image"
	cfg.TextModel = ""
	cfg.ImageModels = []string{" ", "\t"}
	cfg.VideoModels = []string{}
	cfg.TextModels = nil

	if got := cfg.EffectiveImageModels(); !equalStrings(got, []string{"kept-image"}) {
		t.Errorf("blank array should fall back to singular model, got %v", got)
	}
	// Text singular empty → built-in default preserved (old behavior).
	if got := cfg.EffectiveTextModels(); !equalStrings(got, []string{DefaultDoubaoTextModel}) {
		t.Errorf("empty text should fall back to default, got %v", got)
	}
	// Blank video array with no legacy value → nothing configured.
	if got := cfg.EffectiveVideoModels(); got != nil {
		t.Errorf("blank video array with no legacy field = %v, want nil", got)
	}

	unset := DefaultConfig("unknown-provider")
	if got := unset.EffectiveImageModels(); got != nil || unset.EffectiveModel() != "" {
		t.Errorf("unknown provider lists = %v (model %q), want nil and \"\"", got, unset.EffectiveModel())
	}
	if got := unset.EffectiveVideoModels(); got != nil || unset.EffectiveTextModels() != nil {
		t.Errorf("unknown provider video/text lists = %v/%v, want nil/nil", got, unset.EffectiveTextModels())
	}
}

// TestDefaultConfigsClassifyBuiltInModels verifies the per-capability default
// catalogs (task 1.1: Doubao image+video+text, OpenAI image, Agnes image)
// while the legacy effective defaults stay unchanged.
func TestDefaultConfigsClassifyBuiltInModels(t *testing.T) {
	doubao := DefaultConfig(ProviderDoubao)
	if !equalStrings(doubao.EffectiveImageModels(), []string{DefaultDoubaoModel}) {
		t.Errorf("doubao image default = %v", doubao.EffectiveImageModels())
	}
	if !equalStrings(doubao.EffectiveVideoModels(), []string{DefaultDoubaoVideoModel}) {
		t.Errorf("doubao video default = %v", doubao.EffectiveVideoModels())
	}
	if !equalStrings(doubao.EffectiveTextModels(), []string{DefaultDoubaoTextModel}) {
		t.Errorf("doubao text default = %v", doubao.EffectiveTextModels())
	}
	// Existing effective defaults unchanged by the array introduction.
	if doubao.EffectiveModel() != DefaultDoubaoModel || doubao.EffectiveTextModel() != DefaultDoubaoTextModel {
		t.Fatalf("legacy effective models changed: %q / %q", doubao.EffectiveModel(), doubao.EffectiveTextModel())
	}

	openai := DefaultConfig(ProviderOpenAI)
	if !equalStrings(openai.EffectiveImageModels(), []string{DefaultOpenAIModel}) {
		t.Errorf("openai image default = %v", openai.EffectiveImageModels())
	}
	if openai.EffectiveVideoModels() != nil || openai.EffectiveTextModels() != nil {
		t.Errorf("openai video/text defaults = %v/%v, want nil/nil", openai.EffectiveVideoModels(), openai.EffectiveTextModels())
	}

	agnes := DefaultConfig(ProviderAgnes)
	if !equalStrings(agnes.EffectiveImageModels(), []string{DefaultAgnesModel}) {
		t.Errorf("agnes image default = %v", agnes.EffectiveImageModels())
	}
	// 全模态方向：text catalog 与 video 预留目录都随配置携带（video 仅元数据，
	// 能力声明恒为 false —— 全局视频门禁拦截）。
	if !equalStrings(agnes.EffectiveVideoModels(), []string{DefaultAgnesVideoModel}) {
		t.Errorf("agnes video default = %v, want [%s]", agnes.EffectiveVideoModels(), DefaultAgnesVideoModel)
	}
	if !equalStrings(agnes.EffectiveTextModels(), []string{DefaultAgnesTextModel}) {
		t.Errorf("agnes text default = %v, want [%s]", agnes.EffectiveTextModels(), DefaultAgnesTextModel)
	}
}
