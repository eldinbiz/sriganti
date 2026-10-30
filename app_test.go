package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"flutter-portable-manager/internal/env"
	"flutter-portable-manager/internal/sdk"
)

type testAppEnvStore struct {
	pathVal string
	vars    map[string]string
}

func newTestAppEnvStore() *testAppEnvStore {
	return &testAppEnvStore{
		pathVal: `C:\Windows\System32;C:\StaleOld\flutter\bin`,
		vars:    make(map[string]string),
	}
}

func (s *testAppEnvStore) GetUserPath() (string, error)        { return s.pathVal, nil }
func (s *testAppEnvStore) SetUserPath(p string) error          { s.pathVal = p; return nil }
func (s *testAppEnvStore) GetUserVar(k string) (string, error) { return s.vars[k], nil }
func (s *testAppEnvStore) SetUserVar(k, v string) error        { s.vars[k] = v; return nil }
func (s *testAppEnvStore) DeleteUserVar(k string) error        { delete(s.vars, k); return nil }
func (s *testAppEnvStore) BroadcastSettingChange() error       { return nil }

func createLocalDartZip(t *testing.T, zipPath string) {
	t.Helper()
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w1, _ := zw.Create("dart-sdk/bin/dart.exe")
	_, _ = w1.Write([]byte("MZ-dart"))
	w2, _ := zw.Create("dart-sdk/version")
	_, _ = w2.Write([]byte("3.7.0\n"))
	_ = zw.Close()
}

func TestAppLifecycle_InstallRelinkInspectAndCleanup(t *testing.T) {
	tmp := t.TempDir()
	installDir := filepath.Join(tmp, "flutter-portable")
	localZip := filepath.Join(tmp, "dartsdk.zip")
	createLocalDartZip(t, localZip)

	store := newTestAppEnvStore()
	app := NewApp()
	app.envStore = store
	app.provisioner = sdk.NewProvisioner(store)
	app.systemDirs = env.SystemDirs{
		UserProfile:  filepath.Join(tmp, "Profile"),
		AppData:      filepath.Join(tmp, "AppData"),
		LocalAppData: filepath.Join(tmp, "LocalAppData"),
	}

	// 1. Pre-flight before install
	status, err := app.GetPreflightStatus(installDir)
	if err != nil {
		t.Fatalf("GetPreflightStatus failed: %v", err)
	}
	if status.Status != env.StatusNotInstalled {
		t.Fatalf("expected not_installed, got %s", status.Status)
	}

	// 2. Install from local .zip archive
	res, err := app.InstallSDK(sdk.ProvisionRequest{
		Mode:         env.ModeDartOnly,
		SourceType:   "local",
		LocalZipPath: localZip,
		InstallDir:   installDir,
	})
	if err != nil {
		t.Fatalf("InstallSDK failed: %v", err)
	}
	if res.Mode != env.ModeDartOnly || res.Version != "3.7.0" {
		t.Fatalf("unexpected InstallSDK result: %+v", res)
	}

	// 3. Pre-flight after install should be Ready
	status, err = app.GetPreflightStatus(installDir)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != env.StatusReady {
		t.Fatalf("expected status ready, got %s", status.Status)
	}

	// 4. Unregister PATH Only (Keep Files) -> status becomes relink_needed
	if _, err := app.UnregisterPathOnly(installDir); err != nil {
		t.Fatalf("UnregisterPathOnly failed: %v", err)
	}
	status, _ = app.GetPreflightStatus(installDir)
	if status.Status != env.StatusRelinkNeeded {
		t.Fatalf("expected status relink_needed after UnregisterPathOnly, got %s", status.Status)
	}

	// 5. Register PATH Only -> status becomes ready again
	if _, err := app.RegisterPathOnly(installDir, env.ModeDartOnly); err != nil {
		t.Fatalf("RegisterPathOnly failed: %v", err)
	}
	status, _ = app.GetPreflightStatus(installDir)
	if status.Status != env.StatusReady {
		t.Fatalf("expected status ready after RegisterPathOnly, got %s", status.Status)
	}

	// 6. InspectCleanup -> should detect C:\StaleOld\flutter\bin as orphaned path
	inspection, err := app.InspectCleanup(installDir)
	if err != nil {
		t.Fatalf("InspectCleanup failed: %v", err)
	}
	if len(inspection.OrphanedPaths) != 1 || inspection.OrphanedPaths[0].RawPath != `C:\StaleOld\flutter\bin` {
		t.Fatalf("expected 1 orphaned path C:\\StaleOld\\flutter\\bin, got %+v", inspection.OrphanedPaths)
	}

	// 7. ExecuteCleanup -> removes managed env, orphaned path, and portable folder
	cleanRes, err := app.ExecuteCleanup(env.CleanupRequest{
		InstallDir:          installDir,
		RemoveManagedEnv:    true,
		OrphanPathsToRemove: []string{`C:\StaleOld\flutter\bin`},
		DeletePortableDir:   true,
	})
	if err != nil {
		t.Fatalf("ExecuteCleanup failed: %v", err)
	}
	if len(cleanRes.DeletedDirectories) == 0 {
		t.Fatalf("expected portable directory to be deleted")
	}
	if store.pathVal != `C:\Windows\System32` {
		t.Fatalf("expected clean PATH C:\\Windows\\System32, got %q", store.pathVal)
	}
}
