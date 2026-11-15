# Terminal Interface

What the `qrypt` program shows and how it behaves. So far it has two flows: creating an identity and unlocking it.

> **Status:** early stage. After unlocking there is nothing to do yet, because storing secrets is not implemented. Restoring an identity from its recovery phrase is not implemented either.

## Rules for every screen

- **Alternate screen only.** Qrypt draws on the terminal's alternate screen, which disappears when the program exits. A passphrase or a recovery phrase never stays in the terminal's scrollback.
- **Interactive terminal required.** If standard input or standard output is not a terminal, Qrypt prints one line to standard error, exits with status 1, and does nothing else. Secrets can never be piped into a file or into another program.
- **Nothing else receives secrets.** No logs, no clipboard, and nothing on standard output or standard error after the program exits.
- **Ctrl+C quits** at once, on every screen except while the identity file is being written (step 6 of creating an identity). That write takes an instant and is never interrupted, so the exit status always matches what is on disk.
- **Small windows.** If the window is smaller than 60 columns by 20 rows, or its size is not known yet, Qrypt shows only a request to enlarge it. Secrets are never shown cut off.
- **Two languages.** The interface is in English or in Spanish. See [Language](#language).
- **Typed, not pasted.** Fields for a passphrase or a recovery word ignore pasted text. Pasting would mean that a secret had been copied to the clipboard.
- **Messages never repeat what the user typed.**

## Language

Qrypt asks for the language every time it starts. It is the first screen.

- **The screen shows both languages at once.** Up and Down move, Enter selects, and Esc quits with status 1.
- **The system language is selected in advance:** Spanish if the system language is Spanish, English otherwise. On Windows Qrypt reads the user's preferred display language. On other systems it reads `LC_ALL`, `LC_MESSAGES` and `LANG`, in that order.
- **The choice is not remembered.** Remembering it needs a settings file, and Qrypt writes nothing to disk before an identity is saved.
- **Only the interface is translated.** The passphrase and the recovery phrase are always English words, in both languages. An identity therefore has one format, and its words can be typed on any keyboard. The Spanish Welcome screen says that the words are in English.
- **Before the choice, both languages.** The language screen, the request to enlarge the window while no language is chosen, and the lines printed when Qrypt cannot start are in English and in Spanish.

## Starting

Qrypt first asks for the [language](#language). Then:

| Situation                                                         | What Qrypt does                               |
| ----------------------------------------------------------------- | --------------------------------------------- |
| No identity file at the [default location](identity.md#5-storage) | Starts the flow "Creating an identity".       |
| An identity file already exists                                   | Starts the flow "Unlocking an identity".      |

## Creating an identity

Nothing is written to disk until step 6. Quitting before that leaves no file and no directory.

| Step | Screen                                                                                                                                   | The user does            | Then                                                                                                                                                      |
| ---- | ---------------------------------------------------------------------------------------------------------------------------------------- | ------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | **Welcome.** Explains the two secrets, and that both are shown once and cannot be reset by anyone.                                       | Enter                    | Qrypt generates the identity and its passphrase, which takes a moment, then goes to step 2. Esc quits.                                                    |
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

| Step | Screen                                                                                                    | The user does            | Then                                                                                                                                                                                                                 |
| ---- | --------------------------------------------------------------------------------------------------------- | ------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | **Unlock.** A field for the passphrase, hidden as it is typed, and a count of the words typed so far.     | Types the 7 words, Enter | Qrypt opens the identity file with what was typed, which takes a moment. If it opens, step 2. If the passphrase is rejected, the screen says why and the user tries again. If the file cannot be used, step 3. Esc quits. |
| 2    | **Unlocked.** Says that the passphrase is correct and that storing secrets is not available yet.          | Any key                  | Exits with status 0.                                                                                                                                                                                                 |
| 3    | **Cannot open.** Says why and where the identity file is.                                                 | Any key                  | Exits with status 1.                                                                                                                                                                                                 |

Quitting at step 1, or while the passphrase is being checked, exits with status 1.

- **Hidden by default.** Every character typed, spaces included, is shown as `*`. Tab shows what was typed, and Tab again hides it.
- **Word count.** The screen says how many words have been typed, so the user can keep their place while the text is hidden. It reveals nothing else.
- **The field keeps what was typed** after a rejected attempt, so a typing mistake can be found with Tab and corrected.
- **No limit on attempts.** Every attempt already costs one Argon2id computation, and an attacker who has the identity file does not need Qrypt to guess.

## Messages

### When a passphrase or a recovery word is rejected

| Cause                                                             | English                                                    | Spanish                                                      |
| ----------------------------------------------------------------- | ---------------------------------------------------------- | ------------------------------------------------------------ |
| Not 7 words                                                       | `Type exactly 7 words.`                                    | `Escribe exactamente 7 palabras.`                            |
| A word is not in the list                                         | `One of the words is not valid. Check the spelling.`       | `Una de las palabras no es válida. Revisa cómo está escrita.` |
| Creating: seven valid words that do not open the identity         | `That is not the passphrase shown. Check your paper.`      | `Esa no es la frase de contraseña mostrada. Revisa tu papel.` |
| Creating: wrong recovery word                                     | `That is not word <n>. Check your paper.`                  | `Esa no es la palabra <n>. Revisa tu papel.`                 |
| Unlocking: seven valid words that do not open the identity file   | `Wrong passphrase, or the identity file is damaged.`       | `Frase incorrecta, o el archivo de identidad está dañado.`   |
| Pasted text                                                       | `Pasting is not accepted. Type the words from your paper.` | `No se acepta pegar. Escribe las palabras de tu papel.`      |

When unlocking, Qrypt cannot tell a wrong passphrase from a damaged file, so one message covers both.

### On the Cannot open screen

| Cause                                         | English                                                  | Spanish                                                       |
| --------------------------------------------- | -------------------------------------------------------- | ------------------------------------------------------------- |
| The file is not an identity file              | `The identity file is not a valid Qrypt identity.`       | `El archivo de identidad no es una identidad válida de Qrypt.` |
| The file has a format version Qrypt does not know | `This identity was created by a newer version of Qrypt.` | `Esta identidad es de una versión más nueva de Qrypt.`        |
| Anything else, such as a file that cannot be read | As the system reports it                                 | As the system reports it                                      |
