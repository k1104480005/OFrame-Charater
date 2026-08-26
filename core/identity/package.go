package identity

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oframe/character-workbench/core/pathutil"
)

// Package is an opened character identity package rooted at a local directory
// (目录 + manifest, design D3). All mutations go through Update, which takes
// the manifest lock and persists atomically (temp file + rename).
type Package struct {
	root     string
	manifest Manifest
	mu       sync.RWMutex
	log      *slog.Logger
}

// PackageInfo is a lightweight summary of an identity package for listings
// (used by the workspace and the CLI).
type PackageInfo struct {
	Name           string    `json:"name"`
	Path           string    `json:"path"`
	Category       string    `json:"category,omitempty"`
	FormatVersion  int       `json:"formatVersion"`
	CurrentVersion string    `json:"currentVersion"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Root returns the absolute package directory.
func (p *Package) Root() string { return p.root }

// Manifest returns a copy of the current manifest.
func (p *Package) Manifest() Manifest {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.manifest
}

// FormatVersion returns the manifest format version of the open package.
func (p *Package) FormatVersion() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.manifest.FormatVersion
}

// Identity returns a copy of the identity metadata.
func (p *Package) Identity() IdentityMeta {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.manifest.Identity
}

// Logger returns the logger used by this package.
func (p *Package) Logger() *slog.Logger { return p.log }

// Create creates a new identity package at root: a local directory with the
// material/candidate/log/version areas and a manifest (task 2.1). It fails if
// root already exists so an existing package is never overwritten.
func Create(root, name string) (*Package, error) { return CreateWithLogger(root, name, slog.Default()) }

// CreateWithLogger is Create with an explicit logger.
func CreateWithLogger(root, name string, log *slog.Logger) (*Package, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("identity: package name is required")
	}
	root, err := absDir(root)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(root); err == nil {
		return nil, fmt.Errorf("identity: package directory already exists: %s", root)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("identity: cannot stat %s: %w", root, err)
	}
	now := time.Now().UTC()
	m := Manifest{
		FormatVersion: FormatVersion,
		Identity: IdentityMeta{
			ID:        mustID(),
			Name:      name,
			CreatedAt: now,
			UpdatedAt: now,
		},
		References: DefaultReferences(),
		Versions: Versions{
			Current: InitialVersionID,
			Items: []VersionRecord{{
				ID:        InitialVersionID,
				CreatedAt: now,
				Reason:    "initial",
				Immutable: true,
				AssetsRef: filepath.ToSlash(filepath.Join(DirVersions, InitialVersionID, "assets")),
			}},
		},
	}
	p := &Package{root: root, manifest: m, log: log}
	// Create the standard areas before the manifest write so the directory
	// layout and the manifest never disagree.
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("identity: create package directory: %w", err)
	}
	for _, dir := range []string{DirMaterials, DirCandidates, DirLog, DirVersions} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return nil, fmt.Errorf("identity: create %s area: %w", dir, err)
		}
	}
	if err := p.saveLocked(); err != nil {
		return nil, err
	}
	log.Info("identity package created", "path", root, "name", name, "id", m.Identity.ID, "formatVersion", FormatVersion)
	return p, nil
}

// Open loads an existing identity package (task 2.2). It refuses to open — and
// never modifies — packages whose manifest is missing, corrupt, or whose format
// version is newer than the application supports (migration hint).
func Open(root string) (*Package, error) { return OpenWithLogger(root, slog.Default()) }

// OpenWithLogger is Open with an explicit logger.
func OpenWithLogger(root string, log *slog.Logger) (*Package, error) {
	root, err := absDir(root)
	if err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(root, FileName)
	data, err := os.ReadFile(manifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrManifestMissing, manifestPath)
	}
	if err != nil {
		return nil, fmt.Errorf("identity: cannot read manifest %s: %w", manifestPath, err)
	}
	m, err := DecodeManifest(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifestCorrupt, err)
	}
	if m.FormatVersion > FormatVersion {
		return nil, &FormatError{PackageVersion: m.FormatVersion, SupportedVersion: FormatVersion}
	}
	if m.FormatVersion < 1 {
		return nil, fmt.Errorf("identity: manifest format version %d is invalid", m.FormatVersion)
	}
	log.Info("identity package opened", "path", root, "name", m.Identity.Name, "formatVersion", m.FormatVersion)
	return &Package{root: root, manifest: *m, log: log}, nil
}

// Save persists the current manifest.
func (p *Package) Save() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.saveLocked()
}

// Update applies fn to a copy of the manifest, bumps Identity.UpdatedAt, and
// persists atomically. All manifest mutations must go through Update so the
// in-memory state and the on-disk manifest never diverge.
func (p *Package) Update(fn func(*Manifest) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	m := p.manifest
	if err := fn(&m); err != nil {
		return err
	}
	m.Identity.UpdatedAt = time.Now().UTC()
	p.manifest = m
	return p.saveLocked()
}

// saveLocked writes the manifest via a temp file + rename (atomic on the same
// volume). Caller must hold p.mu.
func (p *Package) saveLocked() error {
	data, err := p.manifest.Encode()
	if err != nil {
		return fmt.Errorf("identity: encode manifest: %w", err)
	}
	tmp := filepath.Join(p.root, FileName+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("identity: write manifest: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(p.root, FileName)); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("identity: persist manifest: %w", err)
	}
	return nil
}

// absDir resolves root to an absolute, native-separator path.
func absDir(root string) (string, error) {
	root = pathutil.Normalize(root)
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("identity: resolve path %q: %w", root, err)
	}
	return abs, nil
}
