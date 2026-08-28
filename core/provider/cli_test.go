package provider

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// CLI adapter tests (align-framebaker-providers tasks 3.2/3.3/3.4): argv
// execution without a shell, preflight rejections with zero process spawns,
// distinct failure reporting, and legacy-template compatibility.

// fakeCLIExe compiles the testdata/fakecli helper once per test run into a
// directory whose name contains a SPACE — the executable path itself then
// exercises argv boundary integrity end to end.
var fakeCLIBuild struct {
	once sync.Once
	path string
	err  error
}

func fakeCLIExe(t *testing.T) string {
	t.Helper()
	fakeCLIBuild.once.Do(func() {
		dir, err := os.MkdirTemp("", "oframe-fakecli")
		if err != nil {
			fakeCLIBuild.err = err
			return
		}
		sub := filepath.Join(dir, "fake cli dir")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			fakeCLIBuild.err = err
			return
		}
		name := "fakecli"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		out := filepath.Join(sub, name)
		if data, err := exec.Command("go", "build", "-o", out, "./testdata/fakecli").CombinedOutput(); err != nil {
			fakeCLIBuild.err = errors.New("build fakecli: " + string(data))
			return
		}
		fakeCLIBuild.path = out
	})
	if fakeCLIBuild.err != nil {
		t.Fatal(fakeCLIBuild.err)
	}
	return fakeCLIBuild.path
}

type fakeCLILogEntry struct {
	Args     []string         `json:"args"`
	RefSizes map[string]int64 `json:"refSizes"`
}

func readFakeCLILogs(t *testing.T, path string) []fakeCLILogEntry {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var entries []fakeCLILogEntry
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var e fakeCLILogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("decode fakecli log: %v", err)
		}
		entries = append(entries, e)
	}
	return entries
}

func cliTestConfig(t *testing.T) (ProviderConfig, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "fakecli.jsonl")
	t.Setenv("FAKECLI_LOG", logPath)
	t.Setenv("FAKECLI_MODE", "ok")
	cfg := ProviderConfig{
		ProviderID:     "my-tool",
		Type:           ProviderTypeCLI,
		Name:           "My Tool",
		Model:          "m1",
		CLICommand:     fakeCLIExe(t),
		CLIPromptArg:   "--prompt",
		CLIOutputArg:   "--output",
		CLIModelArg:    "--model",
		CLIRefImageArg: "--ref",
		CLIExtraArgs:   []string{"--verbose"},
	}
	return cfg, logPath
}

