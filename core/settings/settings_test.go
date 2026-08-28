package settings

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/oframe/character-workbench/core/provider"
)

// TestNewInitializesDefaults verifies a fresh store starts EMPTY (人工验收更新:
// no provider cards are pre-seeded — users add providers from the presets).
func TestNewInitializesDefaults(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	ps := s.ProviderSettings()
	if ps.ActiveProvider != "" {
		t.Fatalf("active = %q, want empty", ps.ActiveProvider)
	}
	if len(ps.Providers) != 0 {
		t.Fatalf("providers = %d, want 0 on a fresh install", len(ps.Providers))
	}
	if _, err := os.Stat(s.Path()); err != nil {
		t.Fatalf("settings file missing: %v", err)
	}
}

// TestSaveAndReload verifies keys/models persist across reloads. The doubao
// entry is seeded explicitly — a fresh store carries no provider cards.
func TestSaveAndReload(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ps := s.ProviderSettings()
	cfg := provider.DefaultConfig(provider.ProviderDoubao)
	cfg.APIKey = "ark-secret-key"
	cfg.Model = "doubao-custom-model"
	ps.Providers[provider.ProviderDoubao] = cfg
	ps.ActiveProvider = provider.ProviderDoubao
	if err := s.SaveProviderSettings(ps); err != nil {
		t.Fatal(err)
	}

	reloaded, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.ProviderSettings().ConfigFor(provider.ProviderDoubao)
	if got.APIKey != "ark-secret-key" || got.Model != "doubao-custom-model" {
		t.Fatalf("reloaded config: %+v", got)
	}
}

// TestRecordCallPersists verifies call statistics survive a reload (spec 4.6:
// 每次调用后统计更新).
func TestRecordCallPersists(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordCall(provider.ProviderDoubao, "m1", 0.05); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordCall(provider.ProviderDoubao, "m1", 0.05); err != nil {
		t.Fatal(err)
	}
	reloaded, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	stats := reloaded.Stats()
	if stats.TotalCalls() != 2 {
		t.Fatalf("total calls = %d", stats.TotalCalls())
	}
	doubao := stats.ForProvider(provider.ProviderDoubao)
	if len(doubao) != 1 || doubao[0].CallCount != 2 || doubao[0].EstimatedCost != 0.10 {
		t.Fatalf("doubao stats: %+v", doubao)
	}
}

// TestCorruptFileRefused verifies a corrupt settings file is refused and not
// silently overwritten.
func TestCorruptFileRefused(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(dir); err == nil {
		t.Fatal("expected error for corrupt settings file")
	}
	data, _ := os.ReadFile(filepath.Join(dir, FileName))
	if string(data) != "{not json" {
		t.Fatal("corrupt settings file was modified")
	}
}

// --- categorized model catalogs (align-framebaker-providers task 1.1) ---

// TestNewDefaultsClassifyBuiltInModels verifies the ConfigFor default
// fallback: an id ABSENT from the (empty) fresh store still resolves through
// its built-in defaults — Doubao image+video+text, OpenAI image only, and the
// multimodal Agnes image+text (人工验收反馈). The store itself seeds no cards.
func TestNewDefaultsClassifyBuiltInModels(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	ps := s.ProviderSettings()
	if len(ps.Providers) != 0 {
		t.Fatalf("fresh store providers = %d, want 0 (no pre-seed)", len(ps.Providers))
	}

	doubao := ps.ConfigFor(provider.ProviderDoubao)
	if !equalStrings(doubao.EffectiveImageModels(), []string{provider.DefaultDoubaoModel}) {
		t.Errorf("doubao image list = %v", doubao.EffectiveImageModels())
	}
	if !equalStrings(doubao.EffectiveVideoModels(), []string{provider.DefaultDoubaoVideoModel}) {
		t.Errorf("doubao video list = %v", doubao.EffectiveVideoModels())
	}
	if !equalStrings(doubao.EffectiveTextModels(), []string{provider.DefaultDoubaoTextModel}) {
		t.Errorf("doubao text list = %v", doubao.EffectiveTextModels())
	}

	openai := ps.ConfigFor(provider.ProviderOpenAI)
	if !equalStrings(openai.EffectiveImageModels(), []string{provider.DefaultOpenAIModel}) {
		t.Errorf("openai image list = %v", openai.EffectiveImageModels())
	}
	if openai.EffectiveVideoModels() != nil || openai.EffectiveTextModels() != nil {
		t.Errorf("openai video/text lists = %v/%v, want nil/nil", openai.EffectiveVideoModels(), openai.EffectiveTextModels())
	}

	agnes := ps.ConfigFor(provider.ProviderAgnes)
	if !equalStrings(agnes.EffectiveImageModels(), []string{provider.DefaultAgnesModel}) {
		t.Errorf("agnes image list = %v", agnes.EffectiveImageModels())
	}
	if !equalStrings(agnes.EffectiveTextModels(), []string{provider.DefaultAgnesTextModel}) {
		t.Errorf("agnes text list = %v (multimodal default expected)", agnes.EffectiveTextModels())
	}
}

