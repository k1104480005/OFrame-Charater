package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DashScope (百炼) adapter — align-framebaker-providers task 2.2.
//
// A DashScope provider never reuses another protocol's identity or wire
// format: the adapter type stays ProviderTypeDashscope end to end, while the
// URL on which Alibaba Cloud actually exposes each dialect decides how a
// request is shaped (design D1/D4):
//
//   - 兼容模式: BaseURL pointing at DashScope's OpenAI-compatible surface
//     (DefaultDashscopeCompatibleBaseURL, "/compatible-mode/…"). Requests go
//     through this package's shared OpenAI-compatible helpers
//     (images/generations + chat/completions), so they carry exactly the same
//     auth, timeout, response caps, error parsing and reference-image contract
//     as task 2.1 — but under THIS adapter's identity, never by silently
//     rebuilding another provider type.
//   - 原生协议: any other BaseURL (default DefaultDashscopeBaseURL,
//     "…/api/v1"). Images use the native Text2Image synthesis endpoint
//     POST {base}/services/aigc/text2image/image-synthesis with Bearer auth,
//     the X-DashScope-Async header and the vendor's own body shape
//     (input.prompt, parameters.size as "W*H"); text uses the native
//     generation endpoint POST {base}/services/aigc/text-generation/generation.
//
// Video models stay catalog metadata only: this adapter declares no video
// capability and performs no video external calls, matching the global video
// gate (ErrCapabilityUnsupported before any network request).

const (
	// DefaultDashscopeCompatibleBaseURL is DashScope's OpenAI-compatible-mode
	// root. Configuring a dashscope provider with this address routes its
	// requests through the shared compatible surface instead of native paths.
	DefaultDashscopeCompatibleBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"

	dashscopeNativeImagePath = "/services/aigc/text2image/image-synthesis" // 原生通义万相 Text2Image 提交端点
	dashscopeNativeTextPath  = "/services/aigc/text-generation/generation" // 原生文本生成端点（同步）
	dashscopeTaskQueryPath   = "/tasks/"                                   // 异步任务轮询：GET {base}/tasks/{task_id}

	headerDashscopeAsync      = "X-DashScope-Async" // 原生图片合成要求显式开启异步
	headerDashscopeAsyncValue = "enable"

	// Task status values of the native async image-synthesis workflow
	// (submit → poll GET /tasks/{id} until a terminal status).
	taskStatusSucceeded  = "SUCCEEDED"
	taskStatusPending    = "PENDING"
	taskStatusRunning    = "RUNNING"
	taskStatusFailed     = "FAILED"
	taskStatusCanceled   = "CANCELED"
	taskStatusUnknownST  = "UNKNOWN"
	dashscopeMaxPollsDef = 60 // submit 后最多轮询次数（安全上限，另有 ctx 超时兜底）
)

// dashscopePollAttempts / dashscopePollDelay bound one async task's polling:
// variables only so fake-transport tests can exercise the boundary instantly;
// production keeps attempts=60 with a 2s interval (ctx deadline still applies).
var (
	dashscopePollAttempts = dashscopeMaxPollsDef
	dashscopePollDelay    = 2 * time.Second
)

// DashscopeMode is the routing decision derived from the configured endpoint
// (任务 2.2: 明确区分兼容模式与原生请求的配置/endpoint 路由).
type DashscopeMode string

const (
	DashscopeModeNative     DashscopeMode = "native"     // 百炼原生协议（Text2Image image-synthesis 等）
	DashscopeModeCompatible DashscopeMode = "compatible" // DashScope OpenAI 兼容模式表面
)

// DashscopeRouteMode decides from the effective BaseURL alone which dialect
// this provider speaks: an OpenAI-compatible-mode address (containing
// "/compatible-mode") uses the shared compatible helpers; every other address
// uses the DashScope-native endpoints. The rule is pure, exported and
// tested — routing is never inferred from model names or provider ids.
func DashscopeRouteMode(baseURL string) DashscopeMode {
	if strings.Contains(baseURL, "/compatible-mode") {
		return DashscopeModeCompatible
	}
	return DashscopeModeNative
}

