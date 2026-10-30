package sdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"flutter-portable-manager/internal/env"
)

type mockEnvStore struct {
	pathVal    string
	vars       map[string]string
	broadcasts int
}

func newMockEnvStore() *mockEnvStore {
	return &mockEnvStore{
		pathVal: `C:\Windows\System32`,
		vars:    make(map[string]string),
	}
}

func (m *mockEnvStore) GetUserPath() (string, error)          { return m.pathVal, nil }
func (m *mockEnvStore) SetUserPath(p string) error            { m.pathVal = p; return nil }
func (m *mockEnvStore) GetUserVar(k string) (string, error)   { return m.vars[k], nil }
func (m *mockEnvStore) SetUserVar(k, v string) error          { m.vars[k] = v; return nil }
func (m *mockEnvStore) DeleteUserVar(k string) error          { delete(m.vars, k); return nil }
func (m *mockEnvStore) BroadcastSettingChange() error         { m.broadcasts++; return nil }

func TestProvisioner_OnlineFlutterWithAutoMinGit(t *testing.T) {
	tmp := t.TempDir()

	// Build fake Flutter zip and MinGit zip
	flutterZipPath := filepath.Join(tmp, "flutter_windows_3.29.0-stable.zip")
	createTestZip(t, flutterZipPath, map[string]string{
		"flutter/bin/flutter.bat":                 "@echo off\r\n",
		"flutter/version":                         "3.29.0\n",
		"flutter/bin/cache/dart-sdk/bin/dart.exe": "MZ",
		"flutter/bin/cache/dart-sdk/version":      "3.7.0\n",
	})
	flutterBytes, err := os.ReadFile(flutterZipPath)
	if err != nil {
		t.Fatal(err)
	}
	flutterSum := sha256.Sum256(flutterBytes)
	flutterHex := hex.EncodeToString(flutterSum[:])

	minGitZipPath := filepath.Join(tmp, "mingit.zip")
	createTestZip(t, minGitZipPath, map[string]string{
		"cmd/git.exe": "MZ-git",
	})
	minGitBytes, err := os.ReadFile(minGitZipPath)
	if err != nil {
		t.Fatal(err)
	}
	minGitSum := sha256.Sum256(minGitBytes)
	minGitHex := hex.EncodeToString(minGitSum[:])

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases_windows.json":
			feed := fmt.Sprintf(`{
				"base_url": "%s/releases",
				"current_release": {"stable": "hash-3290"},
				"releases": [{
					"hash": "hash-3290",
					"channel": "stable",
					"version": "3.29.0",
					"dart_sdk_version": "3.7.0",
					"dart_sdk_arch": "x64",
					"release_date": "2026-02-12T10:00:00Z",
					"archive": "stable/windows/flutter_windows_3.29.0-stable.zip",
					"sha256": "%s"
				}]
			}`, srv.URL, flutterHex)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(feed))
		case "/releases/stable/windows/flutter_windows_3.29.0-stable.zip":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(flutterBytes)
		case "/mingit.zip":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(minGitBytes)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	store := newMockEnvStore()
	prov := NewProvisioner(store)
	prov.ReleaseClient.HTTPClient = srv.Client()
	prov.ReleaseClient.FlutterReleasesURL = srv.URL + "/releases_windows.json"
	prov.ReleaseClient.MinGit = MinGitInfo{
		Version:     "2.47.1",
		DownloadURL: srv.URL + "/mingit.zip",
		SHA256:      minGitHex,
	}
	// Simulate missing system git so MinGit is auto-provisioned
	prov.LookPath = func(name string) (string, error) {
		return "", errors.New("git not found in PATH")
	}

	installDir := filepath.Join(tmp, "portable-target")
	res, err := prov.Provision(context.Background(), ProvisionRequest{
		Mode:              env.ModeFlutter,
		SourceType:        "online",
		InstallDir:        installDir,
		AutoInstallMinGit: true,
	})
	if err != nil {
		t.Fatalf("Provision failed: %v", err)
	}

	if !res.MinGitInstalled {
		t.Fatalf("expected MinGitInstalled=true when system git is missing")
	}
	if res.Version != "3.29.0" || res.DartVersion != "3.7.0" {
		t.Fatalf("unexpected version result: %+v", res)
	}

	// Verify state.json and HKCU environment registration
	report := env.InspectPreflight(installDir, store.pathVal, store.vars["PUB_CACHE"], prov.LookPath)
	if report.Status != env.StatusReady {
		t.Fatalf("expected preflight status %q after provisioning, got %q (%+v)", env.StatusReady, report.Status, report)
	}
	if report.GitSource != "mingit" {
		t.Fatalf("expected GitSource=mingit, got %q", report.GitSource)
	}
}
