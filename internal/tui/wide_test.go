package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		x = enter(t, x)
		if !strings.Contains(framed(x), "private-token") {
			t.Fatal("Enter did not reveal focused item")
		}
	}
}

func TestWideNavigationRevealAndResize(t *testing.T) {
	m := send(t, wideFixture(t, English), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = key(t, m, tea.KeyLeft)
	if m.screen != spacesList || m.vlt == nil {
		t.Fatal("left locked spaces")
	}
	m = send(t, m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.screen != spacesList || m.vlt == nil {
		t.Fatal("shift-tab locked spaces")
	}
	m = key(t, m, tea.KeyRight)
	if m.screen != vaultList {
		t.Fatal("right did not focus items")
	}
	m = key(t, m, tea.KeyTab)
	if m.screen != vaultItem || string(m.itemValue) != "private-token" || m.visible {
		t.Fatal("Tab did not focus hidden detail")
	}
	m = enter(t, m)
	if !m.visible || !strings.Contains(framed(m), "private-token") {
		t.Fatal("Enter did not reveal")
	}
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.screen != vaultItem || !m.visible || !strings.Contains(framed(m), "private-token") {
		t.Fatal("resize lost narrow reveal")
	}
	m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = enter(t, m)
	if m.visible || strings.Contains(framed(m), "private-token") {
		t.Fatal("Enter did not hide")
	}
	m = enter(t, m)
	m = key(t, m, tea.KeyTab)
	if m.screen != spacesList || m.vlt == nil || m.itemValue != nil || m.visible || strings.Contains(framed(m), "private-token") {
		t.Fatal("Tab wrap retained value")
	}
	m = key(t, m, tea.KeyRight)
	m = key(t, m, tea.KeyRight)
	m = key(t, m, tea.KeyLeft)
	if m.screen != vaultList || m.itemValue != nil {
		t.Fatal("Left retained value")
	}
	m = send(t, m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.screen != spacesList {
		t.Fatal("Shift-Tab did not move back")
	}
	m = key(t, m, tea.KeyEsc)
	if m.vlt != nil || m.status != 0 {
		t.Fatal("Esc did not lock")
	}
}

func TestWideEmptyNavigationAndSelectionResize(t *testing.T) {
	m := send(t, spacesSession(t, English), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = key(t, m, tea.KeyRight)
	if m.screen != vaultList || m.cursor != 0 {
		t.Fatal("empty space did not open safely")
	}
	for _, code := range []rune{tea.KeyTab, tea.KeyRight} {
		m = key(t, m, code)
		if m.screen != vaultList || m.itemValue != nil || strings.Contains(framed(m), "private-token") {
			t.Fatal("empty item list entered detail")
		}
	}
	m = key(t, m, tea.KeyLeft)
	m = key(t, m, 'd')
	m = key(t, m, 'y')
	m = key(t, m, tea.KeyTab)
	if m.screen != spacesList || m.itemValue != nil {
		t.Fatal("no spaces entered list")
	}

	x := send(t, wideFixture(t, English), tea.WindowSizeMsg{Width: 120, Height: 30})
	x = key(t, x, tea.KeyDown)
	x = key(t, x, tea.KeyRight)
	x = key(t, x, tea.KeyRight)
	x = send(t, x, tea.WindowSizeMsg{Width: 80, Height: 24})
	x = send(t, x, tea.WindowSizeMsg{Width: 120, Height: 30})
	if x.screen != vaultItem || x.spaceTitle() != "Work" || x.item != "work-only" || x.visible || strings.Contains(framed(x), "different-value") {
		t.Fatal("resize lost space/item selection or revealed value")
	}
}

func TestWideTickCacheAndIdle(t *testing.T) {
	m := send(t, wideFixture(t, English), tea.WindowSizeMsg{Width: 120, Height: 30})
	start := m.lastKey
	m.names = nil
	next, _ := m.Update(logoTickMsg{})
	m = next.(Model)
	_ = framed(m)
	if m.lastKey != start || m.names != nil {
		t.Fatal("tick refreshed names or idle")
	}
	clock := start.Add(idleLifetime - time.Second)
	m.now = func() time.Time { return clock }
	m = key(t, m, tea.KeyDown)
	if m.lastKey != clock {
		t.Fatal("key did not reset idle")
	}
	clock = clock.Add(idleLifetime)
	next, _ = m.Update(idleTickMsg{})
	m = next.(Model)
	if m.vlt != nil || m.status != 0 {
		t.Fatal("idle did not lock")
	}
}

func TestWideActionsPreviewAndNarrowPreservation(t *testing.T) {
	m := send(t, wideFixture(t, English), tea.WindowSizeMsg{Width: 100, Height: 20})
	if !strings.Contains(framed(m), "account") || !strings.Contains(framed(m), "Personal (1)") {
		t.Fatal("spaces preview not cached")
	}
	m = key(t, m, tea.KeyDown)
	if !strings.Contains(framed(m), "work-only") || !strings.Contains(framed(m), "Work (1)") {
		t.Fatal("space switch not refreshed")
	}
	m = key(t, m, 'r')
	if m.screen != spacesRename {
		t.Fatal("R did not rename focused space")
	}
	m = typeText(t, m, "Office")
	m = enter(t, m)
	if m.screen != spacesList || !strings.Contains(framed(m), "Office (1)") {
		t.Fatal("rename preview stale")
	}
	m = key(t, m, 'a')
	if m.screen != spacesAdd {
		t.Fatal("A did not add focused space")
	}
	m = typeText(t, m, "Empty")
	m = enter(t, m)
	if !strings.Contains(framed(m), "Empty (0)") {
		t.Fatal("added count stale")
	}
	m = key(t, m, 'd')
	if m.screen != spacesDelete {
		t.Fatal("D did not delete focused space")
	}
	m = key(t, m, 'y')
	if strings.Contains(framed(m), "Empty (0)") {
		t.Fatal("deleted space still cached")
	}
	m = key(t, m, tea.KeyTab)
	if m.screen != vaultList {
		t.Fatal("Tab failed to open space")
	}
	m = copyKey(m)
	if m.message != english.copied || !m.copyActive {
		t.Fatal("list copy notice missing")
	}
	token := m.copyToken
	next, _ := m.Update(clipboardTickMsg{token: token})
	m = next.(Model)
	if m.copyActive {
		t.Fatal("copy did not expire")
	}
	m = key(t, m, 'a')
	if m.screen != vaultAddName {
		t.Fatal("list A did not open add")
	}
	m = typeText(t, m, "new-item")
	m = enter(t, m)
	m = typeText(t, m, "new-secret")
	m = enter(t, m)
	if !strings.Contains(framed(m), "new-item") || !strings.Contains(framed(m), "Office (2)") {
		t.Fatal("added item not reflected")
	}
	m = key(t, m, 'd')
	if m.screen != vaultDelete {
		t.Fatal("list D did not open delete")
	}
	m = key(t, m, 'y')
	if strings.Contains(framed(m), "new-item") || !strings.Contains(framed(m), "Office (1)") {
		t.Fatal("deleted item not reflected")
	}
	m = key(t, m, tea.KeyRight)
	if m.screen != vaultItem {
		t.Fatal("Right failed to open item")
	}
	m = key(t, m, 'e')
	if m.screen != vaultEdit {
		t.Fatal("detail E did not edit")
	}
	m = key(t, m, tea.KeyEsc)
	m = copyKey(m)
	if m.message != english.copied || !m.copyActive {
		t.Fatal("detail C did not copy")
	}
	m = key(t, m, tea.KeyLeft)
	if m.itemValue != nil || strings.Contains(framed(m), "different-value") {
		t.Fatal("copy or value leaked on leave")
	}
	m = key(t, m, tea.KeyLeft)
	m = key(t, m, tea.KeyEsc)
	if m.copyActive || m.copyValue != "" {
		t.Fatal("lock retained clipboard notice")
	}

	narrow := send(t, wideFixture(t, English), tea.WindowSizeMsg{Width: 80, Height: 24})
	narrow = enter(t, enter(t, narrow))
	narrow = enter(t, narrow)
	if narrow.visible {
		t.Fatal("narrow Enter revealed value")
	}
	narrow = key(t, narrow, tea.KeyTab)
	if !narrow.visible || !strings.Contains(framed(narrow), "private-token") {
		t.Fatal("narrow Tab failed to reveal")
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
					hint := map[screen]string{spacesList: lang.text().wideSpacesHint, vaultList: lang.text().wideListHint, vaultItem: lang.text().wideItemHint}[state]
					if !strings.Contains(framed(m), hint) {
						t.Fatalf("missing focused hint %q", hint)
					}
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
	m = enter(t, m)
	if !strings.Contains(framed(m), "The value is too long to") || !strings.Contains(framed(m), "show here.") || strings.Contains(framed(m), "xxxx") {
		t.Fatalf("long value leaked or missing warning: %s", framed(m))
	}
}

// Right on the detail pane has nowhere to go: it must not reveal the value.
func TestWideRightOnDetailPaneNeverReveals(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	m := NewPreview(filepath.Join(t.TempDir(), "identity"), English, id)
	m.copyFn = func(string) error { return nil }
	m.clearFn = func(string) error { return nil }
	m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = enter(t, m) // language screen -> spaces
	m = key(t, m, tea.KeyRight)
	m = key(t, m, tea.KeyRight)
	if m.screen != vaultItem {
		t.Fatalf("detail pane not focused: screen %d", m.screen)
	}
	before := framed(m)
	m = key(t, m, tea.KeyRight)
	if m.screen != vaultItem || m.visible || framed(m) != before || strings.Contains(framed(m), "DEMO-ACCOUNT-0000") {
		t.Fatalf("Right changed the detail pane:\n%s", framed(m))
	}
	m = enter(t, m)
	if !m.visible || !strings.Contains(framed(m), "DEMO-ACCOUNT-0000") {
		t.Fatalf("Enter did not reveal the value:\n%s", framed(m))
	}
}
