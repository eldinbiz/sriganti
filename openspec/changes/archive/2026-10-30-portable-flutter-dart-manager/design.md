# Design

## Context

This project is a greenfield Windows 10+ portable desktop utility for provisioning, linking, and cleanly removing the Flutter and Dart SDKs without triggering User Account Control (UAC). See `proposal.md` for motivation and scope, and `specs/` for behavioral requirements.

## Goals / Non-Goals

**Goals:**
- Produce a single, portable Windows `.exe` using **Wails v2** (Go + WebView2) with `requestedExecutionLevel level="asInvoker"` in its embedded Windows application manifest so UAC is never triggered.
- Serve a **zero-compilation** frontend using raw HTML, Alpine.js, and Tailwind CSS v3 Play runtime embedded via Go `embed.FS`, ensuring offline startup capability.
- Keep all provisioned artifacts (`flutter` or `dart-sdk`, `pub-cache`, optional `mingit`, and `state.json`) self-contained inside `<install_dir>` (defaulting to `<exe_dir>\flutter-portable`).
- Mutate only `HKCU\Environment` (`Path` and `PUB_CACHE`) via native Go Windows Registry APIs and broadcast `WM_SETTINGCHANGE` so terminals and IDEs pick up changes immediately without a reboot or admin prompt.
- Accurately detect and clean orphaned/stale Flutter & Dart user `PATH` entries and user-profile caches (`.dartServer`, `.flutter`, `.dart-tool`, `Pub\Cache`).

**Non-Goals:**
- Modifying system-wide `HKLM` registry keys, installing into `C:\Program Files`, or requiring Administrator elevation.
- Managing Android Studio, Android SDK, or Visual Studio C++ build tools.
- Supporting macOS or Linux targets (this tool specifically targets Windows 10 and above).
- Introducing Node.js/npm/Vite frontend build pipelines.

## Decisions

### 1. Application Framework: Wails v2 with Embedded No-Build Frontend
- **Choice**: Use Wails v2 (`go` + `embed.FS`) with `"frontend:install": ""` and `"frontend:build": ""` in `wails.json`, embedding `frontend/dist/` (containing `index.html`, `assets/tailwindcss.js`, and `assets/alpine.min.js`).
- **Rationale**: Fulfils the HTML + Alpine.js + Tailwind v3 (no-compilation) stack requirement while guaranteeing that the UI still renders when launched offline (e.g., installing from a USB drive or uninstalling without internet). WebView2 is pre-installed on Windows 10/11, keeping the final binary ~10–15 MB.
- **Alternatives Considered**:
  - *Loading Tailwind/Alpine from remote `<script src="https://cdn...">` tags*: Fails to render styling and reactivity when the user opens the portable app offline.
  - *Electron / Tauri*: Electron produces 150 MB+ binaries; Tauri requires Rust + frontend bundler setup, whereas Go's `archive/zip`, `net/http`, and `golang.org/x/sys/windows/registry` provide direct, fast OS primitives.

### 2. SDK & Git Provisioning Pipeline
- **Choice**:
  - **Flutter + Dart (Default)**: Query `https://storage.googleapis.com/flutter_infra_release/releases/releases_windows.json`, filter `stable` channel releases (defaulting to `current_release.stable`), verify the archive against the feed's `sha256` field, and extract to `<install_dir>\flutter`.
  - **Dart SDK Only**: Download from `https://storage.googleapis.com/dart-archive/channels/stable/release/<version>/sdk/dartsdk-windows-x64-release.zip` (defaulting to `latest`), verify checksum (`*.zip.sha256sum`), and extract to `<install_dir>\dart-sdk`.
  - **MinGit Auto-Provisioning**: When installing Full Flutter and `exec.LookPath("git")` fails, fetch the portable 64-bit `MinGit-<ver>-64-bit.zip` archive from Git for Windows releases and extract to `<install_dir>\mingit`, exposing `<install_dir>\mingit\cmd`.
  - **Local `.zip` Support**: Inspect zip entries before extraction to auto-detect whether the archive root contains `flutter/bin/flutter.bat` or `dart-sdk/bin/dart.exe`.
