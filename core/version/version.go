// Package version implements identity version management: immutable identity
// versions formed after explicit appearance revisions, the current identity
// pointer, and preservation of older versions (design D9, task 9.1).
//
// The version records themselves live in the identity package manifest
// (core/identity); this package owns the operations over them. Later phases
// extend it with candidate acceptance, the append-only operation log, and
// rollback (tasks 9.2–9.4).
package version

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/oframe/character-workbench/core/identity"
)

// List returns all identity versions of the package in creation order. Older
// versions remain accessible (task 9.1: 新旧版本可并行访问).
func List(p *identity.Package) []identity.VersionRecord {
	m := p.Manifest()
	return append([]identity.VersionRecord(nil), m.Versions.Items...)
}

// Current returns the identity version that represents the current identity by
// default (task 9.1: 当前身份指向正确).
func Current(p *identity.Package) (identity.VersionRecord, error) {
	m := p.Manifest()
	for _, v := range m.Versions.Items {
		if v.ID == m.Versions.Current {
			return v, nil
		}
	}
	return identity.VersionRecord{}, fmt.Errorf("version: current version %q not found in package", m.Versions.Current)
}

// Get returns a specific identity version by ID; older versions remain
// accessible and preserved (task 9.1).
func Get(p *identity.Package, id string) (identity.VersionRecord, error) {
	m := p.Manifest()
	for _, v := range m.Versions.Items {
		if v.ID == id {
			return v, nil
		}
	}
	return identity.VersionRecord{}, fmt.Errorf("version: version %q not found", id)
}

// CommitAppearanceRevision forms an immutable identity version after an
// explicit appearance revision: the previous current version is sealed
// (immutable) and preserved, and a new version becomes the current identity
// (task 9.1). The revision reason is required so the history stays auditable.
func CommitAppearanceRevision(p *identity.Package, reason string) (identity.VersionRecord, error) {
	if strings.TrimSpace(reason) == "" {
		return identity.VersionRecord{}, fmt.Errorf("version: appearance revision reason is required")
	}
	var created identity.VersionRecord
	err := p.Update(func(m *identity.Manifest) error {
		sealed := false
		for i := range m.Versions.Items {
			if m.Versions.Items[i].ID == m.Versions.Current {
				m.Versions.Items[i].Immutable = true
				sealed = true
			}
		}
		if !sealed {
			return fmt.Errorf("version: current version %q not found; package versions are inconsistent", m.Versions.Current)
		}
		next := nextVersionID(m.Versions.Items)
		created = identity.VersionRecord{
			ID:        next,
			CreatedAt: time.Now().UTC(),
			Reason:    reason,
			Immutable: false,
			AssetsRef: identity.DirVersions + "/" + next + "/assets",
		}
		m.Versions.Items = append(m.Versions.Items, created)
		m.Versions.Current = created.ID
		return nil
	})
	if err != nil {
		return identity.VersionRecord{}, err
	}
	p.Logger().Info("identity appearance revision committed", "package", p.Root(), "version", created.ID, "reason", reason)
	return created, nil
}

// nextVersionID derives the next sequential version ID ("v1" → "v2").
func nextVersionID(items []identity.VersionRecord) string {
	max := 0
	for _, v := range items {
		if n, err := strconv.Atoi(strings.TrimPrefix(v.ID, "v")); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("v%d", max+1)
}
