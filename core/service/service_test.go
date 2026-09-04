package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/motion"
	"github.com/oframe/character-workbench/core/pipeline"
	"github.com/oframe/character-workbench/core/provider"
)

// fakeRT is an in-process transport; tests never call real paid services.
type fakeRT struct {
	handler func(r *http.Request) (*http.Response, error)
	calls   atomic.Int64
}

func (f *fakeRT) RoundTrip(r *http.Request) (*http.Response, error) {
	f.calls.Add(1)
	return f.handler(r)
}

func pngResp() *http.Response {
	return jsonResp(200, map[string]any{
		"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString([]byte("PNG"))}},
	})
}

// filmstripResp builds a VALID PNG filmstrip response for the wired filmstrip
// pipeline (闃舵 5: ConfirmGeneration 鐜板湪鎶?provider 鍘熷瀛楄妭瑙ｇ爜骞惰窇绠＄嚎, 鎵€浠?// fake provider 蹇呴』杩斿洖鐪熷疄鍙В鐮佺殑鑳剁墖鏉?: frameCount frames of w脳h with a
// magenta technical background and one opaque block per frame.
func filmstripResp(t *testing.T, w, h, frameCount int) *http.Response {
	t.Helper()
	canvas, err := identity.NewCanvasSpec(w, h)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := pipeline.NormalizeFrameList(*canvas, frameCount)
	if err != nil {
		t.Fatal(err)
	}
	frames := make([]*image.RGBA, frameCount)
	for i := range frames {
		frames[i] = blockFrame(w, h, 10+(i%3), 22)
	}
	strip, err := pipeline.AssembleFilmstrip(frames, layout)
	if err != nil {
		t.Fatal(err)
	}
	data, err := pipeline.EncodeFilmstripPNG(strip)
	if err != nil {
		t.Fatal(err)
	}
	return jsonResp(200, map[string]any{
		"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString(data)}},
	})
}

// blockFrame is a w脳h magenta frame (娲嬬孩浠呯敤浜庢姞鍥? with a 10脳10 opaque block
// at (bx, by).
func blockFrame(w, h, bx, by int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 255, A: 255})
		}
	}
	for y := by; y < by+10 && y < h; y++ {
		for x := bx; x < bx+10 && x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 120, G: 200, B: 80, A: 255})
		}
	}
	return img
}

