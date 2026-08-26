package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// configSubdir is the application config directory name, shared with the
// provider settings store (core/settings: OFrameCharacter) so related config
// lives together under the user config root.
const configSubdir = "OFrameCharacter"

// configFile is the workspace-choice persistence file.
const configFile = "workspace.json"

// Config is the persisted workspace preference. Path is the user's chosen
// workspace directory; empty means "use the preferred default".
type Config struct {
	Path string `json:"path"`
}

// configPathResolver resolves the config file path. It is a variable so tests
// can redirect it to a temporary location instead of the real user config dir.
var configPathResolver = defaultConfigPath

// defaultConfigPath returns the absolute path of the workspace config file
// under the user config directory.
func defaultConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("workspace: resolve user config dir: %w", err)
	}
	return filepath.Join(base, configSubdir, configFile), nil
}

// configPath returns the absolute path of the workspace config file.
func configPath() (string, error) {
	return configPathResolver()
}

// LoadConfig reads the persisted workspace choice. A missing file is not an
// error (returns a zero Config); a corrupt file surfaces an error so the
// caller can decide whether to ignore it.
func LoadConfig() (Config, error) {
	p, err := configPath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("workspace: read config %s: %w", p, err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("workspace: corrupt config %s: %w", p, err)
	}
	return c, nil
}

// SaveConfig persists the workspace choice atomically (temp file + rename).
func SaveConfig(c Config) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("workspace: create config dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("workspace: encode config: %w", err)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("workspace: write config: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("workspace: persist config: %w", err)
	}
	return nil
}

// PreferredDefaultPath returns the workspace path to use at startup: the
// user's persisted choice if any, otherwise — for a fresh install — a
// non-system-drive default when one is available (so the workspace is not
// forced onto C:), falling back to the user-home default. An existing
// workspace already living at the legacy home default is always preserved so
// we never silently strand a user's packages. See CONTEXT.md (工作区) and the
// workspace-settings requirement to default off the system drive.
func PreferredDefaultPath() (string, error) {
	if cfg, err := LoadConfig(); err == nil && cfg.Path != "" {
		return cfg.Path, nil
	}
	home, err := DefaultPath()
	if err != nil {
		return "", err
	}
	// Keep an existing workspace at the legacy home default (C:\Users\…) so a
	// returning user still sees their packages after we begin defaulting off
	// the system drive.
	if hasIdentityPackages(home) {
		return home, nil
	}
	if p := nonSystemDriveDefault(); p != "" {
		return p, nil
	}
	return home, nil
}

// nonSystemDriveDefault returns a default workspace path on a non-system
// fixed drive when one exists (Windows: any drive letter other than the
// system drive, tried D..Z in order), or "" when none is suitable.
func nonSystemDriveDefault() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	sys := strings.ToUpper(filepath.VolumeName(home)) // e.g. "C:"
	for _, c := range "DEFGHIJKLMNOPQRSTUVWXYZ" {
		cand := string(c) + ":"
		if strings.EqualFold(cand, sys) {
			continue
		}
		root := cand + "\\"
		if _, err := os.Stat(root); err != nil {
			continue
		}
		return filepath.Join(root, DefaultWorkspaceName)
	}
	return ""
}
