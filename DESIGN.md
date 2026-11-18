# Qrypt Design System

Single source of truth for how Qrypt looks and behaves in the terminal. Code in `internal/tui` and `internal/tui/theme` implements it; behavior details live in [docs/tui.md](docs/tui.md) and [docs/settings.md](docs/settings.md). Technical decisions (packages, cryptography, storage, memory protection, delivery) are in [ARCHITECTURE.md](ARCHITECTURE.md).

Scope: Qrypt is a terminal program only. This file does not cover web, app, or desktop design.

> **Status legend.** Rules marked **(implemented)** match the current code. Rules marked **(proposed)** are the target and are not built yet.

## 1. Principles

1. **Secrets first.** Nothing on screen may expose a secret by accident: values are hidden by default, never cut off, and never repeated in messages.
2. **Honest screens.** Nothing on screen may describe data the vault does not hold. Mockups are references for look only.
3. **Content before decoration.** When space runs out, decoration goes first (logo, dividers, bars) and content never does.
4. **Two themes, one identity.** `green` and `carbon`, both built on the same green accent family.
5. **Stable text.** Keep numbered headings and `Name (n)` markers stable; tests and users rely on them.

## 2. Themes

Only two themes exist: **green** (default) and **carbon** (implemented). Both are dark-first; `green` also has a light palette that it uses when the terminal reports a light background. The custom color editor stays, because it edits the roles below. Earlier versions had `violet`, `mono`, `contrast` and `auto`; a saved setting that names one is read as `green`.

### 2.1 Roles

| Role | Use |
| --- | --- |
| Background | Window background |
| Surface | Solid first-screen box, vault panes and modals in every theme (fall back to Background when absent) |
| Raised | Selected row, focused field, modal body (carbon only) |
| Text | Primary content |
| Dim | Secondary content, hints, alternate list rows |
| Muted | Lowest-emphasis content: counts, timestamps, placeholders (carbon only) |
| Accent | Focus, selection marker `>`, headings, logo, borders of the focused panel |
| Success / Warning / Error | State only; never decoration |
| Info / Special | Chips and labels in carbon (for example field kinds, space names) |
| Border | Unfocused panel borders |

### 2.2 `green` (default, dark)

Gentle Shell green. The first screen and vault panes have a solid dark Surface (`#101914`); outside the boxes the terminal background remains visible. The light variant uses a solid white Surface. Info, Special and Muted fall back to Accent, Accent and Dim.

| Role | Hex | Contrast on `#000000` |
| --- | --- | --- |
| Background | `#000000` | n/a |
| Text | `#f5f5f5` | 19.26:1 |
| Dim / Border | `#a3a3a3` | 8.33:1 |
| Accent | `#68b78d` | 8.75:1 |
| Success | `#4ade80` | 12.05:1 |
| Warning | `#facc15` | 13.71:1 |
| Error | `#fb7185` | 7.80:1 |

### 2.3 `carbon`

Two-pane terminal look with layered surfaces and a brighter accent.

| Role | Hex | on Background `#1E1F22` | on Surface `#26282D` |
| --- | --- | --- | --- |
| Background | `#1E1F22` | n/a | n/a |
| Surface | `#26282D` | n/a | n/a |
| Raised | `#2A2D34` | n/a | n/a |
| Text | `#F8F8F2` | 15.46:1 | 13.84:1 |
| Dim / Border | `#a3a3a3` | 6.53:1 | 5.85:1 |
| Muted | `#8b92a5` | 5.30:1 | 4.74:1 |
| Accent / Success | `#50FA7B` | 12.01:1 | 10.75:1 |
| Warning | `#FFB86C` | 9.67:1 | 8.66:1 |
| Error | `#FF5555` | 5.24:1 | 4.69:1 |
| Info | `#8be9fd` | 11.91:1 | 10.66:1 |
| Special | `#bd93f9` | 6.83:1 | 6.11:1 |

### 2.4 Color rules

