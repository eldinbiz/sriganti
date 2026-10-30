package env

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// OrphanedPathEntry represents a stale or broken Flutter/Dart entry detected in HKCU\Environment\Path.
type OrphanedPathEntry struct {
	RawPath      string `json:"rawPath"`
	ExpandedPath string `json:"expandedPath"`
	Reason       string `json:"reason"`
}

// CacheTargetItem represents a directory on disk that can be selectively deleted during uninstall.
type CacheTargetItem struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Path        string `json:"path"`
	Exists      bool   `json:"exists"`
	SizeBytes   int64  `json:"sizeBytes"`
	SizeHuman   string `json:"sizeHuman"`
	DefaultTick bool   `json:"defaultTick"`
}

// CleanupInspection holds all registry and disk findings for the Uninstall & Deep Clean view.
type CleanupInspection struct {
	InstallDir         string              `json:"installDir"`
	ManagedPathEntries []string            `json:"managedPathEntries"`
	PubCacheVarValue   string              `json:"pubCacheVarValue"`
	OrphanedPaths      []OrphanedPathEntry `json:"orphanedPaths"`
	DiskTargets        []CacheTargetItem   `json:"diskTargets"`
}

// CleanupRequest specifies which registry entries and disk folders the user chose to remove.
type CleanupRequest struct {
	InstallDir          string   `json:"installDir"`
	RemoveManagedEnv    bool     `json:"removeManagedEnv"`
	OrphanPathsToRemove []string `json:"orphanPathsToRemove"`
	DeletePortableDir   bool     `json:"deletePortableDir"`
	CacheIDsToDelete    []string `json:"cacheIdsToDelete"`
}

// CleanupResult summarizes completed cleanup actions.
type CleanupResult struct {
	RemovedPaths       []string `json:"removedPaths"`
	PubCacheRemoved    bool     `json:"pubCacheRemoved"`
	DeletedDirectories []string `json:"deletedDirectories"`
	TerminatedProcs    int      `json:"terminatedProcs"`
	Warnings           []string `json:"warnings,omitempty"`
}

// SystemDirs holds host user profile paths so unit tests can override them with temp directories.
type SystemDirs struct {
	UserProfile  string
	AppData      string
	LocalAppData string
}

