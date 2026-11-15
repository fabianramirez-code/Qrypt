package tui

import (
	"bytes"
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

func restoreFixture(t *testing.T, lang Language, existing bool) (Model, string, []string, string, []byte) {
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
	words := strings.Fields(id.RecoveryPhrase())
	var previous string
	var original []byte
	if existing {
		original, previous, err = id.Seal()
		if err != nil {
			t.Fatal(err)
		}
		if err = identity.Save(path, original); err != nil {
			t.Fatal(err)
		}
	}
	m := send(t, New(path, lang), tea.WindowSizeMsg{Width: 80, Height: 24})
	m = enter(t, m)
	return m, path, words, previous, original
}
func restoreStart(t *testing.T, m Model, existing bool) Model {
	t.Helper()
	if existing {
		m = send(t, m, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	} else {
		m = key(t, m, 'R')
	}
	if !strings.Contains(plain(m), m.language.text().restoreTitle) {
		t.Fatal(plain(m))
	}
	return enter(t, m)
}
func restoreWords(t *testing.T, m Model, words []string) (Model, tea.Cmd) {
	t.Helper()
	for i, word := range words {
		m = typeText(t, m, word)
		next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = next.(Model)
		if i == len(words)-1 {
			if cmd == nil {
				t.Fatal("no rebuild command")
			}
			return m, cmd
		}
	}
	t.Fatal("missing words")
	return m, nil
}
func restorePass(t *testing.T, m Model) (Model, string) {
	t.Helper()
	view := plain(m)
	var words []string
	for _, line := range strings.Split(view, "\n") {
		var n int
		var w string
		if _, err := fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "│")), "%d. %s", &n, &w); err == nil && n == len(words)+1 {
			words = append(words, w)
		}
	}
	if len(words) != 7 {
		t.Fatalf("new passphrase: %q", view)
	}
	return m, strings.Join(words, " ")
}
func restoreToSave(t *testing.T, m Model, words []string) (Model, string, tea.Cmd) {
	t.Helper()
	m, rebuild := restoreWords(t, m, words)
	next, _ := m.Update(rebuild())
	m = next.(Model)
	m, pass := restorePass(t, m)
	m = enter(t, m)
	m = typeText(t, m, pass)
	next, check := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	next, save := m.Update(check())
	if save == nil {
		t.Fatal("no save command")
	}
	return next.(Model), pass, save
}
func restoreFinish(t *testing.T, m Model, save tea.Cmd) Model {
	t.Helper()
	next, _ := m.Update(save())
	return next.(Model)
}
func entries(t *testing.T, path string, want int) {
	t.Helper()
	list, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != want {
		t.Fatalf("entries %d, want %d", len(list), want)
	}
}
func noRestoreSecrets(t *testing.T, m Model) {
	t.Helper()
	if m.id != nil || m.pass != "" || len(m.sealed) != 0 || len(m.words) != 0 || len(m.restoreWords) != 0 || m.input != "" {
		t.Fatal("secret retained")
	}
}

