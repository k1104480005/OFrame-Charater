// Typed Go bindings for the identity package session and the Identity sub-page
// of the Make tab. All methods delegate to the shared core services
// (core/identity, core/version) — the same code the oframe CLI calls — so GUI
// and CLI cannot drift (design D1/D12).
package main

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/version"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// PackageSummary is the lightweight identity-package descriptor the frontend
// uses everywhere (launch page list, session header, tab footer).
type PackageSummary struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	FormatVersion  int    `json:"formatVersion"`
	CurrentVersion string `json:"currentVersion"`
}

// CanvasView mirrors identity.CanvasSpec for the frontend.
type CanvasView struct {
	UnitWidth  int `json:"unitWidth"`
	UnitHeight int `json:"unitHeight"`
}

// AnchorView mirrors identity.Anchor for the frontend.
type AnchorView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Preset string `json:"preset"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
}

// AnchorPresetView mirrors identity.AnchorPreset for the frontend.
type AnchorPresetView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// MaterialView mirrors identity.Material for the frontend.
type MaterialView struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Role    string `json:"role"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	AddedAt string `json:"addedAt"`
}

// VersionView mirrors identity.VersionRecord for the frontend.
type VersionView struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
	Reason    string `json:"reason"`
	Immutable bool   `json:"immutable"`
	AssetsRef string `json:"assetsRef"`
}

// IdentityView is the full Identity sub-page payload.
type IdentityView struct {
	Name           string         `json:"name"`
	ID             string         `json:"id"`
	Description    string         `json:"description"`
	EntryKind      string         `json:"entryKind"`
	Canvas         *CanvasView    `json:"canvas,omitempty"`
	Anchors        []AnchorView   `json:"anchors"`
	Materials      []MaterialView `json:"materials"`
	Versions       []VersionView  `json:"versions"`
	CurrentVersion string         `json:"currentVersion"`
}

// --- session: current identity package ---

// CurrentPackage returns the open identity package summary, or nil when the
// app is on the launch page. The same instance backs every tab.
func (a *App) CurrentPackage() *PackageSummary {
	pkg := a.pkg
	if pkg == nil {
		return nil
	}
	return a.packageSummary(pkg)
}

// PackageCreate creates a new identity package inside the current workspace and
// opens it as the session package (launch page → workbench). The package
// directory name is the identity name.
func (a *App) PackageCreate(name string) (*PackageSummary, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errNameRequired
	}
	ws, err := a.ensureWorkspace()
	if err != nil {
		return nil, err
	}
	pkg, err := identity.Create(filepath.Join(ws.Root(), name), name)
	if err != nil {
		return nil, err
	}
	return a.openPackage(pkg)
}

// PackageOpen opens an existing identity package by path and makes it the
// session package. Corrupt / missing / too-new manifests are refused by
// core identity.Open (task 2.2) and the package is never modified.
func (a *App) PackageOpen(path string) (*PackageSummary, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errPathRequired
	}
	pkg, err := identity.Open(path)
	if err != nil {
		return nil, err
	}
	return a.openPackage(pkg)
}

// PackageClose closes the session package and returns to the launch page.
func (a *App) PackageClose() error {
	if a.pkg != nil {
		a.log.Info("identity package closed", "path", a.pkg.Root())
	}
	a.pkg = nil
	a.emit(EventSessionChanged, nil)
	return nil
}

// openPackage installs the opened package as the session package, emits the
// session event, and returns its summary.
func (a *App) openPackage(pkg *identity.Package) (*PackageSummary, error) {
	a.pkg = pkg
	summary := a.packageSummary(pkg)
	a.log.Info("identity package opened in workbench", "path", pkg.Root(), "name", summary.Name)
	a.emit(EventSessionChanged, summary)
	return summary, nil
}

func (a *App) packageSummary(pkg *identity.Package) *PackageSummary {
	m := pkg.Manifest()
	return &PackageSummary{
		Name:           m.Identity.Name,
		Path:           pkg.Root(),
		FormatVersion:  m.FormatVersion,
		CurrentVersion: m.Versions.Current,
	}
}

// requirePackage returns the session package or a typed error.
func (a *App) requirePackage() (*identity.Package, error) {
	if a.pkg == nil {
		return nil, errNoPackageOpen
	}
	return a.pkg, nil
}

// --- identity sub-page ---

// IdentityGet returns the full identity definition for the Identity sub-page.
func (a *App) IdentityGet() (*IdentityView, error) {
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	m := pkg.Manifest()
	view := &IdentityView{
		Name:           m.Identity.Name,
		ID:             m.Identity.ID,
		Description:    m.Identity.Description,
		EntryKind:      m.Identity.EntryKind,
		Anchors:        []AnchorView{},
		Materials:      []MaterialView{},
		Versions:       []VersionView{},
		CurrentVersion: m.Versions.Current,
	}
	if c := m.LogicalCanvas; c != nil {
		view.Canvas = &CanvasView{UnitWidth: c.UnitWidth, UnitHeight: c.UnitHeight}
	}
	for _, an := range m.Anchors {
		view.Anchors = append(view.Anchors, AnchorView{ID: an.ID, Name: an.Name, Preset: an.Preset, X: an.X, Y: an.Y})
	}
	for _, mat := range m.Materials {
		view.Materials = append(view.Materials, MaterialView{
			ID: mat.ID, Kind: mat.Kind, Role: mat.Role, Name: mat.Name, Path: mat.Path,
			AddedAt: mat.AddedAt.Format(time.RFC3339),
		})
	}
	for _, v := range version.List(pkg) {
		view.Versions = append(view.Versions, VersionView{
			ID: v.ID, CreatedAt: v.CreatedAt.Format(time.RFC3339),
			Reason: v.Reason, Immutable: v.Immutable, AssetsRef: v.AssetsRef,
		})
	}
	return view, nil
}

