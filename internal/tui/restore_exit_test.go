package tui

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
)

func TestRestoreEscThenCreate(t *testing.T) {
	m, path, words := fixture(t)
	m = key(t, m, 'R')
	m = key(t, m, tea.KeyEsc)
	if m.screen != welcome || m.restoring || m.backup != "" || m.index != 0 || len(m.restoreWords) != 0 || m.visible || m.message != "" {
		t.Fatal("restore state survived Esc to Welcome")
	}
	m, pass := phrase(t, m)
	if m.screen != passphrase || !strings.Contains(plain(m), "Passphrase\n") || strings.Contains(plain(m), "New passphrase") {
		t.Fatalf("creation passphrase screen: %q", plain(m))
	}
	m = enter(t, m)
	m = typeText(t, m, pass)
	m = enter(t, m)
	if m.screen != recovery {
		t.Fatalf("creation skipped Recovery phrase: %q", plain(m))
	}
	for i, w := range words {
		if !strings.Contains(plain(m), strings.Join([]string{stringNumber(i + 1), w}, " ")) {
			t.Fatalf("missing numbered recovery word %d", i+1)
		}
	}
	m = enter(t, m)
	for _, position := range []int{2, 11, 23} {
		m = typeText(t, m, words[position])
		m = enter(t, m)
	}
	if m.screen != done || m.ExitStatus() != 0 || strings.Contains(plain(m), "Restored") {
		t.Fatalf("creation did not finish: %q", plain(m))
	}
	if _, err := identity.Load(path, []byte(pass)); err != nil {
		t.Fatal(err)
	}
	entries(t, path, 1)
}

func stringNumber(n int) string { return strconv.Itoa(n) + "." }

func TestRestoreEscThenUnlock(t *testing.T) {
	m, path, _, pass, original := restoreFixture(t, English, true)
	m = typeText(t, m, "qzxv")
	m = key(t, m, tea.KeyTab)
	m = send(t, m, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = key(t, m, tea.KeyEsc)
	if m.screen != unlock || m.restoring || m.input != "" || m.visible || m.message != "" || m.index != 0 || len(m.restoreWords) != 0 || m.backup != "" || !strings.Contains(plain(m), "Words typed: 0 of 7") {
		t.Fatalf("Unlock not reset: %q", plain(m))
	}
	m = typeText(t, m, pass)
	m = enter(t, m)
	if m.screen != unlocked || m.ExitStatus() != 0 {
		t.Fatalf("unlock failed: %q", plain(m))
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatalf("identity changed: %v", err)
	}
	entries(t, path, 1)
}

func TestRestoreEscDiscardsPartialWords(t *testing.T) {
	m, _, words, _, _ := restoreFixture(t, English, false)
	m = key(t, m, 'R')
	m = enter(t, m)
	for _, w := range words[:3] {
		m = typeText(t, m, w)
		m = enter(t, m)
	}
	for range 3 {
		m = key(t, m, tea.KeyEsc)
	}
	if m.screen != restoreWord || !strings.Contains(plain(m), "Word 1 of 24") {
		t.Fatal(plain(m))
	}
	m = key(t, m, tea.KeyEsc)
	m = key(t, m, tea.KeyEsc)
	if m.screen != welcome || m.restoring || m.index != 0 || len(m.restoreWords) != 0 || m.input != "" || m.visible || m.message != "" || m.backup != "" {
		t.Fatal("partial restore survived Esc")
	}
	m = key(t, m, 'R')
	m = enter(t, m)
	if m.screen != restoreWord || m.index != 0 || m.input != "" || len(m.restoreWords) != 0 || !strings.Contains(plain(m), "Word 1 of 24") {
		t.Fatal(plain(m))
	}
}

func TestRestoreWelcomeModifiers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		key     tea.KeyPressMsg
		restore bool
	}{
		{"plain lower", tea.KeyPressMsg{Code: 'r'}, true},
		{"plain upper", tea.KeyPressMsg{Code: 'R'}, true},
		{"shift lower", tea.KeyPressMsg{Code: 'r', Mod: tea.ModShift}, true},
		{"shift upper", tea.KeyPressMsg{Code: 'R', Mod: tea.ModShift}, true},
		{"caps lower", tea.KeyPressMsg{Code: 'r', Mod: tea.ModCapsLock}, true},
		{"caps upper", tea.KeyPressMsg{Code: 'R', Mod: tea.ModCapsLock}, true},
		{"ctrl", tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}, false},
		{"alt", tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, path, _ := fixture(t)
			m = send(t, m, tc.key)
			if tc.restore && m.screen != restoreIntro {
				t.Fatalf("R ignored with %s: %q", tc.name, plain(m))
			}
			if !tc.restore && m.screen != welcome {
				t.Fatalf("R accepted with %s: %q", tc.name, plain(m))
			}
			absence(t, path)
		})
	}
}
