package service

import (
	"fmt"
	"os"
	"path/filepath"

	assetexport "github.com/oframe/character-workbench/core/assetexport"
	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/motion"
	"github.com/oframe/character-workbench/core/pipeline"
	"github.com/oframe/character-workbench/core/version"
)

// ExportPackage builds a validated export package from accepted assets of the
// selected identity version. An empty versionID uses the current version.
func (s *Service) ExportPackage(pkgPath, outputDir, target, versionID string) (*assetexport.Result, error) {
	pkg, err := identity.Open(pkgPath)
	if err != nil {
		return nil, err
	}
	m := pkg.Manifest()
	if versionID == "" {
		versionID = m.Versions.Current
	}
	if versionID != m.Versions.Current {
		return nil, fmt.Errorf("service: only the current identity version is exportable")
	}
	assets, err := version.CurrentAssetsIndex(pkg)
	if err != nil {
		return nil, err
	}
	motions, err := motion.NewStore(pkgPath).Load()
	if err != nil {
		return nil, err
	}
	assetsDir, err := version.CurrentAssetsDir(pkg)
	if err != nil {
		return nil, err
	}
	animations := make([]assetexport.Animation, 0, len(assets.Items))
	for _, accepted := range assets.Items {
		motionModel, err := motions.Get(accepted.MotionID)
		if err != nil {
			return nil, fmt.Errorf("service: accepted asset references missing motion %q: %w", accepted.MotionID, err)
		}
		direction := motionModel.Direction(accepted.Direction)
		if direction == nil {
			return nil, fmt.Errorf("service: accepted asset references missing direction %q", accepted.Direction)
		}
		frames := make([]assetexport.Frame, 0, len(direction.Sequence.Frames))
		for _, modelFrame := range direction.Sequence.Frames {
			path := filepath.Join(assetsDir, accepted.MotionID, accepted.Direction, fmt.Sprintf("frame_%03d.png", modelFrame.Index))
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("service: read accepted frame %s: %w", path, err)
			}
			anchors := make([]assetexport.Anchor, 0, len(modelFrame.Anchors))
			for _, anchor := range modelFrame.Anchors {
				anchors = append(anchors, assetexport.Anchor{ID: anchor.Name, X: anchor.X, Y: anchor.Y})
			}
			frames = append(frames, assetexport.Frame{Index: modelFrame.Index, DurationMs: modelFrame.DurationMs, Anchors: anchors, PNG: data})
		}
		animations = append(animations, assetexport.Animation{MotionID: accepted.MotionID, Direction: accepted.Direction, CandidateID: accepted.CandidateID, Frames: frames})
	}
	assetexport.SortAnimations(animations)
	result, err := assetexport.Build(outputDir, target, versionID, animations)
	record := assetexport.HistoryRecord{Target: target, IdentityVersion: versionID, OutputDir: outputDir, Result: "succeeded"}
	if err != nil {
		record.Result = "failed"
		record.Error = err.Error()
	}
	if historyErr := assetexport.RecordHistory(filepath.Join(pkgPath, "exports", "history.jsonl"), record); historyErr != nil && err == nil {
		return nil, historyErr
	}
	return result, err
}

func (s *Service) ExportHistory(pkgPath string) ([]assetexport.HistoryRecord, error) {
	return assetexport.ReadHistory(filepath.Join(pkgPath, "exports", "history.jsonl"))
}

func (s *Service) ValidateExport(outputDir string) error {
	return assetexport.Validate(outputDir)
}

// ExportPreview is the lightweight inspection view used by the export tab.
type ExportPreview struct {
	MotionID    string               `json:"motionId"`
	Direction   string               `json:"direction"`
	CandidateID string               `json:"candidateId"`
	FrameCount  int                  `json:"frameCount"`
	Frames      []ExportPreviewFrame `json:"frames"`
}

type ExportPreviewFrame struct {
	Index      int                    `json:"index"`
	DurationMs int                    `json:"durationMs"`
	Anchors    []pipeline.AnchorPoint `json:"anchors,omitempty"`
}
