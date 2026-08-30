// Binding-level acceptance of the import base source: a local sprite becomes
// a pending candidate (provider "import") with an inline preview, shows up in
// the candidate listing, and adopts exactly like an AI-generated candidate —
// with zero external calls.
package main

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestBaseCharacterImportBinding(t *testing.T) {
	rt := newFakeRTForBaseCharacter(t)
	app, _ := newTestApp(t, fakeClient(rt.handler))

	if _, err := app.PackageCreate("Hero", ""); err != nil {
		t.Fatal(err)
	}
	if err := app.IdentitySetCanvas(32, 32); err != nil {
		t.Fatal(err)
	}

	// A 32×32 local sprite matches the logical canvas.
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	sprite := filepath.Join(t.TempDir(), "hero.png")
	if err := os.WriteFile(sprite, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	callsBefore := rt.calls.Load()
	view, err := app.BaseCharacterImport(sprite)
	if err != nil {
		t.Fatalf("BaseCharacterImport: %v", err)
	}
	if view.Status != "pending" || view.Provider != "import" || view.PNG == "" {
		t.Fatalf("import view = %+v", view)
	}
	if rt.calls.Load() != callsBefore {
		t.Fatal("import made external calls")
	}

	candidates, err := app.BaseCharacterCandidatesGet()
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].ID != view.ID {
		t.Fatalf("candidates after import = %+v", candidates)
	}

	// Same adopt path as AI candidates.
	if err := app.BaseCharacterAdopt(view.ID); err != nil {
		t.Fatal(err)
	}
	identity, err := app.IdentityGet()
	if err != nil {
		t.Fatal(err)
	}
	if identity.BaseCharacterID != view.ID {
		t.Fatalf("identity basis = %q, want %q", identity.BaseCharacterID, view.ID)
	}
}
