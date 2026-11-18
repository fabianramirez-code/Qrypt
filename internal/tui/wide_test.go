package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/vault"
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

func TestWideFolderTreeAndFieldInspector(t *testing.T) {
	m := spacesSession(t, English)
	_, err := m.vlt.Add(vault.Item{Path: []string{"outer", "inner", "deep"}, Title: "nested", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "secret", Value: []byte("never-display-me")}, {Kind: vault.FieldNote, Label: "memo", Value: []byte("line-one\nline-two")}}})
	if err != nil {
		t.Fatal(err)
	}
	m.refreshSpaces()
	m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = enter(t, m)
	if !strings.Contains(framed(m), "outer (1)") || !strings.Contains(framed(m), "[2] Personal") {
		t.Fatal(framed(m))
	}
	m = enter(t, m)
	if !strings.Contains(framed(m), "[2] Personal / outer") || !strings.Contains(framed(m), "inner (1)") {
		t.Fatal(framed(m))
	}
	m = enter(t, m)
	m = enter(t, m)
	m = enter(t, m)
	if m.screen != vaultItem || !strings.Contains(framed(m), "memo: note (2)") || strings.Contains(framed(m), "line-one") || strings.Contains(framed(m), "never-display-me") {
		t.Fatal(framed(m))
	}
	m = enter(t, m)
	if !strings.Contains(framed(m), "never-display-me") {
		t.Fatal("chosen field did not reveal")
	}
	m = key(t, m, tea.KeyDown)
	if strings.Contains(framed(m), "never-display-me") {
		t.Fatal("previous field remained revealed")
	}
	copied := ""
	m.copyFn = func(s string) error { copied = s; return nil }
	m = copyKey(m)
	if copied != "line-one\nline-two" {
		t.Fatalf("copied %q", copied)
	}
	m = enter(t, m)
	if !strings.Contains(framed(m), "line-one") || strings.Contains(framed(m), "never-display-me") {
		t.Fatal(framed(m))
	}
	m = key(t, m, tea.KeyRight)
	m = enter(t, m)
	if strings.Contains(framed(m), "line-one") {
		t.Fatal("note not hidden")
	}
}

func TestWideEveryScreenFitsEveryPaletteIconAndLanguage(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		base := wideFixture(t, lang)
		for _, theme := range []string{"green", "carbon"} {
			for _, icons := range []string{"unicode", "emoji", "nerd"} {
				m := base
				m.themeName = theme
				m.icons = icons
				m.refreshStyles()
				for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 20}, {Width: 120, Height: 30}, {Width: 160, Height: 40}} {
					m = send(t, m, size)
					for _, state := range []screen{spacesList, vaultList, vaultItem, spacesAdd, spacesRename, spacesDelete, problemScreen, mirrorScreen, mirrorEdit, mirrorStop, vaultAddName, vaultAddValue, vaultEdit, vaultDelete, folderName, fieldLabel, fieldKind, fieldNote, vaultRename, fieldDelete} {
						x := m
						x.screen = state
						wideFit(t, x, size.Width, size.Height)
						if strings.Contains(framed(x), "private-token") {
							t.Fatalf("%s %s %d: secret leaked", theme, icons, state)
						}
					}
				}
			}
		}
	}
}

func TestWideInspectorScrollKeepsChosenFieldAndCard(t *testing.T) {
	m := spacesSession(t, English)
	fields := make([]vault.Field, 64)
	for i := range fields {
		fields[i] = vault.Field{Kind: vault.FieldHidden, Label: fmt.Sprintf("field-%02d", i), Value: []byte("not-visible")}
	}
	if _, err := m.vlt.Add(vault.Item{Title: "many", Fields: fields}); err != nil {
		t.Fatal(err)
	}
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})
	m = enter(t, enter(t, m))
	for range 63 {
		m = key(t, m, tea.KeyDown)
	}
	if !strings.Contains(framed(m), "field-63") || !strings.Contains(framed(m), "• • • • • •") || strings.Contains(framed(m), "not-visible") {
		t.Fatal(framed(m))
	}
	wideFit(t, m, 100, 20)
}

