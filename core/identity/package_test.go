package identity

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Task 2.1: create identity package (local directory + manifest); after
// creation the directory and manifest exist and the manifest parses.

func TestCreateIdentityPackage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !dirExists(t, root) {
		t.Fatal("package directory was not created")
	}
	manifestPath := filepath.Join(root, FileName)
	if !fileExists(t, manifestPath) {
		t.Fatal("manifest file was not created")
	}
	// Sub-areas exist.
	for _, d := range []string{DirMaterials, DirCandidates, DirLog, DirVersions} {
		if !dirExists(t, filepath.Join(root, d)) {
			t.Errorf("area %s missing", d)
		}
	}
	// Manifest parses and carries the format version.
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	m, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("manifest does not parse: %v", err)
	}
	if m.FormatVersion != FormatVersion {
		t.Errorf("FormatVersion = %d, want %d", m.FormatVersion, FormatVersion)
	}
	if m.Identity.Name != "Hero" || m.Identity.ID == "" {
		t.Errorf("identity metadata wrong: %+v", m.Identity)
	}
	if pkg.Root() != root {
		t.Errorf("pkg.Root() = %q, want %q", pkg.Root(), root)
	}
}

func TestCreateRejectsExistingDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "existing")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(root, "Hero"); err == nil {
		t.Fatal("Create should refuse an existing directory")
	}
}

func TestCreateRejectsEmptyName(t *testing.T) {
	if _, err := Create(filepath.Join(t.TempDir(), "x"), "  "); err == nil {
		t.Fatal("Create should require a non-empty name")
	}
}

// Task 2.2: open existing identity package — valid loads; missing/corrupt
// manifest refuses entry; format version newer than supported refuses and does
// not modify the package.

func TestOpenValidPackage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	if _, err := Create(root, "Hero"); err != nil {
		t.Fatal(err)
	}
	pkg, err := Open(root)
	if err != nil {
		t.Fatalf("Open valid package: %v", err)
	}
	if pkg.Identity().Name != "Hero" {
		t.Errorf("opened package identity = %+v", pkg.Identity())
	}
}

func TestOpenMissingManifest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "no-manifest")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Open(root)
	if !errors.Is(err, ErrManifestMissing) {
		t.Fatalf("Open missing manifest: got %v, want ErrManifestMissing", err)
	}
}

func TestOpenCorruptManifest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "corrupt")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Open(root)
	if !errors.Is(err, ErrManifestCorrupt) {
		t.Fatalf("Open corrupt manifest: got %v, want ErrManifestCorrupt", err)
	}
}

func TestOpenNewerFormatRefusesWithoutModification(t *testing.T) {
	root := filepath.Join(t.TempDir(), "future")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte(`{"formatVersion": 99, "identity": {"name": "Future"}}`)
	manifestPath := filepath.Join(root, FileName)
	if err := os.WriteFile(manifestPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Open(root)
	var fe *FormatError
	if !errors.As(err, &fe) {
		t.Fatalf("Open newer format: got %v, want *FormatError", err)
	}
	if !errors.Is(err, ErrFormatTooNew) {
		t.Fatalf("Open newer format should unwrap to ErrFormatTooNew, got %v", err)
	}
	if fe.PackageVersion != 99 || fe.SupportedVersion != FormatVersion {
		t.Errorf("FormatError versions = %d/%d", fe.PackageVersion, fe.SupportedVersion)
	}
	// The package content must be untouched.
	after, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(content) {
		t.Error("package was modified during a refused open")
	}
}

func TestOpenInvalidFormatVersion(t *testing.T) {
	root := filepath.Join(t.TempDir(), "zero")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), []byte(`{"formatVersion": 0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(root); err == nil {
		t.Fatal("Open should reject format version 0")
	}
}

// Round-trip: mutate through Update and re-open to verify persistence.

func TestUpdatePersistsToDisk(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Hero")
	pkg, err := Create(root, "Hero")
	if err != nil {
		t.Fatal(err)
	}
	if err := pkg.Update(func(m *Manifest) error {
		m.Identity.Name = "Hero v2"
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.Identity().Name != "Hero v2" {
		t.Errorf("persisted name = %q, want Hero v2", reopened.Identity().Name)
	}
}

func dirExists(t *testing.T, p string) bool {
	t.Helper()
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func fileExists(t *testing.T, p string) bool {
	t.Helper()
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
