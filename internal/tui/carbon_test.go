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
		want := map[string]string{
			"green": "bec835ea8d9a171ae2cfc0cf705386ad7590bd3a13083933f310d10fcbe7b388",
			"mono":  "9f7dff29ef9de8c8b9f4925807425e8118b6d4321a3334b32c811edfa91af60c",
		}[name]
		if got := fmt.Sprintf("%x", sha256.Sum256([]byte(v.Content))); got != want {
			t.Errorf("%s view changed: %s", name, got)
		}
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
