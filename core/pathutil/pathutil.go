// Package pathutil provides cross-platform path helpers with Windows-first
// semantics: native separators, case-insensitive comparison on Windows
// (NTFS default), and safe containment checks that prevent path traversal when
// resolving manifest-relative references into a package directory.
package pathutil

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// IsWindows reports whether the current runtime is Windows.
func IsWindows() bool { return runtime.GOOS == "windows" }

// Clean normalizes a path: cleans redundant elements and converts to native
// separators.
func Clean(p string) string { return filepath.Clean(p) }

// Normalize converts forward slashes to native separators and cleans the path.
func Normalize(p string) string { return filepath.Clean(filepath.FromSlash(p)) }

// ToSlash converts a path to forward slashes (used for manifest-relative
// references so they stay portable across platforms).
func ToSlash(p string) string { return filepath.ToSlash(p) }

// IsAbsolute wraps filepath.IsAbs.
func IsAbsolute(p string) bool { return filepath.IsAbs(p) }

// SamePath reports whether two cleaned paths refer to the same location,
// comparing case-insensitively on Windows (NTFS is case-insensitive by
// default) and case-sensitively elsewhere.
func SamePath(a, b string) bool {
	a, b = Clean(a), Clean(b)
	if IsWindows() {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// HasWindowsDrive reports whether the path starts with a drive letter (C:) or
// a UNC server prefix — i.e. it is rooted in Windows terms.
func HasWindowsDrive(p string) bool {
	if len(p) >= 2 && p[1] == ':' {
		return true
	}
	return strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//")
}

// IsWithin reports whether child is located inside parent (or equals parent).
// Both paths must be absolute; relative inputs are rejected so a caller cannot
// accidentally bypass containment checks.
func IsWithin(parent, child string) (bool, error) {
	parent, child = Clean(parent), Clean(child)
	if !IsAbsolute(parent) || !IsAbsolute(child) {
		return false, fmt.Errorf("pathutil: absolute paths required (parent=%q child=%q)", parent, child)
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false, err
	}
	if rel == "." {
		return true, nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}

// SafeJoin joins base with a relative reference and refuses references that
// escape base (path traversal) or that are absolute (manifest references must
// stay relative so packages remain portable). ref may use forward or native
// separators.
func SafeJoin(base, ref string) (string, error) {
	if IsAbsolute(ref) {
		return "", fmt.Errorf("pathutil: reference %q must be relative", ref)
	}
	joined := filepath.Join(base, filepath.FromSlash(ref))
	ok, err := IsWithin(base, joined)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("pathutil: reference %q escapes base %q", ref, base)
	}
	return joined, nil
}
