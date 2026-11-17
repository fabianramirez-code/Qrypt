package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
)

func unlockFixture(t *testing.T, language Language) (Model, string, string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "identity")
	seed := make([]byte, identity.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	id, err := identity.FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	data, pass, err := id.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if err := identity.Save(path, data); err != nil {
		t.Fatal(err)
	}
	m := send(t, New(path, language), tea.WindowSizeMsg{Width: 80, Height: 24})
	m = enter(t, m)
	t.Cleanup(func() {
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil {
			t.Error(err)
			return
		}
		expected := 1
		if _, err := os.Stat(path); os.IsNotExist(err) {
			expected = 0
		}
		for _, e := range entries {
			if e.Name() == "vault" {
				expected++
			}
		}
		if len(entries) != expected {
			t.Errorf("unexpected directory entries: %d", len(entries))
		}
	})
	return m, path, pass, data
}

func requireUnlock(t *testing.T, m Model, parts ...string) {
	t.Helper()
	if !m.View().AltScreen {
		t.Fatal("not alternate screen")
	}
	for _, part := range parts {
		if !strings.Contains(plain(m), part) {
			t.Fatalf("missing %q in %q", part, plain(m))
		}
	}
	for _, line := range strings.Split(plain(m), "\n") {
		if lipgloss.Width(line) > 60 {
			t.Fatalf("wide line: %q", line)
		}
	}
}

func beginCheck(t *testing.T, m Model, typed string) (Model, tea.Cmd) {
	t.Helper()
	m = typeText(t, m, typed)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("no check command")
	}
	return next.(Model), cmd
}
func finishCheck(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	next, _ := m.Update(cmd())
	return next.(Model)
}

func TestUnlockStartAndField(t *testing.T) {
	for _, tc := range []struct {
		language                   Language
		title, prompt, count, hint string
	}{
		{English, "Unlock Qrypt", "Type your passphrase (7 words):", "Words typed: 0 of 7", "Enter: unlock  Tab: show/hide  Esc: quit"},
		{Spanish, "Abrir Qrypt", "Escribe tu frase de contraseña (7 palabras):", "Palabras escritas: 0 de 7", "Enter: abrir  Tab: mostrar/ocultar  Esc: salir"},
	} {
		t.Run(tc.title, func(t *testing.T) {
			m, _, _, _ := unlockFixture(t, tc.language)
			requireUnlock(t, m, tc.title, tc.prompt, tc.count, tc.hint)
			if strings.Contains(plain(m), "not available yet") || strings.Contains(plain(m), "todavía no está disponible") {
				t.Fatal(plain(m))
			}
			m = typeText(t, m, "zebra zero")
			requireUnlock(t, m, "**********", map[Language]string{English: "Words typed: 2 of 7", Spanish: "Palabras escritas: 2 de 7"}[tc.language])
			if strings.Contains(plain(m), "zebra") {
				t.Fatal("hidden field leaked")
			}
			m = key(t, m, tea.KeyTab)
			requireUnlock(t, m, "zebra zero")
			m = key(t, m, tea.KeyTab)
			if strings.Contains(plain(m), "zebra") {
				t.Fatal("field not hidden")
			}
		})
	}
}

func TestUnlockSuccessAndExit(t *testing.T) {
	m, path, pass, data := unlockFixture(t, Spanish)
	m, cmd := beginCheck(t, m, pass)
	requireUnlock(t, m, "Abrir Qrypt", "Comprobando tu frase de contraseña. Espera un momento.")
	next, ignored := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if ignored != nil || plain(next.(Model)) != plain(m) {
		t.Fatal("waiting accepted key")
	}
	m = finishCheck(t, m, cmd)
	requireUnlock(t, m, "Espacios", "Personal (0)", "Esc bloquear")
	if strings.Contains(plain(m), "*") || strings.Contains(plain(m), pass) || m.input != "" || m.id != nil {
		t.Fatal("secret retained")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, data) {
		t.Fatal("file changed", err)
	}
	next, quit := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if quit == nil || next.(Model).ExitStatus() != 0 || next.(Model).vlt != nil {
		t.Fatal("success exit")
	}
	next, quit = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if quit == nil || next.(Model).ExitStatus() != 0 {
		t.Fatal("success Ctrl+C")
	}
}

func TestUnlockRejectionsAndPaste(t *testing.T) {
	m, _, pass, _ := unlockFixture(t, English)
	for _, tc := range []struct{ input, message string }{
		{"zebra zero zone zoo zebra zero", "Type exactly 7 words."},
		{"zebra zero zone zoo zebra zero notaword", "One of the words is not valid. Check the spelling."},
		{"abandon abandon abandon abandon abandon abandon abandon", "Wrong passphrase, or the identity file is damaged."},
	} {
		var cmd tea.Cmd
		m, cmd = beginCheck(t, m, tc.input)
		m = finishCheck(t, m, cmd)
		requireUnlock(t, m, "Unlock Qrypt", tc.message)
		m = key(t, m, tea.KeyTab)
		if !strings.Contains(plain(m), tc.input) {
			t.Fatal("rejected field lost")
		}
		m = key(t, m, tea.KeyTab)
		for range []rune(tc.input) {
			m = key(t, m, tea.KeyBackspace)
		}
	}
	baseline := plain(m)
	m = send(t, m, tea.PasteMsg{Content: "zebra zero"})
	requireUnlock(t, m, "Pasting is not accepted. Type the words from your paper.")
	if !strings.Contains(plain(m), "\n\nWords typed: 0 of 7\n") || strings.Contains(plain(m), "zebra") || baseline == plain(m) {
		t.Fatal("paste changed field or did not show message")
	}
	m = typeText(t, m, pass[:1])
	if strings.Contains(plain(m), "Pasting is not accepted") {
		t.Fatal("message not cleared")
	}
	m = key(t, m, tea.KeyBackspace)
	m, cmd := beginCheck(t, m, pass)
	m = finishCheck(t, m, cmd)
	requireUnlock(t, m, "Spaces", "Personal (0)")
}

