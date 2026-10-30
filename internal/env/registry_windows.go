package env

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	wmSettingChange = 0x001A
	hwndBroadcast   = 0xFFFF
	smtoAbortIfHung = 0x0002
)

// EnvironmentStore abstracts HKCU\Environment operations for safe testing and Windows Registry access.
type EnvironmentStore interface {
	GetUserPath() (string, error)
	SetUserPath(pathValue string) error
	GetUserVar(name string) (string, error)
	SetUserVar(name, value string) error
	DeleteUserVar(name string) error
	BroadcastSettingChange() error
}

// WindowsRegistryStore mutates HKCU\Environment without triggering UAC.
type WindowsRegistryStore struct{}

// NewWindowsRegistryStore returns a production HKCU\Environment store.
func NewWindowsRegistryStore() *WindowsRegistryStore {
	return &WindowsRegistryStore{}
}

func (s *WindowsRegistryStore) GetUserPath() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()

	val, _, err := k.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	return val, err
}

func (s *WindowsRegistryStore) SetUserPath(pathValue string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	_, valType, err := k.GetStringValue("Path")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	if valType == registry.SZ {
		return k.SetStringValue("Path", pathValue)
	}
	return k.SetExpandStringValue("Path", pathValue)
}

func (s *WindowsRegistryStore) GetUserVar(name string) (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()

	val, _, err := k.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	return val, err
}

func (s *WindowsRegistryStore) SetUserVar(name, value string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	return k.SetStringValue(name, value)
}

