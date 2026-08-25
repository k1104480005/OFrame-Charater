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

// AddReferenceImage stores a reference image into the package material area and
// references it from the manifest (task 2.3 入口 2: 参考图 → 素材区 + 引用). When
// no identity entry exists yet, the material becomes the identity entry basis.
func (p *Package) AddReferenceImage(src, name string) (*Material, error) {
	return p.addMaterial(MaterialKindReferenceImage, src, name)
}

// ImportSprite stores an existing sprite as material and uses it as the basis
// for the identity definition (task 2.3 入口 3: 既有精灵 → 素材区 + 身份定义基础).
func (p *Package) ImportSprite(src, name string) (*Material, error) {
	return p.addMaterial(MaterialKindSprite, src, name)
}

// addMaterial copies src into the package material area and appends a
// reference. On any failure no partial state is left behind: a failed manifest
// write removes the copied file.
func (p *Package) addMaterial(kind, src, name string) (*Material, error) {
	if kind != MaterialKindReferenceImage && kind != MaterialKindSprite {
		return nil, fmt.Errorf("identity: unknown material kind %q", kind)
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
	mat := Material{ID: id, Kind: kind, Name: name, Path: rel, AddedAt: time.Now().UTC()}
	m := p.manifest
	m.Materials = append(m.Materials, mat)
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
	p.log.Info("identity material added", "package", p.root, "kind", kind, "id", id, "path", rel)
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
