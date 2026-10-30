<p align="center">
  <img src="frontend/dist/assets/logo.png" alt="Sriganti Logo" width="128" height="128" />
</p>

# Sriganti

> *Light as a sunbird. Portable Flutter & Dart for Windows.*

A standalone, Zero-UAC desktop manager for installing, linking, and maintaining portable Flutter and Dart SDKs on Windows 10 and 11.

---

## Prerequisites

Before building or running the project, make sure you have:

1. **Go** (version 1.20+ recommended, tested with Go 1.26+)
2. **Wails CLI v2**:
   ```cmd
   go install github.com/wailsapp/wails/v2/cmd/wails@latest
   ```
3. **Microsoft Edge WebView2**: Pre-installed on Windows 10 (version 1803+) and Windows 11.
4. **Git** *(Optional)*: Recommended for building Flutter projects. If not found on the user machine, the manager can auto-provision portable MinGit.

> **Portable Toolchain Note**: If you are using the portable `.tools` environment, initialize your environment variables prior to running commands:
> ```cmd
> set "GOROOT=C:\Users\Public\Downloads\projects\device-security-posture-assesor\desktop-app\.tools\go"
> set "GOPATH=C:\Users\Public\Downloads\projects\device-security-posture-assesor\desktop-app\.tools\gopath"
> set "PATH=%GOROOT%\bin;%GOPATH%\bin;%PATH%"
> ```
> Verify system readiness at any time with:
> ```cmd
> wails doctor
> ```

---

## 1. How to Compile to Production

To produce an optimized, standalone Windows `.exe` that runs without requiring administrator privileges (Zero-UAC):

### Basic Production Build

Run the following command from the project root:

```cmd
wails build -s
```

### Clean Production Build (Recommended)

To wipe previous temporary build artifacts and strip debug symbols for the smallest binary footprint:

```cmd
wails build -s -clean
```

### Build Outputs

- The compiled production executable will be created at:
  ```text
  build/bin/flutter-portable-manager.exe
  ```
- **Zero-UAC Manifest**: The build automatically embeds `build/windows/wails.exe.manifest` configured with:
  ```xml
  <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
  ```
  This ensures the application never prompts for User Account Control (UAC) elevation and can run in restricted, guest, or portable USB environments.

### Useful Build Flags

| Flag | Description |
| :--- | :--- |
| `-s` | Strips symbol table and debug information from the binary to reduce file size. |
| `-clean` | Cleans the `build/bin` directory before building. |
| `-upx` | Compresses the final binary with UPX (if installed). |
| `-nsis` | Generates a standard Windows NSIS installer alongside the standalone `.exe`. |
| `-o <filename>.exe` | Specifies a custom output binary filename. |

---

## 2. How to Debug Live with Hot Reloading

Wails v2 provides an interactive development environment with dual live-reloading for both Go backend code and frontend UI files.

### Start Live Development Mode

From the project root, run:

```cmd
wails dev
```

---

## Running Automated Tests

To run the complete test suite verifying embedded assets, Windows manifest settings, registry operations, and SDK provisioning logic:

```cmd
go test -v ./...
```

To run tests without utilizing cached results:

```cmd
go test -count=1 -v ./...
```

---

## Project Structure

```text
desktop-app/
├── app.go                # Wails backend methods bound to the frontend UI
├── app_test.go           # Integration tests for app bindings and lifecycle
├── main.go               # Application entrypoint & asset embed configuration
├── main_test.go          # Verification of manifest and embedded UI bindings
├── wails.json            # Wails v2 project configuration
├── build/
│   ├── appicon.png       # Source application icon
│   ├── bin/              # Compiled output directory
│   └── windows/          # Windows manifest (asInvoker) & icon resources
├── frontend/
│   └── dist/             # Static frontend (index.html, Alpine.js, Tailwind v3)
└── internal/
    ├── env/              # Windows registry (HKCU\Environment), PATH, and cleanup
    └── sdk/              # Download, SHA256 verification, and ZIP extraction
```