func TestWideTreeSelectionAndPreviewStructure(t *testing.T) {
	id, err := identity.FromSeed(make([]byte, identity.SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	m := NewPreview(filepath.Join(t.TempDir(), "identity"), English, id)
	m.copyFn = func(string) error { return nil }
	m.clearFn = func(string) error { return nil }
	m = enter(t, send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30}))
	if strings.Contains(framed(m), "Work: Google Stitch") || !strings.Contains(framed(m), "Work (5)") {
		t.Fatal(framed(m))
	}
	m = key(t, m, tea.KeyDown)
	if !strings.Contains(framed(m), "AWS (1)") || !strings.Contains(framed(m), "Google Stitch (2)") {
		t.Fatal(framed(m))
	}
	m = key(t, m, tea.KeyDown) // AWS in the tree
	if !strings.Contains(framed(m), "▌") {
		t.Fatal(framed(m))
	}
	m = key(t, m, tea.KeyRight)
	if m.screen != vaultList || !strings.Contains(framed(m), "[2] Work / AWS") || !strings.Contains(framed(m), "prod (1)") {
		t.Fatal(framed(m))
	}
	m = enter(t, m)
	if !strings.Contains(framed(m), "[2] Work / AWS / prod") || !strings.Contains(framed(m), "sample deployment") {
		t.Fatal(framed(m))
	}
	m = enter(t, m)
	if strings.Contains(framed(m), "Fake preview instructions") {
		t.Fatal("hidden note leaked")
	}
	m = enter(t, m)
	if !strings.Contains(framed(m), "Fake preview instructions") {
		t.Fatal("note reveal missing")
	}
	m = key(t, m, tea.KeyLeft)
	if strings.Contains(framed(m), "Fake preview instructions") {
		t.Fatal("note leaked after leaving")
	}
	m = send(t, m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if !strings.Contains(framed(m), "[2] Work / AWS") {
		t.Fatal("folder back missing")
	}
	m = key(t, m, tea.KeyEsc)
	if !strings.Contains(framed(m), "[2] Work") {
		t.Fatal("Esc did not ascend")
	}
}

func TestWideUnboxedColumnsAndRoles(t *testing.T) {
	m := send(t, wideFixture(t, English), tea.WindowSizeMsg{Width: 120, Height: 30})
	m.themeName = "carbon"
	m.refreshStyles()
	view := framed(m)
	for _, corner := range []string{"╭", "╮", "╰", "╯"} {
		if strings.Contains(view, corner) {
			t.Fatalf("pane has rounded corner %q", corner)
		}
	}
	for _, want := range []string{"[1] SPACES", "NAME", "[3] ITEM", "▌", "─", "│", IconGlyph(m.icons, "spaceOpen"), IconGlyph(m.icons, "item"), "(1)"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q", want)
		}
	}
	if !strings.Contains(m.View().Content, "48;2;38;40;45") {
		t.Error("surface background missing")
	}
	wideFit(t, m, 120, 30)
}

