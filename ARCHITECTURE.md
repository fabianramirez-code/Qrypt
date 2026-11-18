# Qrypt Architecture

Technical decisions and the structure of the code. How Qrypt looks and behaves on screen is in [DESIGN.md](DESIGN.md). Formats and protocols are specified in `docs/`; this file is the index and the record of decisions, and it links instead of copying.

> **Status legend.** **(implemented)** matches the current code. **(proposed)** is decided but not built yet.

## 1. Packages

| Package | Purpose | Imports (internal) |
| --- | --- | --- |
| `cmd/qrypt` | Entry point: starts the TUI, `--preview` on throwaway data | `identity`, `tui` |
| `internal/identity` | Seed identity, recovery phrase, sealing and loading, derived keys | none |
| `internal/vault` | Encrypted items, scopes (spaces), signed manifest, rollback record | `identity` |
| `internal/mirror` | Whole-file replica to untrusted storage; `Storage` interface and the folder backend | `identity`, `vault` |
| `internal/settings` | Non-secret interface settings (language, theme, colors) | none |
| `internal/clipboard` | Native clipboard with expiry, excluded from history | none |
| `internal/tui` | Bubble Tea model, screens, wide mode, animation | all of the above, `tui/theme` |
| `internal/tui/theme` | Palettes, roles, custom color validation | none |

Rules:

- Dependencies point inward: `tui` depends on everything, nothing depends on `tui`.
- `identity`, `settings`, `clipboard` and `theme` import no internal package.
- Only `tui` and `cmd/qrypt` talk to the terminal. No other package prints or reads input.

## 2. Reference documents

| Document | Covers |
| --- | --- |
| [docs/identity.md](docs/identity.md) | Identity file format and derived keys |
| [docs/key-model.md](docs/key-model.md) | Why the keys are arranged this way |
| [docs/vault.md](docs/vault.md) | Item, manifest and record formats; integrity |
| [docs/mirror.md](docs/mirror.md) | Replication and restore on a new device |
| [docs/settings.md](docs/settings.md) | Settings file and themes |
| [docs/tui.md](docs/tui.md) | Screen-by-screen behavior |
| [docs/threat-model.md](docs/threat-model.md) | Threats covered and limits |
| [SECURITY.md](SECURITY.md) | How to report a vulnerability |

## 3. Decisions

Each decision lists context, decision, alternatives and consequences.

### D1. Hybrid post-quantum cryptography (implemented)

- **Context:** stored secrets must stay private against an attacker who records data now and owns a quantum computer later.
- **Decision:** every public-key operation pairs a classical and a post-quantum algorithm. Key encapsulation is X-Wing (ML-KEM-768 + X25519). Signatures are Ed25519 and ML-DSA-65, both required. Implementations come from `cloudflare/circl` and `golang.org/x/crypto`.
- **Alternatives:** post-quantum only (younger algorithms, no fallback); classical only (no quantum resistance).
- **Consequences:** larger keys and signatures; an attacker has to break both families.

### D2. One seed, derived keys (implemented)

- **Context:** a user must be able to rebuild everything from a recovery phrase.
- **Decision:** the identity is one seed. Every key is derived from it with HKDF-SHA-256. The seed is sealed on disk with Argon2id (t=3, 256 MiB, p=4, 32-byte key) and XChaCha20-Poly1305.
- **Alternatives:** independent random keys in a keyring file (cannot be rebuilt from a phrase).
- **Consequences:** the recovery phrase is the whole identity and has to be protected as such; unlocking costs 256 MiB of RAM by design.

### D3. One encrypted file per item, opaque names (implemented)

- **Context:** the vault directory may be copied to storage the user does not control.
- **Decision:** each item is its own file (`QRYPTSE` magic; version 2 is written, versions 1 and 2 are read). The title, folder path and fields are encrypted and padded; file names are opaque. Data segments use XChaCha20-Poly1305.
- **Alternatives:** one database file (every change rewrites and re-syncs everything; a partial write risks the whole vault).
- **Consequences:** file count and rough sizes are visible; names, folders and field counts are not. Folders are paths inside items, not directories.

### D4. Spaces are scopes with their own key (implemented)

- **Context:** groups of secrets will later be shared with different people.
- **Decision:** each space is a directory `scopes/<opaque-id>` with a random 32-byte scope key, wrapped to the identity with HPKE (X-Wing, HKDF-SHA-256, ChaCha20-Poly1305). Name, value and scope-name keys derive from it. The space's name is encrypted.
- **Alternatives:** one key for the whole vault (sharing one space would expose all of them).
- **Consequences:** sharing later means wrapping one scope key to another person's public key, without re-encrypting items.

### D5. Signed manifest and rollback record (implemented)