func (s *WindowsRegistryStore) DeleteUserVar(name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	err = k.DeleteValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

// BroadcastSettingChange broadcasts WM_SETTINGCHANGE ("Environment") to all top-level windows.
func (s *WindowsRegistryStore) BroadcastSettingChange() error {
	user32 := windows.NewLazySystemDLL("user32.dll")
	sendMessageTimeout := user32.NewProc("SendMessageTimeoutW")

	envPtr, err := syscall.UTF16PtrFromString("Environment")
	if err != nil {
		return err
	}

	var result uintptr
	r1, _, lastErr := sendMessageTimeout.Call(
		 uintptr(hwndBroadcast),
		 uintptr(wmSettingChange),
		 0,
		 uintptr(unsafe.Pointer(envPtr)),
		 uintptr(smtoAbortIfHung),
		 uintptr(5000),
		 uintptr(unsafe.Pointer(&result)),
	)
	if r1 == 0 && lastErr != nil && lastErr != syscall.Errno(0) {
		return lastErr
	}
	return nil
}

// BuildRegisteredUserPath updates a semicolon-delimited PATH string for the given installDir and mode.
// It swaps flutter\bin <-> dart-sdk\bin on mode changes, strips oldInstallDir entries if moved, and deduplicates case-insensitively.
func BuildRegisteredUserPath(currentPath string, installDir string, mode string, includeMinGit bool, oldInstallDir string) (newPath string, registered []string) {
	cleanDir := filepath.Clean(installDir)

	var activeSDKBin, inactiveSDKBin string
	if mode == ModeDartOnly {
		activeSDKBin = filepath.Join(cleanDir, "dart-sdk", "bin")
		inactiveSDKBin = filepath.Join(cleanDir, "flutter", "bin")
	} else {
		activeSDKBin = filepath.Join(cleanDir, "flutter", "bin")
		inactiveSDKBin = filepath.Join(cleanDir, "dart-sdk", "bin")
	}
	pubCacheBin := filepath.Join(cleanDir, "pub-cache", "bin")
	minGitCmd := filepath.Join(cleanDir, "mingit", "cmd")

	// Entries that should be removed before adding active entries (e.g. inactive mode or moved folder)
	toStrip := map[string]bool{
		NormalizeWindowsPath(inactiveSDKBin): true,
	}
	if !includeMinGit {
		toStrip[NormalizeWindowsPath(minGitCmd)] = true
	}
	if oldInstallDir != "" && !EqualNormalizedPath(oldInstallDir, cleanDir) {
		oldClean := filepath.Clean(oldInstallDir)
		toStrip[NormalizeWindowsPath(filepath.Join(oldClean, "flutter", "bin"))] = true
		toStrip[NormalizeWindowsPath(filepath.Join(oldClean, "dart-sdk", "bin"))] = true
		toStrip[NormalizeWindowsPath(filepath.Join(oldClean, "pub-cache", "bin"))] = true
		toStrip[NormalizeWindowsPath(filepath.Join(oldClean, "mingit", "cmd"))] = true
	}

	seen := make(map[string]bool)
	result := make([]string, 0)

	for _, entry := range SplitPathEntries(currentPath) {
		norm := NormalizeWindowsPath(entry)
		if norm == "" || toStrip[norm] || seen[norm] {
			continue
		}
		seen[norm] = true
		result = append(result, entry)
	}

	desired := []string{activeSDKBin, pubCacheBin}
	if includeMinGit {
		desired = append(desired, minGitCmd)
	}

	for _, d := range desired {
		norm := NormalizeWindowsPath(d)
		if !seen[norm] {
			seen[norm] = true
			result = append(result, d)
		}
		registered = append(registered, d)
	}

	return strings.Join(result, ";"), registered
}

// RemovePortableEntriesFromPath removes portable SDK paths for installDir and any additional specified paths.
func RemovePortableEntriesFromPath(currentPath string, installDir string, additionalRemove []string) (newPath string, removed []string) {
	toRemove := make(map[string]bool)
	if strings.TrimSpace(installDir) != "" {
		cleanDir := filepath.Clean(installDir)
		for _, candidate := range ManagedPathsForDir(cleanDir) {
			toRemove[NormalizeWindowsPath(candidate)] = true
		}
	}
	for _, extra := range additionalRemove {
		norm := NormalizeWindowsPath(extra)
		if norm != "" {
			toRemove[norm] = true
		}
	}

	seen := make(map[string]bool)
	kept := make([]string, 0)
	for _, entry := range SplitPathEntries(currentPath) {
		norm := NormalizeWindowsPath(entry)
		if norm == "" {
			continue
		}
		if toRemove[norm] {
			removed = append(removed, entry)
			continue
		}
		if !seen[norm] {
			seen[norm] = true
			kept = append(kept, entry)
		}
	}
	return strings.Join(kept, ";"), removed
}

// ManagedPathsForDir returns all possible PATH directories managed inside a portable installDir.
func ManagedPathsForDir(installDir string) []string {
	cleanDir := filepath.Clean(installDir)
	return []string{
		filepath.Join(cleanDir, "flutter", "bin"),
		filepath.Join(cleanDir, "dart-sdk", "bin"),
		filepath.Join(cleanDir, "pub-cache", "bin"),
		filepath.Join(cleanDir, "mingit", "cmd"),
	}
}

// RegisterPortableEnvironment configures HKCU\Environment (Path & PUB_CACHE), updates state.json, and broadcasts WM_SETTINGCHANGE.
func RegisterPortableEnvironment(store EnvironmentStore, installDir string, mode string) ([]string, error) {
	cleanDir := filepath.Clean(installDir)
	if mode == "" {
		detectedMode, _, _, exists := DetectDiskSDK(cleanDir)
		if !exists {
			return nil, errors.New("no Flutter or Dart SDK found in target directory to register")
		}
		mode = detectedMode
	}

	pubCacheDir := filepath.Join(cleanDir, "pub-cache")
	pubCacheBin := filepath.Join(pubCacheDir, "bin")
	if err := os.MkdirAll(pubCacheBin, 0755); err != nil {
		return nil, err
	}

	minGitExe := filepath.Join(cleanDir, "mingit", "cmd", "git.exe")
	includeMinGit := fileExists(minGitExe)

	var oldInstallDir string
	st, err := LoadState(cleanDir)
	if err == nil && st != nil && st.InstallDir != "" && !EqualNormalizedPath(st.InstallDir, cleanDir) {
		oldInstallDir = st.InstallDir
	}

	currentPath, err := store.GetUserPath()
	if err != nil {
		return nil, err
	}

	updatedPath, registered := BuildRegisteredUserPath(currentPath, cleanDir, mode, includeMinGit, oldInstallDir)
	if err := store.SetUserPath(updatedPath); err != nil {
		return nil, err
	}
	if err := store.SetUserVar("PUB_CACHE", pubCacheDir); err != nil {
		return nil, err
	}

	// Update state.json to reflect current installDir and registered paths
	_, flutterVer, dartVer, _ := DetectDiskSDK(cleanDir)
	newState := PortableState{
		Mode:            mode,
		Version:         flutterVer,
		DartVersion:     dartVer,
		InstallDir:      cleanDir,
		MinGitInstalled: includeMinGit,
		RegisteredPaths: registered,
	}
	if st != nil {
		if newState.Version == "" {
			newState.Version = st.Version
		}
		if newState.DartVersion == "" {
			newState.DartVersion = st.DartVersion
		}
		newState.InstalledAt = st.InstalledAt
	}
	if newState.Mode == ModeDartOnly && newState.Version == "" {
		newState.Version = newState.DartVersion
	}
	_ = SaveState(cleanDir, newState)

	_ = store.BroadcastSettingChange()
	return registered, nil
}

// UnregisterPortableEnvironment removes portable SDK entries and PUB_CACHE from HKCU\Environment while keeping SDK files.
func UnregisterPortableEnvironment(store EnvironmentStore, installDir string, extraOrphanPaths []string) ([]string, error) {
	cleanDir := filepath.Clean(installDir)

	var extra []string
	extra = append(extra, extraOrphanPaths...)
	if st, err := LoadState(cleanDir); err == nil && st != nil {
		extra = append(extra, st.RegisteredPaths...)
		if st.InstallDir != "" && !EqualNormalizedPath(st.InstallDir, cleanDir) {
			extra = append(extra, ManagedPathsForDir(st.InstallDir)...)
		}
	}

	currentPath, err := store.GetUserPath()
	if err != nil {
		return nil, err
	}

	updatedPath, removed := RemovePortableEntriesFromPath(currentPath, cleanDir, extra)
	if err := store.SetUserPath(updatedPath); err != nil {
		return nil, err
	}

	currentPubCache, err := store.GetUserVar("PUB_CACHE")
	if err == nil && currentPubCache != "" {
		expectedPubCache := filepath.Join(cleanDir, "pub-cache")
		if EqualNormalizedPath(currentPubCache, expectedPubCache) {
			_ = store.DeleteUserVar("PUB_CACHE")
		} else if st, err := LoadState(cleanDir); err == nil && st != nil && st.InstallDir != "" {
			if EqualNormalizedPath(currentPubCache, filepath.Join(st.InstallDir, "pub-cache")) {
				_ = store.DeleteUserVar("PUB_CACHE")
			}
		}
	}

	_ = store.BroadcastSettingChange()
	return removed, nil
}
