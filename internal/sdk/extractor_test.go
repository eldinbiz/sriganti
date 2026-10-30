package sdk

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func createTestZip(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExtractZipConcurrent_ValidFlutterAndDartArchives(t *testing.T) {
	tmp := t.TempDir()
	flutterZip := filepath.Join(tmp, "flutter.zip")
	createTestZip(t, flutterZip, map[string]string{
		"flutter/bin/flutter.bat":                     "@echo off\r\n",
		"flutter/version":                             "3.29.0\n",
		"flutter/bin/cache/dart-sdk/bin/dart.exe":     "MZ",
		"flutter/bin/cache/dart-sdk/version":          "3.7.0\n",
	})

	destDir := filepath.Join(tmp, "install-flutter")
	var lastEvent ProgressEvent
	kind, err := ExtractZipConcurrent(context.Background(), flutterZip, destDir, ArchiveKindFlutter, func(ev ProgressEvent) {
		lastEvent = ev
	})
	if err != nil {
		t.Fatalf("ExtractZipConcurrent failed: %v", err)
	}
	if kind != ArchiveKindFlutter {
		t.Fatalf("expected kind %q, got %q", ArchiveKindFlutter, kind)
	}
	if lastEvent.Percent != 100 || lastEvent.FilesProcessed != 4 {
		t.Fatalf("expected 100%% progress with 4 files, got %+v", lastEvent)
	}
	if _, err := os.Stat(filepath.Join(destDir, "flutter", "bin", "flutter.bat")); err != nil {
		t.Fatalf("expected extracted flutter.bat to exist: %v", err)
	}
}

func TestExtractZipConcurrent_RejectsInvalidArchiveBeforeModifyingDest(t *testing.T) {
	tmp := t.TempDir()
	badZip := filepath.Join(tmp, "random.zip")
	createTestZip(t, badZip, map[string]string{
		"README.txt":        "not an sdk archive",
		"subfolder/foo.txt": "hello",
	})

	destDir := filepath.Join(tmp, "untouched-install-dir")
	_, err := ExtractZipConcurrent(context.Background(), badZip, destDir, "", nil)
	if !errors.Is(err, ErrInvalidArchiveStructure) {
		t.Fatalf("expected ErrInvalidArchiveStructure, got %v", err)
	}
	if _, statErr := os.Stat(destDir); !os.IsNotExist(statErr) {
		t.Fatalf("expected destDir %q not to be created when archive is invalid", destDir)
	}
}