// DefaultSystemDirs resolves %USERPROFILE%, %APPDATA%, and %LOCALAPPDATA% on the current Windows host.
func DefaultSystemDirs() SystemDirs {
	home, _ := os.UserHomeDir()
	appData := os.Getenv("APPDATA")
	if appData == "" && home != "" {
		appData = filepath.Join(home, "AppData", "Roaming")
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" && home != "" {
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	return SystemDirs{
		UserProfile:  home,
		AppData:      appData,
		LocalAppData: localAppData,
	}
}

var winEnvPattern = regexp.MustCompile(`%([^%]+)%`)

// ExpandWindowsEnvVars expands %VAR% tokens in a Windows path string.
func ExpandWindowsEnvVars(raw string, dirs SystemDirs) string {
	return winEnvPattern.ReplaceAllStringFunc(raw, func(token string) string {
		name := strings.Trim(token, "%")
		switch strings.ToUpper(name) {
		case "USERPROFILE":
			if dirs.UserProfile != "" {
				return dirs.UserProfile
			}
		case "APPDATA":
			if dirs.AppData != "" {
				return dirs.AppData
			}
		case "LOCALAPPDATA":
			if dirs.LocalAppData != "" {
				return dirs.LocalAppData
			}
		}
		if val := os.Getenv(name); val != "" {
			return val
		}
		return token
	})
}

// IsFlutterOrDartRelatedPath returns true if a PATH segment looks like a Flutter, Dart, Pub cache, or portable MinGit path.
func IsFlutterOrDartRelatedPath(pathEntry string) bool {
	norm := NormalizeWindowsPath(pathEntry)
	if norm == "" {
		return false
	}
	suffixes := []string{
		`\flutter\bin`,
		`\dart-sdk\bin`,
		`\pub\cache\bin`,
		`\pub-cache\bin`,
		`\flutter-portable\mingit\cmd`,
	}
	for _, s := range suffixes {
		if strings.HasSuffix(norm, s) {
			return true
		}
	}
	return strings.Contains(norm, `\flutter\bin\`) || strings.Contains(norm, `\dart-sdk\bin\`)
}

// ScanOrphanedPaths inspects userPath and classifies managed portable entries vs orphaned Flutter/Dart entries.
func ScanOrphanedPaths(userPath string, installDir string, dirs SystemDirs, dirExistsFn func(string) bool) (managed []string, orphans []OrphanedPathEntry) {
	if dirExistsFn == nil {
		dirExistsFn = func(p string) bool {
			info, err := os.Stat(p)
			return err == nil && info.IsDir()
		}
	}

	cleanDir := filepath.Clean(installDir)
	managedSet := make(map[string]bool)
	for _, mp := range ManagedPathsForDir(cleanDir) {
		managedSet[NormalizeWindowsPath(mp)] = true
	}
	if st, err := LoadState(cleanDir); err == nil && st != nil {
		for _, rp := range st.RegisteredPaths {
			managedSet[NormalizeWindowsPath(rp)] = true
		}
		if st.InstallDir != "" && !EqualNormalizedPath(st.InstallDir, cleanDir) {
			for _, oldMp := range ManagedPathsForDir(st.InstallDir) {
				managedSet[NormalizeWindowsPath(oldMp)] = true
			}
		}
	}

	for _, entry := range SplitPathEntries(userPath) {
		norm := NormalizeWindowsPath(entry)
		if norm == "" {
			continue
		}
		if managedSet[norm] {
			managed = append(managed, entry)
			continue
		}
		if IsFlutterOrDartRelatedPath(entry) {
			expanded := ExpandWindowsEnvVars(entry, dirs)
			if !dirExistsFn(expanded) {
				orphans = append(orphans, OrphanedPathEntry{
					RawPath:      entry,
					ExpandedPath: expanded,
					Reason:       "Target directory no longer exists on disk",
				})
			} else {
				orphans = append(orphans, OrphanedPathEntry{
					RawPath:      entry,
					ExpandedPath: expanded,
					Reason:       "External/leftover Flutter or Dart PATH entry",
				})
			}
		}
	}
	return managed, orphans
}

// InspectCleanup gathers all managed registry entries, orphaned PATH entries, and disk cache targets with sizes.
func InspectCleanup(store EnvironmentStore, installDir string, dirs SystemDirs) (CleanupInspection, error) {
	cleanDir := filepath.Clean(installDir)
	userPath, err := store.GetUserPath()
	if err != nil {
		return CleanupInspection{}, err
	}
	pubCacheVal, _ := store.GetUserVar("PUB_CACHE")

	managed, orphans := ScanOrphanedPaths(userPath, cleanDir, dirs, nil)

	targets := buildCacheTargets(cleanDir, dirs)

	return CleanupInspection{
		InstallDir:         cleanDir,
		ManagedPathEntries: managed,
		PubCacheVarValue:   pubCacheVal,
		OrphanedPaths:      orphans,
		DiskTargets:        targets,
	}, nil
}

func buildCacheTargets(installDir string, dirs SystemDirs) []CacheTargetItem {
	defs := []struct {
		id          string
		label       string
		path        string
		defaultTick bool
	}{
		{
			id:          "portable_dir",
			label:       "Portable SDK, Pub Cache & MinGit Folder",
			path:        installDir,
			defaultTick: true,
		},
		{
			id:          "dart_server",
			label:       "Dart Analysis Server Cache (%USERPROFILE%\\.dartServer)",
			path:        filepath.Join(dirs.UserProfile, ".dartServer"),
			defaultTick: true,
		},
		{
			id:          "flutter_config",
			label:       "Flutter User Settings (%USERPROFILE%\\.flutter)",
			path:        filepath.Join(dirs.UserProfile, ".flutter"),
			defaultTick: true,
		},
		{
			id:          "dart_tool",
			label:       "Dart Tooling State (%APPDATA%\\.dart-tool)",
			path:        filepath.Join(dirs.AppData, ".dart-tool"),
			defaultTick: true,
		},
		{
			id:          "global_pub_cache",
			label:       "Global Windows Pub Cache (%LOCALAPPDATA%\\Pub\\Cache)",
			path:        filepath.Join(dirs.LocalAppData, "Pub", "Cache"),
			defaultTick: false,
		},
	}

	items := make([]CacheTargetItem, 0, len(defs))
	for _, d := range defs {
		if d.path == "" {
			continue
		}
		exists, size := CalculateDirSize(d.path)
		// Also include companion files for flutter_config (.flutter_settings) or dart_tool (.dart)
		if d.id == "flutter_config" && dirs.UserProfile != "" {
			if ex2, sz2 := CalculateDirSize(filepath.Join(dirs.UserProfile, ".flutter_settings")); ex2 {
				exists = true
				size += sz2
			}
		}
		if d.id == "dart_tool" && dirs.UserProfile != "" {
			if ex2, sz2 := CalculateDirSize(filepath.Join(dirs.UserProfile, ".dart")); ex2 {
				exists = true
				size += sz2
			}
		}
		items = append(items, CacheTargetItem{
			ID:          d.id,
			Label:       d.label,
			Path:        d.path,
			Exists:      exists,
			SizeBytes:   size,
			SizeHuman:   FormatBytes(size),
			DefaultTick: d.defaultTick && exists,
		})
	}
	return items
}

// ExecuteCleanup performs selective environment variable cleanup and filesystem deletion.
func ExecuteCleanup(store EnvironmentStore, req CleanupRequest, dirs SystemDirs) (CleanupResult, error) {
	var res CleanupResult
	cleanDir := filepath.Clean(req.InstallDir)

	// 1. Clean HKCU\Environment (managed entries and/or selected orphaned entries)
	if req.RemoveManagedEnv || len(req.OrphanPathsToRemove) > 0 {
		currentPath, err := store.GetUserPath()
		if err != nil {
			return res, err
		}
		targetInstallDir := ""
		if req.RemoveManagedEnv {
			targetInstallDir = cleanDir
		}

		var extra []string
		extra = append(extra, req.OrphanPathsToRemove...)
		if req.RemoveManagedEnv {
			if st, err := LoadState(cleanDir); err == nil && st != nil {
				extra = append(extra, st.RegisteredPaths...)
				if st.InstallDir != "" {
					extra = append(extra, ManagedPathsForDir(st.InstallDir)...)
				}
			}
		}

		updatedPath, removed := RemovePortableEntriesFromPath(currentPath, targetInstallDir, extra)
		if err := store.SetUserPath(updatedPath); err != nil {
			return res, err
		}
		res.RemovedPaths = removed

		if req.RemoveManagedEnv {
			pubVal, _ := store.GetUserVar("PUB_CACHE")
			if pubVal != "" {
				_ = store.DeleteUserVar("PUB_CACHE")
				res.PubCacheRemoved = true
			}
		}
		_ = store.BroadcastSettingChange()
	}

	// 2. Terminate lingering dart.exe / dartaotruntime.exe processes inside installDir before deleting files
	selectedSet := make(map[string]bool)
	for _, id := range req.CacheIDsToDelete {
		selectedSet[id] = true
	}
	if req.DeletePortableDir {
		selectedSet["portable_dir"] = true
	}

	if selectedSet["portable_dir"] || selectedSet["dart_server"] {
		res.TerminatedProcs = TerminateSDKProcessesInDir(cleanDir)
	}

	// 3. Delete selected directories
	deletePath := func(p string) {
		if p == "" {
			return
		}
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return
		}
		if err := os.RemoveAll(p); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Failed to remove %s: %v", p, err))
		} else {
			res.DeletedDirectories = append(res.DeletedDirectories, p)
		}
	}

	if selectedSet["portable_dir"] {
		deletePath(cleanDir)
	}
	if selectedSet["dart_server"] && dirs.UserProfile != "" {
		deletePath(filepath.Join(dirs.UserProfile, ".dartServer"))
	}
	if selectedSet["flutter_config"] && dirs.UserProfile != "" {
		deletePath(filepath.Join(dirs.UserProfile, ".flutter"))
		deletePath(filepath.Join(dirs.UserProfile, ".flutter_settings"))
	}
	if selectedSet["dart_tool"] {
		if dirs.AppData != "" {
			deletePath(filepath.Join(dirs.AppData, ".dart-tool"))
		}
		if dirs.UserProfile != "" {
			deletePath(filepath.Join(dirs.UserProfile, ".dart"))
		}
	}
	if selectedSet["global_pub_cache"] && dirs.LocalAppData != "" {
		deletePath(filepath.Join(dirs.LocalAppData, "Pub", "Cache"))
	}

	return res, nil
}

// TerminateSDKProcessesInDir terminates running dart/flutter/git processes whose executable resides inside installDir.
func TerminateSDKProcessesInDir(installDir string) int {
	normDir := NormalizeWindowsPath(installDir)
	if normDir == "" {
		return 0
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snapshot)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snapshot, &pe); err != nil {
		return 0
	}

	terminated := 0
	for {
		exeName := strings.ToLower(windows.UTF16ToString(pe.ExeFile[:]))
		if exeName == "dart.exe" || exeName == "dartaotruntime.exe" || exeName == "flutter.exe" || exeName == "git.exe" {
			if procImg, err := queryProcessImagePath(pe.ProcessID); err == nil {
				normImg := NormalizeWindowsPath(procImg)
				if strings.HasPrefix(normImg, normDir+`\`) {
					if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pe.ProcessID); err == nil {
						if windows.TerminateProcess(h, 1) == nil {
							terminated++
						}
						_ = windows.CloseHandle(h)
					}
				}
			}
		}
		if err := windows.Process32Next(snapshot, &pe); err != nil {
			break
		}
	}
	return terminated
}

func queryProcessImagePath(pid uint32) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, windows.MAX_PATH*2)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:size]), nil
}

// CalculateDirSize returns whether the file/directory exists and its total size in bytes.
func CalculateDirSize(targetPath string) (bool, int64) {
	info, err := os.Stat(targetPath)
	if err != nil {
		return false, 0
	}
	if !info.IsDir() {
		return true, info.Size()
	}
	var total int64
	_ = filepath.WalkDir(targetPath, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		if fi, err := d.Info(); err == nil {
			total += fi.Size()
		}
		return nil
	})
	return true, total
}

// FormatBytes formats a byte count into a human-readable string (e.g., "145.2 MB").
func FormatBytes(b int64) string {
	if b <= 0 {
		return "0 B"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
