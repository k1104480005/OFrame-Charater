// Package settings provides the local application configuration store: provider
// keys/models and call statistics, persisted as a JSON file under the user
// config directory (Windows: %AppData%\OFrameCharacter\settings.json). It backs
// the generation capability's local key/credit/call-statistics management
// (generation spec 4.6: 密钥、额度与调用统计在本地管理) and is shared by the
// Wails GUI and the oframe CLI through core/service.
//
// Keys are stored locally in plain text for phase 3 (local-first, design D6);
// the store is written atomically (temp file + rename) and is thread-safe.
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/oframe/character-workbench/core/provider"
)

// DirName is the config directory name under the user config root.
const DirName = "OFrameCharacter"

// FileName is the settings file inside the config directory.
const FileName = "settings.json"

// Data is the persisted settings payload.
type Data struct {
	Provider provider.Settings `json:"provider"`
	Stats    provider.Stats    `json:"stats"`
}

// DefaultData returns the default settings (Doubao active, three built-in
// provider configs, empty stats).
func DefaultData() Data {
	return Data{Provider: provider.DefaultSettings()}
}

// Store is a thread-safe settings store persisted at dir/FileName.
type Store struct {
	mu   sync.RWMutex
	dir  string
	path string
	data Data
}

// DefaultDir returns the user config directory for the application.
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("settings: cannot resolve user config dir: %w", err)
	}
	return filepath.Join(base, DirName), nil
}

// New creates (loading or initializing) a settings store rooted at dir. An
// empty dir selects DefaultDir. A missing file is initialized with defaults;
// a corrupt file returns an error without being overwritten.
func New(dir string) (*Store, error) {
	if dir == "" {
		d, err := DefaultDir()
		if err != nil {
			return nil, err
		}
		dir = d
	}
	s := &Store{dir: dir, path: filepath.Join(dir, FileName), data: DefaultData()}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("settings: read %s: %w", s.path, err)
	}
	var d Data
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("settings: corrupt settings file %s: %w", s.path, err)
	}
	s.data = d
	return s, nil
}

// Dir returns the store directory.
func (s *Store) Dir() string { return s.dir }

// Path returns the settings file path.
func (s *Store) Path() string { return s.path }

// ProviderSettings returns a copy of the provider settings.
func (s *Store) ProviderSettings() provider.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := provider.Settings{ActiveProvider: s.data.Provider.ActiveProvider}
	out.Providers = make(map[string]provider.ProviderConfig, len(s.data.Provider.Providers))
	for id, c := range s.data.Provider.Providers {
		out.Providers[id] = c
	}
	return out
}

// SaveProviderSettings persists the provider settings (keys/models/active).
func (s *Store) SaveProviderSettings(p provider.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Provider = p
	return s.saveLocked()
}

// Stats returns a copy of the call statistics.
func (s *Store) Stats() provider.Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := provider.Stats{Items: append([]provider.Stat(nil), s.data.Stats.Items...)}
	return out
}

// RecordCall records one completed provider call and persists immediately
// (spec 4.6: 每次调用后统计次数与费用估算更新).
func (s *Store) RecordCall(providerID, model string, cost float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Stats.RecordCall(providerID, model, cost)
	return s.saveLocked()
}

// saveLocked writes the file atomically. Caller must hold s.mu.
func (s *Store) saveLocked() error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("settings: create config dir: %w", err)
	}
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("settings: encode: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("settings: write: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("settings: persist: %w", err)
	}
	return nil
}
