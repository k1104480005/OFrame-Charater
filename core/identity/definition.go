package identity

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oframe/character-workbench/core/pathutil"
)

// Description returns the recorded text description of the identity.
func (p *Package) Description() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.manifest.Identity.Description
}

// SetTextDescription records a text description as the identity definition
// entry (task 2.3 入口 1: 文字描述写入身份包元数据).
func (p *Package) SetTextDescription(text string) error {
	return p.Update(func(m *Manifest) error {
		m.Identity.Description = text
		m.Identity.EntryKind = EntryKindText
		m.Identity.EntryMaterialID = ""
		return nil
	})
}

// SetName updates the identity display name. The name is display-only: it
// lives in the manifest and never participates in file paths (the package
// directory is the storage path and stays unchanged).
func (p *Package) SetName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("identity: package name is required")
	}
	if len(name) > 64 {
		return fmt.Errorf("identity: package name too long (max 64 chars)")
	}
	return p.Update(func(m *Manifest) error {
		m.Identity.Name = name
		return nil
	})
}

// SetCategory updates the launch-page category of the identity package.
// Empty category means "uncategorized"; a blank input clears it.
func (p *Package) SetCategory(category string) error {
	category = strings.TrimSpace(category)
	if len(category) > 32 {
		return fmt.Errorf("identity: category too long (max 32 chars)")
	}
	return p.Update(func(m *Manifest) error {
		m.Identity.Category = category
		return nil
	})
}

// AddReferenceImage stores a reference image into the package material area and
// references it from the manifest (task 2.3 入口 2: 参考图 → 素材区 + 引用).
// role is RoleMainReference or RoleAuxiliaryReference and is validated against
// the reference-image semantics (1 主参考图 + 最多 2 辅助参考图). When no
// identity entry exists yet, the material becomes the identity entry basis.
func (p *Package) AddReferenceImage(src, name string, role MaterialRole) (*Material, error) {
	return p.addMaterial(MaterialKindReferenceImage, src, name, role)
}

// ImportSprite stores an existing sprite as material and uses it as the basis
// for the identity definition (task 2.3 入口 3: 既有精灵 → 素材区 + 身份定义基础).
// Sprites always carry the sprite role.
func (p *Package) ImportSprite(src, name string) (*Material, error) {
	return p.addMaterial(MaterialKindSprite, src, name, RoleSprite)
}

// RemoveMaterial deletes a material from the manifest and removes its stored
// file inside the package materials area. If the material was the identity
// entry basis, the entry reference is cleared (the text entry can be set again).
func (p *Package) RemoveMaterial(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("identity: material id is required")
	}
	var rel string
	if err := p.Update(func(m *Manifest) error {
		for i, mat := range m.Materials {
			if mat.ID != id {
				continue
			}
			rel = mat.Path
			m.Materials = append(m.Materials[:i], m.Materials[i+1:]...)
			if m.Identity.EntryMaterialID == id {
				m.Identity.EntryMaterialID = ""
				if m.Identity.EntryKind == EntryKindReferenceImage || m.Identity.EntryKind == EntryKindSprite {
					m.Identity.EntryKind = ""
				}
			}
			return nil
		}
		return fmt.Errorf("identity: material %q not found", id)
	}); err != nil {
		return err
	}
	if rel != "" {
		if err := os.Remove(filepath.Join(p.root, filepath.FromSlash(rel))); err != nil && !os.IsNotExist(err) {
			// 清单已更新；文件删除失败不回滚 —— 素材已不再被引用。
			p.log.Warn("identity material file remove failed", "path", rel, "err", err.Error())
		}
	}
	p.log.Info("identity material removed", "package", p.root, "id", id)
	return nil
}

