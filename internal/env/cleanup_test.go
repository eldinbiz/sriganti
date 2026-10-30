package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanOrphanedPaths_And_ExecuteCleanup(t *testing.T) {
	rootTmp := t.TempDir()
	installDir := filepath.Join(rootTmp, "flutter-portable")
	userProfile := filepath.Join(rootTmp, "UserProfile")
	appData := filepath.Join(rootTmp, "AppData", "Roaming")
	localAppData := filepath.Join(rootTmp, "AppData", "Local")

	dirs := SystemDirs{
		UserProfile:  userProfile,
		AppData:      appData,
		LocalAppData: localAppData,
	}

	// Create portable SDK folder and leftover cache folders
	flutterBin := filepath.Join(installDir, "flutter", "bin")
	pubCacheBin := filepath.Join(installDir, "pub-cache", "bin")
	dartServerDir := filepath.Join(userProfile, ".dartServer")
	flutterConfigDir := filepath.Join(userProfile, ".flutter")
	dartToolDir := filepath.Join(appData, ".dart-tool")

	for _, d := range []string{flutterBin, pubCacheBin, dartServerDir, flutterConfigDir, dartToolDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(flutterBin, "flutter.bat"), []byte("@echo off\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dartServerDir, "cache.bin"), make([]byte, 4096), 0644); err != nil {
		t.Fatal(err)
	}

	// Simulated HKCU\Environment\Path with:
	// 1. Normal system entry
	// 2. Managed portable entries
	// 3. Orphaned non-existent Flutter & Dart entries
	orphanFlutter := `C:\DeletedOldSDK\flutter\bin`
	orphanDart := `D:\StaleTools\dart-sdk\bin`
	userPath := strings.Join([]string{
		`C:\Windows\System32`,
		flutterBin,
		pubCacheBin,
		orphanFlutter,
		orphanDart,
	}, ";")

	store := newMemoryEnvStore(userPath)
	store.vars["PUB_CACHE"] = filepath.Join(installDir, "pub-cache")

	inspection, err := InspectCleanup(store, installDir, dirs)
	if err != nil {
		t.Fatalf("InspectCleanup failed: %v", err)
	}

	if len(inspection.ManagedPathEntries) != 2 {
		t.Fatalf("expected 2 managed PATH entries, got %d: %v", len(inspection.ManagedPathEntries), inspection.ManagedPathEntries)
	}
	if len(inspection.OrphanedPaths) != 2 {
		t.Fatalf("expected 2 orphaned PATH entries, got %d: %+v", len(inspection.OrphanedPaths), inspection.OrphanedPaths)
	}

	// Execute deep cleanup removing managed env, orphaned paths, portable_dir, dart_server, flutter_config, and dart_tool
	res, err := ExecuteCleanup(store, CleanupRequest{
		InstallDir:          installDir,
		RemoveManagedEnv:    true,
		OrphanPathsToRemove: []string{orphanFlutter, orphanDart},
		DeletePortableDir:   true,
		CacheIDsToDelete:    []string{"dart_server", "flutter_config", "dart_tool"},
	}, dirs)
	if err != nil {
		t.Fatalf("ExecuteCleanup failed: %v", err)
	}

	if store.pathVal != `C:\Windows\System32` {
		t.Fatalf("expected only C:\\Windows\\System32 to remain in PATH, got %q", store.pathVal)
	}
	if !res.PubCacheRemoved || store.vars["PUB_CACHE"] != "" {
		t.Fatalf("expected PUB_CACHE to be removed")
	}

	for _, deleted := range []string{installDir, dartServerDir, flutterConfigDir, dartToolDir} {
		if _, err := os.Stat(deleted); !os.IsNotExist(err) {
			t.Fatalf("expected directory %q to be deleted, stat err=%v", deleted, err)
		}
	}
}
