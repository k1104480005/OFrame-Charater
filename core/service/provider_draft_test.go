package service

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oframe/character-workbench/core/provider"
	"github.com/oframe/character-workbench/core/settings"
)

// Draft RPC + capability-filter tests (align-framebaker-providers tasks
// 4.2/4.3/4.4/4.5): unsaved form values can be tested and discovered WITHOUT
// persisting anything or touching the active provider, and provider choices
// are filtered strictly by requested capability.

type recordingRT struct {
	requests []string
	auth     []string
	handler  func(r *http.Request) (*http.Response, error)
}

func (rt *recordingRT) RoundTrip(r *http.Request) (*http.Response, error) {
	rt.requests = append(rt.requests, r.URL.String())
	rt.auth = append(rt.auth, r.Header.Get("Authorization"))
	return rt.handler(r)
}

func draftTestService(t *testing.T) (*Service, *recordingRT, string) {
	t.Helper()
	rt := &recordingRT{handler: func(r *http.Request) (*http.Response, error) {
		return jsonResp(200, map[string]any{"data": []map[string]any{{"id": "draft-model"}}}), nil
	}}
	svc, dir := newTestService(t, &http.Client{Transport: rt})
	return svc, rt, dir
}

func draftConfig() provider.ProviderConfig {
	return provider.ProviderConfig{
		ProviderID: "draft-provider",
		Type:       provider.ProviderTypeCompatible,
		Name:       "Draft Provider",
		APIKey:     "sk-draft",
		Model:      "draft-model",
		BaseURL:    "http://draft.example.com/v1",
	}
}

func settingsBytes(t *testing.T, dir string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, settings.FileName))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestProviderDraftTestUsesUnsavedValues (task 4.2): the draft's own Base URL
// and key are used, and nothing is persisted or activated.
func TestProviderDraftTestUsesUnsavedValues(t *testing.T) {
	svc, rt, dir := draftTestService(t)
	before := settingsBytes(t, dir)
	activeBefore := svc.settings.ProviderSettings().ActiveProvider

	res := svc.TestProviderDraft(draftConfig())
	if !res.OK || len(res.Models) != 1 || res.Models[0] != "draft-model" {
		t.Fatalf("result = %+v", res)
	}
	if len(rt.requests) != 1 || !strings.HasPrefix(rt.requests[0], "http://draft.example.com/v1/models") {
		t.Fatalf("requests = %v", rt.requests)
	}
	if rt.auth[0] != "Bearer sk-draft" {
		t.Fatalf("auth = %q", rt.auth[0])
	}
	if string(settingsBytes(t, dir)) != string(before) {
		t.Fatal("draft connection test must not modify settings")
	}
	if after := svc.settings.ProviderSettings().ActiveProvider; after != activeBefore {
		t.Fatalf("active provider drifted: %q → %q", activeBefore, after)
	}
	// The draft was NOT registered.
	if _, err := svc.registry.Get("draft-provider"); err == nil {
		t.Fatal("draft provider must not be registered")
	}
}

// TestProviderDraftModelsList (task 4.3): discovery runs on draft values with
// zero persistence; a CLI draft fails with the discovery-unsupported sentinel
// and never touches the transport.
func TestProviderDraftModelsList(t *testing.T) {
	svc, rt, dir := draftTestService(t)
	before := settingsBytes(t, dir)

	models, err := svc.ListProviderModelsDraft(draftConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0] != "draft-model" {
		t.Fatalf("models = %v", models)
	}
	if len(rt.requests) != 1 || !strings.HasSuffix(rt.requests[0], "/models") {
		t.Fatalf("requests = %v", rt.requests)
	}

	cliDraft := provider.ProviderConfig{ProviderID: "cli-draft", Type: provider.ProviderTypeCLI, Name: "CLI", Model: "m"}
	if _, err := svc.ListProviderModelsDraft(cliDraft); !errors.Is(err, provider.ErrModelDiscoveryUnsupported) {
		t.Fatalf("cli draft err = %v, want ErrModelDiscoveryUnsupported", err)
	}
	if len(rt.requests) != 1 {
		t.Fatalf("CLI discovery must not hit the transport, requests = %v", rt.requests)
	}
	if string(settingsBytes(t, dir)) != string(before) {
		t.Fatal("draft model discovery must not modify settings")
	}
}

