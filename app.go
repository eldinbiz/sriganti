package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"flutter-portable-manager/internal/env"
	"flutter-portable-manager/internal/sdk"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct binds backend methods to the Wails frontend.
type App struct {
	ctx         context.Context
	envStore    env.EnvironmentStore
	provisioner *sdk.Provisioner
	systemDirs  env.SystemDirs
	lookPath    func(string) (string, error)

	cancelMu sync.Mutex
	cancelFn context.CancelFunc
}

// NewApp creates a new App application struct with production Windows dependencies.
func NewApp() *App {
	store := env.NewWindowsRegistryStore()
	prov := sdk.NewProvisioner(store)
	app := &App{
		envStore:    store,
		provisioner: prov,
		systemDirs:  env.DefaultSystemDirs(),
		lookPath:    exec.LookPath,
	}
	prov.OnProgress = app.emitProgress
	prov.OnLog = app.emitLog
	return app
}

// startup is called when the Wails application starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) baseContext() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) emitProgress(ev sdk.ProgressEvent) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "sdk:progress", ev)
	}
}

func (a *App) emitLog(msg string) {
	stamp := time.Now().Format("15:04:05")
	line := fmt.Sprintf("[%s] %s", stamp, msg)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "sdk:log", line)
	}
}

// GetDefaultInstallDir returns the default portable directory (<exe_dir>\flutter-portable).
func (a *App) GetDefaultInstallDir() string {
	return env.DefaultInstallDir()
}

// GetPreflightStatus inspects disk SDK state, Git availability, and HKCU\Environment alignment.
func (a *App) GetPreflightStatus(installDir string) (env.PreflightReport, error) {
	if strings.TrimSpace(installDir) == "" {
		installDir = env.DefaultInstallDir()
	}
	userPath, err := a.envStore.GetUserPath()
	if err != nil {
		return env.PreflightReport{}, err
	}
	pubCache, _ := a.envStore.GetUserVar("PUB_CACHE")
	return env.InspectPreflight(installDir, userPath, pubCache, a.lookPath), nil
}

// FetchReleases queries Google's official release feed for Flutter + Dart and Dart-Only options.
func (a *App) FetchReleases() (*sdk.ReleaseCatalog, error) {
	ctx, cancel := context.WithTimeout(a.baseContext(), 30*time.Second)
	defer cancel()
	return a.provisioner.ReleaseClient.FetchCatalog(ctx, 25)
}

// SelectInstallDirectory opens a native Windows folder picker dialog.
func (a *App) SelectInstallDirectory(currentDir string) (string, error) {
	if a.ctx == nil {
		return currentDir, nil
	}
	defaultDir := strings.TrimSpace(currentDir)
	if defaultDir != "" {
		defaultDir = filepath.Dir(filepath.Clean(defaultDir))
	}
	selected, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Select Portable SDK Directory",
		DefaultDirectory: defaultDir,
	})
	if err != nil {
		return "", err
	}
	return selected, nil
}

// SelectZipArchive opens a native Windows file dialog for picking a local .zip SDK archive.
func (a *App) SelectZipArchive() (string, error) {
	if a.ctx == nil {
		return "", nil
	}
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Flutter or Dart SDK .zip Archive",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "ZIP Archives (*.zip)",
				Pattern:     "*.zip",
			},
		},
	})
}

// InstallSDK runs the end-to-end provisioning and user environment registration workflow.
func (a *App) InstallSDK(req sdk.ProvisionRequest) (*sdk.ProvisionResult, error) {
	a.cancelMu.Lock()
	if a.cancelFn != nil {
		a.cancelFn()
	}
	opCtx, cancel := context.WithCancel(a.baseContext())
	a.cancelFn = cancel
	a.cancelMu.Unlock()

	defer func() {
		a.cancelMu.Lock()
		a.cancelFn = nil
		a.cancelMu.Unlock()
	}()

	a.emitLog(fmt.Sprintf("Starting portable installation (mode=%s, source=%s)...", req.Mode, req.SourceType))
	res, err := a.provisioner.Provision(opCtx, req)
	if err != nil {
		a.emitLog(fmt.Sprintf("ERROR: %v", err))
		a.emitProgress(sdk.ProgressEvent{
			Stage:   "error",
			Message: err.Error(),
		})
		return nil, err
	}
	a.emitLog(fmt.Sprintf("Completed! Installed %s (%s) at %s", res.Mode, res.Version, res.InstallDir))
	return res, nil
}

// CancelOperation cancels an in-progress download or archive extraction.
func (a *App) CancelOperation() {
	a.cancelMu.Lock()
	defer a.cancelMu.Unlock()
	if a.cancelFn != nil {
		a.emitLog("Cancelling active operation...")
		a.cancelFn()
		a.cancelFn = nil
	}
}

// RegisterPathOnly links an existing portable SDK directory into HKCU\Environment without downloading.
func (a *App) RegisterPathOnly(installDir string, mode string) ([]string, error) {
	if strings.TrimSpace(installDir) == "" {
		installDir = env.DefaultInstallDir()
	}
	a.emitLog(fmt.Sprintf("Registering existing portable SDK at %s in HKCU\\Environment...", installDir))
	registered, err := env.RegisterPortableEnvironment(a.envStore, installDir, mode)
	if err != nil {
		a.emitLog(fmt.Sprintf("ERROR: %v", err))
		return nil, err
	}
	a.emitLog(fmt.Sprintf("Registered %d user PATH entries and broadcasted WM_SETTINGCHANGE.", len(registered)))
	return registered, nil
}

// UnregisterPathOnly removes portable SDK entries from HKCU\Environment while keeping all files on disk.
func (a *App) UnregisterPathOnly(installDir string) ([]string, error) {
	if strings.TrimSpace(installDir) == "" {
		installDir = env.DefaultInstallDir()
	}
	a.emitLog(fmt.Sprintf("Unregistering HKCU\\Environment entries for %s (keeping SDK files)...", installDir))
	removed, err := env.UnregisterPortableEnvironment(a.envStore, installDir, nil)
	if err != nil {
		a.emitLog(fmt.Sprintf("ERROR: %v", err))
		return nil, err
	}
	a.emitLog(fmt.Sprintf("Removed %d PATH entries from HKCU\\Environment.", len(removed)))
	return removed, nil
}

// InspectCleanup scans HKCU\Environment for managed & orphaned Flutter/Dart paths and calculates cache sizes.
func (a *App) InspectCleanup(installDir string) (env.CleanupInspection, error) {
	if strings.TrimSpace(installDir) == "" {
		installDir = env.DefaultInstallDir()
	}
	return env.InspectCleanup(a.envStore, installDir, a.systemDirs)
}

// ExecuteCleanup performs selective removal of HKCU\Environment entries, orphaned paths, SDK folder, and caches.
func (a *App) ExecuteCleanup(req env.CleanupRequest) (env.CleanupResult, error) {
	if strings.TrimSpace(req.InstallDir) == "" {
		req.InstallDir = env.DefaultInstallDir()
	}
	a.emitLog(fmt.Sprintf("Starting cleanup for %s...", req.InstallDir))
	res, err := env.ExecuteCleanup(a.envStore, req, a.systemDirs)
	if err != nil {
		a.emitLog(fmt.Sprintf("ERROR during cleanup: %v", err))
		return res, err
	}
	a.emitLog(fmt.Sprintf("Cleanup finished: removed %d PATH entries, deleted %d directories.", len(res.RemovedPaths), len(res.DeletedDirectories)))
	return res, nil
}
