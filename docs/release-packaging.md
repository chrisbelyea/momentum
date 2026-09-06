# Release Packaging

This document describes how Momentum releases are created, where to download pre-built binaries, and how to build release artifacts locally.

> **References**: [Specification §13 Deployment](../requirements/specification.md#13-deployment) · [Specification §14 Tooling and CI/CD](../requirements/specification.md#14-tooling-and-cicd)

---

## Automated Releases (GitHub Releases)

Momentum uses [GoReleaser](https://goreleaser.com/) via a GitHub Actions workflow to automate cross-platform builds and publish GitHub Releases whenever a version tag is pushed.

### Creating a New Release

1. Ensure all changes are merged to `main` and CI is passing.
2. Create and push a version tag following [Semantic Versioning](https://semver.org/):

   ```bash
   git tag v1.2.3
   git push origin v1.2.3
   ```

3. The [Release workflow](../.github/workflows/release.yml) triggers automatically on the tag push.  It builds binaries for all supported platforms and publishes a [GitHub Release](https://github.com/chrisbelyea/momentum/releases) with all artifacts attached.

### Downloading Pre-built Binaries

Visit the [Releases page](https://github.com/chrisbelyea/momentum/releases) and download the archive for your platform:

| Platform | Archive |
|----------|---------|
| Linux x86-64 | `momentum-server-linux-amd64.tar.gz` |
| Linux ARM64 | `momentum-server-linux-arm64.tar.gz` |
| macOS Apple Silicon | `momentum-server-darwin-arm64.tar.gz` |
| macOS Intel | `momentum-server-darwin-amd64.tar.gz` |
| Windows x86-64 | `momentum-server-windows-amd64.zip` |

Each release also includes a `checksums.txt` file containing SHA-256 checksums for all archives.

### Verifying Checksums

After downloading, verify the archive against the published checksum:

**Linux / macOS:**

```bash
# Download the archive and checksums file, then verify:
sha256sum --check --ignore-missing checksums.txt
# or on macOS:
shasum -a 256 --check --ignore-missing checksums.txt
```

**Windows (PowerShell):**

```powershell
(Get-FileHash momentum-server-windows-amd64.zip -Algorithm SHA256).Hash
# Compare with the expected value in checksums.txt
```

### Testing a Release Before Tagging

To test the workflow without creating a real release, push a pre-release tag:

```bash
git tag v0.0.1-test
git push origin v0.0.1-test
```

Clean up after verification:

```bash
git tag -d v0.0.1-test
git push origin :refs/tags/v0.0.1-test
gh release delete v0.0.1-test --yes
```

### Release Workflow Details

The release pipeline is defined in [`.github/workflows/release.yml`](../.github/workflows/release.yml) and [`.goreleaser.yml`](../.goreleaser.yml).

| Detail | Value |
|--------|-------|
| Trigger | Push to a `v*` tag |
| Build tool | [GoReleaser](https://goreleaser.com/) inside the [`goreleaser-cross`](https://github.com/goreleaser/goreleaser-cross) Docker image |
| Cross-compilation | Linux ARM64 via `aarch64-linux-gnu-gcc`; Windows via MinGW-w64; macOS via osxcross |
| Archive format | `.tar.gz` (Linux/macOS), `.zip` (Windows) |
| Checksum | SHA-256, collected in `checksums.txt` |
| Release notes | Auto-generated from commit history |

---

## Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| Go | 1.24+ | CGO required (`gcc`/`clang` must be on `PATH`) |
| `gcc` or `clang` | any modern | Required by `go-sqlite3` CGO driver |
| `git` | any | Used to derive the version string |
| `tar` + checksum tool | standard | `tar` is bundled on Linux/macOS; use `sha256sum` on Linux, or `shasum -a 256` / `openssl dgst -sha256` on macOS (or `gsha256sum` via Homebrew coreutils); Windows users can use WSL |

Verify your environment:

```bash
go version        # go1.24.x linux/amd64 (or similar)
gcc --version     # gcc 13.x.x ...
git --version     # git version 2.x.x
```

---

## Server Single-Executable

The server is a self-contained Go binary. SQLite is compiled into the binary via the `go-sqlite3` CGO driver, so no external SQLite shared library is required at runtime (CGO and a C compiler are required at build time only).

### Quick build (current platform)

```bash
CGO_ENABLED=1 go build -o bin/momentum-server ./cmd/server
```

### Release build with version info and checksum

Use the provided script to produce a release-quality binary under `dist/server/`:

```bash
./scripts/release/build-server.sh
```

**Artifacts produced:**

```
dist/server/
  momentum-server             # server binary (Linux/macOS)
  momentum-server.sha256      # SHA-256 checksum (Linux/macOS)
  momentum-server.exe         # server binary (Windows)
  momentum-server.exe.sha256  # SHA-256 checksum (Windows)
```

### Cross-platform builds

Set `GOOS` and `GOARCH` before running the script. Because `go-sqlite3` uses CGO, cross-compilation requires a cross-compiler for each target platform.

| Target | `GOOS` | `GOARCH` | Cross-compiler example |
|--------|--------|----------|----------------------|
| Linux x86-64 | `linux` | `amd64` | `gcc` (native) |
| Linux ARM64 | `linux` | `arm64` | `aarch64-linux-gnu-gcc` |
| macOS (Apple Silicon) | `darwin` | `arm64` | Xcode on macOS |
| macOS (Intel) | `darwin` | `amd64` | Xcode on macOS |
| Windows x86-64 | `windows` | `amd64` | `x86_64-w64-mingw32-gcc` |

Example (Linux cross-compile to ARM64):

```bash
CC=aarch64-linux-gnu-gcc \
  GOOS=linux GOARCH=arm64 \
  VERSION=v0.1.0 \
  ./scripts/release/build-server.sh
```

> **Note**: `go-sqlite3` requires CGO, so cross-compilation always needs `CC` set to the appropriate cross-compiler. The script will warn (but not abort) if `CC` is unset during cross-compilation.

### Server configuration

At runtime the server reads configuration from environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_PATH` | OS user config directory / `Momentum/momentum.db` | Path to the SQLite database file. On Linux this is typically `~/.config/Momentum/momentum.db`; on Windows it is typically `%AppData%\\Momentum\\momentum.db`. |
| `PORT` | `8080` | TCP port to listen on |
| `MOMENTUM_ENCRYPTION_KEY` | *(dev default)* | AES encryption key for stored credentials |

For production deployments set `MOMENTUM_ENCRYPTION_KEY` to a securely generated random value.

### Deployment as a service

**Linux (systemd)**

Create `/etc/systemd/system/momentum.service`:

```ini
[Unit]
Description=Momentum Task Server
After=network.target

[Service]
ExecStart=/usr/local/bin/momentum-server
Environment="DB_PATH=/var/lib/momentum/momentum.db"
Environment="PORT=8080"
Environment="MOMENTUM_ENCRYPTION_KEY=<your-key>"
WorkingDirectory=/usr/local/share/momentum
Restart=on-failure
User=momentum

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now momentum
```

**Windows (service)**

Register the binary as a Windows service using [NSSM](https://nssm.cc/) or the built-in `sc` command:

```powershell
sc.exe create Momentum binPath= "C:\momentum\momentum-server.exe" start= auto
sc.exe start Momentum
```

---

## PWA Web Assets

The Momentum web frontend is a Progressive Web App (PWA) rendered server-side with Go templates and enhanced with vanilla JavaScript. The server binary serves both the templates and the static assets (PWA manifest, icons, service worker).

### Directory layout

```
web/
  templates/
    index.html      # Kanban board template (Go html/template)
  static/
    manifest.json   # PWA manifest (name, icons, display mode)
    icons/          # App icons (192×192 and 512×512 PNG) — add before shipping
```

### Quick package (current tree)

```bash
./scripts/release/build-pwa.sh
```

**Artifacts produced:**

```
dist/web/
  templates/
    index.html
  static/
    manifest.json
dist/web.tar.gz          # compressed archive for deployment
dist/web.tar.gz.sha256   # SHA-256 checksum
```

### Serving the assets

The server binary embeds the templates and static assets at build time and serves static assets at the `/static/` URL prefix. No `web/` directory is required beside the binary, and the application may be launched from any working directory:

```
/usr/local/bin/momentum-server  # self-contained server binary
```

### PWA installability checklist

A browser will offer "Add to Home Screen" / install prompt when:

- [x] `manifest.json` is served at `/static/manifest.json`
- [x] `<link rel="manifest">` is present in the HTML
- [ ] App icons (192×192 and 512×512 PNG) are added to `web/static/icons/` and listed in `manifest.json` (**required** for full installability; the `icons` array in `manifest.json` is intentionally empty until icons are created)
- [ ] The server is accessed over HTTPS (required in production; `localhost` is exempt)

---

## Complete Local Build

To build all artifacts in one step:

```bash
VERSION=$(git describe --tags --always --dirty) \
  ./scripts/release/build-server.sh

VERSION=$(git describe --tags --always --dirty) \
  ./scripts/release/build-pwa.sh
```

After both scripts complete, `dist/` contains:

```
dist/
  server/
    momentum-server             # Linux/macOS binary
    momentum-server.sha256      # Linux/macOS checksum
    momentum-server.exe         # Windows binary
    momentum-server.exe.sha256  # Windows checksum
  web/
    templates/
    static/
  web.tar.gz
  web.tar.gz.sha256
```

---

## CI Artifacts

The `build-and-test-go` CI job builds the server binary on every push and pull request and uploads it as a GitHub Actions artifact named `momentum-server` (retained for 30 days). These CI artifacts are for development verification only — use [GitHub Releases](https://github.com/chrisbelyea/momentum/releases) for stable, versioned binaries intended for deployment.

See [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) for CI details and [`.github/workflows/release.yml`](../.github/workflows/release.yml) for the release workflow.
