package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

var actionSizes = [][2]int{{120, 30}, {100, 20}}

func resized(t *testing.T, m Model, w, h int) Model {
	t.Helper()
	return send(t, m, tea.WindowSizeMsg{Width: w, Height: h})
}

// paneBounds returns the first column of the center and right panes.
func paneBounds(width int) (center, right int) {
	extra := width - 100
	left := 25 + extra*25/100
	mid := 43 + extra*45/100
	return left + 1, left + mid + 2
}

// paneOf says in which pane (0 left, 1 center, 2 right) the text is drawn, or -1.
func paneOf(m Model, text string) int {
	center, right := paneBounds(m.width)
	for _, line := range strings.Split(plain(m), "\n") {
		if at := strings.Index(line, text); at >= 0 {
			col := lipgloss.Width(line[:at])
			switch {
			case col >= right:
				return 2
			case col >= center:
				return 1
			}
			return 0
		}
	}
	return -1
}

func actionTexts(lang Language) (space, item, folder, field string) {
	t := lang.text()
	return "+ [A] " + t.wideNewSpace, "+ [A] " + t.wideNewItem, "+ [F] " + t.wideNewFolder, "[A] " + t.wideAddField
}

func TestActionRowsAndInspectorButtonText(t *testing.T) {
	want := map[Language][4]string{
		English: {"+ [A] New space", "+ [A] New item", "+ [F] New folder", "[A] Add field"},
		Spanish: {"+ [A] Nuevo espacio", "+ [A] Nuevo elemento", "+ [F] Nueva carpeta", "[A] Añadir campo"},
	}
	for _, lang := range []Language{English, Spanish} {
		for _, size := range actionSizes {
			t.Run(fmt.Sprintf("%d-%dx%d", lang, size[0], size[1]), func(t *testing.T) {
				m := resized(t, wideFixture(t, lang), size[0], size[1])
				w := want[lang]
				for i, pane := range []int{0, 1, 1} {
					if got := paneOf(m, w[i]); got != pane {
						t.Fatalf("%q in pane %d, want %d\n%s", w[i], got, pane, plain(m))
					}
				}
				if got := paneOf(m, w[3]); got != -1 {
					t.Fatalf("add-field button on the Spaces screen: %s", plain(m))
				}
				for _, state := range []screen{vaultList, vaultItem} {
					m = enter(t, m)
					if state == vaultItem {
						m = enter(t, m)
					}
					for i, pane := range []int{0, 1, 1, 2} {
						if got := paneOf(m, w[i]); got != pane {
							t.Fatalf("screen %d: %q in pane %d, want %d\n%s", state, w[i], got, pane, plain(m))
						}
					}
				}
			})
		}
	}
}

func TestActionRowsFollowTheLastEntryIndentedLikeThePreview(t *testing.T) {
	m := resized(t, wideFixture(t, Spanish), 120, 30)
	m = enter(t, m)
	lines := strings.Split(plain(m), "\n")
	for i, line := range lines {
		if strings.Contains(line, "account") && !strings.Contains(line, "ELEMENTO") {
			// Separator, one cell of padding, then two cells of indent: the plus aligns with the entry icons.
			if !strings.Contains(lines[i+1], "│   + [A] Nuevo elemento") {
				t.Fatalf("first row does not follow the entry or is not indented by two cells: %q %q", line, lines[i+1])
			}
			if !strings.Contains(lines[i+2], "│   + [F] Nueva carpeta") {
				t.Fatalf("second row does not follow the first: %q", lines[i+2])
			}
			return
		}
	}
	t.Fatal("no entry row")
}

