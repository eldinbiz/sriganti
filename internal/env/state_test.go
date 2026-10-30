package env

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectPreflight_NotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	fakeLookPath := func(name string) (string, error) {
		return `C:\Program Files\Git\cmd\git.exe`, nil
	}

	report := InspectPreflight(tmpDir, `C:\Windows\System32`, "", fakeLookPath)
	if report.Status != StatusNotInstalled {
		t.Fatalf("expected status %q, got %q", StatusNotInstalled, report.Status)
	}
	if report.SDKExistsOnDisk {
		t.Fatalf("expected SDKExistsOnDisk=false")
	}
	if !report.GitAvailable || report.GitSource != "system" {
		t.Fatalf("expected system git to be detected, got available=%v source=%q", report.GitAvailable, report.GitSource)
	}
}

func TestInspectPreflight_InstalledAndReady(t *testing.T) {
	tmpDir := t.TempDir()
	flutterBin := filepath.Join(tmpDir, "flutter", "bin")
	dartBin := filepath.Join(tmpDir, "flutter", "bin", "cache", "dart-sdk", "bin")
	pubCache := filepath.Join(tmpDir, "pub-cache")
	pubCacheBin := filepath.Join(pubCache, "bin")

	if err := os.MkdirAll(dartBin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pubCacheBin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flutterBin, "flutter.bat"), []byte("@echo off\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "flutter", "version"), []byte("3.29.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "flutter", "bin", "cache", "dart-sdk", "version"), []byte("3.7.0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := SaveState(tmpDir, PortableState{
		Mode:        ModeFlutter,
		Version:     "3.29.0",
		DartVersion: "3.7.0",
		InstallDir:  tmpDir,
	})
	if err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	userPath := strings.Join([]string{`C:\Windows\System32`, flutterBin, pubCacheBin}, ";")
	fakeLookPath := func(name string) (string, error) {
		return `C:\Program Files\Git\cmd\git.exe`, nil
	}

	report := InspectPreflight(tmpDir, userPath, pubCache, fakeLookPath)
	if report.Status != StatusReady {
		t.Fatalf("expected status %q, got %q (pathAligned=%v pubCacheAligned=%v moved=%q)",
			StatusReady, report.Status, report.PathAligned, report.PubCacheAligned, report.MovedFromDir)
	}
	if report.DetectedMode != ModeFlutter {
		t.Fatalf("expected detected mode %q, got %q", ModeFlutter, report.DetectedMode)
	}
	if report.FlutterVersion != "3.29.0" || report.DartVersion != "3.7.0" {
		t.Fatalf("unexpected versions: flutter=%q dart=%q", report.FlutterVersion, report.DartVersion)
	}
}

func TestInspectPreflight_MovedFolderRelinkNeeded(t *testing.T) {
	oldDir := `C:\OldLocation\flutter-portable`
	newDir := t.TempDir()

	dartBin := filepath.Join(newDir, "dart-sdk", "bin")
	if err := os.MkdirAll(dartBin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dartBin, "dart.exe"), []byte("MZ"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "dart-sdk", "version"), []byte("3.7.2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Simulate state.json recorded when the folder was at oldDir
	if err := SaveState(newDir, PortableState{
		Mode:        ModeDartOnly,
		Version:     "3.7.2",
		DartVersion: "3.7.2",
		InstallDir:  oldDir,
	}); err != nil {
		t.Fatal(err)
	}

	// User PATH still points to oldDir
	userPath := filepath.Join(oldDir, "dart-sdk", "bin") + ";" + filepath.Join(oldDir, "pub-cache", "bin")
	userPubCache := filepath.Join(oldDir, "pub-cache")

	fakeLookPath := func(name string) (string, error) {
		return "", errors.New("git not found")
	}

	report := InspectPreflight(newDir, userPath, userPubCache, fakeLookPath)
	if report.Status != StatusRelinkNeeded {
		t.Fatalf("expected status %q for moved folder, got %q", StatusRelinkNeeded, report.Status)
	}
	if report.MovedFromDir != oldDir {
		t.Fatalf("expected MovedFromDir=%q, got %q", oldDir, report.MovedFromDir)
	}
	if report.DetectedMode != ModeDartOnly {
		t.Fatalf("expected mode %q, got %q", ModeDartOnly, report.DetectedMode)
	}
	if report.GitAvailable {
		t.Fatalf("expected GitAvailable=false when neither mingit nor system git exists")
	}
}
