// Command oframe-workbench is the Wails v2 desktop shell (GUI entry point)
// over the shared Go core library — the same core the oframe CLI uses
// (design D1/D2: 单核多端, Wails GUI + CLI share one core; cli spec:
// 与 GUI 共享同一 Go 核心库). Phase 2 ships the frontend skeleton: launch
// page, three-tab shell, Make sub-pages, theme system, and the global task
// drawer; provider / image pipeline / full task queue / export logic land in
// later phases.
package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:             "OFrame Character Workbench",
		Width:             1280,
		Height:            820,
		MinWidth:          1024,
		MinHeight:         700,
		MaxWidth:          1920,
		MaxHeight:         1080,
		DisableResize:     false,
		Fullscreen:        false,
		Frameless:         false,
		StartHidden:       false,
		HideWindowOnClose: false,
		BackgroundColour:  &options.RGBA{R: 250, G: 245, B: 235, A: 255},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Menu:             nil,
		Logger:           nil,
		LogLevel:         logger.DEBUG,
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdown,
		WindowStartState: options.Normal,
		Bind: []interface{}{
			app,
		},
		// Windows platform specific options (Windows 10/11 x64 first release).
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
			WebviewUserDataPath:  "",
			ZoomFactor:           1.0,
		},
	})

	if err != nil {
		log.Fatal(err)
	}
}