// SetMaterialRole re-assigns the role of an existing reference-image material
// (e.g. promoting an auxiliary reference to the main reference), enforcing the
// 1 主参考图 + 最多 2 辅助参考图 bounds. Sprite materials cannot change role.
func (p *Package) SetMaterialRole(id string, role MaterialRole) (*Material, error) {
	if role != RoleMainReference && role != RoleAuxiliaryReference {
		return nil, fmt.Errorf("identity: invalid reference role %q", role)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	idx := -1
	for i := range p.manifest.Materials {
		if p.manifest.Materials[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("identity: material %q not found", id)
	}
	if p.manifest.Materials[idx].Kind != MaterialKindReferenceImage {
		return nil, fmt.Errorf("identity: material %q is %s and cannot carry a reference role", id, p.manifest.Materials[idx].Kind)
	}
	m := p.manifest
	// Validate against the full set with the target's new role applied, so a
	// promotion to main fails while another main exists (2 mains are never
	// allowed).
	snap := make([]Material, len(m.Materials))
	copy(snap, m.Materials)
	snap[idx].Role = role
	if err := checkReferenceRoles(snap); err != nil {
		return nil, err
	}
	m.Materials[idx].Role = role
	m.Identity.UpdatedAt = time.Now().UTC()
	p.manifest = m
	if err := p.saveLocked(); err != nil {
		return nil, err
	}
	p.log.Info("identity material role set", "package", p.root, "id", id, "role", role)
	mat := m.Materials[idx]
	return &mat, nil
}

// SwapMainReference promotes an auxiliary reference image to the main
// reference in one transaction: the current main (if any) is demoted to
// auxiliary, so the 1 主参考图 + 最多 2 辅助参考图 bounds always hold.
func (p *Package) SwapMainReference(auxiliaryID string) (*Material, error) {
	auxiliaryID = strings.TrimSpace(auxiliaryID)
	if auxiliaryID == "" {
		return nil, fmt.Errorf("identity: material id is required")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	idx := -1
	for i := range p.manifest.Materials {
		if p.manifest.Materials[i].ID == auxiliaryID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("identity: material %q not found", auxiliaryID)
	}
	if p.manifest.Materials[idx].Kind != MaterialKindReferenceImage {
		return nil, fmt.Errorf("identity: material %q is %s and cannot carry a reference role", auxiliaryID, p.manifest.Materials[idx].Kind)
	}
	if p.manifest.Materials[idx].Role != RoleAuxiliaryReference {
		return nil, fmt.Errorf("identity: material %q is not an auxiliary reference", auxiliaryID)
	}
	m := p.manifest
	for i := range m.Materials {
		if i == idx {
			m.Materials[i].Role = RoleMainReference
		} else if m.Materials[i].Role == RoleMainReference {
			m.Materials[i].Role = RoleAuxiliaryReference
		}
	}
	if err := checkReferenceRoles(m.Materials); err != nil {
		return nil, err
	}
	m.Identity.UpdatedAt = time.Now().UTC()
	p.manifest = m
	if err := p.saveLocked(); err != nil {
		return nil, err
	}
	p.log.Info("identity main reference swapped", "package", p.root, "id", auxiliaryID)
	mat := m.Materials[idx]
	return &mat, nil
}

// ReferenceImages returns the reference-image materials ordered by role:
// main reference first, then auxiliary references in insertion order.
func (p *Package) ReferenceImages() []Material {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]Material, 0, len(p.manifest.Materials))
	for _, m := range p.manifest.Materials {
		if m.Kind == MaterialKindReferenceImage {
			out = append(out, m)
		}
	}
	stable := func(role string) []Material {
		var r []Material
		for _, m := range out {
			if m.Role == role {
				r = append(r, m)
			}
		}
		return r
	}
	main := stable(RoleMainReference)
	aux := stable(RoleAuxiliaryReference)
	// Unassigned reference images (role empty, from older packages) sort last.
	rest := stable("")
	return append(append(append([]Material(nil), main...), aux...), rest...)
}

// ValidateReferenceRoles checks the 1 主参考图 + 最多 2 辅助参考图 bounds across
// the package's reference-image materials.
func (p *Package) ValidateReferenceRoles() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return checkReferenceRoles(p.manifest.Materials)
}

// checkReferenceRoles enforces the reference-role bounds. Caller must hold the
// manifest lock.
func checkReferenceRoles(materials []Material) error {
	main, aux := 0, 0
	for _, m := range materials {
		if m.Kind != MaterialKindReferenceImage {
			continue
		}
		switch m.Role {
		case RoleMainReference:
			main++
		case RoleAuxiliaryReference:
			aux++
		case "":
			// unassigned — ignored by the bounds
		default:
			return fmt.Errorf("identity: material %q has invalid reference role %q", m.ID, m.Role)
		}
	}
	if main > MaxMainReferences {
		return fmt.Errorf("identity: at most %d main reference image allowed, found %d", MaxMainReferences, main)
	}
	if aux > MaxAuxiliaryReferences {
		return fmt.Errorf("identity: at most %d auxiliary reference images allowed, found %d", MaxAuxiliaryReferences, aux)
	}
	return nil
}

// addMaterial copies src into the package material area and appends a
// reference. On any failure no partial state is left behind: a failed manifest
// write removes the copied file.
func (p *Package) addMaterial(kind, src, name string, role MaterialRole) (*Material, error) {
	if kind != MaterialKindReferenceImage && kind != MaterialKindSprite {
		return nil, fmt.Errorf("identity: unknown material kind %q", kind)
	}
	if kind == MaterialKindReferenceImage && role != RoleMainReference && role != RoleAuxiliaryReference {
		return nil, fmt.Errorf("identity: invalid reference role %q", role)
	}
	src = pathutil.Normalize(src)
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("identity: cannot read source %q: %w", src, err)
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(src)
	}
	ext := filepath.Ext(src)
	id := mustID()
	rel := filepath.ToSlash(filepath.Join(DirMaterials, id+ext))

	p.mu.Lock()
	defer p.mu.Unlock()

	dest, err := pathutil.SafeJoin(p.root, rel)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return nil, fmt.Errorf("identity: store material: %w", err)
	}
	mat := Material{ID: id, Kind: kind, Role: role, Name: name, Path: rel, AddedAt: time.Now().UTC()}
	m := p.manifest
	m.Materials = append(m.Materials, mat)
	if err := checkReferenceRoles(m.Materials); err != nil {
		_ = os.Remove(dest)
		return nil, err
	}
	if m.Identity.EntryKind == "" {
		m.Identity.EntryKind = kind
		m.Identity.EntryMaterialID = id
	}
	m.Identity.UpdatedAt = time.Now().UTC()
	p.manifest = m
	if err := p.saveLocked(); err != nil {
		_ = os.Remove(dest)
		return nil, err
	}
	p.log.Info("identity material added", "package", p.root, "kind", kind, "id", id, "path", rel, "role", role)
	return &mat, nil
}

// Materials returns a copy of the material references in insertion order.
func (p *Package) Materials() []Material {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]Material(nil), p.manifest.Materials...)
}

// Material returns a material reference by id.
func (p *Package) Material(id string) (Material, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, m := range p.manifest.Materials {
		if m.ID == id {
			return m, nil
		}
	}
	return Material{}, fmt.Errorf("identity: material %q not found", id)
}

// MaterialPath resolves a material's relative reference to an absolute path
// inside the package, refusing references that escape the package root.
func (p *Package) MaterialPath(m Material) (string, error) {
	return pathutil.SafeJoin(p.root, m.Path)
}
