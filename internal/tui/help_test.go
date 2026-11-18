package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestHelpEveryScreenAndLanguage(t *testing.T) {
	for state := languageScreen; state <= colorEditor; state++ {
		var englishKeys string
		for _, lang := range []Language{English, Spanish} {
			m := send(t, New(filepath.Join(t.TempDir(), "identity"), lang), tea.WindowSizeMsg{Width: 60, Height: 20})
			m.screen = state
			m.language = lang
			if state == languageScreen {
				m = key(t, m, '?')
			} else {
				m = key(t, m, tea.KeyF1)
			}
			view := framed(m)
			content := strings.Split(plain(m), "\n")
			var keyLines []string
			for _, line := range content {
				line = strings.TrimSpace(line)
				if line == "" || line == "Help" || line == "Ayuda" || line == "Help / Ayuda" || strings.Contains(line, "Esc / ? / F1:") {
					continue
				}
				keyLines = append(keyLines, line)
			}
			keys := strings.Join(keyLines, "\n")
			if state != languageScreen && lang == Spanish && keys == englishKeys {
				t.Fatalf("screen %d has no Spanish key list", state)
			}
			if lang == English {
				englishKeys = keys
			}
			if !strings.Contains(view, map[Language]string{English: "Help", Spanish: "Ayuda"}[lang]) || !strings.Contains(view, "Esc") || strings.TrimSpace(keys) == "" {
				t.Fatalf("screen %d language %d missing help: %q", state, lang, view)
			}
			if len(strings.Split(view, "\n")) > 20 {
				t.Fatalf("screen %d help exceeds 20 rows", state)
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > 60 {
					t.Fatalf("screen %d help exceeds 60 columns: %q", state, line)
				}
			}
		}
	}
}

func TestHelpOverlayIgnoresKeysAndPreservesData(t *testing.T) {
	m, path, _ := vaultSession(t, English)
	m = addVaultItem(t, m, "account", "private-token")
	m = enter(t, m)
	beforeFiles := vaultFiles(t, filepath.Join(filepath.Dir(path), "vault"))
	m.copyFn = func(string) error { t.Fatal("help copied a value"); return nil }
	m = key(t, m, tea.KeyTab)
	if !strings.Contains(plain(m), "private-token") {
		t.Fatal("reveal setup")
	}
	m = key(t, m, '?')
	if strings.Contains(framed(m), "private-token") || !strings.Contains(plain(m), "Help") {
		t.Fatal("help exposed secret or did not open")
	}
	before := framed(m)
	for _, code := range []rune{'c', 'd', tea.KeyEnter, tea.KeyTab} {
		m = key(t, m, code)
		if framed(m) != before {
			t.Fatalf("key %v changed overlay", code)
		}
	}
	m = key(t, m, tea.KeyEsc)
	if strings.Contains(framed(m), "private-token") || strings.Contains(plain(m), "Help") || !strings.Contains(plain(m), "account") {
		t.Fatal("help close changed screen or revealed value")
	}
	m = key(t, m, '?')
	m = key(t, m, tea.KeyF1)
	if strings.Contains(plain(m), "Help") {
		t.Fatal("F1 did not close")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "settings")); !os.IsNotExist(err) {
		t.Fatalf("help wrote settings: %v", err)
	}
	sameVaultFiles(t, filepath.Join(filepath.Dir(path), "vault"), beforeFiles)
}

func TestHelpQuestionTypedInFieldsAndWideModal(t *testing.T) {
	m := send(t, New(filepath.Join(t.TempDir(), "identity"), English), tea.WindowSizeMsg{Width: 80, Height: 24})
	m = enter(t, m)
	m.screen = unlock
	m = key(t, m, '?')
	if !strings.Contains(plain(m), "*") || strings.Contains(plain(m), "Help") {
		t.Fatal("question not typed into passphrase")
	}
	m = key(t, m, tea.KeyF1)
	if !strings.Contains(plain(m), "Help") {
		t.Fatal("F1 did not open in a field")
	}
	m = key(t, m, tea.KeyEsc)
	if !strings.Contains(plain(m), "*") {
		t.Fatal("closing help discarded input")
	}
	m, _, _ = vaultSession(t, English)
	m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	m = key(t, m, tea.KeyF1)
	if !strings.Contains(framed(m), "Help") || !strings.Contains(framed(m), "[1] SPACES") {
		t.Fatal("wide help not drawn over panes")
	}
	m.lastKey = m.now().Add(-idleLifetime)
	m = send(t, m, idleTickMsg{})
	if strings.Contains(framed(m), "Help") {
		t.Fatal("idle lock left help open")
	}
}
