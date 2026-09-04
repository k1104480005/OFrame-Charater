// Base-character bindings (character-creation-workflow): candidate listing
// with inline PNG previews and explicit adoption. Both methods follow the
// established freshness pattern (acceptance.go): the manifest is read/written
// through the shared service, which opens the package fresh from disk — the
// GUI session instance may lag behind out-of-band writes made during
// generation execution.
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/oframe/character-workbench/core/identity"
)

// BaseCharacterCandidateView is one recorded base-character candidate with an
// inline base64 PNG preview (the image lives in the package candidate area,
// which the frontend cannot read from disk directly).
type BaseCharacterCandidateView struct {
	ID        string `json:"id"`
	ImagePath string `json:"imagePath"`
	PNG       string `json:"png"` // base64 PNG bytes
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Status    string `json:"status"` // pending | adopted | rejected
	CreatedAt string `json:"createdAt"`
}

// BaseCharacterCandidatesGet returns every recorded base-character candidate
// of the session package with inline previews.
func (a *App) BaseCharacterCandidatesGet() ([]BaseCharacterCandidateView, error) {
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	candidates, err := svc.BaseCharacterCandidates(pkg.Root())
	if err != nil {
		return nil, err
	}
	out := make([]BaseCharacterCandidateView, 0, len(candidates))
	for _, c := range candidates {
		png := ""
		if path, pathErr := pkg.BaseCharacterImagePath(c.ImagePath); pathErr == nil {
			if data, err := os.ReadFile(path); err == nil {
				png = base64.StdEncoding.EncodeToString(data)
			}
		}
		out = append(out, BaseCharacterCandidateView{
			ID: c.ID, ImagePath: c.ImagePath, PNG: png,
			Provider: c.Provider, Model: c.Model, Status: c.Status,
			CreatedAt: c.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

// BaseCharacterSourceLock explicitly selects and permanently locks the base
// character source before the user starts the corresponding workflow.
func (a *App) BaseCharacterSourceLock(source string) error {
	pkg, err := a.requirePackage()
	if err != nil {
		return err
	}
	if err := pkg.LockBaseCharacterSource(source); err != nil {
		return err
	}
	// Source selection mutates the manifest. Refresh the session and broadcast
	// the change so the launch card and other tabs do not keep stale metadata.
	return a.refreshSessionFromDisk(pkg.Root())
}

// refreshSessionFromDisk re-opens the package after a manifest mutation made
// through a fresh (non-session) instance (import / cropped import / reject /
// delete). Without this the GUI session instance keeps a stale manifest and
// the next session-instance save would resurrect deleted candidates — the
// same freshness rule adoption follows. It also re-emits the session event so
// every tab (and the session header) syncs immediately.
func (a *App) refreshSessionFromDisk(pkgRoot string) error {
	fresh, err := identity.Open(pkgRoot)
	if err != nil {
		return err
	}
	_, err = a.openPackage(fresh)
	return err
}

// BaseCharacterImport records a local sprite image as a PENDING base-character
// candidate — the import base source. No external call is made; the image must
// match the logical canvas (service-side validation). Adoption afterwards is
// the explicit user decision through BaseCharacterAdopt.
func (a *App) BaseCharacterImport(srcPath string) (*BaseCharacterCandidateView, error) {
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	candidate, err := svc.ImportBaseCharacter(pkg.Root(), srcPath)
	if err != nil {
		return nil, err
	}
	a.log.Info("base character imported", "package", pkg.Root(), "candidate", candidate.ID)
	if err := a.refreshSessionFromDisk(pkg.Root()); err != nil {
		return nil, err
	}
	return a.baseCharacterCandidateView(pkg, candidate)
}

// BaseCharacterImportCropped crops the picked image to the given source-pixel
// rectangle (GUI crop tool: aspect pre-locked to the logical canvas with guide
// lines), nearest-resizes the result to the logical canvas and registers it as
// the pending import draft (same one-draft replace rule). No external call.
func (a *App) BaseCharacterImportCropped(srcPath string, x, y, w, h int) (*BaseCharacterCandidateView, error) {
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	candidate, err := svc.ImportBaseCharacterCropped(pkg.Root(), srcPath, x, y, w, h)
	if err != nil {
		return nil, err
	}
	a.log.Info("base character imported (cropped)", "package", pkg.Root(), "candidate", candidate.ID,
		"rect", fmt.Sprintf("(%d,%d) %dx%d", x, y, w, h))
	if err := a.refreshSessionFromDisk(pkg.Root()); err != nil {
		return nil, err
	}
	return a.baseCharacterCandidateView(pkg, candidate)
}

// baseCharacterCandidateView reads the candidate image from the package
// candidate area and wraps it with an inline base64 preview.
func (a *App) baseCharacterCandidateView(pkg *identity.Package, candidate identity.BaseCharacterCandidate) (*BaseCharacterCandidateView, error) {
	png := ""
	if path, pathErr := pkg.BaseCharacterImagePath(candidate.ImagePath); pathErr == nil {
		if data, err := os.ReadFile(path); err == nil {
			png = base64.StdEncoding.EncodeToString(data)
		}
	}
	return &BaseCharacterCandidateView{
		ID: candidate.ID, ImagePath: candidate.ImagePath, PNG: png,
		Provider: candidate.Provider, Model: candidate.Model, Status: candidate.Status,
		CreatedAt: candidate.CreatedAt.Format(time.RFC3339),
	}, nil
}

// BaseCharacterDescribeImage asks the configured prompt-enhancement text model
// (vision-capable) to describe one base-character candidate image (识图生成描
// 述). One billed text call (90s window); the returned text is a suggestion —
// the frontend fills it into the description textarea for user review.
func (a *App) BaseCharacterDescribeImage(candidateID string) (string, error) {
	pkg, err := a.requirePackage()
	if err != nil {
		return "", err
	}
	svc, err := a.service()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	text, err := svc.DescribeBaseCharacterImage(ctx, pkg.Root(), candidateID)
	if err != nil {
		return "", err
	}
	a.log.Info("base character image described", "package", pkg.Root(), "candidate", candidateID)
	return text, nil
}

// BaseCharacterAdopt adopts one candidate as the identity's current base
// character. Adoption never triggers external calls. After the service-side
// adoption the session package is re-opened and the session event re-emitted,
// so every tab immediately sees the new identity basis.
func (a *App) BaseCharacterAdopt(id string) error {
	pkg, err := a.requirePackage()
	if err != nil {
		return err
	}
	svc, err := a.service()
	if err != nil {
		return err
	}
	if err := svc.AdoptBaseCharacter(pkg.Root(), id); err != nil {
		return err
	}
	a.log.Info("base character adopted", "package", pkg.Root(), "candidate", id)
	fresh, err := identity.Open(pkg.Root())
	if err != nil {
		return err
	}
	_, err = a.openPackage(fresh)
	return err
}

// BaseCharacterFlip mirrors one base-character candidate image horizontally
// (水平翻转). AI generation often draws the character facing the wrong way,
// so the flip is a correction available on every candidate — pending, adopted
// or rejected; the adopted basis is the same record and file, so flipping it
// re-aligns the base sprite every later generation sends as reference. The
// flip is its own inverse (flipping twice restores the original). The session
// is refreshed from disk afterwards so the launch-card thumbnail of the
// adopted basis and every tab see the corrected image immediately.
func (a *App) BaseCharacterFlip(candidateID string) error {
	pkg, err := a.requirePackage()
	if err != nil {
		return err
	}
	svc, err := a.service()
	if err != nil {
		return err
	}
	if err := svc.FlipBaseCharacter(pkg.Root(), candidateID); err != nil {
		return err
	}
	a.log.Info("base character candidate flipped", "package", pkg.Root(), "candidate", candidateID)
	return a.refreshSessionFromDisk(pkg.Root())
}

// BaseCharacterReject marks a pending base-character candidate as rejected
// (弃用): it can no longer be adopted. No external call; the identity basis
// is untouched. The session is refreshed from disk so no stale manifest is
// kept around.
func (a *App) BaseCharacterReject(id string) error {
	pkg, err := a.requirePackage()
	if err != nil {
		return err
	}
	svc, err := a.service()
	if err != nil {
		return err
	}
	if err := svc.RejectBaseCharacter(pkg.Root(), id); err != nil {
		return err
	}
	a.log.Info("base character rejected", "package", pkg.Root(), "candidate", id)
	return a.refreshSessionFromDisk(pkg.Root())
}

// BaseCharacterDelete deletes a NON-adopted candidate record together with its
// image file (删除候选图，需前端弹窗确认). No external call; the identity basis
// is untouched. The session is re-opened from disk and the session event is
// re-emitted, so every surface (导入缩略图 / 确认锁定 / 候选网格 / 锚点面板)
// sees the removal immediately and no stale manifest can resurrect the record.
func (a *App) BaseCharacterDelete(id string) error {
	pkg, err := a.requirePackage()
	if err != nil {
		return err
	}
	svc, err := a.service()
	if err != nil {
		return err
	}
	if err := svc.DeleteBaseCharacter(pkg.Root(), id); err != nil {
		return err
	}
	a.log.Info("base character candidate deleted", "package", pkg.Root(), "candidate", id)
	return a.refreshSessionFromDisk(pkg.Root())
}