// TestProviderOptionsCapabilityMatrix (task 4.4): the selection matrix rejects
// every incompatible provider/model combination offline, and reserved video
// catalogs produce an explicit reason instead of a silent empty list.
func TestProviderOptionsCapabilityMatrix(t *testing.T) {
	svc := newGuardService(t)

	custom, err := svc.ProviderAdd(provider.ProviderConfig{
		Name: "Custom API", Type: provider.ProviderTypeAPI, Model: "img-m1",
		TextModels: []string{"chat-m1"}, BaseURL: "https://custom.example.com/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	cli, err := svc.ProviderAdd(provider.ProviderConfig{
		Name: "CLI 卡", Type: provider.ProviderTypeCLI, Model: "cli-m1",
		CLICommand: "C:/tools/tool.exe", CLIOutputArg: "--output",
	})
	if err != nil {
		t.Fatal(err)
	}

	imageIDs := map[string]bool{}
	textIDs := map[string]bool{}
	image, err := svc.ProviderOptions("image")
	if err != nil {
		t.Fatal(err)
	}
	for _, opt := range image {
		if opt.Reason != "" || len(opt.Models) == 0 {
			t.Fatalf("image option %s: reason=%q models=%v", opt.ID, opt.Reason, opt.Models)
		}
		imageIDs[opt.ID] = true
	}
	for _, want := range []string{provider.ProviderDoubao, provider.ProviderOpenAI, provider.ProviderAgnes, custom.ID, cli.ID} {
		if !imageIDs[want] {
			t.Fatalf("image options missing %s: %v", want, imageIDs)
		}
	}

	text, err := svc.ProviderOptions("text")
	if err != nil {
		t.Fatal(err)
	}
	for _, opt := range text {
		if opt.Reason == "" {
			textIDs[opt.ID] = true
			if len(opt.Models) == 0 {
				t.Fatalf("text option %s usable but has no models", opt.ID)
			}
		} else if len(opt.Models) != 0 {
			t.Fatalf("rejected text option %s must not expose models", opt.ID)
		}
	}
	if !textIDs[provider.ProviderDoubao] || !textIDs[custom.ID] || !textIDs[provider.ProviderAgnes] {
		t.Fatalf("text options must include doubao + custom + agnes(multimodal), got %v", textIDs)
	}
	for _, banned := range []string{provider.ProviderOpenAI, cli.ID} {
		if textIDs[banned] {
			t.Fatalf("text options must exclude %s", banned)
		}
	}

	video, err := svc.ProviderOptions("video")
	if err != nil {
		t.Fatal(err)
	}
	if len(video) == 0 {
		t.Fatal("video options must list every provider with a reason")
	}
	doubaoVideoReason := ""
	for _, opt := range video {
		if opt.Models != nil {
			t.Fatalf("video option %s must not expose executable models", opt.ID)
		}
		if opt.Reason == "" {
			t.Fatalf("video option %s must carry a readable reason", opt.ID)
		}
		if opt.ID == provider.ProviderDoubao {
			doubaoVideoReason = opt.Reason
		}
	}
	if !strings.Contains(doubaoVideoReason, "预留") {
		t.Fatalf("doubao video reason should mention the reserved catalog, got %q", doubaoVideoReason)
	}

	// Unknown capability is a readable error, never a silent empty list.
	if _, err := svc.ProviderOptions("smell"); err == nil {
		t.Fatal("expected unknown-capability error")
	}
}

// TestPlanSnapshotCarriesProviderModelCapability (task 4.5): the confirmation
// plan fixes the selected provider, model and capability before any external
// call; the request stays rejectable when the model leaves the catalog.
func TestPlanSnapshotCarriesProviderModelCapability(t *testing.T) {
	svc := newGuardService(t)
	root := newTestPackage(t)

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{PackagePath: root})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Capability != string(provider.ModalityImage) {
		t.Fatalf("plan capability = %q, want image", plan.Capability)
	}
	if plan.ProviderType != "doubao" {
		t.Fatalf("plan providerType = %q, want doubao (task 6.3)", plan.ProviderType)
	}
	if plan.ProviderID != provider.DefaultProviderID {
		t.Fatalf("plan provider = %q, want %q", plan.ProviderID, provider.DefaultProviderID)
	}
	wantModel := provider.DefaultConfig(provider.ProviderDoubao).Model
	if plan.Model != wantModel {
		t.Fatalf("plan model = %q, want %q", plan.Model, wantModel)
	}

	// An explicit selection is carried through verbatim when it is valid.
	customPlan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root,
		ProviderID:  provider.ProviderOpenAI,
		Model:       provider.DefaultOpenAIModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	if customPlan.ProviderID != provider.ProviderOpenAI || customPlan.Model != provider.DefaultOpenAIModel || customPlan.Capability != "image" {
		t.Fatalf("explicit selection drifted: %+v", customPlan)
	}

	// And a selection that leaves the catalog is refused offline (drift guard).
	_, err = svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root,
		ProviderID:  provider.ProviderOpenAI,
		Model:       provider.DefaultDoubaoModel,
	})
	if !errors.Is(err, provider.ErrModelInvalid) {
		t.Fatalf("cross-provider model = %v, want ErrModelInvalid", err)
	}
	// Exactly the two valid preparations are stored — the rejected request
	// left nothing behind.
	svc.plans.mu.Lock()
	n := len(svc.plans.plans)
	svc.plans.mu.Unlock()
	if n != 2 {
		t.Fatalf("plans stored = %d, want 2 (valid preparations only)", n)
	}
}

