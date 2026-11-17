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

Version 1 has **several scopes, each with one member**: the identity that created it. A scope is what the interface calls a *space*, such as `Personal` or `Work`. Each scope has its own key, so the files of one scope say nothing about another. The files already carry what several members and several scopes will need (a scope id and a list of wrapped keys), so adding them later does not change how a secret file is read. How scopes are named, how members are added or removed, and how public keys are trusted is decided with those features.

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

## 3. Scope name file

A scope has a name chosen by the user, such as `Work`. A name can say a lot, so it is stored encrypted, in its own file of 304 bytes:

| Offset | Size | Field                                                   |
| ------ | ---- | ------------------------------------------------------- |
| 0      | 7    | Magic: the ASCII bytes `QRYPTSN`                        |
| 7      | 1    | Version: `0x01`                                         |
| 8      | 24   | Nonce (random)                                          |
| 32     | 272  | Sealed name: 256 bytes of padded name and a 16-byte tag |

- **Key:** derived from the scope key like the other subkeys, with the label `qrypt/v1/vault/scope/name`.
- **Padding and rules:** the same as a [secret's name](#name): valid UTF-8, 1 to 255 bytes, no control characters, padded to 256 bytes.
- **Encryption:** XChaCha20-Poly1305. The associated data is the first 8 bytes of the file followed by the 16 bytes of the scope id, so a name file cannot be moved to another scope.
- **Renaming** writes a new file, with a new nonce, and replaces the old one with an atomic rename.
- **Unique names.** Two scopes of the same vault cannot have the same name, compared byte for byte.
- **A scope without a name file** is shown with the name `Personal`. This is the case of a vault created before scopes had names.

## 4. Secret file

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

The associated data of both segments is the same: the first 8 bytes of the file (magic and version), the 16 bytes of the scope id, and the 16 bytes of the file id (section 5), followed by a one-byte segment tag: `0x01` for the name, `0x02` for the value. This binds each segment to its scope, to its own file name, and to its role, so a file cannot be copied to another scope, renamed, or have its name and value swapped.

### Writing

Both nonces are generated again, at random, every time a secret is written, including when its value is changed.

### Reading

- Qrypt rejects a file that is too small or too large, has a different magic, an unknown version, or a value segment size that does not match the file's size or is not valid.
- Qrypt decrypts only the segment it needs. Listing decrypts names only. Showing one secret decrypts that file's value only.
- If decryption fails, Qrypt cannot tell a damaged file from one of another scope, and reports both as the same error.

## 5. Storage

| System  | Default location                                               |
| ------- | -------------------------------------------------------------- |
| Windows | `%AppData%\qrypt\vault`                                        |
| macOS   | `~/Library/Application Support/qrypt/vault`                    |
| Linux   | `$XDG_CONFIG_HOME/qrypt/vault`, or `~/.config/qrypt/vault`     |

```text
vault/
  scopes/
    7c1e…(32 hex)/         one directory per scope, named with its scope id
      scope                the scope file
      name                 the scope name file
      secrets/
        3f9c…(32 hex).qrs  one file per secret
```

- **Opaque names.** A secret file is named with 32 lowercase hexadecimal characters: its 16-byte **file id**, drawn at random when the secret is created. It says nothing about the secret. The file id does not change when the value is updated.
- **Opaque scope directories.** A scope directory is named with the 32 lowercase hexadecimal characters of its scope id, which is random and is not secret.
- **Earlier layout.** A vault created before scopes existed has its `scope` file and its directory of secret files directly in `vault/`. When Qrypt opens such a vault it moves both into the directory of that scope, with two renames, and changes no file. If it is interrupted between the two renames, it finishes the move the next time.
- **Removing a scope** is only possible when it holds no secret. Qrypt then removes its files and its directory.
- **Never overwritten by accident.** Creating a vault refuses to write where a scope file already exists. Adding a secret refuses a name already in use.
- **Written whole or not at all.** Qrypt writes a file under a temporary name in the same directory, syncs it, and then links it to its final name. Updating a secret replaces the file with an atomic rename. After a crash a file is either complete or absent, and an update leaves either the old secret or the new one. A crash can leave a temporary file behind, with the extension `.tmp`; it is safe to delete, and Qrypt ignores it.
- **Owner only.** The directories are created with mode `0700` and the files with mode `0600`. On Windows these modes have no effect, and the files rely on the permissions of the user profile.
- **Deleting.** Deleting a secret removes its file. Qrypt does not overwrite the bytes first, because overwriting is not reliable on modern storage. Anyone who copied the file earlier can still read it, which is why removed access always requires changing the real secret.

## 6. Integrity

Encryption stops anyone from reading or altering a file. It does not stop someone who can write to the vault from deleting a file, or from putting back an older copy of one. Each scope therefore has a **manifest**: a signed description of the files the scope holds now. Qrypt keeps no history: the manifest describes the current state only.

### Manifest file

The file `manifest` in the scope directory:

| Offset | Size  | Field                                                                              |
| ------ | ----- | ---------------------------------------------------------------------------------- |
| 0      | 7     | Magic: the ASCII bytes `QRYPTSM`                                                   |
| 7      | 1     | Version: `0x01`                                                                    |
| 8      | 16    | Scope id                                                                           |
| 24     | 8     | Generation, as a big-endian 64-bit integer. The first manifest of a scope is 1.    |
| 32     | 32    | SHA-256 of the scope name file, or 32 zero bytes when the scope has no name file   |
| 64     | 4     | Entry count, as a big-endian 32-bit integer                                        |
| 68     | 48×n  | Entries, in ascending order of file id: the file id (16 bytes) and the SHA-256 of that secret file (32 bytes) |
| …      | 64    | Ed25519 signature                                                                  |
| …      | 3,309 | ML-DSA-65 signature                                                                |

- **What is signed:** the ASCII label `qrypt/v1/vault/manifest` followed by every byte of the file before the signatures. Both signatures cover the same message.
- **Both signatures are required.** A manifest is valid only if the Ed25519 signature and the ML-DSA-65 signature both verify. An attacker must break both algorithms to forge one.
- **Keys:** the signing keys of the [identity](identity.md#signing-key-pairs) that is the scope's member. ML-DSA-65 is used in its deterministic variant, with an empty context string.
- **Not secret.** The manifest holds only what the directory already shows: how many files there are and their opaque names.

### Writing

Every change to a scope (adding, updating or deleting a secret, naming the scope) produces a manifest with the generation raised by one. So that a crash can never leave a scope that looks tampered with, Qrypt writes in this order:

1. The new manifest, describing the scope as it will be, under the name `manifest.next`.
2. The change itself, which is already whole or not at all.
3. `manifest.next` renamed over `manifest`.

### Reading

When Qrypt opens a scope it checks both signatures of the manifest, hashes the scope's files, and compares:

- **The files match the manifest:** the scope is intact. A leftover `manifest.next` is removed.
- **The files match a valid `manifest.next` whose generation is one more than the manifest's:** a change was interrupted after step 2. Qrypt completes it with the rename of step 3.
- **Anything else:** the scope has a problem, described below.

While a scope is open, every file Qrypt reads is checked against the manifest again, and a listing checks that no file is missing or unexpected.

Temporary files, with the extension `.tmp`, are not part of the scope and are ignored.

### Local record

A manifest proves that a state was written by the identity. It cannot prove that the state is the latest one: an older manifest with its older files is just as valid. To detect that, Qrypt keeps a record **outside the vault**, in the file `state` next to the identity file. For each scope it holds the scope id, the last generation seen, and the SHA-256 of that manifest.

| The manifest's generation is…                                   | Qrypt concludes                                                        |
| --------------------------------------------------------------- | ---------------------------------------------------------------------- |
| Higher than the record                                          | A newer state, for example written on another device. Accepted.        |
| Equal to the record, with the same hash                         | Unchanged. Accepted.                                                   |
| Equal to the record, with a different hash                      | Problem: two different states claim the same generation.               |
| Lower than the record                                           | Problem: the scope was rolled back to an older state.                  |
| Recorded scope whose directory or manifest is missing           | Problem: the scope was removed outside Qrypt.                          |
| A scope with a manifest and no record                           | First time on this device. Accepted and recorded.                      |
| A scope with no manifest and no record                          | Created before manifests existed. Qrypt writes its first manifest.     |

The record is written whole or not at all, after each change. It holds no secret.

### Problems and accepting

A scope with a problem is not opened. Qrypt reports what it found: how many files are missing, unexpected, changed, or cannot be decrypted, or the two generations of a rollback. It cannot name a missing secret, because the name was inside the missing file. The other scopes open normally.

Damage happens without an attack: a disk error, an interrupted synchronization tool. So the user may **accept** the scope as it is now, after an explicit confirmation. Accepting:

- removes the files that cannot be decrypted with the scope's keys, because they hold nothing readable and would make every listing fail;
- signs a new manifest for the files that remain, with a generation higher than both the old manifest's and the record's;
- updates the record. For a scope that was removed, it deletes its record.

Accepting never brings a file back. Whatever was lost stays lost.

**A scope that cannot be opened at all** is the exception. If its scope file is damaged, or was written for another identity, its key cannot be obtained and nothing in it can be read or signed. Qrypt reports it, leaves every file untouched, and does not offer to accept it: the files may be valid for another identity.

## 7. What the files reveal

An attacker who copies the vault learns:

- how many scopes there are, how many secrets each holds, and roughly how long each name and value is (values to the nearest 256 bytes; every name looks the same);
- when each file was last changed, from the file system;
- the scope id and the public-key fingerprint of the member.

They learn nothing about names or values without the scope key, which needs the identity.

## 8. Limits

- **The local record is not protected.** Someone who can write to the user's configuration directory can change or delete the record, and with it hide a rollback. Integrity protects a vault that travels (a mirror, a synchronized folder), not a device that an attacker controls.
- **A first open cannot detect a rollback.** On a device with no record, Qrypt checks that the scope was signed by the identity, but it cannot know whether it is the latest state.
- **A whole vault with its record.** Putting back an older vault together with its older record is not detected.
- **Listing opens every name.** To list secrets or check that a name is free, Qrypt reads every file in the scope and decrypts the 256-byte name segments, not the values. The [threat model](threat-model.md#1-infected-device) already accepts that an infected device can read what the program holds in memory.
- **One damaged file stops its scope** until the user [accepts](#problems-and-accepting) the scope without it. Damage is never hidden and never repaired silently.
- **One writer at a time.** Two programs adding the same name at the same moment can both succeed. Version 1 does not lock the vault.
- **Secrets in memory:** see the limit described in the [threat model](threat-model.md#1-infected-device).

## 9. Libraries

No new dependency. HPKE with X-Wing, HKDF-SHA-256, SHA-256, and the random generator come from the Go standard library; XChaCha20-Poly1305 comes from `golang.org/x/crypto`, already used by the identity.

The implementation is the package `internal/vault`. Its tests rebuild a scope file and a secret file directly from this document.

## Open questions

- **Members:** adding and removing members, and trusting their public keys.
- **Several devices:** merging changes made on two devices. Today a [mirror](mirror.md) carries the state of one device.
