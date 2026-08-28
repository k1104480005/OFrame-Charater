package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Discovery tests (task 2.6): model listing and connection testing must pick
// the endpoint and authentication of the EXPLICIT protocol type — never
// another vendor's path and never another auth scheme.

func TestDiscoverModelsGeminiUsesHeaderAuthAndStripsPrefix(t *testing.T) {
	var gotPath, gotKey, gotBearer string
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-goog-api-key")
		gotBearer = r.Header.Get("Authorization")
		return jsonResp(200, map[string]any{
			"models": []map[string]any{
				{"name": "models/gemini-2.5-flash-image"},
				{"name": "gemini-2.5-flash"},
				{"name": ""},
			},
		}), nil
	})
	cfg := ProviderConfig{ProviderID: "gem", Type: ProviderTypeGemini, Name: "G", BaseURL: DefaultGeminiBaseURL, APIKey: "gk"}
	models, err := DiscoverModels(context.Background(), client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1beta/models" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotKey != "gk" || gotBearer != "" {
		t.Fatalf("auth: x-goog-api-key=%q Authorization=%q", gotKey, gotBearer)
	}
	want := []string{"gemini-2.5-flash-image", "gemini-2.5-flash"}
	if len(models) != len(want) {
		t.Fatalf("models = %v", models)
	}
	for i := range want {
		if models[i] != want[i] {
			t.Fatalf("models = %v, want %v", models, want)
		}
	}
}

func TestDiscoverModelsDashscopeNativeMapsToCompatibleMode(t *testing.T) {
	var gotURL, gotAuth string
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL.String()
		gotAuth = r.Header.Get("Authorization")
		return jsonResp(200, map[string]any{
			"data": []map[string]any{{"id": "qwen-plus"}},
		}), nil
	})
	cfg := ProviderConfig{ProviderID: "bailian", Type: ProviderTypeDashscope, Name: "百炼", BaseURL: DefaultDashscopeBaseURL, APIKey: "sk-d"}
	if _, err := DiscoverModels(context.Background(), client, cfg); err != nil {
		t.Fatal(err)
	}
	want := "https://dashscope.aliyuncs.com/compatible-mode/v1/models"
	if gotURL != want || gotAuth != "Bearer sk-d" {
		t.Fatalf("url/auth = %q / %q", gotURL, gotAuth)
	}
}

func TestDiscoverModelsDashscopeCompatibleModeStaysOnBase(t *testing.T) {
	var gotURL string
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL.String()
		return jsonResp(200, map[string]any{"data": []map[string]any{{"id": "m"}}}), nil
	})
	cfg := ProviderConfig{ProviderID: "b2", Type: ProviderTypeDashscope, Name: "百炼", BaseURL: DefaultDashscopeCompatibleBaseURL, APIKey: "k"}
	if _, err := DiscoverModels(context.Background(), client, cfg); err != nil {
		t.Fatal(err)
	}
	if gotURL != DefaultDashscopeCompatibleBaseURL+"/models" {
		t.Fatalf("url = %q", gotURL)
	}
}

func TestDiscoverModelsBearerProtocolsUseOwnBase(t *testing.T) {
	cases := []struct {
		name string
		cfg  ProviderConfig
	}{
		{"doubao", ProviderConfig{ProviderID: ProviderDoubao, APIKey: "k"}},
		{"openai", ProviderConfig{ProviderID: ProviderOpenAI, APIKey: "k"}},
		{"compatible", ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible, Name: "C", BaseURL: "https://x.example.com/v1", APIKey: "k"}},
		{"api", ProviderConfig{ProviderID: "a1", Type: ProviderTypeAPI, Name: "A", BaseURL: "https://a.example.com/v1", APIKey: "k"}},
		{"minimax", ProviderConfig{ProviderID: "mm", Type: ProviderTypeMiniMax, Name: "M", BaseURL: DefaultMiniMaxBaseURL, APIKey: "k"}},
		{"volcengine", ProviderConfig{ProviderID: "vol", Type: ProviderTypeVolcengine, Name: "V", BaseURL: DefaultDoubaoBaseURL, APIKey: "k"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotURL, gotAuth string
			client := fakeClient(func(r *http.Request) (*http.Response, error) {
				gotURL = r.URL.String()
				gotAuth = r.Header.Get("Authorization")
				return jsonResp(200, map[string]any{"data": []map[string]any{{"id": "m1"}}}), nil
			})
			if _, err := DiscoverModels(context.Background(), client, tc.cfg); err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(gotURL, "/models") || gotAuth != "Bearer k" {
				t.Fatalf("url/auth = %q / %q", gotURL, gotAuth)
			}
		})
	}
}

