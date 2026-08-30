package service

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oframe/character-workbench/core/identity"
)

func writeTestSprite(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The import base source converges on the same candidate → adopt flow as AI
// generation: record pending with provider "import", then adopt sets the
// identity basis. No external call is involved.
func TestImportBaseCharacterRecordsAndAdopts(t *testing.T) {
	svc, root := newPhase6Svc(t)
	sprite := writeTestSprite(t, t.TempDir(), "hero.png", 32, 32)

	candidate, err := svc.ImportBaseCharacter(root, sprite)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Status != "pending" || candidate.Provider != "import" {
		t.Fatalf("candidate = %+v", candidate)
	}

	// Fresh read sees the recorded candidate (package reopened from disk).
	fresh, err := svc.BaseCharacterCandidates(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh) != 1 || fresh[0].ID != candidate.ID {
		t.Fatalf("candidates after import = %+v", fresh)
	}

	// Adoption is the explicit user decision and sets the identity basis.
	if err := svc.AdoptBaseCharacter(root, candidate.ID); err != nil {
		t.Fatal(err)
	}
	reopened, err := identity.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Manifest().Identity.BaseCharacter != candidate.ID {
		t.Fatalf("identity basis = %q, want %q", reopened.Manifest().Identity.BaseCharacter, candidate.ID)
	}
}

// The base semantic is a single frame at the logical canvas size: a size
// mismatch must be rejected with a message that names both sizes.
func TestImportBaseCharacterRejectsSizeMismatch(t *testing.T) {
	svc, root := newPhase6Svc(t)
	sprite := writeTestSprite(t, t.TempDir(), "small.png", 16, 16)

	_, err := svc.ImportBaseCharacter(root, sprite)
	if err == nil {
		t.Fatal("size mismatch unexpectedly accepted")
	}
	if !strings.Contains(err.Error(), "不一致") || !strings.Contains(err.Error(), "16x16") || !strings.Contains(err.Error(), "32x32") {
		t.Fatalf("error should name both sizes: %v", err)
	}

	// Garbage payload must fail decoding with a friendly message.
	bad := filepath.Join(t.TempDir(), "bad.png")
	if err := os.WriteFile(bad, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportBaseCharacter(root, bad); err == nil {
		t.Fatal("garbage image unexpectedly accepted")
	}
}
