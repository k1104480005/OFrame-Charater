package service

import (
	"path/filepath"
	"testing"
)

// A package with no accepted current-version assets must never produce an
// export, even though its candidate history may contain pending/rejected
// candidates in normal use.
func TestExportRejectsWhenNoAcceptedAssets(t *testing.T) {
	svc, root := newPhase6Svc(t)
	out := filepath.Join(t.TempDir(), "export")
	if _, err := svc.ExportPackage(root, out, "generic", ""); err == nil {
		t.Fatal("export without accepted assets unexpectedly succeeded")
	}
}
