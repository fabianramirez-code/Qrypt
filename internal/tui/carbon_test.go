package tui

import (
	"crypto/sha256"
	"fmt"
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/settings"
	"github.com/fabianramirez-code/qrypt/internal/tui/theme"
)

func TestCarbonSelectionAndCustomRoles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	m := send(t, New(path, English), tea.WindowSizeMsg{Width: 60, Height: 20})
	m = key(t, m, tea.KeyTab)
	for range theme.Names()[:len(theme.Names())-1] {
		m = key(t, m, tea.KeyDown)
	}
	if m.themeName != "carbon" || !strings.Contains(framed(m), "> carbon") {
		t.Fatal("carbon unavailable", framed(m))
	}
	m = enter(t, m)
	stored, err := settings.Load(filepath.Join(filepath.Dir(path), "settings"))
	if err != nil || stored.Theme != "carbon" {
		t.Fatalf("carbon not saved: %+v %v", stored, err)
	}
	m = send(t, New(path, English), tea.WindowSizeMsg{Width: 60, Height: 20})
	if !strings.Contains(framed(m), "> carbon") {
		t.Fatal("carbon not reselected", framed(m))
	}
	m = key(t, m, tea.KeyTab)
	m = key(t, m, tea.KeyDown)
	m = key(t, m, tea.KeyDown)
	m = enter(t, m)
	if m.screen != colorEditor || len(m.settings.Colors) != 7 || m.themeName != "custom" || m.styles.palette.Surface != (color.RGBA{}) {
		t.Fatalf("custom inherited surface or lost roles: %+v", m.styles.palette)
	}
	if _, ok := m.settings.Colors["surface"]; ok {
		t.Fatal("surface is editable")
	}
	if m.settings.Colors["accent"] != "#50fa7b" || m.styles.palette.Background == m.editorPalette.Background {
		t.Fatal("custom did not copy carbon roles without its background")
	}
	m = key(t, m, tea.KeyEsc)
	if m.themeName != "carbon" {
		t.Fatal("unchanged custom editor did not return to carbon")
	}
	m = key(t, m, tea.KeyDown)
	m = key(t, m, tea.KeyDown)
	m = enter(t, m)
	m = typeText(t, m, "#ffffff")
	m = enter(t, m)
	m = key(t, m, tea.KeyEsc)
	stored, err = settings.Load(filepath.Join(filepath.Dir(path), "settings"))
	if err != nil || stored.Theme != "custom" || len(stored.Colors) != 7 || stored.Colors["accent"] != "#ffffff" || m.styles.palette.Surface != (color.RGBA{}) {
		t.Fatalf("carbon-derived custom: %+v, %v", stored, err)
	}
}

func TestCarbonPaintAndExistingThemeView(t *testing.T) {
	m := send(t, New(filepath.Join(t.TempDir(), "identity"), English), tea.WindowSizeMsg{Width: 60, Height: 20})
	for _, name := range []string{"green", "mono"} {
		m.themeName = name
		m.refreshStyles()
		v := m.View()
		if v.BackgroundColor != nil {
			t.Errorf("%s changed terminal background", name)
		}
		// Only the first screen has a new line and hint; welcome is unchanged.
		wantFirst := map[string]string{
			"green": "83349e45933bf5859b92c0897181fc6d463288abb1df214d1a837162f9b1a93b",
			"mono":  "56d76a4cc885cd35e66d8ae3adcdc2e960536d1ad2237491cee9911b6c385ee8",
		}[name]
		if got := fmt.Sprintf("%x", sha256.Sum256([]byte(v.Content))); got != wantFirst {
			t.Errorf("%s first screen changed: %s", name, got)
		}
		m.screen = welcome
		wantWelcome := map[string]string{
			"green": "2d0d44808623e1abfcffb17ab3711f1de671b7bda2605744f4aa4969cfd6c7f6",
			"mono":  "60f4d8efc7dc6d287865fa78d9b73a28c94aae8b6f88aecb2ea5912f4f195031",
		}[name]
		if got := fmt.Sprintf("%x", sha256.Sum256([]byte(m.View().Content))); got != wantWelcome {
			t.Errorf("%s welcome screen changed: %s", name, got)
		}
		m.screen = languageScreen
	}
	m.themeName = "carbon"
	m.refreshStyles()
	v := m.View()
	bg, _ := theme.ParseHex("#1E1F22")
	if v.BackgroundColor != bg {
		t.Errorf("background = %v, want %v", v.BackgroundColor, bg)
	}
	if !strings.Contains(v.Content, "\x1b[48;2;38;40;45m") {
		t.Fatal("first-screen panels lack surface")
	}
	m.screen = welcome
	if !strings.Contains(m.View().Content, "\x1b[48;2;38;40;45m") {
		t.Fatal("regular panel lacks surface")
	}
}

func TestCarbonFirstScreenFitsWithContent(t *testing.T) {
	for _, language := range []Language{English, Spanish} {
		m := send(t, New(filepath.Join(t.TempDir(), "identity"), language), tea.WindowSizeMsg{Width: 60, Height: 20})
		m.themeName = "carbon"
		m.refreshStyles()
		view := framed(m)
		lines := strings.Split(view, "\n")
		if len(lines) > 20 {
			t.Errorf("language %v: %d rows", language, len(lines))
		}
		for _, line := range lines {
			if lipgloss.Width(line) > 60 {
				t.Errorf("language %v: %d columns: %q", language, lipgloss.Width(line), line)
			}
		}
		for _, label := range append(append([]string{"English", "Español", "Edit colors / Editar colores"}, theme.Names()...), "Enter: select / elegir", "Esc: quit / salir") {
			if !strings.Contains(view, label) {
				t.Errorf("language %v: missing %q", language, label)
			}
		}
	}
}
