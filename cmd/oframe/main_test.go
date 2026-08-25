package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	err := run(args, &out, &errb)
	return out.String(), errb.String(), err
}

func TestVersionJSON(t *testing.T) {
	out, _, err := runCLI(t, "version", "--json")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("version output is not valid JSON: %v\n%s", err, out)
	}
	if v["app"] != "oframe" || v["version"] == "" || v["goVersion"] == "" {
		t.Errorf("version fields wrong: %v", v)
	}
}

func TestVersionText(t *testing.T) {
	out, _, err := runCLI(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(out), []byte("oframe")) {
		t.Errorf("text version output missing app name: %q", out)
	}
}

func TestIdentityCreateOpenListJSON(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "ws")

	out, _, err := runCLI(t, "workspace", "init", ws, "--json")
	if err != nil {
		t.Fatalf("workspace init: %v\n%s", err, out)
	}
	var initRes map[string]any
	if err := json.Unmarshal([]byte(out), &initRes); err != nil || initRes["ok"] != true {
		t.Fatalf("init json: %v\n%s", err, out)
	}

	out, _, err = runCLI(t, "identity", "create", "--workspace", ws, "--name", "Hero", "--json")
	if err != nil {
		t.Fatalf("identity create: %v\n%s", err, out)
	}
	var created map[string]any
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("create json: %v\n%s", err, out)
	}
	if created["ok"] != true || created["name"] != "Hero" || created["formatVersion"].(float64) != 1 {
		t.Fatalf("create fields wrong: %v", created)
	}
	if !fileExistsCLI(created["path"].(string) + string(filepath.Separator) + "manifest.json") {
		t.Fatal("manifest.json was not created")
	}

	out, _, err = runCLI(t, "identity", "open", created["path"].(string), "--json")
	if err != nil {
		t.Fatalf("identity open: %v\n%s", err, out)
	}
	var opened map[string]any
	if err := json.Unmarshal([]byte(out), &opened); err != nil {
		t.Fatalf("open json: %v\n%s", err, out)
	}
	if opened["ok"] != true || opened["name"] != "Hero" || opened["currentVersion"] != "v1" {
		t.Fatalf("open fields wrong: %v", opened)
	}

	out, _, err = runCLI(t, "workspace", "list", ws, "--json")
	if err != nil {
		t.Fatalf("workspace list: %v\n%s", err, out)
	}
	var list map[string]any
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("list json: %v\n%s", err, out)
	}
	pkgs, ok := list["packages"].([]any)
	if !ok || len(pkgs) != 1 {
		t.Fatalf("list packages wrong: %v", list)
	}
}

func TestIdentityOpenCorruptJSONError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bad")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCLI(t, "identity", "open", dir, "--json")
	if err == nil {
		t.Fatal("open of a corrupt package should fail")
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("error output is not JSON: %v\n%s", err, out)
	}
	if res["ok"] != false || !bytes.Contains([]byte(res["error"].(string)), []byte("manifest corrupt")) {
		t.Fatalf("error json wrong: %v", res)
	}
}

func TestIdentityOpenMissingManifest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "empty")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, "identity", "open", dir); err == nil {
		t.Fatal("open without manifest should fail")
	}
}

func TestUnknownCommand(t *testing.T) {
	if _, _, err := runCLI(t, "bogus"); err == nil {
		t.Fatal("unknown command should fail")
	}
}

func TestHelp(t *testing.T) {
	out, _, err := runCLI(t, "help")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(out), []byte("identity create")) {
		t.Errorf("help should document identity create: %q", out)
	}
}

func TestWorkspaceListEmpty(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "empty-ws")
	if _, _, err := runCLI(t, "workspace", "init", ws); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCLI(t, "workspace", "list", ws, "--json")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var list map[string]any
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatal(err)
	}
	if pkgs := list["packages"].([]any); len(pkgs) != 0 {
		t.Fatalf("expected no packages, got %v", pkgs)
	}
}

func fileExistsCLI(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
