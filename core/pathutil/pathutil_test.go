package pathutil

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestNormalize(t *testing.T) {
	sep := string(filepath.Separator)
	if got := Normalize("a/b/c"); got != "a"+sep+"b"+sep+"c" {
		t.Errorf("Normalize(a/b/c) = %q", got)
	}
	if got := Normalize("a//b/./c"); got != "a"+sep+"b"+sep+"c" {
		t.Errorf("Normalize(a//b/./c) = %q", got)
	}
}

func TestToSlash(t *testing.T) {
	got := ToSlash(filepath.Join("a", "b"))
	if got != "a/b" {
		t.Errorf("ToSlash = %q, want a/b", got)
	}
}

func TestSamePath(t *testing.T) {
	base := filepath.Join("D:", "oframe", "pkg")
	upper := filepath.Join("d:", "OFRAME", "PKG")
	if IsWindows() {
		if !SamePath(base, upper) {
			t.Error("SamePath should be case-insensitive on Windows")
		}
	} else if SamePath(base, upper) {
		t.Error("SamePath should be case-sensitive off Windows")
	}
	if !SamePath(base, base) {
		t.Error("SamePath of identical paths should be true")
	}
}

func TestHasWindowsDrive(t *testing.T) {
	cases := map[string]bool{
		`C:\oframe\pkg`: true,
		`c:/oframe/pkg`: true,
		`\\srv\share\p`: true,
		`//srv/share/p`: true,
		`oframe/pkg`:    false,
		`/oframe/pkg`:   false,
	}
	for in, want := range cases {
		if got := HasWindowsDrive(in); got != want {
			t.Errorf("HasWindowsDrive(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestIsWithin(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "ws")
	child := filepath.Join(parent, "hero")
	ok, err := IsWithin(parent, child)
	if err != nil || !ok {
		t.Fatalf("IsWithin(child inside parent) = %v, %v", ok, err)
	}
	// Equal paths are within.
	ok, err = IsWithin(parent, parent)
	if err != nil || !ok {
		t.Fatalf("IsWithin(parent, parent) = %v, %v", ok, err)
	}
	// Sibling outside.
	outside := filepath.Join(filepath.Dir(parent), "other")
	ok, err = IsWithin(parent, outside)
	if err != nil || ok {
		t.Fatalf("IsWithin(sibling) = %v, %v; want false", ok, err)
	}
	// Traversal from a child.
	esc := filepath.Join(child, "..", "..", "evil")
	ok, err = IsWithin(parent, esc)
	if err != nil || ok {
		t.Fatalf("IsWithin(traversal) = %v, %v; want false", ok, err)
	}
	// Relative inputs rejected.
	if _, err = IsWithin("rel", "rel/sub"); err == nil {
		t.Fatal("IsWithin should reject relative paths")
	}
}

func TestSafeJoin(t *testing.T) {
	base := filepath.Join(t.TempDir(), "pkg")
	got, err := SafeJoin(base, "materials/abc.png")
	if err != nil {
		t.Fatalf("SafeJoin valid ref: %v", err)
	}
	if want := filepath.Join(base, "materials", "abc.png"); got != want {
		t.Errorf("SafeJoin = %q, want %q", got, want)
	}
	// Traversal refused.
	if _, err := SafeJoin(base, "../evil.png"); err == nil {
		t.Fatal("SafeJoin should refuse traversal")
	}
	// Absolute ref refused (not a relative reference).
	if _, err := SafeJoin(base, filepath.Join(t.TempDir(), "evil")); err == nil {
		t.Fatal("SafeJoin should refuse absolute refs escaping base")
	}
	_ = runtime.GOOS // keep import stable across GOOS
}
