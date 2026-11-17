package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/clipboard"
)

func copyKey(m Model) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: 'c'})
	return next.(Model)
}

func TestVaultCopyAndExpiry(t *testing.T) {
	for _, language := range []Language{English, Spanish} {
		t.Run(map[Language]string{English: "English", Spanish: "Spanish"}[language], func(t *testing.T) {
			m, _, _ := vaultSession(t, language)
			now := time.Unix(100, 0)
			m.now = func() time.Time { return now }
			var copies, clears []string
			m.copyFn = func(s string) error { copies = append(copies, s); return nil }
			m.clearFn = func(s string) error { clears = append(clears, s); return nil }
			m = addVaultItem(t, m, "first", "private-one")
			m = addVaultItem(t, m, "second", "private-two")
			message := map[Language]string{English: "Copied. The clipboard is cleared in 30 seconds.", Spanish: "Copiado. El portapapeles se borra en 30 segundos."}[language]
			check := func(m Model) {
				t.Helper()
				if !strings.Contains(plain(m), message) || strings.Contains(plain(m), "private-") {
					t.Fatalf("copy message or value leaked: %q", plain(m))
				}
				for _, line := range strings.Split(plain(m), "\n") {
					if lipgloss.Width(line) > 60 {
						t.Fatalf("line over 60 columns: %q", line)
					}
				}
			}
			m = copyKey(m)
			check(m)
			if len(copies) != 1 || copies[0] != "private-two" {
				t.Fatalf("list copy: %q", copies)
			}
			old := m.copyToken
			m = enter(t, m)
			m = copyKey(m)
			check(m)
			if len(copies) != 2 || copies[1] != "private-two" || m.copyToken == old {
				t.Fatalf("item copy or token: %q", copies)
			}
			now = now.Add(30 * time.Second)
			m = send(t, m, clipboardTickMsg{token: old})
			if len(clears) != 0 {
				t.Fatalf("old timer cleared new copy: %q", clears)
			}
			m = send(t, m, clipboardTickMsg{token: m.copyToken})
			if len(clears) != 1 || clears[0] != "private-two" {
				t.Fatalf("expiry: %q", clears)
			}
			m = key(t, m, tea.KeyEsc)
			m = key(t, m, tea.KeyUp)
			m = copyKey(m)
			check(m)
			if copies[2] != "private-one" {
				t.Fatalf("selected list copy: %q", copies)
			}
			m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
			requireVaultView(t, m)
		})
	}
}

func TestVaultCopyEmptyValueClears(t *testing.T) {
	m, _, _ := vaultSession(t, English)
	m = addVaultItem(t, m, "empty", "initial")
	m = enter(t, m)
	m = key(t, m, 'e')
	m = enter(t, m)
	m = key(t, m, tea.KeyEsc)
	var cleared []string
	m.copyFn = func(s string) error {
		if s != "" {
			t.Fatalf("copied %q", s)
		}
		return nil
	}
	m.clearFn = func(s string) error { cleared = append(cleared, s); return nil }
	m = copyKey(m)
	if !m.copyActive {
		t.Fatal("empty copy not tracked")
	}
	m = send(t, m, clipboardTickMsg{token: m.copyToken})
	if len(cleared) != 1 || cleared[0] != "" || m.copyActive {
		t.Fatalf("empty expiry: %q", cleared)
	}
}

func TestVaultCopyFailureAndFallback(t *testing.T) {
	m, _, _ := vaultSession(t, English)
	m = addVaultItem(t, m, "name", "secret")
	m.copyFn = func(string) error { return errors.New("clipboard failure") }
	m.clearFn = func(string) error { t.Fatal("unexpected clear"); return nil }
	m = copyKey(m)
	if plain(m) == "" || !strings.Contains(plain(m), "clipboard failure") || strings.Contains(plain(m), "Copied.") {
		t.Fatalf("error message: %q", plain(m))
	}
	m.copyFn = func(string) error { return clipboard.ErrUnavailable }
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c'})
	m = next.(Model)
	if cmd == nil || !strings.Contains(plain(m), "Copied.") || m.copyToken == 0 {
		t.Fatalf("OSC fallback missing: %q", plain(m))
	}
	if cmd() == nil {
		t.Fatal("fallback command returned nil")
	}
	next, cmd = m.Update(clipboardTickMsg{token: m.copyToken})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("OSC clear missing")
	}
	if cmd() == nil {
		t.Fatal("OSC clear command returned nil")
	}
}

func TestVaultLockIdleAndActivity(t *testing.T) {
	for _, exit := range []rune{tea.KeyEsc, 'c'} {
		m, _, _ := vaultSession(t, English)
		m = addVaultItem(t, m, "name", "secret")
		now := time.Unix(100, 0)
		m.now = func() time.Time { return now }
		var cleared []string
		m.copyFn = func(string) error { return nil }
		m.clearFn = func(s string) error { cleared = append(cleared, s); return nil }
		m = copyKey(m)
		var keyMsg tea.KeyPressMsg
		if exit == tea.KeyEsc {
			m = key(t, m, tea.KeyEsc) // Item list returns to Spaces before locking.
			keyMsg = tea.KeyPressMsg{Code: exit}
		} else {
			keyMsg = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
		}
		next, cmd := m.Update(keyMsg)
		m = next.(Model)
		if cmd == nil || m.ExitStatus() != 0 || m.vlt != nil || len(cleared) != 1 || cleared[0] != "secret" || m.copyValue != "" {
			t.Fatalf("lock: status=%d vault=%v clear=%q copy=%q", m.ExitStatus(), m.vlt, cleared, m.copyValue)
		}
	}
	m, _, _ := vaultSession(t, English)
	m = addVaultItem(t, m, "name", "secret")
	now := time.Unix(100, 0)
	m.now = func() time.Time { return now }
	var cleared []string
	m.copyFn = func(string) error { return nil }
	m.clearFn = func(s string) error { cleared = append(cleared, s); return nil }
	m = copyKey(m)
	now = now.Add(4 * time.Minute)
	m = key(t, m, tea.KeyDown)
	now = now.Add(2 * time.Minute)
	m = send(t, m, idleTickMsg{})
	if m.vlt == nil {
		t.Fatal("idle timer ignored key activity")
	}
	now = now.Add(3 * time.Minute)
	next, cmd := m.Update(idleTickMsg{})
	m = next.(Model)
	if cmd == nil || m.ExitStatus() != 0 || m.vlt != nil || len(cleared) != 1 || cleared[0] != "secret" || m.copyValue != "" {
		t.Fatalf("idle lock: status=%d vault=%v clear=%q copy=%q", m.ExitStatus(), m.vlt, cleared, m.copyValue)
	}
}
