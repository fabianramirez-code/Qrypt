package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
var numbered = regexp.MustCompile(`(?m)\b(?:1|2|7|24)\. \w+`)

// framed is the screen exactly as drawn, without color sequences.
func framed(m Model) string { return ansi.ReplaceAllString(m.View().Content, "") }

// plain is the screen's text without the frame: logo, title line, panel borders and bottom bar.
// Content assertions use it; the frame itself is asserted through framed.
func plain(m Model) string {
	logoRows := map[string]bool{"◆ Qrypt": true}
	for _, row := range strings.Split(logo, "\n") {
		logoRows[strings.TrimRight(row, " \r")] = true
	}
	var out []string
	header := false
	for _, line := range strings.Split(framed(m), "\n") {
		if logoRows[line] && line != "" {
			continue
		}
		if r := []rune(line); len(r) > 0 && strings.ContainsRune("╭╰├┏┗┣", r[0]) {
			header = r[0] == '╭' || r[0] == '┏'
			continue
		}
		for _, edge := range []string{"│", "┃"} {
			if strings.HasPrefix(line, edge+" ") && strings.HasSuffix(line, edge) {
				line = strings.TrimRight(strings.TrimSuffix(strings.TrimPrefix(line, edge+" "), edge), " ")
				if header {
					line = strings.TrimSpace(line) // The title is centered in its cell.
				}
				break
			}
		}
		header = false
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
func fixture(t *testing.T) (Model, string, []string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config", "identity")
	seed := make([]byte, identity.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	id, err := identity.FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	m := New(path, English)
	m = send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m.generate = func() (*identity.Identity, error) { return id, nil }
	m.pick = func() [3]int { return [3]int{2, 11, 23} }
	words := strings.Fields(string(id.RecoveryPhrase()))
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return m, path, words
}
func send(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, cmd := m.Update(msg)
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}
	return m
}
func key(t *testing.T, m Model, code rune) Model { return send(t, m, tea.KeyPressMsg{Code: code}) }
func typeText(t *testing.T, m Model, s string) Model {
	return send(t, m, tea.KeyPressMsg{Code: tea.KeyExtended, Text: s})
}
func enter(t *testing.T, m Model) Model { return key(t, m, tea.KeyEnter) }
func ctrlC(t *testing.T, m Model) Model {
	return send(t, m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
}
func phrase(t *testing.T, m Model) (Model, string) {
	t.Helper()
	m = enter(t, m)
	view := plain(m)
	words := make([]string, 0, 7)
	for _, line := range strings.Split(view, "\n") {
		var n int
		var w string
		if _, err := fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "│")), "%d. %s", &n, &w); err == nil && n == len(words)+1 {
			words = append(words, w)
		}
	}
	if len(words) != 7 {
		t.Fatalf("passphrase numbered words: %q", view)
	}
	return m, strings.Join(words, " ")
}
func toRecovery(t *testing.T, m Model) (Model, string) {
	t.Helper()
	m, pass := phrase(t, m)
	m = enter(t, m)
	m = typeText(t, m, pass)
	m = enter(t, m)
	if !strings.Contains(plain(m), "Recovery phrase") {
		t.Fatalf("recovery view: %q", plain(m))
	}
	return m, pass
}
func toCheck(t *testing.T, m Model) (Model, string) {
	t.Helper()
	m, pass := toRecovery(t, m)
	return enter(t, m), pass
}
func absence(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file exists: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory exists: %v", err)
	}
}

func TestLanguageSelection(t *testing.T) {
	for _, tc := range []struct {
		preferred Language
		marker    string
		welcome   string
	}{{English, "> English", "Creating an identity"}, {Spanish, "> Español", "Crear una identidad"}} {
		t.Run(tc.marker, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config", "identity")
			m := send(t, New(path, tc.preferred), tea.WindowSizeMsg{Width: 80, Height: 24})
			if !m.View().AltScreen || !strings.Contains(plain(m), "LANGUAGE / IDIOMA") || !strings.Contains(plain(m), tc.marker) || !strings.Contains(plain(m), "←/→/Tab: list / lista") || !strings.Contains(plain(m), "Enter: select / elegir   Esc: quit / salir") {
				t.Fatal(plain(m))
			}
			absence(t, path)
			if tc.preferred == English {
				m = key(t, m, tea.KeyUp)
				if !strings.Contains(plain(m), "> English") {
					t.Fatal("up wrapped")
				}
				m = key(t, m, tea.KeyDown)
				if !strings.Contains(plain(m), "> Español") {
					t.Fatal("down did not move")
				}
				m = key(t, m, tea.KeyDown)
				if !strings.Contains(plain(m), "> Español") {
					t.Fatal("down wrapped")
				}
				m = key(t, m, tea.KeyUp)
			} else {
				m = key(t, m, tea.KeyDown)
				if !strings.Contains(plain(m), "> Español") {
					t.Fatal("down wrapped")
				}
				m = key(t, m, tea.KeyUp)
				m = key(t, m, tea.KeyDown)
			}
			m = enter(t, m)
			if !strings.Contains(plain(m), tc.welcome) {
				t.Fatal(plain(m))
			}
			absence(t, path)
		})
	}
}

