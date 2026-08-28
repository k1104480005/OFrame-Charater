package service

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/oframe/character-workbench/core/provider"
)

// TestProviderAddRemoveAndRestart covers the custom-provider lifecycle under
// the 人工验收 contract: a fresh store has NO provider cards, every provider
// (including re-added built-in identities) is removable, and providers survive
// a restart (rebuildRegistry reads the persisted settings).
func TestProviderAddRemoveAndRestart(t *testing.T) {
	dir := t.TempDir()
	svc, err := New(Options{SettingsDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	// Fresh install: zero provider cards.
	infos, err := svc.ProviderList()
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 0 {
		t.Fatalf("initial providers = %d, want 0 (no pre-seed)", len(infos))
	}

	// Add a custom provider without a key (allowed at add time).
	info, err := svc.ProviderAdd(provider.ProviderConfig{Name: "My Ark", Model: "m1", BaseURL: "https://x.example.com/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if provider.IsBuiltin(info.ID) || info.Type != provider.ProviderTypeCompatible || info.Builtin {
		t.Fatalf("added provider meta = %+v", info)
	}
	if !strings.HasPrefix(info.ID, "my-ark") {
		t.Fatalf("generated id = %q, want slug from name", info.ID)
	}
	if info.Name != "My Ark" {
		t.Fatalf("name = %q", info.Name)
	}
	if info.HasAPIKey {
		t.Fatal("no key was configured")
	}

	infos, _ = svc.ProviderList()
	if len(infos) != 1 {
		t.Fatalf("providers after add = %d, want 1", len(infos))
	}

	// Duplicate id refused.
	if _, err := svc.ProviderAdd(provider.ProviderConfig{ProviderID: info.ID, Name: "X", Model: "m", BaseURL: "https://x.example.com"}); err == nil {
		t.Fatal("expected duplicate add error")
	}
	// Non-slug custom id refused.
	if _, err := svc.ProviderAdd(provider.ProviderConfig{ProviderID: "Bad ID!", Name: "X", Model: "m", BaseURL: "https://x.example.com"}); err == nil {
		t.Fatal("expected invalid id error")
	}
	// Removing an UNCONFIGURED id is a readable error.
	if err := svc.ProviderRemove("never-added"); err == nil {
		t.Fatal("expected not-configured error")
	}

	// Removing the active custom provider leaves no active (nothing remains).
	if err := svc.SetActiveProvider(info.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ProviderRemove(info.ID); err != nil {
		t.Fatal(err)
	}
	if ps := svc.settings.ProviderSettings(); ps.ActiveProvider != "" || len(ps.Providers) != 0 {
		t.Fatalf("state after removing the only provider: active=%q providers=%d", ps.ActiveProvider, len(ps.Providers))
	}

	// Restart: a persisted custom provider is re-registered.
	info2, err := svc.ProviderAdd(provider.ProviderConfig{Name: "Second", Model: "m2", BaseURL: "https://y.example.com/v1", APIKey: "k2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	svc2, err := New(Options{SettingsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc2.Close() }()
	infos2, err := svc2.ProviderList()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range infos2 {
		if p.ID == info2.ID {
			found = true
			if p.Name != "Second" || !p.HasAPIKey || p.KeySource != "settings" {
				t.Fatalf("restored provider meta = %+v", p)
			}
		}
	}
	if !found {
		t.Fatalf("provider %s not re-registered after restart", info2.ID)
	}
}

// TestBuiltinIdentityRemovableAndReAddable pins the 人工验收 decision end to
// end: a seeded built-in can be REMOVED like any other provider, and the same
// id can be added again (restoring its own adapter, not the generic one).
func TestBuiltinIdentityRemovableAndReAddable(t *testing.T) {
	svc, err := New(Options{SettingsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if err := seedBuiltinProviders(svc); err != nil {
		t.Fatal(err)
	}

	// Remove the active doubao → active falls back to a remaining built-in.
	if err := svc.ProviderRemove(provider.ProviderDoubao); err != nil {
		t.Fatalf("built-in removal must be allowed: %v", err)
	}
	if ps := svc.settings.ProviderSettings(); ps.ActiveProvider != provider.ProviderOpenAI {
		t.Fatalf("active after doubao removal = %q, want openai (first remaining)", ps.ActiveProvider)
	}

	// Re-add the doubao identity: empty type restores its OWN adapter.
	info, err := svc.ProviderAdd(provider.ProviderConfig{ProviderID: provider.ProviderDoubao, Name: "豆包", Model: "m1", BaseURL: "https://x.example.com"})
	if err != nil {
		t.Fatalf("re-adding doubao must be allowed: %v", err)
	}
	if info.Type != provider.ProviderDoubao {
		t.Fatalf("re-added type = %q, want doubao", info.Type)
	}
	if _, err := svc.registry.Get(provider.ProviderDoubao); err != nil {
		t.Fatal(err)
	}

	// Removing ALL providers empties the store with no dangling active.
	for _, id := range []string{provider.ProviderDoubao, provider.ProviderOpenAI, provider.ProviderAgnes} {
		if err := svc.ProviderRemove(id); err != nil {
			t.Fatal(err)
		}
	}
	ps := svc.settings.ProviderSettings()
	if len(ps.Providers) != 0 || ps.ActiveProvider != "" {
		t.Fatalf("state after removing everything: providers=%d active=%q", len(ps.Providers), ps.ActiveProvider)
	}
}

// TestPrepareGenerationWithoutProvidersFailsReadably: a fresh store with no
// provider cards reports an actionable error instead of "unknown provider".
func TestPrepareGenerationWithoutProvidersFailsReadably(t *testing.T) {
	svc, _ := newTestService(t, &http.Client{Transport: &guardRT{}})
	// Strip the seeded trio to simulate the fresh-install state.
	ps := svc.settings.ProviderSettings()
	ps.Providers = map[string]provider.ProviderConfig{}
	ps.ActiveProvider = ""
	if err := svc.settings.SaveProviderSettings(ps); err != nil {
		t.Fatal(err)
	}
	if err := svc.rebuildRegistry(); err != nil {
		t.Fatal(err)
	}
	root := newTestPackage(t)
	_, err := svc.PrepareGeneration(context.Background(), GenerationRequest{PackagePath: root})
	if err == nil || !strings.Contains(err.Error(), "尚未配置任何 Provider") {
		t.Fatalf("err = %v, want the readable no-provider error", err)
	}
	assertNoPlansStored(t, svc)
}
