package provider

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// capabilityCase is one (config, modality, explicit model) → outcome row of the
// shared capability validation matrix.
type capabilityCase struct {
	name     string
	cfg      ProviderConfig
	modality Modality
	explicit string
	want     string // resolved model on success
	wantErr  error  // sentinel expected on failure (nil = success)
}

// capabilityMatrix covers the task-1.4 boundary: each built-in + compatible
// configuration against image/video/text with default, member and non-member
// models. The matrix doubles as the truth-table of DeclaredCapabilities.
func capabilityMatrix() []capabilityCase {
	doubao := DefaultConfig(ProviderDoubao) // catalogs: image+video+text; caps {I,V?F,T}
	customNoCatalog := ProviderConfig{ProviderID: "custom", Type: ProviderTypeCompatible, Name: "Custom"}
	customWithCatalog := customNoCatalog
	customWithCatalog.ImageModels = []string{"img-a", "img-b"}
	customWithCatalog.TextModels = []string{"txt-a"}

	return []capabilityCase{
		// --- Doubao (image default / member / stranger; text members) ---
		{name: "doubao image resolves preset default", cfg: doubao, modality: ModalityImage, want: DefaultDoubaoModel},
		{name: "doubao image member model accepted", cfg: doubao, modality: ModalityImage, explicit: " " + DefaultDoubaoModel + "\t", want: DefaultDoubaoModel},
		{name: "doubao unknown image model rejected", cfg: doubao, modality: ModalityImage, explicit: "not-a-doubao-model", wantErr: ErrModelInvalid},
		// 视频预留目录不代表可调用: even the PRESET Seedance entry must answer
		// unsupported-capability (never unknown-model — capability checks first).
		{name: "doubao video unsupported despite preset video model", cfg: doubao, modality: ModalityVideo, wantErr: ErrCapabilityUnsupported},
		{name: "doubao video model request still unsupported", cfg: doubao, modality: ModalityVideo, explicit: DefaultDoubaoVideoModel, wantErr: ErrCapabilityUnsupported},
		{name: "doubao text resolves preset default", cfg: doubao, modality: ModalityText, want: DefaultDoubaoTextModel},

		// --- OpenAI gpt-image-2: image only ---
		{name: "openai image resolves default", cfg: DefaultConfig(ProviderOpenAI), modality: ModalityImage, want: DefaultOpenAIModel},
		{name: "openai text unsupported (adapter behavior)", cfg: DefaultConfig(ProviderOpenAI), modality: ModalityText, wantErr: ErrCapabilityUnsupported},
		{name: "openai video unsupported", cfg: DefaultConfig(ProviderOpenAI), modality: ModalityVideo, wantErr: ErrCapabilityUnsupported},

		// --- Agnes: multimodal (image + text; 人工验收反馈) ---
		{name: "agnes image resolves default", cfg: DefaultConfig(ProviderAgnes), modality: ModalityImage, want: DefaultAgnesModel},
		{name: "agnes text resolves preset default", cfg: DefaultConfig(ProviderAgnes), modality: ModalityText, want: DefaultAgnesTextModel},
		{name: "agnes stray text model invalid", cfg: DefaultConfig(ProviderAgnes), modality: ModalityText, explicit: "not-agnes-text", wantErr: ErrModelInvalid},
		{name: "agnes video unsupported", cfg: DefaultConfig(ProviderAgnes), modality: ModalityVideo, wantErr: ErrCapabilityUnsupported},

		// --- Custom compatible without catalogs: declared but unconfigured ---
		{name: "compatible empty image catalog not configured", cfg: customNoCatalog, modality: ModalityImage, wantErr: ErrModelNotConfigured},
		{name: "compatible empty image catalog rejects explicit model too", cfg: customNoCatalog, modality: ModalityImage, explicit: "some-image-model", wantErr: ErrModelNotConfigured},
		{name: "compatible empty text catalog not configured", cfg: customNoCatalog, modality: ModalityText, wantErr: ErrModelNotConfigured},
		{name: "compatible video always unsupported", cfg: customNoCatalog, modality: ModalityVideo, wantErr: ErrCapabilityUnsupported},

		// --- Custom compatible with filled catalogs ---
		{name: "compatible text member accepted", cfg: customWithCatalog, modality: ModalityText, explicit: "txt-a", want: "txt-a"},
		{name: "compatible stray text model invalid", cfg: customWithCatalog, modality: ModalityText, explicit: "gpt-x-stray", wantErr: ErrModelInvalid},
		{name: "compatible padded image member trimmed to catalog entry", cfg: customWithCatalog, modality: ModalityImage, explicit: " img-b ", want: "img-b"},
	}
}