// TestCLIGenerateImageArgvContract pins the argv contract: prompt/model/refs/
// output each stay ONE argument element, extra args keep their order, refs are
// materialized in request order, and the output path is adapter-controlled.
func TestCLIGenerateImageArgvContract(t *testing.T) {
	cfg, logPath := cliTestConfig(t)
	prompt := `hero  with "quotes" & spaces — 中文`
	refA := &ReferenceImage{Kind: "reference_image", Role: "main_reference", MIME: "image/png", Data: []byte("ref-A-bytes")}
	refB := &ReferenceImage{Kind: "sprite", Role: "auxiliary_reference", MIME: "image/png", Data: []byte("ref-B-bytes!")}

	p := NewCLI(cfg, nil)
	res, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: prompt, Model: "m1", Width: 512, Height: 512, References: []ReferenceImage{*refA, *refB}})
	if err != nil {
		t.Fatal(err)
	}
	// Output must be the fakecli PNG and identified as PNG.
	if len(res.Data) < 8 || res.Data[0] != 0x89 || string(res.Data[1:4]) != "PNG" {
		t.Fatalf("output is not the expected PNG: %x", res.Data[:min(8, len(res.Data))])
	}
	if res.MIME != "image/png" || res.Provider != "my-tool" || res.Model != "m1" {
		t.Fatalf("result meta: %+v", res)
	}

	entries := readFakeCLILogs(t, logPath)
	if len(entries) != 1 {
		t.Fatalf("fakecli invocations = %d, want 1", len(entries))
	}
	got := entries[0]
	if len(got.Args) != 11 {
		t.Fatalf("argv = %q (len %d)", got.Args, len(got.Args))
	}
	wantShape := []string{"--verbose", "--prompt", prompt, "--model", "m1", "--ref", got.Args[6], "--ref", got.Args[8], "--output", got.Args[10]}
	for i, w := range wantShape {
		if got.Args[i] != w {
			t.Fatalf("argv[%d] = %q, want %q; full argv %q", i, got.Args[i], w, got.Args)
		}
	}
	if got.Args[2] != prompt {
		t.Fatalf("prompt traveled corrupted: %q", got.Args[2])
	}
	if filepath.Base(got.Args[10]) != "output.png" {
		t.Fatalf("output path not adapter-controlled: %q", got.Args[10])
	}
	if got.RefSizes[got.Args[6]] != int64(len(refA.Data)) || got.RefSizes[got.Args[8]] != int64(len(refB.Data)) {
		t.Fatalf("reference bytes did not reach temp files: %v", got.RefSizes)
	}
	// Reference temp files are cleaned up after the call.
	for _, refPath := range []string{got.Args[6], got.Args[8]} {
		if _, err := os.Stat(refPath); !os.IsNotExist(err) {
			t.Fatalf("reference temp file %q was not removed", refPath)
		}
	}
}

// TestCLIGenerateImagePositionalPrompt: without a prompt flag the prompt is
// appended as its own positional argument.
func TestCLIGenerateImagePositionalPrompt(t *testing.T) {
	cfg, logPath := cliTestConfig(t)
	cfg.CLIPromptArg = ""
	p := NewCLI(cfg, nil)
	if _, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "plain prompt"}); err != nil {
		t.Fatal(err)
	}
	entries := readFakeCLILogs(t, logPath)
	if len(entries) != 1 {
		t.Fatalf("invocations = %d", len(entries))
	}
	args := entries[0].Args
	if len(args) != 6 || args[0] != "--verbose" || args[1] != "plain prompt" || args[2] != "--model" || args[3] != "m1" || args[4] != "--output" {
		t.Fatalf("argv = %q", args)
	}
}

// TestCLIReferencePreflightZeroSpawns: references without a configured
// reference flag are rejected BEFORE any process spawn.
func TestCLIReferencePreflightZeroSpawns(t *testing.T) {
	cfg, logPath := cliTestConfig(t)
	cfg.CLIRefImageArg = ""
	cfg.CLICommand = filepath.Join(t.TempDir(), "definitely not a real exe")
	p := NewCLI(cfg, nil)
	_, err := p.GenerateImage(context.Background(), ImageRequest{
		Prompt:     "x",
		References: []ReferenceImage{{Kind: "reference_image", Role: "main_reference", Data: []byte("img")}},
	})
	if err == nil || !strings.Contains(err.Error(), "reference-image parameter is not configured") {
		t.Fatalf("err = %v", err)
	}
	if !IsNotRetryable(err) {
		t.Fatal("preflight rejection must be not-retryable")
	}
	if _, statErr := os.Stat(logPath); !os.IsNotExist(statErr) {
		t.Fatal("preflight rejection must not spawn the CLI")
	}
}

func TestCLIExecutableNotFound(t *testing.T) {
	cfg, logPath := cliTestConfig(t)
	cfg.CLICommand = filepath.Join(t.TempDir(), "no such tool.exe")
	p := NewCLI(cfg, nil)
	_, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
	}
	if !IsNotRetryable(err) {
		t.Fatal("executable-not-found must be not-retryable")
	}
	if _, statErr := os.Stat(logPath); !os.IsNotExist(statErr) {
		t.Fatal("not-found must not produce a fakecli log entry")
	}
}

