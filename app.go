// The Wails-bound application struct. It holds the GUI session state (current
// workspace + current identity package — the same shared instance all tabs
// render) and exposes typed binding methods over the shared Go core services
// (core/workspace, core/identity, core/version). Frontend stays stateless:
// React renders, Go executes and persists (design D11, PLAN §2.2).
package main

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	gort "runtime"
	"strings"
	"sync"
	"time"

	"github.com/oframe/character-workbench/core/identity"
	"github.com/oframe/character-workbench/core/logging"
	"github.com/oframe/character-workbench/core/service"
	"github.com/oframe/character-workbench/core/workspace"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// appVersion is the application version reported to the UI.
const appVersion = "0.2.0"

// App is the Wails-bound application struct.
type App struct {
	ctx context.Context
	log *slog.Logger

	// session state — shared by every tab (design D11: 标签间共享同一身份包实例)
	ws  *workspace.Workspace // current workspace (nil until ensured)
	pkg *identity.Package    // current identity package (nil at launch page)

	// shared application service (阶段 3: GUI/CLI 共享 application service) —
	// provider 配置/验证、调用统计、PerfectPixel 预设、生成确认、持久化任务队列.
	// Created lazily on first use; settingsDir and httpClient are overridable
	// in tests ("" / nil → user config dir / http.DefaultClient).
	svc         *service.Service
	svcMu       sync.Mutex
	settingsDir string
	httpClient  *http.Client
}

// service returns the shared application service, creating it on first use and
// wiring the task:changed runtime event so the global task drawer stays live.
func (a *App) service() (*service.Service, error) {
	a.svcMu.Lock()
	defer a.svcMu.Unlock()
	if a.svc != nil {
		return a.svc, nil
	}
	svc, err := service.New(service.Options{SettingsDir: a.settingsDir, HTTPClient: a.httpClient, Logger: a.log})
	if err != nil {
		return nil, err
	}
	svc.SetTasksChangedHook(func() {
		a.emit(EventTasksChanged, a.tasksListOrEmpty())
	})
	a.svc = svc
	a.log.Info("shared application service ready", "settingsDir", svc.SettingsDir())
	return svc, nil
}

// tasksListOrEmpty reads the task list for the drawer event (nil-safe: the
// service may be mid-construction).
func (a *App) tasksListOrEmpty() []TaskSummary {
	if a.svc == nil {
		return []TaskSummary{}
	}
	list, err := a.svc.TaskList()
	if err != nil {
		return []TaskSummary{}
	}
	out := make([]TaskSummary, 0, len(list))
	for _, v := range list {
		out = append(out, TaskSummary{
			ID: v.ID, Kind: v.Kind, PackagePath: v.PackagePath, Provider: v.Provider,
			Status: TaskStatus(v.Status), Progress: v.Progress,
			Error: v.Error, RetryCount: v.RetryCount, ExpectedCalls: v.ExpectedCalls,
			CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
			Live: v.Live,
		})
	}
	return out
}

// NewApp creates the application struct.
func NewApp() *App {
	log := logging.New(logging.Options{Level: slog.LevelInfo})
	return &App{log: log}
}

// startup is called at application startup.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.log.Info("oframe workbench starting", "version", appVersion)
	// Ensure the default workspace exists so the launch page can list packages
	// immediately. Failure here is non-fatal — the launch page retries and
	// surfaces the error.
	if _, err := a.ensureWorkspace(); err != nil {
		a.log.Warn("default workspace not ready at startup", "error", err)
	}
}

// domReady is called after front-end resources have been loaded.
func (a *App) domReady(ctx context.Context) {
	a.log.Info("oframe workbench frontend ready")
}

// beforeClose is called when the application is about to quit.
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	return false
}

// shutdown is called at application termination.
func (a *App) shutdown(ctx context.Context) {
	a.log.Info("oframe workbench shutting down")
}

// --- runtime events helpers (runtime events 基础) ---

// emit sends a runtime event to the frontend. It is nil-safe so binding
// methods stay unit-testable without a Wails context.
func (a *App) emit(event string, data any) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, event, data)
}

// Event names shared with the frontend.
const (
	EventSessionChanged = "session:changed" // payload: *PackageSummary (nil = launch page)
	EventTasksChanged   = "task:changed"    // payload: []TaskSummary
)

// --- app info (proves the Go binding round-trip; task 1.2) ---

// AppInfo is the application identity shown on the launch page footer.
type AppInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Go      string `json:"go"`
	Format  int    `json:"formatVersion"`
}

// AppInfo returns application identity information.
func (a *App) AppInfo() AppInfo {
	a.log.Info("binding AppInfo called by frontend", "version", appVersion)
	return AppInfo{
		Name:    "OFrame Character Workbench",
		Version: appVersion,
		Go:      gort.Version(),
		Format:  identity.FormatVersion,
	}
}

// --- workspace session ---

// WorkspaceInfo describes the current workspace directory.
type WorkspaceInfo struct {
	Path         string `json:"path"`
	PackageCount int    `json:"packageCount"`
}