// TestResolveValidatedModelMatrix walks the whole matrix through both API
// shapes (free function with live-adapter capabilities, method form with
// config-derived capabilities) and asserts sentinel error identity via errors.Is.
func TestResolveValidatedModelMatrix(t *testing.T) {
	for _, tc := range capabilityMatrix() {
		t.Run(tc.name, func(t *testing.T) {
			// Method form (capabilities derived from the adapter type).
			got, err := tc.cfg.ResolveValidatedModel(tc.modality, tc.explicit)
			checkCapabilityOutcome(t, got, err, tc)

			// Free function fed by a live adapter's Capabilities() when this
			// config's type has a concrete adapter today.
			var caps Capabilities
			switch tc.cfg.EffectiveType() {
			case ProviderDoubao:
				caps = NewDoubao(tc.cfg, nil).Capabilities()
			case ProviderOpenAI:
				caps = NewOpenAI(tc.cfg, nil).Capabilities()
			case ProviderAgnes:
				caps = NewAgnes(tc.cfg, nil).Capabilities()
			default:
				caps = tc.cfg.DeclaredCapabilities()
			}
			got2, err2 := ResolveValidatedModel(caps, tc.cfg, tc.modality, tc.explicit)
			if got2 != got || (err == nil) != (err2 == nil) {
				t.Fatalf("free-form disagrees with method form: (%q,%v) vs (%q,%v)", got, err, got2, err2)
			}
			if !errors.Is(err2, tc.wantErrEquivalent(err)) {
				t.Errorf("free form sentinel mismatch: %v", err2)
			}

			// ValidateCapability must agree (same sentinels, result discarded).
			verr := ValidateCapability(caps, tc.cfg, tc.modality, tc.explicit)
			if (verr == nil) != (err == nil) {
				t.Fatalf("ValidateCapability disagreement: %v vs %v", verr, err)
			}
			if verr != nil && !errors.Is(verr, tc.wantErr) {
				t.Fatalf("ValidateCapability error = %v, want errors.Is %v", verr, tc.wantErr)
			}
		})
	}
}

