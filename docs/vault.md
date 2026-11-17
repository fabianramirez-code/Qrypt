# Vault

How Qrypt stores secrets on disk. This document fixes the exact constructions behind the vault format decision: **one encrypted file per secret, with an opaque file name and the real name encrypted inside the file**. It builds on the [key model](key-model.md) and the [identity](identity.md).

| Element             | Choice                                                         |
| ------------------- | -------------------------------------------------------------- |
| Scope key           | 32 random bytes                                                |
| Scope key wrapping  | HPKE (RFC 9180) with X-Wing, HKDF-SHA-256, ChaCha20-Poly1305   |
| Secret file         | One file per secret, random 128-bit name, format version 1     |
| Secret encryption   | XChaCha20-Poly1305, one key and one tag for the name, another for the value |
| Subkeys             | HKDF-SHA-256 from the scope key, one label per use             |
| Size hiding         | Names padded to 256 bytes, values padded to a multiple of 256  |

> **Status:** format version 1, design stage. Not audited. Do not store real secrets with Qrypt yet.

Version 1 fixes every algorithm and parameter. Nothing is configurable, and the only setting read from a file is its version. Changing any value below requires a new version.

## Scope of version 1

Version 1 has **one scope with one member**: the identity that created it. The files already carry what several members and several scopes will need (a scope id and a list of wrapped keys), so adding them later does not change how a secret file is read. How scopes are named, how members are added or removed, and how public keys are trusted is decided with those features.

## 1. Scope key

The scope key is 32 bytes from the operating system's random generator. Every secret of the scope is encrypted under keys derived from it. The scope key is never stored in the clear: it is stored wrapped for the member (section 2).

Two subkeys are derived from it with HKDF-SHA-256 (RFC 5869), using an empty salt and a unique label. Each derivation outputs 32 bytes.

| Subkey     | HKDF label (`info`)           | Used for                    |
| ---------- | ----------------------------- | --------------------------- |
| Name key   | `qrypt/v1/vault/secret/name`  | Encrypting a secret's name  |
| Value key  | `qrypt/v1/vault/secret/value` | Encrypting a secret's value |

Separate keys for the name and the value mean the name segment can be decrypted to list secrets without ever decrypting a value.

## 2. Scope file

The scope file stores the scope id and the scope key wrapped for each member. In version 1 it has exactly one member, so it is always 1,225 bytes:

| Offset | Size  | Field                                                                      |
| ------ | ----- | -------------------------------------------------------------------------- |
| 0      | 7     | Magic: the ASCII bytes `QRYPTSK`                                           |
| 7      | 1     | Version: `0x01`                                                            |
| 8      | 16    | Scope id (random)                                                          |
| 24     | 1     | Member count: `0x01`                                                       |
| 25     | 32    | Member fingerprint: SHA-256 of the member's X-Wing public key (1,216 bytes) |
| 57     | 1,168 | Wrapped scope key                                                          |

The wrapped scope key is the output of a single-shot HPKE seal (RFC 9180, base mode) to the member's X-Wing public key: the KEM `MLKEM768-X25519` (the same X-Wing used by the identity), the KDF HKDF-SHA-256, and the AEAD ChaCha20-Poly1305. It is the encapsulated key (1,120 bytes) followed by the encrypted scope key (32 bytes) and its tag (16 bytes), 1,168 bytes in total.

- **HPKE `info`:** the ASCII label `qrypt/v1/vault/scope-key`, followed by the 16 bytes of the scope id. This binds the wrapped key to its scope.
- **Fingerprint:** lets a client find its own entry without trying every wrap. It is not secret and not trusted: a wrong fingerprint only makes the unwrap fail.

### Reading

- Qrypt rejects a file of the wrong size, with a different magic, an unknown version, or a member count other than 1.
- Qrypt rejects a file whose fingerprint does not match the identity's public key, before any decapsulation.
- If unwrapping fails, Qrypt cannot tell a wrong identity from a damaged file, and reports both as the same error.

## 3. Secret file

A secret file holds one secret: its name and its value, each sealed on its own.

| Offset | Size          | Field                                                                           |
| ------ | ------------- | ------------------------------------------------------------------------------- |
| 0      | 7             | Magic: the ASCII bytes `QRYPTSE`                                                |
| 7      | 1             | Version: `0x01`                                                                 |
| 8      | 24            | Name nonce (random)                                                             |
| 32     | 272           | Sealed name: 256 bytes of padded name and a 16-byte tag                         |
| 304    | 24            | Value nonce (random)                                                            |
| 328    | 4             | Value segment size: the length of the sealed value, as a big-endian 32-bit integer |
| 332    | 272 to 65,808 | Sealed value: the padded value and a 16-byte tag                                |

### Name

The name is the label the user sees, such as `github token`. It must be valid UTF-8, between 1 and 255 bytes, with no control characters. The 256-byte name segment is the name's length in one byte, the name, and zero bytes up to 256.