// TestEnhanceSettingsAssociation (task 5.5): the association persists, a text-
// incapable provider is refused, a foreign model is refused, and a dangling
// association is cleared on the next load.
func TestEnhanceSettingsAssociation(t *testing.T) {
	svc, _, dir := draftTestService(t)

	// Default: association empty (follow the active provider).
	if got := svc.EnhanceSettingsGet(); got.ProviderID != "" || got.Model != "" {
		t.Fatalf("initial association = %+v", got)
	}

	// Doubao carries a text catalog → association accepted and persisted.
	if err := svc.EnhanceSettingsSet(provider.ProviderDoubao, provider.DefaultConfig(provider.ProviderDoubao).TextModel); err != nil {
		t.Fatal(err)
	}
	if got := svc.EnhanceSettingsGet(); got.ProviderID != provider.ProviderDoubao {
		t.Fatalf("association = %+v", got)
	}

	// OpenAI (image-only) is refused.
	if err := svc.EnhanceSettingsSet(provider.ProviderOpenAI, ""); err == nil {
		t.Fatal("expected text-capability refusal for openai")
	}
	// A foreign model on a text-capable provider is refused.
	if err := svc.EnhanceSettingsSet(provider.ProviderDoubao, "ghost-chat"); !errors.Is(err, provider.ErrModelInvalid) {
		t.Fatalf("foreign enhance model = %v, want ErrModelInvalid", err)
	}
	// Unknown provider refused.
	if err := svc.EnhanceSettingsSet("no-such-provider", "m"); err == nil {
		t.Fatal("expected unknown-provider refusal")
	}

	// Reset follows the active provider again.
	if err := svc.EnhanceSettingsSet("", ""); err != nil {
		t.Fatal(err)
	}
	if got := svc.EnhanceSettingsGet(); got.ProviderID != "" {
		t.Fatalf("reset association = %+v", got)
	}

	// A persisted association to a provider that is later deleted is cleared
	// by normalization on the next service start (缺失 Provider 场景).
	custom, err := svc.ProviderAdd(provider.ProviderConfig{
		Name: "Enhancer", Type: provider.ProviderTypeAPI, Model: "img-m1",
		TextModels: []string{"chat-m1"}, BaseURL: "https://enh.example.com/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.EnhanceSettingsSet(custom.ID, "chat-m1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ProviderRemove(custom.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	svc2, err := New(Options{SettingsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc2.Close() })
	if got := svc2.EnhanceSettingsGet(); got.ProviderID != "" {
		t.Fatalf("dangling association survived restart: %+v", got)
	}
}

// TestVideoExtractionConfigRestartsAndStaysGated (task 6.2): the reserved
// video catalogs are readable and restart-safe, Supported stays false before
// any video pipeline exists, and the check is purely local (zero calls).
func TestVideoExtractionConfigRestartsAndStaysGated(t *testing.T) {
	svc, rt, dir := draftTestService(t)

	cfg, err := svc.VideoExtractionConfig(provider.ProviderDoubao)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Supported {
		t.Fatal("video extraction must stay unsupported before the pipeline exists")
	}
	if len(cfg.VideoModels) == 0 || cfg.VideoModels[0] != provider.DefaultConfig(provider.ProviderDoubao).VideoModels[0] {
		t.Fatalf("video catalog = %v", cfg.VideoModels)
	}
	if !strings.Contains(cfg.Reason, "预留") {
		t.Fatalf("reason = %q, want the reserved-catalog explanation", cfg.Reason)
	}
	if len(rt.requests) != 0 {
		t.Fatalf("video config read must not hit the transport, requests = %v", rt.requests)
	}

	// A provider without any video model still reads cleanly with a reason.
	cfgOpenAI, err := svc.VideoExtractionConfig(provider.ProviderOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	if cfgOpenAI.Supported || len(cfgOpenAI.VideoModels) != 0 || cfgOpenAI.Reason == "" {
		t.Fatalf("openai video config = %+v", cfgOpenAI)
	}

	// Configuration restore across restart: a saved video catalog comes back.
	ps := svc.settings.ProviderSettings()
	doubao := ps.Providers[provider.ProviderDoubao]
	doubao.VideoModels = []string{"seedance-custom-1"}
	ps.Providers[provider.ProviderDoubao] = doubao
	if err := svc.settings.SaveProviderSettings(ps); err != nil {
		t.Fatal(err)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	svc2, err := New(Options{SettingsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc2.Close() })
	cfg2, err := svc2.VideoExtractionConfig(provider.ProviderDoubao)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Supported || len(cfg2.VideoModels) != 1 || cfg2.VideoModels[0] != "seedance-custom-1" {
		t.Fatalf("video config after restart = %+v", cfg2)
	}

	// Unknown provider fails structurally.
	if _, err := svc2.VideoExtractionConfig("no-such-provider"); err == nil {
		t.Fatal("expected unknown-provider error")
	}
}
