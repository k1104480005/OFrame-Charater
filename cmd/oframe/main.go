// Command oframe is the scriptable CLI entry point over the shared Go core
// library — the same core the Wails GUI will bind to (design D1/D12, cli spec:
// 与 GUI 共享同一 Go 核心库). Phase 1 ships the scaffold: version, workspace,
// and identity package commands with machine-readable --json output. Batch
// generation, validation, and export commands land in later phases (12.2–12.4).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/logging"
	"github.com/oframe/character-workbench/core/version"
	"github.com/oframe/character-workbench/core/workspace"
)

// appVersion is the CLI/app version reported by `oframe version`.
const appVersion = "0.1.0"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "oframe:", err)
		os.Exit(1)
	}
}

// run executes the CLI. Logs go to stderr; machine-readable output goes to
// stdout. In --json mode a command failure also emits {"ok":false,"error":...}
// on stdout before returning the error.
func run(args []string, stdout, stderr io.Writer) error {
	jsonOut := false
	logLevel := "info"
	var rest []string
	for i := 0; i < len(args); {
		a := args[i]
		switch {
		case a == "--json":
			jsonOut = true
			i++
		case a == "--log-level":
			if i+1 >= len(args) {
				return fmt.Errorf("--log-level requires a value (debug|info|warn|error)")
			}
			logLevel = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--log-level="):
			logLevel = strings.TrimPrefix(a, "--log-level=")
			i++
		default:
			rest = append(rest, a)
			i++
		}
	}
	level, err := logging.ParseLevel(logLevel)
	if err != nil {
		return err
	}
	logging.Setup(logging.Options{Level: level, JSON: false, Output: stderr})
	slog.Debug("oframe starting", "args", rest, "json", jsonOut)

	if len(rest) == 0 || rest[0] == "help" || rest[0] == "--help" || rest[0] == "-h" {
		return printHelp(stdout)
	}
	cmd, cmdArgs := rest[0], rest[1:]
	var runErr error
	switch cmd {
	case "version":
		runErr = cmdVersion(jsonOut, stdout)
	case "workspace":
		runErr = cmdWorkspace(cmdArgs, jsonOut, stdout)
	case "identity":
		runErr = cmdIdentity(cmdArgs, jsonOut, stdout)
	case "provider":
		runErr = cmdProvider(cmdArgs, jsonOut, stdout)
	case "generation":
		runErr = cmdGeneration(cmdArgs, jsonOut, stdout)
	case "validate":
		runErr = cmdValidate(cmdArgs, jsonOut, stdout)
	case "export":
		runErr = cmdExport(cmdArgs, jsonOut, stdout)
	default:
		runErr = fmt.Errorf("unknown command %q (see 'oframe help')", cmd)
	}
	if runErr != nil && jsonOut {
		data, _ := json.MarshalIndent(map[string]any{"ok": false, "error": runErr.Error()}, "", "  ")
		fmt.Fprintln(stdout, string(data))
	}
	return runErr
}

func printHelp(stdout io.Writer) error {
	fmt.Fprintln(stdout, `oframe — OFrame Character workbench CLI (shared Go core)

Usage:
  oframe [--json] [--log-level <debug|info|warn|error>] <command> [args]

Commands:
  version                       print app and Go versions
  workspace init <path>         create/initialize a workspace directory
  workspace list [path]         list identity packages in a workspace
                                (default: user home workspace)
  identity create --workspace <path> --name <name>
                                create a new identity package
  identity open <path>          open and validate an identity package
  identity canvas --width <w> --height <h> <path>
                                set the logical canvas (required before
                                generation)
  provider list                 list providers and their local config status
  provider config get <id>      show a provider's local config
  provider config set <id> ...  set key/model/endpoint (local, validated)
  provider validate <id>        offline configuration validation
  provider stats                local call statistics (次数与费用估算)
  generation plan [flags] <identity-package-path>
                                build the generation confirmation plan
                                (no external calls)
  generation run [flags] --yes <identity-package-path>
                                execute generation after explicit confirmation;
                                without --yes nothing is called
  validate <path>               validate an identity or export package
  export create --output <dir> [--target <generic|godot|unity>] <pkg>
                                 build and validate an export package
  export validate <dir>         validate an existing export package
  export history [--settings-dir <dir>] <pkg>
                                show export history
  help                          show this help

Global flag --json switches stdout to machine-readable JSON.
Logs are written to stderr and never pollute stdout.`)
	return nil
}

// --- version ---

type versionOut struct {
	App       string `json:"app"`
	Version   string `json:"version"`
	GoVersion string `json:"goVersion"`
}

func cmdVersion(jsonOut bool, stdout io.Writer) error {
	v := versionOut{App: "oframe", Version: appVersion, GoVersion: runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH}
	text := fmt.Sprintf("oframe %s (%s)", v.Version, v.GoVersion)
	return emit(stdout, jsonOut, v, text)
}

// --- workspace ---

func cmdWorkspace(args []string, jsonOut bool, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("workspace requires a subcommand: init <path> | list [path]")
	}
	switch args[0] {
	case "init":
		if len(args) != 2 {
			return fmt.Errorf("usage: oframe workspace init <path>")
		}
		ws, err := workspace.Init(args[1])
		if err != nil {
			return err
		}
		return emit(stdout, jsonOut,
			map[string]any{"ok": true, "action": "init", "path": ws.Root(), "created": true},
			fmt.Sprintf("workspace initialized: %s", ws.Root()))
	case "list":
		path := ""
		if len(args) == 2 {
			path = args[1]
		} else if len(args) > 2 {
			return fmt.Errorf("usage: oframe workspace list [path]")
		}
		return cmdWorkspaceList(path, jsonOut, stdout)
	default:
		return fmt.Errorf("unknown workspace subcommand %q (init|list)", args[0])
	}
}