// checkCapabilityOutcome asserts one matrix row and cross-checks that unrelated
// sentinels do NOT match (error classes stay distinguishable).
func checkCapabilityOutcome(t *testing.T, got string, err error, tc capabilityCase) {
	t.Helper()
	if tc.wantErr != nil {
		if got != "" {
			t.Errorf("expected no model resolution, got %q", got)
		}
		if !errors.Is(err, tc.wantErr) {
			t.Fatalf("err = %v, want errors.Is(%v)", err, tc.wantErr)
		}
		for _, other := range []error{ErrCapabilityUnsupported, ErrModelNotConfigured, ErrModelInvalid} {
			if other != tc.wantErr && errors.Is(err, other) {
				t.Errorf("error %v must not also match sentinel %v", err, other)
			}
		}
		if !strings.Contains(err.Error(), tc.cfg.ProviderID) {
			t.Errorf("readable error should mention provider id: %v", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != tc.want {
		t.Fatalf("resolved model = %q, want %q", got, tc.want)
	}
}

func (tc capabilityCase) wantErrEquivalent(got error) error {
	return tc.wantErr // keep the closure simple for the free-form assert
}

// TestDeclaredCapabilitiesMatchLiveAdapters guards the single source of truth:
// config-derived declarations must equal every registered adapter type's own
// Capabilities(), so draft validation can never drift from runtime behavior.
// Video stays false everywhere until a real video adapter exists.
func TestDeclaredCapabilitiesMatchLiveAdapters(t *testing.T) {
	custom := ProviderConfig{ProviderID: "custom", Type: ProviderTypeCompatible, Name: "Custom"}
	cliCfg := ProviderConfig{ProviderID: "clitool", Type: ProviderTypeCLI, Name: "CLI Tool", Model: "m"}
	pairs := []struct {
		cfg ProviderConfig
		liv Capabilities
	}{
		{DefaultConfig(ProviderDoubao), NewDoubao(DefaultConfig(ProviderDoubao), nil).Capabilities()},
		{DefaultConfig(ProviderOpenAI), NewOpenAI(DefaultConfig(ProviderOpenAI), nil).Capabilities()},
		{DefaultConfig(ProviderAgnes), NewAgnes(DefaultConfig(ProviderAgnes), nil).Capabilities()},
		{custom, NewCompatible(custom, nil).Capabilities()},
		{cliCfg, NewCLI(cliCfg, nil).Capabilities()},
	}
	for _, p := range pairs {
		want := p.liv
		got := p.cfg.DeclaredCapabilities()
		if got != want {
			t.Errorf("provider %s declared %v but live adapter reports %v", p.cfg.EffectiveType(), got, want)
		}
	}
	// Protocol presets declare per their FrameBaker descriptions; never video.
	dashscope, _ := DeclaredCapabilities(ProviderTypeDashscope)
	volcengine, _ := DeclaredCapabilities(ProviderTypeVolcengine)
	minimax, _ := DeclaredCapabilities(ProviderTypeMiniMax)
	cli, _ := DeclaredCapabilities(ProviderTypeCLI)
	api, _ := DeclaredCapabilities(ProviderTypeAPI)
	for tName, c := range map[string]Capabilities{
		ProviderTypeDashscope:  dashscope,
		ProviderTypeVolcengine: volcengine,
		ProviderTypeMiniMax:    minimax,
		ProviderTypeCLI:        cli,
		ProviderTypeAPI:        api,
	} {
		if c.Video {
			t.Errorf("type %s claims video before any video adapter exists", tName)
		}
		if !c.Image {
			t.Errorf("type %s lost its image declaration", tName)
		}
	}
	if _, ok := DeclaredCapabilities("mystery-type"); ok {
		t.Error("unknown adapter types must not be treated as known")
	}
}

// TestValidationFailureMakesZeroNetworkCalls pins the zero-outbound-call
// boundary with a transport that FAILS THE TEST if it is ever touched while a
// validation runs or rejects. The transport is installed as http.DefaultClient
// (what nil-client call paths fall back to) so any accidental outbound attempt
// during validation becomes visible.
func TestValidationFailureMakesZeroNetworkCalls(t *testing.T) {
	touched := false
	prevDefault := http.DefaultClient
	http.DefaultClient = fakeClient(func(r *http.Request) (*http.Response, error) {
		touched = true
		t.Errorf("capability validation issued an outbound request to %s%s", r.URL.Host, r.URL.Path)
		return jsonResp(200, map[string]any{"data": []any{}}), nil
	})
	t.Cleanup(func() { http.DefaultClient = prevDefault })

	rejecting := []struct {
		name string
		run  func() error
	}{
		{"doubao video with preset video model", func() error {
			return DefaultConfig(ProviderDoubao).ValidateVideoGeneration()
		}},
		{"doubao unknown image model", func() error {
			_, err := DefaultConfig(ProviderDoubao).ResolveValidatedModel(ModalityImage, "ghost-model")
			return err
		}},
		{"openai text request", func() error {
			return DefaultConfig(ProviderOpenAI).ValidateCapability(ModalityText, "chat")
		}},
		{"agnes video request", func() error {
			return DefaultConfig(ProviderAgnes).ValidateVideoGeneration()
		}},
		{"compatible without text models", func() error {
			cfg := ProviderConfig{ProviderID: "c1", Type: ProviderTypeCompatible, Name: "C1"}
			_, err := cfg.ResolveValidatedModel(ModalityText, "")
			return err
		}},
		{"compatible stray text model", func() error {
			cfg := ProviderConfig{ProviderID: "c2", Type: ProviderTypeCompatible, Name: "C2", TextModels: []string{"ok-chat"}}
			return cfg.ValidateCapability(ModalityText, "stray")
		}},
		{"unknown adapter type", func() error {
			cfg := ProviderConfig{ProviderID: "u1", Type: "mystery"}
			_, err := cfg.ResolveValidatedModel(ModalityImage, "")
			return err
		}},
	}
	for _, rj := range rejecting {
		if err := rj.run(); err == nil {
			t.Errorf("%s: expected a rejection", rj.name)
		}
	}
	if touched {
		t.Fatal("the fake transport was reached during offline-only validation")
	}
}

// TestVideoGateBlocksPresetVideoModelsBeforeExternalCall asserts the exact
// contract needed for future filmstrip/video flows: reserved video catalogs
// (Doubao Seedance, DashScope wan2.2) do NOT enable video execution anywhere;
// callers get ErrCapabilityUnsupported they can branch on BEFORE calling out.
func TestVideoGateBlocksPresetVideoModelsBeforeExternalCall(t *testing.T) {
	reservedModels := [][]string{
		DefaultConfig(ProviderDoubao).EffectiveVideoModels(), // doubao-seedance-1-0-pro
		DefaultConfig(ProviderDoubao).VideoModels,            // raw preset catalog field
	}
	for _, models := range reservedModels {
		if len(models) == 0 {
			continue
		}
		cfg := DefaultConfig(ProviderDoubao)
		for _, m := range models {
			err := cfg.ValidateVideoGeneration()
			if !errors.Is(err, ErrCapabilityUnsupported) {
				t.Fatalf("video model %q must stay gated pre-adapters, got %v", m, err)
			}
			if _, err := cfg.ResolveValidatedModel(ModalityVideo, m); !errors.Is(err, ErrCapabilityUnsupported) {
				t.Fatalf("explicit video model %q gated, got %v", m, err)
			}
		}
	}
}
