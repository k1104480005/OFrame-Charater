package main

import (
	"github.com/oframe/character-workbench/core/assetexport"
)

// ExportCreate builds a validated package from the current identity package's
// accepted assets. The output directory is chosen by the caller for now; a
// future native folder picker can feed the same binding.
func (a *App) ExportCreate(outputDir, target, versionID string) (*assetexport.Result, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	return svc.ExportPackage(pkg.Root(), outputDir, target, versionID)
}

func (a *App) ExportValidate(outputDir string) error {
	svc, err := a.service()
	if err != nil {
		return err
	}
	return svc.ValidateExport(outputDir)
}

func (a *App) ExportHistory() ([]assetexport.HistoryRecord, error) {
	svc, err := a.service()
	if err != nil {
		return nil, err
	}
	pkg, err := a.requirePackage()
	if err != nil {
		return nil, err
	}
	return svc.ExportHistory(pkg.Root())
}
