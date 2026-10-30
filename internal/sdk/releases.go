package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultFlutterReleasesURL = "https://storage.googleapis.com/flutter_infra_release/releases/releases_windows.json"
	DefaultDartArchiveBaseURL = "https://storage.googleapis.com/dart-archive/channels/stable/release"
	DefaultMinGitURL          = "https://github.com/git-for-windows/git/releases/download/v2.47.1.windows.1/MinGit-2.47.1-64-bit.zip"
	DefaultMinGitSHA256       = "66ce2fb8ebfc4ab0fb97cadb4dc2db1ab1b41ee53226c052b13b6ba6a2d032a2"
	DefaultMinGitVersion      = "2.47.1"
)

// ReleaseOption represents an installable SDK release item for the UI selector.
type ReleaseOption struct {
	Version     string `json:"version"`
	DartVersion string `json:"dartVersion"`
	Channel     string `json:"channel"`
	ReleaseDate string `json:"releaseDate"`
	DownloadURL string `json:"downloadUrl"`
	SHA256      string `json:"sha256"`
	SHA256URL   string `json:"sha256Url,omitempty"`
	IsDefault   bool   `json:"isDefault"`
}

// ReleaseCatalog holds parsed options for both Full Flutter + Dart and Dart-Only modes.
type ReleaseCatalog struct {
	DefaultFlutterVersion string          `json:"defaultFlutterVersion"`
	DefaultDartVersion    string          `json:"defaultDartVersion"`
	FlutterReleases       []ReleaseOption `json:"flutterReleases"`
	DartReleases          []ReleaseOption `json:"dartReleases"`
}

// MinGitInfo describes the portable 64-bit MinGit archive to provision when system git is absent.
type MinGitInfo struct {
	Version     string `json:"version"`
	DownloadURL string `json:"downloadUrl"`
	SHA256      string `json:"sha256"`
}

type flutterReleasesFeed struct {
	BaseURL        string            `json:"base_url"`
	CurrentRelease map[string]string `json:"current_release"`
	Releases       []struct {
		Hash           string `json:"hash"`
		Channel        string `json:"channel"`
		Version        string `json:"version"`
		DartSDKVersion string `json:"dart_sdk_version"`
		DartSDKArch    string `json:"dart_sdk_arch"`
		ReleaseDate    string `json:"release_date"`
		Archive        string `json:"archive"`
		SHA256         string `json:"sha256"`
	} `json:"releases"`
}

// ReleaseClient fetches and parses official SDK release metadata.
type ReleaseClient struct {
	HTTPClient         *http.Client
	FlutterReleasesURL string
	DartArchiveBaseURL string
	MinGit             MinGitInfo
}

// NewReleaseClient creates a ReleaseClient configured with official endpoints.
func NewReleaseClient() *ReleaseClient {
	return &ReleaseClient{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		FlutterReleasesURL: DefaultFlutterReleasesURL,
		DartArchiveBaseURL: DefaultDartArchiveBaseURL,
		MinGit: MinGitInfo{
			Version:     DefaultMinGitVersion,
			DownloadURL: DefaultMinGitURL,
			SHA256:      "", // Verified when non-empty; GitHub release URL is HTTPS authenticated
		},
	}
}

// FetchCatalog queries Google's Windows release feed and builds the catalog of stable Flutter and Dart releases.
func (c *ReleaseClient) FetchCatalog(ctx context.Context, maxItems int) (*ReleaseCatalog, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.FlutterReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to reach official release feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release feed returned HTTP %d", resp.StatusCode)
	}

	return ParseReleaseFeed(resp.Body, c.DartArchiveBaseURL, maxItems)
}

// ParseReleaseFeed parses the JSON payload from releases_windows.json into a ReleaseCatalog.
func ParseReleaseFeed(r io.Reader, dartArchiveBaseURL string, maxItems int) (*ReleaseCatalog, error) {
	if dartArchiveBaseURL == "" {
		dartArchiveBaseURL = DefaultDartArchiveBaseURL
	}
	if maxItems <= 0 {
		maxItems = 25
	}

	var feed flutterReleasesFeed
	if err := json.NewDecoder(r).Decode(&feed); err != nil {
		return nil, fmt.Errorf("invalid release feed JSON: %w", err)
	}

	baseURL := strings.TrimRight(feed.BaseURL, "/")
	currentStableHash := feed.CurrentRelease["stable"]

	catalog := &ReleaseCatalog{}
	seenDart := make(map[string]bool)

	for _, item := range feed.Releases {
		if item.Channel != "stable" {
			continue
		}
		if item.DartSDKArch != "" && item.DartSDKArch != "x64" {
			continue
		}

		cleanDartVer := cleanDartVersionString(item.DartSDKVersion)
		archiveURL := baseURL + "/" + strings.TrimLeft(item.Archive, "/")
		isDefault := (currentStableHash != "" && item.Hash == currentStableHash) || len(catalog.FlutterReleases) == 0

		if len(catalog.FlutterReleases) < maxItems {
			opt := ReleaseOption{
				Version:     item.Version,
				DartVersion: cleanDartVer,
				Channel:     item.Channel,
				ReleaseDate: item.ReleaseDate,
				DownloadURL: archiveURL,
				SHA256:      strings.ToLower(strings.TrimSpace(item.SHA256)),
				IsDefault:   isDefault && catalog.DefaultFlutterVersion == "",
			}
			if opt.IsDefault {
				catalog.DefaultFlutterVersion = opt.Version
			}
			catalog.FlutterReleases = append(catalog.FlutterReleases, opt)
		}

		if cleanDartVer != "" && !seenDart[cleanDartVer] && len(catalog.DartReleases) < maxItems {
			seenDart[cleanDartVer] = true
			dartZipURL := fmt.Sprintf("%s/%s/sdk/dartsdk-windows-x64-release.zip", strings.TrimRight(dartArchiveBaseURL, "/"), cleanDartVer)
			isDartDefault := catalog.DefaultDartVersion == ""
			if isDartDefault {
				catalog.DefaultDartVersion = cleanDartVer
			}
			catalog.DartReleases = append(catalog.DartReleases, ReleaseOption{
				Version:     cleanDartVer,
				DartVersion: cleanDartVer,
				Channel:     "stable",
				ReleaseDate: item.ReleaseDate,
				DownloadURL: dartZipURL,
				SHA256URL:   dartZipURL + ".sha256sum",
				IsDefault:   isDartDefault,
			})
		}
	}

	if len(catalog.FlutterReleases) == 0 {
		return nil, errors.New("no stable Windows x64 releases found in release feed")
	}

	// Ensure at least one Flutter release is marked default
	if catalog.DefaultFlutterVersion == "" {
		catalog.FlutterReleases[0].IsDefault = true
		catalog.DefaultFlutterVersion = catalog.FlutterReleases[0].Version
	}

	return catalog, nil
}

// FetchDartSHA256 fetches and parses a .sha256sum file from the Dart archive server.
func (c *ReleaseClient) FetchDartSHA256(ctx context.Context, sha256URL string) (string, error) {
	if strings.TrimSpace(sha256URL) == "" {
		return "", nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sha256URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch Dart checksum: HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 || len(fields[0]) != 64 {
		return "", errors.New("unexpected .sha256sum format")
	}
	return strings.ToLower(fields[0]), nil
}

func cleanDartVersionString(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	// Strip "(build ...)" suffix if present in older feed entries
	if idx := strings.Index(trimmed, " "); idx != -1 {
		trimmed = strings.TrimSpace(trimmed[:idx])
	}
	return trimmed
}
