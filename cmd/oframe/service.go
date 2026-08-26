package main

import (
	"net/http"

	"github.com/oframe/character-workbench/core/service"
)

// httpClientOverride lets tests inject a fake transport so CLI tests never call
// real paid services. nil → http.DefaultClient.
var httpClientOverride *http.Client

// newCLIService builds the shared application service (GUI/CLI 共享 application
// service): the same core/service type the Wails bindings use. settingsDir ""
// selects the user config directory, so CLI and GUI share keys/models/stats.
func newCLIService(settingsDir string) (*service.Service, error) {
	return service.New(service.Options{
		SettingsDir: settingsDir,
		HTTPClient:  httpClientOverride,
	})
}
