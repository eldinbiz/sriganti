# Tasks

## 1. Wails Project & Zero-UAC Scaffolding

- [x] 1.1 Initialize the Go module (`go.mod`), Wails v2 configuration (`wails.json` with empty frontend build commands), and `main.go` entrypoint, and verify `go vet ./...` succeeds
- [x] 1.2 Configure the Windows application manifest (`build/windows/wails.exe.manifest`) with `requestedExecutionLevel level="asInvoker"` and verify the manifest XML contains no administrator elevation requirement
- [x] 1.3 Embed offline frontend runtime assets (`frontend/dist/index.html`, `frontend/dist/assets/tailwindcss.js`, and `frontend/dist/assets/alpine.min.js`) via `embed.FS` and verify the embedded filesystem resolves all asset paths in a Go unit test

## 2. Portable State & User Environment Lifecycle (`HKCU\Environment`)

- [x] 2.1 Implement `<install_dir>\state.json` persistence and pre-flight detector (`internal/env/state.go`) for inspecting installed SDK mode (`flutter` vs `dart-only`), version, `git` availability, and `PATH` alignment, and verify with unit tests covering installed, missing, and moved-folder states
- [x] 2.2 Implement Windows user registry manager (`internal/env/registry_windows.go`) using `golang.org/x/sys/windows/registry` to read/update `HKCU\Environment\Path` and `PUB_CACHE` with case-insensitive path deduplication, mode swap (`flutter\bin` vs `dart-sdk\bin`), and `WM_SETTINGCHANGE` broadcast, and verify with unit tests on path manipulation and deduplication logic
- [x] 2.3 Implement the orphaned `PATH` scanner and leftover cache inspector/cleaner (`internal/env/cleanup.go`) for `<install_dir>`, `%USERPROFILE%\.dartServer`, `%USERPROFILE%\.flutter`, `%APPDATA%\.dart-tool`, and `%LOCALAPPDATA%\Pub\Cache` (including running `dart.exe`/`flutter` process termination inside `<install_dir>`), and verify with unit tests using temporary directories and mock path lists

## 3. SDK & MinGit Provisioning Engine

- [x] 3.1 Implement release metadata client (`internal/sdk/releases.go`) for Google's Flutter Windows release feed (`releases_windows.json`), standalone Dart SDK release URLs, and portable 64-bit MinGit metadata, and verify feed parsing and default Latest Stable selection with unit tests
- [x] 3.2 Implement streaming HTTP downloader with SHA256 checksum verifier (`internal/sdk/downloader.go`) and progress callbacks, and verify valid and mismatched SHA256 scenarios in unit tests
- [x] 3.3 Implement archive validator and concurrent `.zip` extractor (`internal/sdk/extractor.go`) supporting both online downloads and local `.zip` archives (`flutter/bin/flutter.bat` or `dart-sdk/bin/dart.exe`), and verify valid extraction and invalid-archive rejection with unit tests
- [x] 3.4 Implement end-to-end provisioning orchestrator (`internal/sdk/provisioner.go`) combining pre-flight `git` check, conditional MinGit provisioning into `<install_dir>\mingit`, SDK extraction, `state.json` persistence, and `HKCU\Environment` registration, and verify orchestration flow with unit tests

## 4. Wails App Bindings & HTML + Alpine.js + Tailwind v3 UI

- [x] 4.1 Implement Wails `App` struct methods (`app.go`) for pre-flight status, release listing, directory/file dialogs, install/re-link actions, uninstall inspection, selective cleanup, and Wails progress/log events, and verify `go test ./...` passes across all packages
- [x] 4.2 Build the zero-compilation UI (`frontend/dist/index.html`) using Alpine.js and Tailwind CSS v3 with the Status Banner, Tab 1 (Install & Portable Link with Single Combined Flow, online/local `.zip` source, folder picker, and live progress/log drawer), and Tab 2 (Uninstall & Deep Clean with orphaned `PATH` and cache size checkboxes), and verify all Alpine bindings map to `window.go.main.App` methods
- [x] 4.3 Build the portable Windows executable (`wails build`) and verify the resulting `.exe` launches without UAC and executes pre-flight diagnostics cleanly
