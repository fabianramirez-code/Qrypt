package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
)

func TestUnlockedMemoryWarning(t *testing.T) {
	for _, tc := range []struct {
		language Language
		text     string
	}{
		{English, "Memory is not locked: keys could reach the swap file."},
		{Spanish, "La memoria no está bloqueada: las claves podrían llegar al archivo de intercambio."},
	} {
		for _, width := range []int{60, 100} {
			m := send(t, New(filepath.Join(t.TempDir(), "identity"), tc.language), tea.WindowSizeMsg{Width: width, Height: 20})
			m.screen = spacesList
			m.scopeID = &identity.Identity{}
			m.memoryStatus = func() (bool, string) { return false, "refused" }
			shown := plain(m)
			if !strings.Contains(strings.Join(strings.Fields(shown), " "), tc.text) {
				t.Errorf("%d: missing warning: %q", width, shown)
			}
			lines := strings.Split(framed(m), "\n")
			if len(lines) > 20 {
				t.Errorf("%d: %d rows", width, len(lines))
			}
			for _, line := range lines {
				if lipgloss.Width(line) > width {
					t.Errorf("%d: %d columns", width, lipgloss.Width(line))
				}
			}
			m.memoryStatus = func() (bool, string) { return true, "" }
			if strings.Contains(plain(m), "Memory is not locked") || strings.Contains(plain(m), "La memoria no está bloqueada") {
				t.Errorf("warning on locked memory")
			}
		}
	}
}