// Dashscope is the 百炼 (DashScope) protocol adapter for providers created
// from the FrameBaker 百炼 preset (Type ProviderTypeDashscope).
type Dashscope struct {
	cfg    ProviderConfig
	client *http.Client
}

// NewDashscope creates the DashScope adapter (nil client → http.DefaultClient).
func NewDashscope(cfg ProviderConfig, client *http.Client) *Dashscope {
	return &Dashscope{cfg: cfg, client: newClient(client)}
}

// ID returns the configured provider id (百炼 presets create custom, user-named
// providers, unlike the hard-coded built-in identities).
func (d *Dashscope) ID() string { return d.cfg.ProviderID }

// Name returns the display name, falling back to the preset's vendor name.
func (d *Dashscope) Name() string {
	if n := strings.TrimSpace(d.cfg.Name); n != "" {
		return n
	}
	return "百炼 (DashScope)"
}

// Mode reports which wire dialect this config routes to (derived purely from
// the effective BaseURL; see DashscopeRouteMode).
func (d *Dashscope) Mode() DashscopeMode {
	return DashscopeRouteMode(d.cfg.EffectiveBaseURL())
}

// Capabilities mirrors DeclaredCapabilities(ProviderTypeDashscope): image +
// text execute today, video stays false until a real video adapter exists —
// 预留的视频模型目录不代表可调用.
func (d *Dashscope) Capabilities() Capabilities {
	return Capabilities{Image: true, Video: false, Text: true}
}

func (d *Dashscope) DefaultImageModel() string { return d.cfg.EffectiveModel() }

func (d *Dashscope) DefaultTextModel() string { return d.cfg.EffectiveTextModel() }

// GenerateImage generates one image through the routed mode.
func (d *Dashscope) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	key, err := d.cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	model := ResolveModel(req.Model, "")
	if model == "" {
		if cat := d.cfg.EffectiveImageModels(); len(cat) > 0 {
			model = cat[0]
		}
	}
	if model == "" {
		return nil, MarkNotRetryable(configErrf("provider %s: no image model configured", d.ID()))
	}
	ctx, cancel := applyTimeout(ctx, d.cfg.EffectiveTimeout())
	defer cancel()

	if d.Mode() == DashscopeModeCompatible {
		data, mime, err := imagesGenerations(ctx, d.client, d.cfg.EffectiveBaseURL(), key, model, req.Prompt, req.Width, req.Height, req.References)
		if err != nil {
			return nil, err
		}
		return &ImageResult{Data: data, MIME: mime, Provider: d.ID(), Model: model}, nil
	}
	return d.nativeGenerateImage(ctx, key, model, req)
}

