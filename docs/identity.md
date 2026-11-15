# Identity

How Qrypt creates an identity and stores it on disk. This document fixes the exact constructions behind decisions 1 and 2 of the [key model](key-model.md).

| Element                  | Choice                                   |
| ------------------------ | ---------------------------------------- |
| Master seed              | 32 random bytes                          |
| Recovery phrase          | The master seed as 24 BIP39 words        |
| Key derivation           | HKDF-SHA-256, one label per key          |
| Encryption key pair      | X-Wing (ML-KEM-768 + X25519)             |
| Signing key pairs        | Ed25519 and ML-DSA-65                    |
| Passphrase stretching    | Argon2id: 256 MiB, 3 passes, 4 lanes     |
| Identity file encryption | XChaCha20-Poly1305                       |
| Identity file            | 96 bytes, fixed layout, format version 1 |

> **Status:** format version 1. Not audited. Do not store real secrets with Qrypt yet.

Version 1 fixes every algorithm and parameter. Nothing is configurable, and the only setting read from the file is its version. Changing any value below requires a new version.

## 1. Master seed

The master seed is 32 bytes from the operating system's random generator. Everything else in the identity is derived from it, so the seed is the identity.

## 2. Recovery phrase

The recovery phrase is the master seed written as 24 English words, using the BIP39 word encoding: the 256 bits of the seed plus an 8-bit SHA-256 checksum, split into 24 groups of 11 bits. Each group selects one word from the 2,048-word BIP39 English list.

- Qrypt uses only the BIP39 word encoding. It does not use BIP39's PBKDF2 seed derivation or its optional passphrase: the words decode straight back to the master seed.
- When reading a phrase, Qrypt ignores letter case and extra whitespace. It then requires exactly 24 known words and a valid checksum.
- The phrase is shown once, at creation. Qrypt never stores it.

## 3. Key derivation

Each key is derived from the master seed with HKDF-SHA-256 (RFC 5869), using an empty salt and a label that is unique to that key. Each derivation outputs 32 bytes.

| Key                    | HKDF label (`info`)                     | The 32 bytes are used as  |
| ---------------------- | --------------------------------------- | ------------------------- |
| Encryption             | `qrypt/v1/identity/kem/mlkem768-x25519` | X-Wing private key (seed) |
| Classical signature    | `qrypt/v1/identity/sig/ed25519`         | Ed25519 seed              |
| Post-quantum signature | `qrypt/v1/identity/sig/mldsa65`         | ML-DSA-65 seed            |

### Encryption key pair

X-Wing is a published hybrid of ML-KEM-768 and X25519: the `MLKEM768-X25519` KEM of the IETF HPKE post-quantum draft. Qrypt uses it instead of combining the two algorithms itself. Its private key is a 32-byte seed and its public key is 1,216 bytes.

### Signing key pairs

Qrypt derives two independent signing key pairs: Ed25519 (32-byte public key) and ML-DSA-65 (1,952-byte public key). How the two signatures are combined is decided with the signing feature.

## 4. Identity file

The identity file stores the master seed, encrypted with a key derived from the passphrase. It is always 96 bytes:

| Offset | Size | Field                                                                                  |
| ------ | ---- | -------------------------------------------------------------------------------------- |
| 0      | 7    | Magic: the ASCII bytes `QRYPTID`                                                       |
| 7      | 1    | Version: `0x01`                                                                        |
| 8      | 16   | Salt (random)                                                                          |
| 24     | 24   | Nonce (random)                                                                         |
| 48     | 48   | Ciphertext: the encrypted master seed (32 bytes) and its authentication tag (16 bytes) |

### Passphrase to key

Argon2id (RFC 9106) with the salt above, 256 MiB of memory, 3 passes, 4 lanes, and a 32-byte output. This is the RFC's option for memory-constrained environments (3 passes, 4 lanes, 64 MiB) with four times the memory.

The passphrase is used as its UTF-8 bytes, exactly as given, and must not be empty.

### Encryption

XChaCha20-Poly1305 with the derived key and the nonce above. The first 24 bytes of the file (magic, version, and salt) are authenticated as associated data.

### Writing

The salt and the nonce are generated again, at random, every time an identity is encrypted.

### Reading

- Qrypt rejects a file that is not exactly 96 bytes or has a different magic.
- Qrypt rejects a file with an unknown version.
- If decryption fails, Qrypt cannot tell a wrong passphrase from a damaged file, and reports both as the same error.

## 5. Storage

| System  | Default location                                                 |
| ------- | ---------------------------------------------------------------- |
| Windows | `%AppData%\qrypt\identity`                                       |
| macOS   | `~/Library/Application Support/qrypt/identity`                   |
| Linux   | `$XDG_CONFIG_HOME/qrypt/identity`, or `~/.config/qrypt/identity` |

- **Never overwritten.** Qrypt refuses to write an identity file where one already exists. Replacing an identity by accident would lose access to everything encrypted for it.
- **Owner only.** The directory is created with mode `0700` and the file with mode `0600`. On Windows these modes have no effect, and the file relies on the permissions of the user profile.
- **Never in a repository.** The identity file must not be stored in Git.

## 6. Libraries

Qrypt prefers the Go standard library and adds a dependency only for what the standard library lacks.

| Need                                   | Library                                                                       |
| -------------------------------------- | ----------------------------------------------------------------------------- |
| X-Wing, Ed25519, HKDF, SHA-256, random | Go standard library (Go 1.26 or later)                                        |
| Argon2id, XChaCha20-Poly1305           | `golang.org/x/crypto`                                                         |
| ML-DSA-65                              | `github.com/cloudflare/circl`, until the standard library provides it         |
| BIP39 word encoding                    | Implemented in Qrypt, checked against the official word list and test vectors |

These two libraries bring in `golang.org/x/sys` indirectly, for CPU feature detection. No other module is used.

The implementation is the package `internal/identity`. Its tests rebuild the keys and a reference identity file directly from this document.

## Open questions

- **Passphrase rules:** minimum strength, a built-in generator, and Unicode normalization of typed passphrases.
- **Secrets in memory:** see the limit described in the [threat model](threat-model.md#1-infected-device).
