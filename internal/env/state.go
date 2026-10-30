package env

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	ModeFlutter  = "flutter"
	ModeDartOnly = "dart-only"

	StatusNotInstalled = "not_installed"
	StatusReady        = "ready"
	StatusRelinkNeeded = "relink_needed"

	StateFileName = "state.json"
)

// PortableState persists metadata about the portable installation inside <install_dir>\state.json.
type PortableState struct {
	Mode            string   `json:"mode"`
	Version         string   `json:"version"`
	DartVersion     string   `json:"dartVersion,omitempty"`
	InstallDir      string   `json:"installDir"`
	MinGitInstalled bool     `json:"minGitInstalled"`
	RegisteredPaths []string `json:"registeredPaths,omitempty"`
	InstalledAt     string   `json:"installedAt"`
}

// PreflightReport summarizes the current installation, Git availability, and HKCU environment alignment.
type PreflightReport struct {
	Status              string `json:"status"`
	StatusLabel         string `json:"statusLabel"`
	InstallDir          string `json:"installDir"`
	DetectedMode        string `json:"detectedMode"`
	FlutterVersion      string `json:"flutterVersion"`
	DartVersion         string `json:"dartVersion"`
	GitAvailable        bool   `json:"gitAvailable"`
	GitSource           string `json:"gitSource"`
	GitPath             string `json:"gitPath"`
	PathAligned         bool   `json:"pathAligned"`
	PubCacheAligned     bool   `json:"pubCacheAligned"`
	SDKExistsOnDisk     bool   `json:"sdkExistsOnDisk"`
	MovedFromDir        string `json:"movedFromDir,omitempty"`
	ExpectedSDKBin      string `json:"expectedSdkBin"`
	ExpectedPubCache    string `json:"expectedPubCache"`
	ExpectedPubCacheBin string `json:"expectedPubCacheBin"`
	ExpectedMinGitCmd   string `json:"expectedMinGitCmd,omitempty"`
}

// DefaultInstallDir returns <exe_dir>\flutter-portable as the default portable directory.
func DefaultInstallDir() string {
	exePath, err := os.Executable()
	if err != nil {
		cwd, _ := os.Getwd()
		return filepath.Join(cwd, "flutter-portable")
	}
	return filepath.Join(filepath.Dir(exePath), "flutter-portable")
}

// StateFilePath returns the path to <install_dir>\state.json.
func StateFilePath(installDir string) string {
	return filepath.Join(installDir, StateFileName)
}

// SaveState writes PortableState atomically to <install_dir>\state.json.
func SaveState(installDir string, st PortableState) error {
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return err
	}
	if st.InstallDir == "" {
		st.InstallDir = filepath.Clean(installDir)
	}
	if st.InstalledAt == "" {
		st.InstalledAt = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(StateFilePath(installDir), data, 0644)
}

