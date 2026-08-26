package version

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/motion"
)

// Rollback restores the identity package content to the state at the
// historical point seq (task 9.4: 回退到任一历史点后身份包内容恢复该点状态、后续
// 日志保留): the snapshot recorded by the log entry at seq is applied
// (motions.json, the current version's assets index, the candidate history
// index, and the manifest's current version pointer). Later log entries are
// never deleted — a rollback entry is APPENDED so the append-only audit trail
// stays intact.
func Rollback(p *identity.Package, seq int) error {
	entries, err := Entries(p)
	if err != nil {
		return err
	}
	var target *LogEntry
	for i := range entries {
		if entries[i].Seq == seq {
			target = &entries[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("version: no log entry at seq %d (log has %d entries)", seq, len(entries))
	}
	lastSeq := 0
	if len(entries) > 0 {
		lastSeq = entries[len(entries)-1].Seq
	}
	if err := applySnapshot(p, target.Snapshot); err != nil {
		return err
	}
	if _, err := Append(p, ActionRollback, map[string]any{"fromSeq": lastSeq, "toSeq": seq}); err != nil {
		return err
	}
	return nil
}

// applySnapshot writes the recorded content state back into the package
// (motions.json, assets index, candidate history, manifest current version).
func applySnapshot(p *identity.Package, snap StateSnapshot) error {
	// motions.json
	motionsPath := motion.NewStore(p.Root()).Path()
	if snap.Motions != "" {
		if err := os.MkdirAll(filepath.Dir(motionsPath), 0o755); err != nil {
			return fmt.Errorf("version: rollback: create motions dir: %w", err)
		}
		if err := os.WriteFile(motionsPath, []byte(snap.Motions), 0o644); err != nil {
			return fmt.Errorf("version: rollback: restore motions: %w", err)
		}
	} else if _, err := os.Stat(motionsPath); err == nil {
		_ = os.Remove(motionsPath)
	}

	// Candidate history index.
	var history identity.CandidateHistory
	if snap.CandidateHistory != "" {
		if err := json.Unmarshal([]byte(snap.CandidateHistory), &history); err != nil {
			return fmt.Errorf("version: rollback: parse candidate history snapshot: %w", err)
		}
	}
	if err := p.SaveCandidateHistory(history); err != nil {
		return fmt.Errorf("version: rollback: restore candidate history: %w", err)
	}

	// Current version + its assets index.
	if err := p.Update(func(m *identity.Manifest) error {
		if snap.CurrentVersion != "" {
			m.Versions.Current = snap.CurrentVersion
		}
		return nil
	}); err != nil {
		return fmt.Errorf("version: rollback: restore current version: %w", err)
	}
	if snap.CurrentVersion != "" {
		for _, v := range p.Manifest().Versions.Items {
			if v.ID == snap.CurrentVersion {
				idxPath := filepath.Join(p.Root(), filepath.FromSlash(v.AssetsRef), "index.json")
				if snap.AssetsIndex != "" {
					if err := os.MkdirAll(filepath.Dir(idxPath), 0o755); err != nil {
						return err
					}
					if err := os.WriteFile(idxPath, []byte(snap.AssetsIndex), 0o644); err != nil {
						return fmt.Errorf("version: rollback: restore assets index: %w", err)
					}
				} else if _, err := os.Stat(idxPath); err == nil {
					_ = os.Remove(idxPath)
				}
				break
			}
		}
	}
	return nil
}
