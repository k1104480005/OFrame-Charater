package provider

import (
	"encoding/json"
	"reflect"
	"testing"
)

// --- settings read normalization & migration (align-framebaker-providers
// task 1.2): pure-function unit tests; the store-level restart behavior is
// covered by core/settings. ---

// assertIdempotent verifies normalize(normalize(s)) == normalize(s).
func assertIdempotent(t *testing.T, s Settings) {
	t.Helper()
	if !reflect.DeepEqual(s, NormalizeSettings(s)) {
		t.Fatalf("normalization not idempotent: %+v → %+v", s, NormalizeSettings(s))
	}
}

// TestNormalizeSettingsKeepsFreshInstallEmpty covers empty/missing provider
// data after the 人工验收 update: a zero payload, nil map and empty map all
// stay EMPTY (no built-ins are re-seeded — deleted providers stay deleted),
// the active slot clears, and a dangling enhance association clears too.
func TestNormalizeSettingsKeepsFreshInstallEmpty(t *testing.T) {
	cases := []struct {
		name string
		in   Settings
	}{
		{"zero value settings", Settings{}},
		{"nil providers with stray active", Settings{ActiveProvider: "ghost-provider"}},
		{"empty providers map", Settings{ActiveProvider: ProviderDoubao, Providers: map[string]ProviderConfig{}}},
		{"whitespace-only active, no providers", Settings{ActiveProvider: "   "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeSettings(tc.in)
			if got.ActiveProvider != "" {
				t.Errorf("active = %q, want empty (no providers to activate)", got.ActiveProvider)
			}
			if len(got.Providers) != 0 {
				t.Fatalf("providers = %d entries, want 0 (no silent re-seed)", len(got.Providers))
			}
			if got.EnhanceProviderID != "" || got.EnhanceModel != "" {
				t.Errorf("enhance association survived an empty store: %+v", got)
			}
			assertIdempotent(t, got)
		})
	}
}

// TestNormalizeSettingsPreservesEntriesVerbatim pins that existing entries —
// built-ins, compatible customs and customs carrying an explicit future
// protocol type — survive normalization byte-for-byte (per field), so nothing
// is silently coerced to "compatible" and legacy singular/array fields are
// left exactly as stored.
func TestNormalizeSettingsPreservesEntriesVerbatim(t *testing.T) {
	customGemini := ProviderConfig{
		ProviderID: "banana-direct", Type: ProviderTypeGemini, Name: "Banana Direct",
		BaseURL: "https://g.example.com/v1", Model: "gemini-img-pro",
		APIKey: "g-key",
	}
	customVolc := ProviderConfig{
		ProviderID: "ark-native", Type: ProviderTypeVolcengine, Name: "Ark Native",
		BaseURL: "https://ark.example.com/api/v3", Model: "seedream-x", APIKey: "v-key",
	}
	legacyDoubao := ProviderConfig{
		ProviderID: ProviderDoubao, Type: ProviderDoubao, APIKey: "k-d",
		Model: "legacy-img", TextModel: "legacy-txt",
		ImageModels: []string{" ", "dup", " dup "}, // blanks/dupes stay as stored
	}
	compatCustom := ProviderConfig{
		ProviderID: "local-runner", Type: ProviderTypeCompatible, Name: "Local Runner",
		BaseURL: "http://127.0.0.1:8000/v1", Model: "runner-model",
	}
	bareOpenAI := ProviderConfig{ProviderID: ProviderOpenAI} // no type, no fields at all

	in := Settings{
		ActiveProvider: "banana-direct",
		Providers: map[string]ProviderConfig{
			ProviderDoubao:  legacyDoubao,
			"banana-direct": customGemini,
			"ark-native":    customVolc,
			"local-runner":  compatCustom,
			ProviderOpenAI:  bareOpenAI,
		},
	}
	got := NormalizeSettings(in)

	for _, tc := range []struct {
		id   string
		want ProviderConfig
	}{
		{ProviderDoubao, legacyDoubao},
		{"banana-direct", customGemini},
		{"ark-native", customVolc},
		{"local-runner", compatCustom},
		{ProviderOpenAI, bareOpenAI},
	} {
		gotCfg, ok := got.Providers[tc.id]
		if !ok {
			t.Fatalf("entry %q lost during normalization", tc.id)
		}
		if !reflect.DeepEqual(gotCfg, tc.want) {
			t.Errorf("entry %q changed: got %+v, want %+v", tc.id, gotCfg, tc.want)
		}
	}
	// Explicit types kept, never silently rewritten to compatible.
	for id, want := range map[string]string{
		"banana-direct": ProviderTypeGemini,
		"ark-native":    ProviderTypeVolcengine,
		"local-runner":  ProviderTypeCompatible,
	} {
		if typ := got.ConfigFor(id).EffectiveType(); typ != want {
			t.Errorf("%s effective type = %q, want %q", id, typ, want)
		}
	}
	// Legacy singular fields still feed the effective lists after migration.
	if l := got.ConfigFor(ProviderDoubao).EffectiveImageModels(); !equalStrings(l, []string{"dup"}) {
		t.Errorf("doubao image fallback = %v, want [dup]", l)
	}
	if l := got.ConfigFor(ProviderDoubao).EffectiveTextModels(); !equalStrings(l, []string{"legacy-txt"}) {
		t.Errorf("doubao text fallback = %v, want [legacy-txt]", l)
	}
	// Built-in entry without any stored model falls back to its default catalog.
	if l := got.ConfigFor(ProviderOpenAI).EffectiveImageModels(); !equalStrings(l, []string{DefaultOpenAIModel}) {
		t.Errorf("openai default catalog = %v, want [%s]", l, DefaultOpenAIModel)
	}
	// No new entries injected into a non-empty map.
	if len(got.Providers) != len(in.Providers) {
		t.Errorf("providers grew from %d to %d during normalization", len(in.Providers), len(got.Providers))
	}
	assertIdempotent(t, got)
}

