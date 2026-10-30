package sdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProgressEvent represents a real-time provisioning progress update for the UI.
type ProgressEvent struct {
	Stage          string `json:"stage"`
	Message        string `json:"message"`
	Percent        int    `json:"percent"`
	BytesProcessed int64  `json:"bytesProcessed"`
	BytesTotal     int64  `json:"bytesTotal"`
	FilesProcessed int    `json:"filesProcessed"`
	FilesTotal     int    `json:"filesTotal"`
}

// ErrChecksumMismatch is returned when a downloaded archive's SHA256 digest does not match expectedSHA256.
var ErrChecksumMismatch = errors.New("SHA256 checksum mismatch")

// DownloadAndVerify streams a file from url to destPath while computing its SHA256 digest.
// If expectedSHA256 is non-empty and does not match, destPath is deleted and ErrChecksumMismatch is returned.
func DownloadAndVerify(
	ctx context.Context,
	httpClient *http.Client,
	url string,
	destPath string,
	expectedSHA256 string,
	onProgress func(ProgressEvent),
) (string, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 0}
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with HTTP %d", resp.StatusCode)
	}

	outFile, err := os.Create(destPath)
	if err != nil {
		return "", err
	}

	cleanupOnError := func() {
		_ = outFile.Close()
		_ = os.Remove(destPath)
	}

	hasher := sha256.New()
	totalBytes := resp.ContentLength
	var processedBytes int64

	buf := make([]byte, 64*1024)
	lastEmit := time.Time{}

	for {
		select {
		case <-ctx.Done():
			cleanupOnError()
			return "", ctx.Err()
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if _, wErr := outFile.Write(chunk); wErr != nil {
				cleanupOnError()
				return "", wErr
			}
			_, _ = hasher.Write(chunk)
			processedBytes += int64(n)

			if onProgress != nil && time.Since(lastEmit) >= 80*time.Millisecond {
				lastEmit = time.Now()
				pct := 0
				if totalBytes > 0 {
					pct = int((processedBytes * 100) / totalBytes)
					if pct > 100 {
						pct = 100
					}
				}
				onProgress(ProgressEvent{
					Stage:          "downloading",
					Message:        "Downloading archive...",
					Percent:        pct,
					BytesProcessed: processedBytes,
					BytesTotal:     totalBytes,
				})
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			cleanupOnError()
			return "", fmt.Errorf("stream read error: %w", readErr)
		}
	}

	if err := outFile.Close(); err != nil {
		_ = os.Remove(destPath)
		return "", err
	}

	actualHex := strings.ToLower(hex.EncodeToString(hasher.Sum(nil)))
	expectedClean := strings.ToLower(strings.TrimSpace(expectedSHA256))

	if onProgress != nil {
		onProgress(ProgressEvent{
			Stage:          "verifying",
			Message:        "Verifying SHA256 checksum...",
			Percent:        100,
			BytesProcessed: processedBytes,
			BytesTotal:     processedBytes,
		})
	}

	if expectedClean != "" && actualHex != expectedClean {
		_ = os.Remove(destPath)
		return actualHex, fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expectedClean, actualHex)
	}

	return actualHex, nil
}
