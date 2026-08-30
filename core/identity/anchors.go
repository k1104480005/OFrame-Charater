package identity

import (
	"fmt"
	"strings"
)

// AnchorPreset is a named, reusable anchor definition at the identity level
// (task 2.5: 锚点定义与预设, 脚底/手持点等).
type AnchorPreset struct {
	ID   string
	Name string
}

// Built-in anchor presets.
var (
	PresetFeet      = AnchorPreset{ID: "feet", Name: "脚底"}
	PresetHandLeft  = AnchorPreset{ID: "hand_left", Name: "左侧挂载参考点"}
	PresetHandRight = AnchorPreset{ID: "hand_right", Name: "右侧挂载参考点"}
	PresetCenter       = AnchorPreset{ID: "center", Name: "画布中心"}
	PresetHeadTop      = AnchorPreset{ID: "head_top", Name: "头顶挂载参考点"}
	PresetWeaponLeft   = AnchorPreset{ID: "weapon_left", Name: "左侧武器/特效参考点"}
	PresetWeaponRight  = AnchorPreset{ID: "weapon_right", Name: "右侧武器/特效参考点"}
)

// AnchorPresets returns the built-in presets in definition order.
func AnchorPresets() []AnchorPreset {
	return []AnchorPreset{
		PresetFeet, PresetCenter, PresetHandLeft, PresetHandRight,
		PresetHeadTop, PresetWeaponLeft, PresetWeaponRight,
	}
}

// DefaultAnchorPosition computes a useful starting position for a preset.
// These are editable runtime markers, not an automatic pose detector.
func DefaultAnchorPosition(p AnchorPreset, c *CanvasSpec) (x, y int) {
	w, h := c.UnitWidth, c.UnitHeight
	switch p.ID {
	case PresetFeet.ID:
		return w / 2, h - 1
	case PresetHandLeft.ID:
		return w / 3, h / 2
	case PresetHandRight.ID:
		return w * 2 / 3, h / 2
	case PresetHeadTop.ID:
		return w / 2, h / 8
	case PresetWeaponLeft.ID:
		return w / 3, h / 2
	case PresetWeaponRight.ID:
		return w * 2 / 3, h / 2
	default:
		return w / 2, h / 2
	}
}

// AddAnchorPreset defines a new anchor at the preset's default position on the
// identity's logical canvas (task 2.5: 预设锚点写入 manifest).
func (p *Package) AddAnchorPreset(preset AnchorPreset, name string) (*Anchor, error) {
	c := p.LogicalCanvas()
	if c == nil {
		return nil, fmt.Errorf("identity: logical canvas must be set before defining anchors")
	}
	x, y := DefaultAnchorPosition(preset, c)
	if preset.ID == PresetFeet.ID && p.PerfectPixelStandard() {
		y = c.UnitHeight - 24
		if y < 0 {
			y = 0
		}
	}
	return p.addAnchor(name, preset, x, y)
}

// AddAnchor defines a new anchor at explicit coordinates, validated against the
// logical canvas coordinate range (task 2.5).
func (p *Package) AddAnchor(name string, preset AnchorPreset, x, y int) (*Anchor, error) {
	return p.addAnchor(name, preset, x, y)
}

func (p *Package) addAnchor(name string, preset AnchorPreset, x, y int) (*Anchor, error) {
	if strings.TrimSpace(name) == "" {
		name = preset.Name
	}
	if strings.TrimSpace(preset.ID) == "" {
		preset = AnchorPreset{ID: "custom", Name: name}
	}
	c := p.LogicalCanvas()
	if c == nil {
		return nil, fmt.Errorf("identity: logical canvas must be set before defining anchors")
	}
	if !c.Contains(x, y) {
		return nil, fmt.Errorf("identity: anchor (%d,%d) is outside logical canvas range %v", x, y, c.CoordinateRange)
	}
	a := &Anchor{
		ID:              mustID(),
		Name:            name,
		Preset:          preset.ID,
		X:               x,
		Y:               y,
		CoordinateRange: c.CoordinateRange,
	}
	if err := p.Update(func(m *Manifest) error {
		m.Anchors = append(m.Anchors, *a)
		return nil
	}); err != nil {
		return nil, err
	}
	p.log.Info("identity anchor defined", "package", p.root, "id", a.ID, "name", a.Name, "x", a.X, "y", a.Y)
	return a, nil
}

// DeleteAnchor removes a custom runtime anchor by ID.
func (p *Package) DeleteAnchor(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("identity: anchor id is required")
	}
	return p.Update(func(m *Manifest) error {
		for i, anchor := range m.Anchors {
			if anchor.ID != id {
				continue
			}
			m.Anchors = append(m.Anchors[:i], m.Anchors[i+1:]...)
			return nil
		}
		return fmt.Errorf("identity: anchor %q not found", id)
	})
}

// currentBaseCharacterID returns the adopted base-character candidate ID.
func (p *Package) currentBaseCharacterID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return strings.TrimSpace(p.manifest.Identity.BaseCharacter)
}

// Anchors returns a copy of the identity-level anchor definitions.
func (p *Package) Anchors() []Anchor {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]Anchor(nil), p.manifest.Anchors...)
}

// Anchor returns an anchor by id (the reference target for motions and
// direction sets in later phases).
func (p *Package) Anchor(id string) (Anchor, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, a := range p.manifest.Anchors {
		if a.ID == id {
			return a, nil
		}
	}
	return Anchor{}, fmt.Errorf("identity: anchor %q not found", id)
}
