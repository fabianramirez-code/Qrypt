package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
)

func spanishFixture(t *testing.T) (Model, string, []string) {
	t.Helper()
	m, path, words := fixture(t)
	m = New(path, Spanish)
	m.generate = func() (*identity.Identity, error) {
		seed := make([]byte, identity.SeedSize)
		for i := range seed {
			seed[i] = byte(i)
		}
		return identity.FromSeed(seed)
	}
	m.pick = func() [3]int { return [3]int{2, 11, 23} }
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return enter(t, m), path, words
}

func requireView(t *testing.T, m Model, pieces ...string) {
	t.Helper()
	for _, piece := range pieces {
		if !strings.Contains(plain(m), piece) {
			t.Fatalf("missing %q in %q", piece, plain(m))
		}
	}
	for _, line := range strings.Split(plain(m), "\n") {
		if lipgloss.Width(line) > 60 {
			t.Fatalf("wide line: %q", line)
		}
	}
}

func TestMinimumWidthAllScreens(t *testing.T) {
	for _, language := range []Language{English, Spanish} {
		m, _, words := fixture(t)
		m.language = language
		m.words = words
		m.pass = strings.Join(words[:7], " ")
		m.positions = [3]int{2, 11, 23}
		m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
		for state := languageScreen; state <= cannotOpen; state++ {
			m.screen = state
			requireView(t, m)
		}
	}
}

func TestSpanishFlow(t *testing.T) {
	m, path, words := spanishFixture(t)
	requireView(t, m, "Crear una identidad", "Qrypt te dará dos secretos.", "Las palabras están en inglés. Cópialas tal cual.")
	t.Logf("Welcome:\n%s", plain(m))
	next, seal := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	requireView(t, m, "Crear una identidad", "Preparando tu identidad. Espera un momento.")
	next, _ = m.Update(seal())
	m = next.(Model)
	requireView(t, m, "Frase de contraseña", "Anota estas 7 palabras en papel, en orden.")
	t.Logf("Passphrase:\n%s", regexp.MustCompile(`(?m)(\d+\. )\w+`).ReplaceAllString(plain(m), "${1}<word>"))
	// Read the seven displayed words through the same public view seam as the English tests.
	var pass []string
	for _, line := range strings.Split(plain(m), "\n") {
		var n int
		var word string
		if _, err := fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "│")), "%d. %s", &n, &word); err == nil && n == len(pass)+1 {
			pass = append(pass, word)
		}
	}
	if len(pass) != 7 {
		t.Fatal("seven words not visible")
	}
	m = enter(t, m)
	requireView(t, m, "Confirmar frase de contraseña", "Escribe las 7 palabras en orden (visibles):")
	m = typeText(t, m, strings.Join(pass, " "))
	next, open := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	requireView(t, m, "Confirmar frase de contraseña", "Comprobando tu frase de contraseña. Espera un momento.")
	next, _ = m.Update(open())
	m = next.(Model)
	requireView(t, m, "Frase de recuperación", "Anota estas 24 palabras en papel, en orden.", "Guarda el papel en un lugar seguro, lejos de este equipo.")
	t.Logf("Recovery:\n%s", plain(m))
	englishModel, _, _ := fixture(t)
	englishModel, _ = toRecovery(t, englishModel)
	for i, w := range words {
		numbered := strconv.Itoa(i+1) + ". " + w
		requireView(t, m, numbered)
		requireView(t, englishModel, numbered)
	}
	m = enter(t, m)
	requireView(t, m, "Confirmar frase de recuperación", "Palabra 1 de 3", "Escribe la palabra 3:")
	t.Logf("Confirm recovery:\n%s", plain(m))
	for _, p := range []int{2, 11} {
		m = typeText(t, m, words[p])
		m = enter(t, m)
	}
	m = typeText(t, m, words[23])
	next, save := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	requireView(t, m, "Guardando", "Guardando tu identidad. Espera un momento.")
	next, _ = m.Update(save())
	m = next.(Model)
	requireView(t, m, "Listo", "Tu identidad está lista.", "Archivo de identidad:", "Guarda bien los dos papeles.")
	t.Logf("Done:\n%s", strings.ReplaceAll(plain(m), wrapLine("Archivo de identidad: "+path), "Archivo de identidad: <path>"))
	loaded, err := identity.Load(path, []byte(strings.Join(pass, " ")))
	if err != nil || loaded.RecoveryPhrase() != strings.Join(words, " ") {
		t.Fatalf("saved identity: %v", err)
	}
}

func TestSpanishErrorsAndOtherScreens(t *testing.T) {
	m, path, words := spanishFixture(t)
	m, pass := phrase(t, m)
	m = enter(t, m)
	wrong := strings.Fields(pass)
	if wrong[0] == "abandon" {
		wrong[0] = "ability"
	} else {
		wrong[0] = "abandon"
	}
	for _, tc := range []struct{ input, message string }{{"abandon", "Escribe exactamente 7 palabras."}, {"notaword " + strings.Repeat("abandon ", 6), "Una de las palabras no es válida. Revisa cómo está escrita."}, {strings.Join(wrong, " "), "Esa no es la frase de contraseña mostrada. Revisa tu papel."}} {
		m = typeText(t, m, tc.input)
		m = enter(t, m)
		requireView(t, m, tc.message)
		absence(t, path)
	}
	m = send(t, m, tea.PasteMsg{Content: pass})
	requireView(t, m, "No se acepta pegar. Escribe las palabras de tu papel.")
	m = typeText(t, m, pass)
	m = enter(t, m)
	requireView(t, m, "Frase de recuperación")
	m = enter(t, m)
	m = typeText(t, m, "wrong")
	m = enter(t, m)
	requireView(t, m, "Esa no es la palabra 3. Revisa tu papel.")
	m = send(t, m, tea.PasteMsg{Content: words[2]})
	requireView(t, m, "No se acepta pegar. Escribe las palabras de tu papel.")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []int{2, 11, 23} {
		m = typeText(t, m, words[p])
		m = enter(t, m)
	}
	requireView(t, m, "No se guardó", "Qrypt no pudo guardar tu identidad:", "Ya existe un archivo de identidad.", "No se guardó nada.", "Deséchalas.")
	t.Logf("Not saved:\n%s", plain(m))
	existing := send(t, New(path, Spanish), tea.WindowSizeMsg{Width: 80, Height: 24})
	existing = enter(t, existing)
	requireView(t, existing, "Abrir Qrypt", "Palabras escritas: 0 de 7")
	t.Logf("Unlock:\n%s", plain(existing))
	small := send(t, existing, tea.WindowSizeMsg{Width: 59, Height: 19})
	requireView(t, small, "Agranda la ventana a 60 columnas por 20 filas como mínimo.")
	if !strings.Contains(strings.ReplaceAll(plain(send(t, New(path, Spanish), tea.WindowSizeMsg{Width: 59, Height: 19})), "\n", ""), "Enlarge the window to at least 60 by 20. / Agranda la ventana a 60 por 20 como mínimo.") {
		t.Fatal("bilingual small view")
	}
}
