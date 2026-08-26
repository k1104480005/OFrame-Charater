package pipeline

import (
	"testing"

	"github.com/oframe/character-workbench/core/identity"
)

func TestNormalizeFrameList(t *testing.T) {
	canvas, err := identity.NewCanvasSpec(32, 32)
	if err != nil {
		t.Fatalf("canvas: %v", err)
	}
	fl, err := NormalizeFrameList(*canvas, 4)
	if err != nil {
		t.Fatalf("NormalizeFrameList: %v", err)
	}
	if fl.FrameCount != 4 {
		t.Fatalf("FrameCount = %d, want 4", fl.FrameCount)
	}
	if fl.StripWidth != 128 || fl.StripHeight != 32 {
		t.Fatalf("strip = %dx%d, want 128x32", fl.StripWidth, fl.StripHeight)
	}
	if len(fl.Frames) != 4 {
		t.Fatalf("len(frames) = %d, want 4", len(fl.Frames))
	}
	want := [][2]int{{0, 0}, {32, 0}, {64, 0}, {96, 0}}
	for i, spec := range fl.Frames {
		if spec.Index != i {
			t.Errorf("frame %d: Index = %d", i, spec.Index)
		}
		if spec.Width != 32 || spec.Height != 32 {
			t.Errorf("frame %d: size = %dx%d, want 32x32", i, spec.Width, spec.Height)
		}
		if [2]int{spec.X, spec.Y} != want[i] {
			t.Errorf("frame %d: pos = %v, want %v", i, [2]int{spec.X, spec.Y}, want[i])
		}
	}
}

func TestNormalizeFrameListErrors(t *testing.T) {
	canvas, err := identity.NewCanvasSpec(32, 32)
	if err != nil {
		t.Fatalf("canvas: %v", err)
	}
	if _, err := NormalizeFrameList(*canvas, 0); err == nil {
		t.Error("frame count 0: expected error")
	}
	if _, err := NormalizeFrameList(*canvas, -1); err == nil {
		t.Error("negative frame count: expected error")
	}
	if _, err := NormalizeFrameList(identity.CanvasSpec{}, 4); err == nil {
		t.Error("invalid canvas: expected error")
	}
}

func TestFrameListFrameAt(t *testing.T) {
	canvas, _ := identity.NewCanvasSpec(16, 16)
	fl, _ := NormalizeFrameList(*canvas, 3)
	if _, err := fl.FrameAt(0); err != nil {
		t.Errorf("FrameAt(0): %v", err)
	}
	if _, err := fl.FrameAt(3); err == nil {
		t.Error("FrameAt(3) out of range: expected error")
	}
	if _, err := fl.FrameAt(-1); err == nil {
		t.Error("FrameAt(-1) out of range: expected error")
	}
}

func TestNormalizeFrameListConformsToCanvas(t *testing.T) {
	// The normalized list must conform to the logical canvas (filmstrip spec
	// "Frame list normalization"): fixed count, size, coordinates. Frame sizes
	// must equal the canvas unit size; frame positions must be inside the
	// filmstrip bounds.
	canvas, _ := identity.NewCanvasSpec(48, 64)
	fl, err := NormalizeFrameList(*canvas, 2)
	if err != nil {
		t.Fatalf("NormalizeFrameList: %v", err)
	}
	for _, f := range fl.Frames {
		if err := canvas.ValidateFrame(f.Width, f.Height); err != nil {
			t.Errorf("frame %d does not conform to canvas: %v", f.Index, err)
		}
		if f.X < 0 || f.Y < 0 || f.X+f.Width > fl.StripWidth || f.Y+f.Height > fl.StripHeight {
			t.Errorf("frame %d placement (%d,%d %dx%d) outside strip %dx%d", f.Index, f.X, f.Y, f.Width, f.Height, fl.StripWidth, fl.StripHeight)
		}
	}
}