func TestRestoreNewDeviceAndForgottenPassphrase(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			m, path, words, previous, original := restoreFixture(t, English, existing)
			m = restoreStart(t, m, existing)
			m, pass, save := restoreToSave(t, m, words)
			if m.screen != saving {
				t.Fatal("not saving")
			}
			for _, k := range []tea.KeyPressMsg{{Code: 'c', Mod: tea.ModCtrl}, {Code: tea.KeyEsc}, {Code: tea.KeyEnter}} {
				next, cmd := m.Update(k)
				if cmd != nil || next.(Model).screen != saving {
					t.Fatal("saving accepted key")
				}
			}
			if existing {
				data, _ := os.ReadFile(path)
				if !bytes.Equal(data, original) {
					t.Fatal("written early")
				}
			} else {
				absence(t, path)
			}
			m = restoreFinish(t, m, save)
			if m.screen != restored || m.ExitStatus() != 0 || numbered.MatchString(plain(m)) || !strings.Contains(strings.ReplaceAll(plain(m), "\n", ""), path) {
				t.Fatal(plain(m))
			}
			noRestoreSecrets(t, m)
			restored, err := identity.Load(path, []byte(pass))
			if err != nil {
				t.Fatal(err)
			}
			seed := make([]byte, identity.SeedSize)
			for i := range seed {
				seed[i] = byte(i)
			}
			expected, _ := identity.FromSeed(seed)
			if !bytes.Equal(restored.PublicKeys().Ed25519, expected.PublicKeys().Ed25519) || !bytes.Equal(restored.PublicKeys().KEM, expected.PublicKeys().KEM) || !bytes.Equal(restored.PublicKeys().MLDSA65, expected.PublicKeys().MLDSA65) {
				t.Fatal("different keys")
			}
			if existing {
				if m.backup == "" || !strings.Contains(strings.ReplaceAll(plain(m), "\n", ""), m.backup) {
					t.Fatal("backup not displayed")
				}
				if _, err := identity.Load(path, []byte(previous)); !errors.Is(err, identity.ErrDecryption) {
					t.Fatal(err)
				}
				if _, err := identity.Load(m.backup, []byte(previous)); err != nil {
					t.Fatal(err)
				}
				entries(t, path, 2)
			} else {
				if m.backup != "" || strings.Contains(plain(m), "Previous file kept as:") {
					t.Fatal("unexpected backup")
				}
				entries(t, path, 1)
			}
			next, quit := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
			if quit == nil || next.(Model).ExitStatus() != 0 {
				t.Fatal("success exit")
			}
		})
	}
}
func TestRestoreWordEntryAndNavigation(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			m, path, words, _, _ := restoreFixture(t, English, existing)
			m = restoreStart(t, m, existing)
			m = typeText(t, m, "qzxv")
			m = enter(t, m)
			if !strings.Contains(plain(m), "That is not a valid word. Check the spelling.") || !strings.Contains(plain(m), "Word 1 of 24") || m.input != "qzxv" {
				t.Fatal(plain(m))
			}
			for range "qzxv" {
				m = key(t, m, tea.KeyBackspace)
			}
			m = typeText(t, m, "  "+strings.ToUpper(words[0])+"  ")
			m = key(t, m, tea.KeyTab)
			if !strings.Contains(plain(m), strings.ToUpper(words[0])) {
				t.Fatal("not visible")
			}
			m = key(t, m, tea.KeyTab)
			if strings.Contains(plain(m), strings.ToUpper(words[0])) {
				t.Fatal("not hidden")
			}
			m = enter(t, m)
			if m.input != "" || m.visible || !strings.Contains(plain(m), "Word 2 of 24") {
				t.Fatal(plain(m))
			}
			m = send(t, m, tea.PasteMsg{Content: "qzxv"})
			if m.input != "" || !strings.Contains(plain(m), "Pasting is not accepted. Type the words from your paper.") {
				t.Fatal(plain(m))
			}
			m = key(t, m, tea.KeyEsc)
			if m.input != "" || !strings.Contains(plain(m), "Word 1 of 24") {
				t.Fatal(plain(m))
			}
			m = key(t, m, tea.KeyEsc)
			if m.screen != restoreIntro {
				t.Fatal(plain(m))
			}
			m = key(t, m, tea.KeyEsc)
			if existing {
				if m.screen != unlock || m.input != "" {
					t.Fatal(plain(m))
				}
			} else if m.screen != welcome {
				t.Fatal(plain(m))
			}
			if !existing {
				absence(t, path)
			}
		})
	}
}
func TestRestoreInvalidChecksum(t *testing.T) {
	m, path, words, _, _ := restoreFixture(t, English, false)
	m = restoreStart(t, m, false)
	for _, candidate := range []string{"abandon", "ability", "able", "about", "above"} {
		if candidate == words[23] {
			continue
		}
		bad := append([]string(nil), words...)
		bad[23] = candidate
		if _, err := identity.FromRecoveryPhrase(strings.Join(bad, " ")); errors.Is(err, identity.ErrChecksum) {
			words = bad
			break
		}
	}
	if _, err := identity.FromRecoveryPhrase(strings.Join(words, " ")); !errors.Is(err, identity.ErrChecksum) {
		t.Fatal("no checksum mismatch")
	}
	m, cmd := restoreWords(t, m, words)
	if !strings.Contains(plain(m), "Rebuilding your identity. Please wait.") {
		t.Fatal(plain(m))
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.screen != restoreWord || m.index != 0 || len(m.restoreWords) != 0 || !strings.Contains(plain(m), "Not a valid recovery phrase. Type the 24 words again.") {
		t.Fatal(plain(m))
	}
	absence(t, path)
}
func TestRestoreQuitBeforeSave(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, stage := range []string{"intro", "word", "rebuild", "pass", "confirm"} {
			t.Run(fmt.Sprintf("%v/%s", existing, stage), func(t *testing.T) {
				m, path, words, _, original := restoreFixture(t, English, existing)
				if existing {
					m = send(t, m, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
				} else {
					m = key(t, m, 'r')
				}
				if stage != "intro" {
					m = enter(t, m)
				}
				if stage == "rebuild" || stage == "pass" || stage == "confirm" {
					var cmd tea.Cmd
					m, cmd = restoreWords(t, m, words)
					if stage != "rebuild" {
						next, _ := m.Update(cmd())
						m = next.(Model)
					}
				}
				if stage == "confirm" {
					m = enter(t, m)
				}
				next, quit := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
				m = next.(Model)
				if quit == nil || m.ExitStatus() != 1 {
					t.Fatal("quit failed")
				}
				noRestoreSecrets(t, m)
				if existing {
					data, _ := os.ReadFile(path)
					if !bytes.Equal(data, original) {
						t.Fatal("changed")
					}
					entries(t, path, 1)
				} else {
					absence(t, path)
				}
			})
		}
	}
}
func TestRestoreSaveFailureAndTranslation(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		t.Run(fmt.Sprint(lang), func(t *testing.T) {
			m, path, words, _, _ := restoreFixture(t, lang, false)
			if err := os.WriteFile(filepath.Dir(path), []byte("untouched"), 0600); err != nil {
				t.Fatal(err)
			}
			m = restoreStart(t, m, false)
			m, _, save := restoreToSave(t, m, words)
			m = restoreFinish(t, m, save)
			if m.screen != notRestored || m.ExitStatus() != 1 {
				t.Fatal(plain(m))
			}
			noRestoreSecrets(t, m)
			for _, s := range []string{m.language.text().notRestoredTitle, m.language.text().notRestoredBody, m.language.text().notRestoredOutro, m.language.text().discardNew, m.language.text().phraseStillValid} {
				if !strings.Contains(plain(m), s) {
					t.Fatal(plain(m))
				}
			}
			content, _ := os.ReadFile(filepath.Dir(path))
			if string(content) != "untouched" {
				t.Fatal("changed")
			}
			m.message = m.language.text().backupExists
			if !strings.Contains(plain(m), m.language.text().backupExists) {
				t.Fatal("translation")
			}
		})
	}
}
func TestRestoreViews(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		t.Run(fmt.Sprint(lang), func(t *testing.T) {
			m, _, words, _, _ := restoreFixture(t, lang, true)
			text := lang.text()
			check := func(m Model) {
				t.Helper()
				if !m.View().AltScreen {
					t.Fatal("alternate screen")
				}
				minimum := send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
				for _, line := range strings.Split(plain(minimum), "\n") {
					if lipgloss.Width(line) > 60 {
						t.Fatalf("wide %q", line)
					}
				}
			}
			check(m) // Unlock.
			m = send(t, m, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
			check(m) // Restore introduction.
			m = enter(t, m)
			check(m) // Recovery word.
			m = typeText(t, m, "zebra")
			for _, visible := range []bool{false, true} {
				if visible != strings.Contains(plain(m), "zebra") {
					t.Fatal("word visibility")
				}
				for _, size := range []tea.WindowSizeMsg{{Width: 59, Height: 20}, {Width: 60, Height: 19}, {Width: 0, Height: 0}} {
					small := send(t, m, size)
					if plain(small) != text.enlarge+"\n" {
						t.Fatal(plain(small))
					}
				}
				m = key(t, m, tea.KeyTab)
			}
			for range "zebra" {
				m = key(t, m, tea.KeyBackspace)
			}
			m, rebuild := restoreWords(t, m, words)
			check(m) // Rebuilding.
			next, _ := m.Update(rebuild())
			m = next.(Model)
			check(m) // New passphrase.
			m, pass := restorePass(t, m)
			m = enter(t, m)
			check(m) // Confirm passphrase.
			m = typeText(t, m, pass)
			next, opening := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = next.(Model)
			check(m) // Checking passphrase.
			next, save := m.Update(opening())
			m = next.(Model)
			check(m) // Saving.
			failed, _ := m.Update(savedMsg{err: identity.ErrBackupExists})
			check(failed.(Model)) // Not restored.
			m = restoreFinish(t, m, save)
			check(m) // Restored.
		})
	}
}