func TestLanguageQuit(t *testing.T) {
	for _, code := range []tea.KeyPressMsg{{Code: tea.KeyEsc}, {Code: 'c', Mod: tea.ModCtrl}} {
		path := filepath.Join(t.TempDir(), "config", "identity")
		m := New(path, English)
		next, cmd := m.Update(code)
		if cmd == nil || next.(Model).ExitStatus() != 1 {
			t.Fatalf("quit key %v", code)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatal("not a quit command")
		}
		absence(t, path)
	}
}

func TestLanguageTag(t *testing.T) {
	for _, tc := range []struct {
		tag  string
		want Language
	}{{"es", Spanish}, {"es-MX", Spanish}, {"es_ES.UTF-8", Spanish}, {"ES-ar", Spanish}, {"es.UTF-8", Spanish}, {"", English}, {"C", English}, {"en-US", English}, {"en_GB.UTF-8", English}, {"pt-BR", English}, {"est", English}, {"eslovaco", English}} {
		if got := FromTag(tc.tag); got != tc.want {
			t.Errorf("%q: got %v, want %v", tc.tag, got, tc.want)
		}
	}
}

func TestFullFlow(t *testing.T) {
	m, path, words := fixture(t)
	capture := func(step int, view string) {
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > 60 {
				t.Fatalf("step %d exceeds 60 columns: %q", step, line)
			}
		}
		t.Logf("Step %d (80x24):\n%s", step, strings.ReplaceAll(view, wrapLine("Identity file: "+path), "Identity file: <path>"))
	}
	capture(1, plain(m))
	for _, text := range []string{"Creating an identity", "Qrypt will give you two secrets.", "Passphrase (7 words)", "You type it every time you open Qrypt on this device.", "Recovery phrase (24 words)", "It rebuilds your identity if you lose this device", "or forget the passphrase.", "Write both on paper. They are shown only now,", "and nobody can reset them.", "Enter: create   R: restore   Esc: quit"} {
		if !strings.Contains(plain(m), text) {
			t.Fatal(plain(m))
		}
	}
	m, pass := phrase(t, m)
	capture(2, regexp.MustCompile(`(?m)(\d+\. )\w+`).ReplaceAllString(plain(m), "${1}<word>"))
	for _, text := range []string{"Passphrase", "Write these 7 words on paper, in order.", "You will type them on the next screen."} {
		if !strings.Contains(plain(m), text) {
			t.Fatal(plain(m))
		}
	}
	m = enter(t, m)
	capture(3, plain(m))
	if !strings.Contains(plain(m), "Confirm passphrase") {
		t.Fatal(plain(m))
	}
	m = typeText(t, m, pass)
	m = enter(t, m)
	view := plain(m)
	capture(4, view)
	for _, text := range []string{"Recovery phrase", "Write these 24 words on paper, in order.", "Keep the paper somewhere safe, away from this device.", "Do not store it in a photo, a note or a file."} {
		if !strings.Contains(view, text) {
			t.Fatal(view)
		}
	}
	for i, w := range words {
		if !strings.Contains(view, fmt.Sprintf("%d. %s", i+1, w)) {
			t.Fatalf("missing recovery %d", i+1)
		}
	}
	m = enter(t, m)
	capture(5, plain(m))
	if !strings.Contains(plain(m), "Confirm recovery phrase") || !strings.Contains(plain(m), "Word 1 of 3") {
		t.Fatal(plain(m))
	}
	for _, p := range []int{2, 11} {
		m = typeText(t, m, words[p])
		m = enter(t, m)
	}
	m = typeText(t, m, words[23])
	next, save := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	capture(6, plain(m))
	if !strings.Contains(plain(m), "Saving") || save == nil {
		t.Fatal(plain(m))
	}
	next, _ = m.Update(save())
	m = next.(Model)
	capture(7, plain(m))
	if !strings.Contains(plain(m), "Done") || !strings.Contains(strings.ReplaceAll(plain(m), "\n", ""), path) || !strings.Contains(plain(m), "Keep both papers safe.") || numbered.MatchString(plain(m)) {
		t.Fatal(plain(m))
	}
	loaded, err := identity.Load(path, []byte(pass))
	if err != nil || string(loaded.RecoveryPhrase()) != strings.Join(words, " ") {
		t.Fatal(err)
	}
	m = enter(t, m)
	if m.ExitStatus() != 0 {
		t.Fatal(m.ExitStatus())
	}
}
func TestSavingIgnoresKeysUntilResult(t *testing.T) {
	m, path, words := fixture(t)
	m, pass := toCheck(t, m)
	for _, position := range []int{2, 11} {
		m = typeText(t, m, words[position])
		m = enter(t, m)
	}
	m = typeText(t, m, words[23])
	next, save := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if save == nil || m.screen != saving || m.ExitStatus() != 1 {
		t.Fatal("save did not start")
	}
	absence(t, path)
	for _, pressed := range []tea.KeyPressMsg{{Code: 'c', Mod: tea.ModCtrl}, {Code: tea.KeyEnter}, {Code: tea.KeyEsc}, {Code: tea.KeyExtended, Text: "x"}} {
		before := m.View().Content
		next, cmd := m.Update(pressed)
		m = next.(Model)
		if cmd != nil || m.screen != saving || m.ExitStatus() != 1 || m.View().Content != before {
			t.Fatalf("key %v changed saving state or quit", pressed)
		}
		absence(t, path)
	}
	next, cmd := m.Update(save())
	m = next.(Model)
	if cmd != nil || m.screen != done || !strings.Contains(plain(m), "Done") || m.ExitStatus() != 0 {
		t.Fatal("save result did not show Done")
	}
	loaded, err := identity.Load(path, []byte(pass))
	if err != nil || string(loaded.RecoveryPhrase()) != strings.Join(words, " ") {
		t.Fatalf("saved identity does not open: %v", err)
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil || next.(Model).ExitStatus() != 0 {
		t.Fatal("Ctrl+C on Done did not exit successfully")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Done did not quit")
	}
}

func TestCtrlCWhileGeneratingOrChecking(t *testing.T) {
	for _, state := range []screen{waitingSeal, waitingOpen} {
		t.Run(fmt.Sprint(state), func(t *testing.T) {
			m, path, _ := fixture(t)
			var pending tea.Cmd
			if state == waitingSeal {
				next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				m, pending = next.(Model), cmd
			} else {
				var pass string
				m, pass = phrase(t, m)
				m = enter(t, m)
				m = typeText(t, m, pass)
				next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				m, pending = next.(Model), cmd
			}
			if pending == nil || m.screen != state {
				t.Fatal("waiting command not returned")
			}
			next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
			m = next.(Model)
			if cmd == nil || m.ExitStatus() != 1 {
				t.Fatal("Ctrl+C did not quit waiting state with status 1")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("Ctrl+C did not return QuitMsg")
			}
			absence(t, path)
		})
	}
}

func TestTypedTextFitsMinimumWindow(t *testing.T) {
	input := strings.TrimSpace(strings.Repeat("abcdefgh ", 7))
	if len(input) != 62 {
		t.Fatal("test input is not 62 characters")
	}
	for _, step := range []int{3, 5} {
		t.Run(fmt.Sprint(step), func(t *testing.T) {
			m, _, _ := fixture(t)
			if step == 3 {
				m, _ = phrase(t, m)
				m = enter(t, m)
			} else {
				m, _ = toCheck(t, m)
			}
			m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
			m = typeText(t, m, input)
			view := plain(m)
			prefix := "Type the 7 words in order (visible):\n"
			if step == 5 {
				prefix = "Type word 3:\n"
			}
			field := strings.SplitN(view, prefix, 2)
			if len(field) != 2 {
				t.Fatalf("missing input field: %q", view)
			}
			visible := strings.SplitN(field[1], "\n\nEnter: check", 2)
			if len(visible) != 2 || strings.ReplaceAll(visible[0], "\n", "") != input {
				t.Fatalf("typed text not fully visible: %q", view)
			}
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > 60 {
					t.Fatalf("line exceeds 60 columns: %q", line)
				}
			}
		})
	}
}

