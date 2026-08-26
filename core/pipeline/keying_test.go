package pipeline

import (
	"image"
	"image/color"
	"testing"
)

// keyScene builds a 32×32 scene: magenta technical background, a green
// character square with an enclosed magenta "hole", a magenta blob touching
// the border, and a magenta-fringe pixel adjacent to the character.
func keyScene() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 255, A: 255})
		}
	}
	// Green character 10×10 at (10,10)..(19,19).
	for y := 10; y < 20; y++ {
		for x := 10; x < 20; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 0, G: 255, B: 0, A: 255})
		}
	}
	// Enclosed magenta hole 3×3 at (13,13)..(15,15) — inside the character.
	for y := 13; y < 16; y++ {
		for x := 13; x < 16; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 255, A: 255})
		}
	}
	// Magenta blob touching the border at (0,0)..(5,5).
	for y := 0; y < 6; y++ {
		for x := 0; x < 6; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 255, A: 255})
		}
	}
	// Fringe pixel just left of the green square (9,10): magenta spill.
	img.SetRGBA(9, 10, color.RGBA{R: 255, G: 200, B: 255, A: 255})
	return img
}

func TestKeyChromaBackgroundRemoved(t *testing.T) {
	img := keyScene()
	out := KeyChroma(img, KeyOptions{})
	// Background pixel: fully transparent, RGB kept as the magenta technical
	// background (洋红仅用于抠图技术背景).
	bg := out.RGBAAt(20, 20)
	if bg.A != 0 {
		t.Errorf("background alpha = %d, want 0", bg.A)
	}
	if bg.R != 255 || bg.G != 0 || bg.B != 255 {
		t.Errorf("background RGB = %v, want magenta (255,0,255)", bg)
	}
	// Character pixel preserved.
	ch := out.RGBAAt(12, 12)
	if ch.A != 255 || ch.R != 0 || ch.G != 255 || ch.B != 0 {
		t.Errorf("character pixel = %v, want opaque green", ch)
	}
}

func TestKeyChromaFloodFillPreservesInteriorHole(t *testing.T) {
	out := KeyChroma(keyScene(), KeyOptions{})
	// The enclosed magenta hole is NOT connected to the border → preserved as
	// character detail (flood fill from the border only).
	hole := out.RGBAAt(14, 14)
	if hole.A != 255 {
		t.Errorf("interior magenta hole alpha = %d, want 255 (preserved by flood fill)", hole.A)
	}
	// A magenta blob touching the border IS removed.
	blob := out.RGBAAt(2, 2)
	if blob.A != 0 {
		t.Errorf("border-touching magenta blob alpha = %d, want 0", blob.A)
	}
}

func TestKeyChromaNoFloodFill(t *testing.T) {
	out := KeyChroma(keyScene(), KeyOptions{NoFloodFill: true})
	// Without flood fill, every key-like pixel is background, including the
	// interior hole.
	if hole := out.RGBAAt(14, 14); hole.A != 0 {
		t.Errorf("interior hole alpha = %d, want 0 without flood fill", hole.A)
	}
}

func TestKeyChromaDespill(t *testing.T) {
	out := KeyChroma(keyScene(), KeyOptions{})
	// Fringe pixel (255,200,255): above the key tolerance, within the despill
	// range → magenta spill (R/B) removed, green channel untouched.
	fringe := out.RGBAAt(9, 10)
	if fringe.A != 255 {
		t.Fatalf("fringe alpha = %d, want 255", fringe.A)
	}
	if fringe.R >= 255 || fringe.B >= 255 {
		t.Errorf("fringe RGB = %v, want despilled (R,B < 255)", fringe)
	}
	if fringe.G != 200 {
		t.Errorf("fringe green = %d, want 200 (despill must not touch green)", fringe.G)
	}
}

func TestKeyChromaDeterministic(t *testing.T) {
	img := keyScene()
	a := KeyChroma(img, KeyOptions{})
	b := KeyChroma(img, KeyOptions{})
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				t.Fatalf("keying not deterministic at (%d,%d): %v vs %v", x, y, a.RGBAAt(x, y), b.RGBAAt(x, y))
			}
		}
	}
}
