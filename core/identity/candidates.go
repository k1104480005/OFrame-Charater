package identity

import (
	"fmt"
	"strings"
	"time"
)

// BaseCharacterCandidate statuses.
const (
	BaseCharacterPending  = "pending"  // recorded, not yet the identity basis
	BaseCharacterAdopted  = "adopted"  // the identity's current base character
	BaseCharacterRejected = "rejected" // superseded by a newer adoption

	BaseCharacterSourceAI     = "ai"
	BaseCharacterSourceImport = "import"
)

func candidateSource(candidate BaseCharacterCandidate) string {
	if candidate.Source == BaseCharacterSourceImport || candidate.Provider == BaseCharacterSourceImport {
		return BaseCharacterSourceImport
	}
	return BaseCharacterSourceAI
}

// BaseCharacterSource returns the immutable source selection. Legacy packages
// without the field are inferred from their adopted candidate when possible.
func (p *Package) BaseCharacterSource() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.manifest.Identity.BaseCharacterSource != "" {
		return p.manifest.Identity.BaseCharacterSource
	}
	if p.manifest.Identity.BaseCharacter != "" {
		for _, candidate := range p.manifest.BaseCharacters {
			if candidate.ID == p.manifest.Identity.BaseCharacter {
				return candidateSource(candidate)
			}
		}
	}
	return ""
}

// LockBaseCharacterSource selects the only permitted source for this identity.
// The lock is intentionally separate from candidate adoption: choosing a path
// is irreversible even if its first generation/import later fails or is
// cancelled, matching the identity-package contract.
func (p *Package) LockBaseCharacterSource(source string) error {
	if source != BaseCharacterSourceAI && source != BaseCharacterSourceImport {
		return fmt.Errorf("identity: invalid base character source %q", source)
	}
	return p.Update(func(m *Manifest) error {
		current := m.Identity.BaseCharacterSource
		if current == "" && m.Identity.BaseCharacter != "" {
			for _, candidate := range m.BaseCharacters {
				if candidate.ID == m.Identity.BaseCharacter {
					if candidate.Provider == BaseCharacterSourceImport {
						current = BaseCharacterSourceImport
					} else {
						current = BaseCharacterSourceAI
					}
					break
				}
			}
		}
		if current != "" && current != source {
			return fmt.Errorf("identity: base character source is locked to %q and cannot switch to %q", current, source)
		}
		m.Identity.BaseCharacterSource = source
		return nil
	})
}

// BaseCharacterCandidate records a generated or imported identity basis.
// ImagePath is package-relative so packages remain portable.
type BaseCharacterCandidate struct {
	ID        string    `json:"id"`
	ImagePath string    `json:"imagePath"`
	Provider  string    `json:"provider,omitempty"`
	Model     string    `json:"model,omitempty"`
	Prompt    string    `json:"prompt,omitempty"`
	Source    string    `json:"source,omitempty"` // ai | import; inferred for legacy records
	Status    string    `json:"status"`           // pending | adopted | rejected
	CreatedAt time.Time `json:"createdAt"`
}

// BaseCharacterCandidates returns a copy of the recorded candidate history.
func (p *Package) BaseCharacterCandidates() []BaseCharacterCandidate {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]BaseCharacterCandidate(nil), p.manifest.BaseCharacters...)
}

// AddBaseCharacterCandidate keeps the legacy API and records an AI candidate.
func (p *Package) AddBaseCharacterCandidate(imagePath, providerID, model, prompt string) (BaseCharacterCandidate, error) {
	return p.AddBaseCharacterCandidateFromSource(imagePath, providerID, model, prompt, BaseCharacterSourceAI)
}

// AddBaseCharacterCandidateFromSource records a candidate without changing the
// current identity, while retaining the source explicitly for future checks.
func (p *Package) AddBaseCharacterCandidateFromSource(imagePath, providerID, model, prompt, source string) (BaseCharacterCandidate, error) {
	if strings.TrimSpace(imagePath) == "" {
		return BaseCharacterCandidate{}, fmt.Errorf("identity: base character image path is required")
	}
	if source != BaseCharacterSourceAI && source != BaseCharacterSourceImport {
		return BaseCharacterCandidate{}, fmt.Errorf("identity: invalid base character source %q", source)
	}
	candidate := BaseCharacterCandidate{
		ID: mustID(), ImagePath: imagePath, Provider: providerID, Model: model,
		Prompt: prompt, Source: source, Status: BaseCharacterPending, CreatedAt: time.Now().UTC(),
	}
	if err := p.Update(func(m *Manifest) error {
		m.BaseCharacters = append(m.BaseCharacters, candidate)
		return nil
	}); err != nil {
		return BaseCharacterCandidate{}, err
	}
	return candidate, nil
}

// AdoptBaseCharacter marks one candidate as the identity's current basis.
func (p *Package) AdoptBaseCharacter(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("identity: base character candidate id is required")
	}
	return p.Update(func(m *Manifest) error {
		found := false
		candidateSourceValue := BaseCharacterSourceAI
		for i := range m.BaseCharacters {
			if m.BaseCharacters[i].ID == id {
				found = true
				candidateSourceValue = candidateSource(m.BaseCharacters[i])
				if m.BaseCharacters[i].Status == BaseCharacterRejected {
					return fmt.Errorf("identity: base character candidate %q is rejected and cannot be adopted", id)
				}
			}
		}
		if !found {
			return fmt.Errorf("identity: base character candidate %q not found", id)
		}
		current := m.Identity.BaseCharacterSource
		if current != "" && current != candidateSourceValue {
			return fmt.Errorf("identity: candidate source %q conflicts with locked source %q", candidateSourceValue, current)
		}
		m.Identity.BaseCharacterSource = candidateSourceValue
		for i := range m.BaseCharacters {
			if m.BaseCharacters[i].ID == id {
				m.BaseCharacters[i].Status = BaseCharacterAdopted
			} else if m.BaseCharacters[i].Status == BaseCharacterAdopted {
				m.BaseCharacters[i].Status = BaseCharacterRejected
			}
		}
		m.Identity.BaseCharacter = id
		return nil
	})
}
