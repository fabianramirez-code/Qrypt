package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
)

func wideFixture(t *testing.T, language Language) Model {
	t.Helper()
	m := spacesSession(t, language)
	m = enter(t, m)
	m = addVaultItem(t, m, "account", "private-token")
	m = key(t, m, tea.KeyEsc)
	m = key(t, m, 'a')
	m = typeText(t, m, "Work")
	m = enter(t, m)
	m = enter(t, m)
	m = addVaultItem(t, m, "work-only", "different-value")
	m = key(t, m, tea.KeyEsc)
	m = key(t, m, tea.KeyUp)
	return m
}

func wideFit(t *testing.T, m Model, width, height int) {
	t.Helper()
	lines := strings.Split(framed(m), "\n")
	if len(lines) > height {
		t.Fatalf("%d rows > %d", len(lines), height)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > width {
			t.Fatalf("line %d > %d: %q", lipgloss.Width(line), width, line)
		}
	}
}

func TestWideThresholdPanesAndFocus(t *testing.T) {
	m := wideFixture(t, English)
	for _, size := range []tea.WindowSizeMsg{{Width: 99, Height: 30}, {Width: 100, Height: 19}} {
		narrow := send(t, m, size)
		if strings.Contains(framed(narrow), "work-only") || strings.Contains(framed(narrow), "account") {
			t.Fatalf("narrow showed adjacent pane: %s", framed(narrow))
		}
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 20}, {Width: 120, Height: 30}, {Width: 160, Height: 40}} {
		x := send(t, m, size)
		for _, state := range []screen{spacesList, vaultList, vaultItem} {
			if state == vaultList {
				x = enter(t, x)
			}
			if state == vaultItem {
				x = enter(t, x)
			}
			view := framed(x)
			panelTop := ""
			for _, line := range strings.Split(view, "\n") {
				if strings.HasPrefix(line, "╭") || strings.HasPrefix(line, "┏") {
					panelTop = line
					break
				}
			}
			if panelTop == "" {
				t.Fatal("no panes")
			}
			borders := strings.Fields(panelTop)
			if len(borders) != 3 {
				t.Fatalf("expected three bordered panes: %q", panelTop)
			}
			wantLeft := 25 + (size.Width-100)*25/100
			wantCenter := 43 + (size.Width-100)*45/100
			for i, want := range []int{wantLeft, wantCenter, size.Width - wantLeft - wantCenter - 2} {
				if lipgloss.Width(borders[i]) != want {
					t.Fatalf("pane %d width %d, want %d", i, lipgloss.Width(borders[i]), want)
				}
			}
			wideFit(t, x, size.Width, size.Height)
			for _, part := range []string{"Personal (1)", "Work (1)", "account"} {
				if !strings.Contains(view, part) {
					t.Fatalf("%dx%d %d missing %q: %s", size.Width, size.Height, state, part, view)
				}
			}
			if strings.Contains(view, "private-token") {
				t.Fatal("hidden value leaked")
			}
			focus := map[screen]int{spacesList: 0, vaultList: 1, vaultItem: 2}[state]
			for i, w := range []int{wantLeft, wantCenter, size.Width - wantLeft - wantCenter - 2} {
				style := x.styles.dim
				if i == focus {
					style = x.styles.accent
				}
				if !strings.Contains(x.View().Content, style.Render("╭"+strings.Repeat("─", w-2)+"╮")) {
					t.Fatalf("pane %d lacks expected focus border on %d", i, state)
				}
			}
		}
		x = key(t, x, tea.KeyTab)
		if !strings.Contains(framed(x), "private-token") {
			t.Fatal("Tab did not reveal focused item")
		}
	}
}

func TestWideHeaderFooterPreviewAndForms(t *testing.T) {
	m := wideFixture(t, English)
	for _, height := range []int{20, 30} {
		x := send(t, m, tea.WindowSizeMsg{Width: 100, Height: height})
		view := framed(x)
		if !strings.Contains(view, "Personal") || !strings.Contains(view, "1 items") {
			t.Fatalf("header/footer: %s", view)
		}
		if height == 20 && !strings.Contains(view, "◆ Qrypt") {
			t.Fatal("missing short header")
		}
		if height == 30 && !strings.Contains(view, strings.TrimRight(strings.Split(strings.ReplaceAll(logo, "\r", ""), "\n")[0], " ")) {
			t.Fatal("missing logo")
		}
		x = enter(t, x)
		x = copyKey(x) // fake clipboard; do not execute the timer command
		if !strings.Contains(framed(x), english.copied) {
			t.Fatal("missing copy status")
		}
		x.message = "an error"
		if !strings.Contains(framed(x), "an error") {
			t.Fatal("missing error status")
		}
	}
	for _, state := range []screen{spacesAdd, spacesRename, spacesDelete, vaultAddName, vaultAddValue, vaultEdit, vaultDelete} {
		x := send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
		x.screen = state
		if strings.Contains(framed(x), "Work (1)") {
			t.Fatalf("form %d used panes", state)
		}
	}
	seed := make([]byte, identity.SeedSize)
	id, err := identity.FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "identity")
	preview := enter(t, send(t, NewPreview(p, English, id), tea.WindowSizeMsg{Width: 100, Height: 20}))
	if !strings.Contains(framed(preview), english.previewBanner) {
		t.Fatal("preview banner missing")
	}
}

func TestWideAllThemesLanguagesAndLongValue(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		for _, name := range []string{"green", "violet", "mono", "contrast", "auto", "carbon"} {
			t.Run(fmt.Sprintf("%d-%s", lang, name), func(t *testing.T) {
				m := wideFixture(t, lang)
				m.themeName = name
				m.refreshStyles()
				m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})
				for _, state := range []screen{spacesList, vaultList, vaultItem} {
					m.screen = state
					wideFit(t, m, 100, 20)
				}
				if name == "carbon" && !strings.Contains(m.View().Content, "48;2;") {
					t.Fatal("surface not painted")
				}
			})
		}
	}
	m := wideFixture(t, English)
	m = enter(t, send(t, m, tea.WindowSizeMsg{Width: 100, Height: 20}))
	m = addVaultItem(t, m, "long", strings.Repeat("x", 301))
	m = enter(t, m)
	m = key(t, m, tea.KeyTab)
	if !strings.Contains(framed(m), "The value is too long to") || !strings.Contains(framed(m), "show here.") || strings.Contains(framed(m), "xxxx") {
		t.Fatalf("long value leaked or missing warning: %s", framed(m))
	}
}