- **Rationale**: Official Google storage buckets require no scraping, include cryptographic SHA256 hashes, and extract cleanly in user space. Bundling/auto-fetching MinGit prevents the #1 failure on fresh Windows machines (`Error: Unable to find git in your PATH`).

### 3. Portable Directory Structure & State Tracking
- **Choice**: Store all portable files under `<install_dir>` (default `<exe_dir>\flutter-portable`):
  ```text
  <install_dir>\
    +-- flutter\          (or dart-sdk\)
    +-- pub-cache\        (target of HKCU\Environment\PUB_CACHE)
    +-- mingit\           (optional, provisioned if system git was missing)
    +-- state.json        (installed mode, version, timestamp, registered paths)
  ```
- **Rationale**: Redirecting `PUB_CACHE` into `<install_dir>\pub-cache` keeps gigabytes of downloaded pub packages inside the portable folder rather than silently filling `%LOCALAPPDATA%\Pub\Cache`. `state.json` enables instant detection when the portable folder is moved to a new path or plugged into another PC.

### 4. Windows User Registry (`HKCU\Environment`) & `WM_SETTINGCHANGE`
- **Choice**: Use `golang.org/x/sys/windows/registry` to open `registry.CURRENT_USER` -> `Environment` with `QUERY_VALUE | SET_VALUE`. Parse `Path` on `;`, normalize paths (`filepath.Clean` + case-insensitive comparison on Windows) to prevent duplicates, preserve the original value type (`REG_EXPAND_SZ` or `REG_SZ`), and broadcast `WM_SETTINGCHANGE` (`0x001A`) with `"Environment"` via `user32.dll` `SendMessageTimeoutW` (`HWND_BROADCAST`, `SMTO_ABORTIFHUNG`, 5000ms).
- **Rationale**: Using `setx` in PowerShell/CMD truncates `PATH` strings longer than 1024 characters and corrupts unexpanded `%VAR%` tokens. Direct registry manipulation via `golang.org/x/sys/windows/registry` has no 1024-char limit, preserves `REG_EXPAND_SZ`, never triggers UAC, and `SendMessageTimeoutW` notifies Explorer and IDEs immediately.

### 5. Orphaned Path Detection & Leftover Cache Cleanup
- **Choice**:
  - **Orphaned PATH Scanner**: Inspect every segment in `HKCU\Environment\Path`. Flag segments that match known SDK/cache patterns (`*\flutter\bin`, `*\dart-sdk\bin`, `*\Pub\Cache\bin`, `*\pub-cache\bin`, `*\mingit\cmd`) where either the directory no longer exists on disk or it belongs to the portable installation being uninstalled.
  - **Leftover Caches**: Inspect and calculate disk usage for `%USERPROFILE%\.dartServer`, `%USERPROFILE%\.flutter`, `%APPDATA%\.dart-tool`, and `%LOCALAPPDATA%\Pub\Cache` so the user can selectively wipe them during uninstallation.
- **Rationale**: Safely targets only Flutter/Dart-related `PATH` entries and caches without risking unrelated user `PATH` entries.

## Risks / Trade-offs

- **[Risk] Slow extraction of Flutter `.zip` (~20,000+ small files on Windows NTFS / Windows Defender real-time scan)** → **Mitigation**: Use a worker pool in Go for file extraction with buffered I/O, stream granular file count progress to the UI via Wails `runtime.EventsEmit`, and support cancellation via `context.Context`.
- **[Risk] Locked files during uninstall (e.g., a background `dart.exe` analysis server or `flutter` daemon still holding `<install_dir>` or `.dartServer`)** → **Mitigation**: Before deleting `<install_dir>` or `.dartServer`, check for and gracefully terminate running `dart.exe` / `flutter` processes spawned from `<install_dir>`, and surface clear retry/skip diagnostics if any file remains locked.
- **[Risk] Tailwind Play CDN script order with Alpine.js** → **Mitigation**: Load `tailwindcss.js` synchronously in `<head>` and load `alpine.min.js` with `defer` after registering `Alpine.data(...)` controller bindings to `window.go.main.App`.
