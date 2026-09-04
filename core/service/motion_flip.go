package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/motion"
	"github.com/oframe/character-workbench/core/pipeline"
	"github.com/oframe/character-workbench/core/version"
)

// MotionFlipDirection mirrors one generated direction's animation horizontally
// (动作卡九宫格右键"水平翻转") — the same real-pixel correction as the identity
// base-character flip, applied to a motion direction's retained candidate. AI
// generation frequently draws the character stepping/facing the mirrored way;
// the flip is its own inverse, so flipping twice restores the original pixels.
//
// Mirror-pair semantics: a mirror-derived direction (origin "mirrored")
// displays the horizontal mirror of its SOURCE direction's candidate resolved
// at read time, so the pixels always live in the source candidate. The flip
// therefore operates on the owning candidate and BOTH members of the mirror
// pair visually flip by one mirror step (a pair shows the same body from
// mirrored sides — they cannot diverge). The flip updates, coherently:
//
//  1. the owning candidate artifacts (raw filmstrip + processed frames +
//     per-frame anchor sets), including the in-memory candidate cache;
//  2. the motion sequence anchors (X' = width-1-X) of the owner and the
//     re-derived anchors of the mirrored sibling (after the source flip the
//     sibling displays the pre-flip source pixels, so its anchors revert to
//     the original values);
//  3. the accepted asset snapshot frames of every visually-flipped direction
//     — export reads snapshot pixels but anchors from the motion sequence, so
//     both must flip together or the exported atlas desyncs;
//  4. an operation log entry (auditability).
func (s *Service) MotionFlipDirection(pkgPath, motionID, direction string) (string, error) {
	direction = strings.TrimSpace(direction)
	st := motion.NewStore(pkgPath)
	ms, err := st.Load()
	if err != nil {
		return "", err
	}
	m, err := ms.Get(motionID)
	if err != nil {
		return "", err
	}
	dir := m.Direction(direction)
	if dir == nil {
		return "", fmt.Errorf("service: motion %s has no direction %s", motionID, direction)
	}
	if len(dir.Sequence.Frames) == 0 || dir.Sequence.Frames[0].AssetRef == "" {
		return "", fmt.Errorf("service: motion %s direction %s has no generated animation yet", motionID, direction)
	}

	// Resolve the pixel owner (a mirror-derived direction borrows the source
	// candidate) and the set of visually-flipped directions.
	ownerName := direction
	derivedName := motion.MirroredFrom(direction)
	if dir.Origin == motion.OriginMirrored {
		ownerName = motion.MirrorSource(direction)
		ownerDir := m.Direction(ownerName)
		if ownerName == "" || ownerDir == nil || len(ownerDir.Sequence.Frames) == 0 || ownerDir.Sequence.Frames[0].AssetRef == "" {
			return "", fmt.Errorf("service: mirror source of direction %s has no generated animation", direction)
		}
		derivedName = direction
	}
	owner := m.Direction(ownerName)

	// 1. Flip the owning candidate(s): raw strip, processed frames, anchors.
	flipped, err := s.flipMotionCandidates(pkgPath, owner)
	if err != nil {
		return "", err
	}

	// 2. Mirror the owner's sequence anchors; re-derive the sibling's.
	flippedDirs := []string{ownerName}
	for i, f := range owner.Sequence.Frames {
		w, werr := frameWidthOf(flipped, f.AssetRef, i)
		if werr != nil {
			return "", werr
		}
		owner.Sequence.Frames[i].Anchors = motion.MirrorAnchors(f.Anchors, w)
	}
	if derivedName != "" {
		if sb := m.Direction(derivedName); sb != nil && sb.Origin == motion.OriginMirrored && len(sb.Sequence.Frames) == len(owner.Sequence.Frames) {
			for i := range sb.Sequence.Frames {
				w, werr := frameWidthOf(flipped, owner.Sequence.Frames[i].AssetRef, i)
				if werr != nil {
					return "", werr
				}
				sb.Sequence.Frames[i].Anchors = motion.MirrorAnchors(owner.Sequence.Frames[i].Anchors, w)
			}
			flippedDirs = append(flippedDirs, derivedName)
		}
	}
	if err := st.Save(ms); err != nil {
		return "", err
	}

	// 3+4. Sync accepted snapshots and append the operation log.
	pkg, perr := identity.Open(pkgPath)
	if perr != nil {
		return "", perr
	}
	synced, err := flipAcceptedSnapshots(pkg, motionID, flippedDirs)
	if err != nil {
		return "", err
	}
	if _, aerr := version.Append(pkg, version.ActionMotionFlip, map[string]any{
		"motionId": motionID, "direction": direction, "flipped": flippedDirs, "assetsSynced": synced,
	}); aerr != nil {
		// The flip itself is done; a failed log line must not roll it back.
		s.log.Warn("motion flip log append failed", "package", pkgPath, "err", aerr.Error())
	}

	s.log.Info("motion direction flipped", "package", pkgPath, "motion", motionID,
		"direction", direction, "flipped", strings.Join(flippedDirs, ","), "assetsSynced", strings.Join(synced, ","))
	if len(flippedDirs) > 1 {
		return fmt.Sprintf("已水平翻转（镜像对 %s / %s 同步翻转 —— 再点一次可翻回）", flippedDirs[0], flippedDirs[1]), nil
	}
	return fmt.Sprintf("已水平翻转 %s 方向的动画（再点一次可翻回）", ownerName), nil
}

