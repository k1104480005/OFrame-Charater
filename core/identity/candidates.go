package identity

import (
	"fmt"
	"os"
	"path/filepath"
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

// normalizeAdoptedBasis enforces the "adoption is final" invariant when a
// package is opened: once an identity basis exists, every other candidate must
// be rejected. This repairs packages written before the rule (stale pending
// entries) so the UI never offers a second adoption beside an adopted basis.
// It only writes when there is something to normalize.
func (p *Package) normalizeAdoptedBasis() {
	if p.manifest.Identity.BaseCharacter == "" {
		return
	}
	hasPending := false
	for _, c := range p.manifest.BaseCharacters {
		if c.Status == BaseCharacterPending {
			hasPending = true
			break
		}
	}
	if !hasPending {
		return
	}
	if err := p.Update(func(m *Manifest) error {
		for i := range m.BaseCharacters {
			if m.BaseCharacters[i].Status == BaseCharacterPending {
				m.BaseCharacters[i].Status = BaseCharacterRejected
			}
		}
		return nil
	}); err != nil {
		p.log.Warn("identity base character normalization failed", "package", p.root, "err", err.Error())
		return
	}
	p.log.Info("identity base character candidates normalized", "package", p.root)
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
		// 采用即定稿：身份基准已存在时，后录入的候选直接记为 rejected，
		// 界面永远不会出现"已有基准却还能采用"的候选。
		if m.Identity.BaseCharacter != "" {
			candidate.Status = BaseCharacterRejected
		}
		m.BaseCharacters = append(m.BaseCharacters, candidate)
		return nil
	}); err != nil {
		return BaseCharacterCandidate{}, err
	}
	return candidate, nil
}

// RejectBaseCharacter marks a PENDING candidate as rejected so it can no
// longer be adopted (弃用). The adopted basis can never be rejected — that
// would leave the identity without a basis; re-rejecting is idempotent.
func (p *Package) RejectBaseCharacter(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("identity: base character candidate id is required")
	}
	return p.Update(func(m *Manifest) error {
		for i := range m.BaseCharacters {
			if m.BaseCharacters[i].ID == id {
				switch m.BaseCharacters[i].Status {
				case BaseCharacterAdopted:
					return fmt.Errorf("identity: base character candidate %q is the adopted basis and cannot be rejected", id)
				case BaseCharacterRejected:
					return nil
				default:
					m.BaseCharacters[i].Status = BaseCharacterRejected
					return nil
				}
			}
		}
		return fmt.Errorf("identity: base character candidate %q not found", id)
	})
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
			} else if m.BaseCharacters[i].Status == BaseCharacterPending || m.BaseCharacters[i].Status == BaseCharacterAdopted {
				// Adopting is final for this candidate set: all other candidates
				// become unavailable for adoption, including pending alternatives.
				m.BaseCharacters[i].Status = BaseCharacterRejected
			}
		}
		m.Identity.BaseCharacter = id
		return nil
	})
}

// DeleteBaseCharacterCandidate removes a NON-adopted candidate record and
// deletes its image file from the package candidate area. The adopted basis is
// protected: deleting it would leave the identity without a base character.
// Rejected candidates are deletable through the same path (cleanup), the UI
// simply does not offer the action. A missing image file is not an error —
// the manifest entry is the source of truth.
func (p *Package) DeleteBaseCharacterCandidate(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("identity: base character candidate id is required")
	}
	var rel string
	if err := p.Update(func(m *Manifest) error {
		for i, candidate := range m.BaseCharacters {
			if candidate.ID != id {
				continue
			}
			if candidate.Status == BaseCharacterAdopted {
				return fmt.Errorf("identity: base character candidate %q is the adopted basis and cannot be deleted", id)
			}
			rel = candidate.ImagePath
			m.BaseCharacters = append(m.BaseCharacters[:i], m.BaseCharacters[i+1:]...)
			return nil
		}
		return fmt.Errorf("identity: base character candidate %q not found", id)
	}); err != nil {
		return err
	}
	if rel != "" {
		if err := os.Remove(filepath.Join(p.root, filepath.FromSlash(rel))); err != nil && !os.IsNotExist(err) {
			// 清单已更新；文件删除失败不回滚 —— 记录已不再被引用。
			p.log.Warn("identity base character file remove failed", "path", rel, "err", err.Error())
		}
	}
	p.log.Info("identity base character candidate deleted", "package", p.root, "id", id)
	return nil
}
