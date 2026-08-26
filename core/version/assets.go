package version

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/oframe/character-workbench/core/identity"
)

// AssetFrame is one accepted frame to copy into the current version's asset
// area (already losslessly encoded, so the engine asset equals the preview).
type AssetFrame struct {
	Index int
	PNG   []byte
}

// AssetRecord is one accepted animation asset entry of a version's asset area
// (task 9.2: 候选接受成为当前动画资产).
type AssetRecord struct {
	MotionID    string `json:"motionId,omitempty"`
	Direction   string `json:"direction"`
	CandidateID string `json:"candidateId"`
	AcceptedAt  string `json:"acceptedAt"`
	FrameCount  int    `json:"frameCount"`
}

// AssetsIndex lists the accepted assets of one identity version.
type AssetsIndex struct {
	Items []AssetRecord `json:"items"`
}

// CurrentAssetsIndex returns the current version's assets index.
func CurrentAssetsIndex(p *identity.Package) (AssetsIndex, error) {
	path, err := currentAssetsIndexPath(p)
	if err != nil {
		return AssetsIndex{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return AssetsIndex{Items: []AssetRecord{}}, nil
	}
	if err != nil {
		return AssetsIndex{}, err
	}
	var idx AssetsIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return AssetsIndex{}, fmt.Errorf("version: parse assets index: %w", err)
	}
	if idx.Items == nil {
		idx.Items = []AssetRecord{}
	}
	return idx, nil
}

// AcceptAssets makes an accepted candidate the CURRENT animation asset of a
// motion direction (task 9.2): its frames are copied losslessly into
// versions/<current>/assets/<motion>/<direction>/, the version's assets index
// is updated (replacing a previous asset of the same motion+direction), and an
// acceptance entry is appended to the operation log. The candidate's history
// record must already be marked accepted (RecordDecision).
func AcceptAssets(p *identity.Package, motionID, direction, candidateID string, frames []AssetFrame) error {
	assetsDir, err := currentAssetsDir(p)
	if err != nil {
		return err
	}
	motionDir := motionID
	if motionDir == "" {
		motionDir = "motion"
	}
	dir := filepath.Join(assetsDir, motionDir, direction)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("version: create asset dir: %w", err)
	}
	// Frames are replaced wholesale: remove stale frame files first so a
	// shorter accepted sequence never leaves orphan frames behind.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		_ = os.Remove(filepath.Join(dir, e.Name()))
	}
	for _, f := range frames {
		name := fmt.Sprintf("frame_%03d.png", f.Index)
		if err := os.WriteFile(filepath.Join(dir, name), f.PNG, 0o644); err != nil {
			return fmt.Errorf("version: write asset frame %s: %w", name, err)
		}
	}

	idx, err := CurrentAssetsIndex(p)
	if err != nil {
		return err
	}
	rec := AssetRecord{
		MotionID:    motionID,
		Direction:   direction,
		CandidateID: candidateID,
		AcceptedAt:  time.Now().UTC().Format(time.RFC3339),
		FrameCount:  len(frames),
	}
	replaced := false
	for i := range idx.Items {
		if idx.Items[i].MotionID == motionID && idx.Items[i].Direction == direction {
			idx.Items[i] = rec
			replaced = true
			break
		}
	}
	if !replaced {
		idx.Items = append(idx.Items, rec)
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("version: encode assets index: %w", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "index.json"), data, 0o644); err != nil {
		return fmt.Errorf("version: write assets index: %w", err)
	}

	if _, err := Append(p, ActionAcceptance, map[string]any{
		"motionId":    motionID,
		"direction":   direction,
		"candidateId": candidateID,
	}); err != nil {
		return err
	}
	return nil
}

// CurrentAssetsDir resolves versions/<current>/assets of the package for
// export and inspection callers.
func CurrentAssetsDir(p *identity.Package) (string, error) {
	return currentAssetsDir(p)
}

// currentAssetsDir resolves versions/<current>/assets of the package.
func currentAssetsDir(p *identity.Package) (string, error) {
	m := p.Manifest()
	for _, v := range m.Versions.Items {
		if v.ID == m.Versions.Current {
			if v.AssetsRef == "" {
				return "", fmt.Errorf("version: current version %s has no assets ref", v.ID)
			}
			return filepath.Join(p.Root(), filepath.FromSlash(v.AssetsRef)), nil
		}
	}
	return "", fmt.Errorf("version: current version %q not found", m.Versions.Current)
}

// currentAssetsIndexPath resolves versions/<current>/assets/index.json.
func currentAssetsIndexPath(p *identity.Package) (string, error) {
	dir, err := currentAssetsDir(p)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "index.json"), nil
}
