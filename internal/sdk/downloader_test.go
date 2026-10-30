package sdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadAndVerify_ValidAndMismatchSHA256(t *testing.T) {
	payload := []byte("portable-flutter-dart-sdk-archive-content-for-sha256-test")
	sum := sha256.Sum256(payload)
	validHex := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	destValid := filepath.Join(tmpDir, "valid.zip")

	var events []ProgressEvent
	gotSum, err := DownloadAndVerify(context.Background(), srv.Client(), srv.URL, destValid, validHex, func(ev ProgressEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("expected valid download to succeed, got error: %v", err)
	}
	if gotSum != validHex {
		t.Fatalf("expected SHA256 %s, got %s", validHex, gotSum)
	}
	if len(events) == 0 {
		t.Fatalf("expected progress events to be emitted")
	}
	if _, err := os.Stat(destValid); err != nil {
		t.Fatalf("expected downloaded file to exist: %v", err)
	}

	// Now test mismatched SHA256 -> file must be deleted and ErrChecksumMismatch returned
	destBad := filepath.Join(tmpDir, "corrupt.zip")
	wrongHex := "0000000000000000000000000000000000000000000000000000000000000000"
	_, err = DownloadAndVerify(context.Background(), srv.Client(), srv.URL, destBad, wrongHex, nil)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("expected ErrChecksumMismatch, got %v", err)
	}
	if _, statErr := os.Stat(destBad); !os.IsNotExist(statErr) {
		t.Fatalf("expected corrupted archive %s to be deleted after checksum failure", destBad)
	}
}
