# Qrypt Architecture

Technical decisions and the structure of the code. How Qrypt looks and behaves on screen is in [DESIGN.md](DESIGN.md). Formats and protocols are specified in `docs/`; this file is the index and the record of decisions, and it links instead of copying.

> **Status legend.** **(implemented)** matches the current code. **(proposed)** is decided but not built yet.

## 1. Packages

| Package | Purpose | Imports (internal) |
| --- | --- | --- |
| `cmd/qrypt` | Entry point: starts the TUI, `--preview` on throwaway data | `identity`, `tui` |
| `internal/identity` | Seed identity, recovery phrase, sealing and loading, derived keys | `secmem` |
| `internal/secmem` | Page-backed, best-effort locked key buffers | none |
| `internal/vault` | Encrypted items, scopes (spaces), signed manifest, rollback record | `identity` |
| `internal/mirror` | Whole-file replica to untrusted storage; `Storage` interface and the folder backend | `identity`, `vault` |
| `internal/settings` | Non-secret interface settings (language, theme, colors) | none |
| `internal/clipboard` | Native clipboard with expiry, excluded from history | none |
| `internal/tui` | Bubble Tea model, screens, wide mode, animation | all of the above, `tui/theme` |
| `internal/tui/theme` | Palettes, roles, custom color validation | none |

Rules:

- Dependencies point inward: `tui` depends on everything, nothing depends on `tui`.
- `settings`, `clipboard`, `secmem` and `theme` import no internal package; `identity` imports `secmem`.
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
- **Decision:** direct dependencies are Bubble Tea, Lip Gloss, `circl`, `x/crypto`, `x/sys` and `x/term`. No CLI framework, no configuration library. The locked-memory implementation uses the existing `x/sys`; no new dependency was added.
- **Alternatives:** Cobra and Viper for commands and configuration.
- **Consequences:** a little more hand-written code; a smaller supply chain.

### D9. Best-effort page locking for keys (implemented, phase 1; warning in phase 2)

- **Context:** swap, hibernation and dumps can retain keys. Manual `clear` alone cannot prevent paging. TUI input and copied values still use immutable Go strings.
- **Decision:** use the small `internal/secmem` package with anonymous private `mmap` on Linux/macOS and `VirtualAlloc` on Windows, `mlock`/`VirtualLock`, and Linux `MADV_DONTDUMP`. Page allocations and locks are best effort: a refusal records its first reason, but the buffer stays usable (heap fallback if allocation fails); close wipes and releases it. Long-lived identity seed and scope-derived and signing key bytes use these buffers. The TUI keeps typed text, the passphrase, the recovery phrase and the copied value as byte slices that are overwritten when discarded (`identity.Seal`, `RecoveryPhrase` and `FromRecoveryPhrase` use bytes too). These slices are ordinary heap memory, not locked pages, and Bubble Tea key events, the clipboard API and rendering still create short-lived strings that Go cannot wipe.
- **Alternatives:** `memguard` v0.23.0 was rejected because its page-lock failure panics and triggers a purge of all live buffers, and purge itself may panic; that contradicts continued operation on lock refusal. Manual `clear` alone does not prevent paging.
- **Consequences:** no new dependency. Buffers reject requests over 1 MiB without allocating; their status records the first refusal. An unlocked vault warns when `secmem.Status` reports refusal (wide status bar or narrow Spaces). This does **not** provide guard pages, canaries, encryption of keys at rest in RAM, protection of copies made by the Go runtime or cryptographic libraries, or protection for TUI strings. Bubble Tea key events already arrive as Go strings; revealed values necessarily become strings while visible. The TUI passphrase, recovery input, field editor and clipboard-held value remain strings pending completion of phase 2.
- **Known copies outside the locked pages** (found by independent verification): the X-Wing private key object that the HPKE library builds for each wrap and unwrap cannot be cleared; Argon2's working memory (256 MiB) is not wiped; HKDF and HMAC internal state is not wiped; Ed25519 signing uses a short-lived heap copy that is cleared, because Go's Ed25519 implementation needs a heap pointer; the ML-DSA key is expanded per signature and zeroed after use. An identity produced by a command that is still running when the user presses Ctrl+C is released by the process exit, not wiped. `Identity.Close` is safe against concurrent use (a mutex guards the seed), and `Buffer.Destroy` frees once however often it is called. The Linux and macOS paths compile, pass vet and their tests run in CI on `ubuntu-latest` and `macos-latest`; whether those runners grant the page lock is not known. **Further limits found by the 2026-11 adversarial review and accepted:** the key derived from the passphrase and the KEM key object built by `PublicKeys` stay in the heap until the garbage collector frees them (the libraries offer no way to clear them); the identity file and the vault write a file and rename it, but the directory is not synced, so after a power cut a just-saved file can be missing (the recovery phrase still rebuilds the identity); `Load` blocks on a special file such as a pipe at the identity path; a mirror folder that is itself a symbolic link is followed; an interrupted `Restore` (process killed between two renames) leaves the old scopes in a `.restore-*` folder that `OpenScopes` does not recover; `Accept` removes unreadable files before it writes the new manifest, so an interruption in between needs a second acceptance; only Linux asks the system to leave locked pages out of core dumps (`MADV_DONTDUMP`), so on Windows and macOS a crash dump can contain key pages; `secmem.SimulateFailureForTest` is exported from the production package.

## 4. Quality and delivery

| Area | Decision | Status |
| --- | --- | --- |
| Tests | `go vet ./...` and `go test ./...`; TUI tests read content through `plain(m)` and the frame through `framed(m)`; cross-platform vets cover page locking | implemented |
| Fuzzing | Native Go fuzz tests kept in the repository for every parser of untrusted bytes: item file, item head, fields, scope file, scope name file, manifest, record, identity file layout, typed passphrase, settings | implemented |
| Lint | `golangci-lint` v2 with a committed `.golangci.yml` and an Ubuntu CI job; reports 0 issues. Unchecked errors on cleanup paths that predate the linter are excluded by narrow rules | implemented |
| Vulnerabilities | `govulncheck` Ubuntu CI job; dependency updates are done by hand (Dependabot was removed at the owner's request) | implemented |
| CI | GitHub Actions: vet and test on Windows, Linux and macOS; lint and `govulncheck` on Ubuntu | implemented |
| Releases | `build.ps1` builds a trimmed static Windows binary, bundles licenses and the Symbols Nerd Font (pinned SHA-256), and writes checksums | implemented |
| Releases | GoReleaser v2 builds amd64 Windows/Linux/macOS archives and checksums; Cosign signs the checksum file; draft prerelease (Scoop/Homebrew publishing deferred) | implemented |
| Demos | VHS scripts in `design/vhs/` for README recordings and visual review; written, not recorded yet | implemented |

## 5. Platform support

- **Windows:** primary platform; native clipboard with expiry and history exclusion.
- **Linux and macOS:** anonymous private pages use `mmap` and `mlock`; Linux additionally excludes them from core dumps with `MADV_DONTDUMP`. Cross-platform vet checks compilation, not whether the host will grant a page lock; clipboard support remains host-dependent.

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