func TestDiscoverModelsCLISupportedNeverTouchesNetwork(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		t.Fatal("CLI discovery must not issue network requests")
		return nil, nil
	})
	cfg := ProviderConfig{ProviderID: "cli1", Type: ProviderTypeCLI, Name: "CLI"}
	_, err := DiscoverModels(context.Background(), client, cfg)
	if !errors.Is(err, ErrModelDiscoveryUnsupported) {
		t.Fatalf("err = %v, want ErrModelDiscoveryUnsupported", err)
	}
}

func TestDiscoverModelsMissingKeyFailsBeforeNetwork(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		t.Fatal("missing key must fail before any request")
		return nil, nil
	})
	cfg := ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible, Name: "C", BaseURL: "https://x.example.com/v1"}
	_, err := DiscoverModels(context.Background(), client, cfg)
	if !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("err = %v, want ErrNoAPIKey", err)
	}
}

func TestDiscoverModelsUnknownTypeRejected(t *testing.T) {
	cfg := ProviderConfig{ProviderID: "u1", Type: "nope", Name: "U", BaseURL: "https://x.example.com", APIKey: "k"}
	_, err := DiscoverModels(context.Background(), nil, cfg)
	if err == nil || !strings.Contains(err.Error(), "unknown provider type") {
		t.Fatalf("err = %v", err)
	}
}

func TestDiscoverModelsGeminiAuthFailureClassified(t *testing.T) {
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"bad key"}}`))}, nil
	})
	cfg := ProviderConfig{ProviderID: "gem", Type: ProviderTypeGemini, Name: "G", BaseURL: DefaultGeminiBaseURL, APIKey: "gk"}
	_, err := DiscoverModels(context.Background(), client, cfg)
	if err == nil || !strings.Contains(err.Error(), "认证失败") {
		t.Fatalf("err = %v", err)
	}
	if !IsNotRetryable(err) {
		t.Fatal("auth failure must be not-retryable")
	}
}

func TestTestConnectionRoutesGeminiProtocol(t *testing.T) {
	var gotPath, gotKey string
	client := fakeClient(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-goog-api-key")
		return jsonResp(200, map[string]any{"models": []map[string]any{{"name": "models/gemini-2.5-flash"}}}), nil
	})
	cfg := ProviderConfig{ProviderID: "gem", Type: ProviderTypeGemini, Name: "G", BaseURL: DefaultGeminiBaseURL, APIKey: "gk"}
	res := TestConnection(context.Background(), client, cfg)
	if !res.OK || len(res.Models) != 1 || res.Models[0] != "gemini-2.5-flash" {
		t.Fatalf("result = %+v", res)
	}
	if gotPath != "/v1beta/models" || gotKey != "gk" {
		t.Fatalf("path/key = %q / %q", gotPath, gotKey)
	}
}

func TestValidateBaseURLForDiscovery(t *testing.T) {
	cli := ProviderConfig{ProviderID: "c", Type: ProviderTypeCLI, Name: "C"}
	if err := ValidateBaseURLForDiscovery(cli); !errors.Is(err, ErrModelDiscoveryUnsupported) {
		t.Fatalf("cli err = %v", err)
	}
	bad := ProviderConfig{ProviderID: "c", Type: ProviderTypeAPI, Name: "C", BaseURL: "://nope"}
	if err := ValidateBaseURLForDiscovery(bad); err == nil {
		t.Fatal("expected invalid base url error")
	}
	ok := ProviderConfig{ProviderID: "c", Type: ProviderTypeAPI, Name: "C", BaseURL: "https://x.example.com/v1"}
	if err := ValidateBaseURLForDiscovery(ok); err != nil {
		t.Fatalf("valid url rejected: %v", err)
	}
}
