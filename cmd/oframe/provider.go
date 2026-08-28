package main

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/oframe/character-workbench/core/provider"
)

// cmdProvider implements `oframe provider ...`: list | config get | config set
// | validate | stats — over the shared application service.
func cmdProvider(args []string, jsonOut bool, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("provider requires a subcommand: list | config | validate | stats")
	}
	switch args[0] {
	case "list":
		return cmdProviderList(args[1:], jsonOut, stdout)
	case "config":
		return cmdProviderConfig(args[1:], jsonOut, stdout)
	case "validate":
		return cmdProviderValidate(args[1:], jsonOut, stdout)
	case "stats":
		return cmdProviderStats(args[1:], jsonOut, stdout)
	default:
		return fmt.Errorf("unknown provider subcommand %q (list|config|validate|stats)", args[0])
	}
}

// settingsFlagSet is the common flag set for commands that need the shared
// service.
func settingsFlagSet(name string, args []string) (*flag.FlagSet, *string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	settingsDir := fs.String("settings-dir", "", "local settings directory (default: user config dir)")
	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}
	return fs, settingsDir, nil
}

func cmdProviderList(args []string, jsonOut bool, stdout io.Writer) error {
	_, settingsDir, err := settingsFlagSet("provider list", args)
	if err != nil {
		return err
	}
	svc, err := newCLIService(*settingsDir)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }()
	infos, err := svc.ProviderList()
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "providers (%s):\n", svc.SettingsDir())
	for _, p := range infos {
		mark := "  "
		if p.Active {
			mark = " *"
		}
		key := "key: none"
		if p.HasAPIKey {
			key = "key: " + p.KeySource
		}
		fmt.Fprintf(&b, "%s %-8s %-22s model=%s %s\n", mark, p.ID, p.Name, p.ImageModel, key)
	}
	return emit(stdout, jsonOut, map[string]any{"ok": true, "providers": infos}, b.String())
}

func cmdProviderConfig(args []string, jsonOut bool, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("provider config requires: get <id> | set <id>")
	}
	switch args[0] {
	case "get":
		return cmdProviderConfigGet(args[1:], jsonOut, stdout)
	case "set":
		return cmdProviderConfigSet(args[1:], jsonOut, stdout)
	default:
		return fmt.Errorf("unknown provider config subcommand %q (get|set)", args[0])
	}
}

func cmdProviderConfigGet(args []string, jsonOut bool, stdout io.Writer) error {
	fs := flag.NewFlagSet("provider config get", flag.ContinueOnError)
	settingsDir := fs.String("settings-dir", "", "local settings directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: oframe provider config get <id> [--settings-dir <dir>]")
	}
	svc, err := newCLIService(*settingsDir)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }()
	cfg, err := svc.ProviderConfig(fs.Arg(0))
	if err != nil {
		return err
	}
	return emit(stdout, jsonOut, map[string]any{"ok": true, "config": cfg},
		fmt.Sprintf("provider %s: model=%s textModel=%s baseUrl=%s maxAttempts=%d pricePerCall=%.4f key=%s",
			cfg.ProviderID, cfg.EffectiveModel(), cfg.EffectiveTextModel(), cfg.EffectiveBaseURL(),
			cfg.EffectiveMaxAttempts(), cfg.EffectivePrice(), redactKey(cfg.APIKey)))
}

func cmdProviderConfigSet(args []string, jsonOut bool, stdout io.Writer) error {
	fs := flag.NewFlagSet("provider config set", flag.ContinueOnError)
	settingsDir := fs.String("settings-dir", "", "local settings directory")
	key := fs.String("key", "", "API key (omit to keep the stored key)")
	model := fs.String("model", "", "image model")
	textModel := fs.String("text-model", "", "text model")
	baseURL := fs.String("base-url", "", "base URL")
	maxAttempts := fs.Int("max-attempts", 0, "max attempts per direction (default 3)")
	price := fs.Float64("price-per-call", 0, "cost estimate per call")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: oframe provider config set <id> [--key <key>] [--model <m>] [--text-model <m>] [--base-url <u>] [--max-attempts <n>] [--price-per-call <p>] [--settings-dir <dir>]")
	}
	id := fs.Arg(0)
	svc, err := newCLIService(*settingsDir)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }()
	cfg, err := svc.ProviderConfig(id)
	if err != nil {
		// Unknown id (fresh store carries no provider cards since the 人工验收
		// update): start from the id's built-in defaults — SaveProviderConfig
		// validates, persists and registers it in one step.
		cfg = provider.DefaultConfig(id)
	}
	if *key != "" {
		cfg.APIKey = *key
	}
	if *model != "" {
		cfg.Model = *model
	}
	if *textModel != "" {
		cfg.TextModel = *textModel
	}
	if *baseURL != "" {
		cfg.BaseURL = *baseURL
	}
	if *maxAttempts != 0 {
		cfg.MaxAttempts = *maxAttempts
	}
	if *price != 0 {
		cfg.PricePerCall = *price
	}
	if err := svc.SaveProviderConfig(id, cfg); err != nil {
		return err
	}
	return emit(stdout, jsonOut, map[string]any{"ok": true, "provider": id, "config": cfg},
		fmt.Sprintf("provider %s configured (model=%s, maxAttempts=%d)", id, cfg.EffectiveModel(), cfg.EffectiveMaxAttempts()))
}

func cmdProviderValidate(args []string, jsonOut bool, stdout io.Writer) error {
	fs := flag.NewFlagSet("provider validate", flag.ContinueOnError)
	settingsDir := fs.String("settings-dir", "", "local settings directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: oframe provider validate <id> [--settings-dir <dir>]")
	}
	svc, err := newCLIService(*settingsDir)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }()
	if err := svc.ValidateProvider(fs.Arg(0)); err != nil {
		return err
	}
	return emit(stdout, jsonOut, map[string]any{"ok": true, "provider": fs.Arg(0), "valid": true},
		fmt.Sprintf("provider %s configuration is valid", fs.Arg(0)))
}

func cmdProviderStats(args []string, jsonOut bool, stdout io.Writer) error {
	_, settingsDir, err := settingsFlagSet("provider stats", args)
	if err != nil {
		return err
	}
	svc, err := newCLIService(*settingsDir)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }()
	stats := svc.ProviderStats()
	var b strings.Builder
	fmt.Fprintf(&b, "total calls: %d\n", stats.TotalCalls())
	for _, st := range stats.Items {
		fmt.Fprintf(&b, "  %-8s %-24s calls=%d est=%s %.4f\n", st.ProviderID, st.Model, st.CallCount, st.Currency, st.EstimatedCost)
	}
	return emit(stdout, jsonOut, map[string]any{"ok": true, "stats": stats}, b.String())
}

// redactKey masks a key for human output: "sk-****last4".
func redactKey(key string) string {
	if key == "" {
		return "<unset>"
	}
	n := len(key)
	if n <= 4 {
		return "****"
	}
	return key[:2] + "****" + key[n-4:]
}
