package version

import (
	"encoding/json"
	"fmt"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/pipeline"
)

// AcceptanceThresholds is the score-threshold part of the quality acceptance
// gate (task 8.3: 评分达阈值且用户在 PixelPerfect 预览确认方通过). The second
// half — the user's confirmation in the preview — is enforced by RecordDecision.
type AcceptanceThresholds struct {
	// Overall is the minimum composite score (0..1) for a candidate to pass
	// the threshold gate.
	Overall float64 `json:"overall"`
}

// DefaultAcceptanceThresholds returns the built-in gate thresholds.
func DefaultAcceptanceThresholds() AcceptanceThresholds {
	return AcceptanceThresholds{Overall: 0.7}
}

// Passes reports whether the candidate's scores meet the thresholds, with the
// failing reasons (empty when it passes). Advisory-only metrics feed this
// gate; the user's preview confirmation is the authoritative second half.
func (t AcceptanceThresholds) Passes(s pipeline.QualityScores) (bool, []string) {
	var reasons []string
	if s.Overall < t.Overall {
		reasons = append(reasons, fmt.Sprintf("overall %.2f below threshold %.2f", s.Overall, t.Overall))
	}
	return len(reasons) == 0, reasons
}

// scoresOf decodes the pipeline scores stored on a candidate history record.
func scoresOf(rec identity.CandidateHistoryRecord) (pipeline.QualityScores, error) {
	var s pipeline.QualityScores
	if len(rec.Scores) == 0 {
		return s, fmt.Errorf("version: candidate %s has no scores recorded", rec.ID)
	}
	if err := json.Unmarshal(rec.Scores, &s); err != nil {
		return s, fmt.Errorf("version: candidate %s scores unreadable: %w", rec.ID, err)
	}
	return s, nil
}

// RecordDecision applies the FULL quality acceptance gate for a candidate
// (task 8.3): it passes only when the scores meet the thresholds AND the user
// confirmed the PixelPerfect preview. The decision (accepted / rejected) with
// the note is written into the candidate history (task 8.4). Rejected
// candidates belong only to candidate history and never become assets.
//
// Returns the decision status ("accepted" | "rejected").
func RecordDecision(p *identity.Package, candidateID string, confirm bool, note string, t AcceptanceThresholds) (string, error) {
	var scores pipeline.QualityScores
	var found bool
	if err := p.UpdateCandidateHistory(candidateID, func(r *identity.CandidateHistoryRecord) error {
		s, err := scoresOf(*r)
		if err != nil {
			return err
		}
		scores = s
		found = true
		return nil
	}); err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("version: candidate %q not in history", candidateID)
	}

	decision := identity.CandidateRejected
	switch {
	case !confirm:
		note = joinNote(note, "rejected by user in PixelPerfect preview")
	case scores.Overall < t.Overall:
		note = joinNote(note, fmt.Sprintf("scores below threshold (overall %.2f < %.2f)", scores.Overall, t.Overall))
	default:
		decision = identity.CandidateAccepted
	}
	if err := p.UpdateCandidateHistory(candidateID, func(r *identity.CandidateHistoryRecord) error {
		r.Status = decision
		r.AcceptanceNote = note
		return nil
	}); err != nil {
		return "", err
	}
	return decision, nil
}

func joinNote(note, extra string) string {
	if note == "" {
		return extra
	}
	return note + "; " + extra
}
