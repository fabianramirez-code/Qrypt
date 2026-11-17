package tui

import (
	"image/color"
	"path/filepath"
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

func TestMonoLogoIsStatic(t *testing.T) {
	m := logoModel(t, "mono")
	a := m.logoView()
	m.logoPhase = 17
	if m.logoView() != a {
		t.Fatal("mono logo changed with phase")
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
