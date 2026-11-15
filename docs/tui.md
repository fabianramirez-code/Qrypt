# Terminal Interface

What the `qrypt` program shows and how it behaves. So far it has one flow: creating an identity.

> **Status:** early stage. Unlocking an existing identity, and everything after it, is not implemented yet.

## Rules for every screen

- **Alternate screen only.** Qrypt draws on the terminal's alternate screen, which disappears when the program exits. A passphrase or a recovery phrase never stays in the terminal's scrollback.
- **Interactive terminal required.** If standard input or standard output is not a terminal, Qrypt prints one line to standard error, exits with status 1, and does nothing else. Secrets can never be piped into a file or into another program.
- **Nothing else receives secrets.** No logs, no clipboard, and nothing on standard output or standard error after the program exits.
- **Ctrl+C quits** at once, on every screen except while the identity file is being written (step 6). That write takes an instant and is never interrupted, so the exit status always matches what is on disk.
- **Small windows.** If the window is smaller than 60 columns by 20 rows, or its size is not known yet, Qrypt shows only a request to enlarge it. Secrets are never shown cut off.

## Starting

| Situation                                                         | What Qrypt does                                                                                    |
| ----------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| No identity file at the [default location](identity.md#5-storage) | Starts the flow "Creating an identity".                                                             |
| An identity file already exists                                   | Shows where it is and says that unlocking is not available yet. Generates nothing. Any key exits. |

## Creating an identity

Nothing is written to disk until step 6. Quitting before that leaves no file and no directory.

| Step | Screen                                                                                                                                 | The user does           | Then                                                                                                                                                             |
| ---- | -------------------------------------------------------------------------------------------------------------------------------------- | ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | **Welcome.** Explains the two secrets, and that both are shown once and cannot be reset by anyone.                                     | Enter                   | Qrypt generates the identity and its passphrase, which takes a moment, then goes to step 2. Esc quits.                                                             |
| 2    | **Passphrase.** The 7 words, numbered, with the instruction to write them on paper in order.                                           | Enter                   | Step 3.                                                                                                                                                          |
| 3    | **Confirm passphrase.** A text field. What the user types is visible.                                                                  | Types the 7 words, Enter | Qrypt opens the encrypted identity with what was typed. If it opens, step 4. If not, the screen says why and the user tries again. Esc returns to step 2.          |
| 4    | **Recovery phrase.** The 24 words, numbered, in 3 columns, with the instruction to write them on paper and keep it away from the device. | Enter                   | Step 5.                                                                                                                                                          |
| 5    | **Confirm recovery phrase.** Asks for 3 of the 24 words, one at a time, by position. The positions are random and all different.       | Types each word, Enter  | A wrong word says so and asks again. Esc returns to step 4. After the third correct word, step 6.                                                                 |
| 6    | **Saving.**                                                                                                                            |                         | Qrypt writes the identity file. Keys are ignored until it finishes. On success, step 7. On failure, step 8.                                                                                          |
| 7    | **Done.** Says that the identity is ready and where the file is. Shows no secret.                                                      | Any key                 | Exits with status 0.                                                                                                                                             |
| 8    | **Not saved.** Says why, that nothing was saved, and that the words written down belong to no identity and must be discarded.          | Any key                 | Exits with status 1.                                                                                                                                             |

Quitting before step 7 exits with status 1, because no identity was created.

"Shown once" means during this flow. Until the identity is saved, the user can return to a secret screen with Esc. Afterwards Qrypt never shows the passphrase or the recovery phrase again.

### Why the confirmations

- **Passphrase:** typing it back proves that the paper copy opens the identity, before the user depends on it. The check uses the real encrypted identity, not a comparison of text.
- **Recovery phrase:** asking for 3 words proves that it was written down, without making the user type all 24.
- **Typed, not pasted:** the confirmation fields ignore pasted text. Pasting would prove nothing about the paper copy, and it would mean that a secret had been copied to the clipboard.

### Messages when a confirmation fails

| Cause                                           | Message                                               |
| ----------------------------------------------- | ----------------------------------------------------- |
| Not 7 words                                     | `Type exactly 7 words.`                               |
| A word is not in the list                       | `One of the words is not valid. Check the spelling.`  |
| Seven valid words that do not open the identity | `That is not the passphrase shown. Check your paper.` |
| Wrong recovery word                             | `That is not word <n>. Check your paper.`             |
| Pasted text                                     | `Pasting is not accepted. Type the words from your paper.` |

A message never repeats what the user typed.
