package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/settings"
)

func firstScreen(t *testing.T, width, height int) Model {
	t.Helper()
	m, _, _ := fixture(t)
	m.screen = languageScreen
	return send(t, m, tea.WindowSizeMsg{Width: width, Height: height})
}

func TestFirstScreenSingleCenteredBox(t *testing.T) {
	m := firstScreen(t, 100, 32)
	rows := strings.Split(framed(m), "\n")
	top, bottom := -1, -1
	for i, row := range rows {
		if strings.Contains(row, "╭") && top < 0 {
			top = i
		}
		if strings.Contains(row, "╯") {
			bottom = i
		}
	}
	if top < 0 || bottom <= top || top != (32-(bottom-top+1))/2 {
		t.Fatalf("box not vertically centered: %s", framed(m))
	}
	left := strings.Index(rows[top], "╭")
	if left != (100-lipgloss.Width(strings.TrimSpace(rows[top])))/2 {
		t.Fatalf("box not horizontally centered: %s", rows[top])
	}
	for _, label := range []string{"┌──────┐", "LANGUAGE / IDIOMA", "THEME / TEMA", "ICONS / ICONOS", "ANIMATION / ANIMACIÓN", "Enter: select"} {
		found := false
		for i := top + 1; i < bottom; i++ {
			if strings.Contains(rows[i], label) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s outside box or absent", label)
		}
	}
}

func TestFirstScreenSolidBoxTransparentExterior(t *testing.T) {
	m := firstScreen(t, 100, 32)
	for _, name := range []string{"green", "carbon", "custom"} {
		m.themeName = name
		m.refreshStyles()
		content := m.View().Content
		if m.View().BackgroundColor != nil {
			t.Errorf("%s paints exterior", name)
		}
		if !strings.Contains(content, "\x1b[48;2;") {
			t.Errorf("%s lacks solid background", name)
		}
	}
}

func TestFirstScreenSettingsAreMenuSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	m := send(t, New(path, English), tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, name := range []string{"ICONS / ICONOS", "ANIMATION / ANIMACIÓN"} {
		if !strings.Contains(framed(m), name) {
			t.Fatal(framed(m))
		}
	}
	if strings.Contains(framed(m), "Icons / Iconos:") || strings.Contains(framed(m), "Animation / Animación:") {
		t.Fatal("loose setting lines", framed(m))
	}
	m = key(t, m, tea.KeyTab)
	m = key(t, m, tea.KeyTab)
	m = key(t, m, tea.KeyDown)
	if !strings.Contains(framed(m), "> nerd") {
		t.Fatal("icon selection", framed(m))
	}
	m = key(t, m, tea.KeyRight)
	m = key(t, m, tea.KeyDown)
	if !strings.Contains(framed(m), "> off / detenida") {
		t.Fatal("animation selection", framed(m))
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = next.(Model)
	if cmd == nil || !strings.Contains(framed(m), "> on / activa") {
		t.Fatal("animation did not restart", framed(m))
	}
	m = key(t, m, tea.KeyDown)
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "settings")); !os.IsNotExist(err) {
		t.Fatalf("selection wrote: %v", err)
	}
	m = enter(t, m)
	stored, err := settings.Load(filepath.Join(filepath.Dir(path), "settings"))
	if err != nil || stored.Icons != "nerd" || stored.Motion != "reduced" {
		t.Fatalf("settings: %+v %v", stored, err)
	}
}

func TestFirstScreenFocusReadable(t *testing.T) {
	m := firstScreen(t, 80, 24)
	before := m.View().Content
	m = key(t, m, tea.KeyTab)
	if before == m.View().Content || !strings.Contains(framed(m), "> English") || !strings.Contains(framed(m), "> green") {
		t.Fatal("focus or unfocused selection invisible", framed(m))
	}
}

func TestFirstScreenMinimumAndBelow(t *testing.T) {
	m := firstScreen(t, 60, 20)
	lines := strings.Split(framed(m), "\n")
	if len(lines) > 20 {
		t.Fatalf("too tall: %d", len(lines))
	}
	for _, line := range lines {
		if lipgloss.Width(line) > 60 {
			t.Fatalf("too wide: %q", line)
		}
	}
	m = send(t, m, tea.WindowSizeMsg{Width: 59, Height: 19})
	if !strings.Contains(plain(m), "Enlarge the window") {
		t.Fatal(plain(m))
	}
}