// flipMotionCandidates mirrors the raw filmstrip, every processed frame and
// every per-frame anchor set of the candidates referenced by the owner's
// sequence, persists them back and keeps the in-memory cache coherent. It
// returns the flipped candidates keyed by id (frame widths are read from them
// afterwards for anchor mirroring).
func (s *Service) flipMotionCandidates(pkgPath string, owner *motion.DirectionSet) (map[string]*pipeline.Candidate, error) {
	flipped := map[string]*pipeline.Candidate{}
	for i, f := range owner.Sequence.Frames {
		candID, _, ok := parseCandidateRef(f.AssetRef)
		if !ok {
			return nil, fmt.Errorf("service: frame %d references unsupported asset %q (only candidate frames are flippable)", i, f.AssetRef)
		}
		if flipped[candID] != nil {
			continue
		}
		cand, err := s.findCandidate(pkgPath, candID)
		if err != nil {
			return nil, err
		}
		if len(cand.FilmstripPNG) > 0 {
			img, derr := pipeline.DecodeImageAny(cand.FilmstripPNG)
			if derr != nil {
				return nil, fmt.Errorf("service: decode filmstrip of candidate %s: %w", candID, derr)
			}
			enc, eerr := pipeline.EncodeFilmstripPNG(pipeline.FlipHorizontal(img))
			if eerr != nil {
				return nil, eerr
			}
			cand.FilmstripPNG = enc
		}
		for j, fr := range cand.Frames {
			if fr == nil {
				return nil, fmt.Errorf("service: candidate %s frame %d is missing", candID, j)
			}
			cand.Frames[j] = pipeline.FlipHorizontal(fr)
			if j < len(cand.AnchorSets) {
				cand.AnchorSets[j] = motion.MirrorAnchors(cand.AnchorSets[j], cand.Frames[j].Bounds().Dx())
			}
		}
		if err := pipeline.SaveCandidate(filepath.Join(pkgPath, identity.DirCandidates, candID), cand); err != nil {
			return nil, err
		}
		// findCandidate serves cached pixels — replace the stale copy.
		s.candidatesFor(pkgPath).Replace(cand)
		flipped[candID] = &cand
	}
	return flipped, nil
}

// frameWidthOf returns the pixel width of the candidate frame a sequence frame
// references (the width anchor mirroring needs).
func frameWidthOf(flipped map[string]*pipeline.Candidate, assetRef string, seqIdx int) (int, error) {
	candID, frameIdx, ok := parseCandidateRef(assetRef)
	if !ok {
		return 0, fmt.Errorf("service: frame %d references unsupported asset %q", seqIdx, assetRef)
	}
	cand := flipped[candID]
	if cand == nil || frameIdx < 0 || frameIdx >= len(cand.Frames) || cand.Frames[frameIdx] == nil {
		return 0, fmt.Errorf("service: frame %d asset %q unavailable", seqIdx, assetRef)
	}
	return cand.Frames[frameIdx].Bounds().Dx(), nil
}

// flipAcceptedSnapshots mirrors the accepted asset snapshot frames of every
// visually-flipped direction (when those directions were accepted). Missing
// snapshot dirs mean "not accepted yet" and are skipped silently. Returns the
// directions whose snapshots were actually rewritten.
func flipAcceptedSnapshots(pkg *identity.Package, motionID string, dirs []string) ([]string, error) {
	assetsDir, err := version.CurrentAssetsDir(pkg)
	if err != nil {
		// No current version / assets area yet — nothing has been accepted.
		return nil, nil
	}
	var synced []string
	for _, dirName := range dirs {
		dir := filepath.Join(assetsDir, motionID, dirName)
		entries, rerr := os.ReadDir(dir)
		if rerr != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, "frame_") || !strings.HasSuffix(name, ".png") {
				continue
			}
			path := filepath.Join(dir, name)
			data, rr := os.ReadFile(path)
			if rr != nil {
				return nil, fmt.Errorf("service: read accepted frame: %w", rr)
			}
			img, derr := pipeline.DecodeImageAny(data)
			if derr != nil {
				return nil, fmt.Errorf("service: decode accepted frame %s: %w", name, derr)
			}
			enc, eerr := pipeline.EncodeFilmstripPNG(pipeline.FlipHorizontal(img))
			if eerr != nil {
				return nil, eerr
			}
			if werr := os.WriteFile(path, enc, 0o644); werr != nil {
				return nil, fmt.Errorf("service: write accepted frame: %w", werr)
			}
		}
		synced = append(synced, dirName)
	}
	return synced, nil
}
