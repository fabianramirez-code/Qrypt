# PQC (Post-Quantum Cryptography) TUI Secrets and Password Manager

An ultralight, secure, TUI-based secrets manager designed to run 100% on top of Git repositories as its storage and identity backend, with no need for complex centralized servers.

> **Status:** early stage. Qrypt can create an identity; it cannot unlock one or store secrets yet. Nothing is audited: do not rely on Qrypt for real secrets. The design is documented in [`docs/`](docs/).

## Key Features

- **Workspaces:** Personal and Enterprise.
- **Hierarchy by environment** (`DEV`, `QA`, `PRO`) and by category.
- **One protection level:** personal, test, and production secrets are all protected the same way.
- **Quantum-resistant hybrid cryptography** (classical + NIST post-quantum standards):
  - **X25519 + ML-KEM-768** (FIPS 203) for key encapsulation. Secrets are encrypted with a symmetric AEAD.
  - **Ed25519 + ML-DSA-65** (FIPS 204) for digital signatures.
- **Access control** based on public keys and per-recipient encryption.
- **Secure memory handling:** direct wiping of in-RAM buffers and a self-clearing clipboard.

## Tech Stack

| Area          | Technology                                                                    |
| ------------- | ----------------------------------------------------------------------------- |
| Language      | Go (Golang)                                                                   |
| TUI Framework | `charmbracelet/bubbletea` & `charmbracelet/lipgloss`                          |
| Cryptography  | Hybrid: X25519 + ML-KEM-768, Ed25519 + ML-DSA-65                              |
| Crypto code   | Go standard library, `golang.org/x/crypto`, `cloudflare/circl` (ML-DSA only)  |
| Storage       | File-based vaults on top of Git (GitHub integration)                          |

## Design

| Document                             | Content                                                              |
| ------------------------------------ | -------------------------------------------------------------------- |
| [Key model](docs/key-model.md)       | Key types, private key protection, and encryption for several people |
| [Identity](docs/identity.md)         | Exact constructions and the identity file format                     |
| [Terminal interface](docs/tui.md)    | Screens and the rules they follow                                    |
| [Threat model](docs/threat-model.md) | Main attack paths and the planned defenses                           |

## Project Structure

```
.
├── cmd/        # Binary entry points
├── internal/   # Private application code
└── pkg/        # Public reusable packages
```

## Usage

Run Qrypt from an interactive terminal. On Windows, use Windows Terminal, PowerShell or cmd.

```
go run ./cmd/qrypt
```

Qrypt first asks for the language of the interface: English or Spanish.

The first run creates your identity. It shows your passphrase and your recovery phrase so you can write them on paper, and it saves nothing until you have confirmed both. The words of both phrases are always in English. See [Terminal interface](docs/tui.md).

At this stage you can delete the identity file to start over. Its location is shown at the end.

## Development

Qrypt requires Go 1.26.7 or later. An older Go installation downloads that version automatically. Run the tests with:

```
go test ./...
```

## License

To be defined.
