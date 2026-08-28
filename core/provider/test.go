package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrModelDiscoveryUnsupported reports protocols that do not expose a remote
// model catalog (notably local CLI providers). Callers keep manually
// configured catalogs instead of probing an unrelated HTTP surface.
var ErrModelDiscoveryUnsupported = errors.New("provider: model discovery is not supported by this protocol")

// TestResult is the outcome of a live connection test against a provider's
// protocol-specific model discovery endpoint: ok, latency and the models
// exposed (when the endpoint responds), or a readable error.
type TestResult struct {
	OK        bool     `json:"ok"`
	LatencyMS int64    `json:"latencyMs,omitempty"`
	Models    []string `json:"models,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// TestConnection performs a live connectivity check (settings "测试连接"): it
// probes the configured protocol's model-discovery endpoint, measures the
// latency and collects the model list. Failures (auth, timeout, bad endpoint,
// unsupported protocol) are folded into the result — the caller shows them
// inline instead of aborting the settings panel. The configured per-call
// timeout bounds the probe so a stalled endpoint cannot hang the panel.
func TestConnection(ctx context.Context, client *http.Client, cfg ProviderConfig) TestResult {
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := applyTimeout(ctx, cfg.EffectiveTimeout())
	defer cancel()
	start := time.Now()
	models, err := DiscoverModels(ctx, client, cfg)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return TestResult{OK: false, LatencyMS: latency, Error: err.Error()}
	}
	return TestResult{OK: true, LatencyMS: latency, Models: models}
}

// ListModels preserves the legacy OpenAI-compatible model catalog API.
func ListModels(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	return listBearerModels(ctx, client, strings.TrimRight(baseURL, "/")+"/models", apiKey)
}

// DiscoverModels selects model discovery by the explicit provider protocol.
func DiscoverModels(ctx context.Context, client *http.Client, cfg ProviderConfig) ([]string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if cfg.EffectiveType() == ProviderTypeCLI {
		return nil, ErrModelDiscoveryUnsupported
	}
	// Known placeholder endpoints (Agnes 专项) fail with an actionable message
	// instead of a raw DNS error, BEFORE any network request.
	if err := cfg.CheckPlaceholderEndpoint(); err != nil {
		return nil, err
	}
	key, err := cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(cfg.EffectiveBaseURL(), "/")
	switch cfg.EffectiveType() {
	case ProviderTypeGemini:
		return listGeminiModels(ctx, client, base+"/models", key)
	case ProviderTypeDashscope:
		return listBearerModels(ctx, client, dashscopeModelListURL(base), key)
	case ProviderDoubao, ProviderOpenAI, ProviderAgnes, ProviderTypeCompatible,
		ProviderTypeAPI, ProviderTypeMiniMax, ProviderTypeVolcengine:
		return listBearerModels(ctx, client, base+"/models", key)
	default:
		return nil, configErrf("model discovery: unknown provider type %q", cfg.EffectiveType())
	}
}

func dashscopeModelListURL(base string) string {
	base = strings.TrimRight(base, "/")
	if DashscopeRouteMode(base) == DashscopeModeCompatible {
		return base + "/models"
	}
	if strings.HasSuffix(base, "/api/v1") {
		return strings.TrimSuffix(base, "/api/v1") + "/compatible-mode/v1/models"
	}
	return base + "/models"
}

func listBearerModels(ctx context.Context, client *http.Client, endpoint, apiKey string) ([]string, error) {
	raw, status, err := sendAPI(ctx, client, http.MethodGet, endpoint, apiKey, nil, maxModelListBytes, "model list")
	if err != nil {
		return nil, err
	}
	if isAuthStatus(status) {
		return nil, MarkNotRetryable(fmt.Errorf("认证失败（HTTP %d）：%s", status, non2xxDetail(raw, apiKey)))
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("HTTP %d：%s", status, non2xxDetail(raw, apiKey))
	}
	return parseModelList(raw, apiKey)
}

func listGeminiModels(ctx context.Context, client *http.Client, endpoint, apiKey string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("provider: build Gemini models request: %w", err)
	}
	req.Header.Set(headerGeminiAPIKey, apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider: Gemini models request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := readCappedBody(resp.Body, maxModelListBytes, "model list")
	if err != nil {
		return nil, fmt.Errorf("provider: read model list response: %w", err)
	}
	if isAuthStatus(resp.StatusCode) {
		return nil, MarkNotRetryable(fmt.Errorf("认证失败（HTTP %d）：%s", resp.StatusCode, non2xxDetail(raw, apiKey)))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d：%s", resp.StatusCode, non2xxDetail(raw, apiKey))
	}
	return parseModelList(raw, apiKey)
}

func parseModelList(raw []byte, apiKey string) ([]string, error) {
	var out struct {
		Data []struct {
			ID    string `json:"id"`
			Model string `json:"model"`
		} `json:"data"`
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
		Error *apiError `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("provider: decode models response: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return nil, fmt.Errorf("provider: models API error: %s", redactSecret(out.Error.Message, apiKey))
	}
	models := make([]string, 0, len(out.Data)+len(out.Models))
	seen := make(map[string]struct{}, len(out.Data)+len(out.Models))
	add := func(id string) {
		id = strings.TrimSpace(strings.TrimPrefix(id, "models/"))
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	for _, model := range out.Data {
		if model.ID != "" {
			add(model.ID)
		} else {
			add(model.Model)
		}
	}
	for _, model := range out.Models {
		add(model.Name)
	}
	return models, nil
}

func ValidateBaseURLForDiscovery(cfg ProviderConfig) error {
	if cfg.EffectiveType() == ProviderTypeCLI {
		return ErrModelDiscoveryUnsupported
	}
	u, err := url.Parse(cfg.EffectiveBaseURL())
	if err != nil || u.Scheme == "" || u.Host == "" {
		return configErrf("provider %s: invalid base url %q", cfg.ProviderID, cfg.EffectiveBaseURL())
	}
	return nil
}
