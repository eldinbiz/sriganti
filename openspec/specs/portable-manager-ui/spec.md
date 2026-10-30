# portable-manager-ui Specification

## Purpose

Defines the user interface and desktop window behavior for the Wails v2 portable Flutter and Dart manager using a zero-compilation HTML, Alpine.js, and Tailwind CSS v3 frontend.

## Requirements

### Requirement: Zero-UAC desktop launch and offline-capable UI rendering
The system SHALL launch as a standalone Windows 10+ desktop application without triggering a User Account Control (UAC) prompt and SHALL render its HTML, Alpine.js, and Tailwind CSS v3 user interface even when the machine has no internet connection.

#### Scenario: Launching the portable executable without admin privileges
- **WHEN** a standard (non-administrator) Windows 10+ user launches the application executable
- **THEN** Windows opens the application window directly without displaying a UAC elevation dialog

#### Scenario: Rendering the interface while offline
- **WHEN** the application is launched on a Windows 10+ machine disconnected from the internet
- **THEN** all Tailwind CSS v3 styles and Alpine.js interactive bindings load from embedded application assets and the UI remains fully interactive for local `.zip` installation, path registration, and uninstallation

### Requirement: Pre-flight diagnostics and installation dashboard
The system SHALL display real-time pre-flight status (installed SDK state, detected Flutter and Dart versions, `git` availability, and `HKCU\Environment\Path` alignment) and provide interactive controls for selecting install mode, source (online version vs. local `.zip`), and target directory (defaulting to `<exe_dir>\flutter-portable`).

#### Scenario: Inspecting pre-flight status on startup
- **WHEN** the application finishes loading the main view
- **THEN** the status banner displays whether Flutter/Dart is installed, whether `git` is detected, and whether `HKCU\Environment` matches `<install_dir>`

#### Scenario: Streaming live progress during provisioning
- **WHEN** a download, checksum verification, or archive extraction is running
- **THEN** the UI displays a progress bar with percentage/file counts and appends real-time step messages to the activity log

### Requirement: Granular uninstall and leftover inspection view
The system SHALL present an uninstall and cleanup view listing managed `HKCU\Environment` entries, detected orphaned `flutter`/`dart` `PATH` entries, the portable SDK directory, and detected `%USERPROFILE%`/`%APPDATA%` leftover cache directories with estimated disk sizes before deletion.

#### Scenario: Reviewing and executing selective cleanup
- **WHEN** the user opens the Uninstall & Deep Clean tab
- **THEN** the UI displays checkboxes and calculated disk sizes for the portable folder and each detected leftover cache directory, and executes removal only for the items the user selects and confirms