// TestNormalizeSettingsRecoversActiveProvider tables the active-slot recovery:
// invalid/hanging actives fall back deterministically instead of leaving a
// dangling reference across a restart.
func TestNormalizeSettingsRecoversActiveProvider(t *testing.T) {
	cases := []struct {
		name       string
		active     string
		providers  map[string]ProviderConfig
		wantActive string
	}{
		{
			name:       "active points at deleted provider, doubao present",
			active:     "removed-one",
			providers:  map[string]ProviderConfig{ProviderDoubao: {}, ProviderOpenAI: {}},
			wantActive: ProviderDoubao,
		},
		{
			name:       "no active set, first remaining builtin wins",
			active:     "",
			providers:  map[string]ProviderConfig{ProviderAgnes: {}, ProviderOpenAI: {}, "zeta-llm": {ProviderID: "zeta-llm"}},
			wantActive: ProviderOpenAI,
		},
		{
			name:       "only custom providers → lexicographic first stays deterministic",
			active:     "gone",
			providers:  map[string]ProviderConfig{"zeta-llm": {ProviderID: "zeta-llm"}, "banana-direct": {ProviderID: "banana-direct"}},
			wantActive: "banana-direct",
		},
		{
			name:   "valid custom active preserved verbatim",
			active: "banana-direct",
			providers: map[string]ProviderConfig{
				ProviderDoubao:  {},
				"banana-direct": {ProviderID: "banana-direct", Type: ProviderTypeAPI},
			},
			wantActive: "banana-direct",
		},
		{
			name:       "surrounding whitespace trimmed from valid active",
			active:     "  openai  ",
			providers:  map[string]ProviderConfig{ProviderDoubao: {}, ProviderOpenAI: {}},
			wantActive: ProviderOpenAI,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeSettings(Settings{ActiveProvider: tc.active, Providers: tc.providers})
			if got.ActiveProvider != tc.wantActive {
				t.Fatalf("active = %q, want %q", got.ActiveProvider, tc.wantActive)
			}
			if _, ok := got.Providers[got.ActiveProvider]; !ok {
				t.Fatalf("recovered active %q has no config entry", got.ActiveProvider)
			}
			// Entries other than the recovered active remain untouched.
			for id := range tc.providers {
				if !reflect.DeepEqual(got.Providers[id], tc.providers[id]) {
					t.Errorf("entry %q mutated during recovery", id)
				}
			}
			assertIdempotent(t, got)
		})
	}
}

