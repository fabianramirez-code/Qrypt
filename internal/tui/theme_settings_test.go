package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/settings"
	"github.com/fabianramirez-code/qrypt/internal/tui/theme"
)

func TestSettingsSelectionAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	m := send(t, New(path, English), tea.WindowSizeMsg{Width: 60, Height: 20})
	if !strings.Contains(framed(m), "> green") {
		t.Fatal(framed(m))
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "settings")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	original := m.View().Content
	m = key(t, m, tea.KeyTab)
	m = key(t, m, tea.KeyDown)
	if !strings.Contains(framed(m), "> carbon") || original == m.View().Content {
		t.Fatal("theme preview did not recolor", framed(m))
	}
	m = enter(t, m)
	saved, err := os.ReadFile(filepath.Join(filepath.Dir(path), "settings"))
	if err != nil || !strings.Contains(string(saved), "theme=carbon") {
		t.Fatalf("settings %q: %v", saved, err)
	}
	next := send(t, New(path, English), tea.WindowSizeMsg{Width: 60, Height: 20})
	if !strings.Contains(framed(next), "> carbon") {
		t.Fatal(framed(next))
	}
}

func TestTabSwitchesListsAndNoChangeWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "identity")
	m := send(t, New(path, English), tea.WindowSizeMsg{Width: 60, Height: 20})
	m = key(t, m, tea.KeyDown)
	if !strings.Contains(framed(m), "> Español") || !strings.Contains(framed(m), "> green") {
		t.Fatal(framed(m))
	}
	m = key(t, m, tea.KeyTab)
	m = key(t, m, tea.KeyDown)
	if !strings.Contains(framed(m), "> carbon") || !strings.Contains(framed(m), "> Español") {
		t.Fatal(framed(m))
	}
	m = key(t, m, tea.KeyLeft)
	m = key(t, m, tea.KeyUp)
	if !strings.Contains(framed(m), "> English") || !strings.Contains(framed(m), "> carbon") {
		t.Fatal(framed(m))
	}
	m = key(t, m, tea.KeyTab)
	m = key(t, m, tea.KeyUp)
	m = enter(t, m)
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("unchanged preference created directory: %v", err)
	}
}

func TestUnreadableSettingsNotice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity")
	if err := os.WriteFile(filepath.Join(dir, "settings"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	m := send(t, New(path, English), tea.WindowSizeMsg{Width: 60, Height: 20})
	if !strings.Contains(framed(m), "Settings could not be read") || !strings.Contains(framed(m), "> green") {
		t.Fatal(framed(m))
	}
	m = enter(t, m)
	data, err := os.ReadFile(filepath.Join(dir, "settings"))
	if err != nil || string(data) != "invalid" {
		t.Fatalf("overwritten: %q %v", data, err)
	}
}

func editor(t *testing.T) (Model, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "identity")
	m := send(t, New(path, English), tea.WindowSizeMsg{Width: 60, Height: 20})
	m = key(t, m, tea.KeyTab)
	for i := 0; i < len(theme.Names())+1; i++ {
		m = key(t, m, tea.KeyDown)
	}
	m = enter(t, m)
	if !strings.Contains(framed(m), "accent") {
		t.Fatal(framed(m))
	}
	return m, filepath.Join(filepath.Dir(path), "settings")
}

func TestCustomColors(t *testing.T) {
	m, path := editor(t)
	m = typeText(t, m, "#ff0000")
	m = enter(t, m)
	if !strings.Contains(framed(m), "#ff0000") {
		t.Fatal(framed(m))
	}
	m = typeText(t, m, "#xyzxyz")
	m = enter(t, m)
	if !strings.Contains(framed(m), "#ff0000") || !strings.Contains(framed(m), "invalid hex") {
		t.Fatal(framed(m))
	}
	m = key(t, m, tea.KeyEsc)
	s, err := settings.Load(path)
	if err != nil || s.Theme != "custom" || s.Colors["accent"] != "#ff0000" {
		t.Fatalf("stored: %+v %v", s, err)
	}
}

func TestEditorRejectsLowContrastAndBothResets(t *testing.T) {
	m, path := editor(t)
	original := m.settings.Colors["accent"]
	m = typeText(t, m, "#010101")
	m = enter(t, m)
	if !strings.Contains(framed(m), "contrast below 3:1") || m.settings.Colors["accent"] != original {
		t.Fatal("low contrast changed role", framed(m))
	}
	m = key(t, m, tea.KeyBackspace)
	for range "#010101" {
		m = key(t, m, tea.KeyBackspace)
	}
	m = typeText(t, m, "#ff0000")
	m = enter(t, m)
	if m.settings.Colors["accent"] != "#ff0000" {
		t.Fatal(framed(m))
	}
	m = send(t, m, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if m.settings.Colors["accent"] != original {
		t.Fatal("role reset failed")
	}
	m = typeText(t, m, "#ff0000")
	m = enter(t, m)
	m = send(t, m, tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if m.settings.Colors["accent"] != original {
		t.Fatal("whole theme reset failed")
	}
	m = key(t, m, tea.KeyEsc)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("resets to original wrote settings: %v", err)
	}
}

func TestBuiltInScreensFitMinimumWindow(t *testing.T) {
	for _, name := range theme.Names() {
		for _, language := range []Language{English, Spanish} {
			t.Run(name+string(rune('0'+language)), func(t *testing.T) {
				m := send(t, New(filepath.Join(t.TempDir(), "identity"), language), tea.WindowSizeMsg{Width: 60, Height: 20})
				m.themeName = name
				m.refreshStyles()
				m.phrase.Set(strings.Repeat("abandon ", 24))
				m.pass.Set(strings.TrimSpace(strings.Repeat("abandon ", 7)))
				for state := languageScreen; state <= colorEditor; state++ {
					m.screen = state
					view := m.View().Content
					lines := strings.Split(view, "\n")
					if len(lines) > 20 {
						t.Errorf("screen %d: %d rows", state, len(lines))
					}
					for _, line := range lines {
						if lipgloss.Width(line) > 60 {
							t.Errorf("screen %d: %d columns", state, lipgloss.Width(line))
						}
					}
				}
			})
		}
	}
}
