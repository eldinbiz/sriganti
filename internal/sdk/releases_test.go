package sdk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleReleasesFeedJSON = `{
  "base_url": "https://storage.googleapis.com/flutter_infra_release/releases",
  "current_release": {
    "beta": "beta-hash-1",
    "dev": "dev-hash-1",
    "stable": "stable-hash-3290"
  },
  "releases": [
    {
      "hash": "beta-hash-1",
      "channel": "beta",
      "version": "3.30.0-0.1.pre",
      "dart_sdk_version": "3.8.0 (build 3.8.0-10.0.dev)",
      "dart_sdk_arch": "x64",
      "release_date": "2026-02-15T10:00:00Z",
      "archive": "beta/windows/flutter_windows_3.30.0-0.1.pre-beta.zip",
      "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    },
    {
      "hash": "stable-hash-3290",
      "channel": "stable",
      "version": "3.29.0",
      "dart_sdk_version": "3.7.0",
      "dart_sdk_arch": "x64",
      "release_date": "2026-02-12T10:00:00Z",
      "archive": "stable/windows/flutter_windows_3.29.0-stable.zip",
      "sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    },
    {
      "hash": "stable-hash-3274",
      "channel": "stable",
      "version": "3.27.4",
      "dart_sdk_version": "3.6.2 (build 3.6.2-0)",
      "dart_sdk_arch": "x64",
      "release_date": "2026-01-20T10:00:00Z",
      "archive": "stable/windows/flutter_windows_3.27.4-stable.zip",
      "sha256": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
    }
  ]
}`

func TestParseReleaseFeed_DefaultLatestStableAndDartCatalog(t *testing.T) {
	catalog, err := ParseReleaseFeed(strings.NewReader(sampleReleasesFeedJSON), "https://storage.googleapis.com/dart-archive/channels/stable/release", 10)
	if err != nil {
		t.Fatalf("ParseReleaseFeed failed: %v", err)
	}

	if catalog.DefaultFlutterVersion != "3.29.0" {
		t.Fatalf("expected DefaultFlutterVersion=3.29.0, got %q", catalog.DefaultFlutterVersion)
	}
	if len(catalog.FlutterReleases) != 2 {
		t.Fatalf("expected 2 stable Flutter releases, got %d", len(catalog.FlutterReleases))
	}
	first := catalog.FlutterReleases[0]
	if !first.IsDefault || first.Version != "3.29.0" || first.DartVersion != "3.7.0" {
		t.Fatalf("unexpected first Flutter release: %+v", first)
	}
	expectedURL := "https://storage.googleapis.com/flutter_infra_release/releases/stable/windows/flutter_windows_3.29.0-stable.zip"
	if first.DownloadURL != expectedURL {
		t.Fatalf("expected DownloadURL %q, got %q", expectedURL, first.DownloadURL)
	}

	if catalog.DefaultDartVersion != "3.7.0" {
		t.Fatalf("expected DefaultDartVersion=3.7.0, got %q", catalog.DefaultDartVersion)
	}
	if len(catalog.DartReleases) != 2 {
		t.Fatalf("expected 2 Dart releases, got %d", len(catalog.DartReleases))
	}
	if catalog.DartReleases[1].Version != "3.6.2" {
		t.Fatalf("expected cleaned Dart version 3.6.2, got %q", catalog.DartReleases[1].Version)
	}
}

func TestFetchCatalog_And_FetchDartSHA256(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases_windows.json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(sampleReleasesFeedJSON))
		case "/3.7.0/sdk/dartsdk-windows-x64-release.zip.sha256sum":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd *dartsdk-windows-x64-release.zip\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewReleaseClient()
	client.FlutterReleasesURL = srv.URL + "/releases_windows.json"
	client.DartArchiveBaseURL = srv.URL

	catalog, err := client.FetchCatalog(context.Background(), 10)
	if err != nil {
		t.Fatalf("FetchCatalog failed: %v", err)
	}
	if catalog.DefaultFlutterVersion != "3.29.0" {
		t.Fatalf("expected 3.29.0, got %s", catalog.DefaultFlutterVersion)
	}

	sum, err := client.FetchDartSHA256(context.Background(), catalog.DartReleases[0].SHA256URL)
	if err != nil {
		t.Fatalf("FetchDartSHA256 failed: %v", err)
	}
	if sum != "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd" {
		t.Fatalf("unexpected checksum: %s", sum)
	}
}