func TestActionRowsAbsentWhenTheirKeyWouldNotWork(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		for _, size := range actionSizes {
			t.Run(fmt.Sprintf("%d-%dx%d", lang, size[0], size[1]), func(t *testing.T) {
				space, item, folder, field := actionTexts(lang)
				// A problem selected: A does nothing, and the other panes are empty.
				m, _, _ := problemFixture(t, lang, false, func(t *testing.T, dir string) {
					t.Helper()
					if err := os.WriteFile(filepath.Join(dir, "secrets", "0123456789abcdef0123456789abcdef.qrs"), []byte("broken"), 0600); err != nil {
						t.Fatal(err)
					}
				})
				m = resized(t, m, size[0], size[1])
				if !strings.Contains(plain(m), space) {
					t.Fatalf("healthy space selected must offer %q:\n%s", space, plain(m))
				}
				m = key(t, m, tea.KeyDown)
				if m.selectedProblem() == nil {
					t.Fatal("setup: problem not selected")
				}
				for _, text := range []string{space, item, folder, field} {
					if strings.Contains(plain(m), text) {
						t.Fatalf("%q shown while a problem is selected:\n%s", text, plain(m))
					}
				}
				before := m.screen
				if m = key(t, m, 'a'); m.screen != before {
					t.Fatal("A acted on a problem row")
				}

				// A folder in the inspector: nothing to add a field to.
				m = spacesSession(t, lang)
				if _, err := m.vlt.Add(vault.Item{Path: []string{"dir"}, Title: "nested", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "secret", Value: []byte("fake")}}}); err != nil {
					t.Fatal(err)
				}
				m.refreshSpaces()
				m = resized(t, m, size[0], size[1])
				m = enter(t, m)
				if !strings.Contains(plain(m), item) || !strings.Contains(plain(m), folder) || strings.Contains(plain(m), field) {
					t.Fatalf("folder selected: rows must stay, add-field must not:\n%s", plain(m))
				}
			})
		}
	}
}

// Pressing the advertised key in each pane opens the matching flow.
func TestActionKeysDoWhatTheyAdvertise(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		for _, size := range actionSizes {
			t.Run(fmt.Sprintf("%d-%dx%d", lang, size[0], size[1]), func(t *testing.T) {
				tx := lang.text()
				m := resized(t, wideFixture(t, lang), size[0], size[1])
				expect := func(m Model, s screen, title string) {
					t.Helper()
					if m.screen != s || !strings.Contains(framed(m), title) {
						t.Fatalf("screen %d, want %d with %q:\n%s", m.screen, s, title, framed(m))
					}
				}
				expect(key(t, m, 'a'), spacesAdd, tx.spaceAddTitle)

				list := enter(t, m)
				expect(key(t, list, 'a'), vaultAddName, tx.addTitle)
				expect(key(t, list, 'f'), folderName, tx.folderAddTitle)

				item := enter(t, list)
				if item.screen != vaultItem {
					t.Fatal("setup: item not open")
				}
				expect(key(t, item, 'a'), fieldLabel, tx.fieldAddTitle)
				deleting := key(t, item, 'd')
				expect(deleting, fieldDelete, tx.fieldDeleteTitle)
				if deleting = key(t, deleting, 'y'); len(deleting.itemData.Fields) != 0 {
					t.Fatal("D did not delete the chosen field")
				}
				// R renames the item, not a field.
				renaming := key(t, item, 'r')
				expect(renaming, vaultRename, tx.renameTitle)
				renaming = enter(t, typeText(t, renaming, "renamed-item"))
				if renaming.item != "renamed-item" || len(renaming.itemData.Fields) != len(item.itemData.Fields) || renaming.itemData.Fields[0].Label != item.itemData.Fields[0].Label {
					t.Fatalf("R did not rename the item: %q", renaming.item)
				}
			})
		}
	}
}

