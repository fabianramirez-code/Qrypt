package tui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

func vaultSession(t *testing.T, language Language) (Model, string, string) {
	t.Helper()
	m, path, pass, _ := unlockFixture(t, language)
	dir := filepath.Join(filepath.Dir(path), "vault")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("vault exists before unlock: %v", err)
	}
	m, cmd := beginCheck(t, m, pass)
	m = finishCheck(t, m, cmd)
	m = enter(t, m) // Unlock now lands on Spaces; open Personal for item tests.
	if m.ExitStatus() != 0 || !strings.Contains(plain(m), map[Language]string{English: "Nothing stored yet", Spanish: "Aún no hay nada guardado"}[language]) {
		t.Fatalf("not empty vault list: %q", plain(m))
	}
	if _, err := os.Stat(filepath.Join(dir, "scopes", m.vlt.ID(), "scope")); err != nil {
		t.Fatalf("vault not created: %v", err)
	}
	return m, path, pass
}

func addVaultItem(t *testing.T, m Model, name, value string) Model {
	t.Helper()
	m = key(t, m, 'a')
	m = send(t, m, tea.PasteMsg{Content: name})
	m = enter(t, m)
	m = typeText(t, m, "value")
	m = enter(t, m)
	if m.language == Spanish {
		m = key(t, m, 'o')
	} else {
		m = key(t, m, 'h')
	}
	if !strings.Contains(plain(m), "Secret (hidden)") && !strings.Contains(plain(m), "Secreto (oculto)") {
		t.Fatalf("not in value field: %q", plain(m))
	}
	m = send(t, m, tea.PasteMsg{Content: value})
	m = enter(t, m)
	if !strings.Contains(plain(m), name) || strings.Contains(plain(m), value) {
		t.Fatalf("list leaked or lost item: %q", plain(m))
	}
	return m
}

// vaultFiles records actual persisted bytes, including the scope file, without
// relying on opaque secret-file names or any vault implementation detail.
func vaultFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "scopes"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one scope: %v %v", entries, err)
	}
	dir = filepath.Join(dir, "scopes", entries[0].Name())
	files := make(map[string][]byte)
	for _, subdir := range []string{"", "secrets"} {
		entries, err := os.ReadDir(filepath.Join(dir, subdir))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := filepath.Join(subdir, entry.Name())
			files[name], err = os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	return files
}

func sameVaultFiles(t *testing.T, dir string, before map[string][]byte) {
	t.Helper()
	after := vaultFiles(t, dir)
	if len(after) != len(before) {
		t.Fatalf("vault file count changed: %d -> %d", len(before), len(after))
	}
	for name, data := range before {
		if !bytes.Equal(data, after[name]) {
			t.Fatalf("vault file changed: %s", name)
		}
	}
}

func requireVaultView(t *testing.T, m Model) {
	t.Helper()
	if !m.View().AltScreen {
		t.Fatal("not alternate screen")
	}
	for _, line := range strings.Split(plain(m), "\n") {
		if lipgloss.Width(line) > 60 {
			t.Fatalf("vault view exceeds 60 columns: %q", line)
		}
	}
}