func TestUnlockCannotOpen(t *testing.T) {
	for _, language := range []Language{English, Spanish} {
		for _, kind := range []string{"magic", "version", "missing"} {
			t.Run(kind+string(rune('0'+language)), func(t *testing.T) {
				m, path, _, data := unlockFixture(t, language)
				altered := append([]byte(nil), data...)
				switch kind {
				case "magic":
					altered[0] ^= 0xff
				case "version":
					altered[7] = 2
				case "missing":
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
				if kind != "missing" {
					if err := os.WriteFile(path, altered, 0600); err != nil {
						t.Fatal(err)
					}
				}
				m, cmd := beginCheck(t, m, "zebra")
				m = finishCheck(t, m, cmd)
				title, cause, label := "Cannot open the identity", "The identity file is not a valid Qrypt identity.", "Identity file: "
				if language == Spanish {
					title, cause, label = "No se puede abrir la identidad", "El archivo de identidad no es una identidad válida de Qrypt.", "Archivo de identidad: "
				}
				if kind == "version" {
					cause = "This identity was created by a newer version of Qrypt."
					if language == Spanish {
						cause = "Esta identidad es de una versión más nueva de Qrypt."
					}
				}
				if kind == "missing" {
					cause = "identity"
				}
				requireUnlock(t, m, title, cause)
				if !strings.Contains(strings.ReplaceAll(plain(m), "\n", ""), label+path) {
					t.Fatal("missing path")
				}
				if kind != "missing" {
					after, err := os.ReadFile(path)
					if err != nil || !bytes.Equal(after, altered) {
						t.Fatal("file changed", err)
					}
				}
				for _, keyMsg := range []tea.KeyPressMsg{{Code: tea.KeyEnter}, {Code: 'c', Mod: tea.ModCtrl}} {
					next, quit := m.Update(keyMsg)
					if quit == nil || next.(Model).ExitStatus() != 1 {
						t.Fatal("failure exit")
					}
				}
			})
		}
	}
}

func TestUnlockSmallAndMinimumWidth(t *testing.T) {
	for _, language := range []Language{English, Spanish} {
		m, _, _, _ := unlockFixture(t, language)
		input := strings.TrimSpace(strings.Repeat("abcdefgh ", 7))
		m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
		m = typeText(t, m, input)
		for _, visible := range []bool{false, true} {
			requireUnlock(t, m)
			field := strings.Repeat("*", len(input))
			if visible {
				field = input
			}
			if !strings.Contains(strings.ReplaceAll(plain(m), "\n", ""), field) {
				t.Fatal("field did not wrap intact")
			}
			if strings.Contains(plain(m), field) {
				t.Fatal("62-character field did not wrap at 60")
			}
			for _, size := range []tea.WindowSizeMsg{{Width: 59, Height: 20}, {Width: 60, Height: 19}, {Width: 0, Height: 0}} {
				small := send(t, m, size)
				if strings.Contains(plain(small), "*") || strings.Contains(plain(small), input) || strings.Contains(plain(small), "Words typed") || strings.Contains(plain(small), "Palabras escritas") {
					t.Fatal("small window leaked")
				}
			}
			m = key(t, m, tea.KeyTab)
		}
	}
}

func TestUnlockRenderSpanish(t *testing.T) {
	m, path, pass, data := unlockFixture(t, Spanish)
	show := func(name string) {
		t.Helper()
		t.Logf("%s (80x24):\n%s", name, strings.ReplaceAll(plain(m), wrapLine("Archivo de identidad: "+path), "Archivo de identidad: <path>"))
	}
	show("Empty")
	m = typeText(t, m, "zebra zero")
	show("Hidden")
	m = key(t, m, tea.KeyTab)
	show("Visible")
	m = key(t, m, tea.KeyTab)
	for range []rune("zebra zero") {
		m = key(t, m, tea.KeyBackspace)
	}
	m, check := beginCheck(t, m, "abandon abandon abandon abandon abandon abandon abandon")
	m = finishCheck(t, m, check)
	show("Wrong passphrase")
	for range []rune("abandon abandon abandon abandon abandon abandon abandon") {
		m = key(t, m, tea.KeyBackspace)
	}
	m, check = beginCheck(t, m, pass)
	m = finishCheck(t, m, check)
	show("Qrypt vault")
	data[0] ^= 0xff
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	m = send(t, New(path, Spanish), tea.WindowSizeMsg{Width: 80, Height: 24})
	m = enter(t, m)
	m, check = beginCheck(t, m, "zebra")
	m = finishCheck(t, m, check)
	show("Wrong magic")
}

func TestUnlockQuit(t *testing.T) {
	m, _, pass, _ := unlockFixture(t, English)
	for _, keyMsg := range []tea.KeyPressMsg{{Code: tea.KeyEsc}, {Code: 'c', Mod: tea.ModCtrl}} {
		next, quit := m.Update(keyMsg)
		if quit == nil || next.(Model).ExitStatus() != 1 {
			t.Fatal("unlock quit")
		}
	}
	m, cmd := beginCheck(t, m, pass)
	next, quit := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if quit == nil || next.(Model).ExitStatus() != 1 {
		t.Fatal("waiting quit")
	}
	_ = cmd
}
