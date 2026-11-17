# Terminal Interface

What the `qrypt` program shows and how it behaves. It has four flows: creating an identity, unlocking it, restoring it from its recovery phrase, and managing the stored items once it is unlocked.

> **Status:** early stage. Items are stored in a [vault](vault.md) that is not audited yet. Do not store real passwords with Qrypt yet.

## Rules for every screen

- **Alternate screen only.** Qrypt draws on the terminal's alternate screen, which disappears when the program exits. A passphrase or a recovery phrase never stays in the terminal's scrollback.
- **Interactive terminal required.** If standard input or standard output is not a terminal, Qrypt prints one line to standard error, exits with status 1, and does nothing else. Secrets can never be piped into a file or into another program.
- **Nothing else receives secrets.** No logs, no clipboard, and nothing on standard output or standard error after the program exits. The only exception is copying a value to the clipboard on the user's request: see [Copying and locking](#copying-and-locking).
- **Ctrl+C quits** at once, on every screen except while the identity file is being written (the Saving step of creating and of restoring). That write takes an instant and is never interrupted, so the exit status always matches what is on disk.
- **Small windows.** If the window is smaller than 60 columns by 20 rows, or its size is not known yet, Qrypt shows only a request to enlarge it. Secrets are never shown cut off.
- **Two languages.** The interface is in English or in Spanish. See [Language](#language).
- **Typed, not pasted.** Fields for a passphrase or a recovery word ignore pasted text. Pasting would mean that a secret had been copied to the clipboard. The fields of the [vault screens](#managing-the-vault) are the exception, because a user must be able to paste a token they were given.
- **Messages never repeat what the user typed.**

## Language

Qrypt offers language and theme on the first screen every time it starts.

- **The screen shows both languages and themes at once.** Up and Down move within the focused panel; Left and Right or Tab switch between LANGUAGE and THEME. Each panel retains its `>` selection, while only the focused selection is highlighted. Enter continues, and Esc quits with status 1. Selecting `Edit colors` opens the custom color editor; Ctrl+R resets one role, Ctrl+A resets all roles, and Esc saves any changes and returns.
- **The system language is selected in advance:** Spanish if the system language is Spanish, English otherwise. On Windows Qrypt reads the user's preferred display language. On other systems it reads `LC_ALL`, `LC_MESSAGES` and `LANG`, in that order.
- **The choice is remembered** in a [settings file](settings.md), written only when the user changes it. The saved language is selected in advance. A settings file that cannot be read is ignored. The same screen also chooses the theme; see [Settings and themes](settings.md). Starting and quitting without a change writes nothing, so quitting before an identity is saved still leaves no identity file and no vault.
- **Only the interface is translated.** The passphrase and the recovery phrase are always English words, in both languages. An identity therefore has one format, and its words can be typed on any keyboard. The Spanish Welcome screen says that the words are in English.
- **Before the choice, both languages.** The first screen, its key bar and the request to enlarge the window while no language is chosen are bilingual, as are the lines printed when Qrypt cannot start. After the choice, the bottom bar uses the chosen language only.

## Look

Every full-size screen has the six-row Qrypt ASCII logo in a horizontal accent-to-light gradient (`mono` uses bold without a gradient). A soft band of light sweeps across the logo in a loop; the animation does not count as key activity, so it never postpones the idle lock, and `mono` stays static. Each screen also has a bordered panel with a centered bold heading and a divider, and key hints beneath the panel. The first screen has two side-by-side panels with a clear accent border on the focused one; selected rows keep their `>` marker. List rows alternate normal and dim. Moving through themes previews borders, headings and logo live. When the window is too short, decoration is removed in this order and content never is: the logo becomes the one-line `◆ Qrypt`, then the panel loses its header divider, then the bottom bar, then the title line. The panel is 60 columns wide in a 60-column window and grows to at most 64, so that every line of text fits without wrapping; at 60 columns a longer line wraps between words. Recovery words are never clipped. `green` is the default. See [Settings and themes](settings.md) for saved choices and custom colors.

## Starting

Qrypt first asks for the [language](#language). Then:

| Situation                                                         | What Qrypt does                          |
| ----------------------------------------------------------------- | ---------------------------------------- |
| No identity file at the [default location](identity.md#5-storage) | Starts the flow "Creating an identity".  |
| An identity file already exists                                   | Starts the flow "Unlocking an identity". |

Both flows offer a way into "Restoring an identity".

## Preview mode

Run `qrypt --preview` to choose a language and explore a vault containing four fake sample items without entering a passphrase. Every vault screen marks the session as temporary. The identity and vault live only in a throwaway directory, removed when Qrypt exits; changes are not kept. This does not unlock your real identity or vault. The normal `qrypt` command still requires your passphrase. Both commands require an interactive terminal.

## Creating an identity

Nothing is written to disk until step 6. Quitting before that leaves no file and no directory.

| Step | Screen                                                                                                                                   | The user does            | Then                                                                                                                                                      |
| ---- | ---------------------------------------------------------------------------------------------------------------------------------------- | ------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | **Welcome.** Explains the two secrets, and that both are shown once and cannot be reset by anyone.                                       | Enter                    | Qrypt generates the identity and its passphrase, which takes a moment, then goes to step 2. R starts "Restoring an identity". Esc quits.                  |
| 2    | **Passphrase.** The 7 words, numbered, with the instruction to write them on paper in order.                                             | Enter                    | Step 3.                                                                                                                                                   |
| 3    | **Confirm passphrase.** A text field. What the user types is visible.                                                                    | Types the 7 words, Enter | Qrypt opens the encrypted identity with what was typed. If it opens, step 4. If not, the screen says why and the user tries again. Esc returns to step 2. |
| 4    | **Recovery phrase.** The 24 words, numbered, in 3 columns, with the instruction to write them on paper and keep it away from the device. | Enter                    | Step 5.                                                                                                                                                   |
| 5    | **Confirm recovery phrase.** Asks for 3 of the 24 words, one at a time, by position. The positions are random and all different.         | Types each word, Enter   | A wrong word says so and asks again. Esc returns to step 4. After the third correct word, step 6.                                                         |
| 6    | **Saving.**                                                                                                                              |                          | Qrypt writes the identity file. Keys are ignored until it finishes. On success, step 7. On failure, step 8.                                               |
| 7    | **Done.** Says that the identity is ready and where the file is. Shows no secret.                                                        | Any key                  | Exits with status 0.                                                                                                                                      |
| 8    | **Not saved.** Says why, that nothing was saved, and that the words written down belong to no identity and must be discarded.            | Any key                  | Exits with status 1.                                                                                                                                      |

Quitting before step 7 exits with status 1, because no identity was created.

"Shown once" means during this flow. Until the identity is saved, the user can return to a secret screen with Esc. Afterwards Qrypt never shows the passphrase or the recovery phrase again.

On the Not saved screen, Qrypt translates the cause when it knows it, such as an identity file that already exists. Any other cause is shown as the system reports it.

### Why the confirmations

- **Passphrase:** typing it back proves that the paper copy opens the identity, before the user depends on it. The check uses the real encrypted identity, not a comparison of text.
- **Recovery phrase:** asking for 3 words proves that it was written down, without making the user type all 24.
- **Typed, not pasted:** pasting would prove nothing about the paper copy.

## Unlocking an identity

Unlocking writes nothing to disk and never changes the identity file.

| Step | Screen                                                                                                | The user does            | Then                                                                                                                                                                                                                                                              |
| ---- | ----------------------------------------------------------------------------------------------------- | ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | **Unlock.** A field for the passphrase, hidden as it is typed, and a count of the words typed so far. | Types the 7 words, Enter | Qrypt opens the identity file with what was typed, which takes a moment. If it opens, step 2. If the passphrase is rejected, the screen says why and the user tries again. If the file cannot be used, step 3. Ctrl+R starts "Restoring an identity". Esc quits. |
| 2    | **Vault.** The list of [managing the vault](#managing-the-vault). If the identity has no vault yet, Qrypt creates it first. |                          | Quitting from there exits with status 0.                                                                                                                                                                                                                          |
| 3    | **Cannot open.** Says why and where the identity file is.                                             | Any key                  | Exits with status 1.                                                                                                                                                                                                                                              |

Quitting at step 1, or while the passphrase is being checked, exits with status 1.

- **Hidden by default.** Every character typed, spaces included, is shown as `*`. Tab shows what was typed, and Tab again hides it.
- **Word count.** The screen says how many words have been typed, so the user can keep their place while the text is hidden. It reveals nothing else.
- **The field keeps what was typed** after a rejected attempt, so a typing mistake can be found with Tab and corrected.
- **No limit on attempts.** Every attempt already costs one Argon2id computation, and an attacker who has the identity file does not need Qrypt to guess.

## Restoring an identity

Restoring rebuilds an identity from its recovery phrase and gives it a new passphrase. It serves two cases: a new device with no identity file, and a forgotten passphrase.

Nothing is written to disk until step 5. Quitting before that changes nothing.

| Step | Screen                                                                                                                                                                                            | The user does            | Then                                                                                                                                                                                                                     |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1    | **Restore.** Explains that the 24-word recovery phrase is needed and that the identity gets a new passphrase. When an identity file exists, it also says that the file will be kept as a backup. | Enter                    | Step 2. Esc returns to the screen the user came from.                                                                                                                                                                    |
| 2    | **Recovery phrase, word by word.** Asks for word 1 to word 24, one at a time. The field is hidden as it is typed. Tab shows or hides it.                                                          | Types each word, Enter   | A word that is not in the word list is rejected at once and asked again. Esc goes back one word, and at word 1 it returns to step 1. After word 24 Qrypt checks the whole phrase. If it is valid, step 3. If not, the screen says so and starts again at word 1. |
| 3    | **New passphrase.** Qrypt rebuilds the identity and generates a new passphrase, which takes a moment. Then it shows the 7 words, numbered, as when creating an identity.                          | Enter                    | Step 4.                                                                                                                                                                                                                  |
| 4    | **Confirm passphrase.** As when creating an identity.                                                                                                                                             | Types the 7 words, Enter | If it opens the identity, step 5. If not, the screen says why and the user tries again. Esc returns to step 3.                                                                                                           |
| 5    | **Saving.**                                                                                                                                                                                       |                          | Qrypt writes the identity file. If one already existed, it is first kept as a backup. Keys are ignored until it finishes. On success, step 6. On failure, step 7.                                                       |
| 6    | **Restored.** Says that the identity is restored, where the file is, where the backup is if there is one, and that the previous passphrase no longer opens Qrypt on this device. Shows no secret. | Any key                  | Exits with status 0.                                                                                                                                                                                                     |
| 7    | **Not restored.** Says why, that nothing was changed, that the new passphrase is not valid and must be discarded, and that the recovery phrase is still valid.                                    | Any key                  | Exits with status 1.                                                                                                                                                                                                     |

Quitting before step 6 exits with status 1.

- **The same identity.** A restored identity has the same keys as before. Only the passphrase is new.
- **The previous file is always kept.** Qrypt cannot tell whether the recovery phrase belongs to the identity file it replaces, because that file cannot be opened. The backup is described under [Storage](identity.md#5-storage).
- **No recovery phrase is shown.** The user already has it, so this flow never displays it.
- **Each word is checked as it is entered.** A typing mistake is caught at that word, not after all 24.

## Managing the vault

After unlocking, Qrypt opens the [vault](vault.md#4-storage) next to the identity file, and creates it if it does not exist. The first time, this writes the vault's scope file; nothing else is written until the user adds an item. If the vault cannot be opened, the screen says why and Qrypt exits with status 1.

| Screen     | Shows                                                                                      | Keys                                                                                        |
| ---------- | ------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------- |
| **List**   | The names of all items in byte order, one selected, or a line saying there are none yet.    | Up and Down move, Enter opens the selected item, A adds, D deletes, Esc locks and exits.    |
| **Item**   | The name and the value, hidden as `*` until revealed.                                      | Tab shows or hides the value, E edits it, Esc returns to the list.                          |
| **Add**    | Two steps: the name, then the value, each in a text field.                                 | Enter accepts a step, Esc goes back one step and, at the name, returns to the list.         |
| **Edit**   | The name and a field for the new value.                                                    | Enter saves, Esc returns to the item without changing it.                                   |
| **Delete** | The name, and a question to confirm.                                                       | Y deletes. Any other key cancels.                                                           |

- **Locking.** Esc on the list, and Ctrl+C anywhere, erase the keys from memory and exit with status 0 once the vault was opened. Writes to the vault finish before the next key is read, and each file is written whole or not at all, so quitting never leaves a half-written file.
- **Values are hidden by default** and shown only with Tab on the Item screen. Leaving the screen hides them again. The value field of Add and Edit is hidden as it is typed, and Tab shows or hides it.
- **Names are visible** in the list and in the name field. A name too long for one row is cut with `…` in the list; the Item screen shows it whole.
- **A long value is not cut off.** If a value has more than 300 characters, the Item screen says that it is too long to show instead of showing part of it.
- **Pasting is accepted** in the name and value fields. A pasted value with a line break is rejected, because values typed here are one line. Values of several lines need a later import feature.
- **Limits are checked when the user presses Enter.** A name must follow the [vault rules](vault.md#name) and a value may be at most 64 KiB. Rejected input leaves the vault unchanged and keeps what was typed.
- **Deleting needs a Y.** Deleting is permanent: the file is removed and Qrypt keeps no copy.
- **A damaged vault file** makes the list fail, as [documented](vault.md#6-limits). The screen says so and Qrypt exits with status 1.
- **Messages never repeat a value.**

## Copying and locking

- **Copy.** C copies the value of the open item, or of the selected item on the list, without showing it. The screen says that the clipboard will be cleared in 30 seconds.
- **Cleared on time.** Qrypt clears the clipboard 30 seconds after the last copy, when the vault locks, and when the program quits. Where the system lets Qrypt read the clipboard, it clears it only if it still holds the copied value, so something the user copied afterwards is never erased. A newer copy restarts the 30 seconds, and an older timer never clears a newer copy.
- **Excluded from history.** On Windows, Qrypt marks the copy so that Windows clipboard history and cloud clipboard sync skip it. Elsewhere no such marking exists in Qrypt yet.
- **Terminals without a native clipboard.** On systems where Qrypt has no native clipboard code, it asks the terminal to set the clipboard (OSC 52). Terminals that do not support it ignore the request, and Qrypt cannot tell. It cannot read the clipboard either, so at expiry it clears the clipboard without checking what it holds.
- **Limit.** Clearing does not reach copies kept by clipboard managers or other programs, as the [threat model](threat-model.md) says.
- **Automatic lock.** After 5 minutes without a key press while the vault is open, Qrypt locks: it erases the keys, clears the clipboard, and exits with status 0. The alternate screen disappears with it, so no name or value stays visible.

## Messages

### Vault

| Cause                                            | English                                                                        | Spanish                                                                          |
| ------------------------------------------------ | ------------------------------------------------------------------------------ | -------------------------------------------------------------------------------- |
| Name empty, too long, or with control characters | `That name is not valid. Use 1 to 255 characters, with no control characters.` | `Ese nombre no es válido. Usa de 1 a 255 caracteres, sin caracteres de control.` |
| Name already used                                | `An item with that name already exists.`                                       | `Ya existe un elemento con ese nombre.`                                          |
| Value too large                                  | `That value is too large. The limit is 64 KiB.`                                | `Ese valor es demasiado grande. El límite es 64 KiB.`                            |
| Pasted value with a line break                   | `Values are one line. Remove the line break.`                                  | `Los valores son de una línea. Quita el salto de línea.`                         |
| Vault file damaged, or another identity's vault  | `A vault file is damaged, or the vault belongs to another identity.`           | `Un archivo de la bóveda está dañado, o la bóveda es de otra identidad.`         |
| Value copied                                     | `Copied. The clipboard is cleared in 30 seconds.`                              | `Copiado. El portapapeles se borra en 30 segundos.`                              |
| The clipboard could not be used                  | As the system reports it                                                       | As the system reports it                                                         |
| Anything else                                    | As the system reports it                                                       | As the system reports it                                                         |

### Identity

#### When a passphrase or a recovery word is rejected

| Cause                                                           | English                                                    | Spanish                                                       |
| --------------------------------------------------------------- | ---------------------------------------------------------- | ------------------------------------------------------------- |
| Not 7 words                                                     | `Type exactly 7 words.`                                    | `Escribe exactamente 7 palabras.`                             |
| A passphrase word is not in the list                            | `One of the words is not valid. Check the spelling.`       | `Una de las palabras no es válida. Revisa cómo está escrita.` |
| Creating or restoring: seven valid words that do not open the identity | `That is not the passphrase shown. Check your paper.`      | `Esa no es la frase de contraseña mostrada. Revisa tu papel.` |
| Creating: wrong recovery word                                   | `That is not word <n>. Check your paper.`                  | `Esa no es la palabra <n>. Revisa tu papel.`                  |
| Unlocking: seven valid words that do not open the identity file | `Wrong passphrase, or the identity file is damaged.`       | `Frase incorrecta, o el archivo de identidad está dañado.`    |
| Restoring: a word is not in the list                            | `That is not a valid word. Check the spelling.`            | `Esa no es una palabra válida. Revisa cómo está escrita.`     |
| Restoring: the 24 words are not a valid recovery phrase         | `Not a valid recovery phrase. Type the 24 words again.`    | `Frase de recuperación no válida. Escríbela de nuevo.`        |
| Pasted text                                                     | `Pasting is not accepted. Type the words from your paper.` | `No se acepta pegar. Escribe las palabras de tu papel.`       |

When unlocking, Qrypt cannot tell a wrong passphrase from a damaged file, so one message covers both.

#### On the Cannot open screen

| Cause                                             | English                                                  | Spanish                                                        |
| ------------------------------------------------- | -------------------------------------------------------- | -------------------------------------------------------------- |
| The file is not an identity file                  | `The identity file is not a valid Qrypt identity.`       | `El archivo de identidad no es una identidad válida de Qrypt.` |
| The file has a format version Qrypt does not know | `This identity was created by a newer version of Qrypt.` | `Esta identidad es de una versión más nueva de Qrypt.`         |
| Anything else, such as a file that cannot be read | As the system reports it                                 | As the system reports it                                       |