func TestCursorNeverRestsOnAnActionRow(t *testing.T) {
	// A selection marker replaces the bar cell, so it would read "▌ + [".
	onRow := func(m Model) bool {
		return strings.Contains(framed(m), "▌ + [") || strings.Contains(framed(m), "▌+ [")
	}
	for _, lang := range []Language{English, Spanish} {
		for _, size := range actionSizes {
			m := resized(t, wideFixture(t, lang), size[0], size[1])
			for i := 0; i < 5; i++ {
				m = key(t, m, tea.KeyDown)
			}
			cursor := m.treeCursor
			if cursor != len(m.treeRows)-1 || key(t, m, tea.KeyDown).treeCursor != cursor || onRow(m) {
				t.Fatalf("tree cursor %d of %d", cursor, len(m.treeRows))
			}
			m = enter(t, m)
			m = addVaultItem(t, m, "second", "fake")
			for i := 0; i < 5; i++ {
				m = key(t, m, tea.KeyDown)
			}
			last := len(m.folderRows) - 1
			if m.cursor != last || key(t, m, tea.KeyDown).cursor != last || onRow(m) {
				t.Fatalf("cursor %d, last %d: %s", m.cursor, last, framed(m))
			}
			// One marker in the tree and one on the entry; none on an action row.
			if got := strings.Count(framed(m), "▌"); got != 2 {
				t.Fatalf("selection markers: %d: %s", got, framed(m))
			}
		}
	}
}

func addRoot(t *testing.T, m Model, n int) Model {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := m.vlt.Add(vault.Item{Title: fmt.Sprintf("entry-%02d", i), Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "k", Value: []byte("fake")}}}); err != nil {
			t.Fatal(err)
		}
	}
	m.refreshSpaces()
	m.refresh()
	return m
}

// At 100x20 the center pane has a header and 13 further rows.
func TestActionRowsAreDroppedFirstAndNeverDisplaceContent(t *testing.T) {
	cases := []struct {
		entries            int
		wantItem, wantFold bool
	}{{11, true, true}, {12, true, false}, {13, false, false}, {30, false, false}}
	for _, lang := range []Language{English, Spanish} {
		_, item, folder, _ := actionTexts(lang)
		for _, c := range cases {
			m := addRoot(t, spacesSession(t, lang), c.entries)
			m = resized(t, m, 100, 20)
			m = enter(t, m)
			view := plain(m)
			if strings.Contains(view, item) != c.wantItem || strings.Contains(view, folder) != c.wantFold {
				t.Fatalf("%d entries: item row %v folder row %v\n%s", c.entries, strings.Contains(view, item), strings.Contains(view, folder), view)
			}
			shown := 0
			for i := 0; i < c.entries; i++ {
				if strings.Contains(view, fmt.Sprintf("entry-%02d", i)) {
					shown++
				}
			}
			if shown != min(c.entries, 13) {
				t.Fatalf("%d entries: %d shown", c.entries, shown)
			}
			if !strings.Contains(view, "entry-00") {
				t.Fatalf("first entry displaced:\n%s", view)
			}
			wideFit(t, m, 100, 20)
			// Scrolled to the end, the last entry is shown and no row follows it.
			for i := 0; i < c.entries; i++ {
				m = key(t, m, tea.KeyDown)
			}
			if c.entries >= 13 && (!strings.Contains(plain(m), fmt.Sprintf("entry-%02d", c.entries-1)) || strings.Contains(plain(m), item)) {
				t.Fatalf("scrolled list:\n%s", plain(m))
			}
		}
	}
}

func TestSpaceRowDroppedWhenTheTreeHasNoRoom(t *testing.T) {
	m := spacesSession(t, English)
	for i := 0; i < 10; i++ { // Personal plus ten: eleven rows, one free in 100x20.
		m = key(t, m, 'a')
		m = typeText(t, m, fmt.Sprintf("Space %02d", i))
		m = enter(t, m)
	}
	m = resized(t, m, 100, 20)
	if !strings.Contains(plain(m), "+ [A] New space") {
		t.Fatalf("one free row:\n%s", plain(m))
	}
	m = key(t, m, 'a')
	m = typeText(t, m, "Space 10")
	m = enter(t, m)
	if strings.Contains(plain(m), "+ [A] New space") || !strings.Contains(plain(m), "Space 10") {
		t.Fatalf("no free row:\n%s", plain(m))
	}
}

