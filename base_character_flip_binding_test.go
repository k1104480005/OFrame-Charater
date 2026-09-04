// Binding-level acceptance of base-character horizontal flip (姘村钩缈昏浆): an
// imported sprite flips pixel-exactly, flipping twice restores the original,
// and the flip works on the ADOPTED basis too 鈥?the same record/file the
// generation pipeline later reads as its base sprite reference. Zero external
// calls.
package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func decodeViewPNG(t *testing.T, b64 string) image.Image {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("decode base64 preview: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode preview PNG: %v", err)
	}
	return img
}

// px reads a pixel generically (the persisted PNG may decode as RGBA or
// NRGBA depending on whether it contains semi-transparent pixels).
func px(img image.Image, x, y int) color.RGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func TestBaseCharacterFlipBinding(t *testing.T) {
	rt := newFakeRTForBaseCharacter(t)
	app, _ := newTestApp(t, fakeClient(rt.handler))

	if _, err := app.PackageCreate("Hero", ""); err != nil {
		t.Fatal(err)
	}
	if err := app.IdentitySetCanvas(32, 32); err != nil {
		t.Fatal(err)
	}

	// Asymmetric sprite: left column red, right column blue 鈥?a flip must
	// swap the columns.
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	for y := 0; y < 32; y++ {
		img.SetRGBA(0, y, red)
		img.SetRGBA(31, y, blue)
	}
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
	decoded := decodeViewPNG(t, view.PNG)
	if px(decoded, 0, 15) != red || px(decoded, 31, 15) != blue {
		t.Fatalf("imported sprite lost its asymmetry: %v / %v", px(decoded, 0, 15), px(decoded, 31, 15))
	}

	// Flip the pending candidate: columns swap.
	if err := app.BaseCharacterFlip(view.ID); err != nil {
		t.Fatalf("BaseCharacterFlip (pending): %v", err)
	}
	list, err := app.BaseCharacterCandidatesGet()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("candidates after flip = %d, want 1", len(list))
	}
	decoded = decodeViewPNG(t, list[0].PNG)
	if px(decoded, 0, 15) != blue || px(decoded, 31, 15) != red {
		t.Fatalf("pending flip did not mirror: %v / %v", px(decoded, 0, 15), px(decoded, 31, 15))
	}

	// Flip twice restores the original pixels 鈥?no backup state needed.
	if err := app.BaseCharacterFlip(view.ID); err != nil {
		t.Fatalf("BaseCharacterFlip (inverse): %v", err)
	}
	list, err = app.BaseCharacterCandidatesGet()
	if err != nil {
		t.Fatal(err)
	}
	decoded = decodeViewPNG(t, list[0].PNG)
	if px(decoded, 0, 15) != red || px(decoded, 31, 15) != blue {
		t.Fatalf("double flip did not restore: %v / %v", px(decoded, 0, 15), px(decoded, 31, 15))
	}

	// Adopt, then flip the ADOPTED basis 鈥?the same file later generations
	// read; the binding must not refuse it.
	if err := app.BaseCharacterAdopt(view.ID); err != nil {
		t.Fatal(err)
	}
	if err := app.BaseCharacterFlip(view.ID); err != nil {
		t.Fatalf("BaseCharacterFlip (adopted): %v", err)
	}
	list, err = app.BaseCharacterCandidatesGet()
	if err != nil {
		t.Fatal(err)
	}
	decoded = decodeViewPNG(t, list[0].PNG)
	if px(decoded, 0, 15) != blue || px(decoded, 31, 15) != red {
		t.Fatalf("adopted flip did not mirror: %v / %v", px(decoded, 0, 15), px(decoded, 31, 15))
	}
	identity, err := app.IdentityGet()
	if err != nil {
		t.Fatal(err)
	}
	if identity.BaseCharacterID != view.ID {
		t.Fatalf("flip changed the identity basis: %q, want %q", identity.BaseCharacterID, view.ID)
	}

	if rt.calls.Load() != callsBefore {
		t.Fatal("flip made external calls")
	}
}

func TestBaseCharacterFlipUnknownCandidate(t *testing.T) {
	rt := newFakeRTForBaseCharacter(t)
	app, _ := newTestApp(t, fakeClient(rt.handler))

	if _, err := app.PackageCreate("Hero", ""); err != nil {
		t.Fatal(err)
	}
	if err := app.BaseCharacterFlip("no-such-id"); err == nil {
		t.Fatal("flip of unknown candidate id must fail")
	}
}
