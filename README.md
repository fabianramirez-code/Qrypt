# PQC (Post-Quantum Cryptography) TUI Secrets and Password Manager

An ultralight, secure, TUI-based secrets manager designed to run 100% on top of Git repositories as its storage and identity backend, with no need for complex centralized servers.

> **Status:** early stage. There is no implementation code yet. The design is documented in [`docs/`](docs/).

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
| Cryptography  | Hybrid: X25519 + ML-KEM-768, Ed25519 + ML-DSA-65 (PQC via `cloudflare/circl`) |
| Storage       | File-based vaults on top of Git (GitHub integration)                          |

## Design

| Document                             | Content                                                              |
| ------------------------------------ | -------------------------------------------------------------------- |
| [Key model](docs/key-model.md)       | Key types, private key protection, and encryption for several people |
| [Threat model](docs/threat-model.md) | Main attack paths and the planned defenses                           |

## Project Structure

```
.
├── cmd/        # Binary entry points
├── internal/   # Private application code
└── pkg/        # Public reusable packages
```

## License

To be defined.
