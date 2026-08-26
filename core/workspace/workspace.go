// Package workspace manages the local space where the user organizes and
// manages character identity packages (CONTEXT.md: 工作区). A workspace is a
// directory whose entries are identity packages; it backs the launch page
// (select/create) and the CLI in later phases.
package workspace

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/pathutil"
)

// DefaultWorkspaceName is the workspace directory name under the user home.
const DefaultWorkspaceName = "OFrameWorkspace"

// DirTrash is the workspace trash directory. Deleted identity packages are
// moved here (recoverable from the file manager) instead of hard-deleted.
// List() never sees its children because they are not top-level entries.
const DirTrash = ".trash"

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
		Category:       m.Identity.Category,
		FormatVersion:  m.FormatVersion,
		CurrentVersion: m.Versions.Current,
		CreatedAt:      m.Identity.CreatedAt,
		UpdatedAt:      m.Identity.UpdatedAt,
	}, nil
}

// hasIdentityPackages reports whether dir exists and contains at least one
// identity package. It lets PreferredDefaultPath preserve an existing
// workspace at the legacy home default rather than stranding its packages.
func hasIdentityPackages(dir string) bool {
	if _, err := os.Stat(dir); err != nil {
		return false
	}
	ws, err := Open(dir)
	if err != nil {
		return false
	}
	pkgs, err := ws.List()
	if err != nil {
		return false
	}
	return len(pkgs) > 0
}

func abs(root string) (string, error) {
	root = pathutil.Normalize(root)
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("workspace: resolve %q: %w", root, err)
	}
	return abs, nil
}

// Migrate copies (or moves, when move is true) every identity package found
// directly inside the workspace to dst. Non-package subdirectories are left
// untouched. When move is true, a source package is removed only after a
// verified copy into dst. Packages that already exist in dst are skipped to
// avoid clobbering. See the workspace-migration requirement (切换/迁移工作区).
func (w *Workspace) Migrate(dst string, move bool) error {
	dst, err := abs(dst)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("workspace: create %s: %w", dst, err)
	}
	entries, err := os.ReadDir(w.root)
	if err != nil {
		return fmt.Errorf("workspace: read %s: %w", w.root, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		src := filepath.Join(w.root, e.Name())
		if _, err := identity.Open(src); err != nil {
			// Not an identity package — leave it where it is.
			continue
		}
		target := filepath.Join(dst, e.Name())
		if _, err := os.Stat(target); err == nil {
			w.log.Warn("workspace migrate: target exists, skipped", "target", target)
			continue
		}
		if err := copyTree(src, target); err != nil {
			return fmt.Errorf("workspace: migrate %s: %w", src, err)
		}
		if move {
			if err := os.RemoveAll(src); err != nil {
				return fmt.Errorf("workspace: remove source %s after migrate: %w", src, err)
			}
			w.log.Info("workspace migrated package (moved)", "from", src, "to", target)
		} else {
			w.log.Info("workspace migrated package (copied)", "from", src, "to", target)
		}
	}
	return nil
}

// copyTree recursively copies a directory tree from src to dst.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// TrashPackage moves an identity package directory into the workspace trash
// (<root>/.trash/<name>-<ts>) rather than hard-deleting it, so it can be
// recovered manually from the file manager. Refuses paths that are not valid
// identity packages or that escape the workspace root.
func (w *Workspace) TrashPackage(path string) (string, error) {
	clean := pathutil.Normalize(path)
	within, err := pathutil.IsWithin(w.root, clean)
	if err != nil {
		return "", fmt.Errorf("workspace: trash %s: %w", path, err)
	}
	if !within {
		return "", fmt.Errorf("workspace: refuse to trash %s: outside workspace", path)
	}
	if _, err := identity.Open(clean); err != nil {
		return "", fmt.Errorf("workspace: refuse to trash %s: not an identity package", path)
	}
	trashRoot := filepath.Join(w.root, DirTrash)
	if err := os.MkdirAll(trashRoot, 0o755); err != nil {
		return "", err
	}
	ts := time.Now().UTC().Format("20060102-150405")
	dst := filepath.Join(trashRoot, filepath.Base(clean)+"-"+ts)
	if err := os.Rename(clean, dst); err != nil {
		return "", err
	}
	w.log.Info("identity package moved to trash", "from", clean, "to", dst)
	return dst, nil
}
