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

- **The screen shows both languages and themes at once.** Up and Down move within the focused panel; Left and Right or Tab switch between LANGUAGE and THEME. Each panel retains its `>` selection, while only the focused selection is highlighted. I cycles the icon set (`unicode`, `emoji`, `nerd`); a preview line shows the set name and its glyphs so you can check your terminal font. Enter continues, and Esc quits with status 1. Selecting `Edit colors` opens the custom color editor; Ctrl+R resets one role, Ctrl+A resets all roles, and Esc saves any changes and returns.
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

Run `qrypt --preview` to choose a language and explore a vault with three spaces holding fake sample items (`Personal`, `Work`, and `Work: Google Stitch`, which shows one service with two accounts and one key per tool), without entering a passphrase. Every vault screen marks the session as temporary. The identity and vault live only in a throwaway directory, removed when Qrypt exits; changes are not kept. The Mirror screen is available there too, but accepts only folders inside that temporary directory; a path outside it is refused without reading or writing it. This does not unlock your real identity or vault. The normal `qrypt` command still requires your passphrase. Both commands require an interactive terminal.

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

After unlocking, Qrypt opens the [vault](vault.md#5-storage) next to the identity file, and creates it if it does not exist. The vault is divided into **spaces** (the [scopes](vault.md#scope-of-version-1) of the vault format), such as `Personal` and `Work`. Each space has its own key. The first time, Qrypt creates one space named `Personal`; nothing else is written until the user adds an item or a space. If the vault cannot be opened, the screen says why and Qrypt exits with status 1.

### Spaces

The first screen after unlocking lists the spaces by name, each with the number of items it holds.

| Screen           | Shows                                                   | Keys                                                                                                  |
| ---------------- | ------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| **Spaces**       | The names of the spaces in byte order, one selected.    | Up and Down move, Enter opens the selected space, A adds, R renames, D deletes, M opens Mirror, Esc locks and exits. |
| **Add a space**  | A text field for the name.                              | Enter creates the space and returns to the list with it selected. Esc returns without creating it.    |
| **Rename**       | The current name and a field for the new one.           | Enter renames. Esc returns without changing it.                                                       |
| **Delete space** | The name, and a question to confirm.                    | Y deletes. Any other key cancels.                                                                     |

- **A space is deleted only when it is empty.** If it holds items, the screen says so and deletes nothing. The user deletes or moves nothing by accident.
- **Names follow the item name rules** and are unique among spaces. They are stored encrypted.
- **Inside a space**, the Folder and Item screens work on that space only. Esc from its top-level Folder screen returns to Spaces instead of locking. The title of the Folder screen is the name of the space.
- **Moving an item between spaces** is not offered yet.

### Problems

A problem in one space does not block healthy spaces. The Spaces list keeps their names and item counts; each affected space is a `!` row followed by its name. When its name cannot be read, the row says `! Space <first 8 characters of id>` or `! Espacio <first 8 characters of id>`. Enter opens the single-panel **Problem** screen; in wide mode Right and Tab do so too. The other panes are empty while a problem row is selected. A, R and D do nothing on that row.

| Kind | What the Problem screen says |
| ---- | ---------------------------- |
| Files | Counts of missing, unexpected, changed and undecryptable files (including zero counts). |
| Signature | The manifest signature is invalid; the file counts are shown. |
| Fork | Two different states have the same generation; both generation numbers are shown. |
| Rollback | The current and recorded generations are shown. |
| Removed | The space was removed outside Qrypt; accepting removes its local record. |
| Unreadable | The space cannot be opened with this identity. Qrypt leaves it untouched. There is no confirmation field; only Esc returns. |

For every accept-able problem, the screen explains that accepting removes files that cannot be decrypted, signs the surviving state, and cannot restore anything lost. Type `ACCEPT` in English or `ACEPTAR` in Spanish **exactly**, then Enter, to accept. Other input changes nothing and shows the rejection message. Esc returns without changing the vault; pasting into the field is refused. On success Qrypt reopens all spaces, so a repaired row becomes a normal space and a removed row disappears. A failed acceptance shows the system error without silently accepting it. The Problem screen shows no item names or values.

### Mirror

M on Spaces opens the single-panel **Mirror** screen, showing the chosen folder (or `No mirror` / `Sin espejo`) and the result of the last update. E opens a field to type or paste a folder path; Enter creates the folder if necessary, saves the path as typed in settings, and runs the first update. A file instead of a folder or a folder that cannot be created shows the system error and saves nothing. Esc returns to Spaces. In preview, only a folder within the temporary directory can be chosen; paths outside it are refused without access.

Qrypt synchronously updates the mirror after each successful item or space change, after accepting a problem, and after unlocking. The vault change always succeeds independently: an update failure leaves the device's change intact and shows a warning-colored `Mirror not updated: <reason>` / `Espejo sin actualizar: <motivo>` line. The next change retries. A mirror ahead is not overwritten; a folder belonging to another identity receives nothing. A space with a problem is not copied; a scope held only by the mirror is left there and reported. Deleting a space removes its files from the mirror at deletion time, if available.

X asks for Y to stop mirroring; any other key cancels. Confirming removes the mirror setting, never the folder or its contents. R restores from the selected folder only if the device holds no item; on success Spaces refreshes with the restored names and counts. If it holds an item, the restore is refused without changing anything. An empty, incomplete, or foreign mirror is refused with its own message; other failures show the system error. Restoring never copies the identity file. Ctrl+C and the five-minute idle lock exit normally on this screen.

### Wide mode

At 100 columns by 20 rows or larger, the vault's Spaces, List and Item screens show three unboxed columns: spaces and their item counts on the left, the selected space's item names in the center, and the selected item's inspector on the right. At 100 columns the columns are 25, 43 and 30 cells wide, separated by one vertical line each; extra width is distributed at roughly 25/45/30. Each column has a header and a thin rule beneath it. Carbon paints the columns with Surface; other themes leave their background unpainted. Selected rows have an accent bar and bold text (and Raised background in carbon). Tab and Right move focus from Spaces to List to Item; Tab on Item wraps to Spaces without locking. Shift+Tab and Left move back one pane, doing nothing on Spaces; Esc on Spaces still locks. Enter opens a selected space or item and, on Item, shows or hides its value. Tab on Item does not reveal it, and Right on Item does nothing, because Item is the last pane. Up and Down move within the focused pane. A, C, D, E and R retain their actions on the focused pane. Leaving Item clears its decrypted value from memory. Below the wide threshold, Tab still reveals on the Item screen and Enter does not.

Below the one-line title (or six-row logo from 30 rows), the status bar reads `>_ QRYPT // <active space name>` on the left and `● PQC: ML-KEM-768 + X25519` on the right. The three panes are columns without boxes: each has a one-row heading and a rule under it, one vertical line separates them, and the focused pane is the one whose heading is bright and whose rule carries the accent. The left heading is `[1] SPACES` / `[1] ESPACIOS` with the number of spaces. Flat rows show an open-folder icon for the active space, a closed-folder icon otherwise, the name, and beside it the item count in Muted, so the row still reads `Name (n)`; the selected row has an accent bar at its left. Problems show the problem icon and `! Name` in Error, without a Raised background even when selected. At the bottom, when height permits, a status card shows the mirror state as a chip (off, up to date, or not updated) and a `m:ss` lock countdown with a progress bar that empties over five minutes. The middle heading is `[2]` and the name of the space; its first row is `NAME` / `NOMBRE` with the item count; rows have item icons. For names containing ` / `, the path prefix through the last separator is Muted and the final component uses normal text; the full name remains the row's plain text. The right heading is `[3] ITEM // <item name>` (`[3] ELEMENTO`). Its first card keeps the labeled Space, Cipher (`XChaCha20-Poly1305`), Key (`ML-KEM-768 + X25519`), Signed (`Ed25519 + ML-DSA-65`) and Integrity (`verified, generation <n>` from the selected space, shortened to `✓ gen <n>` when needed) rows, with uncut values even at 100 columns. The Value card has a Special-colored label and a fixed number of spaced dots when hidden; when revealed it shows the value or the existing too-long line. Reveal/Hide and Copy sit side by side with their icons when the pane is wide enough, and one under the other otherwise; Copy is filled with the accent, and Edit sits below in Muted. Unfocused, buttons are dim and the value stays masked. Footer hints for the focused pane use Info-colored key chips (Raised in carbon) with Muted labels; the right shows status, the lock countdown, and applicable clipboard seconds in Warning. Status moves to the second footer row when it cannot fit. The preview banner remains visible. The display uses only real vault data: no window buttons, host or session names, environments or tags, nested folders, fabricated item paths, type chips, modified time, filter box, revision, roles, sync, checksum, tabs, sort, Git state or clock; no cipher per item, created-by metadata, entropy, latency, memory, rotate or revoke. Those data and actions do not exist in the vault; search is a separate task. The preview banner remains visible. In wide mode, add item (name and value), edit value, delete item, add space, rename space, delete space, Problem, and Mirror (main screen, folder field and stop question) appear as accent-bordered modals centered over the faint three panes, with their titles, bodies and key hints inside. The pane belonging to the modal remains focused (items for add/delete item, detail for edit, spaces for space forms, Problem and Mirror). Behind a modal the detail pane always shows a mask, even if its value was revealed before. The footer retains the status message but not the focused pane's hints. When a modal cannot fit the window height, it falls back to the single bordered panel with all content intact. Below 100 by 20, the existing single-column rendering is unchanged. Resizing across the threshold keeps the screen and selected space and item; a revealed value follows the narrow Item screen's existing reveal state and never appears in another pane. Names and counts refresh on a key or resize, not on logo animation ticks.

### Folders and items

A space is browsed one folder at a time. The Spaces screen counts items at any depth. Inside a space, the **Folder** screen shows the space's name and a location line joining the space and each folder with ` / `. Folders appear first, in byte order, with a folder icon and the number of items under them at any depth; items follow in byte order, with an item icon and their titles. Up and Down move the selection; Enter opens a folder or item. Esc goes up one folder, then returns to Spaces from the space's top level.

| Screen | Keys |
| ------ | ---- |
| **Folder** | A adds an item here; F names a folder and opens it; R renames the selected folder or item; D deletes a selected item after Y. D on a folder does nothing: it disappears when its last item is deleted. |
| **Item** | The title and location line precede one row per field. Up and Down select a field; Tab shows or hides the selected hidden field or note; C copies its value; E edits its value; A adds a field; D deletes a field after Y; R renames the title; Esc returns to the folder. |
| **Add item** | Enter a title, then the first field's label, kind and value. Esc cancels or goes back a step. |
| **Add field** | Enter a label, choose H (hidden), V (visible) or N (note); in Spanish choose O (oculto), V (visible) or N (nota); then enter its value. |
| **Edit value** | Enter saves a single-line value; Esc cancels. Tab shows or hides hidden text while typing. |
| **Note editor** | Enter starts a new line; pasted text retains line breaks; Ctrl+S saves; Esc cancels without changing the item. |
| **Delete** | Y confirms an item or field deletion; any other key cancels. |

A hidden field always displays a fixed number of dots regardless of its value. A visible field displays its value; a note displays the word for a note and its number of lines until shown. Leaving and reopening an item hides every field again. Existing items open at the top of their space with one hidden field labeled `value` and are not rewritten until edited. Folder names are stored only in items: an empty new folder is temporary, with a notice explaining that saving an item will keep it. Renaming a folder changes the path of every item under it, including nested folders. Folder and item names may match, but item titles must be unique within their folder and labels within their item. Moving an item or folder elsewhere is not offered yet.

- **Locking.** Esc on Spaces and Ctrl+C anywhere erase the keys and exit with status 0 after opening the vault. The five-minute idle lock applies on every screen.
- **Copying.** C copies only the selected field, with the existing 30-second expiry. No value appears on Spaces or Folder screens.
- **Pasting.** Names, titles, labels and values can be pasted. A line break in a single-line value is refused without losing what was already typed; note paste preserves line breaks.
- **Limits and refusals.** Invalid names, duplicate titles or labels, too many fields, an oversized item, an overlong folder path and a line break in a single-line value each have their own message. Rejection keeps the input and changes no file. A damaged vault file still makes the screen fail with status 1. Messages never repeat a value.

## Copying and locking

- **Copy.** C on the Item screen copies exactly the selected field's value, without showing it. For compatibility, C on a selected item in the Folder screen copies its first field. The screen says that the clipboard will be cleared in 30 seconds.
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
| Item too large                                  | `That item is too large. The limit is 65,788 bytes.` | `Ese elemento es demasiado grande. El límite es 65.788 bytes.` |
| Too many fields | `An item can have at most 64 fields.` | `Un elemento puede tener como máximo 64 campos.` |
| Label already used | `A field with that label already exists.` | `Ya existe un campo con esa etiqueta.` |
| Folder path too long | `That folder path is too long.` | `La ruta de esa carpeta es demasiado larga.` |
| Folder cannot be deleted directly | `A folder disappears when its last item is deleted.` | `Una carpeta desaparece cuando se borra su último elemento.` |
| Newly created folder is empty | `This folder is temporary until you save an item in it.` | `Esta carpeta es temporal hasta que guardes un elemento en ella.` |
| Pasted value with a line break                   | `Values are one line. Remove the line break.`                                  | `Los valores son de una línea. Quita el salto de línea.`                         |
| Vault file damaged, or another identity's vault  | `A vault file is damaged, or the vault belongs to another identity.`           | `Un archivo de la bóveda está dañado, o la bóveda es de otra identidad.`         |
| Value copied                                     | `Copied. The clipboard is cleared in 30 seconds.`                              | `Copiado. El portapapeles se borra en 30 segundos.`                              |
| The clipboard could not be used                  | As the system reports it                                                       | As the system reports it                                                         |
| Space name already used                          | `A space with that name already exists.`                                       | `Ya existe un espacio con ese nombre.`                                           |
| Deleting a space that holds items                | `This space is not empty. Delete its items first.`                             | `Este espacio no está vacío. Borra primero sus elementos.`                       |
| Problem title | `Problem` | `Problema` |
| Unknown space name | `Space <first 8 characters of id>` | `Espacio <first 8 characters of id>` |
| Missing files | `Missing files: <n>` | `Archivos faltantes: <n>` |
| Unexpected files | `Unexpected files: <n>` | `Archivos inesperados: <n>` |
| Changed files | `Changed files: <n>` | `Archivos cambiados: <n>` |
| Undecryptable files | `Undecryptable files: <n>` | `Archivos que no se pueden descifrar: <n>` |
| Invalid manifest signature | `The manifest signature is invalid.` | `La firma del manifiesto no es válida.` |
| Fork | `Two different states claim the same generation.` | `Dos estados distintos tienen la misma generación.` |
| Rollback | `This space was rolled back.` | `Este espacio volvió a un estado anterior.` |
| Current generation | `Current generation: <n>` | `Generación actual: <n>` |
| Recorded generation | `Recorded generation: <n>` | `Generación registrada: <n>` |
| Removed space | `This space was removed outside Qrypt.` | `Este espacio se eliminó fuera de Qrypt.` |
| Removed-space acceptance | `Accepting removes its local record.` | `Aceptar elimina su registro local.` |
| Unreadable space | `This space cannot be opened with this identity. Qrypt leaves it untouched.` | `Este espacio no se puede abrir con esta identidad. Qrypt lo deja intacto.` |
| Acceptance consequence | `Accepting removes files that cannot be decrypted, signs the surviving state, and cannot restore anything lost.` | `Aceptar elimina los archivos que no se pueden descifrar, firma el estado restante y no recupera nada de lo perdido.` |
| Confirmation prompt | `Type ACCEPT to accept:` | `Escribe ACEPTAR para aceptar:` |
| Wrong confirmation | `Type ACCEPT exactly. Nothing was changed.` | `Escribe ACEPTAR exactamente. No se cambió nada.` |
| Paste into confirmation | `Pasting is not accepted. Type the confirmation.` | `No se acepta pegar. Escribe la confirmación.` |
| Problem keys | `Enter: accept  Esc: back` | `Enter: aceptar  Esc: volver` |
| Unreadable problem keys | `Esc: back` | `Esc: volver` |
| Mirror title | `Mirror` | `Espejo` |
| No selected mirror | `No mirror` | `Sin espejo` |
| Mirror edit prompt | `Folder path:` | `Ruta de la carpeta:` |
| Mirror keys | `E: folder  X: stop  R: restore  Esc: back` | `E: carpeta  X: detener  R: restaurar  Esc: volver` |
| Mirror path keys | `Enter: save  Esc: back` | `Enter: guardar  Esc: volver` |
| Mirror stop question | `Stop mirroring? The folder will be left untouched.` | `¿Detener el espejo? La carpeta no se modificará.` |
| Mirror stop keys | `Y: stop  Any other key: cancel` | `Y: detener  Cualquier otra tecla: cancelar` |
| Update result | `Updated: <n>  Unchanged: <n>` | `Actualizados: <n>  Sin cambios: <n>` |
| Mirror ahead | `Mirror ahead: <n> space(s). Nothing overwritten.` | `Espejo adelantado: <n> espacio(s). No se sobrescribió nada.` |
| Foreign mirror | `Folder of another vault: <n> space(s). Nothing written.` | `Carpeta de otra bóveda: <n> espacio(s). No se escribió nada.` |
| Unknown space | `Mirror holds <n> space(s) absent from this device.` | `El espejo tiene <n> espacio(s) ausentes en este equipo.` |
| Problem space | `Not copied: <n> space(s) with a problem.` | `Sin copiar: <n> espacio(s) con problemas.` |
| Failed mirror update | `Mirror not updated: <reason>` | `Espejo sin actualizar: <motivo>` |
| Preview path outside temporary directory | `Preview mirror must stay in the temporary directory.` | `El espejo de prueba debe estar en el directorio temporal.` |
| Restore with an item on device | `Cannot restore: the vault holds a secret.` | `No se puede restaurar: la bóveda contiene un secreto.` |
| Incomplete mirror | `Cannot restore: the mirror is incomplete.` | `No se puede restaurar: el espejo está incompleto.` |
| Foreign mirror | `Cannot restore: the mirror belongs to another identity.` | `No se puede restaurar: el espejo pertenece a otra identidad.` |
| Empty mirror | `Cannot restore: the mirror is empty.` | `No se puede restaurar: el espejo está vacío.` |
| Restored mirror | `Mirror restored.` | `Espejo restaurado.` |
| The folder holds a vault and this device holds no item | `This folder already holds a vault. Press R to restore it.` | `Esta carpeta ya contiene una bóveda. Pulsa R para restaurarla.` |
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
