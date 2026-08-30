package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// DraftFileName is the unsaved-draft sidecar inside an identity package
// (workbench-ui spec: 未提交草稿在标签切换/任务运行/应用重启后保留). It is a
// plain sidecar file: it never enters the manifest, package validation, or
// exports, and it travels with the package directory (copy/migrate keep it).
const DraftFileName = ".draft.json"

// Draft is the user's uncommitted work-in-progress for one identity package:
// the identity description textarea and the motion creation form. Cleared
// when the user saves the real fields.
type Draft struct {
	Description  string    `json:"description,omitempty"`
	MotionName   string    `json:"motionName,omitempty"`
	MotionCount  int       `json:"motionCount,omitempty"`
	MotionMirror *bool     `json:"motionMirror,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Empty reports whether the draft carries no user input at all.
func (d Draft) Empty() bool {
	return d.Description == "" && d.MotionName == "" && d.MotionCount == 0 && d.MotionMirror == nil
}

// SaveDraft persists the draft sidecar (atomic temp+rename like the manifest).
func (p *Package) SaveDraft(d Draft) error {
	d.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("identity: encode draft: %w", err)
	}
	path := filepath.Join(p.root, DraftFileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("identity: write draft: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("identity: persist draft: %w", err)
	}
	return nil
}

// LoadDraft returns the persisted draft, or the zero Draft when none exists
// (fresh packages and cleared drafts are indistinguishable to callers).
func (p *Package) LoadDraft() (Draft, error) {
	data, err := os.ReadFile(filepath.Join(p.root, DraftFileName))
	if errors.Is(err, fs.ErrNotExist) {
		return Draft{}, nil
	}
	if err != nil {
		return Draft{}, fmt.Errorf("identity: read draft: %w", err)
	}
	var d Draft
	if err := json.Unmarshal(data, &d); err != nil {
		// A corrupt draft must never block the package: drop it and continue.
		_ = os.Remove(filepath.Join(p.root, DraftFileName))
		return Draft{}, nil
	}
	return d, nil
}

// ClearDraft removes the draft sidecar (called after the real fields are
// saved). Missing file is a no-op success.
func (p *Package) ClearDraft() error {
	err := os.Remove(filepath.Join(p.root, DraftFileName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("identity: clear draft: %w", err)
	}
	return nil
}
