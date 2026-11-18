package tui

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func logoModel(t *testing.T, themeName string) Model {
	t.Helper()
	m := send(t, New(filepath.Join(t.TempDir(), "identity"), English), tea.WindowSizeMsg{Width: 80, Height: 24})
	m.themeName = themeName
	m.refreshStyles()
	return m
}

func TestMotionUnchangedDoesNotSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	m := send(t, New(path, English), tea.WindowSizeMsg{Width: 80, Height: 24})
	m = key(t, m, 'a')
	m = key(t, m, 'a')
	m = enter(t, m)
	if _, err := os.Stat(m.settingsPath); !os.IsNotExist(err) {
		t.Fatalf("unchanged motion wrote settings: %v", err)
	}
}

func TestPlainLogoTickStops(t *testing.T) {
	m := logoModel(t, "green")
	m.plain = true
	_, cmd := m.Update(logoTickMsg{})
	if cmd != nil {
		t.Fatal("NO_COLOR rescheduled tick")
	}
}

func TestLogoTintAnimates(t *testing.T) {
	accent := color.RGBA{R: 40, G: 200, B: 90, A: 255}
	moved := false
	for col := 0; col < 46; col++ {
		if logoTint(accent, col, 46, 0, 0) != logoTint(accent, col, 46, 0, 20) {
			moved = true
		}
	}
	if !moved {
		t.Fatal("tint identical across phases")
	}
}

func TestLogoTintIsPeriodic(t *testing.T) {
	accent := color.RGBA{R: 40, G: 200, B: 90, A: 255}
	for _, p := range []int{0, 7, 33} {
		for col := 0; col < 46; col++ {
			for row := 0; row < 6; row++ {
				if logoTint(accent, col, 46, row, p) != logoTint(accent, col, 46, row, p+logoPeriod) {
					t.Fatalf("phase %d col %d row %d not periodic", p, col, row)
				}
			}
		}
	}
}

func TestLogoTintKeepsWhiteWhite(t *testing.T) {
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	for p := 0; p < logoPeriod; p++ {
		for col := 0; col < 46; col++ {
			for row := 0; row < 6; row++ {
				if got := logoTint(white, col, 46, row, p); got != white {
					t.Fatalf("phase %d col %d row %d: %v", p, col, row, got)
				}
			}
		}
	}
}

func TestInitStartsLogoAnimation(t *testing.T) {
	batch, ok := New(filepath.Join(t.TempDir(), "identity"), English).Init()().(tea.BatchMsg)
	if !ok {
		t.Fatal("Init is not a batch")
	}
	for _, cmd := range batch {
		if _, ok := cmd().(logoTickMsg); ok {
			return
		}
	}
	t.Fatal("Init does not start the logo tick")
}

func TestLogoTextNeverChanges(t *testing.T) {
	m := logoModel(t, "green")
	a := ansi.ReplaceAllString(m.logoView(), "")
	m.logoPhase = 17
	if b := ansi.ReplaceAllString(m.logoView(), ""); a != b {
		t.Fatal("logo text changed with phase")
	}
}

func TestPlainLogoIsStatic(t *testing.T) {
	m := logoModel(t, "green")
	m.plain = true
	m.refreshStyles()
	a := m.logoView()
	m.logoPhase = 17
	if m.logoView() != a {
		t.Fatal("plain logo changed with phase")
	}
}

func TestLogoTickAdvancesWithoutCountingAsActivity(t *testing.T) {
	m := logoModel(t, "green")
	m.lastKey = m.now().Add(-time.Minute)
	before := m.lastKey
	next, cmd := m.Update(logoTickMsg{})
	got := next.(Model)
	if got.logoPhase != m.logoPhase+1 {
		t.Fatalf("phase %d, want %d", got.logoPhase, m.logoPhase+1)
	}
	if cmd == nil {
		t.Fatal("tick did not reschedule")
	}
	if !got.lastKey.Equal(before) {
		t.Fatal("animation tick counted as user activity")
	}
}

// NO_COLOR selects the plain look: no hue anywhere and a logo that does not move.
func TestNoColorSelectsPlainLook(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := send(t, New(filepath.Join(t.TempDir(), "identity"), English), tea.WindowSizeMsg{Width: 80, Height: 24})
	if !m.plain {
		t.Fatal("NO_COLOR did not select the plain look")
	}
	before := m.View().Content
	if strings.Contains(before, "\x1b[38;2;") {
		t.Fatal("plain look draws a color")
	}
	m.logoPhase = 17
	if m.View().Content != before {
		t.Fatal("plain logo changed with phase")
	}
	t.Setenv("NO_COLOR", "")
	if New(filepath.Join(t.TempDir(), "identity"), English).plain {
		t.Fatal("empty NO_COLOR selected the plain look")
	}
}
