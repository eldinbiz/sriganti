package sdk

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"flutter-portable-manager/internal/env"
)

// ProvisionRequest defines the parameters for an SDK provisioning run.
type ProvisionRequest struct {
	Mode              string `json:"mode"`
	SourceType        string `json:"sourceType"`
	Version           string `json:"version"`
	DartVersion       string `json:"dartVersion"`
	DownloadURL       string `json:"downloadUrl"`
	ExpectedSHA256    string `json:"expectedSha256"`
	SHA256URL         string `json:"sha256Url"`
	LocalZipPath      string `json:"localZipPath"`
	InstallDir        string `json:"installDir"`
	AutoInstallMinGit bool   `json:"autoInstallMinGit"`
}

// ProvisionResult describes the completed portable installation.
type ProvisionResult struct {
	Mode            string   `json:"mode"`
	Version         string   `json:"version"`
	DartVersion     string   `json:"dartVersion"`
	InstallDir      string   `json:"installDir"`
	MinGitInstalled bool     `json:"minGitInstalled"`
	RegisteredPaths []string `json:"registeredPaths"`
}

// Provisioner orchestrates pre-flight Git checks, MinGit setup, SDK download/extraction, and HKCU environment registration.
type Provisioner struct {
	ReleaseClient *ReleaseClient
	EnvStore      env.EnvironmentStore
	LookPath      func(string) (string, error)
	OnProgress    func(ProgressEvent)
	OnLog         func(string)
}

// NewProvisioner creates a Provisioner with default dependencies.
func NewProvisioner(store env.EnvironmentStore) *Provisioner {
	if store == nil {
		store = env.NewWindowsRegistryStore()
	}
	return &Provisioner{
		ReleaseClient: NewReleaseClient(),
		EnvStore:      store,
		LookPath:      exec.LookPath,
	}
}

func (p *Provisioner) logf(format string, args ...interface{}) {
	if p.OnLog != nil {
		p.OnLog(fmt.Sprintf(format, args...))
	}
}

func (p *Provisioner) emitProgress(ev ProgressEvent) {
	if p.OnProgress != nil {
		p.OnProgress(ev)
	}
}