func TestCLIExitCodeReported(t *testing.T) {
	cfg, _ := cliTestConfig(t)
	t.Setenv("FAKECLI_MODE", "fail-exit")
	p := NewCLI(cfg, nil)
	_, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "code 3") || !strings.Contains(err.Error(), "deliberate failure") {
		t.Fatalf("err = %v", err)
	}
}

func TestCLIMissingOutputReported(t *testing.T) {
	cfg, _ := cliTestConfig(t)
	t.Setenv("FAKECLI_MODE", "no-output")
	p := NewCLI(cfg, nil)
	_, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "did not produce the output file") {
		t.Fatalf("err = %v", err)
	}
}

func TestCLIBadFormatReported(t *testing.T) {
	cfg, _ := cliTestConfig(t)
	t.Setenv("FAKECLI_MODE", "bad-format")
	p := NewCLI(cfg, nil)
	_, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "not a valid image") {
		t.Fatalf("err = %v", err)
	}
}

func TestCLIEmptyOutputReported(t *testing.T) {
	cfg, _ := cliTestConfig(t)
	t.Setenv("FAKECLI_MODE", "empty-output")
	p := NewCLI(cfg, nil)
	_, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("err = %v", err)
	}
}

func TestCLICancelledBeforeStart(t *testing.T) {
	cfg, logPath := cliTestConfig(t)
	p := NewCLI(cfg, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.GenerateImage(ctx, ImageRequest{Prompt: "x"})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(logPath); !os.IsNotExist(statErr) {
		t.Fatal("cancelled generation must not spawn the CLI")
	}
}

func TestCLIGenerateTextUnsupported(t *testing.T) {
	cfg, logPath := cliTestConfig(t)
	p := NewCLI(cfg, nil)
	_, err := p.GenerateText(context.Background(), TextRequest{Prompt: "x"})
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, ErrCapabilityUnsupported) {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(logPath); !os.IsNotExist(statErr) {
		t.Fatal("text refusal must not spawn the CLI")
	}
}

// TestBuildCLIArgsBoundary pins the argv boundary contract directly: spaces,
// quotes and CJK characters never split an element, and empty optional flags
// degrade gracefully.
func TestBuildCLIArgsBoundary(t *testing.T) {
	cfg := ProviderConfig{
		CLIPromptArg:   "--prompt",
		CLIOutputArg:   "--output",
		CLIModelArg:    "--model",
		CLIRefImageArg: "--ref",
		CLIExtraArgs:   []string{"-v", "--seed", "42"},
	}
	got := buildCLIArgs(cfg, `a "b" c\n中文`, "m", []string{`C:\ref 1.png`, "/tmp/ref 2.png"}, `C:\out dir\o.png`)
	want := []string{
		"-v", "--seed", "42",
		"--prompt", `a "b" c\n中文`,
		"--model", "m",
		"--ref", `C:\ref 1.png`, "--ref", "/tmp/ref 2.png",
		"--output", `C:\out dir\o.png`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %q\nwant  %q", got, want)
	}
	// No model flag configured → no model pair; no prompt flag → positional.
	cfg2 := ProviderConfig{CLIOutputArg: "-o"}
	got2 := buildCLIArgs(cfg2, "p", "m", nil, "o.png")
	if !reflect.DeepEqual(got2, []string{"p", "-o", "o.png"}) {
		t.Fatalf("argv = %q", got2)
	}
}

func TestSniffImageFormat(t *testing.T) {
	cases := map[string]struct {
		data []byte
		want string
	}{
		"png":    {[]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x01}, "image/png"},
		"jpeg":   {[]byte{0xFF, 0xD8, 0xFF, 0xE0}, "image/jpeg"},
		"gif87a": {[]byte("GIF87a...."), "image/gif"},
		"gif89a": {[]byte("GIF89a...."), "image/gif"},
		"webp":   {[]byte("RIFF\x00\x00\x00\x00WEBPXX"), "image/webp"},
		"text":   {[]byte("hello world"), ""},
		"short":  {[]byte{0x89}, ""},
	}
	for name, tc := range cases {
		if got := sniffImageFormat(tc.data); got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}

// --- 3.4: legacy template compatibility ---

// TestCLILegacyTemplateIgnoredOnRun: a config that carries a legacy template
// string still executes through its STRUCTURED fields; the template never
// reaches the argv.
func TestCLILegacyTemplateIgnoredOnRun(t *testing.T) {
	cfg, logPath := cliTestConfig(t)
	cfg.CLITemplate = "{prompt} --evil-injected-flag -o hijacked.png"
	p := NewCLI(cfg, nil)
	if _, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "legacy prompt"}); err != nil {
		t.Fatal(err)
	}
	entries := readFakeCLILogs(t, logPath)
	if len(entries) != 1 {
		t.Fatalf("invocations = %d", len(entries))
	}
	args := entries[0].Args
	if strings.Contains(strings.Join(args, "\x00"), "evil-injected") || strings.Contains(strings.Join(args, "\x00"), "hijacked") {
		t.Fatalf("legacy template leaked into argv: %q", args)
	}
}

