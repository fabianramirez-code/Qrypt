# Settings and themes

How Qrypt remembers the interface language and theme, and how a user edits colors. Decided with the user: settings are saved in a file.

> **Status:** implemented. Qrypt asks the terminal for its background color; `green` uses its light palette on a light background and its dark palette otherwise, including when there is no reply.

## What is stored

| Setting  | Values                                                              |
| -------- | ------------------------------------------------------------------- |
| Language | `en` or `es`                                                        |
| Theme    | `green`, `carbon`, or `custom`                                      |
| Colors   | For `custom` only: the hex value of each color role                 |
| `icons`  | `emoji` (default), `unicode`, or `nerd`                                |
| `motion` | `reduced` to stop the logo sweep; absent means animated                |
| `mirror` | Optional folder path as typed on the Mirror screen                  |

The file never holds a secret, a name, a value, a path to the working vault, or anything about the identity. The optional `mirror=<path>` line does reveal where the copy is. It is plain text, so the user can read it and Qrypt can show exactly what it saves.

## Storage

| System  | Default location                                                       |
| ------- | ---------------------------------------------------------------------- |
| Windows | `%AppData%\qrypt\settings`                                             |
| macOS   | `~/Library/Application Support/qrypt/settings`                         |
| Linux   | `$XDG_CONFIG_HOME/qrypt/settings`, or `~/.config/qrypt/settings`       |

- **Written when the user changes something**, not at startup. Starting and quitting without a change writes nothing.
- **Whole or not at all:** a temporary file in the same directory, synced, then renamed over the old one.
- **Owner only:** directory `0700`, file `0600`.
- **Never fatal.** A missing, unreadable, too large (over 4 KiB) or invalid file is ignored: Qrypt starts with the defaults (system language, theme `green`) and shows a one-line notice on the first screen. It never overwrites a file it could not read until the user changes a setting.
- **Format:** one `key=value` per line, UTF-8, for example `language=es`, `theme=custom`, `motion=reduced`, `color.accent=#22d3ee`. Unknown keys are ignored. Invalid values make that one line be ignored. An absent or empty `mirror` means no mirror; stopping mirroring removes the line without deleting the folder.

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
| `green`    | The green of Gentle Shell (`#68b78d` on dark backgrounds, `#1f6b47` on light ones). The default. |
| `carbon`   | Dark only: base background `#1E1F22`, panel surface `#26282D`, green `#50FA7B` and amber `#FFB86C`. It paints its own background; a terminal that ignores the background instruction shows its own background. |

- Every built-in theme keeps text at a contrast ratio of at least 4.5:1 against its background, and a test checks it. `carbon` also meets this ratio against its panel surface.
- `surface` is a theme property for painting panels, not an editable color role. `green` does not paint a surface or set the terminal background.
- `info` (headers, footer keys, inspector title), `special` (chips and value label), and `muted` (secondary text) are theme properties, not editable roles and not stored by `custom`. In `green` and `custom` they follow accent, accent, and dim; `carbon` uses `#8be9fd`, `#bd93f9`, and `#8b92a5`. `raised` is an optional background level (`#2A2D34` in `carbon`) for selections, chips and keys; `green` and `custom` do not paint it.
- Every text color has at least 4.5:1 contrast on Background and Surface. On Raised, only Text, Accent, Info, Special, Success and Warning are drawn, each at least 4.5:1; Error, Dim and Muted are never drawn there. A selected problem row uses Error with an accent bar but no Raised background; a selected row's count uses Text.
- Meaning never depends on color alone: errors and successes also carry words, and the selected row also carries the `>` marker.
- **Removed themes.** Earlier versions also had `violet`, `mono`, `contrast` and `auto`. A settings file that names one of them is read as `green`; the file itself is rewritten only when the user next changes a setting.
- **`NO_COLOR`.** When the environment variable `NO_COLOR` is set to a non-empty value, Qrypt draws without hue (bold, dim and reverse video only), whichever theme is selected, and the logo does not move.

## Motion

Press A on the first screen to turn the logo animation on or off. Reduced motion saves `motion=reduced` only when selected; the default keeps the animated sweep. The static accent-to-light gradient remains. `NO_COLOR` also stops the animation, regardless of this preference.

## Icons

The first screen previews the selected glyphs and cycles sets with I (emoji, nerd, unicode). `emoji` is the default, so spaces show as folders without installing anything; a terminal that draws emoji badly can switch to `unicode`, which uses one-cell symbols available in ordinary fonts. `emoji` needs a terminal and font that render emoji (some occupy two cells). `nerd` needs a Nerd Font with the private-use icon glyphs. Qrypt cannot detect the terminal font; choose the set whose preview looks right. Missing or invalid `icons` defaults to `unicode`.

## Custom colors

The user edits a theme from the first screen. Choosing `Edit colors` copies the current theme into `custom` and lists the seven color roles. A custom theme made from `carbon` keeps only those seven roles: it loses the carbon background and panel surface.

- **Input:** a hex value `#rrggbb`, typed. Qrypt shows a swatch next to it, and the role's sample text in that color.
- **Checks before accepting:** the value is valid hex, and for `text`, `dim`, `accent`, `success`, `error` and `warning` the contrast against the theme background is at least 3:1. A color that fails is rejected with a message, never silently changed.
- **Reset:** Ctrl+R restores the selected role, and Ctrl+A restores the whole theme.
- **Saved as the user leaves the editor**, and only if something changed.

## The first screen

Language and theme are chosen on the same first screen, as today but with a second list.

- LANGUAGE and THEME appear in side-by-side bordered panels. Up and Down move within the focused panel; Left and Right or Tab switch panels. Both selections keep a `>` marker, and only the focused selection is highlighted. Enter continues. Esc quits with status 1.
- The theme preview is live: moving in the theme list recolors the screen at once.
- The saved choice is selected in advance. Without a settings file, the language comes from the system and the theme is `green`.
- Continuing saves the language, theme, icons and motion only if they differ from what is stored.

## What does not change

- The rules for [every screen](tui.md#rules-for-every-screen) stay: alternate screen, interactive terminal, minimum window of 60 by 20, and lines of at most 60 columns.
- Themes change color and borders only. Text, keys and layout are the same in every theme.
- The first screen still shows both languages at once and still appears before an identity exists.

## Open questions

- Importing and exporting a custom theme file.