// TestLegacySettingsFileFallsBackToSingularFields writes an old-shape settings
// file (no model arrays) and verifies reading it keeps the singular fields as
// the effective one-entry lists while built-in default classification still
// applies — and that a pure read does not rewrite the file.
func TestLegacySettingsFileFallsBackToSingularFields(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{
	  "provider": {
	    "activeProvider": "doubao",
	    "providers": {
	      "doubao": {"providerId":"doubao","type":"doubao","apiKey":"k","model":"legacy-image-m","textModel":"legacy-text-m"},
	      "openai": {"providerId":"openai","type":"openai"}
	    }
	  }
	}`
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ps := s.ProviderSettings()

	doubao := ps.ConfigFor(provider.ProviderDoubao)
	if got := doubao.EffectiveImageModels(); !equalStrings(got, []string{"legacy-image-m"}) {
		t.Errorf("doubao image list = %v, want [legacy-image-m]", got)
	}
	if got := doubao.EffectiveTextModels(); !equalStrings(got, []string{"legacy-text-m"}) {
		t.Errorf("doubao text list = %v, want [legacy-text-m]", got)
	}
	if got := doubao.EffectiveVideoModels(); got != nil {
		t.Errorf("doubao video list = %v, want nil", got)
	}

	// No legacy fields at all → the built-in default classification holds.
	openai := ps.ConfigFor(provider.ProviderOpenAI)
	if got := openai.EffectiveImageModels(); !equalStrings(got, []string{provider.DefaultOpenAIModel}) {
		t.Errorf("openai image list = %v, want [%s]", got, provider.DefaultOpenAIModel)
	}
	if openai.EffectiveVideoModels() != nil || openai.EffectiveTextModels() != nil {
		t.Errorf("openai video/text lists = %v/%v, want nil/nil", openai.EffectiveVideoModels(), openai.EffectiveTextModels())
	}

	after, _ := os.ReadFile(path)
	if string(after) != legacy {
		t.Fatal("loading a legacy settings file rewrote it before any explicit save")
	}
}

// TestSaveAndReloadCategorizedModels covers the array save/reload loop: arrays
// persist byte-identically (no data loss at rest) and come back through the
// normalized Effective* helpers (去空白、去重).
func TestSaveAndReloadCategorizedModels(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ps := s.ProviderSettings()
	cfg := ps.Providers[provider.ProviderDoubao]
	cfg.APIKey = "ark-secret-key"
	cfg.ImageModels = []string{" seed-a ", "seed-b", "seed-a", ""}
	cfg.VideoModels = []string{" vid-1 ", "vid-1"}
	cfg.TextModels = []string{" t1 ", "t1", "", " t2 "}
	ps.Providers[provider.ProviderDoubao] = cfg
	if err := s.SaveProviderSettings(ps); err != nil {
		t.Fatal(err)
	}

	reloaded, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.ProviderSettings().ConfigFor(provider.ProviderDoubao)

	// Raw persisted values survive the round trip untouched (normalize-on-read).
	if !equalStrings(got.ImageModels, cfg.ImageModels) {
		t.Errorf("raw image array after reload = %q, want %q", got.ImageModels, cfg.ImageModels)
	}
	if !equalStrings(got.VideoModels, cfg.VideoModels) {
		t.Errorf("raw video array after reload = %q, want %q", got.VideoModels, cfg.VideoModels)
	}
	if !equalStrings(got.TextModels, cfg.TextModels) {
		t.Errorf("raw text array after reload = %q, want %q", got.TextModels, cfg.TextModels)
	}
	if got.APIKey != "ark-secret-key" {
		t.Errorf("api key lost on reload: %q", got.APIKey)
	}

	if got := got.EffectiveImageModels(); !equalStrings(got, []string{"seed-a", "seed-b"}) {
		t.Errorf("effective image list = %v, want [seed-a seed-b]", got)
	}
	if got := got.EffectiveVideoModels(); !equalStrings(got, []string{"vid-1"}) {
		t.Errorf("effective video list = %v, want [vid-1]", got)
	}
	if got := got.EffectiveTextModels(); !equalStrings(got, []string{"t1", "t2"}) {
		t.Errorf("effective text list = %v, want [t1 t2]", got)
	}
}

// equalStrings reports whether two string slices are identical element-wise.
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

// --- read-time normalization & migration / restart recovery
// (align-framebaker-providers task 1.2) ---

// writeRawSettings writes content verbatim as the settings file inside dir and
// returns the file path.
func writeRawSettings(t *testing.T, dir, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestNewNormalizesEmptyProviderDataTable covers the "empty provider data"
// boundary after the 人工验收 update: every variant loads successfully with an
// EMPTY provider set (no built-ins re-seeded) and an empty active slot — and
// none of these pure reads rewrites the original file.
func TestNewNormalizesEmptyProviderDataTable(t *testing.T) {
	cases := []struct{ name, body string }{
		{"empty json object", `{}`},
		{"null provider section", `{"provider": null}`},
		{"missing providers key", `{"provider":{"activeProvider":"doubao"}}`},
		{"null providers map", `{"provider":{"activeProvider":"doubao","providers":null}}`},
		{"empty providers map", `{"provider":{"activeProvider":"doubao","providers":{}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "cfg")
			path := writeRawSettings(t, dir, tc.body)
			before, _ := os.ReadFile(path)

			s, err := New(dir)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			ps := s.ProviderSettings()
			if ps.ActiveProvider != "" {
				t.Errorf("active = %q, want empty (nothing to activate)", ps.ActiveProvider)
			}
			if len(ps.Providers) != 0 {
				t.Fatalf("providers = %d, want 0 (no silent re-seed)", len(ps.Providers))
			}
			doubao := ps.ConfigFor(provider.ProviderDoubao)
			if !equalStrings(doubao.EffectiveImageModels(), []string{provider.DefaultDoubaoModel}) ||
				!equalStrings(doubao.EffectiveVideoModels(), []string{provider.DefaultDoubaoVideoModel}) ||
				!equalStrings(doubao.EffectiveTextModels(), []string{provider.DefaultDoubaoTextModel}) {
				t.Errorf("doubao catalogs = %v/%v/%v",
					doubao.EffectiveImageModels(), doubao.EffectiveVideoModels(), doubao.EffectiveTextModels())
			}

			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Error("a pure load rewrote the settings file")
			}
		})
	}
}