// WorkspacePath returns the current workspace path, or "" when none is set.
func (a *App) WorkspacePath() string {
	if a.ws == nil {
		return ""
	}
	return a.ws.Root()
}

// WorkspaceEnsureDefault ensures the default workspace (user home
// /OFrameWorkspace) exists and opens it, returning its info.
func (a *App) WorkspaceEnsureDefault() (WorkspaceInfo, error) {
	ws, err := a.ensureWorkspace()
	if err != nil {
		return WorkspaceInfo{}, err
	}
	return a.workspaceInfo(ws)
}

// WorkspaceOpen opens an explicit workspace directory and persists the choice
// so it survives the next launch (workspace-settings requirement: remember the
// chosen workspace instead of always resetting to the C: default).
func (a *App) WorkspaceOpen(path string) (WorkspaceInfo, error) {
	ws, err := workspace.Open(path)
	if err != nil {
		return WorkspaceInfo{}, err
	}
	a.ws = ws
	if err := workspace.SaveConfig(workspace.Config{Path: ws.Root()}); err != nil {
		a.log.Warn("workspace: persist choice", "error", err)
	}
	a.log.Info("workspace opened", "path", ws.Root())
	return a.workspaceInfo(ws)
}

// WorkspaceList lists the identity packages in the current (or default)
// workspace — the launch page data source.
func (a *App) WorkspaceList() ([]PackageSummary, error) {
	ws, err := a.ensureWorkspace()
	if err != nil {
		return nil, err
	}
	infos, err := ws.List()
	if err != nil {
		return nil, err
	}
	out := make([]PackageSummary, 0, len(infos))
	for _, info := range infos {
		out = append(out, PackageSummary{
			Name:                info.Name,
			Path:                info.Path,
			Category:            info.Category,
			FormatVersion:       info.FormatVersion,
			CurrentVersion:      info.CurrentVersion,
			BaseCharacterSource: info.BaseCharacterSource,
			BaseCharacterThumb:  info.BaseCharacterThumb,
			CreatedAt:           info.CreatedAt.Format(time.RFC3339),
			UpdatedAt:           info.UpdatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

// ensureWorkspace returns the current workspace, initializing the preferred
// default one on first use (idempotent: Init creates-if-missing). The preferred
// default prefers a non-system drive and remembers a previously chosen path;
// if that location is unusable we fall back to the user-home default so the
// app still launches.
func (a *App) ensureWorkspace() (*workspace.Workspace, error) {
	if a.ws != nil {
		return a.ws, nil
	}
	root, err := workspace.PreferredDefaultPath()
	if err != nil {
		return nil, err
	}
	ws, err := workspace.Init(root)
	if err != nil {
		home, herr := workspace.DefaultPath()
		if herr != nil {
			return nil, err
		}
		ws, err = workspace.Init(home)
		if err != nil {
			return nil, err
		}
	}
	a.ws = ws
	// Seed the default choice, and repair a stale persisted path after falling
	// back, so a removed temporary workspace cannot capture future launches.
	if cfg, cerr := workspace.LoadConfig(); cerr == nil && cfg.Path != ws.Root() {
		_ = workspace.SaveConfig(workspace.Config{Path: ws.Root()})
	}
	a.log.Info("default workspace ensured", "path", ws.Root())
	return ws, nil
}

func (a *App) workspaceInfo(ws *workspace.Workspace) (WorkspaceInfo, error) {
	pkgs, err := ws.List()
	if err != nil {
		return WorkspaceInfo{}, err
	}
	return WorkspaceInfo{Path: ws.Root(), PackageCount: len(pkgs)}, nil
}

// PickWorkspaceDir opens a native directory-selection dialog (Go-side; Wails
// does not expose dialogs to the frontend) and returns the chosen absolute
// path, or "" when the user cancels (workspace-settings requirement: a folder
// picker instead of hand-typing the path).
func (a *App) PickWorkspaceDir(title string) (string, error) {
	if a.ctx == nil {
		return "", nil // headless/test: no dialog, treat as cancel
	}
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: title,
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

// WorkspaceMigrate copies or moves the current workspace's identity packages
// into dst, then switches the active workspace to dst and persists the choice.
// When move is true the source packages are removed only after a verified copy
// (workspace-settings requirement: optional migrate on workspace change).
func (a *App) WorkspaceMigrate(dst string, move bool) (WorkspaceInfo, error) {
	ws, err := a.ensureWorkspace()
	if err != nil {
		return WorkspaceInfo{}, err
	}
	if strings.TrimSpace(dst) == "" {
		return WorkspaceInfo{}, errPathRequired
	}
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return WorkspaceInfo{}, err
	}
	if dstAbs == ws.Root() {
		return a.workspaceInfo(ws)
	}
	// A currently-open package would dangle once its directory is moved; drop
	// the session back to the launch page so it re-lists from the new location.
	if a.pkg != nil {
		a.pkg = nil
		a.emit(EventSessionChanged, nil)
	}
	if err := ws.Migrate(dstAbs, move); err != nil {
		return WorkspaceInfo{}, err
	}
	return a.WorkspaceOpen(dstAbs)
}
