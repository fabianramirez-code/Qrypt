package tui

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

func TestWideAndModalOpaqueWithDestination(t *testing.T) {
	m := spacesSession(t, Spanish)
	if _, err := m.vlt.Add(vault.Item{Path: []string{"Google", "Proyecto 2"}, Title: "cuenta", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "Secreto", Value: []byte("sensible")}}}); err != nil {
		t.Fatal(err)
	}
	m.refreshSpaces()
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if !strings.Contains(m.View().Content, "48;2;") {
		t.Fatal("tree has no solid background")
	}
	m = enter(t, m)
	m = key(t, m, tea.KeyDown)
	m = key(t, m, tea.KeyDown)
	m = key(t, m, 'a')
	if m.screen != vaultAddName || !strings.Contains(framed(m), "Crear en: Personal / Google / Proyecto 2") {
		t.Fatalf("modal destination: %s", framed(m))
	}
	if !strings.Contains(m.View().Content, "48;2;") {
		t.Fatal("modal has no solid background")
	}
}

func TestTwoPaneTreeShowsNestedItemAndFullLocation(t *testing.T) {
	m := spacesSession(t, Spanish)
	if _, err := m.vlt.Add(vault.Item{Path: []string{"Google", "Proyecto 2"}, Title: "cuenta", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "Secreto", Value: []byte("sensible")}}}); err != nil {
		t.Fatal(err)
	}
	m.refreshSpaces()
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = enter(t, m)
	m = key(t, m, tea.KeyDown)
	m = key(t, m, tea.KeyDown)
	m = key(t, m, tea.KeyDown)
	m = enter(t, m)
	if m.screen != vaultItem {
		t.Fatalf("not in detail: %d", m.screen)
	}
	view := framed(m)
	if strings.Count(view, "┼") != 1 || !strings.Contains(view, "Google") || !strings.Contains(view, "Proyecto 2") || !strings.Contains(view, "cuenta") || !strings.Contains(view, "Personal / Google / Proyecto 2 / cuenta") || strings.Contains(view, "sensible") {
		t.Fatalf("tree/detail: %s", view)
	}
	wideFit(t, m, 100, 30)
}

func TestTwoPaneTreeRowsNavigationDoesNotWrite(t *testing.T) {
	m := spacesSession(t, English)
	if _, err := m.vlt.Add(vault.Item{Path: []string{"outer", "inner"}, Title: "entry", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "value", Value: []byte("secret-never-on-tree")}}}); err != nil {
		t.Fatal(err)
	}
	m.refreshSpaces()
	m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	before := spaceSnapshot(t, vaultDir(m.path))
	for _, want := range []string{"outer (1)", "inner (1)", "[2] ITEM // Personal / outer / inner / entry"} {
		m = key(t, m, tea.KeyDown)
		if !strings.Contains(framed(m), want) || strings.Contains(framed(m), "secret-never-on-tree") || !reflect.DeepEqual(before, spaceSnapshot(t, vaultDir(m.path))) {
			t.Fatalf("selection %q changed disk or preview: %s", want, framed(m))
		}
	}
	for i := 0; i < 3; i++ {
		m = key(t, m, tea.KeyUp)
	}
	if m.screen != spacesList || !reflect.DeepEqual(before, spaceSnapshot(t, vaultDir(m.path))) {
		t.Fatal("up changed disk or lost space selection")
	}
}

func TestTwoPaneCreateInsideSelectedFolder(t *testing.T) {
	for _, create := range []rune{'a', 'f'} {
		m := spacesSession(t, English)
		if _, err := m.vlt.Add(vault.Item{Path: []string{"parent"}, Title: "existing", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "value", Value: []byte("fake")}}}); err != nil {
			t.Fatal(err)
		}
		m.refreshSpaces()
		m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
		m = key(t, m, tea.KeyDown) // select folder without entering it
		m = key(t, m, create)
		if !strings.Contains(framed(m), "Create in: Personal / parent") {
			t.Fatalf("wrong destination: %s", framed(m))
		}
		m = typeText(t, m, "child")
		m = enter(t, m)
		if create == 'a' {
			m = typeText(t, m, "value")
			m = enter(t, m)
			m = key(t, m, 'h')
			m = typeText(t, m, "fake-child")
			m = enter(t, m)
		} else {
			// A temporary folder is not stored until its first item is saved.
			m = addVaultItem(t, m, "new", "fake-child")
		}
		found := false
		for _, item := range diskItems(t, m) {
			if item.Title == "child" && reflect.DeepEqual(item.Path, []string{"parent"}) || item.Title == "new" && reflect.DeepEqual(item.Path, []string{"parent", "child"}) {
				found = true
			}
		}
		if !found {
			t.Fatalf("%c did not save inside selected folder", create)
		}
	}
}

