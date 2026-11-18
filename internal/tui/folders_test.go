package tui

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

func TestFolderNavigationAndCounts(t *testing.T) {
	m := spacesSession(t, English)
	_, err := m.vlt.Add(vault.Item{Path: []string{"outer", "inner", "deep"}, Title: "z", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "token", Value: []byte("secret")}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.vlt.Add(vault.Item{Path: []string{"outer"}, Title: "a", Fields: []vault.Field{{Kind: vault.FieldVisible, Label: "public", Value: []byte("visible")}}})
	if err != nil {
		t.Fatal(err)
	}
	m.refreshSpaces()
	if !strings.Contains(plain(m), "Personal (2)") || strings.Contains(plain(m), "secret") {
		t.Fatal(plain(m))
	}
	m = enter(t, m)
	for _, want := range []string{"Personal", "outer (2)"} {
		if !strings.Contains(plain(m), want) {
			t.Fatal(plain(m))
		}
	}
	if strings.Contains(plain(m), "secret") || strings.Contains(plain(m), "visible") {
		t.Fatal("folder leaked value")
	}
	m = enter(t, m)
	if !strings.Contains(plain(m), "Personal / outer") || !strings.Contains(plain(m), "inner (1)") {
		t.Fatal(plain(m))
	}
	m = enter(t, m)
	m = enter(t, m)
	if !strings.Contains(plain(m), "Personal / outer / inner / deep") {
		t.Fatal(plain(m))
	}
	for range 3 {
		m = key(t, m, tea.KeyEsc)
	}
	if !strings.Contains(plain(m), "outer (2)") {
		t.Fatal(plain(m))
	}
	m = key(t, m, tea.KeyEsc)
	if !strings.Contains(plain(m), "Personal (2)") {
		t.Fatal(plain(m))
	}
}

func TestFolderTemporaryAndRename(t *testing.T) {
	m := spacesSession(t, English)
	m = enter(t, m)
	m = key(t, m, 'f')
	m = typeText(t, m, "outer")
	m = enter(t, m)
	if !strings.Contains(plain(m), "temporary") {
		t.Fatal(plain(m))
	}
	m = key(t, m, tea.KeyEsc)
	if strings.Contains(plain(m), "outer") {
		t.Fatal(plain(m))
	}
	m = key(t, m, 'f')
	m = typeText(t, m, "outer")
	m = enter(t, m)
	m = addVaultItem(t, m, "entry", "secret")
	entries, err := m.vlt.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !reflect.DeepEqual(entries[0].Path, []string{"outer"}) {
		t.Fatal(entries)
	}
	m = key(t, m, tea.KeyEsc)
	m = key(t, m, 'r')
	m = typeText(t, m, "renamed")
	m = enter(t, m)
	entries, err = m.vlt.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID == "" || !reflect.DeepEqual(entries[0].Path, []string{"renamed"}) {
		t.Fatal(entries)
	}
}

func TestFieldEditingNotesAndIdentity(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		m := spacesSession(t, lang)
		m = enter(t, m)
		m = addVaultItem(t, m, "account", "hidden-secret")
		entries, err := m.vlt.Entries()
		if err != nil || len(entries) != 1 {
			t.Fatal(entries, err)
		}
		id := entries[0].ID
		m = enter(t, m)
		if strings.Contains(plain(m), "hidden-secret") || !strings.Contains(plain(m), "••••••") {
			t.Fatal(plain(m))
		}
		m = key(t, m, 'a')
		m = typeText(t, m, "memo")
		m = enter(t, m)
		m = key(t, m, 'n')
		m = send(t, m, tea.PasteMsg{Content: "first\nsecond"})
		if strings.Contains(plain(m), "hidden-secret") {
			t.Fatal("note leaked hidden field")
		}
		m = send(t, m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		if !strings.Contains(plain(m), "(2)") || strings.Contains(plain(m), "first") {
			t.Fatal(plain(m))
		}
		m = key(t, m, tea.KeyTab)
		if !strings.Contains(plain(m), "first") {
			t.Fatal(plain(m))
		}
		copied := ""
		m.copyFn = func(s string) error { copied = s; return nil }
		m = copyKey(m)
		if copied != "first\nsecond" {
			t.Fatal(copied)
		}
		m = key(t, m, 'e')
		m = send(t, m, tea.PasteMsg{Content: "discarded"})
		m = key(t, m, tea.KeyEsc)
		item, err := m.vlt.Read(id)
		if err != nil || string(item.Fields[1].Value) != "first\nsecond" {
			t.Fatal(item, err)
		}
		m = key(t, m, 'r')
		m = typeText(t, m, "renamed")
		m = enter(t, m)
		entries, err = m.vlt.Entries()
		if err != nil || len(entries) != 1 || entries[0].ID != id || entries[0].Title != "renamed" {
			t.Fatal(entries, err)
		}
		m = key(t, m, 'd')
		m = key(t, m, 'n')
		item, err = m.vlt.Read(id)
		if err != nil || len(item.Fields) != 2 {
			t.Fatal(err, item)
		}
		m = key(t, m, 'd')
		m = key(t, m, 'y')
		item, err = m.vlt.Read(id)
		if err != nil || len(item.Fields) != 1 {
			t.Fatal(err, item)
		}
	}
}

func TestFolderRefusalsPreserveBytes(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		m := spacesSession(t, lang)
		m = enter(t, m)
		m = addVaultItem(t, m, "same", "secret")
		root := vaultDir(m.path)
		before := spaceSnapshot(t, root)
		m = key(t, m, 'a')
		m = typeText(t, m, "same")
		m = enter(t, m)
		if m.message != lang.text().nameExists || m.input.String() != "same" || !reflect.DeepEqual(before, spaceSnapshot(t, root)) {
			t.Fatal(plain(m))
		}
		m = key(t, m, tea.KeyEsc)
		m = enter(t, m)
		m = key(t, m, 'a')
		m = typeText(t, m, "value")
		m = enter(t, m)
		if m.message != lang.text().labelExists || m.input.String() != "value" || !reflect.DeepEqual(before, spaceSnapshot(t, root)) {
			t.Fatal(plain(m))
		}
		m = key(t, m, tea.KeyEsc)
		m = key(t, m, 'e')
		m = send(t, m, tea.PasteMsg{Content: "line\nbreak"})
		if m.message != lang.text().lineBreak || m.input.String() != "" || !reflect.DeepEqual(before, spaceSnapshot(t, root)) {
			t.Fatal(plain(m))
		}
		m = key(t, m, tea.KeyEsc)
		m = key(t, m, tea.KeyEsc)
		m = key(t, m, 'f')
		m = typeText(t, m, "empty")
		m = enter(t, m)
		m = key(t, m, tea.KeyEsc)
		if strings.Contains(plain(m), "empty") || !reflect.DeepEqual(before, spaceSnapshot(t, root)) {
			t.Fatal(plain(m))
		}
	}
}