func TestEmptyFolderKeepsItsNoticeBeforeTheRows(t *testing.T) {
	m := resized(t, spacesSession(t, English), 120, 30)
	m = enter(t, m)
	m = key(t, m, 'f')
	m = typeText(t, m, "empty")
	m = enter(t, m)
	lines := strings.Split(plain(m), "\n")
	for i, line := range lines {
		if strings.Contains(line, english.folderTemporary[:20]) {
			if !strings.Contains(lines[i+1], "+ [A] New item") || !strings.Contains(lines[i+2], "+ [F] New folder") {
				t.Fatalf("rows do not follow the notice:\n%s", plain(m))
			}
			return
		}
	}
	t.Fatalf("notice missing:\n%s", plain(m))
}

func barLine(t *testing.T, m Model) string {
	t.Helper()
	lines := strings.Split(framed(m), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "[?]") {
			return lines[i]
		}
	}
	t.Fatalf("no bar:\n%s", framed(m))
	return ""
}

func TestBarNamesKeysAndDropsWholeHintsFromTheEnd(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		tx := lang.text()
		panes := []struct {
			state     screen
			full      string
			mandatory int
		}{{spacesList, tx.wideSpacesHint, 2}, {vaultList, tx.wideListHint, 3}, {vaultItem, tx.wideItemHint, 2}}
		for _, pane := range panes {
			hints := strings.Split(pane.full, "  ")
			for _, width := range []int{100, 101, 120, 170} {
				m := resized(t, wideFixture(t, lang), width, 30)
				m.screen = pane.state
				line := barLine(t, m)
				if strings.Contains(line, "…") || lipgloss.Width(line) > width {
					t.Fatalf("%d %d: bar cut or too wide: %q", pane.state, width, line)
				}
				for _, h := range hints[:pane.mandatory] {
					if !strings.Contains(line, h) {
						t.Fatalf("%d %d: %q missing from %q", pane.state, width, h, line)
					}
				}
				// Whole hints only, always a prefix of the priority order.
				shown := 0
				for shown < len(hints) && strings.Contains(line, hints[shown]) {
					shown++
				}
				for _, h := range hints[shown:] {
					if strings.Contains(line, h) {
						t.Fatalf("%d %d: hint %q kept after a dropped one: %q", pane.state, width, h, line)
					}
				}
				if width == 170 && shown != len(hints) {
					t.Fatalf("%d: hints dropped at 170: %q", pane.state, line)
				}
				// The status counter is still on the same line.
				if !strings.Contains(line, tx.wideCount(len(m.folderRows))) {
					t.Fatalf("%d %d: counter lost: %q", pane.state, width, line)
				}
			}
		}
	}
}

func TestBarDropsAtTheThresholdAreStable(t *testing.T) {
	m := resized(t, wideFixture(t, English), 100, 20)
	if line := barLine(t, m); strings.Contains(line, "[↑↓] move") || !strings.Contains(line, "[Esc] lock") {
		t.Fatalf("spaces bar at 100: %q", line)
	}
}

func TestHelpListsEveryWideKeyOfEachPane(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		tx := lang.text()
		for _, size := range actionSizes {
			for state, full := range map[screen]string{spacesList: tx.wideSpacesHint, vaultList: tx.wideListHint, vaultItem: tx.wideItemHint} {
				m := resized(t, wideFixture(t, lang), size[0], size[1])
				m.screen = state
				m = key(t, m, '?')
				var pages strings.Builder
				for i := 0; i < 6; i++ {
					pages.WriteString(framed(m) + "\n")
					m = key(t, m, tea.KeyDown)
				}
				for _, hint := range strings.Split(full, "  ") {
					if !strings.Contains(pages.String(), hint) {
						t.Fatalf("%d %dx%d: help lacks %q", state, size[0], size[1], hint)
					}
				}
			}
		}
	}
}