func jsonResp(code int, v any) *http.Response {
	data, _ := json.Marshal(v)
	return &http.Response{
		StatusCode: code,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

func newTestService(t *testing.T, client *http.Client) (*Service, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cfg")
	svc, err := New(Options{
		SettingsDir: dir,
		HTTPClient:  client,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Most service tests exercise flows that assume the classic built-in trio
	// (doubao default primary + two fallbacks). Fresh installs start EMPTY
	// since the 浜哄伐楠屾敹 update, so tests seed the trio explicitly.
	if err := seedBuiltinProviders(svc); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc, dir
}

// seedBuiltinProviders writes the classic doubao/openai/agnes trio into the
// store and rebuilds the registry from it. Fresh installs start with NO
// provider cards (浜哄伐楠屾敹鏇存柊: 涓嶅浐瀹氶缃?3 涓唴缃?Provider) 鈥?tests that
// need a pre-configured doubao call this explicitly.
func seedBuiltinProviders(svc *Service) error {
	ps := svc.settings.ProviderSettings()
	ps.Providers = map[string]provider.ProviderConfig{
		provider.ProviderDoubao: provider.DefaultConfig(provider.ProviderDoubao),
		provider.ProviderOpenAI: provider.DefaultConfig(provider.ProviderOpenAI),
		provider.ProviderAgnes:  provider.DefaultConfig(provider.ProviderAgnes),
	}
	ps.ActiveProvider = provider.DefaultProviderID
	if err := svc.settings.SaveProviderSettings(ps); err != nil {
		return err
	}
	return svc.rebuildRegistry()
}

// newTestPackage creates a ready identity package: canvas set, one main
// reference image and one auxiliary reference image. NOTE: source file names
// must avoid Windows reserved device names (aux.png would open the AUX
// character device and block reads forever).
func newTestPackage(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := identity.Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetTextDescription("a small green pixel hero"); err != nil {
		t.Fatal(err)
	}
	if err := pkg.SetLogicalCanvas(32, 32); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(t.TempDir(), "main-ref.png")
	aux := filepath.Join(t.TempDir(), "extra-ref.png")
	if err := os.WriteFile(main, []byte("main-img"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(aux, []byte("aux-img"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := pkg.AddReferenceImage(main, "main ref", identity.RoleMainReference); err != nil {
		t.Fatal(err)
	}
	if _, err := pkg.AddReferenceImage(aux, "aux ref", identity.RoleAuxiliaryReference); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestPrepareGenerationIsExternalCallFree verifies PrepareGeneration performs
// no provider calls (any transport hit is a bug).
func TestPrepareGenerationIsExternalCallFree(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		t.Fatal("PrepareGeneration must not call any provider")
		return nil, nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	root := newTestPackage(t)

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 4, StylePresetID: "pixel", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rt.calls.Load() != 0 {
		t.Fatalf("transport calls = %d, want 0", rt.calls.Load())
	}
	// 鏂瑰悜鏁?4 鈫?3 鐢熸垚 + 1 闀滃儚; 姣忔柟鍚戞渶澶?3 娆℃€诲皾璇?
	if plan.Directions != 4 || plan.BasicDirections != 3 || plan.MirroredDirections != 1 {
		t.Fatalf("direction plan: %+v", plan)
	}
	if plan.ExpectedCalls != 3 || plan.MaxAttemptsPerDirection != 3 || plan.MaxTotalAttempts != 9 {
		t.Fatalf("budget: expected=%d maxAttempts=%d maxTotal=%d",
			plan.ExpectedCalls, plan.MaxAttemptsPerDirection, plan.MaxTotalAttempts)
	}
	// 榛樿璺敱 Doubao (棣栨鐢熸垚榛樿).
	if plan.ProviderID != provider.ProviderDoubao || plan.Model != provider.DefaultDoubaoModel {
		t.Fatalf("default provider: %s/%s", plan.ProviderID, plan.Model)
	}
	if plan.Capability != provider.ModalityImage.String() {
		t.Fatalf("generation capability = %q, want image", plan.Capability)
	}
	// 澶栧彂绱犳潗 = 涓?+ 杈呭姪鍙傝€冨浘.
	if len(plan.OutboundMaterials) != 2 {
		t.Fatalf("outbound materials = %d, want 2", len(plan.OutboundMaterials))
	}
	if plan.OutboundMaterials[0].Role != identity.RoleMainReference {
		t.Errorf("first outbound material not main: %+v", plan.OutboundMaterials[0])
	}
	if plan.Prompt.StylePresetID != "pixel" || plan.Prompt.ActionPresetID != "walk" {
		t.Errorf("prompt snapshot presets: %+v", plan.Prompt)
	}
	if plan.Prompt.FrameCount != 4 || plan.Prompt.CanvasWidth != 32 {
		t.Errorf("prompt snapshot canvas/frames: %+v", plan.Prompt)
	}
	if plan.Status != PlanPending {
		t.Errorf("plan status = %q", plan.Status)
	}
	if plan.Currency != "CNY" || plan.ExpectedCost <= 0 || plan.MaxCost <= plan.ExpectedCost {
		t.Errorf("cost: per=%v currency=%s expected=%v max=%v", plan.CostPerCall, plan.Currency, plan.ExpectedCost, plan.MaxCost)
	}
}

// TestPrepareGenerationEightDirections verifies the 8-direction strategy's
// automatic mirror 鍙ｅ緞 with the review report's explicit semantics: 5 鐢熸垚
// (right/up/down/up-right/down-left) + 3 闀滃儚 (left/up-left/down-right) 鈥?// down-right is derived ONE-WAY from down-left, so down-left joins the basic
// set; down (and up) are self-symmetric under horizontal mirroring.
func TestPrepareGenerationEightDirections(t *testing.T) {
	svc, _ := newTestService(t, nil)
	root := newTestPackage(t)

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 8, StylePresetID: "pixel", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 8 鏂瑰悜 鈫?5 鐢熸垚 + 3 闀滃儚; 姣忔柟鍚戞渶澶?3 娆℃€诲皾璇?
	if plan.Directions != 8 || plan.BasicDirections != 5 || plan.MirroredDirections != 3 {
		t.Fatalf("direction plan: %+v", plan)
	}
	if plan.ExpectedCalls != 5 || plan.MaxAttemptsPerDirection != 3 || plan.MaxTotalAttempts != 15 {
		t.Fatalf("budget: expected=%d maxAttempts=%d maxTotal=%d",
			plan.ExpectedCalls, plan.MaxAttemptsPerDirection, plan.MaxTotalAttempts)
	}
	// 鍙ｅ緞閿佸畾: 鐢熸垚鏂瑰悜涓庨暅鍍忔柟鍚戠殑鍏蜂綋鏍囩 (澶嶆牳鎶ュ憡璇箟).
	if got := plan.BasicLabels; !slices.Equal(got, []string{"right", "up", "down", "up-right", "down-left"}) {
		t.Fatalf("basicLabels(8) = %v", got)
	}
	if got := plan.MirroredLabels; !slices.Equal(got, []string{"left", "up-left", "down-right"}) {
		t.Fatalf("mirroredLabels(8) = %v", got)
	}
	// 姣忎釜闀滃儚鏂瑰悜閮借兘鍦ㄥ熀鏈泦鍚堜腑鎵惧埌鍏堕暅鍍忔簮 (down 鑷绉? 涓嶄骇鐢熸淳鐢?.
	for _, md := range plan.MirroredLabels {
		src := motion.MirrorSource(md)
		if !slices.Contains(plan.BasicLabels, src) {
			t.Errorf("mirrored %s has no source in basic set %v", md, plan.BasicLabels)
		}
	}
}

// TestConfirmGenerationCancelMakesNoCalls verifies the cancel branch (spec 4.5:
// 鍙栨秷鍒欎笉鍙戣捣浠讳綍璋冪敤).
func TestConfirmGenerationCancelMakesNoCalls(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		t.Fatal("cancelled confirmation must not call any provider")
		return nil, nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	root := newTestPackage(t)
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{PackagePath: root, Directions: 1})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Accepted || res.Status != PlanCancelled || res.CallsMade != 0 {
		t.Fatalf("cancel result: %+v", res)
	}
	if rt.calls.Load() != 0 {
		t.Fatalf("transport calls after cancel = %d, want 0", rt.calls.Load())
	}
	// The plan is final; a second decision is refused.
	if _, err := svc.ConfirmGeneration(context.Background(), plan.ID, true); err == nil {
		t.Fatal("expected error deciding a settled plan again")
	}
}

// TestConfirmGenerationAcceptsAndExecutes verifies the accept branch executes
// one provider call per generated direction, runs the filmstrip pipeline on
// the raw bytes (瑙ｇ爜 鈫?ProcessFilmstrip 鈫?鍊欓€夎惤鐩?鈫?CandidateSet 淇濈暀), and
// records call statistics.
func TestConfirmGenerationAcceptsAndExecutes(t *testing.T) {
	var sawBodies [][]byte
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		sawBodies = append(sawBodies, body)
		return filmstripResp(t, 32, 32, 4), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	root := newTestPackage(t)
	// Configure a key so execution can reach the (fake) transport.
	cfg, err := svc.ProviderConfig(provider.ProviderDoubao)
	if err != nil {
		t.Fatal(err)
	}
	cfg.APIKey = "test-key"
	if err := svc.SaveProviderConfig(provider.ProviderDoubao, cfg); err != nil {
		t.Fatal(err)
	}
	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 4, ActionPresetID: "idle",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Accepted || res.Status != PlanExecuted || res.CallsMade != 3 || res.Attempts != 3 {
		t.Fatalf("execute result: %+v", res)
	}
	if len(res.Results) != 3 {
		t.Fatalf("direction results = %d", len(res.Results))
	}
	// 鐢熸垚鎵ц閾惧凡鎺ュ叆 filmstrip 绠＄嚎: 姣忎釜鏂瑰悜浜у嚭鍊欓€夊苟钀界洏.
	for _, r := range res.Results {
		if r.CandidateID == "" {
			t.Fatalf("direction %s produced no candidate: %+v", r.Direction, r)
		}
		dir := filepath.Join(root, identity.DirCandidates, r.CandidateID)
		for _, f := range []string{"filmstrip.png", "prompt.json", "scores.json"} {
			if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
				t.Errorf("candidate artifact %s missing: %v", f, err)
			}
		}
	}
	// CandidateSet 淇濈暀: 鍊欓€夊彲琚悗缁搷浣滆鍒?
	if got := svc.CandidateList(root); len(got) != 3 {
		t.Fatalf("retained candidates = %d, want 3", len(got))
	}
	if len(sawBodies) != 3 {
		t.Fatalf("transport calls = %d, want 3 (right/up/down generated)", len(sawBodies))
	}
	// 澶栧彂绱犳潗杩涘叆璇锋眰浣?(reference_images).
	var body map[string]any
	if err := json.Unmarshal(sawBodies[0], &body); err != nil {
		t.Fatal(err)
	}
	if refs, _ := body["reference_images"].([]any); len(refs) != 2 {
		t.Fatalf("request reference_images = %v", body["reference_images"])
	}
	// 姣忔璋冪敤鍚庣粺璁℃洿鏂?
	stats := svc.ProviderStats()
	doubao := stats.ForProvider(provider.ProviderDoubao)
	if len(doubao) != 1 || doubao[0].CallCount != 3 || doubao[0].EstimatedCost < 0.14 {
		t.Fatalf("stats after execution: %+v", doubao)
	}
}

// TestConfirmGenerationFailsAtRetryCap verifies the hard cap: a provider that
// keeps failing is attempted exactly maxAttempts times per direction, then the
// plan fails with the recorded reason and no further calls are issued.
func TestConfirmGenerationFailsAtRetryCap(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		return jsonResp(500, map[string]any{"error": map[string]any{"message": "boom"}}), nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	root := newTestPackage(t)

	// Configure Doubao with maxAttempts=2 so the test stays fast.
	cfg, err := svc.ProviderConfig(provider.ProviderDoubao)
	if err != nil {
		t.Fatal(err)
	}
	cfg.APIKey = "test-key"
	cfg.MaxAttempts = 2
	if err := svc.SaveProviderConfig(provider.ProviderDoubao, cfg); err != nil {
		t.Fatal(err)
	}

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 4, MaxAttemptsPerDirection: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ConfirmGeneration(context.Background(), plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != PlanFailed || res.Error == "" {
		t.Fatalf("failure result: %+v", res)
	}
	// 2 attempts on the first generated direction, then stop.
	if rt.calls.Load() != 2 {
		t.Fatalf("transport calls = %d, want 2 (cap reached, no further calls)", rt.calls.Load())
	}
	got, _ := svc.GetPlan(plan.ID)
	if got.Status != PlanFailed {
		t.Fatalf("plan status = %q", got.Status)
	}
}

// TestProviderConfigLifecycle verifies Save/Validate/SetActive round-trips
// through the shared service, and that a fresh service instance (CLI process,
// GUI restart) loads the persisted settings.
func TestProviderConfigLifecycle(t *testing.T) {
	svc, dir := newTestService(t, nil)

	// Save OpenAI key + model, validate offline.
	cfg, err := svc.ProviderConfig(provider.ProviderOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	cfg.APIKey = "sk-secret"
	if err := svc.SaveProviderConfig(provider.ProviderOpenAI, cfg); err != nil {
		t.Fatal(err)
	}
	if err := svc.ValidateProvider(provider.ProviderOpenAI); err != nil {
		t.Fatalf("validate openai: %v", err)
	}

	// Runtime switch; the previous config is preserved.
	if err := svc.SetActiveProvider(provider.ProviderOpenAI); err != nil {
		t.Fatal(err)
	}
	list, err := svc.ProviderList()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ProviderInfo{}
	for _, p := range list {
		byID[p.ID] = p
	}
	if !byID[provider.ProviderOpenAI].Active {
		t.Fatal("openai should be active after switch")
	}
	if byID[provider.ProviderDoubao].Active {
		t.Fatal("doubao should not be active after switch")
	}
	if !byID[provider.ProviderOpenAI].HasAPIKey {
		t.Fatal("openai key should be reported as set")
	}
	// Doubao without key fails offline validation (no env in tests).
	if err := svc.ValidateProvider(provider.ProviderDoubao); err == nil {
		t.Fatal("expected validation error for doubao without key")
	}

	// A fresh service instance loads the persisted settings (GUI/CLI 鍏变韩).
	svc2, err := New(Options{
		SettingsDir: dir,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = svc2.Close() }()
	list2, err := svc2.ProviderList()
	if err != nil {
		t.Fatal(err)
	}
	byID2 := map[string]ProviderInfo{}
	for _, p := range list2 {
		byID2[p.ID] = p
	}
	if !byID2[provider.ProviderOpenAI].Active {
		t.Fatalf("persisted active provider lost")
	}
	if !byID2[provider.ProviderOpenAI].HasAPIKey {
		t.Fatalf("persisted openai key lost")
	}
}

// TestPresetCatalog verifies the shared service exposes the PerfectPixel
// presets (鍥涗釜椋庢牸棰勮 + 鍔ㄤ綔棰勮).
func TestPresetCatalog(t *testing.T) {
	svc, _ := newTestService(t, nil)
	cat := svc.PresetCatalog()
	if len(cat.Styles) != 4 {
		t.Fatalf("style presets = %d, want 4", len(cat.Styles))
	}
	if len(cat.Actions) < 4 {
		t.Fatalf("action presets = %d", len(cat.Actions))
	}
}

// TestPrepareGenerationSendsAdoptedBaseSprite verifies the perfectpixel
// alignment: the adopted base character sprite is attached as the FIRST
// outbound reference (canonical base sprite) and the prompt snapshot carries
// the Subject lock identity section (identity-consistency key).
func TestPrepareGenerationSendsAdoptedBaseSprite(t *testing.T) {
	rt := &fakeRT{handler: func(r *http.Request) (*http.Response, error) {
		t.Fatal("PrepareGeneration must not call any provider")
		return nil, nil
	}}
	svc, _ := newTestService(t, &http.Client{Transport: rt})
	root := newTestPackage(t)

	pkg, err := identity.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	rel := "candidates/base-sprite.png"
	abs := filepath.Join(root, "candidates", "base-sprite.png")
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	cand, err := pkg.AddBaseCharacterCandidate(rel, "prov", "model", "prompt")
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.AdoptBaseCharacter(cand.ID); err != nil {
		t.Fatal(err)
	}

	plan, err := svc.PrepareGeneration(context.Background(), GenerationRequest{
		PackagePath: root, Directions: 4, StylePresetID: "pixel", ActionPresetID: "walk",
	})
	if err != nil {
		t.Fatal(err)
	}
	// outbound materials = base sprite + main + aux references (sprite first).
	if len(plan.OutboundMaterials) != 3 {
		t.Fatalf("outbound materials = %d, want 3 (base sprite + main + aux)", len(plan.OutboundMaterials))
	}
	first := plan.OutboundMaterials[0]
	if first.Kind != "base_sprite" || first.Role != "base_sprite" || first.MaterialID != "base-sprite" {
		t.Errorf("first outbound material = %+v, want the canonical base sprite", first)
	}
	if first.Path != abs {
		t.Errorf("base sprite path = %q, want %q", first.Path, abs)
	}
	if !strings.Contains(plan.Prompt.Prompt, "Subject lock (top priority)") {
		t.Errorf("prompt misses the subject lock section: %s", plan.Prompt.Prompt)
	}
	found := false
	for _, r := range plan.Prompt.References {
		if r.Role == "base_sprite" {
			found = true
		}
	}
	if !found {
		t.Errorf("prompt snapshot references miss the base sprite: %+v", plan.Prompt.References)
	}
}
