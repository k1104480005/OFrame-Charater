package identity

import "fmt"

// LogicalCanvas returns a copy of the logical canvas specification, or nil if
// the identity has not set one yet.
func (p *Package) LogicalCanvas() *CanvasSpec {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.manifest.LogicalCanvas == nil {
		return nil
	}
	c := *p.manifest.LogicalCanvas
	return &c
}

// SetLogicalCanvas sets the single logical canvas specification (unit size and
// coordinate range) shared by all motions and direction sets, and writes it to
// the manifest (task 2.4).
func (p *Package) SetLogicalCanvas(w, h int) error {
	c, err := NewCanvasSpec(w, h)
	if err != nil {
		return err
	}
	if err := c.Validate(); err != nil {
		return err
	}
	return p.Update(func(m *Manifest) error {
		m.LogicalCanvas = c
		return nil
	})
}

// ValidateFrame checks that a frame of the given pixel size conforms to the
// identity's logical canvas. This is the conformance check that motion and
// frame-sequence validation reference (task 2.4: 规格被后续动作/帧序列校验引用);
// later phases call it before a sequence may enter acceptance or export.
func (p *Package) ValidateFrame(w, h int) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	c := p.manifest.LogicalCanvas
	if c == nil {
		return fmt.Errorf("identity: logical canvas is not set")
	}
	return c.ValidateFrame(w, h)
}
