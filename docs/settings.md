# Settings and themes

How Qrypt remembers the interface language and theme, and how a user edits colors. Decided with the user: settings are saved in a file.

> **Status:** design only. Not implemented yet.

## What is stored

| Setting  | Values                                                              |
| -------- | ------------------------------------------------------------------- |
| Language | `en` or `es`                                                        |
| Theme    | `cyan`, `violet`, `mono`, `contrast`, `auto`, or `custom`           |
| Colors   | For `custom` only: the hex value of each color role                 |

The file never holds a secret, a name, a value, a path to the vault, or anything about the identity. It is plain text, so the user can read it and Qrypt can show exactly what it saves.

## Storage

| System  | Default location                                                       |
| ------- | ---------------------------------------------------------------------- |
| Windows | `%AppData%\qrypt\settings`                                             |
| macOS   | `~/Library/Application Support/qrypt/settings`                         |
| Linux   | `$XDG_CONFIG_HOME/qrypt/settings`, or `~/.config/qrypt/settings`       |

- **Written when the user changes something**, not at startup. Starting and quitting without a change writes nothing.
- **Whole or not at all:** a temporary file in the same directory, synced, then renamed over the old one.
- **Owner only:** directory `0700`, file `0600`.
- **Never fatal.** A missing, unreadable, too large (over 4 KiB) or invalid file is ignored: Qrypt starts with the defaults (system language, theme `cyan`) and shows a one-line notice on the first screen. It never overwrites a file it could not read until the user changes a setting.
- **Format:** one `key=value` per line, UTF-8, for example `language=es`, `theme=custom`, `color.accent=#22d3ee`. Unknown keys are ignored. Invalid values make that one line be ignored.

## Themes

A theme gives a color to each role. The interface uses roles, never raw colors.

| Role      | Used for                                           |
| --------- | -------------------------------------------------- |
| `accent`  | Titles, the selected row, the logo                 |
| `text`    | Normal text                                        |
| `dim`     | Hints and secondary text                           |
| `success` | Confirmations                                      |
| `error`   | Rejected input and failures                        |
| `warning` | Reminders such as the clipboard countdown          |
| `border`  | Panels and the box around secret words             |

| Theme      | Look                                                                                  |
| ---------- | ------------------------------------------------------------------------------------- |
| `cyan`     | Cyan accent. The default.                                                             |
| `violet`   | Violet accent.                                                                        |
| `mono`     | No hue: bold, dim and reverse video only. Sober, and the safest on odd terminals.     |
| `contrast` | White on black with saturated colors, and thick borders. Meets WCAG AAA for text.     |
| `auto`     | Follows the terminal's light or dark background and picks readable colors for each.   |

- Every built-in theme keeps text at a contrast ratio of at least 4.5:1 against its background, and a test checks it.
- Meaning never depends on color alone: errors and successes also carry words, and the selected row also carries the `>` marker.
- When the terminal has no color, Qrypt behaves as `mono`.

## Custom colors

The user edits a theme from the first screen. Choosing `Edit colors` copies the current theme into `custom` and lists the roles.

- **Input:** a hex value `#rrggbb`, typed. Qrypt shows a swatch next to it, and the role's sample text in that color.
- **Checks before accepting:** the value is valid hex, and for `text`, `dim`, `accent`, `success`, `error` and `warning` the contrast against the theme background is at least 3:1. A color that fails is rejected with a message, never silently changed.
- **Reset:** a key restores the role, and another restores the whole theme.
- **Saved as the user leaves the editor**, and only if something changed.

## The first screen

Language and theme are chosen on the same first screen, as today but with a second list.

- Up and Down move within a list. Tab switches between the language list and the theme list. Enter continues. Esc quits with status 1.
- The theme preview is live: moving in the theme list recolors the screen at once.
- The saved choice is selected in advance. Without a settings file, the language comes from the system and the theme is `cyan`.
- Continuing saves the language and theme only if they differ from what is stored.

## What does not change

- The rules for [every screen](tui.md#rules-for-every-screen) stay: alternate screen, interactive terminal, minimum window of 60 by 20, and lines of at most 60 columns.
- Themes change color and borders only. Text, keys and layout are the same in every theme.
- The first screen still shows both languages at once and still appears before an identity exists.

## Open questions

- Whether `auto` can read the terminal background on every terminal Qrypt supports. Where it cannot, it falls back to `cyan` on a dark background.
- Importing and exporting a custom theme file.