Names are unique within a scope, compared byte for byte.

### Value

The value is the secret's bytes, from 0 to 65,536 bytes. The value segment is the value's length as a big-endian 32-bit integer, the value, and zero bytes up to the next multiple of 256 bytes in total.

### Encryption

Each segment is encrypted with XChaCha20-Poly1305: the name with the name key, the value with the value key, each with its own random nonce.

The associated data of both segments is the same: the first 8 bytes of the file (magic and version), the 16 bytes of the scope id, and the 16 bytes of the file id (section 4), followed by a one-byte segment tag: `0x01` for the name, `0x02` for the value. This binds each segment to its scope, to its own file name, and to its role, so a file cannot be copied to another scope, renamed, or have its name and value swapped.

### Writing

Both nonces are generated again, at random, every time a secret is written, including when its value is changed.

### Reading

- Qrypt rejects a file that is too small or too large, has a different magic, an unknown version, or a value segment size that does not match the file's size or is not valid.
- Qrypt decrypts only the segment it needs. Listing decrypts names only. Showing one secret decrypts that file's value only.
- If decryption fails, Qrypt cannot tell a damaged file from one of another scope, and reports both as the same error.

## 4. Storage

| System  | Default location                                               |
| ------- | -------------------------------------------------------------- |
| Windows | `%AppData%\qrypt\vault`                                        |
| macOS   | `~/Library/Application Support/qrypt/vault`                    |
| Linux   | `$XDG_CONFIG_HOME/qrypt/vault`, or `~/.config/qrypt/vault`     |

```text
vault/
  scope                  the scope file
  secrets/
    3f9c…(32 hex).qrs    one file per secret
```

- **Opaque names.** A secret file is named with 32 lowercase hexadecimal characters: its 16-byte **file id**, drawn at random when the secret is created. It says nothing about the secret. The file id does not change when the value is updated.
- **Never overwritten by accident.** Creating a vault refuses to write where a scope file already exists. Adding a secret refuses a name already in use.
- **Written whole or not at all.** Qrypt writes a file under a temporary name in the same directory, syncs it, and then links it to its final name. Updating a secret replaces the file with an atomic rename. After a crash a file is either complete or absent, and an update leaves either the old secret or the new one. A crash can leave a temporary file behind, with the extension `.tmp`; it is safe to delete, and Qrypt ignores it.
- **Owner only.** The directories are created with mode `0700` and the files with mode `0600`. On Windows these modes have no effect, and the files rely on the permissions of the user profile.
- **Deleting.** Deleting a secret removes its file. Qrypt does not overwrite the bytes first, because overwriting is not reliable on modern storage. Anyone who copied the file earlier can still read it, which is why removed access always requires changing the real secret.

## 5. What the files reveal

An attacker who copies the vault learns:

- how many secrets there are, and roughly how long each name and value is (values to the nearest 256 bytes; every name looks the same);
- when each file was last changed, from the file system;
- the scope id and the public-key fingerprint of the member.

They learn nothing about names or values without the scope key, which needs the identity.

## 6. Limits

- **Tampering and rollback are not detected yet.** Anyone able to write to the vault can delete a file or put back an older copy of one. A person who can write can also build a scope file wrapped to their own key. The AEAD stops them from changing what a file says, not from replacing the whole vault. Detecting this needs the signed member list and chained log of the [threat model](threat-model.md#3-tampering-with-the-vault-repository), which are not part of version 1.
- **Listing opens every name.** To list secrets or check that a name is free, Qrypt reads every file in the scope and decrypts the 256-byte name segments, not the values. The [threat model](threat-model.md#1-infected-device) already accepts that an infected device can read what the program holds in memory.
- **One damaged file stops listing.** If a name segment fails to decrypt, Qrypt reports the error instead of skipping the file, so damage is never hidden. Showing other secrets still needs the listing, so a damaged file has to be repaired or removed by hand in version 1.
- **One writer at a time.** Two programs adding the same name at the same moment can both succeed. Version 1 does not lock the vault.
- **Secrets in memory:** see the limit described in the [threat model](threat-model.md#1-infected-device).

## 7. Libraries

No new dependency. HPKE with X-Wing, HKDF-SHA-256, SHA-256, and the random generator come from the Go standard library; XChaCha20-Poly1305 comes from `golang.org/x/crypto`, already used by the identity.

The implementation is the package `internal/vault`. Its tests rebuild a scope file and a secret file directly from this document.

## Open questions

- **Scopes:** their names, how several scopes coexist, and what a scope is for personal and for work secrets.
- **Members:** adding and removing members, and trusting their public keys.
- **Signatures and the change log:** how the Ed25519 and ML-DSA-65 signatures are combined and what they cover.
- **Synchronization:** how the vault moves between devices and a repository.
