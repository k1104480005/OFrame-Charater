package identity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Candidate acceptance statuses (task 8.3/8.4: 候选接受/拒绝写入候选历史).
const (
	CandidatePending  = "pending"  // 未验收 (not yet decided)
	CandidateAccepted = "accepted" // 验收通过 → 成为当前动画资产 (task 9.2)
	CandidateRejected = "rejected" // 验收未通过 (评分不达标 / 用户拒绝)
)

// CandidateHistoryRecord is one candidate's entry in the identity package
// candidate history (task 8.4: 未接受候选连同评分与验收结果写入身份包元数据;
// quality spec: scoring and acceptance results written into the identity
// package metadata). Scores are carried opaquely (the pipeline owns the
// QualityScores type); Overall is duplicated for cheap threshold checks.
type CandidateHistoryRecord struct {
	ID             string          `json:"id"`
	MotionID       string          `json:"motionId,omitempty"`
	Direction      string          `json:"direction,omitempty"`
	CreatedAt      string          `json:"createdAt"`
	Status         string          `json:"status"` // pending | accepted | rejected
	Overall        float64         `json:"overall"`
	Scores         json.RawMessage `json:"scores"` // QualityScores JSON
	AcceptanceAt   string          `json:"acceptanceAt,omitempty"`
	AcceptanceNote string          `json:"acceptanceNote,omitempty"`
	RegenerationOf string          `json:"regenerationOf,omitempty"`
}

// CandidateHistory is the persisted candidate history index of an identity
// package.
type CandidateHistory struct {
	Items []CandidateHistoryRecord `json:"items"`
}

// candidateHistoryPath resolves the candidate history index path of the
// package (the manifest declares it via References.CandidateHistory).
func (p *Package) candidateHistoryPath() string {
	ref := p.manifest.References.CandidateHistory
	if ref == "" {
		ref = DefaultReferences().CandidateHistory
	}
	return filepath.Join(p.root, filepath.FromSlash(ref))
}

// LoadCandidateHistory reads the candidate history index of the package. A
// missing index is an empty history (not an error).
func (p *Package) LoadCandidateHistory() (CandidateHistory, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	data, err := os.ReadFile(p.candidateHistoryPath())
	if os.IsNotExist(err) {
		return CandidateHistory{Items: []CandidateHistoryRecord{}}, nil
	}
	if err != nil {
		return CandidateHistory{}, fmt.Errorf("identity: read candidate history: %w", err)
	}
	var h CandidateHistory
	if err := json.Unmarshal(data, &h); err != nil {
		return CandidateHistory{}, fmt.Errorf("identity: parse candidate history %s: %w", p.candidateHistoryPath(), err)
	}
	if h.Items == nil {
		h.Items = []CandidateHistoryRecord{}
	}
	return h, nil
}

// SaveCandidateHistory persists the candidate history index atomically
// (task 8.4: 写入身份包元数据).
func (p *Package) SaveCandidateHistory(h CandidateHistory) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if h.Items == nil {
		h.Items = []CandidateHistoryRecord{}
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return fmt.Errorf("identity: encode candidate history: %w", err)
	}
	path := p.candidateHistoryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("identity: create candidate history dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("identity: write candidate history: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("identity: persist candidate history: %w", err)
	}
	return nil
}

// AppendCandidateHistory appends one record and persists the index.
func (p *Package) AppendCandidateHistory(rec CandidateHistoryRecord) error {
	h, err := p.LoadCandidateHistory()
	if err != nil {
		return err
	}
	for i := range h.Items {
		if h.Items[i].ID == rec.ID {
			h.Items[i] = rec // deterministic: replace by id
			return p.SaveCandidateHistory(h)
		}
	}
	h.Items = append(h.Items, rec)
	return p.SaveCandidateHistory(h)
}

// UpdateCandidateHistory mutates the record with the given id (via fn) and
// persists. Returns an error when the candidate is not in the history.
func (p *Package) UpdateCandidateHistory(id string, fn func(*CandidateHistoryRecord) error) error {
	h, err := p.LoadCandidateHistory()
	if err != nil {
		return err
	}
	for i := range h.Items {
		if h.Items[i].ID == id {
			if err := fn(&h.Items[i]); err != nil {
				return err
			}
			return p.SaveCandidateHistory(h)
		}
	}
	return fmt.Errorf("identity: candidate %q not in history", id)
}