- **Never use a color alone to carry meaning.** Errors and warnings also carry a word or a marker (`!`, `x`, or a label), so the screen works in `NO_COLOR` and for color-blind users.
- **Contrast floor:** 4.5:1 for text (WCAG 2.2 AA). Carbon Muted on Surface (4.74:1) and Error on Surface (4.69:1) are the lowest accepted values; do not go below them. A theme test fails below 4.5:1 for every text role on every background it is drawn on (implemented).
- **No loose hex values in views.** Views read roles from the theme; a new color is a new role first.
- **Custom colors** replace a role's value only and keep the theme's background. Invalid hex is rejected.

## 3. Typography and icons

A terminal program cannot choose the terminal font. Qrypt can only ask for a good one.

- **Text:** the user's terminal font, monospace. Bold marks headings and the focused item; dim marks secondary text. No italics or underline for meaning.
- **Icons:** one set at a time, chosen on the first screen with `I`:
  - `emoji` (default, so folders show in any terminal),
  - `nerd` (needs a Nerd Font),
  - `unicode` (plain symbols, safest).
- **Nerd Font:** `SymbolsNerdFontMono-Regular.ttf` is bundled in the release zip (pinned SHA-256) with install instructions; it is not embedded in the executable.
- **Every icon has a text fallback** in the `unicode` set, and no information exists only as an icon.
- The first screen offers the icon sets as choices inside its Icons section; I cycles them as a shortcut.

## 4. Layout

### 4.1 Window sizes (implemented)

| Window | Behavior |
| --- | --- |
| Smaller than 60x20, or size unknown | Only a request to enlarge the window. Secrets are never shown cut off. |
| 60x20 up to 99 columns | Narrow flow: one panel, 60 columns wide, growing to at most 64. |
| 100x20 and larger | Wide mode: two solid columns (tree, inspector), status bar, solid modals over the panes. |

### 4.2 Decoration drop order (implemented)

When the window is too short, remove in this order and never remove content:

1. Logo becomes the one-line `◆ Qrypt`.
2. Panel loses its header divider.
3. Bottom key bar.
4. Title line.

In wide mode the creation hints live inside the tree after the destination folder's last row (and New space at the end). They are not selectable; scrolling follows the selected tree row. The bottom key bar drops whole pairs from the end when the line is too narrow.

### 4.3 Spacing

- One-column padding inside panels; one blank row between a heading and its content.
- List rows alternate normal and dim.
- Recovery words and secret values are never clipped; a longer line wraps between words.
- Counts follow the name as `Name (n)`.

## 5. Components

| Component | Rule |
| --- | --- |
| Panel | Bordered, bold centered heading, divider. Focused panel uses an Accent border; others use Border. |
| Selected row | `>` marker plus Raised background (carbon) or bold (green). Only the focused list highlights; unfocused text remains legible. |
| Chip | Short label in Info or Special (carbon) or Accent (green). Text always present. |
| Tree | Every space; all folders and items of the selected space, with `├─ ` / `└─ ` branches and three-cell nesting; `(n)` counts on spaces and folders. Up/Down select one row without writing to disk. |
| Inspector | Field list for the selected item: hidden (masked), visible, note. Reveal is explicit. |
| Modal | Solid background on every cell, Accent border, centered over the two faint panes; creation forms show `Create in: <location>` / `Crear en: <location>`; Esc closes. The help overlay uses this compositor in wide mode. |
| Action row | Wide mode: dim `+ [key] Label` after the last tree row of the creation destination, indented three cells per folder level; New space is the last tree line. Hints are never selectable. |
| Key bar | Bottom line: key and action pairs in the chosen language only (both languages before the choice). In wide mode the pairs are listed in priority order, create keys and `[?] help` first; they are one line, never cut with an ellipsis, and whole pairs are dropped from the end until the line fits beside the item counter. |
| Status bar | Wide mode only: space, item count, mirror and integrity state, lock countdown; a warning if memory locking was refused. |
| Problem screen | Integrity failure. Accepting needs the typed word `ACCEPT` (`ACEPTAR` in Spanish); never a single key. |
| Fields for secrets | Typed, not pasted (except vault fields where a user must paste a token). Masked while typing. |

