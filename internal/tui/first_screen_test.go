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
	for _, label := range []string{"██████╗", "LANGUAGE / IDIOMA", "THEME / TEMA", "Enter: select"} {
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

// Icons and animation are not options: the first screen does not offer them,
// their old keys do nothing, and values saved by earlier versions are ignored.
func TestFirstScreenHasNoIconOrAnimationOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	saved := "icons=nerd\nmotion=reduced\n"
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "settings"), []byte(saved), 0o600); err != nil {
		t.Fatal(err)
	}
	m := send(t, New(path, English), tea.WindowSizeMsg{Width: 100, Height: 34})
	if m.icons != settings.DefaultIcons || m.motion != "" {
		t.Fatalf("saved values used: icons %q motion %q", m.icons, m.motion)
	}
	before := plain(m)
	for _, gone := range []string{"ICONS", "ICONOS", "ANIMATION", "ANIMACIÓN", "I: icons"} {
		if strings.Contains(before, gone) {
			t.Errorf("first screen still shows %q", gone)
		}
	}
	for _, code := range []rune{'i', 'a'} {
		next, cmd := m.Update(tea.KeyPressMsg{Code: code, Text: string(code)})
		m = next.(Model)
		if cmd != nil || plain(m) != before || m.icons != settings.DefaultIcons || m.motion != "" {
			t.Fatalf("key %q changed the first screen", code)
		}
	}
	// Tab still alternates between the two remaining sections.
	if m = key(t, key(t, m, tea.KeyTab), tea.KeyTab); m.preferenceSection != 0 {
		t.Fatalf("section %d after two Tabs", m.preferenceSection)
	}
}