// IdentitySetDescription records the text-description identity entry
// (task 2.3 入口 1: 文字描述写入身份包元数据).
func (a *App) IdentitySetDescription(text string) error {
	pkg, err := a.requirePackage()
	if err != nil {
		return err
	}
	if err := pkg.SetTextDescription(text); err != nil {
		return err
	}
	return a.identityChanged()
}

// IdentitySetCanvas sets the logical canvas specification (task 2.4).
func (a *App) IdentitySetCanvas(width, height int) error {
	pkg, err := a.requirePackage()
	if err != nil {
		return err
	}
	if err := pkg.SetLogicalCanvas(width, height); err != nil {
		return err
	}
	return a.identityChanged()
}

// IdentityAnchorPresets returns the built-in anchor presets (脚底/手持点/中心).
func (a *App) IdentityAnchorPresets() []AnchorPresetView {
	presets := identity.AnchorPresets()
	out := make([]AnchorPresetView, 0, len(presets))
	for _, p := range presets {
		out = append(out, AnchorPresetView{ID: p.ID, Name: p.Name})
	}
	return out
}

// IdentityAddAnchorPreset defines an anchor from a preset at its default
// position on the logical canvas (task 2.5).
func (a *App) IdentityAddAnchorPreset(presetID, name string) (*AnchorView, error) {
	preset, ok := findPreset(presetID)
	if !ok {
		return nil, errUnknownPreset(presetID)
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	an, err := pkg.AddAnchorPreset(preset, name)
	if err != nil {
		return nil, err
	}
	if err := a.identityChanged(); err != nil {
		return nil, err
	}
	return &AnchorView{ID: an.ID, Name: an.Name, Preset: an.Preset, X: an.X, Y: an.Y}, nil
}

// IdentityAddAnchor defines an anchor at explicit coordinates (task 2.5).
func (a *App) IdentityAddAnchor(name, presetID string, x, y int) (*AnchorView, error) {
	preset, ok := findPreset(presetID)
	if !ok {
		preset = identity.AnchorPreset{ID: presetID, Name: name}
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	an, err := pkg.AddAnchor(name, preset, x, y)
	if err != nil {
		return nil, err
	}
	if err := a.identityChanged(); err != nil {
		return nil, err
	}
	return &AnchorView{ID: an.ID, Name: an.Name, Preset: an.Preset, X: an.X, Y: an.Y}, nil
}

// IdentityImportMaterial stores a reference image or existing sprite into the
// package material area and uses it as the identity entry basis when none is
// set yet (task 2.3 入口 2/3). kind is "reference_image" or "sprite"; srcPath
// is the absolute source file path (picked via PickMaterialFile). role is the
// reference-image role for kind reference_image: "main_reference" (主参考图,
// 最多 1 张) or "auxiliary_reference" (辅助参考图, 最多 2 张); empty defaults
// to main_reference. Sprites always carry the sprite role.
func (a *App) IdentityImportMaterial(kind, srcPath, name, role string) (*MaterialView, error) {
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	var mat *identity.Material
	switch kind {
	case identity.MaterialKindReferenceImage:
		if role == "" {
			role = identity.RoleMainReference
		}
		mat, err = pkg.AddReferenceImage(srcPath, name, role)
	case identity.MaterialKindSprite:
		mat, err = pkg.ImportSprite(srcPath, name)
	default:
		return nil, errUnknownMaterialKind(kind)
	}
	if err != nil {
		return nil, err
	}
	if err := a.identityChanged(); err != nil {
		return nil, err
	}
	return &MaterialView{
		ID: mat.ID, Kind: mat.Kind, Role: mat.Role, Name: mat.Name, Path: mat.Path,
		AddedAt: mat.AddedAt.Format(time.RFC3339),
	}, nil
}

// PickMaterialFile opens a native file-open dialog (Go-side; Wails does not
// expose dialogs to the frontend) and returns the chosen absolute path, or ""
// when the user cancels.
func (a *App) PickMaterialFile(title string) (string, error) {
	if a.ctx == nil {
		return "", nil // headless/test: no dialog, treat as cancel
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: title,
		Filters: []runtime.FileFilter{
			{DisplayName: "图片文件", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.bmp;*.webp"},
			{DisplayName: "所有文件", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// identityChanged re-emits the session event so every tab sees the updated
// manifest state (tabs share one package instance; nothing is lost on switch).
func (a *App) identityChanged() error {
	a.emit(EventSessionChanged, a.CurrentPackage())
	return nil
}

// findPreset looks up a built-in anchor preset by ID.
func findPreset(id string) (identity.AnchorPreset, bool) {
	for _, p := range identity.AnchorPresets() {
		if p.ID == id {
			return p, true
		}
	}
	return identity.AnchorPreset{}, false
}