// nativeGenerateImage runs the DashScope-native text-to-image flow: POST
// image-synthesis (X-DashScope-Async), parse a synchronous results payload
// when present, otherwise poll GET /tasks/{id} until terminal status.
//
// The native API has no inline reference-image field for these models, so a
// request carrying references is rejected BEFORE any external call (清晰错误、
// 零外呼) instead of silently dropping or mis-encoding the input.
func (d *Dashscope) nativeGenerateImage(ctx context.Context, key, model string, req ImageRequest) (*ImageResult, error) {
	if len(req.References) > 0 {
		return nil, MarkNotRetryable(configErrf(
			"provider %s (dashscope native): reference images are not supported by the native text2image image-synthesis API (%d attached); configure the DashScope compatible-mode BaseURL %q if you need a compatible surface",
			d.ID(), len(req.References), DefaultDashscopeCompatibleBaseURL))
	}
	width, height := req.Width, req.Height
	if width <= 0 {
		width = DefaultGenerationSize
	}
	if height <= 0 {
		height = DefaultGenerationSize
	}
	body := map[string]any{
		"model": model,
		"input": map[string]any{"prompt": req.Prompt},
		// DashScope 原生 size 使用 "宽*高" 分隔符（区别于 OpenAI 的 "宽x 高"—
		// 显式按本协议形状构造，不套用另一协议的字段）。
		"parameters": map[string]any{
			"size": fmt.Sprintf("%d*%d", width, height),
			"n":    1,
		},
	}
	raw, status, err := postJSONWithHeaders(ctx, d.client, d.cfg.EffectiveBaseURL()+dashscopeNativeImagePath, key,
		map[string]string{headerDashscopeAsync: headerDashscopeAsyncValue}, body)
	if err != nil {
		return nil, err
	}
	if err := classifyDashscopeStatus("image-synthesis submit", status, raw, key); err != nil {
		return nil, err
	}

	var sub struct {
		Output  dashscopeOutput `json:"output"`
		Code    string          `json:"code"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(raw, &sub); err != nil {
		return nil, fmt.Errorf("provider: decode dashscope submit response: %w", err)
	}
	out := sub.Output
	// Synchronous payload support ("同步 fake 响应"): when the submission answer
	// already carries final results there is no polling round-trip at all.
	if data, mime, ok, perr := dashscopeImageFromResults(out.Results, ctx, d.client); ok || perr != nil {
		if perr != nil {
			return nil, perr
		}
		return &ImageResult{Data: data, MIME: mime, Provider: d.ID(), Model: model}, nil
	}
	submitStatus := strings.ToUpper(strings.TrimSpace(out.TaskStatus))
	switch submitStatus {
	case taskStatusFailed, taskStatusCanceled, taskStatusUnknownST:
		return nil, dashscopeTaskFailureError(submitStatus, out, key)
	}
	if out.TaskID == "" {
		return nil, fmt.Errorf("provider: dashscope submit response has no task_id: %s", non2xxDetail(raw, key))
	}
	rawRes, err := d.pollDashscopeTask(ctx, out.TaskID, key)
	if err != nil {
		return nil, err
	}
	var done struct {
		Output dashscopeOutput `json:"output"`
	}
	if err := json.Unmarshal(rawRes, &done); err != nil {
		return nil, fmt.Errorf("provider: decode dashscope task response: %w", err)
	}
	data, mime, ok, perr := dashscopeImageFromResults(done.Output.Results, ctx, d.client)
	if !ok || perr != nil {
		if perr != nil {
			return nil, perr
		}
		return nil, fmt.Errorf("provider: dashscope task %s succeeded but returned no image results", out.TaskID)
	}
	return &ImageResult{Data: data, MIME: mime, Provider: d.ID(), Model: model}, nil
}

// dashscopeOutput is the shared output block of native DashScope responses.
type dashscopeOutput struct {
	TaskID     string                 `json:"task_id"`
	TaskStatus string                 `json:"task_status"`
	Code       string                 `json:"code"`
	Message    string                 `json:"message"`
	Results    []dashscopeImageResult `json:"results"`
}

type dashscopeImageResult struct {
	URL     string `json:"url"`
	B64JSON string `json:"b64_json"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// dashscopeTaskFailureError renders a terminal non-SUCCEEDED native task
// status as a readable error: vendor code + message first (task-level, then
// per-result fallbacks), never leaking the API key.
func dashscopeTaskFailureError(status string, out dashscopeOutput, apiKey string) error {
	detail := strings.TrimSpace(out.Message)
	if detail == "" && len(out.Results) > 0 {
		detail = strings.TrimSpace(out.Results[0].Message)
	}
	if c := strings.TrimSpace(out.Code); c != "" {
		switch detail {
		case "":
			detail = c
		default:
			detail = c + ": " + detail
		}
	}
	if detail == "" {
		detail = "(vendor gave no reason)"
	}
	return fmt.Errorf("provider: dashscope image-synthesis task ended with status %s: %s", status, redactSecret(detail, apiKey))
}

// dashscopeImageFromResults extracts the first usable result item. It returns
// ok=false (no error) when the results list carries nothing usable yet — the
// caller decides whether that means "keep polling" or "readable failure".
func dashscopeImageFromResults(results []dashscopeImageResult, ctx context.Context, client *http.Client) ([]byte, string, bool, error) {
	for _, r := range results {
		switch {
		case r.B64JSON != "":
			data, err := base64.StdEncoding.DecodeString(r.B64JSON)
			if err != nil {
				return nil, "", true, fmt.Errorf("provider: decode b64 image: %w", err)
			}
			return data, "image/png", true, nil
		case r.URL != "":
			data, mime, err := fetchGeneratedImage(ctx, client, r.URL)
			if err != nil {
				return nil, "", true, err
			}
			return data, mime, true, nil
		case r.Code != "" || r.Message != "":
			detail := strings.TrimSpace(r.Message)
			if c := strings.TrimSpace(r.Code); c != "" {
				if detail == "" {
					detail = c
				} else {
					detail = c + ": " + detail
				}
			}
			return nil, "", true, fmt.Errorf("provider: dashscope image result failed: %s", detail)
		}
	}
	return nil, "", false, nil
}

// fetchGeneratedImage downloads a generated image from the vendor's CDN using
// the SAME rules as the shared images/generations path: no Authorization
// header on arbitrary URLs (the credential must never reach a foreign host)
// and the same response-size cap.
func fetchGeneratedImage(ctx context.Context, client *http.Client, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

// pollDashscopeTask polls GET {base}/tasks/{task_id} until the native task
// reaches SUCCEEDED or a failure state. Polling honors the context deadline,
// keeps the unified auth/read-cap/error layer and gives readable failures
// (vendor code/message first, exhausted polls naming the task id).
func (d *Dashscope) pollDashscopeTask(ctx context.Context, taskID, key string) ([]byte, error) {
	url := d.cfg.EffectiveBaseURL() + dashscopeTaskQueryPath + taskID
	var lastRaw []byte
	for attempt := 1; ; attempt++ {
		req, err := newAuthedRequest(ctx, http.MethodGet, url, key, nil)
		if err != nil {
			return nil, err
		}
		resp, err := d.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("provider: dashscope task poll failed: %w", err)
		}
		raw, err := readCappedBody(resp.Body, maxGenerationResponseBytes, "generation")
		closeErr := resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("provider: read generation response: %w", err)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("provider: close task poll response: %w", closeErr)
		}
		lastRaw = raw
		if isAuthStatus(resp.StatusCode) {
			return nil, MarkNotRetryable(fmt.Errorf("provider: auth failed (HTTP %d): %s", resp.StatusCode, non2xxDetail(raw, key)))
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("provider: dashscope task poll unexpected status %d: %s", resp.StatusCode, non2xxDetail(raw, key))
		}

		var st struct {
			Output dashscopeOutput `json:"output"`
		}
		if err := json.Unmarshal(raw, &st); err != nil {
			return nil, fmt.Errorf("provider: decode dashscope task response: %w", err)
		}
		status := strings.ToUpper(strings.TrimSpace(st.Output.TaskStatus))
		switch status {
		case taskStatusSucceeded:
			return raw, nil
		case taskStatusFailed, taskStatusCanceled, taskStatusUnknownST:
			return nil, dashscopeTaskFailureError(status, st.Output, key)
		case "", taskStatusPending, taskStatusRunning:
			// keep waiting below
		default:
			// 未知的中间状态不能当作失败丢弃（厂商可能新增状态）——继续等待，
			// 直到上限或终端状态。Next iteration re-reads the fresh state.
		}
		if attempt >= dashscopePollAttempts {
			body := "(…)"
			if s := strings.TrimSpace(string(lastRaw)); s != "" {
				body = truncate(s, 120)
			}
			return nil, fmt.Errorf("provider: dashscope task %s did not finish after %d polls (last status %q): %s", taskID, attempt, status, body)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("provider: dashscope task %s polling canceled: %w", taskID, ctx.Err())
		case <-time.After(dashscopePollDelay):
		}
	}
}

// GenerateText generates text through the routed mode: compatible mode uses
// the shared chat/completions helper; native mode posts the DashScope text
// generation contract ({model, input.messages}) and reads output.text or
// output.choices[0].message.content.
func (d *Dashscope) GenerateText(ctx context.Context, req TextRequest) (*TextResult, error) {
	key, err := d.cfg.ResolveAPIKey()
	if err != nil {
		return nil, err
	}
	model := ResolveModel(req.Model, "")
	if model == "" {
		if cat := d.cfg.EffectiveTextModels(); len(cat) > 0 {
			model = cat[0]
		}
	}
	if model == "" {
		return nil, MarkNotRetryable(configErrf("provider %s: no text model configured", d.ID()))
	}
	ctx, cancel := applyTimeout(ctx, d.cfg.EffectiveTimeout())
	defer cancel()

	if d.Mode() == DashscopeModeCompatible {
		text, err := chatCompletionText(ctx, d.client, d.cfg.EffectiveBaseURL(), key, model, req.Prompt, req.ImageDataURL)
		if err != nil {
			return nil, err
		}
		return &TextResult{Text: text, Provider: d.ID(), Model: model}, nil
	}

	body := map[string]any{
		"model": model,
		"input": map[string]any{
			"messages": []map[string]string{{"role": "user", "content": req.Prompt}},
		},
	}
	raw, err := postJSON(ctx, d.client, d.cfg.EffectiveBaseURL()+dashscopeNativeTextPath, key, body)
	if err != nil {
		return nil, err
	}
	var out struct {
		Output struct {
			Text    string `json:"text"`
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		} `json:"output"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("provider: decode dashscope text response: %w", err)
	}
	if m := strings.TrimSpace(out.Message); m != "" && out.Output.Text == "" && len(out.Output.Choices) == 0 {
		code := strings.TrimSpace(out.Code)
		return nil, fmt.Errorf("provider: dashscope text API error: %s", redactSecret(code+": "+m, key))
	}
	text := strings.TrimSpace(out.Output.Text)
	if text == "" && len(out.Output.Choices) > 0 {
		text = strings.TrimSpace(out.Output.Choices[0].Message.Content)
	}
	if text == "" {
		return nil, fmt.Errorf("provider: dashscope text response has no content")
	}
	return &TextResult{Text: text, Provider: d.ID(), Model: model}, nil
}

// postJSONWithHeaders is the dashscope-specific authenticated POST. It reuses
// the shared building blocks exactly like sendAPI/postJSON (newAuthedRequest
// for Bearer auth, readCappedBody for the response cap) and additionally sets
// protocol headers such as X-DashScope-Async on the same request.
func postJSONWithHeaders(ctx context.Context, client *http.Client, url, apiKey string, extraHeaders map[string]string, body any) ([]byte, int, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("provider: encode request: %w", err)
	}
	req, err := newAuthedRequest(ctx, http.MethodPost, url, apiKey, data)
	if err != nil {
		return nil, 0, err
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("provider: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := readCappedBody(resp.Body, maxGenerationResponseBytes, "generation")
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("provider: read generation response: %w", err)
	}
	return raw, resp.StatusCode, nil
}

// classifyDashscopeStatus turns a non-2xx native call into the same error
// classes as the shared postJSON path: 401/403 not-retryable, other statuses
// retryable, vendor message preferred over a raw dump, keys always redacted.
func classifyDashscopeStatus(action string, statusCode int, raw []byte, apiKey string) error {
	if isAuthStatus(statusCode) {
		return MarkNotRetryable(fmt.Errorf("provider: auth failed (HTTP %d): %s", statusCode, non2xxDetail(raw, apiKey)))
	}
	if statusCode < 200 || statusCode >= 300 {
		return fmt.Errorf("provider: dashscope %s unexpected status %d: %s", action, statusCode, dashscopeErrorDetail(raw, apiKey))
	}
	return nil
}

// dashscopeErrorDetail prefers the DashScope-native top-level
// {"code":"…","message":"…"} envelope, falls back to the shared detail builder
// (nested error.message → truncated echo → placeholder), keeping errors
// readable and secret-free either way.
func dashscopeErrorDetail(raw []byte, apiKey string) string {
	var probe struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &probe); err == nil && strings.TrimSpace(probe.Message) != "" {
		msg := strings.TrimSpace(probe.Message)
		if c := strings.TrimSpace(probe.Code); c != "" {
			msg = c + ": " + msg
		}
		return truncate(redactSecret(msg, apiKey), 300)
	}
	return non2xxDetail(raw, apiKey)
}
