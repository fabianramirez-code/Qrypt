package tui

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/tui/theme"
)

func TestArrowOnlyThemeSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	m := send(t, New(path, English), tea.WindowSizeMsg{Width: 80, Height: 24})
	m = key(t, m, tea.KeyRight)
	m = key(t, m, tea.KeyDown)
	if !strings.Contains(framed(m), "> carbon") || !strings.Contains(framed(m), "> English") {
		t.Fatalf("arrow-only theme selection: %s", framed(m))
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "settings")); !os.IsNotExist(err) {
		t.Fatalf("selection wrote settings: %v", err)
	}
}

func TestThemeAccentsOnFirstScreen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	m := send(t, New(path, English), tea.WindowSizeMsg{Width: 80, Height: 24})
	views := map[string]string{}
	for _, name := range []string{"green", "carbon"} {
		m.themeName = name
		m.refreshStyles()
		views[name] = m.View().Content
		accent, _ := theme.ByName(name, true)
		sgr := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", accent.Accent.R, accent.Accent.G, accent.Accent.B)
		if strings.Count(views[name], sgr) < 2 {
			t.Errorf("%s: border and header missing accent %s", name, sgr)
		}
	}
	if views["green"] == views["carbon"] {
		t.Fatal("theme colors did not change")
	}
}

func TestEmbeddedLogoVerbatim(t *testing.T) {
	m := send(t, New(filepath.Join(t.TempDir(), "identity"), English), tea.WindowSizeMsg{Width: 80, Height: 24})
	view := framed(m)
	data, err := os.ReadFile("logo.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range strings.Split(strings.TrimRight(string(data), " \n"), "\n") {
		if !strings.Contains(view, strings.TrimRight(row, " ")) {
			t.Fatalf("missing logo row %q", row)
		}
		if lipgloss.Width(strings.TrimRight(row, " ")) > 60 {
			t.Fatalf("wide logo row: %q", row)
		}
	}
}

func TestGreenFollowsTerminalBackground(t *testing.T) {
	m := send(t, New(filepath.Join(t.TempDir(), "identity"), English), tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, tc := range []struct {
		bg   color.RGBA
		dark bool
	}{{color.RGBA{255, 255, 255, 255}, false}, {color.RGBA{0, 0, 0, 255}, true}} {
		m = send(t, m, tea.BackgroundColorMsg{Color: tc.bg})
		palette, _ := theme.ByName("green", tc.dark)
		sgr := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", palette.Accent.R, palette.Accent.G, palette.Accent.B)
		if !strings.Contains(m.View().Content, sgr) {
			t.Fatalf("background %v: wrong accent", tc.bg)
		}
	}
}

func TestWidePanelWidth(t *testing.T) {
	m := send(t, New(filepath.Join(t.TempDir(), "identity"), English), tea.WindowSizeMsg{Width: 100, Height: 30})
	m = enter(t, m)
	lines := strings.Split(framed(m), "\n")
	for _, line := range lines {
		if strings.Contains(line, "╭") {
			width := lipgloss.Width(strings.TrimLeft(line, " "))
			if width != 64 {
				t.Fatalf("wide panel is not 64 columns: %q", line)
			}
			return
		}
	}
	t.Fatal("no rounded panel")
}

// Every screen keeps its content at the minimum window: it is never replaced by
// the request to enlarge, and the frame never exceeds 60 columns by 20 rows.
func TestFramedScreensFitMinimumWindowInEveryTheme(t *testing.T) {
	for _, language := range []Language{English, Spanish} {
		for _, name := range []string{"green"} {
			m, _, words := fixture(t)
			m.language = language
			m.themeName = name
			m.refreshStyles()
			m.words = words
			m.pass = strings.Join(words[:7], " ")
			m.positions = [3]int{2, 11, 23}
			m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
			for state := languageScreen; state <= cannotOpen; state++ {
				m.screen = state
				view := framed(m)
				lines := strings.Split(view, "\n")
				if strings.Contains(view, m.language.text().enlarge) || len(lines) > 20 {
					t.Fatalf("%s screen %d does not fit: %d rows\n%s", name, state, len(lines), view)
				}
				for _, line := range lines {
					if lipgloss.Width(line) > 60 {
						t.Fatalf("%s screen %d wide line: %q", name, state, line)
					}
				}
			}
		}
	}
}
