// Base-character bindings (character-creation-workflow): candidate listing
// with inline PNG previews and explicit adoption. Both methods follow the
// established freshness pattern (acceptance.go): the manifest is read/written
// through the shared service, which opens the package fresh from disk — the
// GUI session instance may lag behind out-of-band writes made during
// generation execution.
package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
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
		if data, err := os.ReadFile(filepath.Join(pkg.Root(), filepath.FromSlash(c.ImagePath))); err == nil {
			png = base64.StdEncoding.EncodeToString(data)
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
	return pkg.LockBaseCharacterSource(source)
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
	png := ""
	if data, err := os.ReadFile(filepath.Join(pkg.Root(), filepath.FromSlash(candidate.ImagePath))); err == nil {
		png = base64.StdEncoding.EncodeToString(data)
	}
	a.log.Info("base character imported", "package", pkg.Root(), "candidate", candidate.ID)
	return &BaseCharacterCandidateView{
		ID: candidate.ID, ImagePath: candidate.ImagePath, PNG: png,
		Provider: candidate.Provider, Model: candidate.Model, Status: candidate.Status,
		CreatedAt: candidate.CreatedAt.Format(time.RFC3339),
	}, nil
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

// BaseCharacterReject marks a pending base-character candidate as rejected
// (弃用): it can no longer be adopted. No external call; the identity basis
// is untouched, so no session event is required — the caller refreshes the
// candidate list.
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
	return nil
}

// BaseCharacterDelete deletes a NON-adopted candidate record together with its
// image file (删除候选图，需前端弹窗确认). No external call; the identity basis
// is untouched, so no session event is required — the caller refreshes the
// candidate list.
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
	return nil
}