func TestVaultAddShowEditAndDelete(t *testing.T) {
	m, path, pass := vaultSession(t, English)
	dir := filepath.Join(filepath.Dir(path), "vault")
	m = addVaultItem(t, m, "account", "private-token")
	m = enter(t, m)
	if !strings.Contains(plain(m), "••••••") || strings.Contains(plain(m), "private-token") {
		t.Fatalf("item value not hidden: %q", plain(m))
	}
	m = key(t, m, tea.KeyTab)
	if !strings.Contains(plain(m), "private-token") {
		t.Fatal("Tab did not reveal value")
	}
	m = key(t, m, tea.KeyTab)
	if strings.Contains(plain(m), "private-token") {
		t.Fatal("second Tab did not hide value")
	}
	m = key(t, m, tea.KeyTab)
	m = key(t, m, tea.KeyEsc)
	if strings.Contains(plain(m), "private-token") {
		t.Fatal("list leaked value")
	}
	m = enter(t, m)
	if strings.Contains(plain(m), "private-token") || !strings.Contains(plain(m), "••••••") {
		t.Fatal("reopened item did not hide value")
	}
	m = key(t, m, 'e')
	m = send(t, m, tea.PasteMsg{Content: "replacement"})
	if strings.Contains(plain(m), "replacement") {
		t.Fatal("edit field revealed value")
	}
	m = enter(t, m)
	id, err := identity.Load(path, []byte(pass))
	if err != nil {
		t.Fatal(err)
	}
	opened, err := vault.Open(filepath.Join(dir, "scopes", m.vlt.ID()), id)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := opened.Entries()
	if err != nil || len(entries) != 1 || entries[0].Title != "account" {
		t.Fatalf("persisted entries: %+v, %v", entries, err)
	}
	got, err := opened.Read(entries[0].ID)
	opened.Close()
	if err != nil || len(got.Fields) != 1 || got.Fields[0].Label != "value" || string(got.Fields[0].Value) != "replacement" {
		t.Fatalf("persisted edit: %+v, %v", got, err)
	}
	m = key(t, m, tea.KeyEsc)
	before := vaultFiles(t, dir)
	m = key(t, m, 'd')
	m = key(t, m, 'n')
	sameVaultFiles(t, dir, before)
	if !strings.Contains(plain(m), "account") || strings.Contains(plain(m), "Delete this account?") {
		t.Fatal("delete cancellation failed")
	}
	m = key(t, m, 'd')
	m = key(t, m, 'y')
	if !strings.Contains(plain(m), "Nothing stored yet") {
		t.Fatalf("delete did not return empty list: %q", plain(m))
	}
	after := vaultFiles(t, dir)
	if len(after) != 3 || len(before) != 4 { // The scope, name and manifest files remain.
		t.Fatalf("deleted item file remains: %d -> %d files", len(before), len(after))
	}
}

func TestVaultRejectedInputAndPaste(t *testing.T) {
	for _, language := range []Language{English, Spanish} {
		t.Run(map[Language]string{English: "English", Spanish: "Spanish"}[language], func(t *testing.T) {
			m, path, _ := vaultSession(t, language)
			dir := filepath.Join(filepath.Dir(path), "vault")
			m = addVaultItem(t, m, "existing", "first-value")
			baseline := vaultFiles(t, dir)
			invalid, duplicate, oversized, lineBreak := "That name is not valid. Use 1 to 255 characters, with no control characters.", "An account with that name already exists.", "That account is too large. The limit is 65,788 bytes.", "Values are one line. Remove the line break."
			if language == Spanish {
				invalid, duplicate, oversized, lineBreak = "Ese nombre no es válido. Usa de 1 a 255 caracteres, sin caracteres de control.", "Ya existe una cuenta con ese nombre.", "Esa cuenta es demasiado grande. El límite es 65.788 bytes.", "Los valores son de una línea. Quita el salto de línea."
			}
			for _, tc := range []struct{ name, message string }{{"", invalid}, {"bad\tname", invalid}, {strings.Repeat("x", 256), invalid}, {"existing", duplicate}} {
				m = key(t, m, 'a')
				m = send(t, m, tea.PasteMsg{Content: tc.name})
				m = enter(t, m)
				if !strings.Contains(strings.ReplaceAll(plain(m), "\n", ""), tc.message) && !strings.Contains(plain(m), tc.message[:min(40, len(tc.message))]) {
					t.Fatalf("name %q: %q", tc.name, plain(m))
				}
				sameVaultFiles(t, dir, baseline)
				m = key(t, m, tea.KeyEsc)
			}
			m = key(t, m, 'a')
			m = send(t, m, tea.PasteMsg{Content: "pasted-name"})
			if !strings.Contains(plain(m), "pasted-name") {
				t.Fatal("plain name paste rejected")
			}
			m = enter(t, m)
			m = typeText(t, m, "value")
			m = enter(t, m)
			if language == Spanish {
				m = key(t, m, 'o')
			} else {
				m = key(t, m, 'h')
			}
			m = send(t, m, tea.PasteMsg{Content: "line\nbreak"})
			if !strings.Contains(strings.ReplaceAll(plain(m), "\n", ""), lineBreak) {
				t.Fatalf("line break message: %q", plain(m))
			}
			sameVaultFiles(t, dir, baseline)
			m = send(t, m, tea.PasteMsg{Content: strings.Repeat("v", 66000)})
			m = enter(t, m)
			if !strings.Contains(strings.ReplaceAll(plain(m), "\n", ""), oversized) {
				t.Fatalf("oversized value message missing")
			}
			sameVaultFiles(t, dir, baseline)
			m = key(t, m, tea.KeyEsc)
			m = key(t, m, tea.KeyEsc)
			m = key(t, m, tea.KeyEsc)
			m = enter(t, m) // title preserved; re-enter field steps
			m = typeText(t, m, "value")
			m = enter(t, m)
			if language == Spanish {
				m = key(t, m, 'o')
			} else {
				m = key(t, m, 'h')
			}
			m = send(t, m, tea.PasteMsg{Content: "pasted-value"})
			m = enter(t, m)
			if !strings.Contains(plain(m), "pasted-name") || strings.Contains(plain(m), "pasted-value") {
				t.Fatalf("plain value paste rejected or leaked: %q", plain(m))
			}
		})
	}
}

