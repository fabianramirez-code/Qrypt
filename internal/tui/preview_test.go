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

func TestPreviewScreensAndExit(t *testing.T) {
	for _, tc := range []struct {
		lang   Language
		banner string
	}{{English, "PREVIEW: temporary data, nothing is saved"}, {Spanish, "VISTA PREVIA: datos temporales, no se guarda nada"}} {
		t.Run(tc.banner, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "identity")
			id, err := identity.FromSeed(make([]byte, identity.SeedSize))
			if err != nil {
				t.Fatal(err)
			}
			m := NewPreview(path, tc.lang, id)
			m.copyFn = func(string) error { return nil }
			m.clearFn = func(string) error { return nil }
			m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
			if !strings.Contains(plain(m), "LANGUAGE / IDIOMA") || strings.Contains(plain(m), tc.banner) {
				t.Fatal("not language first", plain(m))
			}
			m = enter(t, m)
			if m.ExitStatus() != 0 || !strings.Contains(plain(m), "Spaces") && !strings.Contains(plain(m), "Espacios") {
				t.Fatal("not vault", plain(m))
			}
			if strings.Contains(plain(m), "passphrase") || !strings.Contains(plain(m), "Personal (4)") || !strings.Contains(plain(m), "Work (5)") {
				t.Fatal("not sample vault", plain(m))
			}
			check := func() {
				t.Helper()
				if !strings.Contains(plain(m), tc.banner) {
					t.Fatal("banner absent", plain(m))
				}
				for _, line := range strings.Split(plain(m), "\n") {
					if lipgloss.Width(line) > 60 {
						t.Fatalf("too wide: %q", line)
					}
				}
			}
			check()
			m = enter(t, m) // Open Personal from Spaces.
			if len(m.names) != 4 {
				t.Fatal("Personal samples missing", plain(m))
			}
			check()
			m = enter(t, m)
			check()
			m = key(t, m, tea.KeyTab)
			check()
			m = key(t, m, tea.KeyEsc)
			check()
			m = key(t, m, 'a')
			check()
			m = enter(t, m)
			check()
			m = key(t, m, tea.KeyEsc)
			check()
			m = key(t, m, tea.KeyEsc)
			check()
			m = key(t, m, 'd')
			check()
			m = key(t, m, 'n')
			check()
			m = key(t, m, tea.KeyEsc) // Leave Personal for Spaces.
			m = key(t, m, tea.KeyEsc)
			if m.ExitStatus() != 0 || m.vlt != nil {
				t.Fatal("lock failed")
			}
		})
	}
}

// The expected decrypted items retain the preview fixture's original contents.
func TestPreviewDecryptedFixture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	id, err := identity.FromSeed(make([]byte, identity.SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	m := enter(t, send(t, NewPreview(path, English, id), tea.WindowSizeMsg{Width: 60, Height: 20}))
	defer func() { m.closeVault() }()
	want := map[string][]vault.Item{
		"Personal": {
			{Title: "example: bank", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "value", Value: []byte("DEMO-ACCOUNT-0000")}}},
			{Title: "example: email", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "value", Value: []byte("demo@example.invalid")}}},
			{Title: "example: github token", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "value", Value: []byte("fake-token-not-valid")}}},
			{Title: "example: wifi", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "value", Value: []byte("sample-wifi-password")}}},
		},
		"Work": {
			{Title: "example: meeting", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "value", Value: []byte("sample-agenda")}}},
			{Title: "example: support", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "value", Value: []byte("support@example.invalid")}}},
			{Path: []string{"AWS", "prod"}, Title: "sample deployment", Fields: []vault.Field{{Kind: vault.FieldNote, Label: "instructions", Value: []byte("Fake preview instructions\nNo real credentials")}}},
			{Path: []string{"Google Stitch"}, Title: "account-1@example.invalid", Fields: []vault.Field{
				{Kind: vault.FieldHidden, Label: "Claude Code", Value: []byte("FAKE-CLAUDE-0001")},
				{Kind: vault.FieldHidden, Label: "codex", Value: []byte("FAKE-CODEX-0001")},
				{Kind: vault.FieldHidden, Label: "gemini", Value: []byte("FAKE-GEMINI-0001")},
				{Kind: vault.FieldVisible, Label: "plan", Value: []byte("demo-only")},
			}},
			{Path: []string{"Google Stitch"}, Title: "account-2@example.invalid", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "Claude Code", Value: []byte("FAKE-CLAUDE-0002")}}},
		},
	}
	for _, scope := range m.scopes {
		name, err := scope.Name()
		if err != nil {
			t.Fatal(err)
		}
		entries, err := scope.Entries()
		if err != nil {
			t.Fatal(err)
		}
		var got []vault.Item
		for _, entry := range entries {
			item, err := scope.Read(entry.ID)
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, item)
		}
		if !reflect.DeepEqual(got, want[name]) {
			t.Fatalf("%s decrypted items: got %+v, want %+v", name, got, want[name])
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatalf("missing preview spaces: %v", want)
	}
}

func TestPreviewAutoLockAndFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity")
	id, err := identity.FromSeed(make([]byte, identity.SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	m := enter(t, send(t, NewPreview(path, English, id), tea.WindowSizeMsg{Width: 60, Height: 20}))
	m.now = func() time.Time { return m.lastKey.Add(idleLifetime) }
	next, cmd := m.Update(idleTickMsg{})
	if cmd == nil || next.(Model).ExitStatus() != 0 || next.(Model).vlt != nil {
		t.Fatal("auto lock failed")
	}
	// The failed vault view still identifies temporary data.
	block := filepath.Join(t.TempDir(), "block")
	if err := os.WriteFile(block, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	failed := NewPreview(filepath.Join(block, "identity"), English, id)
	failed = enter(t, send(t, failed, tea.WindowSizeMsg{Width: 60, Height: 20}))
	if failed.ExitStatus() != 1 || !strings.Contains(plain(failed), "PREVIEW: temporary data, nothing is saved") {
		t.Fatal("failure view", plain(failed))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("constructor wrote identity", err)
	}
}

func TestNormalStillRequiresPassphrase(t *testing.T) {
	m, _, _, _ := unlockFixture(t, English)
	if !strings.Contains(plain(m), "Type your passphrase (7 words):") || m.ExitStatus() != 1 {
		t.Fatal("normal unlock bypassed")
	}
	m = enter(t, m)
	if !strings.Contains(plain(m), "Type exactly 7 words.") || m.ExitStatus() != 1 {
		t.Fatal("empty phrase bypassed")
	}
}
