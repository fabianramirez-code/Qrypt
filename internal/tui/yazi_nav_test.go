package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

func yaziSession(t *testing.T) (Model, string) {
	t.Helper()
	m, path, _ := vaultSession(t, English)
	for _, item := range []vault.Item{
		{Title: "wifi"},
		{Path: []string{"aws"}, Title: "root"},
		{Path: []string{"google", "one"}, Title: "first"},
		{Path: []string{"google", "two"}, Title: "second"},
	} {
		item.Fields = []vault.Field{{Kind: vault.FieldHidden, Label: "k", Value: []byte("fake")}}
		if _, err := m.vlt.Add(item); err != nil {
			t.Fatal(err)
		}
	}
	return send(t, m, tea.WindowSizeMsg{Width: 110, Height: 30}), path
}

func letter(t *testing.T, m Model, r rune) Model {
	t.Helper()
	return send(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
}

// A folder opens only when entered with Right (or l), never by moving over it.
func TestFolderOpensOnlyOnRight(t *testing.T) {
	m, path := yaziSession(t)
	before := spaceSnapshot(t, vaultDir(path))
	if m.screen != vaultList || strings.Contains(plain(m), "root") || strings.Contains(plain(m), "one") {
		t.Fatalf("folders must start closed:\n%s", plain(m))
	}
	m = key(t, m, tea.KeyDown) // onto google
	if strings.Contains(plain(m), "one") || strings.Contains(plain(m), "two") || len(m.folderPath) != 0 {
		t.Fatalf("moving over a folder opened it:\n%s", plain(m))
	}
	m = key(t, m, tea.KeyRight)
	if !strings.Contains(plain(m), "one") || !strings.Contains(plain(m), "two") || !reflect.DeepEqual(m.folderPath, []string{"google"}) || strings.Contains(plain(m), "first") {
		t.Fatalf("Right did not open exactly one level:\n%s", plain(m))
	}
	m = key(t, m, tea.KeyLeft)
	if strings.Contains(plain(m), "one") || len(m.folderPath) != 0 || m.selectedRow().name != "google" {
		t.Fatalf("Left did not close the folder and keep it selected:\n%s", plain(m))
	}
	m = key(t, m, tea.KeyLeft)
	if m.screen != spacesList || strings.Contains(plain(m), "wifi") {
		t.Fatalf("Left at the root did not close the space:\n%s", plain(m))
	}
	if !reflect.DeepEqual(before, spaceSnapshot(t, vaultDir(path))) {
		t.Fatal("navigation wrote to disk")
	}
}

// h/j/k/l, g/G, Home/End and the half-page keys navigate like the arrows.
func TestYaziNavigationKeys(t *testing.T) {
	m, path := yaziSession(t)
	before := spaceSnapshot(t, vaultDir(path))
	state := func(m Model) string {
		return strings.Join(m.folderPath, "/") + "|" + m.selectedRow().name + "|" + plain(m)
	}
	for _, pair := range []struct {
		letter rune
		arrow  rune
	}{{'j', tea.KeyDown}, {'l', tea.KeyRight}, {'j', tea.KeyDown}, {'k', tea.KeyUp}, {'h', tea.KeyLeft}, {'h', tea.KeyLeft}, {'l', tea.KeyRight}} {
		viaArrow := key(t, m, pair.arrow)
		m = letter(t, m, pair.letter)
		if state(m) != state(viaArrow) {
			t.Fatalf("%q differs from its arrow", pair.letter)
		}
	}
	last := len(m.folderRows) - 1
	if m = send(t, m, tea.KeyPressMsg{Code: 'g', Text: "G", Mod: tea.ModShift}); m.cursor != last {
		t.Fatalf("G: cursor %d, want %d", m.cursor, last)
	}
	if m = letter(t, m, 'g'); m.cursor != 0 {
		t.Fatalf("g: cursor %d", m.cursor)
	}
	if m = key(t, m, tea.KeyEnd); m.cursor != last {
		t.Fatalf("End: cursor %d", m.cursor)
	}
	if m = key(t, m, tea.KeyHome); m.cursor != 0 {
		t.Fatalf("Home: cursor %d", m.cursor)
	}
	if m = send(t, m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}); m.cursor != last {
		t.Fatalf("Ctrl+D: cursor %d", m.cursor)
	}
	if m = send(t, m, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}); m.cursor != 0 {
		t.Fatalf("Ctrl+U: cursor %d", m.cursor)
	}
	if m.screen != vaultList || !reflect.DeepEqual(before, spaceSnapshot(t, vaultDir(path))) {
		t.Fatal("navigation left the tree or wrote to disk")
	}
	// In the detail pane l never reveals and h returns to the tree.
	m = key(t, key(t, m, tea.KeyEnd), tea.KeyRight)
	if m.screen != vaultItem || m.item != "wifi" {
		t.Fatalf("Right on an item did not open it: screen %d", m.screen)
	}
	if m = letter(t, m, 'l'); m.visible || strings.Contains(plain(m), "fake") {
		t.Fatal("l revealed the value")
	}
	if m = letter(t, m, 'h'); m.screen != vaultList {
		t.Fatal("h did not return to the tree")
	}
}