func TestVaultExitAndMinimumWidth(t *testing.T) {
	for _, language := range []Language{English, Spanish} {
		t.Run(map[Language]string{English: "English", Spanish: "Spanish"}[language], func(t *testing.T) {
			m, path, pass := vaultSession(t, language)
			m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
			check := func(_ string, state Model) { t.Helper(); requireVaultView(t, state) }
			check("empty list", m)
			m = key(t, m, 'a')
			check("add name", m)
			m = send(t, m, tea.PasteMsg{Content: strings.Repeat("N", 255)})
			requireVaultView(t, m)
			m = enter(t, m)
			check("add label", m)
			m = typeText(t, m, "value")
			m = enter(t, m)
			if language == Spanish {
				m = key(t, m, 'o')
			} else {
				m = key(t, m, 'h')
			}
			check("add value", m)
			m = send(t, m, tea.PasteMsg{Content: strings.Repeat("v", 80)})
			requireVaultView(t, m)
			m = enter(t, m)
			check("long-name list", m)
			m = enter(t, m)
			check("item", m)
			m = key(t, m, tea.KeyTab)
			requireVaultView(t, m)
			m = key(t, m, 'e')
			check("edit", m)
			m = key(t, m, tea.KeyEsc)
			m = key(t, m, tea.KeyEsc)
			m = key(t, m, 'd')
			check("delete", m)
			m = key(t, m, 'n')
			for i := 0; i < 16; i++ {
				m = addVaultItem(t, m, fmt.Sprintf("item-%02d", i), "secret-value")
			}
			for i := 0; i < 17; i++ {
				m = key(t, m, tea.KeyDown)
				requireVaultView(t, m)
			}
			check("scrolled list", m)
			for _, label := range []string{"list", "add name", "add value", "item", "edit", "delete"} {
				t.Run("quit "+label, func(t *testing.T) {
					fresh := send(t, New(path, language), tea.WindowSizeMsg{Width: 60, Height: 20})
					fresh = enter(t, fresh)
					fresh, cmd := beginCheck(t, fresh, pass)
					fresh = finishCheck(t, fresh, cmd)
					fresh = enter(t, fresh) // Open Personal from Spaces.
					switch label {
					case "add name", "add value":
						fresh = key(t, fresh, 'a')
						if label == "add value" {
							fresh = typeText(t, fresh, "new-name")
							fresh = enter(t, fresh)
						}
					case "item", "edit":
						fresh = enter(t, fresh)
						if label == "edit" {
							fresh = key(t, fresh, 'e')
						}
					case "delete":
						fresh = key(t, fresh, 'd')
					}
					requireVaultView(t, fresh)
					next, quit := fresh.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
					if quit == nil || next.(Model).ExitStatus() != 0 || next.(Model).vlt != nil {
						t.Fatal("Ctrl+C did not lock and quit successfully")
					}
					if _, ok := quit().(tea.QuitMsg); !ok {
						t.Fatal("not QuitMsg")
					}
				})
			}
			m = key(t, m, tea.KeyEsc) // Item list returns to Spaces.
			next, quit := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
			if quit == nil || next.(Model).ExitStatus() != 0 || next.(Model).vlt != nil {
				t.Fatal("Esc did not lock and quit successfully")
			}
		})
	}
}
