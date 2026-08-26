// Typed errors returned by the Wails binding layer. Core errors (manifest
// missing / corrupt / format-too-new) flow through unchanged from core/identity
// so the launch page can render them verbatim.
package main

import (
	"errors"
	"fmt"
)

var (
	errNameRequired  = errors.New("identity package name is required")
	errPathRequired  = errors.New("identity package path is required")
	errNoPackageOpen = errors.New("no identity package is open")
)

func errUnknownPreset(id string) error {
	return fmt.Errorf("unknown anchor preset %q", id)
}

func errUnknownMaterialKind(kind string) error {
	return fmt.Errorf("unknown material kind %q", kind)
}