// TestNewRecoversActiveProviderTable verifies the stored active slot survives
// restarts: hanging references fall back deterministically to a configured
// provider, without touching the file on disk.
func TestNewRecoversActiveProviderTable(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantActive string
	}{
		{
			name:       "active points at deleted provider",
			wantActive: provider.ProviderDoubao,
			body: `{"provider":{"activeProvider":"removed-one","providers":{
				"doubao":{"providerId":"doubao","type":"doubao"},
				"openai":{"providerId":"openai","type":"openai"}}}}`,
		},
		{
			name:       "no active at all → first builtin in order",
			wantActive: provider.ProviderOpenAI,
			body: `{"provider":{"providers":{
				"agnes":{"providerId":"agnes","type":"agnes"},
				"openai":{"providerId":"openai","type":"openai"},
				"zeta-llm":{"providerId":"zeta-llm","type":"compatible","name":"Zeta"}}}}`,
		},
		{
			name:       "only custom providers → lexicographic first",
			wantActive: "banana-direct",
			body: `{"provider":{"activeProvider":"gone","providers":{
				"banana-direct":{"providerId":"banana-direct","type":"gemini","name":"Banana"},
				"zeta-llm":{"providerId":"zeta-llm","type":"api","name":"Zeta"}}}}`,
		},
		{
			name:       "valid custom active preserved",
			wantActive: "banana-direct",
			body: `{"provider":{"activeProvider":"banana-direct","providers":{
				"doubao":{"providerId":"doubao","type":"doubao"},
				"banana-direct":{"providerId":"banana-direct","type":"gemini","name":"Banana","model":"gemini-img-x"}}}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "cfg")
			path := writeRawSettings(t, dir, tc.body)
			before, _ := os.ReadFile(path)

			s, err := New(dir)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			ps := s.ProviderSettings()
			if ps.ActiveProvider != tc.wantActive {
				t.Fatalf("active = %q, want %q", ps.ActiveProvider, tc.wantActive)
			}
			if _, ok := ps.Providers[ps.ActiveProvider]; !ok {
				t.Fatalf("recovered active %q has no config entry", ps.ActiveProvider)
			}

			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Error("a pure load rewrote the settings file")
			}
		})
	}
}

// TestLegacyFileRestartMigrationLoop is the end-to-end restart-recovery test:
// an old-shape settings file (single-model fields, no model arrays) plus mixed
// custom providers loads intact; nothing is written until an explicit save,
// which persists the new per-capability catalog fields so the next process
// start restores them instead of degrading back.
func TestLegacyFileRestartMigrationLoop(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	legacy := `{
	  "provider": {
	    "activeProvider": "banana-direct",
	    "providers": {
	      "doubao": {"providerId":"doubao","type":"doubao","apiKey":"k-d","model":"legacy-img","textModel":"legacy-txt"},
	      "openai": {"providerId":"openai","type":"openai"},
	      "banana-direct": {"providerId":"banana-direct","type":"gemini","name":"Banana Direct","baseUrl":"https://g.example.com/v1","apiKey":"g-k","model":"gemini-img-pro"},
	      "local-runner": {"providerId":"local-runner","type":"compatible","name":"Local Runner","baseUrl":"http://127.0.0.1:8000/v1"}
	    }
	  }
	}`
	path := writeRawSettings(t, dir, legacy)

	s0, err := New(dir)
	if err != nil {
		t.Fatalf("load legacy: %v", err)
	}

	// Byte-stable read: loading old settings never rewrites them.
	raw, _ := os.ReadFile(path)
	if string(raw) != legacy {
		t.Fatal("loading the legacy settings file rewrote it before any explicit save")
	}

	ps0 := s0.ProviderSettings()
	if ps0.ActiveProvider != "banana-direct" {
		t.Fatalf("active = %q, want banana-direct", ps0.ActiveProvider)
	}
	banana := ps0.ConfigFor("banana-direct")
	if banana.Type != provider.ProviderTypeGemini || banana.Name != "Banana Direct" {
		t.Errorf("custom type/name lost: %+v", banana)
	}
	if got := banana.EffectiveImageModels(); !equalStrings(got, []string{"gemini-img-pro"}) {
		t.Errorf("banana image models = %v, want [gemini-img-pro]", got)
	}
	if runner := ps0.ConfigFor("local-runner"); runner.EffectiveType() != provider.ProviderTypeCompatible {
		t.Errorf("local-runner type = %q", runner.EffectiveType())
	}
	doubao0 := ps0.ConfigFor(provider.ProviderDoubao)
	if doubao0.APIKey != "k-d" {
		t.Errorf("doubao api key = %q", doubao0.APIKey)
	}
	if got := doubao0.EffectiveImageModels(); !equalStrings(got, []string{"legacy-img"}) {
		t.Errorf("legacy image fallback = %v", got)
	}
	if got := doubao0.EffectiveTextModels(); !equalStrings(got, []string{"legacy-txt"}) {
		t.Errorf("legacy text fallback = %v", got)
	}
	// The bare openai entry keeps its built-in default classification — 缺失数组不丢配置。
	if openai := ps0.ConfigFor(provider.ProviderOpenAI); !equalStrings(openai.EffectiveImageModels(), []string{provider.DefaultOpenAIModel}) {
		t.Errorf("openai default catalog = %v", openai.EffectiveImageModels())
	}

	// Explicit save: user edits carry the new per-capability catalogs.
	psEdit := s0.ProviderSettings()
	edit := psEdit.Providers[provider.ProviderDoubao]
	edit.ImageModels = []string{"img-new"}
	edit.VideoModels = []string{"vid-new"}
	edit.TextModels = []string{"txt-new"}
	psEdit.Providers[provider.ProviderDoubao] = edit
	if err := s0.SaveProviderSettings(psEdit); err != nil {
		t.Fatal(err)
	}

	saved, _ := os.ReadFile(path)
	var persisted Data
	if err := json.Unmarshal(saved, &persisted); err != nil {
		t.Fatalf("saved file unparseable: %v", err)
	}
	savedDoubao := persisted.Provider.Providers[provider.ProviderDoubao]
	if !equalStrings(savedDoubao.ImageModels, []string{"img-new"}) ||
		!equalStrings(savedDoubao.VideoModels, []string{"vid-new"}) ||
		!equalStrings(savedDoubao.TextModels, []string{"txt-new"}) {
		t.Errorf("save did not persist the new array fields: %+v", savedDoubao)
	}

	// Restart: everything — new fields included — comes back untouched.
	s1, err := New(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	ps1 := s1.ProviderSettings()
	if ps1.ActiveProvider != "banana-direct" {
		t.Fatalf("active after restart = %q", ps1.ActiveProvider)
	}
	reloaded := ps1.ConfigFor(provider.ProviderDoubao)
	if reloaded.APIKey != "k-d" ||
		!equalStrings(reloaded.ImageModels, []string{"img-new"}) ||
		!equalStrings(reloaded.VideoModels, []string{"vid-new"}) ||
		!equalStrings(reloaded.TextModels, []string{"txt-new"}) {
		t.Fatalf("restarted config lost data: %+v", reloaded)
	}
	if got := reloaded.EffectiveImageModels(); !equalStrings(got, []string{"img-new"}) {
		t.Errorf("effective image list after restart = %v, want [img-new]", got)
	}
	if banana1 := ps1.ConfigFor("banana-direct"); banana1.Type != provider.ProviderTypeGemini || banana1.APIKey != "g-k" {
		t.Errorf("custom provider drifted across restart: %+v", banana1)
	}
	if runner1 := ps1.ConfigFor("local-runner"); runner1.EffectiveType() != provider.ProviderTypeCompatible {
		t.Errorf("compatible custom drifted across restart: %+v", runner1)
	}
}

// TestEnvKeyFallbackAfterNormalization pins that the environment-variable key
// fallback keeps working for entries loaded from disk: the stored apiKey stays
// empty (no rewrite), resolution falls back to OFRAME_*_API_KEY and offline
// validation passes against the built-in defaults.
func TestEnvKeyFallbackAfterNormalization(t *testing.T) {
	t.Setenv(provider.EnvKeyAgnes, "env-agnes-key")
	dir := filepath.Join(t.TempDir(), "cfg")
	path := writeRawSettings(t, dir, `{"provider":{"activeProvider":"agnes","providers":{
		"agnes":{"providerId":"agnes","type":"agnes"}}}}`)
	before, _ := os.ReadFile(path)

	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	agnes := s.ProviderSettings().ConfigFor(provider.ProviderAgnes)
	if agnes.APIKey != "" {
		t.Errorf("stored apiKey unexpectedly set: %q", agnes.APIKey)
	}
	key, err := agnes.ResolveAPIKey()
	if err != nil || key != "env-agnes-key" {
		t.Fatalf("env fallback = %q, %v; want env-agnes-key, nil", key, err)
	}
	if err := agnes.Validate(); err != nil {
		t.Errorf("offline validation with env key failed: %v", err)
	}

	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Error("resolving via env fallback rewrote the settings file")
	}
}
