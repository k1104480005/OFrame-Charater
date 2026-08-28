package provider

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// CLI configuration serialization tests (align-framebaker-providers task 3.1:
// 配置序列化测试验证字段持久化). The structured CLI fields must survive a JSON
// round trip verbatim — they are the durable contract behind argv execution
// (task 3.2), which never interpolates user values into a shell string.

func TestCLIConfigJSONRoundTrip(t *testing.T) {
	cfg := ProviderConfig{
		ProviderID:     "my-tool",
		Type:           ProviderTypeCLI,
		Name:           "My CLI Tool",
		Model:          "tool-image-model",
		CLICommand:     `C:\tools\my tool.exe`,
		CLIPromptArg:   "--prompt",
		CLIOutputArg:   "--out",
		CLIModelArg:    "--model",
		CLIRefImageArg: "--ref",
		CLIExtraArgs:   []string{"--verbose", "--seed 42", "--weird 'quote'"},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var back ProviderConfig
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, back) {
		t.Fatalf("round trip drift:\n got %#v\nwant %#v", back, cfg)
	}
}

func TestCLIConfigOmitEmpty(t *testing.T) {
	raw, err := json.Marshal(ProviderConfig{ProviderID: "doubao", Type: ProviderDoubao})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, key := range []string{"cliCommand", "cliPromptArg", "cliOutputArg", "cliModelArg", "cliRefImageArg", "cliExtraArgs", "cliTemplate"} {
		if strings.Contains(s, key) {
			t.Fatalf("empty CLI field %q must not be serialized: %s", key, s)
		}
	}
}

func TestNormalizeSettingsPreservesCLIFields(t *testing.T) {
	in := Settings{
		ActiveProvider: "my-tool",
		Providers: map[string]ProviderConfig{
			"my-tool": {
				ProviderID:     "my-tool",
				Type:           ProviderTypeCLI,
				Name:           "My Tool",
				CLICommand:     "C:/tools/tool.exe",
				CLIPromptArg:   "-p",
				CLIOutputArg:   "-o",
				CLIModelArg:    "-m",
				CLIRefImageArg: "-r",
				CLIExtraArgs:   []string{"--flag", "value with spaces"},
				CLITemplate:    "{prompt} -o out.png", // legacy template kept verbatim
			},
		},
	}
	out := NormalizeSettings(in)
	got := out.Providers["my-tool"]
	if !reflect.DeepEqual(got, in.Providers["my-tool"]) {
		t.Fatalf("normalize drifted CLI fields:\n got %#v\nwant %#v", got, in.Providers["my-tool"])
	}
	if got.CLITemplate != in.Providers["my-tool"].CLITemplate {
		t.Fatal("legacy template must survive normalization verbatim")
	}
	if out.ActiveProvider != "my-tool" {
		t.Fatalf("active = %q", out.ActiveProvider)
	}
}

func TestPresetConfigDraftCLI(t *testing.T) {
	draft, err := PresetConfigDraft(PresetKeyCLI)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Type != ProviderTypeCLI {
		t.Fatalf("draft type = %q", draft.Type)
	}
	if draft.ProviderID != "" || draft.APIKey != "" || draft.CLICommand != "" {
		t.Fatalf("draft must leave id/key/command to the user: %+v", draft)
	}
	want := map[string]string{
		"CLIPromptArg":   "--prompt",
		"CLIOutputArg":   "--output",
		"CLIModelArg":    "--model",
		"CLIRefImageArg": "--ref",
	}
	if draft.CLIPromptArg != want["CLIPromptArg"] || draft.CLIOutputArg != want["CLIOutputArg"] ||
		draft.CLIModelArg != want["CLIModelArg"] || draft.CLIRefImageArg != want["CLIRefImageArg"] {
		t.Fatalf("CLI draft args = %+v", draft)
	}
	if draft.Name != "自定义 CLI" {
		t.Fatalf("draft name = %q", draft.Name)
	}

	// The draft is an independent copy: mutating it never touches the preset
	// table or later reads.
	draft.CLIExtraArgs = append(draft.CLIExtraArgs, "injected")
	draft.CLIModelArg = "hijacked"
	fresh, err := PresetConfigDraft(PresetKeyCLI)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.CLIModelArg != "--model" || len(fresh.CLIExtraArgs) != 0 {
		t.Fatalf("draft shares state with the preset table: %+v", fresh)
	}
}

func TestPresetConfigDraftAPIPreset(t *testing.T) {
	draft, err := PresetConfigDraft(PresetKeyOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Type != ProviderOpenAI || draft.BaseURL != DefaultOpenAIBaseURL {
		t.Fatalf("draft = %+v", draft)
	}
	if draft.CLICommand != "" || draft.CLIPromptArg != "" || draft.CLIExtraArgs != nil {
		t.Fatalf("non-CLI draft must not carry CLI fields: %+v", draft)
	}
	if _, err := PresetConfigDraft("nope"); err == nil {
		t.Fatal("unknown key must error")
	}
}