func TestQuitBeforeSave(t *testing.T) {
	for step := 1; step <= 5; step++ {
		t.Run(fmt.Sprint(step), func(t *testing.T) {
			m, path, words := fixture(t)
			if step >= 2 {
				m, _ = phrase(t, m)
			}
			if step >= 3 {
				m = enter(t, m)
			}
			if step >= 4 { // use displayed passphrase from separate flow instead
				m, path, words = fixture(t)
				var pass string
				m, pass = phrase(t, m)
				m = enter(t, m)
				m = typeText(t, m, pass)
				m = enter(t, m)
			}
			if step >= 5 {
				m = enter(t, m)
			}
			_ = words
			m = ctrlC(t, m)
			if m.ExitStatus() != 1 {
				t.Fatal(m.ExitStatus())
			}
			absence(t, path)
		})
	}
	m, path, _ := fixture(t)
	m = key(t, m, tea.KeyEsc)
	if m.ExitStatus() != 1 {
		t.Fatal(m.ExitStatus())
	}
	absence(t, path)
}
func TestPassphraseErrors(t *testing.T) {
	m, path, _ := fixture(t)
	m, pass := phrase(t, m)
	m = enter(t, m)
	wrong := strings.Fields(pass)
	if wrong[0] == "abandon" {
		wrong[0] = "ability"
	} else {
		wrong[0] = "abandon"
	}
	for _, tc := range []struct{ input, message string }{{"abandon " + strings.Repeat("abandon ", 5), "Type exactly 7 words."}, {"notaword " + strings.Repeat("abandon ", 6), "One of the words is not valid. Check the spelling."}, {strings.Join(wrong, " "), "That is not the passphrase shown. Check your paper."}} {
		m = typeText(t, m, tc.input)
		m = enter(t, m)
		if !strings.Contains(plain(m), tc.message) || !strings.Contains(plain(m), "Confirm passphrase") {
			t.Fatal(plain(m))
		}
		absence(t, path)
		for range []rune(tc.input) {
			m = key(t, m, tea.KeyBackspace)
		}
	}
	m = key(t, m, tea.KeyEsc)
	if !strings.Contains(plain(m), "Passphrase") {
		t.Fatal(plain(m))
	}
	m = enter(t, m)
	m = typeText(t, m, pass)
	m = enter(t, m)
	if !strings.Contains(plain(m), "Recovery phrase") {
		t.Fatal(plain(m))
	}
}
func TestRecoveryErrors(t *testing.T) {
	m, path, words := fixture(t)
	m, _ = toCheck(t, m)
	if words[2] == words[11] {
		t.Fatal("fixed recovery positions have equal words")
	}
	for _, input := range []string{words[11], "", "   "} {
		m = typeText(t, m, input)
		m = enter(t, m)
		if !strings.Contains(plain(m), "That is not word 3. Check your paper.") || !strings.Contains(plain(m), "Word 1 of 3\nType word 3:") {
			t.Fatal(plain(m))
		}
		absence(t, path)
	}
	m = typeText(t, m, "wrong")
	m = enter(t, m)
	if !strings.Contains(plain(m), "That is not word 3. Check your paper.") {
		t.Fatal(plain(m))
	}
	absence(t, path)
	m = key(t, m, tea.KeyEsc)
	if !strings.Contains(plain(m), "Recovery phrase") {
		t.Fatal(plain(m))
	}
	m = enter(t, m)
	for _, p := range []int{2, 11, 23} {
		m = typeText(t, m, "  "+strings.ToUpper(words[p])+"  ")
		m = enter(t, m)
	}
	if !strings.Contains(plain(m), "Done") {
		t.Fatal(plain(m))
	}
}
func TestSaveFailure(t *testing.T) {
	m, path, words := fixture(t)
	m, _ = toCheck(t, m)
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
	v := plain(m)
	if numbered.MatchString(v) {
		t.Fatalf("not-saved screen shows a numbered word list: %q", v)
	}
	for _, line := range strings.Split(v, "\n") {
		if lipgloss.Width(line) > 60 {
			t.Fatalf("step 8 exceeds 60 columns: %q", line)
		}
	}
	t.Logf("Step 8 (80x24):\n%s", v)
	for _, s := range []string{"Not saved", "Qrypt could not save your identity:", "An identity file already exists.", "Nothing was saved", "The words you wrote down belong to no identity.", "Discard them."} {
		if !strings.Contains(v, s) {
			t.Fatal(v)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "untouched" {
		t.Fatal(err)
	}
	m = enter(t, m)
	if m.ExitStatus() != 1 {
		t.Fatal(m.ExitStatus())
	}
}
func TestCtrlCExitStatuses(t *testing.T) {
	check := func(label string, m Model, want int) {
		t.Helper()
		next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if cmd == nil {
			t.Fatalf("%s: Ctrl+C did not quit", label)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("%s: Ctrl+C did not return QuitMsg", label)
		}
		if got := next.(Model).ExitStatus(); got != want {
			t.Fatalf("%s: status %d, want %d", label, got, want)
		}
	}
	m, path, words := fixture(t)
	m, pass := toCheck(t, m)
	for _, position := range []int{2, 11, 23} {
		m = typeText(t, m, words[position])
		m = enter(t, m)
	}
	if !strings.Contains(plain(m), "Done") {
		t.Fatal(plain(m))
	}
	check("done", m, 0)
	loaded, err := identity.Load(path, []byte(pass))
	if err != nil || string(loaded.RecoveryPhrase()) != strings.Join(words, " ") {
		t.Fatalf("saved identity does not open: %v", err)
	}
	check("unlock", enter(t, New(path, English)), 1)

	failed, failedPath, failedWords := fixture(t)
	failed, _ = toCheck(t, failed)
	if err := os.MkdirAll(filepath.Dir(failedPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(failedPath, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, position := range []int{2, 11, 23} {
		failed = typeText(t, failed, failedWords[position])
		failed = enter(t, failed)
	}
	if !strings.Contains(plain(failed), "Not saved") {
		t.Fatal(plain(failed))
	}
	check("not saved", failed, 1)
}

func TestPastePassphrase(t *testing.T) {
	m, path, _ := fixture(t)
	m, pass := phrase(t, m)
	m = enter(t, m)
	baseline := plain(m)
	const message = "Pasting is not accepted. Type the words from your paper."
	m = send(t, m, tea.PasteMsg{Content: pass})
	want := strings.Replace(baseline, "\n\nEnter: check", "\n"+message+"\nEnter: check", 1)
	if plain(m) != want || m.ExitStatus() != 1 {
		t.Fatalf("paste changed passphrase field: %q", plain(m))
	}
	absence(t, path)
	m = typeText(t, m, pass[:1])
	if plain(m) != strings.Replace(baseline, "visible):\n", "visible):\n"+pass[:1], 1) {
		t.Fatal("typing did not dismiss paste message")
	}
	partial := plain(m)
	m = send(t, m, tea.PasteMsg{Content: pass})
	if plain(m) != strings.Replace(partial, "\n\nEnter: check", "\n"+message+"\nEnter: check", 1) {
		t.Fatal("paste changed a nonempty passphrase field")
	}
	m = typeText(t, m, pass[1:])
	if strings.Contains(plain(m), message) {
		t.Fatal("typing did not dismiss paste message")
	}
	m = enter(t, m)
	if !strings.Contains(plain(m), "Recovery phrase") {
		t.Fatal(plain(m))
	}
	absence(t, path)
}

func TestPasteRecovery(t *testing.T) {
	m, path, words := fixture(t)
	m, _ = toCheck(t, m)
	baseline := plain(m)
	const message = "Pasting is not accepted. Type the words from your paper."
	m = send(t, m, tea.PasteMsg{Content: words[2]})
	want := strings.Replace(baseline, "\n\nEnter: check", "\n"+message+"\nEnter: check", 1)
	if plain(m) != want || m.ExitStatus() != 1 {
		t.Fatalf("paste changed recovery field or position: %q", plain(m))
	}
	absence(t, path)
	m = typeText(t, m, words[2][:1])
	if plain(m) != strings.Replace(baseline, "Type word 3:\n", "Type word 3:\n"+words[2][:1], 1) {
		t.Fatal("typing did not dismiss paste message")
	}
	partial := plain(m)
	m = send(t, m, tea.PasteMsg{Content: words[2]})
	if plain(m) != strings.Replace(partial, "\n\nEnter: check", "\n"+message+"\nEnter: check", 1) {
		t.Fatal("paste changed a nonempty recovery field")
	}
	m = typeText(t, m, words[2][1:])
	if strings.Contains(plain(m), message) {
		t.Fatal("typing did not dismiss paste message")
	}
	m = enter(t, m)
	if !strings.Contains(plain(m), "Word 2 of 3\nType word 12:") {
		t.Fatal(plain(m))
	}
	absence(t, path)
}

func TestPasteOtherScreens(t *testing.T) {
	m, _, words := fixture(t)
	m.phrase.Set(strings.Join(words, " "))
	m.pass.Set("abandon abandon abandon abandon abandon abandon abandon")
	for _, screen := range []screen{welcome, waitingSeal, passphrase, waitingOpen, recovery, saving, done, notSaved, vaultList, cannotOpen} {
		m.screen = screen
		before := m.View().Content
		next, cmd := m.Update(tea.PasteMsg{Content: "ignored"})
		m = next.(Model)
		if cmd != nil || m.screen != screen || m.View().Content != before || strings.Contains(plain(m), "Pasting is not accepted.") {
			t.Fatalf("paste changed screen %d", screen)
		}
	}
}

func TestAltScreen(t *testing.T) {
	m, path, words := fixture(t)
	check := func(name string) {
		t.Helper()
		if !m.View().AltScreen {
			t.Fatalf("%s is not on the alternate screen", name)
		}
	}
	check("welcome")
	m = send(t, m, tea.WindowSizeMsg{Width: 59, Height: 19})
	check("small window")
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	next, seal := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	check("sealing")
	next, _ = m.Update(seal())
	m = next.(Model)
	check("passphrase")
	var pass []string
	for _, line := range strings.Split(plain(m), "\n") {
		var n int
		var word string
		if _, err := fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "│")), "%d. %s", &n, &word); err == nil && n == len(pass)+1 {
			pass = append(pass, word)
		}
	}
	if len(pass) != 7 {
		t.Fatal("passphrase not visible")
	}
	m = enter(t, m)
	check("confirm passphrase")
	m = typeText(t, m, strings.Join(pass, " "))
	next, open := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	check("opening")
	next, _ = m.Update(open())
	m = next.(Model)
	check("recovery phrase")
	m = enter(t, m)
	check("confirm recovery")
	for _, p := range []int{2, 11} {
		m = typeText(t, m, words[p])
		m = enter(t, m)
	}
	m = typeText(t, m, words[23])
	next, save := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	check("saving")
	failed := m
	next, _ = m.Update(save())
	m = next.(Model)
	check("done")
	next, _ = failed.Update(save())
	failed = next.(Model)
	if !failed.View().AltScreen || !strings.Contains(plain(failed), "Not saved") {
		t.Fatal("not saved has no alternate screen")
	}
	m = New(path, English)
	check("language")
	m = enter(t, m)
	check("unlock")
}

func TestWindowSize(t *testing.T) {
	m, _, _ := fixture(t)
	m.width = 0
	m.height = 0
	for _, sz := range []tea.WindowSizeMsg{{Width: 0, Height: 0}, {Width: 59, Height: 20}, {Width: 60, Height: 19}} {
		m = send(t, m, sz)
		if !strings.Contains(plain(m), "Enlarge") || numbered.MatchString(plain(m)) {
			t.Fatal(plain(m))
		}
	}
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = phrase(t, m)
	for _, sz := range []tea.WindowSizeMsg{{Width: 59, Height: 20}, {Width: 60, Height: 19}} {
		m = send(t, m, sz)
		if !strings.Contains(plain(m), "Enlarge") || numbered.MatchString(plain(m)) {
			t.Fatal(plain(m))
		}
	}
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if !numbered.MatchString(plain(m)) {
		t.Fatal(plain(m))
	}
	m, _, _ = fixture(t)
	m, _ = toRecovery(t, m)
	m = send(t, m, tea.WindowSizeMsg{Width: 59, Height: 19})
	if numbered.MatchString(plain(m)) {
		t.Fatal(plain(m))
	}
	m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
	t.Logf("Step 4 (60x20):\n%s", plain(m))
	if !numbered.MatchString(plain(m)) {
		t.Fatal(plain(m))
	}
	for _, line := range strings.Split(plain(m), "\n") {
		if lipgloss.Width(line) > 60 {
			t.Fatalf("line exceeds 60 columns: %q", line)
		}
	}
}
