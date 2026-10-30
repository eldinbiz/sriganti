package sdk

import (
	"archive/zip"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	ArchiveKindFlutter = "flutter"
	ArchiveKindDart    = "dart-only"
	ArchiveKindMinGit  = "mingit"
)

// ErrInvalidArchiveStructure is returned when a .zip file does not contain a recognizable Flutter, Dart, or MinGit layout.
var ErrInvalidArchiveStructure = errors.New("invalid archive structure: missing flutter/bin/flutter.bat or dart-sdk/bin/dart.exe")

// InspectZipArchive validates a .zip file and identifies whether it is a Flutter SDK, Dart SDK, or MinGit archive.
func InspectZipArchive(zipPath string) (kind string, fileCount int, err error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", 0, fmt.Errorf("unable to open zip archive: %w", err)
	}
	defer zr.Close()

	hasFlutterBat := false
	hasDartExe := false
	hasMinGitExe := false

	for _, f := range zr.File {
		norm := strings.ToLower(strings.ReplaceAll(f.Name, `\`, `/`))
		norm = strings.TrimLeft(norm, "/")
		if strings.Contains(norm, "../") || strings.HasPrefix(norm, "..") {
			return "", 0, errors.New("unsafe zip entry path traversal detected")
		}
		if !f.FileInfo().IsDir() {
			fileCount++
		}
		switch norm {
		case "flutter/bin/flutter.bat":
			hasFlutterBat = true
		case "dart-sdk/bin/dart.exe":
			hasDartExe = true
		case "cmd/git.exe":
			hasMinGitExe = true
		}
	}

	if hasFlutterBat {
		return ArchiveKindFlutter, fileCount, nil
	}
	if hasDartExe {
		return ArchiveKindDart, fileCount, nil
	}
	if hasMinGitExe {
		return ArchiveKindMinGit, fileCount, nil
	}
	return "", fileCount, ErrInvalidArchiveStructure
}

// ExtractZipConcurrent validates zipPath before modifying destDir and extracts its files using a worker pool.
func ExtractZipConcurrent(
	ctx context.Context,
	zipPath string,
	destDir string,
	expectedKind string,
	onProgress func(ProgressEvent),
) (string, error) {
	detectedKind, totalFiles, err := InspectZipArchive(zipPath)
	if err != nil {
		return "", err
	}
	if expectedKind != "" && detectedKind != expectedKind {
		return detectedKind, fmt.Errorf(
			"%w: archive is %q but selected mode requires %q",
			ErrInvalidArchiveStructure, detectedKind, expectedKind,
		)
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	cleanDest := filepath.Clean(destDir)
	if err := os.MkdirAll(cleanDest, 0755); err != nil {
		return "", err
	}

	// Pre-create all directories first so file workers never race on MkdirAll
	filesToExtract := make([]*zip.File, 0, totalFiles)
	dirSet := make(map[string]bool)

	for _, f := range zr.File {
		targetPath, err := safeJoinDestination(cleanDest, f.Name)
		if err != nil {
			return "", err
		}
		if f.FileInfo().IsDir() {
			dirSet[targetPath] = true
			continue
		}
		dirSet[filepath.Dir(targetPath)] = true
		filesToExtract = append(filesToExtract, f)
	}

	for dirPath := range dirSet {
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return "", err
		}
	}

	if len(filesToExtract) == 0 {
		return detectedKind, nil
	}

	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	if workers < 2 {
		workers = 2
	}

	jobs := make(chan *zip.File, workers*2)
	var processed atomic.Int32
	var firstErr atomic.Value
	var progressMu sync.Mutex
	lastEmit := time.Time{}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 64*1024)
			for f := range jobs {
				if ctx.Err() != nil {
					return
				}
				if err := extractSingleZipEntry(cleanDest, f, buf); err != nil {
					firstErr.CompareAndSwap(nil, err)
					cancel()
					return
				}
				doneCount := int(processed.Add(1))
				if onProgress != nil {
					progressMu.Lock()
					shouldEmit := doneCount == totalFiles || time.Since(lastEmit) >= 75*time.Millisecond
					if shouldEmit {
						lastEmit = time.Now()
						pct := (doneCount * 100) / totalFiles
						onProgress(ProgressEvent{
							Stage:          "extracting",
							Message:        fmt.Sprintf("Extracting archive (%d / %d files)...", doneCount, totalFiles),
							Percent:        pct,
							FilesProcessed: doneCount,
							FilesTotal:     totalFiles,
						})
					}
					progressMu.Unlock()
				}
			}
		}()
	}

	for _, f := range filesToExtract {
		select {
		case <-ctx.Done():
			break
		case jobs <- f:
		}
	}
	close(jobs)
	wg.Wait()

	if storedErr := firstErr.Load(); storedErr != nil {
		return detectedKind, storedErr.(error)
	}
	if err := ctx.Err(); err != nil {
		return detectedKind, err
	}

	return detectedKind, nil
}

func extractSingleZipEntry(cleanDest string, f *zip.File, buf []byte) error {
	targetPath, err := safeJoinDestination(cleanDest, f.Name)
	if err != nil {
		return err
	}

	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	mode := f.Mode().Perm()
	if mode == 0 {
		mode = 0644
	}

	outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	bw := bufio.NewWriterSize(outFile, 64*1024)
	_, copyErr := io.CopyBuffer(bw, rc, buf)
	flushErr := bw.Flush()
	closeErr := outFile.Close()

	if copyErr != nil {
		return copyErr
	}
	if flushErr != nil {
		return flushErr
	}
	return closeErr
}

func safeJoinDestination(cleanDest string, entryName string) (string, error) {
	rel := filepath.FromSlash(strings.TrimLeft(strings.ReplaceAll(entryName, `\`, `/`), "/"))
	target := filepath.Clean(filepath.Join(cleanDest, rel))
	prefix := cleanDest + string(os.PathSeparator)
	if target != cleanDest && !strings.HasPrefix(strings.ToLower(target), strings.ToLower(prefix)) {
		return "", fmt.Errorf("unsafe zip entry path: %s", entryName)
	}
	return target, nil
}
