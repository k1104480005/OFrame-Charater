package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	assetexport "github.com/oframe/character-workbench/core/assetexport"
	"github.com/oframe/character-workbench/core/identity"
)

func cmdValidate(args []string, jsonOut bool, stdout io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: oframe validate <identity-package|export-package>")
	}
	path := args[0]
	// An identity package carries identity metadata (identity.name); an export
	// package also has a manifest.json (formatVersion 1) but no identity
	// object — so distinguish by the identity name, not by identity.Open alone.
	if pkg, err := identity.Open(path); err == nil && strings.TrimSpace(pkg.Manifest().Identity.Name) != "" {
		return emit(stdout, jsonOut, map[string]any{"ok": true, "kind": "identityPackage", "path": path}, fmt.Sprintf("valid identity package: %s", path))
	}
	if err := assetexport.Validate(path); err != nil {
		return err
	}
	return emit(stdout, jsonOut, map[string]any{"ok": true, "kind": "exportPackage", "path": path}, fmt.Sprintf("valid export package: %s", path))
}

func cmdExport(args []string, jsonOut bool, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("export requires a subcommand: create | validate | history")
	}
	switch args[0] {
	case "validate":
		return cmdValidate(args[1:], jsonOut, stdout)
	case "history":
		return cmdExportHistory(args[1:], jsonOut, stdout)
	case "create":
		return cmdExportCreate(args[1:], jsonOut, stdout)
	default:
		return fmt.Errorf("unknown export subcommand %q (create|validate|history)", args[0])
	}
}

func cmdExportCreate(args []string, jsonOut bool, stdout io.Writer) error {
	fs := flag.NewFlagSet("export create", flag.ContinueOnError)
	pkgPath := fs.String("package", "", "identity package path")
	output := fs.String("output", "", "export output directory")
	target := fs.String("target", assetexport.TargetGeneric, "generic|godot|unity")
	versionID := fs.String("version", "", "identity version, defaults to current")
	settingsDir := fs.String("settings-dir", "", "local settings directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*pkgPath) == "" && fs.NArg() == 1 {
		*pkgPath = fs.Arg(0)
	}
	if strings.TrimSpace(*pkgPath) == "" || strings.TrimSpace(*output) == "" {
		return fmt.Errorf("usage: oframe export create <package> --output <dir> --target <generic|godot|unity>")
	}
	if !filepath.IsAbs(*output) {
		abs, err := filepath.Abs(*output)
		if err != nil {
			return err
		}
		*output = abs
	}
	svc, err := newCLIService(*settingsDir)
	if err != nil {
		return err
	}
	defer svc.Close()
	result, err := svc.ExportPackage(*pkgPath, *output, *target, *versionID)
	if err != nil {
		return err
	}
	return emit(stdout, jsonOut, map[string]any{"ok": true, "target": result.Target, "outputDir": result.OutputDir, "manifest": result.Manifest}, fmt.Sprintf("exported %s package to %s", result.Target, result.OutputDir))
}

func cmdExportHistory(args []string, jsonOut bool, stdout io.Writer) error {
	fs := flag.NewFlagSet("export history", flag.ContinueOnError)
	settingsDir := fs.String("settings-dir", "", "local settings directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: oframe export history <identity-package>")
	}
	svc, err := newCLIService(*settingsDir)
	if err != nil {
		return err
	}
	defer svc.Close()
	items, err := svc.ExportHistory(fs.Arg(0))
	if err != nil {
		return err
	}
	return emit(stdout, jsonOut, map[string]any{"ok": true, "items": items}, fmt.Sprintf("export history: %d record(s)", len(items)))
}

func cmdExportValidate(path string, stdout io.Writer) error {
	if err := assetexport.Validate(path); err != nil {
		return err
	}
	_, err := fmt.Fprintln(stdout, "valid export package:", path)
	return err
}

var _ = os.ErrNotExist
