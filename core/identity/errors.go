package identity

import (
	"errors"
	"fmt"
)

// Sentinel errors for opening and validating identity packages (task 2.2).
var (
	// ErrManifestMissing is returned when a package directory has no manifest.
	ErrManifestMissing = errors.New("identity: manifest missing")
	// ErrManifestCorrupt is returned when the manifest cannot be parsed.
	ErrManifestCorrupt = errors.New("identity: manifest corrupt")
	// ErrFormatTooNew is returned when the manifest format version is newer
	// than the application supports; the package is refused and never modified.
	ErrFormatTooNew = errors.New("identity: manifest format version is newer than supported")
)

// FormatError wraps ErrFormatTooNew with the concrete versions so callers can
// render a migration hint (design D3: format version + migrators).
type FormatError struct {
	PackageVersion   int
	SupportedVersion int
}

func (e *FormatError) Error() string {
	return fmt.Sprintf("%s: package format v%d, application supports v%d (migration hint: upgrade the application or migrate the package)", ErrFormatTooNew, e.PackageVersion, e.SupportedVersion)
}

func (e *FormatError) Unwrap() error { return ErrFormatTooNew }