// Provision executes the end-to-end portable SDK installation workflow.
func (p *Provisioner) Provision(ctx context.Context, req ProvisionRequest) (*ProvisionResult, error) {
	installDir := strings.TrimSpace(req.InstallDir)
	if installDir == "" {
		installDir = env.DefaultInstallDir()
	}
	cleanDir := filepath.Clean(installDir)

	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = env.ModeFlutter
	}
	sourceType := strings.TrimSpace(req.SourceType)
	if sourceType == "" {
		sourceType = "online"
	}

	lookPath := p.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	// Step 1: If installing from a local .zip archive, validate it BEFORE modifying cleanDir
	var zipToExtract string
	if sourceType == "local" {
		if strings.TrimSpace(req.LocalZipPath) == "" {
			return nil, errors.New("please select a local .zip archive file")
		}
		p.logf("Inspecting local archive: %s", req.LocalZipPath)
		detectedKind, fileCount, err := InspectZipArchive(req.LocalZipPath)
		if err != nil {
			return nil, err
		}
		if detectedKind != ArchiveKindFlutter && detectedKind != ArchiveKindDart {
			return nil, ErrInvalidArchiveStructure
		}
		mode = detectedKind
		zipToExtract = req.LocalZipPath
		p.logf("Validated local %s archive (%d files)", mode, fileCount)
	}

	// Step 2: Check Git availability when provisioning Full Flutter + Dart
	minGitInstalled := false
	minGitExe := filepath.Join(cleanDir, "mingit", "cmd", "git.exe")
	if _, err := os.Stat(minGitExe); err == nil {
		minGitInstalled = true
	}

	if mode == env.ModeFlutter {
		sysGit, gitErr := lookPath("git")
		if gitErr == nil && sysGit != "" {
			p.logf("System Git detected at: %s", sysGit)
		} else if minGitInstalled {
			p.logf("Portable MinGit already present at: %s", minGitExe)
		} else if req.AutoInstallMinGit {
			p.logf("Git not found in PATH. Provisioning portable 64-bit MinGit into %s\\mingit...", cleanDir)
			if err := p.provisionMinGit(ctx, cleanDir); err != nil {
				return nil, fmt.Errorf("portable MinGit provisioning failed: %w", err)
			}
			minGitInstalled = true
			p.logf("Portable MinGit provisioned successfully.")
		} else {
			p.logf("Warning: Git was not found in PATH and auto-MinGit was disabled.")
		}
	}

	// Step 3: Resolve and download online SDK archive if sourceType == "online"
	version := strings.TrimSpace(req.Version)
	dartVersion := strings.TrimSpace(req.DartVersion)

	if sourceType == "online" {
		downloadURL := strings.TrimSpace(req.DownloadURL)
		expectedSHA := strings.TrimSpace(req.ExpectedSHA256)
		sha256URL := strings.TrimSpace(req.SHA256URL)

		if downloadURL == "" {
			p.logf("Fetching official release metadata from Google...")
			catalog, err := p.ReleaseClient.FetchCatalog(ctx, 25)
			if err != nil {
				return nil, err
			}
			opt, err := selectReleaseFromCatalog(catalog, mode, version)
			if err != nil {
				return nil, err
			}
			version = opt.Version
			dartVersion = opt.DartVersion
			downloadURL = opt.DownloadURL
			expectedSHA = opt.SHA256
			sha256URL = opt.SHA256URL
		}

		if expectedSHA == "" && sha256URL != "" {
			p.logf("Fetching SHA256 checksum for Dart SDK %s...", version)
			if sum, err := p.ReleaseClient.FetchDartSHA256(ctx, sha256URL); err == nil {
				expectedSHA = sum
			}
		}

		tmpZip := filepath.Join(cleanDir, ".tmp-sdk-download.zip")
		defer func() {
			_ = os.Remove(tmpZip)
		}()

		p.logf("Downloading %s (%s) from %s...", mode, version, downloadURL)
		actualSHA, err := DownloadAndVerify(ctx, p.ReleaseClient.HTTPClient, downloadURL, tmpZip, expectedSHA, func(ev ProgressEvent) {
			p.emitProgress(ev)
		})
		if err != nil {
			return nil, err
		}
		p.logf("Download complete. Verified SHA256: %s", actualSHA)
		zipToExtract = tmpZip
	}

	// Step 4: Prepare target SDK subfolder and extract archive
	env.TerminateSDKProcessesInDir(cleanDir)
	if mode == env.ModeDartOnly {
		_ = os.RemoveAll(filepath.Join(cleanDir, "dart-sdk"))
	} else {
		_ = os.RemoveAll(filepath.Join(cleanDir, "flutter"))
	}

	p.logf("Extracting archive into %s...", cleanDir)
	_, err := ExtractZipConcurrent(ctx, zipToExtract, cleanDir, mode, func(ev ProgressEvent) {
		p.emitProgress(ev)
	})
	if err != nil {
		return nil, err
	}

	// Step 5: Detect extracted versions and persist state.json + register HKCU\Environment
	_, diskFlutterVer, diskDartVer, _ := env.DetectDiskSDK(cleanDir)
	if diskFlutterVer != "" {
		version = diskFlutterVer
	}
	if diskDartVer != "" {
		dartVersion = diskDartVer
	}
	if version == "" {
		if mode == env.ModeDartOnly && dartVersion != "" {
			version = dartVersion
		} else {
			version = "portable"
		}
	}

	p.emitProgress(ProgressEvent{
		Stage:   "configuring",
		Message: "Registering User PATH and PUB_CACHE in HKCU\\Environment...",
		Percent: 95,
	})
	p.logf("Registering portable paths in HKCU\\Environment (No UAC)...")

	if err := env.SaveState(cleanDir, env.PortableState{
		Mode:            mode,
		Version:         version,
		DartVersion:     dartVersion,
		InstallDir:      cleanDir,
		MinGitInstalled: minGitInstalled,
	}); err != nil {
		return nil, fmt.Errorf("failed to write state.json: %w", err)
	}

	registeredPaths, err := env.RegisterPortableEnvironment(p.EnvStore, cleanDir, mode)
	if err != nil {
		return nil, fmt.Errorf("failed to register user environment: %w", err)
	}

	p.logf("Registered %d user PATH entries and broadcasted WM_SETTINGCHANGE.", len(registeredPaths))
	p.emitProgress(ProgressEvent{
		Stage:   "done",
		Message: "Installation & user PATH registration complete!",
		Percent: 100,
	})

	return &ProvisionResult{
		Mode:            mode,
		Version:         version,
		DartVersion:     dartVersion,
		InstallDir:      cleanDir,
		MinGitInstalled: minGitInstalled,
		RegisteredPaths: registeredPaths,
	}, nil
}

func (p *Provisioner) provisionMinGit(ctx context.Context, cleanDir string) error {
	minGitInfo := p.ReleaseClient.MinGit
	if strings.TrimSpace(minGitInfo.DownloadURL) == "" {
		return errors.New("MinGit download URL is not configured")
	}

	tmpMinGitZip := filepath.Join(cleanDir, ".tmp-mingit.zip")
	defer func() {
		_ = os.Remove(tmpMinGitZip)
	}()

	_, err := DownloadAndVerify(ctx, p.ReleaseClient.HTTPClient, minGitInfo.DownloadURL, tmpMinGitZip, minGitInfo.SHA256, func(ev ProgressEvent) {
		ev.Message = fmt.Sprintf("Downloading portable MinGit %s...", minGitInfo.Version)
		p.emitProgress(ev)
	})
	if err != nil {
		return err
	}

	minGitDest := filepath.Join(cleanDir, "mingit")
	_ = os.RemoveAll(minGitDest)
	_, err = ExtractZipConcurrent(ctx, tmpMinGitZip, minGitDest, ArchiveKindMinGit, func(ev ProgressEvent) {
		ev.Message = fmt.Sprintf("Extracting portable MinGit (%d / %d files)...", ev.FilesProcessed, ev.FilesTotal)
		p.emitProgress(ev)
	})
	return err
}

func selectReleaseFromCatalog(catalog *ReleaseCatalog, mode string, requestedVersion string) (ReleaseOption, error) {
	var list []ReleaseOption
	if mode == env.ModeDartOnly {
		list = catalog.DartReleases
	} else {
		list = catalog.FlutterReleases
	}
	if len(list) == 0 {
		return ReleaseOption{}, fmt.Errorf("no releases available for mode %q", mode)
	}
	if requestedVersion != "" {
		for _, item := range list {
			if strings.EqualFold(item.Version, requestedVersion) {
				return item, nil
			}
		}
		return ReleaseOption{}, fmt.Errorf("requested version %q not found in release feed", requestedVersion)
	}
	for _, item := range list {
		if item.IsDefault {
			return item, nil
		}
	}
	return list[0], nil
}
