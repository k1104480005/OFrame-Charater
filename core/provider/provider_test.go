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
	a := NewAgnes(cfg, client)
	res, err := a.GenerateImage(context.Background(), ImageRequest{Prompt: "hero"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Provider != ProviderAgnes {
		t.Fatalf("result: %+v", res)
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
	if s.ActiveProvider != DefaultProviderID {
		t.Fatalf("default active = %q", s.ActiveProvider)
	}
	if len(s.Providers) != 3 {
		t.Fatalf("default providers = %d", len(s.Providers))
	}
	if got := s.ConfigFor(ProviderDoubao).EffectiveModel(); got != DefaultDoubaoModel {
		t.Fatalf("doubao default model = %q", got)
	}
	if got := s.ConfigFor("nope").EffectiveMaxAttempts(); got != DefaultMaxAttemptsPerDirection {
		t.Fatalf("default max attempts = %d", got)
	}
}