## 6. Motion

Motion is a signature of Qrypt and is **kept**.

### 6.1 Logo sweep (implemented)

- The six-row, 43-column block-letter logo (`internal/tui/logo.txt`) is drawn in a horizontal gradient from Accent to a lighter tint.
- A wide, near-white band of light sweeps across the logo in a loop. One frame is **66 ms** (`logoFrame`), driven by `logoTickMsg`; the phase advances one step per tick (`logoTint` in `frame.go`).
- The animation is **not key activity**: it never postpones the idle lock.
- The logo appears only on full-size screens; below that it becomes `◆ Qrypt` and the animation stops.

### 6.2 Motion rules

- Motion only on the logo. No spinners, blinking, or sliding on screens that show secrets.
- The sweep must never change the logo's width or the layout; it changes color only.
- With `NO_COLOR` set, Qrypt draws without hue and the logo is bold and static (implemented).
- **(implemented)** A `reduce motion` setting keeps the gradient and stops the sweep. This is the terminal equivalent of `prefers-reduced-motion`.
- **(proposed)** Stop the tick while a modal covers the logo, to save redraws and battery.

## 7. Security-driven UX

These rules are part of the design, not extras.

- Alternate screen only; secrets never reach the terminal scrollback.
- Interactive terminal required; refuse piped input and output.
- Hidden by default; revealing a value is an explicit action.
- **(implemented)** A revealed value hides again 15 seconds after reveal and on lock; a newer reveal restarts the expiry.
- Clipboard copy clears after 30 seconds and is excluded from clipboard history (Windows).
- Idle auto-lock after 5 minutes; lock clears revealed values.
- Messages never repeat what the user typed.
- When memory locking is refused, the unlocked vault stays usable and warns in the wide status bar or on narrow Spaces; locked memory shows no warning.
- Ctrl+C quits at once, except during the identity write (a few milliseconds), so the exit status always matches the disk.

How secrets are protected in memory (keys, decrypted values, the passphrase) is an architecture decision: see [ARCHITECTURE.md](ARCHITECTURE.md), decision D9.

## 8. Accessibility

- Contrast floor of 4.5:1 for text, checked per role (section 2.4).
- Meaning never depends on color or on an icon alone.
- Every action is reachable by keyboard; there is no mouse requirement.
- Focus is always visible: Accent border on the focused section, `>` on every selected row. The first screen centers its logo, four sections and hints inside one solid Surface box, leaving the exterior unpainted.
- Two languages (English and Spanish); only the interface is translated, recovery words are always English.
- `NO_COLOR` and the `reduce motion` setting are respected (implemented; section 6.2).
- **(implemented)** Help overlay on `?` when no text field is active, or F1 anywhere, listing the current screen's keys in the chosen language.

## 9. Voice and copy

- Short, literal, in the chosen language. Technical artifacts (identifiers, files) are English.
- Errors say what happened and what to do next; they never blame the user and never include secret values.
- Spanish uses neutral Spanish with `tú` forms.

## 10. Tooling and verification

| Need | Tool |
| --- | --- |
| Framework, styles | Bubble Tea v2, Lip Gloss v2 |
| Visual review and README demos | VHS scripts under `design/vhs/` (written, not recorded yet) |
| Icons and font | Nerd Fonts `SymbolsNerdFontMono` |
| Contrast check | WCAG 2.2 formula in the theme unit tests (implemented) |

Test conventions (implemented): read content through `plain(m)` (frame stripped) and the frame through `framed(m)`; keep stable text markers and adapt only layout assertions.

Lint, CI, fuzzing, releases and every other engineering tool are in [ARCHITECTURE.md](ARCHITECTURE.md), section 4.

## 11. Open work

Design items only; engineering work is tracked in [ARCHITECTURE.md](ARCHITECTURE.md).

1. Record the VHS demos in `design/vhs/` and use their output in the README.
