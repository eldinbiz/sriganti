# Spec Delta

## Purpose

Defines the Windows 10+ user-level Registry environment variable management (`HKCU\Environment`), portable directory re-linking, orphaned `PATH` detection and removal, and deep filesystem cleanup of SDK directories and user profile caches without UAC.

## ADDED Requirements

### Requirement: Non-admin user environment registration
The system SHALL register the portable SDK binary directory (`<install_dir>\flutter\bin` or `<install_dir>\dart-sdk\bin`), portable package cache binary directory (`<install_dir>\pub-cache\bin`), optional portable MinGit (`<install_dir>\mingit\cmd`), and the `PUB_CACHE` environment variable (`<install_dir>\pub-cache`) strictly inside `HKCU\Environment` without modifying machine-wide (`HKLM`) registry keys or triggering UAC, and SHALL broadcast `WM_SETTINGCHANGE` upon completion.

#### Scenario: Registering Full Flutter and Dart portable environment
- **WHEN** provisioning completes or the user triggers "Register Path Only" for a Full Flutter installation at `<install_dir>`
- **THEN** the system sets `HKCU\Environment\PUB_CACHE` to `<install_dir>\pub-cache`, ensures `<install_dir>\flutter\bin` and `<install_dir>\pub-cache\bin` are present in `HKCU\Environment\Path` without duplicating entries, and broadcasts `WM_SETTINGCHANGE`

#### Scenario: Upgrading or switching between Dart-Only and Full Flutter
- **WHEN** the user installs or registers "Full Flutter + Dart SDK" while `<install_dir>\dart-sdk\bin` is currently in `HKCU\Environment\Path` (or vice versa)
- **THEN** the system replaces the previous mode's SDK `bin` entry in `HKCU\Environment\Path` with the active mode's `bin` entry so both do not collide

### Requirement: Portable folder relocation and path re-linking
The system SHALL detect when an existing portable SDK directory has been moved or opened on a different Windows host and allow the user to register or unregister `HKCU\Environment` entries without re-downloading or deleting SDK files.

#### Scenario: Relinking a moved portable folder
- **WHEN** the application starts and detects a valid SDK inside `<install_dir>` whose path does not match the current `HKCU\Environment\Path`
- **THEN** the system reports a "Path Mismatch / Unregistered" state and allows the user to update `HKCU\Environment` to the new `<install_dir>` and strip the old path entry in a single action

#### Scenario: Unregistering environment variables while keeping SDK files
- **WHEN** the user selects "Unregister PATH Only (Keep Files)"
- **THEN** the system removes the portable SDK, `pub-cache\bin`, `mingit\cmd`, and `PUB_CACHE` entries from `HKCU\Environment` and broadcasts `WM_SETTINGCHANGE` while leaving all files in `<install_dir>` intact

### Requirement: Orphaned Flutter and Dart PATH scanning and cleanup
The system SHALL scan `HKCU\Environment\Path` for stale, non-existent, or leftover Flutter and Dart SDK path entries and allow the user to remove them.

#### Scenario: Detecting and removing orphaned Flutter or Dart PATH entries
- **WHEN** `HKCU\Environment\Path` contains entries referencing `flutter\bin`, `dart-sdk\bin`, or `Pub\Cache\bin` whose target directories no longer exist on disk (or belong to a removed installation)
- **THEN** the system lists those orphaned entries during cleanup and removes them from `HKCU\Environment\Path` upon user confirmation

### Requirement: Deep uninstall of SDK files and user profile caches
The system SHALL allow the user to completely remove the portable `<install_dir>` directory (including SDK files, `<install_dir>\pub-cache`, and `<install_dir>\mingit`) alongside selectable user-profile leftover directories (`%USERPROFILE%\.dartServer`, `%USERPROFILE%\.flutter`, `%APPDATA%\.dart-tool`, and `%LOCALAPPDATA%\Pub\Cache`).

#### Scenario: Full uninstall with leftover cache removal
- **WHEN** the user confirms uninstallation with the portable folder and user profile cache options checked
- **THEN** the system cleans `HKCU\Environment`, broadcasts `WM_SETTINGCHANGE`, deletes `<install_dir>`, and deletes each selected leftover cache directory that exists on disk
