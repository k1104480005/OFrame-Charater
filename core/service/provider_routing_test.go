package service

import (
	"testing"

	"github.com/oframe/character-workbench/core/provider"
)

// TestProviderAddProtocolTypesRouteExplicitly (task 2.7): a custom provider
// added with an explicit FrameBaker protocol type keeps that type across
// save + restart, and the registry constructs exactly the matching adapter —
// never a silent fallback to the generic compatible surface.
func TestProviderAddProtocolTypesRouteExplicitly(t *testing.T) {
	dir := t.TempDir()
	svc, err := New(Options{SettingsDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		typ  string
		name string
	}{
		{provider.ProviderTypeDashscope, "百炼卡"},
		{provider.ProviderTypeGemini, "banana 卡"},
		{provider.ProviderTypeMiniMax, "MiniMax 卡"},
		{provider.ProviderTypeVolcengine, "Ark 卡"},
		{provider.ProviderTypeAPI, "自定义 API 卡"},
	}
	added := map[string]string{}
	for _, tc := range cases {
		info, err := svc.ProviderAdd(provider.ProviderConfig{
			Name: tc.name, Type: tc.typ, Model: "m1",
			BaseURL: "https://" + tc.typ + ".example.com/v1",
		})
		if err != nil {
			t.Fatalf("add %s: %v", tc.typ, err)
		}
		if info.Type != tc.typ {
			t.Fatalf("provider %s: type = %q, want %q", info.ID, info.Type, tc.typ)
		}
		added[info.ID] = tc.typ
	}

	// Restart: persisted protocol types are rebuilt into the matching adapters.
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	svc2, err := New(Options{SettingsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc2.Close() }()

	for id, wantType := range added {
		p, err := svc2.registry.Get(id)
		if err != nil {
			t.Fatalf("restart lost %s: %v", id, err)
		}
		var gotType string
		switch p.(type) {
		case *provider.Dashscope:
			gotType = provider.ProviderTypeDashscope
		case *provider.Gemini:
			gotType = provider.ProviderTypeGemini
		case *provider.MiniMax:
			gotType = provider.ProviderTypeMiniMax
		case *provider.Volcengine:
			gotType = provider.ProviderTypeVolcengine
		case *provider.Compatible:
			gotType = provider.ProviderTypeCompatible
		default:
			t.Fatalf("provider %s: unexpected adapter %T", id, p)
		}
		// The 自定义 API preset is the OpenAI-compatible protocol by
		// definition; every other protocol must have its own adapter.
		if gotType != wantType && !(wantType == provider.ProviderTypeAPI && gotType == provider.ProviderTypeCompatible) {
			t.Fatalf("provider %s (%s): adapter is %s — silent protocol fallback", id, wantType, gotType)
		}
	}

	infos, err := svc2.ProviderList()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, info := range infos {
		seen[info.ID] = info.Type
	}
	for id, wantType := range added {
		if got := seen[id]; got != wantType {
			t.Fatalf("ProviderList %s: type = %q, want %q", id, got, wantType)
		}
	}
}

// TestProviderAddCLITypeRoutesAdapter (updated by task 3.2): the CLI preset's
// adapter exists now, so a CLI provider is added under its own type, rebuilt
// as *provider.CLI after a restart, and never falls back to another protocol.
func TestProviderAddCLITypeRoutesAdapter(t *testing.T) {
	dir := t.TempDir()
	svc, err := New(Options{SettingsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	info, err := svc.ProviderAdd(provider.ProviderConfig{
		Name: "CLI 卡", Type: provider.ProviderTypeCLI, Model: "m1",
		CLICommand: `C:\tools\my tool.exe`, CLIPromptArg: "--prompt",
		CLIOutputArg: "--output", CLIModelArg: "--model", CLIRefImageArg: "--ref",
	})
	if err != nil {
		t.Fatalf("add CLI provider: %v", err)
	}
	if info.Type != provider.ProviderTypeCLI {
		t.Fatalf("type = %q, want cli", info.Type)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	svc2, err := New(Options{SettingsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc2.Close() }()
	p, err := svc2.registry.Get(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.(*provider.CLI); !ok {
		t.Fatalf("adapter after restart = %T, want *provider.CLI", p)
	}
	infos, err := svc2.ProviderList()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range infos {
		if item.ID == info.ID && item.Type != provider.ProviderTypeCLI {
			t.Fatalf("ProviderList type = %q, want cli", item.Type)
		}
	}
}

// TestProviderAddUnknownTypeRefused: an unsupported type must never fall back
// to compatible.
func TestProviderAddUnknownTypeRefused(t *testing.T) {
	svc, err := New(Options{SettingsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc.Close() }()
	if _, err := svc.ProviderAdd(provider.ProviderConfig{Name: "X", Type: "mystery", Model: "m", BaseURL: "https://x.example.com"}); err == nil {
		t.Fatal("expected unsupported-type refusal")
	}
}
