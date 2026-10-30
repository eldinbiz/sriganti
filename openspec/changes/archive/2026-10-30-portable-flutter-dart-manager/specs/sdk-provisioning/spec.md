# Spec Delta

## Purpose

Defines the user-space provisioning behavior for discovering, downloading, checksum-verifying, and extracting the Flutter SDK, standalone Dart SDK, and portable MinGit on Windows 10 and above without triggering User Account Control (UAC).

## ADDED Requirements

### Requirement: Single combined flow SDK mode selection
The system SHALL support provisioning in either "Full Flutter + Dart SDK" mode (default, providing both `flutter` and `dart` CLI tools from the official Flutter Windows release archive) or "Dart SDK Only" mode (providing only the `dart` CLI tool from the standalone Dart Windows x64 archive).

#### Scenario: Default Full Flutter and Dart provisioning
- **WHEN** the user initiates provisioning with the default "Full Flutter + Dart SDK" mode selected
- **THEN** the system provisions the Flutter Windows SDK archive into `<install_dir>\flutter` and exposes both `flutter` and `dart` executables from `<install_dir>\flutter\bin`

#### Scenario: Lightweight Dart SDK Only provisioning
- **WHEN** the user selects "Dart SDK Only" mode and initiates provisioning
- **THEN** the system provisions the standalone Dart Windows archive into `<install_dir>\dart-sdk` and exposes the `dart` executable from `<install_dir>\dart-sdk\bin`

### Requirement: Online release discovery and version selection
The system SHALL fetch official Windows release metadata from Google's release endpoints, default to the latest Stable release, and allow the user to select a specific available release version for download.

#### Scenario: Fetching latest stable release metadata
- **WHEN** the user opens the online source selector with internet connectivity available
- **THEN** the system queries Google's Windows release feed, selects the latest Stable release by default, and lists available release versions with their bundled Dart version

#### Scenario: Handling offline or unreachable release feed
- **WHEN** the release metadata endpoint cannot be reached
- **THEN** the system displays a clear connectivity error and prompts the user to retry or switch to installing from a local `.zip` archive

### Requirement: Local zip archive provisioning
The system SHALL allow users to provision the SDK from a local `.zip` archive file without requiring an internet download for the SDK payload.

#### Scenario: Installing from a valid local Flutter or Dart zip archive
- **WHEN** the user selects a local `.zip` file containing a valid Flutter SDK (`flutter/bin/flutter.bat`) or Dart SDK (`dart-sdk/bin/dart.exe`) structure
- **THEN** the system extracts the archive into `<install_dir>` and records the installed mode and version in `<install_dir>\state.json`

#### Scenario: Rejecting an invalid local zip archive
- **WHEN** the user selects a local `.zip` file that does not contain a recognizable `flutter/bin` or `dart-sdk/bin` directory structure
- **THEN** the system aborts extraction before modifying `<install_dir>` and reports an invalid archive structure error

### Requirement: Integrity verification and user-space extraction
The system SHALL verify SHA256 checksums for online downloads whenever release metadata provides a hash, extract archives into the target portable directory without requesting administrator elevation (no UAC), and emit progress updates during download and extraction.

#### Scenario: Online download passes SHA256 verification
- **WHEN** an online SDK archive finishes downloading and its computed SHA256 digest matches the release metadata hash
- **THEN** the system proceeds to extract the archive into `<install_dir>` and writes `<install_dir>\state.json`

#### Scenario: Online download fails SHA256 verification
- **WHEN** an online SDK archive finishes downloading and its computed SHA256 digest does not match the expected hash
- **THEN** the system deletes the corrupted temporary archive, aborts installation, and reports a checksum mismatch error

### Requirement: Automatic portable MinGit provisioning when Git is missing
When provisioning in "Full Flutter + Dart SDK" mode, the system SHALL check whether `git` is available on the system and automatically provision portable MinGit into `<install_dir>\mingit` if `git` is missing.

#### Scenario: System Git is already available
- **WHEN** the user installs "Full Flutter + Dart SDK" on a Windows system where `git.exe` is already resolvable in `PATH`
- **THEN** the system skips downloading MinGit and uses the existing system Git

#### Scenario: System Git is missing during Flutter installation
- **WHEN** the user installs "Full Flutter + Dart SDK" on a Windows system where `git.exe` is not found in `PATH`
- **THEN** the system downloads and extracts the 64-bit portable MinGit archive into `<install_dir>\mingit` and registers `<install_dir>\mingit\cmd` in the user's `PATH`