func cmdWorkspaceList(path string, jsonOut bool, stdout io.Writer) error {
	ws, err := openWorkspace(path)
	if err != nil {
		return err
	}
	pkgs, err := ws.List()
	if err != nil {
		return err
	}
	if pkgs == nil {
		pkgs = []identity.PackageInfo{} // JSON: [] instead of null
	}
	out := map[string]any{"ok": true, "path": ws.Root(), "packages": pkgs}
	var b strings.Builder
	fmt.Fprintf(&b, "workspace: %s\n", ws.Root())
	if len(pkgs) == 0 {
		b.WriteString("no identity packages found\n")
	}
	for _, p := range pkgs {
		fmt.Fprintf(&b, "  %s\t%s\tformat v%d\tcurrent %s\n", p.Name, p.Path, p.FormatVersion, p.CurrentVersion)
	}
	return emit(stdout, jsonOut, out, b.String())
}

func openWorkspace(path string) (*workspace.Workspace, error) {
	if path == "" {
		def, err := workspace.DefaultPath()
		if err != nil {
			return nil, err
		}
		path = def
	}
	ws, err := workspace.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%v (run 'oframe workspace init %s' first)", err, path)
	}
	return ws, nil
}

// --- identity ---

func cmdIdentity(args []string, jsonOut bool, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("identity requires a subcommand: create | open | canvas")
	}
	switch args[0] {
	case "create":
		return cmdIdentityCreate(args[1:], jsonOut, stdout)
	case "open":
		return cmdIdentityOpen(args[1:], jsonOut, stdout)
	case "canvas":
		return cmdIdentityCanvas(args[1:], jsonOut, stdout)
	default:
		return fmt.Errorf("unknown identity subcommand %q (create|open|canvas)", args[0])
	}
}

// cmdIdentityCanvas sets the logical canvas specification of an identity
// package (task 2.4). Generation requires a canvas, so CLI-created packages
// must set it before `generation plan|run`.
func cmdIdentityCanvas(args []string, jsonOut bool, stdout io.Writer) error {
	fs := flag.NewFlagSet("identity canvas", flag.ContinueOnError)
	width := fs.Int("width", 0, "logical canvas unit width (pixels)")
	height := fs.Int("height", 0, "logical canvas unit height (pixels)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || *width <= 0 || *height <= 0 {
		return fmt.Errorf("usage: oframe identity canvas --width <w> --height <h> <identity-package-path>")
	}
	pkg, err := identity.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	if err := pkg.SetLogicalCanvas(*width, *height); err != nil {
		return err
	}
	return emit(stdout, jsonOut,
		map[string]any{"ok": true, "action": "canvas", "path": fs.Arg(0), "canvas": map[string]int{"width": *width, "height": *height}},
		fmt.Sprintf("logical canvas set to %dx%d", *width, *height))
}

func cmdIdentityCreate(args []string, jsonOut bool, stdout io.Writer) error {
	fs := flag.NewFlagSet("identity create", flag.ContinueOnError)
	wsPath := fs.String("workspace", "", "workspace directory")
	name := fs.String("name", "", "identity package name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*name) == "" {
		return fmt.Errorf("identity create requires --name <name>")
	}
	ws, err := openWorkspace(*wsPath)
	if err != nil {
		return err
	}
	pkg, err := identity.Create(filepath.Join(ws.Root(), *name), *name)
	if err != nil {
		return err
	}
	m := pkg.Manifest()
	out := map[string]any{
		"ok":             true,
		"action":         "create",
		"name":           m.Identity.Name,
		"id":             m.Identity.ID,
		"path":           pkg.Root(),
		"formatVersion":  m.FormatVersion,
		"currentVersion": m.Versions.Current,
	}
	return emit(stdout, jsonOut, out,
		fmt.Sprintf("created identity package %q at %s (id %s, format v%d)", m.Identity.Name, pkg.Root(), m.Identity.ID, m.FormatVersion))
}

func cmdIdentityOpen(args []string, jsonOut bool, stdout io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: oframe identity open <path>")
	}
	pkg, err := identity.Open(args[0])
	if err != nil {
		return err
	}
	m := pkg.Manifest()
	cur, err := version.Current(pkg)
	if err != nil {
		return err
	}
	var canvas any
	if c := m.LogicalCanvas; c != nil {
		canvas = c
	}
	out := map[string]any{
		"ok":             true,
		"name":           m.Identity.Name,
		"id":             m.Identity.ID,
		"path":           pkg.Root(),
		"formatVersion":  m.FormatVersion,
		"entryKind":      m.Identity.EntryKind,
		"logicalCanvas":  canvas,
		"anchors":        len(m.Anchors),
		"materials":      len(m.Materials),
		"currentVersion": cur.ID,
	}
	return emit(stdout, jsonOut, out,
		fmt.Sprintf("opened identity package %q (id %s, format v%d, current version %s, %d anchors, %d materials)",
			m.Identity.Name, m.Identity.ID, m.FormatVersion, cur.ID, len(m.Anchors), len(m.Materials)))
}

// emit writes JSON to stdout in --json mode, otherwise the human text.
func emit(stdout io.Writer, jsonOut bool, v any, text string) error {
	if jsonOut {
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Errorf("encode json output: %w", err)
		}
		fmt.Fprintln(stdout, string(data))
		return nil
	}
	fmt.Fprintln(stdout, text)
	return nil
}