func TestActionRowColorsFocusedUnfocusedAndPlain(t *testing.T) {
	segment := func(m Model, text string) string {
		for _, line := range strings.Split(m.View().Content, "\n") {
			stripped := ansi.ReplaceAllString(line, "")
			if !strings.Contains(stripped, text) {
				continue
			}
			cut := line[:strings.Index(line, text[len(text)-4:])+4]
			if at := strings.LastIndex(cut, "│"); at >= 0 {
				cut = cut[at:]
			}
			return cut
		}
		t.Fatalf("no row %q:\n%s", text, framed(m))
		return ""
	}
	for _, theme := range []string{"green", "carbon"} {
		m := wideFixture(t, English)
		m.themeName = theme
		m.refreshStyles()
		m = resized(t, m, 120, 30)
		info := fmt.Sprintf("38;2;%d;%d;%dm", m.styles.palette.Info.R, m.styles.palette.Info.G, m.styles.palette.Info.B)
		if !strings.Contains(segment(m, "+ [A] New space"), info) {
			t.Fatalf("%s: focused key has no key color", theme)
		}
		if strings.Contains(segment(m, "+ [A] New item"), info) || strings.Contains(segment(m, "+ [F] New folder"), info) {
			t.Fatalf("%s: unfocused row uses the key color", theme)
		}
		m = enter(t, m)
		if strings.Contains(segment(m, "+ [A] New space"), info) || !strings.Contains(segment(m, "+ [A] New item"), info) || !strings.Contains(segment(m, "+ [F] New folder"), info) {
			t.Fatalf("%s: focus did not move the key color", theme)
		}
	}
	m := wideFixture(t, English)
	m.plain = true
	m.refreshStyles()
	color := regexp.MustCompile(`\x1b\[(?:[0-9;]*;)?(?:3[0-8]|4[0-8]|9[0-7]|10[0-7])[;m]`)
	for _, size := range actionSizes {
		m = resized(t, m, size[0], size[1])
		for _, state := range []screen{spacesList, vaultList, vaultItem} {
			m.screen = state
			if color.MatchString(m.View().Content) {
				t.Fatalf("plain look emits color at %v screen %d: %q", size, state, color.FindString(m.View().Content))
			}
		}
	}
	if !strings.Contains(plain(m), "+ [A] New") {
		t.Fatal("plain look lost the rows")
	}
}

func TestNarrowModeHasNoActionRowsAndSpecificWording(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		for _, size := range [][2]int{{60, 20}, {80, 24}} {
			m := resized(t, wideFixture(t, lang), size[0], size[1])
			_, item, folder, field := actionTexts(lang)
			space, _, _, _ := actionTexts(lang)
			tx := lang.text()
			for _, state := range []screen{spacesList, vaultList, vaultItem} {
				m.screen = state
				if state == vaultItem {
					m = enter(t, resized(t, m, size[0], size[1]))
					m.screen = vaultItem
				}
				view := framed(m)
				for _, text := range []string{space, item, folder, field, "+ [A]", "+ [F]", tx.wideAddField} {
					if strings.Contains(view, text) {
						t.Fatalf("%dx%d screen %d shows %q", size[0], size[1], state, text)
					}
				}
				wideFit(t, m, size[0], size[1])
			}
			m.screen = spacesList
			want := map[Language][2]string{English: {"A: space", "A: field"}, Spanish: {"A espacio", "A campo"}}[lang]
			if !strings.Contains(framed(m), want[0]) {
				t.Fatalf("spaces hint: %s", framed(m))
			}
			m.screen = vaultItem
			if !strings.Contains(framed(m), want[1]) {
				t.Fatalf("fields hint: %s", framed(m))
			}
		}
	}
}