// TestNormalizeSettingsMatchesJSONRoundTrip ensures the exported function's
// result unmarshals/marshals without dropping unknown-id entries — forward
// compatibility for settings written by future versions.
func TestNormalizeSettingsMatchesJSONRoundTrip(t *testing.T) {
	in := `{
	  "activeProvider":"future-thing",
	  "providers":{
	    "future-thing":{"providerId":"future-thing","type":"minimax","name":"Future","apiKey":"fk","unknownField":{"nested":1}},
	    "doubao":{"providerId":"doubao","type":"doubao","model":"kept"}
	  }
	}`
	var s Settings
	if err := json.Unmarshal([]byte(in), &s); err != nil {
		t.Fatal(err)
	}
	got := NormalizeSettings(s)
	if got.ActiveProvider != "future-thing" || got.ConfigFor("future-thing").EffectiveType() != ProviderTypeMiniMax {
		t.Fatalf("future entry not preserved: %+v", got)
	}
	out, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var again Settings
	if err := json.Unmarshal(out, &again); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, got) {
		t.Fatalf("json round trip lost data:\n got  %+v\n again %+v", got, again)
	}
}

// TestNormalizeSettingsMigratesAgnesPlaceholderURL (人工验收反馈: Agnes 真实
// 网关接入): an agnes entry still carrying the legacy placeholder host is
// rewritten to the real gateway on load, and the old invented MODEL names
// migrate to the documented ids — 用户的旧 Agnes 卡片重启即可用.
// Non-agnes entries with the same host are NOT touched, and the rewrite is
// idempotent.
func TestNormalizeSettingsMigratesAgnesPlaceholderURL(t *testing.T) {
	in := Settings{
		ActiveProvider: "agnes",
		Providers: map[string]ProviderConfig{
			"agnes": {
				ProviderID: "agnes", Type: ProviderAgnes, Name: "Agnes", APIKey: "k",
				BaseURL: "https://api.agnes.local/v1",
				Model:   "agnes-image-v1", TextModel: "agnes-text-v1",
				ImageModels: []string{"agnes-image-v1", "custom-img"},
				TextModels:  []string{"agnes-text-v1"},
				VideoModels: []string{"agnes-video-v1"},
			},
			"custom1": {ProviderID: "custom1", Type: ProviderTypeCompatible, Name: "C", BaseURL: "https://api.agnes.local/v1"},
		},
	}
	got := NormalizeSettings(in)
	agnes := got.ConfigFor("agnes")
	if agnes.BaseURL != DefaultAgnesBaseURL {
		t.Fatalf("agnes url = %q, want %q", agnes.BaseURL, DefaultAgnesBaseURL)
	}
	if agnes.Model != DefaultAgnesModel || agnes.TextModel != DefaultAgnesTextModel {
		t.Fatalf("agnes singular models = %q/%q", agnes.Model, agnes.TextModel)
	}
	if !equalStrings(agnes.ImageModels, []string{DefaultAgnesModel, "custom-img"}) {
		t.Fatalf("agnes image list = %v", agnes.ImageModels)
	}
	if !equalStrings(agnes.TextModels, []string{DefaultAgnesTextModel}) {
		t.Fatalf("agnes text list = %v", agnes.TextModels)
	}
	// 视频预留名保持现状（仍是当前占位 ID）。
	if !equalStrings(agnes.VideoModels, []string{DefaultAgnesVideoModel}) {
		t.Fatalf("agnes video list = %v", agnes.VideoModels)
	}
	// Non-agnes entries keep their stored URL verbatim.
	if got.ConfigFor("custom1").BaseURL != "https://api.agnes.local/v1" {
		t.Fatalf("non-agnes url was rewritten: %q", got.ConfigFor("custom1").BaseURL)
	}
	// Idempotent: the rewritten values are stable across normalization.
	if !reflect.DeepEqual(got, NormalizeSettings(got)) {
		t.Fatal("agnes URL migration not idempotent")
	}
}
