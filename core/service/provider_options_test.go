package service

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/oframe/character-workbench/core/provider"
)

func TestDraftProviderOperationsDoNotPersistOrActivate(t *testing.T) {
	var seenURL string
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		seenURL = r.URL.String()
		return jsonResp(http.StatusOK, map[string]any{"data": []map[string]string{{"id": "draft-model"}}}), nil
	}}
	svc, dir := newTestService(t, &http.Client{Transport: rt})
	before, err := os.ReadFile(dir + string(os.PathSeparator) + "settings.json")
	if err != nil {
		t.Fatal(err)
	}
	beforeSettings := svc.settings.ProviderSettings()
	beforeActive := beforeSettings.ActiveProvider
	cfg := provider.DefaultConfig(provider.ProviderDoubao)
	cfg.ProviderID = ""
	cfg.BaseURL = "https://draft.example/v1"
	cfg.APIKey = "draft-key"
	result := svc.TestProviderDraft(cfg)
	if !result.OK || !strings.HasSuffix(seenURL, "/models") {
		t.Fatalf("draft connection result=%+v url=%q", result, seenURL)
	}
	if !strings.HasPrefix(seenURL, cfg.BaseURL) {
		t.Fatalf("draft request URL=%q, want base %q", seenURL, cfg.BaseURL)
	}
	models, err := svc.ListProviderModelsDraft(cfg)
	if err != nil || len(models) != 1 || models[0] != "draft-model" {
		t.Fatalf("draft models=%v err=%v", models, err)
	}
	after, err := os.ReadFile(dir + string(os.PathSeparator) + "settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("draft operations changed settings.json")
	}
	afterSettings := svc.settings.ProviderSettings()
	if afterSettings.ActiveProvider != beforeActive || len(afterSettings.Providers) != len(beforeSettings.Providers) {
		t.Fatalf("draft operations changed settings: before=%+v after=%+v", beforeSettings, afterSettings)
	}
}

func TestListProviderModelsDraftCLIUnsupported(t *testing.T) {
	svc, _ := newTestService(t, &http.Client{Transport: &guardRT{}})
	_, err := svc.ListProviderModelsDraft(provider.ProviderConfig{Type: provider.ProviderTypeCLI})
	if !errors.Is(err, provider.ErrModelDiscoveryUnsupported) {
		t.Fatalf("CLI draft discovery err=%v, want ErrModelDiscoveryUnsupported", err)
	}
}

// TestProviderOptionsCapabilityMatrix lives in provider_draft_test.go with
// the extended matrix (custom protocol providers + reserved-video reasons).

func TestProviderOptionsRejectsUnknownCapability(t *testing.T) {
	svc, _ := newTestService(t, nil)
	if _, err := svc.ProviderOptions("audio"); err == nil {
		t.Fatal("unknown capability should fail")
	}
}
