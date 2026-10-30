package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memoryEnvStore struct {
	pathVal    string
	vars       map[string]string
	broadcasts int
}

func newMemoryEnvStore(initialPath string) *memoryEnvStore {
	return &memoryEnvStore{
		pathVal: initialPath,
		vars:    make(map[string]string),
	}
}

func (m *memoryEnvStore) GetUserPath() (string, error) {
	return m.pathVal, nil
}

func (m *memoryEnvStore) SetUserPath(pathValue string) error {
	m.pathVal = pathValue
	return nil
}

func (m *memoryEnvStore) GetUserVar(name string) (string, error) {
	return m.vars[name], nil
}

func (m *memoryEnvStore) SetUserVar(name, value string) error {
	m.vars[name] = value
	return nil
}

func (m *memoryEnvStore) DeleteUserVar(name string) error {
	delete(m.vars, name)
	return nil
}

func (m *memoryEnvStore) BroadcastSettingChange() error {
	m.broadcasts++
	return nil
}

func TestBuildRegisteredUserPath_DeduplicationAndModeSwap(t *testing.T) {
	installDir := `C:\Portable\flutter-portable`
	initialPath := `C:\Windows\System32;c:\portable\flutter-portable\dart-sdk\bin;C:\Windows\System32`

	// Switching from Dart-Only to Full Flutter + MinGit should replace dart-sdk\bin with flutter\bin and deduplicate System32
	updated, registered := BuildRegisteredUserPath(initialPath, installDir, ModeFlutter, true, "")

	if ContainsNormalizedPath(updated, filepath.Join(installDir, "dart-sdk", "bin")) {
		t.Fatalf("expected dart-sdk\\bin to be swapped out when registering ModeFlutter, got PATH=%q", updated)
	}
	if !ContainsNormalizedPath(updated, filepath.Join(installDir, "flutter", "bin")) {
		t.Fatalf("expected flutter\\bin in PATH, got %q", updated)
	}
	if !ContainsNormalizedPath(updated, filepath.Join(installDir, "pub-cache", "bin")) {
		t.Fatalf("expected pub-cache\\bin in PATH, got %q", updated)
	}
	if !ContainsNormalizedPath(updated, filepath.Join(installDir, "mingit", "cmd")) {
		t.Fatalf("expected mingit\\cmd in PATH, got %q", updated)
	}
	if len(registered) != 3 {
		t.Fatalf("expected 3 registered paths, got %d", len(registered))
	}

	// Ensure C:\Windows\System32 appears only once
	countSys32 := 0
	for _, p := range SplitPathEntries(updated) {
		if EqualNormalizedPath(p, `C:\Windows\System32`) {
			countSys32++
		}
	}
	if countSys32 != 1 {
		t.Fatalf("expected System32 to appear once after deduplication, got %d", countSys32)
	}
}

func TestRegisterAndUnregisterPortableEnvironment_MovedFolder(t *testing.T) {
	oldDir := `C:\OldUSB\flutter-portable`
	newDir := t.TempDir()

	flutterBin := filepath.Join(newDir, "flutter", "bin")
	if err := os.MkdirAll(flutterBin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flutterBin, "flutter.bat"), []byte("@echo off\r\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Save state pointing to oldDir to simulate moved folder
	if err := SaveState(newDir, PortableState{
		Mode:       ModeFlutter,
		Version:    "3.29.0",
		InstallDir: oldDir,
	}); err != nil {
		t.Fatal(err)
	}

	initialPath := strings.Join([]string{
		`C:\Users\dev\bin`,
		filepath.Join(oldDir, "flutter", "bin"),
		filepath.Join(oldDir, "pub-cache", "bin"),
	}, ";")

	store := newMemoryEnvStore(initialPath)
	store.vars["PUB_CACHE"] = filepath.Join(oldDir, "pub-cache")

	// Relink to newDir
	_, err := RegisterPortableEnvironment(store, newDir, ModeFlutter)
	if err != nil {
		t.Fatalf("RegisterPortableEnvironment failed: %v", err)
	}

	if ContainsNormalizedPath(store.pathVal, filepath.Join(oldDir, "flutter", "bin")) {
		t.Fatalf("expected oldDir flutter\\bin to be stripped, got %q", store.pathVal)
	}
	if !ContainsNormalizedPath(store.pathVal, flutterBin) {
		t.Fatalf("expected newDir flutter\\bin to be registered, got %q", store.pathVal)
	}
	if !EqualNormalizedPath(store.vars["PUB_CACHE"], filepath.Join(newDir, "pub-cache")) {
		t.Fatalf("expected PUB_CACHE updated to newDir, got %q", store.vars["PUB_CACHE"])
	}
	if store.broadcasts == 0 {
		t.Fatalf("expected WM_SETTINGCHANGE broadcast to be triggered")
	}

	// Now Unregister while keeping files
	removed, err := UnregisterPortableEnvironment(store, newDir, nil)
	if err != nil {
		t.Fatalf("UnregisterPortableEnvironment failed: %v", err)
	}
	if len(removed) < 2 {
		t.Fatalf("expected at least 2 paths removed, got %v", removed)
	}
	if ContainsNormalizedPath(store.pathVal, flutterBin) {
		t.Fatalf("expected flutter\\bin removed from PATH, got %q", store.pathVal)
	}
	if store.vars["PUB_CACHE"] != "" {
		t.Fatalf("expected PUB_CACHE deleted from user env, got %q", store.vars["PUB_CACHE"])
	}
	// Verify SDK files still exist on disk
	if !fileExists(filepath.Join(flutterBin, "flutter.bat")) {
		t.Fatalf("expected SDK files to remain on disk after UnregisterPortableEnvironment")
	}
}