func TestWideCreatingModalNamesDestinationInBothLanguages(t *testing.T) {
	for _, tc := range []struct {
		language Language
		line     string
	}{{English, "Create in: Personal / parent"}, {Spanish, "Crear en: Personal / parent"}} {
		m := spacesSession(t, tc.language)
		if _, err := m.vlt.Add(vault.Item{Path: []string{"parent"}, Title: "existing", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "value", Value: []byte("fake")}}}); err != nil {
			t.Fatal(err)
		}
		m.refreshSpaces()
		m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
		m = key(t, m, tea.KeyDown)
		for _, keyCode := range []rune{'a', 'f'} {
			x := key(t, m, keyCode)
			if !strings.Contains(framed(x), tc.line) {
				t.Fatalf("%c: %s", keyCode, framed(x))
			}
		}
	}
}

// Read rendered SGR state cell by cell: the modal must cover the entire rectangle,
// including its padding and border, not just the text glyphs.
func TestWideModalRectangleIsOpaque(t *testing.T) {
	escape := regexp.MustCompile(`\x1b\[([0-9;]*)m`)
	for _, theme := range []string{"green", "carbon"} {
		m := send(t, spacesSession(t, English), tea.WindowSizeMsg{Width: 120, Height: 30})
		m.themeName = theme
		m.refreshStyles()
		m = key(t, m, 'a')
		rows := strings.Split(m.View().Content, "\n")
		plainRows := strings.Split(framed(m), "\n")
		for y, line := range plainRows {
			start := strings.Index(line, "╭"+strings.Repeat("─", 62)+"╮")
			if start < 0 {
				continue
			}
			x := lipgloss.Width(line[:start])
			for row := y; row < len(rows); row++ {
				background, col := false, 0
				for len(rows[row]) > 0 {
					seq := escape.FindStringIndex(rows[row])
					if seq != nil && seq[0] == 0 {
						params := strings.TrimSuffix(strings.TrimPrefix(rows[row][:seq[1]], "\x1b["), "m")
						parts := strings.Split(params, ";")
						for i, part := range parts {
							if part == "0" || part == "49" || part == "" {
								background = false
							}
							if part == "48" && i+1 < len(parts) && parts[i+1] == "2" {
								background = true
							}
						}
						rows[row] = rows[row][seq[1]:]
						continue
					}
					r, width := utf8.DecodeRuneInString(rows[row])
					cells := lipgloss.Width(string(r))
					if cells > 0 && col >= x && col < x+64 && !background {
						t.Fatalf("%s modal cell (%d,%d) transparent, row %q", theme, col, row, strings.Split(m.View().Content, "\n")[row])
					}
					col += cells
					rows[row] = rows[row][width:]
				}
				if row > y && strings.Contains(plainRows[row], "╰"+strings.Repeat("─", 62)+"╯") {
					break
				}
			}
			break
		}
		if !strings.Contains(framed(m), "╭"+strings.Repeat("─", 62)+"╮") {
			t.Fatal("modal rectangle not found")
		}
	}
}

func TestWideInspectorKeyExchangeUncutAtHundredColumns(t *testing.T) {
	for _, language := range []Language{English, Spanish} {
		m := send(t, spacesSession(t, language), tea.WindowSizeMsg{Width: 100, Height: 30})
		if _, err := m.vlt.Add(vault.Item{Title: "item", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "secret", Value: []byte("fake")}}}); err != nil {
			t.Fatal(err)
		}
		m.refreshSpaces()
		m = enter(t, m)
		m = enter(t, m)
		label := "Key exchange"
		if language == Spanish {
			label = "Intercambio de claves"
		}
		if !strings.Contains(framed(m), label) || !strings.Contains(framed(m), "ML-KEM-768 + X25519") {
			t.Fatal(framed(m))
		}
		wideFit(t, m, 100, 30)
	}
}
