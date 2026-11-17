package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
)

func TestPreviewScreensAndExit(t *testing.T) {
	for _, tc := range []struct {
		lang   Language
		banner string
	}{{English, "PREVIEW: temporary data, nothing is saved"}, {Spanish, "VISTA PREVIA: datos temporales, no se guarda nada"}} {
		t.Run(tc.banner, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "identity")
			id, err := identity.FromSeed(make([]byte, identity.SeedSize))
			if err != nil {
				t.Fatal(err)
			}
			m := NewPreview(path, tc.lang, id)
			m.copyFn = func(string) error { return nil }
			m.clearFn = func(string) error { return nil }
			m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
			if !strings.Contains(plain(m), "LANGUAGE / IDIOMA") || strings.Contains(plain(m), tc.banner) {
				t.Fatal("not language first", plain(m))
			}
			m = enter(t, m)
			if m.ExitStatus() != 0 || !strings.Contains(plain(m), "Qrypt") && !strings.Contains(plain(m), "Bóveda") {
				t.Fatal("not vault", plain(m))
			}
			if strings.Contains(plain(m), "passphrase") || len(m.names) != 4 {
				t.Fatal("not sample vault", plain(m))
			}
			check := func() {
				t.Helper()
				if !strings.Contains(plain(m), tc.banner) {
					t.Fatal("banner absent", plain(m))
				}
				for _, line := range strings.Split(plain(m), "\n") {
					if lipgloss.Width(line) > 60 {
						t.Fatalf("too wide: %q", line)
					}
				}
			}
			check()
			m = enter(t, m)
			check()
			m = key(t, m, tea.KeyTab)
			check()
			m = key(t, m, tea.KeyEsc)
			check()
			m = key(t, m, 'a')
			check()
			m = enter(t, m)
			check()
			m = key(t, m, tea.KeyEsc)
			check()
			m = key(t, m, tea.KeyEsc)
			check()
			m = key(t, m, 'd')
			check()
			m = key(t, m, 'n')
			check()
			m = key(t, m, tea.KeyEsc)
			if m.ExitStatus() != 0 || m.vlt != nil {
				t.Fatal("lock failed")
			}
		})
	}
}

func TestPreviewAutoLockAndFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	id, err := identity.FromSeed(make([]byte, identity.SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	m := enter(t, send(t, NewPreview(path, English, id), tea.WindowSizeMsg{Width: 60, Height: 20}))
	m.now = func() time.Time { return m.lastKey.Add(idleLifetime) }
	next, cmd := m.Update(idleTickMsg{})
	if cmd == nil || next.(Model).ExitStatus() != 0 || next.(Model).vlt != nil {
		t.Fatal("auto lock failed")
	}
	// The failed vault view still identifies temporary data.
	block := filepath.Join(t.TempDir(), "block")
	if err := os.WriteFile(block, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	failed := NewPreview(filepath.Join(block, "identity"), English, id)
	failed = enter(t, send(t, failed, tea.WindowSizeMsg{Width: 60, Height: 20}))
	if failed.ExitStatus() != 1 || !strings.Contains(plain(failed), "PREVIEW: temporary data, nothing is saved") {
		t.Fatal("failure view", plain(failed))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("constructor wrote identity", err)
	}
}

func TestNormalStillRequiresPassphrase(t *testing.T) {
	m, _, _, _ := unlockFixture(t, English)
	if !strings.Contains(plain(m), "Type your passphrase (7 words):") || m.ExitStatus() != 1 {
		t.Fatal("normal unlock bypassed")
	}
	m = enter(t, m)
	if !strings.Contains(plain(m), "Type exactly 7 words.") || m.ExitStatus() != 1 {
		t.Fatal("empty phrase bypassed")
	}
}
