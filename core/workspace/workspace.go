// Package workspace manages the local space where the user organizes and
// manages character identity packages (CONTEXT.md: 工作区). A workspace is a
// directory whose entries are identity packages; it backs the launch page
// (select/create) and the CLI in later phases.
package workspace

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/pathutil"
)

// DefaultWorkspaceName is the workspace directory name under the user home.
const DefaultWorkspaceName = "OFrameWorkspace"

// Workspace is an opened workspace directory.
type Workspace struct {
	root string
	log  *slog.Logger
}

// Init creates (idempotently) a workspace directory at root and opens it.
func Init(root string) (*Workspace, error) { return InitWithLogger(root, slog.Default()) }

// InitWithLogger is Init with an explicit logger.
func InitWithLogger(root string, log *slog.Logger) (*Workspace, error) {
	root, err := abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("workspace: create %s: %w", root, err)
	}
	log.Info("workspace initialized", "path", root)
	return &Workspace{root: root, log: log}, nil
}

// Open opens an existing workspace directory.
func Open(root string) (*Workspace, error) {
	root, err := abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("workspace: cannot open %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace: %s is not a directory", root)
	}
	return &Workspace{root: root, log: slog.Default()}, nil
}

// Root returns the absolute workspace directory.
func (w *Workspace) Root() string { return w.root }

// List returns the identity packages found directly inside the workspace
// (directories whose manifest loads), sorted by name. Non-package directories
// are ignored; corrupted packages surface through identity.Open.
func (w *Workspace) List() ([]identity.PackageInfo, error) {
	entries, err := os.ReadDir(w.root)
	if err != nil {
		return nil, fmt.Errorf("workspace: list %s: %w", w.root, err)
	}
	var out []identity.PackageInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(w.root, e.Name())
		pkg, err := identity.Open(dir)
		if err != nil {
			// Not an identity package (or corrupted); skip for listings —
			// corruption is reported when the user opens it.
			continue
		}
		info, err := pkgInfo(pkg)
		if err != nil {
			continue
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Resolve maps a name or an absolute path to an identity package path inside
// the workspace, refusing paths that escape the workspace root.
func (w *Workspace) Resolve(nameOrPath string) (string, error) {
	if pathutil.IsAbsolute(nameOrPath) {
		cleaned := pathutil.Clean(nameOrPath)
		ok, err := pathutil.IsWithin(w.root, cleaned)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", fmt.Errorf("workspace: %q is outside the workspace %s", nameOrPath, w.root)
		}
		return cleaned, nil
	}
	joined := filepath.Join(w.root, pathutil.Normalize(nameOrPath))
	ok, err := pathutil.IsWithin(w.root, joined)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("workspace: %q escapes the workspace root", nameOrPath)
	}
	return joined, nil
}

// DefaultPath returns the default workspace location under the user home
// directory (Windows: %USERPROFILE%\OFrameWorkspace).
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("workspace: cannot resolve user home: %w", err)
	}
	return filepath.Join(home, DefaultWorkspaceName), nil
}

// pkgInfo builds a PackageInfo for an opened package.
func pkgInfo(pkg *identity.Package) (identity.PackageInfo, error) {
	m := pkg.Manifest()
	return identity.PackageInfo{
		Name:           m.Identity.Name,
		Path:           pkg.Root(),
		FormatVersion:  m.FormatVersion,
		CurrentVersion: m.Versions.Current,
	}, nil
}

func abs(root string) (string, error) {
	root = pathutil.Normalize(root)
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("workspace: resolve %q: %w", root, err)
	}
	return abs, nil
}
