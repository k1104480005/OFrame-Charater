package service

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/motion"
	"github.com/oframe/character-workbench/core/pipeline"
)

// MotionService methods (阶段 5, motion capability): the GUI bindings and the
// oframe CLI both call these — the motion model lives in core/motion and is
// persisted per identity package to motions.json (motion.Store).

// MotionCreate creates a motion with the given direction strategy in the
// identity package (task 3.1/3.2: 动作由方向集构成; 单方向默认 down/正面).
func (s *Service) MotionCreate(pkgPath, name string, strategy motion.DirectionStrategy) (*motion.Motion, error) {
	if strings.TrimSpace(pkgPath) == "" {
		return nil, fmt.Errorf("service: package path is required")
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("service: motion name is required")
	}
	st := motion.NewStore(pkgPath)
	ms, err := st.Load()
	if err != nil {
		return nil, err
	}
	m, err := motion.NewMotion(newPlanID(), name, strategy)
	if err != nil {
		return nil, err
	}
	if err := ms.Add(m); err != nil {
		return nil, err
	}
	if err := st.Save(ms); err != nil {
		return nil, err
	}
	s.log.Info("motion created", "package", pkgPath, "motion", m.ID, "name", name,
		"directions", strategy.Count, "mirror", strategy.Mirror)
	return m, nil
}

// MotionList returns the motions of an identity package in insertion order.
func (s *Service) MotionList(pkgPath string) ([]*motion.Motion, error) {
	ms, err := motion.NewStore(pkgPath).Load()
	if err != nil {
		return nil, err
	}
	return ms.List(), nil
}

// MotionGet returns one motion by id.
func (s *Service) MotionGet(pkgPath, id string) (*motion.Motion, error) {
	ms, err := motion.NewStore(pkgPath).Load()
	if err != nil {
		return nil, err
	}
	return ms.Get(id)
}

// MotionSetStrategy re-derives a motion's direction set for a new strategy
// (task 3.2/3.3: 方向策略可调整; 关闭镜像 → 所有方向独立生成).
func (s *Service) MotionSetStrategy(pkgPath, id string, strategy motion.DirectionStrategy) (*motion.Motion, error) {
	st := motion.NewStore(pkgPath)
	ms, err := st.Load()
	if err != nil {
		return nil, err
	}
	m, err := ms.Get(id)
	if err != nil {
		return nil, err
	}
	if err := m.SetStrategy(strategy); err != nil {
		return nil, err
	}
	if err := st.Save(ms); err != nil {
		return nil, err
	}
	s.log.Info("motion strategy set", "package", pkgPath, "motion", id,
		"directions", strategy.Count, "mirror", strategy.Mirror)
	return m, nil
}

// MotionSetFrameDurations sets the per-frame display durations (rhythm) of a
// direction (task 3.6: 帧时长调整; 预览按新节奏回放).
func (s *Service) MotionSetFrameDurations(pkgPath, id, dir string, durationsMs []int) (*motion.Motion, error) {
	st := motion.NewStore(pkgPath)
	ms, err := st.Load()
	if err != nil {
		return nil, err
	}
	m, err := ms.Get(id)
	if err != nil {
		return nil, err
	}
	if err := m.SetFrameDurations(dir, durationsMs); err != nil {
		return nil, err
	}
	if err := st.Save(ms); err != nil {
		return nil, err
	}
	return m, nil
}

// MotionPlaybackTempo returns the playback rhythm of a direction's sequence
// (task 3.6: 调整后预览按新节奏回放).
func (s *Service) MotionPlaybackTempo(pkgPath, id, dir string) ([]int, error) {
	ms, err := motion.NewStore(pkgPath).Load()
	if err != nil {
		return nil, err
	}
	m, err := ms.Get(id)
	if err != nil {
		return nil, err
	}
	return m.PlaybackTempo(dir)
}

// CandidateList returns the retained filmstrip candidates of a package,
// loaded from the persisted candidate area (<package>/candidates/<id>/) so
// candidates survive app restarts (the in-memory CandidateSet is a session
// cache; the persisted artifacts are the source of truth).
func (s *Service) CandidateList(pkgPath string) []pipeline.Candidate {
	ids, err := pipeline.ListCandidates(filepath.Join(pkgPath, identity.DirCandidates))
	if err != nil {
		return nil
	}
	out := make([]pipeline.Candidate, 0, len(ids))
	for _, id := range ids {
		if c, err := pipeline.LoadCandidate(filepath.Join(pkgPath, identity.DirCandidates, id)); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// PackageMotionPath resolves the motions file path of an identity package
// (convenience for tests/CLI; the motion store is the source of truth).
func PackageMotionPath(pkgPath string) string {
	return motion.NewStore(pkgPath).Path()
}
