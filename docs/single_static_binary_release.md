# Single Static Binary Release Specification & Cross-Compilation Guide

`schemago` is designed to be shipped as a **single, zero-dependency, statically linked binary** across macOS, Linux, and Windows. This document details the architectural decisions, cross-compilation pipeline, release automation via GoReleaser, and verification procedures.

---

## 1. Architectural Philosophy: Zero Runtime Dependencies

To serve as a reliable deployment pipeline tool, `schemago` adheres to the "download one file, run it" superpower:

- **No CGO (`CGO_ENABLED=0`)**: Standard C libraries (`libc`, `glibc`, `musl`) are disabled. The Go runtime handles network operations, TLS encryption, and standard input/output using pure Go standard libraries (`net`, `crypto/tls`, `os`).
- **No Shared Libraries (`.so`, `.dylib`, `.dll`)**: Binaries carry zero external dynamic dependencies.
- **No External Runtimes**: No Node.js, Python, JVM, or database CLI tools (`psql`) are required.

---

## 2. Target Matrix & Cross-Compilation Support

`schemago` cross-compiles static binaries for all primary operating systems and CPU architectures:

| Operating System | Architecture | Binary Archive Target | Format |
|---|---|---|---|
| **Linux** | `amd64` (64-bit x86) | `schemago_<version>_linux_amd64.tar.gz` | `tar.gz` |
| **Linux** | `arm64` (64-bit ARM) | `schemago_<version>_linux_arm64.tar.gz` | `tar.gz` |
| **macOS (Darwin)** | `amd64` (Intel Macs) | `schemago_<version>_darwin_amd64.tar.gz` | `tar.gz` |
| **macOS (Darwin)** | `arm64` (Apple Silicon M1/M2/M3) | `schemago_<version>_darwin_arm64.tar.gz` | `tar.gz` |
| **Windows** | `amd64` (64-bit x86) | `schemago_<version>_windows_amd64.zip` | `zip` |
| **Windows** | `arm64` (64-bit ARM) | `schemago_<version>_windows_arm64.zip` | `zip` |

---

## 3. Automated Release Pipeline (GoReleaser + GitHub Actions)

Releases are fully automated via GoReleaser (`.goreleaser.yaml`) and GitHub Actions (`.github/workflows/release.yml`).

### Release Trigger
Creating and pushing a semver git tag triggers the automated release workflow:

```bash
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

### Build & Release Lifecycle
1. **Source Checkout**: Clones repo with full tag history.
2. **Environment Setup**: Configures Go environment using `go.mod`.
3. **Cross-Compilation**: GoReleaser executes builds across all 6 targets with `CGO_ENABLED=0`, `-trimpath`, and version injection (`-X github.com/parthdagia05/schemago/internal/cli.Version={{.Version}}`).
4. **Archive Packaging**: Compresses Unix binaries into `.tar.gz` and Windows binaries into `.zip`.
5. **Checksum Generation**: Computes SHA-256 digests in `checksums.txt`.
6. **GitHub Release Publication**: Drafts/publishes release assets to GitHub Releases.

---

## 4. Manual Cross-Compilation Instructions

To build static binaries locally without GoReleaser:

### Linux (amd64)
```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X github.com/parthdagia05/schemago/internal/cli.Version=manual" -o schemago-linux-amd64 ./cmd/schemago
```

### Linux (arm64)
```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w -X github.com/parthdagia05/schemago/internal/cli.Version=manual" -o schemago-linux-arm64 ./cmd/schemago
```

### macOS (Intel amd64)
```bash
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w -X github.com/parthdagia05/schemago/internal/cli.Version=manual" -o schemago-darwin-amd64 ./cmd/schemago
```

### macOS (Apple Silicon arm64)
```bash
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w -X github.com/parthdagia05/schemago/internal/cli.Version=manual" -o schemago-darwin-arm64 ./cmd/schemago
```

### Windows (amd64)
```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w -X github.com/parthdagia05/schemago/internal/cli.Version=manual" -o schemago-windows-amd64.exe ./cmd/schemago
```

### Windows (arm64)
```bash
CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags="-s -w -X github.com/parthdagia05/schemago/internal/cli.Version=manual" -o schemago-windows-arm64.exe ./cmd/schemago
```

---

## 5. Binary Verification & Inspection

To verify that a built binary is strictly static and free of shared runtime library dependencies:

### Linux Verification
```bash
# Check header format (expect: statically linked)
file schemago-linux-amd64

# Check dynamic linkage (expect: not a dynamic executable)
ldd schemago-linux-amd64
```

### macOS Verification
```bash
# Check header format
file schemago-darwin-arm64

# Verify load commands (expect no dynamic libraries loaded)
otool -L schemago-darwin-arm64
```