// TestCLILegacyTemplateOnlyRejected: template without a command fails with a
// readable pointer to the structured field instead of executing anything.
func TestCLILegacyTemplateOnlyRejected(t *testing.T) {
	cfg := ProviderConfig{
		ProviderID:   "old-cli",
		Type:         ProviderTypeCLI,
		Name:         "Old",
		Model:        "m",
		CLITemplate:  "{prompt} -o out.png",
		CLIOutputArg: "--output",
	}
	p := NewCLI(cfg, nil)
	_, err := p.GenerateImage(context.Background(), ImageRequest{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "cliTemplate is legacy read-only input") {
		t.Fatalf("err = %v", err)
	}
	if !IsNotRetryable(err) {
		t.Fatal("missing-command rejection must be not-retryable")
	}
}

// TestCLILegacyConfigStillRunnable: an old-format JSON config (template +
// structured fields) survives load → normalize → adapter construction →
// generation (历史 Provider 仍可运行).
func TestCLILegacyConfigStillRunnable(t *testing.T) {
	legacyJSON := `{
	  "providerId": "old-cli",
	  "type": "cli",
	  "name": "Old Tool",
	  "model": "m1",
	  "cliTemplate": "{prompt}",
	  "cliCommand": "REPLACED_BY_TEST",
	  "cliPromptArg": "--prompt",
	  "cliOutputArg": "--output",
	  "cliModelArg": "--model",
	  "cliRefImageArg": "--ref"
	}`
	cfg, _ := cliTestConfig(t)

	var stored ProviderConfig
	if err := json.Unmarshal([]byte(legacyJSON), &stored); err != nil {
		t.Fatal(err)
	}
	// The real executable path carries backslashes a raw JSON string cannot
	// hold unescaped, so it is injected into the struct after decoding
	// (the cliCommand round trip itself is covered by the other tests).
	stored.CLICommand = cfg.CLICommand
	normalized := NormalizeSettings(Settings{ActiveProvider: "old-cli", Providers: map[string]ProviderConfig{"old-cli": stored}})
	if normalized.Providers["old-cli"].CLITemplate != "{prompt}" {
		t.Fatal("legacy template must survive normalization")
	}
	adapter, err := NewAdapter("old-cli", normalized.Providers["old-cli"], nil)
	if err != nil {
		t.Fatalf("legacy config must still build an adapter: %v", err)
	}
	if _, ok := adapter.(*CLI); !ok {
		t.Fatalf("adapter = %T, want *CLI", adapter)
	}
	res, err := adapter.GenerateImage(context.Background(), ImageRequest{Prompt: "old"})
	if err != nil {
		t.Fatal(err)
	}
	if res.MIME != "image/png" {
		t.Fatalf("result: %+v", res)
	}
}