// LoadState reads <install_dir>\state.json if it exists.
func LoadState(installDir string) (*PortableState, error) {
	raw, err := os.ReadFile(StateFilePath(installDir))
	if err != nil {
		return nil, err
	}
	var st PortableState
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// DetectDiskSDK inspects <install_dir> on disk to determine whether Flutter or Dart SDK binaries exist.
func DetectDiskSDK(installDir string) (mode string, flutterVer string, dartVer string, exists bool) {
	flutterBat := filepath.Join(installDir, "flutter", "bin", "flutter.bat")
	dartExeInFlutter := filepath.Join(installDir, "flutter", "bin", "cache", "dart-sdk", "bin", "dart.exe")
	standaloneDartExe := filepath.Join(installDir, "dart-sdk", "bin", "dart.exe")

	if fileExists(flutterBat) {
		mode = ModeFlutter
		exists = true
		flutterVer = readVersionFile(filepath.Join(installDir, "flutter", "version"))
		dartVer = readVersionFile(filepath.Join(installDir, "flutter", "bin", "cache", "dart-sdk", "version"))
		return
	}
	if fileExists(standaloneDartExe) {
		mode = ModeDartOnly
		exists = true
		dartVer = readVersionFile(filepath.Join(installDir, "dart-sdk", "version"))
		return
	}
	if fileExists(dartExeInFlutter) {
		mode = ModeFlutter
		exists = true
		dartVer = readVersionFile(filepath.Join(installDir, "flutter", "bin", "cache", "dart-sdk", "version"))
		return
	}
	return "", "", "", false
}

// InspectPreflight evaluates disk state, state.json, Git availability, and HKCU Environment alignment.
func InspectPreflight(installDir string, userPath string, userPubCache string, lookPath func(string) (string, error)) PreflightReport {
	cleanDir := filepath.Clean(installDir)
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	report := PreflightReport{
		Status:              StatusNotInstalled,
		StatusLabel:         "Not Installed",
		InstallDir:          cleanDir,
		ExpectedPubCache:    filepath.Join(cleanDir, "pub-cache"),
		ExpectedPubCacheBin: filepath.Join(cleanDir, "pub-cache", "bin"),
	}

	// Check Git availability (portable MinGit inside installDir or system PATH)
	minGitExe := filepath.Join(cleanDir, "mingit", "cmd", "git.exe")
	if fileExists(minGitExe) {
		report.GitAvailable = true
		report.GitSource = "mingit"
		report.GitPath = minGitExe
		report.ExpectedMinGitCmd = filepath.Join(cleanDir, "mingit", "cmd")
	} else if sysGit, err := lookPath("git"); err == nil && sysGit != "" {
		report.GitAvailable = true
		report.GitSource = "system"
		report.GitPath = sysGit
	} else {
		report.GitAvailable = false
		report.GitSource = "missing"
	}

	// Check disk SDK & state.json
	diskMode, diskFlutterVer, diskDartVer, sdkExists := DetectDiskSDK(cleanDir)
	st, stateErr := LoadState(cleanDir)

	report.SDKExistsOnDisk = sdkExists
	if !sdkExists {
		// Default expected bin for UI display is Flutter mode
		report.ExpectedSDKBin = filepath.Join(cleanDir, "flutter", "bin")
		return report
	}

	mode := diskMode
	if stateErr == nil && st != nil {
		if st.Mode != "" && mode == "" {
			mode = st.Mode
		}
		if diskFlutterVer == "" && st.Mode == ModeFlutter {
			diskFlutterVer = st.Version
		}
		if diskDartVer == "" {
			if st.DartVersion != "" {
				diskDartVer = st.DartVersion
			} else if st.Mode == ModeDartOnly {
				diskDartVer = st.Version
			}
		}
		if st.InstallDir != "" && !EqualNormalizedPath(st.InstallDir, cleanDir) {
			report.MovedFromDir = st.InstallDir
		}
	}

	report.DetectedMode = mode
	report.FlutterVersion = diskFlutterVer
	report.DartVersion = diskDartVer

	if mode == ModeDartOnly {
		report.ExpectedSDKBin = filepath.Join(cleanDir, "dart-sdk", "bin")
	} else {
		report.ExpectedSDKBin = filepath.Join(cleanDir, "flutter", "bin")
	}

	// Verify PATH & PUB_CACHE alignment
	hasSDKBin := ContainsNormalizedPath(userPath, report.ExpectedSDKBin)
	hasPubCacheBin := ContainsNormalizedPath(userPath, report.ExpectedPubCacheBin)
	hasMinGit := true
	if report.ExpectedMinGitCmd != "" && report.GitSource == "mingit" {
		hasMinGit = ContainsNormalizedPath(userPath, report.ExpectedMinGitCmd)
	}

	report.PathAligned = hasSDKBin && hasPubCacheBin && hasMinGit
	report.PubCacheAligned = EqualNormalizedPath(userPubCache, report.ExpectedPubCache)

	if report.PathAligned && report.PubCacheAligned && report.MovedFromDir == "" {
		report.Status = StatusReady
		report.StatusLabel = "Installed & Ready"
	} else {
		report.Status = StatusRelinkNeeded
		report.StatusLabel = "Path Mismatch / Relink Needed"
	}

	return report
}

// NormalizeWindowsPath cleans a Windows path and lowercases it for case-insensitive comparison.
func NormalizeWindowsPath(p string) string {
	trimmed := strings.TrimSpace(strings.Trim(p, `"`))
	if trimmed == "" {
		return ""
	}
	cleaned := filepath.Clean(trimmed)
	cleaned = strings.ReplaceAll(cleaned, "/", `\`)
	cleaned = strings.TrimRight(cleaned, `\`)
	return strings.ToLower(cleaned)
}

// EqualNormalizedPath compares two Windows file paths case-insensitively.
func EqualNormalizedPath(a, b string) bool {
	na := NormalizeWindowsPath(a)
	nb := NormalizeWindowsPath(b)
	return na != "" && na == nb
}

// SplitPathEntries splits a semicolon-delimited Windows PATH string into non-empty trimmed entries.
func SplitPathEntries(pathValue string) []string {
	raw := strings.Split(pathValue, ";")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// ContainsNormalizedPath returns true if targetPath is present in semicolon-delimited pathValue.
func ContainsNormalizedPath(pathValue, targetPath string) bool {
	targetNorm := NormalizeWindowsPath(targetPath)
	if targetNorm == "" {
		return false
	}
	for _, entry := range SplitPathEntries(pathValue) {
		if NormalizeWindowsPath(entry) == targetNorm {
			return true
		}
	}
	return false
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func readVersionFile(p string) string {
	raw, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}
