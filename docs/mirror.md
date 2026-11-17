# Mirror

How Qrypt keeps a second copy of the vault, and how anyone can add a place to keep it.

> **Status:** design. Not implemented yet. Not audited.

## Why

Qrypt has no server and nobody can reset anything. With one copy of the vault, a failed disk or a stolen device loses every secret at once. The [recovery phrase](identity.md#2-recovery-phrase) rebuilds the identity, which is the keys; it does not bring back what the vault held.

A mirror is a copy of the vault **as it is now**. It is not a history: when a secret changes, the mirror changes with it, and nothing older is kept.

## What a mirror is

- **The working vault stays on the device,** in the [vault directory](vault.md#5-storage). Qrypt reads and writes there, because it needs the guarantees of a local file system.
- **The mirror is a replica somewhere else:** a folder on a USB drive, a folder that another tool synchronizes, or any storage someone writes a backend for.
- **One mirror, optional.** Without one, Qrypt works exactly the same.

## Why a mirror is not a weakness

The mirror holds the same files as the vault, byte for byte: encrypted files and signed manifests. The vault format already assumes that an attacker can read them.

- **Nothing can be read without the identity.** The mirror shows how many scopes and secrets there are, their approximate sizes, and when they changed. It shows no name and no value.
- **The key does not travel with it.** Qrypt never copies the identity file to a mirror. On a new device the identity is rebuilt from the recovery phrase.
- **The storage is not trusted.** Whatever comes back from a mirror is checked against the [signed manifests](vault.md#6-integrity) before Qrypt uses it.

## Updating the mirror

Qrypt updates the mirror after each change to the vault, and when the vault is unlocked.

For each scope, Qrypt first reads the manifest the mirror holds:

| The mirror's manifest is…                                                      | Qrypt                                                                 |
| ------------------------------------------------------------------------------ | --------------------------------------------------------------------- |
| Absent                                                                         | Copies the scope.                                                     |
| Valid, with a lower generation than the device's                               | Updates the scope.                                                    |
| The same manifest as the device's                                              | Copies only the files that are missing or different.                  |
| Valid, with a higher generation, or the same generation and different content  | Does not write. The mirror holds changes this device does not have.   |
| Not valid: not signed by this identity, or damaged                             | Does not write. The folder may belong to another vault.               |

- **Another vault's mirror receives nothing.** If any scope in the mirror has a manifest that this identity did not sign, Qrypt writes nothing at all to that mirror, not even the scopes it lacks.
- **Order of writes:** new and changed files first, then the manifest, then removal of the files that no longer exist. A mirror interrupted halfway does not match its manifest; the next update repairs it.
- **A scope with a [problem](vault.md#problems-and-accepting) is never copied.** The mirror keeps what it had for that scope until the problem is resolved.
- **Removed scopes** are removed from the mirror at the moment they are removed on the device. If the mirror cannot be reached then, the scope stays in the mirror.
- **A scope the device does not have** is left alone in the mirror, and Qrypt says that the mirror holds one. Qrypt cannot tell a scope added by another device from one that was removed while the mirror was out of reach, so it never deletes it on its own.
- **A failed update never blocks the vault.** The change is already saved on the device. Qrypt says that the mirror was not updated and why, and tries again at the next change.
- **Refusing is deliberate.** Overwriting a mirror that is ahead would destroy changes made elsewhere. Merging the changes of two devices is not offered yet.

## Restoring from a mirror

On a device with an empty vault, Qrypt can bring the vault back from a mirror.

- **Checked before anything is written.** Every scope must have a manifest signed by the identity, and files that match it exactly. If one scope fails, nothing is restored and Qrypt reports which.
- **Only into an empty vault.** Qrypt restores when the device has no secret. It never merges into, or replaces, a vault that holds something. The empty scopes of the device, such as the `Personal` scope created on first use, are replaced by the scopes of the mirror.
- **Whole or not at all.** The scopes are first written apart and verified there. Only then do they take their place in the vault.
- **Limit:** Qrypt can prove that the mirror was written by the identity. On a new device it cannot prove that it is the latest state, because the device has no [record](vault.md#local-record) yet.

## Adding a place to keep the mirror

A mirror is anything that implements four operations on whole files, named by a relative path with `/` separators:

| Operation | Meaning                                                                             |
| --------- | ----------------------------------------------------------------------------------- |
| List      | The paths of all files.                                                             |
| Read      | The whole content of one file.                                                      |
| Write     | Stores one file, whole or not at all, creating what its path needs.                 |
| Remove    | Deletes one file. Deleting a file that does not exist is not an error.              |

In Go this is the interface `mirror.Storage`. Qrypt includes one implementation, a directory of the local file system. That already covers a USB drive and any folder synchronized by another tool.

- **What an implementation may assume:** it only ever receives encrypted files and signed manifests. It needs no cryptography of its own.
- **What it must do:** return exactly what was stored, or an error. Qrypt detects anything else and refuses it.
- **What it must not do:** keep older versions on purpose. Storage that keeps versions, such as Git, works against the rule that Qrypt holds only the current state.
- **Network storage is not part of Qrypt's core.** Each kind brings dependencies, credentials and network code, and Qrypt keeps its dependencies few. A backend for object storage, WebDAV or SFTP belongs in a separate module.

## Limits

- **One device writes.** Two devices that change the same vault end with a mirror one of them refuses to update.
- **No server-side protection.** Plain storage cannot refuse a rollback by itself. Only a device with a record detects one.
- **The folder's location is stored** in the [settings file](settings.md), as the path the user typed. It is not a secret, but it says where the copy is.
