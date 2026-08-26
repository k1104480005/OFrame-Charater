package motion

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileName is the motions file inside an identity package directory. It sits
// alongside the manifest (manifest.json) and holds the package's motion model
// (动作/方向集/帧序列). The identity-package spec does not define a motions
// location; a well-known companion file keeps the versioned manifest stable.
const FileName = "motions.json"

// MotionSet is the ordered, thread-safe set of motions of one identity
// package.
type MotionSet struct {
	mu    sync.RWMutex
	items map[string]*Motion
	order []string
}

// NewMotionSet creates an empty motion set.
func NewMotionSet() *MotionSet {
	return &MotionSet{items: make(map[string]*Motion)}
}

// Add inserts a motion; duplicate ids are rejected.
func (ms *MotionSet) Add(m *Motion) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if _, ok := ms.items[m.ID]; ok {
		return fmt.Errorf("motion: motion %q already exists", m.ID)
	}
	ms.items[m.ID] = m
	ms.order = append(ms.order, m.ID)
	return nil
}

// Get returns a motion by id.
func (ms *MotionSet) Get(id string) (*Motion, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	m, ok := ms.items[id]
	if !ok {
		return nil, fmt.Errorf("motion: unknown motion %q", id)
	}
	return m, nil
}

// Update replaces a motion; unknown ids are rejected.
func (ms *MotionSet) Update(m *Motion) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if _, ok := ms.items[m.ID]; !ok {
		return fmt.Errorf("motion: unknown motion %q", m.ID)
	}
	ms.items[m.ID] = m
	return nil
}

// Delete removes a motion by id.
func (ms *MotionSet) Delete(id string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if _, ok := ms.items[id]; !ok {
		return fmt.Errorf("motion: unknown motion %q", id)
	}
	delete(ms.items, id)
	for i, o := range ms.order {
		if o == id {
			ms.order = append(ms.order[:i], ms.order[i+1:]...)
			break
		}
	}
	return nil
}

// List returns the motions in insertion order.
func (ms *MotionSet) List() []*Motion {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	out := make([]*Motion, 0, len(ms.order))
	for _, id := range ms.order {
		out = append(out, ms.items[id])
	}
	return out
}

// Len returns the number of motions.
func (ms *MotionSet) Len() int {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return len(ms.items)
}

// Encode serializes the motion set (insertion order).
func (ms *MotionSet) Encode() ([]byte, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return json.MarshalIndent(ms.List(), "", "  ")
}

// DecodeMotionSet parses motion set bytes (a JSON array of motions).
func DecodeMotionSet(data []byte) (*MotionSet, error) {
	var list []*Motion
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("motion: cannot parse motions: %w", err)
	}
	ms := NewMotionSet()
	for _, m := range list {
		if m == nil || strings.TrimSpace(m.ID) == "" {
			return nil, fmt.Errorf("motion: invalid motion entry in motions file")
		}
		if err := ValidateStrategy(m.Strategy); err != nil {
			return nil, fmt.Errorf("motion: motion %q has invalid strategy: %w", m.ID, err)
		}
		ms.items[m.ID] = m
		ms.order = append(ms.order, m.ID)
	}
	return ms, nil
}

// Store persists the motion set of one identity package to <root>/motions.json
// (atomic temp-file + rename, mirroring the manifest persistence).
type Store struct {
	root string
}

// NewStore creates a store rooted at an identity package directory.
func NewStore(root string) *Store { return &Store{root: root} }

// Path returns the motions file path.
func (s *Store) Path() string { return filepath.Join(s.root, FileName) }

// Load reads the motion set; a missing file yields an empty set.
func (s *Store) Load() (*MotionSet, error) {
	data, err := os.ReadFile(s.Path())
	if err != nil {
		if os.IsNotExist(err) {
			return NewMotionSet(), nil
		}
		return nil, fmt.Errorf("motion: read motions: %w", err)
	}
	return DecodeMotionSet(data)
}

// Save writes the motion set atomically.
func (s *Store) Save(ms *MotionSet) error {
	data, err := ms.Encode()
	if err != nil {
		return fmt.Errorf("motion: encode motions: %w", err)
	}
	tmp := s.Path() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("motion: write motions: %w", err)
	}
	if err := os.Rename(tmp, s.Path()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("motion: persist motions: %w", err)
	}
	return nil
}
