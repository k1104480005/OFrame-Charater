package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oframe/character-workbench/core/provider"
)

// TestNewInitializesDefaults verifies a fresh store is created with the
// default provider settings (Doubao active).
func TestNewInitializesDefaults(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	ps := s.ProviderSettings()
	if ps.ActiveProvider != provider.DefaultProviderID {
		t.Fatalf("active = %q", ps.ActiveProvider)
	}
	if len(ps.Providers) != 3 {
		t.Fatalf("providers = %d", len(ps.Providers))
	}
	if _, err := os.Stat(s.Path()); err != nil {
		t.Fatalf("settings file missing: %v", err)
	}
}

// TestSaveAndReload verifies keys/models persist across reloads.
func TestSaveAndReload(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg")
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ps := s.ProviderSettings()
	cfg := ps.Providers[provider.ProviderDoubao]
	cfg.APIKey = "ark-secret-key"
	cfg.Model = "doubao-custom-model"
	ps.Providers[provider.ProviderDoubao] = cfg
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
