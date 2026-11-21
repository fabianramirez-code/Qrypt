# Key Model

How Qrypt creates, protects, and uses keys. Three decisions define it:

| Decision                      | Choice                                                         |
| ----------------------------- | -------------------------------------------------------------- |
| Key type                      | Hybrid: classical + post-quantum                               |
| Private key protection        | Local file encrypted with a passphrase, plus a recovery phrase |
| Encryption for several people | One key per scope                                              |

> **Status:** the keys, the hybrid algorithms and the one-member space are implemented (see [Vault](vault.md)); wrapping a key for other people is design only. Not audited. Do not store real secrets with Qrypt yet.

## Design principles

Qrypt's goal is one safe place for personal and work secrets that anyone can use. Three rules follow from it:

- **One protection level.** Every secret uses the same algorithms and parameters. A personal password or a `DEV` key is protected exactly like a `PRO` API key. There is no lighter mode.
- **Same strength, separate keys.** Keys are split by scope to decide who can read what, not to protect some secrets more than others.
- **Safe defaults.** The user never chooses algorithms or parameters.

## 1. Key type: hybrid

Each identity has two key pairs:

| Purpose                   | Classical | Post-quantum          |
| ------------------------- | --------- | --------------------- |
| Receive encrypted secrets | X25519    | ML-KEM-768 (FIPS 203) |
| Sign changes              | Ed25519   | ML-DSA-65 (FIPS 204)  |

An attacker must break both the classical and the post-quantum algorithm to read a secret or forge a signature.

The encryption pair is X-Wing, a published combination of X25519 and ML-KEM-768.

### Why hybrid

- ML-KEM and ML-DSA became standards in August 2024. If a flaw appears in them or in an implementation, the classical algorithm still protects the data.
- Encrypted data that leaves the device (a mirror in a synchronized folder, a backup) can be kept indefinitely by whoever copies it. Data protected only by classical encryption can be copied today and decrypted once a large quantum computer exists.

### Alternatives considered

| Option            | Why not                                                                     |
| ----------------- | --------------------------------------------------------------------------- |
| Post-quantum only | Simplest, but no fallback if a young algorithm or its implementation fails. |
| Classical only    | Vulnerable to "harvest now, decrypt later".                                 |

## 2. Private key protection

All keys of an identity are derived from **one random seed**.

- The seed is stored in a local identity file, encrypted with a key derived from a passphrase with Argon2id. Argon2id makes every passphrase guess slow and expensive.
- Qrypt generates the passphrase: 7 random words. The user does not choose it, because people choose weak passphrases.
- The identity file lives in the user's configuration directory. It is never stored in a repository.
- The seed is shown once, at creation, as a **recovery phrase** to write on paper.

There is no server that can reset a forgotten passphrase. Without the recovery phrase, the identity is lost, and with it access to everything encrypted for it.

The exact constructions and the file format are specified in [Identity](identity.md).

### Alternatives considered

| Option         | Why not now                                                                                                                                                                      |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| OS keychain    | Convenient, but it behaves differently on each system, is often missing on servers without a desktop, and is hard to back up or move. It may be added later as an optional convenience. |
| Hardware token | Potentially the strongest option, but ML-KEM and ML-DSA support in current tokens is unconfirmed.                                                                                |

## 3. Encryption for several people: scope keys

A **scope** is the unit of access, for example the `PRO` environment of a workspace.

- Each scope has its own random symmetric key.
- The scope key is wrapped (encrypted) once for each member, using that member's hybrid public key.
- Secrets in the scope are encrypted with a symmetric AEAD under the scope key.

Adding a member is one operation: wrap the scope key for the new member.

### Removing a member

Removing a member does not take back what they already saw. Two steps are always required:

1. Rotate the scope key and re-encrypt the scope.
2. Change the real secrets the removed member had access to, because they may have kept a copy.

### Alternatives considered

| Option                                  | Why not                                                                                                                                                       |
| --------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| One master password for the whole vault | Everyone shares one password, so access cannot be removed for one person.                                                                                      |
| One envelope per secret                 | Fine-grained, but it adds more than 1 KB per member per secret (an ML-KEM-768 ciphertext is 1,088 bytes), and adding or removing a member rewrites every file. |

## Open questions

These are not decided yet:

- **Trust in public keys:** how a client verifies that a public key really belongs to a member. See the [threat model](threat-model.md#3-tampering-with-the-vault-repository).
- **Constructions for several members:** the vault constructions for one member are fixed in [Vault](vault.md), including how the two signatures are combined. Wrapping a scope key for other people and signing the member list are not decided yet.