func TestRestoreExactLanguageAndRender(t *testing.T) {
	for _, language := range []Language{English, Spanish} {
		t.Run(fmt.Sprint(language), func(t *testing.T) {
			m, path, words, _, _ := restoreFixture(t, language, true)
			text := language.text()
			show := func(label string) {
				t.Helper()
				view := plain(m)
				view = strings.ReplaceAll(view, wrapLine(text.fileLabel+path), text.fileLabel+"<path>")
				if m.backup != "" {
					view = strings.ReplaceAll(view, wrapLine(text.backupLabel+m.backup), text.backupLabel+"<backup>")
				}
				view = regexp.MustCompile(`(?m)(\d+\. )\w+`).ReplaceAllString(view, "${1}<word>")
				t.Logf("%s (80x24):\n%s", label, view)
				if !m.View().AltScreen {
					t.Fatal("alternate screen")
				}
				for _, line := range strings.Split(plain(m), "\n") {
					if lipgloss.Width(line) > 60 {
						t.Fatalf("wide: %q", line)
					}
				}
			}
			m.screen = welcome
			for _, part := range []string{text.welcomeRestore, text.welcomeHint} {
				if !strings.Contains(plain(m), part) {
					t.Fatal(plain(m))
				}
			}
			show("Welcome")
			m.screen = unlock
			if !strings.Contains(plain(m), text.unlockRestoreHint) {
				t.Fatal(plain(m))
			}
			show("Unlock empty")
			m = typeText(t, m, "qzxv")
			m = send(t, m, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
			if m.input != "" || !strings.Contains(plain(m), text.restoreBackup) || !strings.Contains(plain(m), text.restoreIntro) || !strings.Contains(plain(m), text.restoreIntroHint) {
				t.Fatal(plain(m))
			}
			show("Restore introduction")
			m = enter(t, m)
			for i := 0; i < 4; i++ {
				m = typeText(t, m, words[i])
				m = enter(t, m)
			}
			m = typeText(t, m, "zebra")
			for _, visible := range []bool{false, true} {
				if !strings.Contains(plain(m), text.restoreProgress(5)+"\n"+text.restorePrompt(5)) || !strings.Contains(plain(m), text.restoreWordHint) {
					t.Fatal(plain(m))
				}
				if visible != strings.Contains(plain(m), "zebra") {
					t.Fatal("word visibility")
				}
				show(fmt.Sprintf("Word 5 visible=%t", visible))
				m = key(t, m, tea.KeyTab)
			}
			for range "zebra" {
				m = key(t, m, tea.KeyBackspace)
			}
			m = typeText(t, m, "qzxv")
			m = enter(t, m)
			if !strings.Contains(plain(m), text.invalidWord) || strings.Contains(plain(m), "qzxv") {
				t.Fatal(plain(m))
			}
			show("Rejected word")
			for range "qzxv" {
				m = key(t, m, tea.KeyBackspace)
			}
			remaining := words[4:]
			m, rebuild := restoreWords(t, m, remaining)
			if !strings.Contains(plain(m), text.rebuilding) {
				t.Fatal(plain(m))
			}
			show("Rebuilding")
			next, _ := m.Update(rebuild())
			m = next.(Model)
			if !strings.Contains(plain(m), text.newPassTitle) || strings.Contains(plain(m), "Recovery phrase") {
				t.Fatal(plain(m))
			}
			show("New passphrase")
			m, pass := restorePass(t, m)
			m = enter(t, m)
			m = typeText(t, m, "qzxv")
			m = enter(t, m)
			if !strings.Contains(plain(m), text.wordCount) {
				t.Fatal(plain(m))
			}
			m = key(t, m, tea.KeyEsc)
			if !strings.Contains(plain(m), text.newPassTitle) {
				t.Fatal(plain(m))
			}
			m = enter(t, m)
			m = typeText(t, m, pass)
			next, check := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = next.(Model)
			next, save := m.Update(check())
			m = next.(Model)
			m = restoreFinish(t, m, save)
			for _, part := range []string{text.restoredTitle, text.restoredBody, text.backupLabel, text.previousPass, text.keepNewPass, text.exitHint} {
				if !strings.Contains(plain(m), part) {
					t.Fatal(plain(m))
				}
			}
			show("Restored")
			m.screen = notRestored
			m.message = text.backupExists
			for _, part := range []string{text.notRestoredTitle, text.notRestoredBody, text.notRestoredOutro, text.discardNew, text.phraseStillValid, text.backupExists} {
				if !strings.Contains(plain(m), part) {
					t.Fatal(plain(m))
				}
			}
			show("Not restored")
		})
	}
}

func TestRestoreKnownSaveErrors(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		for _, cause := range []error{identity.ErrAlreadyExists, identity.ErrBackupExists} {
			m, _, words, _, _ := restoreFixture(t, lang, false)
			m = restoreStart(t, m, false)
			m, _, _ = restoreToSave(t, m, words)
			next, _ := m.Update(savedMsg{err: fmt.Errorf("save: %w", cause)})
			m = next.(Model)
			want := lang.text().alreadyExists
			if errors.Is(cause, identity.ErrBackupExists) {
				want = lang.text().backupExists
			}
			if m.screen != notRestored || m.ExitStatus() != 1 || !strings.Contains(plain(m), want) {
				t.Fatal(plain(m))
			}
		}
	}
}
