# PQC (Post-Quantum Cryptography) TUI Secrets and Password Manager

An ultralight, secure, TUI-based secrets manager whose vault is a local directory of encrypted files, with an optional mirror folder (for example one synchronized by a cloud service). It needs no server and no Git repository.

> **Status:** early stage. Qrypt can create, unlock and restore an identity, and store, view, edit and delete items in a local vault. Nothing is audited: do not rely on Qrypt for real secrets. The design is documented in [`docs/`](docs/).

## Key Features

- **Spaces:** each space has its own key and one member, the identity that created it. Sharing a space with other people is planned, not built.
- **Folders** inside spaces, at any depth, and accounts with several secrets each (hidden, visible or long text).
- **One protection level:** personal, test, and production secrets are all protected the same way.
- **Quantum-resistant hybrid cryptography** (classical + NIST post-quantum standards):
  - **X25519 + ML-KEM-768** (FIPS 203) for key encapsulation. Secrets are encrypted with a symmetric AEAD.
  - **Ed25519 + ML-DSA-65** (FIPS 204) for digital signatures.
- **Integrity:** a signed manifest per space detects a removed file or a rollback.
- **Secure memory handling:** keys and typed secrets are held in locked, wiped memory where the system allows it (the limits are listed in `ARCHITECTURE.md`, D9), and the clipboard clears itself.

## Tech Stack

| Area          | Technology                                                                    |
| ------------- | ----------------------------------------------------------------------------- |
| Language      | Go (Golang)                                                                   |
| TUI Framework | `charmbracelet/bubbletea` & `charmbracelet/lipgloss`                          |
| Cryptography  | Hybrid: X25519 + ML-KEM-768, Ed25519 + ML-DSA-65                              |
| Crypto code   | Go standard library, `golang.org/x/crypto`, `cloudflare/circl` (ML-DSA only)  |
| Storage       | Local vault directory with an optional mirror folder                          |

## Design

| Document                             | Content                                                              |
| ------------------------------------ | -------------------------------------------------------------------- |
| [Architecture](ARCHITECTURE.md)      | Packages, technical decisions, quality and delivery                  |
| [Design system](DESIGN.md)           | Themes, layout, components, motion and accessibility                 |
| [Key model](docs/key-model.md)       | Key types, private key protection, and encryption for several people |
| [Identity](docs/identity.md)         | Exact constructions and the identity file format                     |
| [Vault](docs/vault.md)               | Exact constructions and the secret file format                       |
| [Terminal interface](docs/tui.md)    | Screens and the rules they follow                                    |
| [Settings](docs/settings.md)         | Language, themes and custom colors                                   |
| [Mirror](docs/mirror.md)             | The optional second copy of the vault                                |
| [Threat model](docs/threat-model.md) | Main attack paths and which defenses exist                           |

## Project Structure

```
.
├── cmd/        # Binary entry points
└── internal/   # Private application code (identity, vault, mirror, TUI...)
```

## Download

Prebuilt programs are published (Windows ZIP, Linux and macOS tar.gz) on the [Releases](https://github.com/fabianramirez-code/Qrypt/releases) page. You do not need Go to run it.

1. Download `qrypt-<version>-windows-amd64.zip` and unzip it.
2. Double-click `qrypt.exe`, or run it from Windows Terminal, PowerShell or cmd.

The program is not signed. The first time, Windows may show a "Windows protected your PC" notice: choose "More info", then "Run anyway". To check the download, compare it with the `.sha256` file published next to it:

```
Get-FileHash .\qrypt-<version>-windows-amd64.zip -Algorithm SHA256
```

## Usage

To run Qrypt from source, use an interactive terminal. On Windows, use Windows Terminal, PowerShell or cmd.

```
go run ./cmd/qrypt
# Explore with temporary sample data, without a passphrase:
go run ./cmd/qrypt --preview
```

Qrypt first asks for the language of the interface: English or Spanish.

The first run creates your identity. It shows your passphrase and your recovery phrase so you can write them on paper, and it saves nothing until you have confirmed both. The words of both phrases are always in English. See [Terminal interface](docs/tui.md).

On later runs Qrypt asks for your passphrase to unlock the identity. `--preview` instead opens a temporary sample vault after language selection; it never accesses your real identity or vault, and removes the sample data on exit. See [Preview mode](docs/tui.md#preview-mode).

At this stage you can delete the identity file to start over. Its location is shown at the end.

## Development

Qrypt requires Go 1.26.7 or later. An older Go installation downloads that version automatically. Run the tests with:

```
go test ./...
```

### Development checks

CI runs `go vet ./...` and `go test ./... -count=1` on Windows, Linux and macOS. On Ubuntu it also runs `golangci-lint` (configured in `.golangci.yml`) and `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`. Run the linter locally with `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...`. Findings that existed before the linter was added (unchecked errors on cleanup paths, a few unused helpers) are excluded by narrow rules in `.golangci.yml`; new code must pass without new exclusions.

### Releases

The tag workflow (`v*`) runs GoReleaser v2 (`go run github.com/goreleaser/goreleaser/v2@latest check` validates its configuration), producing Windows ZIP and Linux/macOS tar.gz archives for amd64, a SHA-256 `checksums.txt`, and a keyless Cosign signature and certificate for that checksum file. Releases stay draft/prerelease and must be reviewed before publication. Scoop and Homebrew manifests require additional repositories or tokens and are deferred.

For now, `build.ps1` remains the way to produce the Windows ZIP with the SHA-256-pinned Symbols Nerd Font; GoReleaser does not bundle the font (or the script's complete dependency licenses).

To build the Windows release zip:

```
.\build.ps1 -Version v0.1.0
```

The script uses Go from your PATH. If Go is not installed, it downloads the official Go zip into a local `go` folder, checks its SHA-256, and uses it from there. Nothing is installed on the system. The result is written to `dist`.

## License

Qrypt is licensed under the [Apache License 2.0](LICENSE).