func TestWideInspectorStatusAndClock(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		m := send(t, wideFixture(t, lang), tea.WindowSizeMsg{Width: 120, Height: 30})
		now := time.Unix(1000, 0)
		m.now = func() time.Time { return now }
		m.lastKey = now
		view := framed(m)
		for _, want := range []string{">_ QRYPT // Personal", "● PQC: ML-KEM-768 + X25519", "[2] Personal", IconGlyph(m.icons, "spaceClosed"), IconGlyph(m.icons, "spaceOpen"), "5:00"} {
			if !strings.Contains(view, want) {
				t.Errorf("missing %q: %s", want, view)
			}
		}
		if strings.Contains(view, "1024") {
			t.Fatal("wrong algorithm")
		}
		if lang == English {
			for _, want := range []string{"[1] SPACES", "[3] ITEM", "NAME", "No mirror"} {
				if !strings.Contains(view, want) {
					t.Errorf("missing %q", want)
				}
			}
		} else {
			for _, want := range []string{"[1] ESPACIOS", "[3] ELEMENTO", "NOMBRE", "Sin espejo"} {
				if !strings.Contains(view, want) {
					t.Errorf("missing %q", want)
				}
			}
		}
		m = enter(t, enter(t, m))
		view = framed(m)
		for _, want := range []string{"XChaCha20-Poly1305", "ML-KEM-768 + X25519", "Ed25519 + ML-DSA-65", "• • • • • •", "[Enter]", "[C]", "[E]"} {
			if !strings.Contains(view, want) {
				t.Errorf("missing inspector %q: %s", want, view)
			}
		}
		if strings.Contains(view, "private-token") {
			t.Fatal("hidden value leaked")
		}
		if !strings.Contains(m.View().Content, "\x1b[") {
			t.Fatal("missing ANSI styles")
		}
		m = enter(t, m)
		if !strings.Contains(framed(m), "private-token") {
			t.Fatal("reveal missing")
		}
		m = key(t, m, tea.KeyLeft)
		if strings.Contains(framed(m), "private-token") {
			t.Fatal("unfocused value leaked")
		}
		m = copyKey(m)
		if !strings.Contains(framed(m), "30s") {
			t.Fatal("clipboard countdown missing")
		}
		start := m.lastKey
		now = now.Add(10 * time.Second)
		m = send(t, m, logoTickMsg{})
		if m.lastKey != start || !strings.Contains(framed(m), "4:50") || !strings.Contains(framed(m), "20s") {
			t.Fatalf("tick changed idle or countdown: %s", framed(m))
		}
		now = now.Add(20 * time.Second)
		m = send(t, m, clipboardTickMsg{token: m.copyToken})
		if strings.Contains(framed(m), "20s") || strings.Contains(framed(m), "30s") {
			t.Fatal("clipboard countdown persisted")
		}
		for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 20}, {Width: 120, Height: 30}, {Width: 160, Height: 40}} {
			wideFit(t, send(t, m, size), size.Width, size.Height)
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
			// The rule under the headings is crossed once per separator: it gives the pane widths.
			rule := ""
			for _, line := range strings.Split(view, "\n") {
				if strings.Contains(line, "┼") {
					rule = line
					break
				}
			}
			if rule == "" || strings.ContainsAny(view, "╭╮╰╯") {
				t.Fatalf("no unboxed panes: %s", view)
			}
			borders := strings.Split(rule, "┼")
			if len(borders) != 3 {
				t.Fatalf("expected three panes: %q", rule)
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
			// The focused pane is the one whose heading rule carries the accent.
			for i, w := range []int{wantLeft, wantCenter, size.Width - wantLeft - wantCenter - 2} {
				style := x.styles.dim
				if i == focus {
					style = x.styles.accent
				}
				if !strings.Contains(x.View().Content, style.Render(strings.Repeat("─", w))) {
					t.Fatalf("pane %d lacks expected focus rule on %d", i, state)
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
		t.Fatalf("shift-tab locked spaces: screen=%d vlt=%v wide=%v", m.screen, m.vlt != nil, m.wide())
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
		t.Fatalf("rename preview stale: %s", framed(m))
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
	m = typeText(t, m, "value")
	m = enter(t, m)
	m = key(t, m, 'h')
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
		if !strings.Contains(framed(x), "Work (1)") || !strings.Contains(framed(x), "[1] "+strings.ToUpper(x.language.text().spacesTitle)) {
			t.Fatalf("modal %d lacks panes", state)
		}
		wideFit(t, x, 120, 30)
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
		for _, name := range []string{"green", "carbon"} {
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
	if !strings.Contains(framed(m), "The value is too") || !strings.Contains(framed(m), "show here.") || strings.Contains(framed(m), "xxxx") {
		t.Fatalf("long value leaked or missing warning: %s", framed(m))
	}
}

func TestWideModalAddCompositionAndNarrow(t *testing.T) {
	m := send(t, wideFixture(t, English), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = enter(t, m)
	before := strings.Split(framed(m), "\n")
	m = key(t, m, 'a')
	if m.screen != vaultAddName {
		t.Fatal("add not opened")
	}
	view := framed(m)
	for _, title := range []string{"[1] SPACES", "Personal", "[3] ITEM", english.addTitle} {
		if !strings.Contains(view, title) {
			t.Fatalf("missing %q from modal and panes", title)
		}
	}
	wideFit(t, m, 120, 30)
	rows := strings.Split(view, "\n")
	start, end := -1, -1
	for i, row := range rows {
		if strings.Contains(row, "╭"+strings.Repeat("─", 62)+"╮") {
			start = i
		}
		if start >= 0 && strings.Contains(row, "╰"+strings.Repeat("─", 62)+"╯") {
			end = i
			break
		}
	}
	if start < 0 || end <= start || start <= 2 || end >= len(rows)-2 {
		t.Fatalf("modal borders not within panes: %d..%d", start, end)
	}
	if x := strings.Index(rows[start], "╭"+strings.Repeat("─", 62)+"╮"); x < 0 || lipgloss.Width(rows[start][:x]) != 28 || lipgloss.Width(rows[end][:strings.Index(rows[end], "╰")]) != 28 {
		t.Fatalf("modal borders not centered within window: %q / %q", rows[start], rows[end])
	}
	for i := range before {
		if (i < start || i > end) && i < len(before)-2 {
			// The compositor drops the trailing spaces of an unboxed row.
			if strings.TrimRight(rows[i], " ") != strings.TrimRight(before[i], " ") {
				t.Fatalf("background row %d changed: %q / %q", i, rows[i], before[i])
			}
		}
	}
	m = typeText(t, m, "new-entry")
	if !strings.Contains(framed(m), "new-entry") {
		t.Fatal("name not in modal")
	}
	m = enter(t, m)
	m = typeText(t, m, "value")
	m = enter(t, m)
	m = key(t, m, 'h')
	m = typeText(t, m, "fresh-value")
	if strings.Contains(framed(m), "fresh-value") {
		t.Fatal("value exposed")
	}
	m = key(t, m, tea.KeyTab)
	if !strings.Contains(framed(m), "fresh-value") {
		t.Fatal("Tab failed to reveal field")
	}
	m = enter(t, m)
	if m.screen != vaultList || !strings.Contains(framed(m), "new-entry") || strings.Contains(framed(m), "fresh-value") {
		t.Fatal("save did not return to panes safely")
	}
	m = key(t, m, 'a')
	m = key(t, m, tea.KeyEsc)
	if m.screen != vaultList || strings.Contains(framed(m), english.addTitle) {
		t.Fatal("Esc did not close modal")
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 60, Height: 20}} {
		x := send(t, m, size)
		x = key(t, x, 'a')
		if strings.Contains(framed(x), "[1] SPACES") || strings.Contains(framed(x), "[3] ITEM") {
			t.Fatalf("narrow modal got panes at %dx%d", size.Width, size.Height)
		}
	}
}

func TestWideModalScreensThemesAndSecurity(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		for _, theme := range []string{"green", "carbon"} {
			m := wideFixture(t, lang)
			m.themeName = theme
			m.refreshStyles()
			for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 20}, {Width: 120, Height: 30}} {
				m = send(t, m, size)
				for _, state := range []screen{spacesAdd, spacesRename, spacesDelete, vaultAddName, vaultAddValue, vaultEdit, vaultDelete, mirrorScreen, mirrorEdit, mirrorStop} {
					x := m
					x.screen = state
					wideFit(t, x, size.Width, size.Height)
					if !strings.Contains(framed(x), "[1] "+strings.ToUpper(lang.text().spacesTitle)) || !strings.Contains(framed(x), "[3] "+strings.ToUpper(lang.text().itemTitle)) {
						t.Fatalf("%s %d %dx%d did not overlay panes", theme, state, size.Width, size.Height)
					}
					if strings.Contains(framed(x), "private-token") {
						t.Fatal("background exposed value")
					}
					if theme == "carbon" && !strings.Contains(x.View().Content, "48;2;") {
						t.Fatal("modal surface not painted")
					}
				}
			}
		}
	}
	m := send(t, wideFixture(t, English), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = enter(t, enter(t, m))
	m = enter(t, m)
	m = key(t, m, 'e')
	background := m
	background.screen, background.visible = vaultItem, false
	if strings.Contains(m.View().Content, "private-token") || !strings.Contains(framed(background), "• • • • • •") {
		t.Fatal("edit leaked previously revealed value or background not masked")
	}
	m = typeText(t, m, "private-token")
	if strings.Contains(framed(m), "private-token") {
		t.Fatal("edit field not masked")
	}
	m = key(t, m, tea.KeyTab)
	if !strings.Contains(framed(m), "private-token") {
		t.Fatal("edit Tab did not reveal field")
	}
	m = key(t, m, tea.KeyEsc)
	if strings.Contains(framed(m), "private-token") || !strings.Contains(framed(m), "• • • • • •") {
		t.Fatal("edit Esc did not restore mask")
	}
}

func TestWideModalFaintFooterPreviewAndFocus(t *testing.T) {
	m := send(t, wideFixture(t, English), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = enter(t, m)
	m = key(t, m, 'a')
	m.message = "an error"
	m.preview = true
	content := m.View().Content
	if !strings.Contains(framed(m), "an error") || !strings.Contains(framed(m), english.previewBanner) || strings.Contains(framed(m), english.wideListHint) {
		t.Fatal("modal footer or preview wrong")
	}
	if !regexp.MustCompile(`(?s)\x1b\[[0-9;]*2m[^\n]*Personal`).MatchString(content) {
		t.Fatal("background is not faint")
	}
	if regexp.MustCompile(`\x1b\[[0-9;]*2m[^\n]*Add item`).MatchString(content) {
		t.Fatal("modal title is faint")
	}
	for _, tc := range []struct{ state, focus screen }{
		{vaultAddName, vaultList}, {vaultAddValue, vaultList}, {vaultDelete, vaultList},
		{vaultEdit, vaultItem}, {spacesAdd, spacesList}, {spacesRename, spacesList},
		{spacesDelete, spacesList}, {mirrorScreen, spacesList}, {mirrorEdit, spacesList}, {mirrorStop, spacesList},
	} {
		x := m
		x.screen = tc.state
		if x.modalPane() != tc.focus {
			t.Fatalf("screen %d focused wrong pane", tc.state)
		}
	}
}

func TestWideModalSpaceAndMirrorActions(t *testing.T) {
	m := send(t, wideFixture(t, English), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = key(t, m, 'a')
	if m.screen != spacesAdd || !strings.Contains(framed(m), english.spaceAddTitle) {
		t.Fatal("space add modal missing")
	}
	m = typeText(t, m, "New Space")
	m = enter(t, m)
	if m.screen != spacesList || !strings.Contains(framed(m), "New Space (0)") {
		t.Fatalf("space add not reflected in panes: %s", framed(m))
	}
	m = key(t, m, 'r')
	m = typeText(t, m, "Renamed")
	m = enter(t, m)
	if m.screen != spacesList || !strings.Contains(framed(m), "Renamed (0)") {
		t.Fatal("space rename not reflected in panes")
	}
	m = key(t, m, 'd')
	m = key(t, m, 'n')
	if !strings.Contains(framed(m), "Renamed (0)") {
		t.Fatal("space deletion cancel changed list")
	}
	m = key(t, m, 'd')
	m = key(t, m, 'y')
	if strings.Contains(framed(m), "Renamed (0)") {
		t.Fatal("space deletion not reflected")
	}
	m.settingsPath = filepath.Join(t.TempDir(), "settings")
	m = key(t, m, 'm')
	if m.screen != mirrorScreen || !strings.Contains(framed(m), english.mirrorTitle) {
		t.Fatal("mirror main modal missing")
	}
	m = key(t, m, 'e')
	if m.screen != mirrorEdit {
		t.Fatal("mirror folder modal missing")
	}
	folder := filepath.Join(t.TempDir(), "mirror")
	m = typeText(t, m, folder)
	m = enter(t, m)
	if m.screen != mirrorScreen || m.settings.Mirror != folder || !strings.Contains(framed(m), filepath.Base(folder)) {
		t.Fatal("mirror folder not saved or not shown")
	}
	m = key(t, m, 'x')
	if m.screen != mirrorStop || !strings.Contains(framed(m), english.mirrorStopPrompt) {
		t.Fatal("mirror stop modal missing")
	}
	m = key(t, m, 'n')
	if m.settings.Mirror != folder || m.screen != mirrorScreen {
		t.Fatal("mirror stop cancel changed setting")
	}
	m = key(t, m, 'x')
	m = key(t, m, 'y')
	if m.settings.Mirror != "" || m.screen != mirrorScreen {
		t.Fatal("mirror stop did not clear setting")
	}
	m = key(t, m, tea.KeyEsc)
	if m.screen != spacesList || !strings.Contains(framed(m), "[1] SPACES") {
		t.Fatal("mirror Esc did not return to panes")
	}
}

func TestWideModalRealProblemFallback(t *testing.T) {
	m, _, _ := problemFixture(t, English, false, func(t *testing.T, dir string) {
		if err := os.WriteFile(filepath.Join(dir, "secrets", "0123456789abcdef0123456789abcdef.qrs"), []byte("broken"), 0600); err != nil {
			t.Fatal(err)
		}
	})
	m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = key(t, m, tea.KeyDown)
	m = enter(t, m)
	if m.screen != problemScreen || !strings.Contains(framed(m), english.problemTitle) || !strings.Contains(framed(m), "[1] SPACES") {
		t.Fatal("real problem not over panes")
	}
	wideFit(t, m, 120, 30)
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})
	wideFit(t, m, 100, 20)
	if !strings.Contains(framed(m), english.problemPrompt) || !strings.Contains(framed(m), english.problemHint) {
		t.Fatal("problem fallback lost content")
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

// At the smallest wide window the inspector shows every value whole, its value
// box is closed on both sides, and the generation follows the space's changes.
func TestWideInspectorFitsItsValuesAtTheThreshold(t *testing.T) {
	for _, language := range []Language{English, Spanish} {
		id, err := identity.Generate()
		if err != nil {
			t.Fatal(err)
		}
		m := NewPreview(filepath.Join(t.TempDir(), "identity"), language, id)
		m.copyFn = func(string) error { return nil }
		m.clearFn = func(string) error { return nil }
		m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})
		m = enter(t, m) // language screen -> spaces
		m = enter(t, m) // spaces -> items
		m = enter(t, m) // items -> detail
		view := framed(m)
		for _, want := range []string{"Personal", "XChaCha20-Poly1305", "ML-KEM-768 + X25519", "Ed25519 + ML-DSA-65"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%q missing or cut:\n%s", want, view)
			}
		}
		generation := regexp.MustCompile(`(?:generation|generación|gen) (\d+)`)
		before := generation.FindStringSubmatch(view)
		// The whole values are asserted above; a long space name may still be cut in its own pane.
		if before == nil {
			t.Fatalf("generation missing:\n%s", view)
		}
		// The value card: a rule carrying the label, the hidden dots, and a closing rule.
		lines := strings.Split(view, "\n")
		card := -1
		for i, line := range lines {
			if strings.Contains(line, "── "+m.language.text().wideValue+" ─") {
				card = i
			}
		}
		if card < 0 || card+2 >= len(lines) || !strings.Contains(lines[card+1], "• • • • • •") || !strings.Contains(lines[card+2], "──────") {
			t.Fatalf("value card not found:\n%s", view)
		}
		m = key(t, m, 'e')
		m = typeText(t, m, "new value")
		m = enter(t, m)
		after := generation.FindStringSubmatch(framed(m))
		if after == nil || after[1] == before[1] {
			t.Fatalf("generation did not change after an edit: %v -> %v\n%s", before, after, framed(m))
		}
	}
}

// Every icon set fits every wide pane screen: emoji are two cells wide and the
// Nerd Font glyphs are private-use code points.
func TestWidePanesFitWithEveryIconSet(t *testing.T) {
	for _, icons := range []string{"unicode", "emoji", "nerd"} {
		for _, name := range []string{"green", "carbon"} {
			for _, language := range []Language{English, Spanish} {
				m := wideFixture(t, language)
				m.icons = icons
				m.themeName = name
				m.refreshStyles()
				for _, size := range [][2]int{{100, 20}, {120, 30}, {160, 40}} {
					x := send(t, m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
					for _, state := range []screen{spacesList, vaultList, vaultItem} {
						y := x
						if state != spacesList {
							y = enter(t, y)
						}
						if state == vaultItem {
							y = enter(t, y)
						}
						wideFit(t, y, size[0], size[1])
						if !strings.Contains(framed(y), IconGlyph(icons, "spaceOpen")) {
							t.Fatalf("%s %s: the open space icon is missing", icons, name)
						}
					}
				}
			}
		}
	}
}
