package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestRevealExpiresAndLockClears(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 24}, {120, 30}} {
		m, _, _ := vaultSession(t, English)
		m = addVaultItem(t, m, "account", "private-token")
		m = send(t, m, tea.WindowSizeMsg{Width: size.width, Height: size.height})
		m = enter(t, m)
		if strings.Contains(plain(m), "private-token") {
			t.Fatal("value visible before reveal")
		}
		code := tea.KeyTab
		if size.width >= 100 {
			code = tea.KeyEnter
		}
		next, cmd := m.Update(tea.KeyPressMsg{Code: code})
		m = next.(Model)
		if cmd == nil || !strings.Contains(plain(m), "private-token") || !strings.Contains(framed(m), "private-token") {
			t.Fatalf("value not revealed at %d: %s", size.width, plain(m))
		}
		if batch, ok := cmd().(tea.BatchMsg); !ok || len(batch) != 2 {
			t.Fatal("reveal must schedule one expiry")
		}
		first := revealTickMsg{token: m.revealToken}
		next, cmd = m.Update(tea.KeyPressMsg{Code: code}) // Hide manually.
		m = next.(Model)
		if cmd != nil || strings.Contains(plain(m), "private-token") {
			t.Fatal("manual hide failed")
		}
		next, cmd = m.Update(tea.KeyPressMsg{Code: code})
		m = next.(Model)
		if cmd == nil || !strings.Contains(plain(m), "private-token") {
			t.Fatal("second reveal failed")
		}
		second := revealTickMsg{token: m.revealToken}
		last := m.lastKey
		next, _ = m.Update(first)
		m = next.(Model)
		if !strings.Contains(plain(m), "private-token") {
			t.Fatal("stale tick hid newer reveal")
		}
		next, cmd = m.Update(second)
		m = next.(Model)
		if cmd != nil || strings.Contains(plain(m), "private-token") || !strings.Contains(framed(m), "••••••") {
			t.Fatal("expiry did not mask value")
		}
		if !m.lastKey.Equal(last) {
			t.Fatal("reveal timer counted as key activity")
		}
		next, cmd = m.Update(tea.KeyPressMsg{Code: code})
		m = next.(Model)
		if cmd == nil || !strings.Contains(plain(m), "private-token") {
			t.Fatal("reveal before lock failed")
		}
		m.lastKey = m.now().Add(-idleLifetime - time.Second)
		next, _ = m.Update(idleTickMsg{})
		m = next.(Model)
		if strings.Contains(framed(m), "private-token") || m.scopeID != nil {
			t.Fatal("lock retained revealed value")
		}
	}
}
