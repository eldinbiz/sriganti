# Proposal

## Why

Installing the Flutter and Dart SDKs on Windows 10 and above typically requires manual archive extraction, manual editing of system/user `PATH` variables, separate Git setup, and scatters gigabytes of package and analyzer caches (`%LOCALAPPDATA%\Pub\Cache`, `%USERPROFILE%\.dartServer`, `.flutter`, `.dart-tool`) across the user profile. Furthermore, standard installers often trigger User Account Control (UAC) prompts—blocking restricted or portable environments—and deleting an SDK folder manually leaves behind broken `PATH` entries and hidden cache leftovers. A standalone, zero-UAC portable desktop manager solves both portable provisioning and complete, trace-free removal on Windows 10+.

## What Changes

- Introduce a standalone Windows 10+ portable desktop application built with **Wails v2** (Go backend + WebView2) configured with an `asInvoker` application manifest so it never triggers UAC.
- Implement a **Single Combined Flow** for SDK provisioning that installs **Full Flutter + Dart SDK** by default (since Flutter bundles Dart) or **Dart SDK Only** as a lightweight option, supporting both online downloads from Google's official release feeds (defaulting to Latest Stable with version selection) and offline local `.zip` archives.
- Implement automatic `git` pre-flight detection that transparently downloads and extracts portable **MinGit** (`MinGit-<ver>-64-bit.zip`) into the portable directory when installing Flutter on a machine without Git in `PATH`.
- Configure self-contained portable storage (`<install_dir>\flutter` or `dart-sdk`, `<install_dir>\pub-cache`, `<install_dir>\mingit`, and `<install_dir>\state.json`) and manage Windows user-level environment variables (`HKCU\Environment\Path` and `PUB_CACHE`) with `WM_SETTINGCHANGE` broadcasts—plus one-click "Register / Unregister Path Only" for moved folders or USB drives.
- Provide a deep uninstaller and leftover cleaner that deletes the portable SDK directory, removes managed `HKCU\Environment` entries, detects and strips orphaned/broken Flutter and Dart entries from User `PATH`, and optionally cleans user profile caches (`.dartServer`, `.flutter`, `.dart-tool`, and default `Pub\Cache`).
- Build a zero-compilation frontend using plain **HTML, Alpine.js, and Tailwind CSS v3** (with embedded runtime scripts so the UI works completely offline).

## Capabilities

### New Capabilities
- `sdk-provisioning`: Fetching official Flutter/Dart release metadata, verifying SHA256 checksums, extracting SDK archives (from online feeds or local `.zip` files), and provisioning portable MinGit when `git` is missing—all in user space without UAC.
- `user-environment-lifecycle`: Reading and mutating `HKCU\Environment` (`Path` and `PUB_CACHE`), broadcasting `WM_SETTINGCHANGE`, re-linking moved portable directories, scanning User `PATH` for orphaned Flutter/Dart entries, and performing deep filesystem cleanup of SDK folders and leftover caches.
- `portable-manager-ui`: Wails v2 desktop window and embedded no-build HTML + Alpine.js + Tailwind v3 interface for pre-flight diagnostics, single-flow SDK installation/re-linking, live progress/log streaming, and selective uninstallation/cleanup.

### Modified Capabilities
<!-- None: greenfield project with no existing specs. -->

## Impact

- **Application Backend (Go / Wails v2)**: New Go packages for Windows Registry management (`golang.org/x/sys/windows/registry`), `WM_SETTINGCHANGE` Win32 syscall broadcasting, streaming HTTP download + SHA256 verification, fast `.zip` extraction, pre-flight environment inspection, and Wails event emitters.
- **Application Frontend (`frontend/`)**: No-build static bundle containing `index.html`, embedded Tailwind CSS v3 Play runtime, embedded Alpine.js runtime, and UI state bindings to Wails Go methods.
- **Windows Build & Manifest (`build/windows/`)**: Wails Windows manifest explicitly enforcing `requestedExecutionLevel level="asInvoker"` to guarantee zero UAC elevation prompts.
