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
// mismatch must be intercepted (拦截) with a message that names both sizes.
func TestImportBaseCharacterRejectsSizeMismatch(t *testing.T) {
	svc, root := newPhase6Svc(t)
	sprite := writeTestSprite(t, t.TempDir(), "small.png", 16, 16)

	_, err := svc.ImportBaseCharacter(root, sprite)
	if err == nil {
		t.Fatal("size mismatch unexpectedly accepted")
	}
	if !strings.Contains(err.Error(), "已拦截") || !strings.Contains(err.Error(), "不一致") || !strings.Contains(err.Error(), "16x16") || !strings.Contains(err.Error(), "32x32") {
		t.Fatalf("error should name both sizes and the interception: %v", err)
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

// 一次一张的草稿模型：重新导入会替换旧的未锁定草稿（记录与图片文件一并移除）。
func TestImportBaseCharacterReplacesPendingDraft(t *testing.T) {
	svc, root := newPhase6Svc(t)
	dir := t.TempDir()
	first := writeTestSprite(t, dir, "first.png", 32, 32)
	second := writeTestSprite(t, dir, "second.png", 32, 32)

	c1, err := svc.ImportBaseCharacter(root, first)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := svc.ImportBaseCharacter(root, second)
	if err != nil {
		t.Fatal(err)
	}

	fresh, err := svc.BaseCharacterCandidates(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh) != 1 || fresh[0].ID != c2.ID {
		t.Fatalf("candidates after re-import = %+v, want only %q", fresh, c2.ID)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(c1.ImagePath))); !os.IsNotExist(err) {
		t.Fatalf("old draft image should be removed, stat err = %v", err)
	}
}

// The GUI crop tool crops the picked image to a canvas-aspect rectangle; the
// service nearest-resizes the result to the logical canvas before registering
// the pending draft. Out-of-bounds and wrong-aspect rects are rejected, and a
// rejected crop leaves the existing draft untouched.
func TestImportBaseCharacterCropped(t *testing.T) {
	svc, root := newPhase6Svc(t)
	dir := t.TempDir()
	src := writeTestSprite(t, dir, "big.png", 64, 64) // canvas is 32x32

	c, err := svc.ImportBaseCharacterCropped(root, src, 16, 16, 32, 32)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != "pending" || c.Provider != "import" {
		t.Fatalf("candidate = %+v", c)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(c.ImagePath)))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 32 || img.Bounds().Dy() != 32 {
		t.Fatalf("cropped candidate size = %dx%d, want 32x32", img.Bounds().Dx(), img.Bounds().Dy())
	}

	// Wrong-aspect rect: rejected (would distort).
	if _, err := svc.ImportBaseCharacterCropped(root, src, 0, 0, 32, 16); err == nil {
		t.Fatal("wrong-aspect rect unexpectedly accepted")
	}
	// Out-of-bounds rect: rejected.
	if _, err := svc.ImportBaseCharacterCropped(root, src, 40, 40, 32, 32); err == nil {
		t.Fatal("out-of-bounds rect unexpectedly accepted")
	}
	// The rejected crops must not have replaced the earlier valid draft.
	fresh, err := svc.BaseCharacterCandidates(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh) != 1 || fresh[0].ID != c.ID {
		t.Fatalf("candidates after rejected crops = %+v, want only %q", fresh, c.ID)
	}
}
