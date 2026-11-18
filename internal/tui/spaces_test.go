package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

func spacesSession(t *testing.T, language Language) Model {
	t.Helper()
	m, _, pass, _ := unlockFixture(t, language)
	m, cmd := beginCheck(t, m, pass)
	m = finishCheck(t, m, cmd)
	m.copyFn = func(string) error { return nil }
	m.clearFn = func(string) error { return nil }
	return m
}

func TestSpacesCreateNavigateAndPersist(t *testing.T) {
	m := spacesSession(t, English)
	if !strings.Contains(plain(m), "Personal (0)") || !strings.Contains(plain(m), "Spaces") {
		t.Fatal(plain(m))
	}
	m = key(t, m, 'a')
	m = send(t, m, tea.PasteMsg{Content: "Work"})
	m = enter(t, m)
	if !strings.Contains(plain(m), "> Work (0)") {
		t.Fatal(plain(m))
	}
	m = enter(t, m)
	if !strings.Contains(plain(m), "Work") || strings.Contains(plain(m), "Qrypt vault") {
		t.Fatal(plain(m))
	}
	m = addVaultItem(t, m, "work-only", "private")
	m = key(t, m, tea.KeyEsc)
	if !strings.Contains(plain(m), "Work (1)") {
		t.Fatal(plain(m))
	}
	m = key(t, m, tea.KeyUp)
	m = enter(t, m)
	if strings.Contains(plain(m), "work-only") || !strings.Contains(plain(m), "Personal") {
		t.Fatal(plain(m))
	}
	m = addVaultItem(t, m, "home-only", "secret")
	m = key(t, m, tea.KeyEsc)
	if !strings.Contains(plain(m), "Personal (1)") {
		t.Fatal(plain(m))
	}
	m = enter(t, m)
	m = key(t, m, 'd')
	m = key(t, m, 'y')
	m = key(t, m, tea.KeyEsc)
	if !strings.Contains(plain(m), "Personal (0)") {
		t.Fatal(plain(m))
	}
}

func TestSpacesRejectedNamesAndDelete(t *testing.T) {
	for _, tc := range []struct {
		language                     Language
		duplicate, invalid, nonempty string
	}{
		{English, "A space with that name already exists.", "That name is not valid. Use 1 to 255 characters, with no control characters.", "This space is not empty. Delete its items first."},
		{Spanish, "Ya existe un espacio con ese nombre.", "Ese nombre no es válido. Usa de 1 a 255 caracteres, sin caracteres de control.", "Este espacio no está vacío. Borra primero sus elementos."},
	} {
		t.Run(tc.duplicate, func(t *testing.T) {
			m := spacesSession(t, tc.language)
			root := vaultDir(m.path)
			before := spaceSnapshot(t, root)
			for _, test := range []struct{ name, message string }{{"Personal", tc.duplicate}, {"", tc.invalid}, {"bad\tname", tc.invalid}, {strings.Repeat("x", 256), tc.invalid}} {
				m = key(t, m, 'a')
				m = send(t, m, tea.PasteMsg{Content: test.name})
				m = enter(t, m)
				if !strings.Contains(strings.ReplaceAll(plain(m), "\n", ""), test.message) || m.input != test.name {
					t.Fatalf("rejection %q: %s", test.name, plain(m))
				}
				if !reflect.DeepEqual(before, spaceSnapshot(t, root)) {
					t.Fatal("rejected name changed files")
				}
				m = key(t, m, tea.KeyEsc)
			}
			m = key(t, m, 'a')
			m = send(t, m, tea.PasteMsg{Content: "line\nbreak"})
			if m.input != "" || !strings.Contains(strings.ReplaceAll(plain(m), "\n", ""), tc.language.text().lineBreak) {
				t.Fatal(plain(m))
			}
			m = key(t, m, tea.KeyEsc)
			m = enter(t, m)
			m = addVaultItem(t, m, "entry", "hidden")
			m = key(t, m, tea.KeyEsc)
			before = spaceSnapshot(t, root)
			m = key(t, m, 'd')
			m = key(t, m, 'y')
			if !strings.Contains(plain(m), tc.nonempty) || !reflect.DeepEqual(before, spaceSnapshot(t, root)) {
				t.Fatal(plain(m))
			}
			m = key(t, m, 'n')
			m = enter(t, m)
			m = key(t, m, 'd')
			m = key(t, m, 'y')
			m = key(t, m, tea.KeyEsc)
			m = key(t, m, 'a')
			m = send(t, m, tea.PasteMsg{Content: "Work"})
			m = enter(t, m)
			m = key(t, m, 'r')
			before = spaceSnapshot(t, root)
			m = send(t, m, tea.PasteMsg{Content: "bad\tname"})
			m = enter(t, m)
			if !strings.Contains(strings.ReplaceAll(plain(m), "\n", ""), tc.invalid) || m.input != "bad\tname" || !reflect.DeepEqual(before, spaceSnapshot(t, root)) {
				t.Fatal("invalid rename changed data", plain(m))
			}
			m = key(t, m, tea.KeyEsc)
			m = key(t, m, 'r')
			m = send(t, m, tea.PasteMsg{Content: "Personal"})
			m = enter(t, m)
			if !strings.Contains(strings.ReplaceAll(plain(m), "\n", ""), tc.duplicate) || m.input != "Personal" {
				t.Fatal(plain(m))
			}
			m = key(t, m, tea.KeyEsc)
			m = key(t, m, 'r')
			m = send(t, m, tea.PasteMsg{Content: "Office"})
			m = enter(t, m)
			if !strings.Contains(plain(m), "Office (0)") {
				t.Fatal(plain(m))
			}
			m = key(t, m, 'd')
			m = key(t, m, 'n')
			if !strings.Contains(plain(m), "Office (0)") {
				t.Fatal(plain(m))
			}
			m = key(t, m, 'd')
			m = key(t, m, 'y')
			if strings.Contains(plain(m), "Office (0)") {
				t.Fatal(plain(m))
			}
		})
	}
}

func spaceSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		name, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		result[name] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestSpacesLockAndPreview(t *testing.T) {
	m := spacesSession(t, English)
	m = key(t, m, 'a')
	m = send(t, m, tea.PasteMsg{Content: "Work"})
	m = enter(t, m)
	for _, state := range []screen{spacesList, spacesAdd, spacesRename, spacesDelete} {
		x := spacesSession(t, English)
		x = key(t, x, 'a')
		x = send(t, x, tea.PasteMsg{Content: "Work"})
		x = enter(t, x)
		x.screen = state
		next, cmd := x.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		locked := next.(Model)
		if cmd == nil || locked.ExitStatus() != 0 || locked.vlt != nil || locked.scopeID != nil {
			t.Fatalf("Ctrl+C screen %d", state)
		}
		for _, scope := range x.scopes {
			if _, err := scope.Name(); err != vault.ErrClosed {
				t.Fatalf("scope remained open: %v", err)
			}
		}
	}
	scopes := append([]*vault.Vault(nil), m.scopes...)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	for _, scope := range scopes {
		if _, err := scope.Name(); err != vault.ErrClosed {
			t.Fatalf("Esc left scope open: %v", err)
		}
	}
	if cmd == nil || next.(Model).ExitStatus() != 0 || next.(Model).vlt != nil || next.(Model).scopeID != nil {
		t.Fatal("Esc did not lock")
	}
	m.now = func() time.Time { return m.lastKey.Add(idleLifetime) }
	scopes = append([]*vault.Vault(nil), m.scopes...)
	next, cmd = m.Update(idleTickMsg{})
	for _, scope := range scopes {
		if _, err := scope.Name(); err != vault.ErrClosed {
			t.Fatalf("idle left scope open: %v", err)
		}
	}
	if cmd == nil || next.(Model).ExitStatus() != 0 || next.(Model).scopeID != nil {
		t.Fatal("idle lock")
	}
	id, err := identity.FromSeed(make([]byte, identity.SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "identity")
	preview := enter(t, send(t, NewPreview(p, English, id), tea.WindowSizeMsg{Width: 60, Height: 20}))
	if !strings.Contains(plain(preview), "Personal (4)") || !strings.Contains(plain(preview), "Work (5)") || !strings.Contains(plain(preview), "PREVIEW:") {
		t.Fatal(plain(preview))
	}
	preview = key(t, preview, tea.KeyDown)
	preview = enter(t, preview)
	if !strings.Contains(plain(preview), "PREVIEW:") || !strings.Contains(plain(preview), "example: meeting") || !strings.Contains(plain(preview), "example: support") || !strings.Contains(plain(preview), "Google Stitch (2)") || !strings.Contains(plain(preview), "AWS (1)") || strings.Contains(plain(preview), "example: bank") {
		t.Fatal(plain(preview))
	}
}

func TestSpacesClipboardExpiryOnNewScreens(t *testing.T) {
	for _, state := range []screen{spacesList, spacesAdd, spacesRename, spacesDelete} {
		m := spacesSession(t, English)
		m = enter(t, m)
		m = addVaultItem(t, m, "account", "private")
		copied, cleared := 0, 0
		m.copyFn = func(s string) error {
			if s != "private" {
				t.Fatal("wrong copy")
			}
			copied++
			return nil
		}
		m.clearFn = func(s string) error {
			if s != "private" {
				t.Fatal("wrong clear")
			}
			cleared++
			return nil
		}
		m = copyKey(m) // Do not execute the 30-second tea.Tick command in the test.
		m = key(t, m, tea.KeyEsc)
		m.screen = state
		next, cmd := m.Update(clipboardTickMsg{token: m.copyToken})
		m = next.(Model)
		if cmd != nil || m.copyActive || copied != 1 || cleared != 1 {
			t.Fatalf("expiry on screen %d: copied=%d cleared=%d", state, copied, cleared)
		}
	}
}

func TestSpacesFramedFit(t *testing.T) {
	for _, language := range []Language{English, Spanish} {
		for _, theme := range []string{"green"} {
			m := spacesSession(t, language)
			m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
			m.themeName = theme
			m.refreshStyles()
			for _, state := range []screen{spacesList, spacesAdd, spacesRename, spacesDelete} {
				m.screen = state
				view := framed(m)
				lines := strings.Split(view, "\n")
				if len(lines) > 20 {
					t.Fatalf("%s %d: %d rows", theme, state, len(lines))
				}
				for _, line := range lines {
					if lipgloss.Width(line) > 60 {
						t.Fatalf("%s %d: %q", theme, state, line)
					}
				}
			}
		}
	}
}
