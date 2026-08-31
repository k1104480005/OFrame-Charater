package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestAgnesWireContract pins the Agnes gateway's OWN request contract, which
// deliberately differs from the generic OpenAI-compatible surface (官方文档 +
// 2026-08-31 网关实测):
//   - `response_format` 只能放在 `extra_body` 内部（"url" 为实测可用的输出路径）；
//     顶层 response_format（历史 "png"/"b64_json"）会让网关把请求吞进永不返回的
//     队列或直接 400；
//   - 不传 `n`（未在官方文档中定义）；
//   - 参考图走 `extra_body.image`（Data URI 字符串数组，无 role 字段），而不是
//     兼容面的顶层 `reference_images`；
//   - `size` 保持顶层 "WxH"；
//   - 响应 data[0].url 由解析器下载（不带 Bearer 凭证）。
func TestAgnesWireContract(t *testing.T) {
	var gotBody map[string]any
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			// The adapter fetched data[0].url — serve the image bytes.
			return jsonResp(200, map[string]any{}), nil
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		return jsonResp(200, map[string]any{"data": []map[string]any{{"url": "https://cdn.example/ok.png"}}}), nil
	})
	cfg := ProviderConfig{ProviderID: ProviderAgnes, Type: ProviderAgnes, Name: "Agnes",
		APIKey: "sk-agnes", Model: "agnes-image-2.0-flash", BaseURL: "https://apihub.agnes-ai.com/v1"}
	p := NewAgnes(cfg, client)
	res, err := p.GenerateImage(context.Background(), ImageRequest{
		Prompt: "hero",
		References: []ReferenceImage{
			{Kind: "reference_image", Role: "main_reference", MIME: "image/png", Data: []byte("abc")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Data) == 0 {
		t.Fatal("expected image bytes from the url fetch")
	}
	if gotBody["model"] != "agnes-image-2.0-flash" || gotBody["prompt"] != "hero" || gotBody["size"] != "1024x1024" {
		t.Fatalf("top-level basics drifted: %v", gotBody)
	}
	if _, exists := gotBody["response_format"]; exists {
		t.Errorf("top-level response_format must not be sent: %v", gotBody["response_format"])
	}
	if _, exists := gotBody["n"]; exists {
		t.Errorf("undocumented n must not be sent: %v", gotBody["n"])
	}
	if _, exists := gotBody["reference_images"]; exists {
		t.Errorf("foreign reference_images field must not be sent: %v", gotBody["reference_images"])
	}
	extra, ok := gotBody["extra_body"].(map[string]any)
	if !ok {
		t.Fatalf("extra_body missing: %v", gotBody["extra_body"])
	}
	if extra["response_format"] != "url" {
		t.Fatalf("extra_body.response_format = %v, want url", extra["response_format"])
	}
	imgs, ok := extra["image"].([]any)
	if !ok || len(imgs) != 1 {
		t.Fatalf("extra_body.image = %v, want one data URI", extra["image"])
	}
	if s, _ := imgs[0].(string); !strings.HasPrefix(s, "data:image/png;base64,") {
		t.Fatalf("reference image data URI = %q", s)
	}
}