- **Context:** untrusted storage can delete, replace or restore old files.
- **Decision:** each scope has a manifest with a generation number and name and file hashes, signed with Ed25519 and ML-DSA-65. Changes write `manifest.next`, perform the action, then rename. A local record keeps `scope-id generation manifest-hash` per scope and detects rollback and forks. A mismatch opens the Problem screen; accepting requires a typed word.
- **Alternatives:** trusting per-file authentication only (cannot detect a removed or outdated file).
- **Consequences:** every change rewrites the manifest; the record is local, so a brand-new device cannot detect a rollback that happened before its first restore.

### D6. Mirror through a small storage interface (implemented)

- **Context:** users want a copy elsewhere, and backends will vary.
- **Decision:** `mirror.Storage` has four methods (`List`, `Read`, `Write`, `Remove`) over whole files. The only backend today is a folder with owner-only atomic writes. The mirror holds ciphertext only and is never trusted.
- **Alternatives:** a sync protocol with diffs (more code in the trusted base for little gain at this size).
- **Consequences:** a new backend is one small type; no merge between two devices yet (see section 6).

### D7. Settings hold nothing secret (implemented)

- **Context:** language and theme are needed before the identity is unlocked.
- **Decision:** a plain `key=value` file with validated values. Unknown or invalid lines are skipped, so an older or newer file never blocks startup. A removed theme name falls back to `green`.
- **Alternatives:** a configuration library such as Viper (many dependencies for a handful of keys).
- **Consequences:** the settings parser is small enough to fuzz and to audit.

### D8. Few dependencies (implemented)

- **Context:** every dependency of a secrets manager is part of what must be trusted.
- **Decision:** direct dependencies are Bubble Tea, Lip Gloss, `circl`, `x/crypto`, `x/sys` and `x/term`. No CLI framework, no configuration library. A new dependency needs a decision in this file.
- **Alternatives:** Cobra and Viper for commands and configuration.
- **Consequences:** a little more hand-written code; a smaller supply chain.

### D9. Memory protection with memguard (proposed)

- **Context:** the threat model includes swap files, hibernation files and memory dumps. Today keys are `[]byte` cleared with `clear` on lock, but the passphrase, typed input and the value being copied travel through the TUI as Go `string`, which cannot be wiped.
- **Decision:** add `github.com/awnumar/memguard`.
  - Private keys and scope keys live in enclaves (encrypted in RAM) and are opened into locked buffers only for one operation.
  - Decrypted values are opened at the moment of use (reveal, copy, edit) and destroyed right after; lock, auto-hide and quit destroy all of them.
  - Secrets travel through the model as `[]byte` or enclaves, never as `string`; the TUI renders a masked copy.
  - `memguard.CatchInterrupt` and `memguard.Purge` run on every exit path.
  - If the operating system refuses to lock pages (`VirtualLock` on Windows, `mlock` elsewhere), Qrypt keeps working and shows a warning in the status bar.
- **Alternatives:** keep manual `clear` only (no protection against swap, and strings stay); `x/sys` calls written by hand (re-implements memguard in the trusted base).
- **Consequences:** one more dependency in a critical path (exception to D8, accepted); text input has to stop using strings for secrets; a string still exists for the instant a value is drawn revealed on screen, which no library can avoid.

## 4. Quality and delivery

| Area | Decision | Status |
| --- | --- | --- |
| Tests | `go vet ./...` and `go test ./...`; TUI tests read content through `plain(m)` and the frame through `framed(m)` | implemented |
| Fuzzing | Native Go fuzz tests kept in the repository for every parser of untrusted bytes: item file, fields, manifest, record, settings | proposed |
| Lint | `golangci-lint` with a committed `.golangci.yml` | proposed |
| Vulnerabilities | `govulncheck` in CI; Dependabot for Go modules and GitHub Actions | proposed |
| CI | GitHub Actions: vet, test, lint and `govulncheck` on Windows, Linux and macOS | proposed |
| Releases | `build.ps1` builds a trimmed static Windows binary, bundles licenses and the Symbols Nerd Font (pinned SHA-256), and writes checksums | implemented |
| Releases | GoReleaser for cross-compilation and checksums, artifacts signed with Cosign; Scoop and Homebrew manifests generated from it | proposed |
| Demos | VHS scripts for README recordings and visual review | proposed |

## 5. Platform support

- **Windows:** primary platform; native clipboard with expiry and history exclusion.
- **Linux and macOS:** the code builds, but clipboard support and page locking are not verified there. CI on the three systems (section 4) is the first step.

## 6. Open decisions and non-goals

Open:

1. Sharing a space with other people: membership, and how a member's public key is trusted.
2. Merging changes made on two devices.
3. More mirror backends behind `mirror.Storage`.
4. External security audit before a release is published as "Latest".
5. Whether `odd/` (working notes) is versioned; today it is git-ignored.

Non-goals:

- A graphical, web or mobile interface.
- A server or an account system run by the project.
- Reading secrets from pipes or writing them to standard output.
