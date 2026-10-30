package main

import (
	"io/fs"
	"os"
	"strings"
	"testing"
)

func TestEmbeddedAssetsExist(t *testing.T) {
	required := []string{
		"frontend/dist/index.html",
		"frontend/dist/assets/tailwindcss.js",
		"frontend/dist/assets/alpine.min.js",
	}

	for _, path := range required {
		data, err := fs.ReadFile(assets, path)
		if err != nil {
			t.Fatalf("expected embedded asset %q to be readable, got error: %v", path, err)
		}
		if len(data) == 0 {
			t.Fatalf("expected embedded asset %q to be non-empty", path)
		}
	}
}

func TestManifestIsAsInvokerNoUAC(t *testing.T) {
	raw, err := os.ReadFile("build/windows/wails.exe.manifest")
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}
	content := string(raw)
	if !strings.Contains(content, `level="asInvoker"`) {
		t.Fatalf("manifest must specify level=\"asInvoker\" to avoid UAC prompts")
	}
	if strings.Contains(content, "requireAdministrator") || strings.Contains(content, "highestAvailable") {
		t.Fatalf("manifest must not request administrator elevation")
	}
}

func TestFrontendAlpineBindingsMatchAppMethods(t *testing.T) {
	data, err := fs.ReadFile(assets, "frontend/dist/index.html")
	if err != nil {
		t.Fatalf("failed to read embedded index.html: %v", err)
	}
	html := string(data)

	expectedBindings := []string{
		"window.go.main.App.GetDefaultInstallDir",
		"window.go.main.App.GetPreflightStatus",
		"window.go.main.App.FetchReleases",
		"window.go.main.App.SelectInstallDirectory",
		"window.go.main.App.SelectZipArchive",
		"window.go.main.App.InstallSDK",
		"window.go.main.App.CancelOperation",
		"window.go.main.App.RegisterPathOnly",
		"window.go.main.App.UnregisterPathOnly",
		"window.go.main.App.InspectCleanup",
		"window.go.main.App.ExecuteCleanup",
	}

	for _, binding := range expectedBindings {
		if !strings.Contains(html, binding) {
			t.Fatalf("expected index.html to bind to %q", binding)
		}
	}
}