func TestFolderNestedDeletionAndManyFields(t *testing.T) {
	m := spacesSession(t, English)
	fields := make([]vault.Field, 64)
	for i := range fields {
		fields[i] = vault.Field{Kind: vault.FieldHidden, Label: fmt.Sprintf("field-%02d", i), Value: []byte("private")}
	}
	_, err := m.vlt.Add(vault.Item{Path: []string{"one", "two", "three"}, Title: "nested", Fields: fields})
	if err != nil {
		t.Fatal(err)
	}
	m.refreshSpaces()
	m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
	m = enter(t, m)
	for range 3 {
		wideFit(t, m, 60, 20)
		m = enter(t, m)
	}
	m = enter(t, m)
	for range 63 {
		m = key(t, m, tea.KeyDown)
		wideFit(t, m, 60, 20)
	}
	if !strings.Contains(plain(m), "field-63") || strings.Contains(plain(m), "private") {
		t.Fatal(plain(m))
	}
	m = key(t, m, tea.KeyEsc)
	m = key(t, m, 'd')
	m = key(t, m, 'y')
	if len(m.folderPath) != 0 || strings.Contains(plain(m), "one (1)") {
		t.Fatal(plain(m))
	}
}

func TestNewScreensLockAndFit(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		for _, theme := range []string{"green", "carbon"} {
			m := spacesSession(t, lang)
			m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
			m.themeName = theme
			m.refreshStyles()
			m = enter(t, m)
			m = addVaultItem(t, m, "name", "hidden-payload")
			for _, icons := range []string{"unicode", "emoji", "nerd"} {
				m.icons = icons
				for _, screen := range []screen{vaultList, vaultItem, vaultAddName, folderName, vaultRename, fieldLabel, fieldKind, vaultAddValue, fieldNote, vaultEdit, vaultDelete, fieldDelete} {
					x := m
					x.screen = screen
					wideFit(t, x, 60, 20)
					if strings.Contains(plain(x), "hidden-payload") {
						t.Fatalf("%v leaked", screen)
					}
				}
			}
		}
	}
}

// A hidden value appears only on its own row and only while the user shows it.
func TestHiddenFieldIsShownOnlyOnRequest(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	m := NewPreview(filepath.Join(t.TempDir(), "identity"), English, id)
	m.copyFn = func(string) error { return nil }
	m.clearFn = func(string) error { return nil }
	m = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	const secret = "DEMO-ACCOUNT-0000"
	m = enter(t, m) // language screen -> spaces
	if strings.Contains(framed(m), secret) {
		t.Fatal("value on the Spaces screen")
	}
	m = enter(t, m) // spaces -> the top folder of Personal
	if strings.Contains(framed(m), secret) || !strings.Contains(plain(m), "example: bank") {
		t.Fatalf("folder screen:\n%s", framed(m))
	}
	m = enter(t, m) // the first item, "example: bank"
	if strings.Contains(framed(m), secret) || !strings.Contains(plain(m), "value") {
		t.Fatalf("item screen shows the value before it is asked for:\n%s", framed(m))
	}
	m = key(t, m, tea.KeyTab)
	if !strings.Contains(framed(m), secret) {
		t.Fatalf("Tab did not show the chosen field:\n%s", framed(m))
	}
	m = key(t, m, tea.KeyTab)
	if strings.Contains(framed(m), secret) {
		t.Fatal("Tab did not hide the field again")
	}
	m = key(t, m, tea.KeyTab)
	m = key(t, m, tea.KeyEsc)
	if strings.Contains(framed(m), secret) {
		t.Fatal("value stayed visible after leaving the item")
	}
	m = enter(t, m)
	if strings.Contains(framed(m), secret) {
		t.Fatal("value visible again when the item is reopened")
	}
}
