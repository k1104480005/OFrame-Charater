package provider

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Prompt-enhancement association tests (task 5.5) and the DefaultSize field
// (task 1.1/5.2 尺寸): association normalization clears dangling provider
// references, and the advisory size survives a JSON round trip.

func TestNormalizeSettingsEnhanceAssociation(t *testing.T) {
	base := func() Settings {
		return Settings{
			ActiveProvider: ProviderDoubao,
			Providers: map[string]ProviderConfig{
				ProviderDoubao: DefaultConfig(ProviderDoubao),
			},
		}
	}

	// A dangling enhance provider reference is cleared (缺失 Provider 场景).
	in := base()
	in.EnhanceProviderID = "deleted-provider"
	in.EnhanceModel = "some-chat-model"
	out := NormalizeSettings(in)
	if out.EnhanceProviderID != "" || out.EnhanceModel != "" {
		t.Fatalf("dangling enhance association survived: %+v", out)
	}

	// A valid reference is kept verbatim.
	in.EnhanceProviderID = ProviderDoubao
	in.EnhanceModel = "doubao-1-5-pro-32k"
	out = NormalizeSettings(in)
	if out.EnhanceProviderID != ProviderDoubao || out.EnhanceModel != "doubao-1-5-pro-32k" {
		t.Fatalf("valid enhance association drifted: %+v", out)
	}

	// The association is idempotent under normalization.
	if again := NormalizeSettings(out); !reflect.DeepEqual(again, out) {
		t.Fatalf("normalization not idempotent: %+v vs %+v", again, out)
	}
}

func TestDefaultSizeRoundTrip(t *testing.T) {
	cfg := ProviderConfig{
		ProviderID:  "c1",
		Type:        ProviderTypeCompatible,
		Name:        "C1",
		Model:       "m1",
		DefaultSize: "768x768",
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var back ProviderConfig
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, back) {
		t.Fatalf("round trip drift:\n got %#v\nwant %#v", back, cfg)
	}
	if !jsonContains(string(raw), "defaultSize") {
		t.Fatalf("defaultSize not serialized: %s", raw)
	}
}

func jsonContains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
