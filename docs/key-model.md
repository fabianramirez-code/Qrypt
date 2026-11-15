# Key Model

How Qrypt creates, protects, and uses keys. Three decisions define it:

| Decision                      | Choice                                                         |
| ----------------------------- | -------------------------------------------------------------- |
| Key type                      | Hybrid: classical + post-quantum                               |
| Private key protection        | Local file encrypted with a passphrase, plus a recovery phrase |
| Encryption for several people | One key per scope                                              |

> **Status:** design only. Nothing here is implemented or audited. Do not store real secrets with Qrypt yet.

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

### Why hybrid

- ML-KEM and ML-DSA became standards in August 2024. If a flaw appears in them or in an implementation, the classical algorithm still protects the data.
- Data pushed to Git is stored indefinitely. Data protected only by classical encryption can be copied today and decrypted once a large quantum computer exists.

### Alternatives considered

| Option            | Why not                                                                     |
| ----------------- | --------------------------------------------------------------------------- |
| Post-quantum only | Simplest, but no fallback if a young algorithm or its implementation fails. |
| Classical only    | Vulnerable to "harvest now, decrypt later".                                 |

## 2. Private key protection

All keys of an identity are derived from **one random seed**.

- The seed is stored in a local identity file, encrypted with a key derived from the user's passphrase with Argon2id. Argon2id makes every passphrase guess slow and expensive.
- The identity file lives in the user's configuration directory. It is never stored in a repository.
- The seed is shown once, at creation, as a **recovery phrase** to write on paper.

There is no server that can reset a forgotten passphrase. Without the recovery phrase, the identity is lost, and with it access to everything encrypted for it.

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

- **Vault format:** file layout in the repository, exact scope boundaries (for example, whether categories get their own key), and how names and other metadata are encrypted.
- **Trust in public keys:** how a client verifies that a public key really belongs to a member. See the [threat model](threat-model.md#3-tampering-with-the-vault-repository).
- **Exact constructions and parameters:** hybrid combiner, key derivation from the seed, AEAD, Argon2id parameters, and recovery phrase encoding. Qrypt will use published, publicly reviewed constructions rather than custom ones.
- **Library choice:** `cloudflare/circl` for everything, or Go's standard library `crypto/mlkem` for ML-KEM.
