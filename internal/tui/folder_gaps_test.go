package tui

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

// A folder is only a path in an item; creating an empty one has no disk write.
// Moving a folder to another parent is not offered by the UI; R renames its prefix.
func TestFolderFieldMirrorAfterEachChange(t *testing.T) {
	for _, width := range []int{60, 120} {
		t.Run(map[int]string{60: "narrow", 120: "wide"}[width], func(t *testing.T) {
			m := mirrorFixture(t, English)
			m = send(t, m, tea.WindowSizeMsg{Width: width, Height: 30})
			folder := filepath.Join(t.TempDir(), "mirror")
			m = mirrorChoose(t, m, folder)
			check := func(action string) { t.Helper(); t.Run(action, func(t *testing.T) { mirroredFiles(t, m, folder) }) }
			check("configured")
			m = key(t, m, tea.KeyEsc)
			m = enter(t, m)
			m = addVaultItem(t, m, "account", "fake-token")
			check("add item")
			m = enter(t, m)
			m = key(t, m, 'a')
			m = typeText(t, m, "memo")
			m = enter(t, m)
			m = key(t, m, 'v')
			m = typeText(t, m, "example.invalid")
			m = enter(t, m)
			check("add field")
			m = key(t, m, 'e')
			m = typeText(t, m, "changed.example.invalid")
			m = enter(t, m)
			check("edit field value")
			// R on the inspector renames the item title, not a field label.
			m = key(t, m, 'r')
			m = typeText(t, m, "renamed-account")
			m = enter(t, m)
			check("rename account")
			m = key(t, m, 'd')
			m = key(t, m, 'y')
			check("delete field")
			m = key(t, m, tea.KeyEsc)
			m = key(t, m, 'f')
			m = typeText(t, m, "outer")
			m = enter(t, m)
			check("create temporary folder")
			m = addVaultItem(t, m, "nested", "fake-nested")
			check("persist folder with item")
			m = key(t, m, tea.KeyEsc)
			m = key(t, m, 'r')
			m = typeText(t, m, "renamed-folder")
			m = enter(t, m)
			check("rename folder prefix")
			m = enter(t, m)
			m = key(t, m, 'd')
			m = key(t, m, 'y')
			check("delete last item and folder")
			// After deletion the location automatically ascends to the root.
			m = key(t, m, 'd')
			m = key(t, m, 'y')
			check("delete item")
		})
	}
}

// Inspect persisted state with an independent vault handle, never with the model's cached entries.
func diskItems(t *testing.T, m Model) map[string]vault.Item {
	t.Helper()
	opened, err := vault.Open(filepath.Join(vaultDir(m.path), "scopes", m.vlt.ID()), m.scopeID)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	entries, err := opened.Entries()
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]vault.Item, len(entries))
	for _, e := range entries {
		item, err := opened.Read(e.ID)
		if err != nil {
			t.Fatal(err)
		}
		got[e.ID] = item
	}
	return got
}
func TestWideFolderFieldActionsPersistOnDisk(t *testing.T) {
	m := send(t, spacesSession(t, English), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = enter(t, m)
	root := vaultDir(m.path)
	expected := map[string]vault.Item{}
	selectedID := ""
	check := func(action string, before map[string]string, changed bool) {
		t.Helper()
		t.Run(action, func(t *testing.T) {
			got := diskItems(t, m)
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("disk items: got %#v, want %#v", got, expected)
			}
			after := spaceSnapshot(t, root)
			if !changed && !reflect.DeepEqual(before, after) {
				t.Fatalf("cancelled %s changed files", action)
			}
			for name, data := range before {
				if strings.HasSuffix(name, ".qrs") && after[name] != data {
					id := strings.TrimSuffix(filepath.Base(name), ".qrs")
					if id != selectedID {
						t.Fatalf("unrelated item %s changed on %s", id, action)
					}
				}
			}
		})
	}
	original := spaceSnapshot(t, root)
	m = key(t, m, 'a')
	m = typeText(t, m, "discard")
	m = key(t, m, tea.KeyEsc)
	check("cancel add item", original, false)
	before := spaceSnapshot(t, root)
	m = addVaultItem(t, m, "account", "fake-token")
	for id := range diskItems(t, m) {
		selectedID = id
		expected[id] = vault.Item{Title: "account", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "value", Value: []byte("fake-token")}}}
	}
	if len(expected) != 1 {
		t.Fatal("add did not persist exactly one item")
	}
	check("add item", before, true)
	m = enter(t, m)
	before = spaceSnapshot(t, root)
	m = key(t, m, 'e')
	m = typeText(t, m, "discard")
	m = key(t, m, tea.KeyEsc)
	check("cancel edit field", before, false)
	before = spaceSnapshot(t, root)
	m = key(t, m, 'e')
	m = typeText(t, m, "new-token")
	m = enter(t, m)
	item := expected[selectedID]
	item.Fields[0].Value = []byte("new-token")
	expected[selectedID] = item
	check("edit field", before, true)
	before = spaceSnapshot(t, root)
	m = key(t, m, 'a')
	m = typeText(t, m, "cancelled")
	m = key(t, m, tea.KeyEsc)
	check("cancel add field", before, false)
	before = spaceSnapshot(t, root)
	m = key(t, m, 'a')
	m = typeText(t, m, "site")
	m = enter(t, m)
	m = key(t, m, 'v')
	m = typeText(t, m, "example.invalid")
	m = enter(t, m)
	item = expected[selectedID]
	item.Fields = append(item.Fields, vault.Field{Kind: vault.FieldVisible, Label: "site", Value: []byte("example.invalid")})
	expected[selectedID] = item
	check("add field", before, true)
	before = spaceSnapshot(t, root)
	m = key(t, m, 'd')
	m = key(t, m, tea.KeyEsc)
	check("cancel delete field", before, false)
	before = spaceSnapshot(t, root)
	m = key(t, m, 'd')
	m = key(t, m, 'y')
	item = expected[selectedID]
	item.Fields = item.Fields[:1]
	expected[selectedID] = item
	check("delete field", before, true)
	before = spaceSnapshot(t, root)
	m = key(t, m, 'r')
	m = typeText(t, m, "renamed")
	m = enter(t, m)
	item = expected[selectedID]
	item.Title = "renamed"
	expected[selectedID] = item
	check("rename account from inspector", before, true)
	m = key(t, m, tea.KeyEsc)
	before = spaceSnapshot(t, root)
	m = key(t, m, 'f')
	m = typeText(t, m, "outer")
	m = enter(t, m)
	check("create temporary folder", before, false)
	before = spaceSnapshot(t, root)
	m = addVaultItem(t, m, "nested", "nested-fake")
	for id := range diskItems(t, m) {
		if id != selectedID {
			expected[id] = vault.Item{Path: []string{"outer"}, Title: "nested", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "value", Value: []byte("nested-fake")}}}
		}
	}
	if len(expected) != 2 {
		t.Fatal("nested item missing")
	}
	check("persist folder", before, true)
	m = key(t, m, tea.KeyEsc)
	before = spaceSnapshot(t, root)
	m = key(t, m, 'r')
	m = typeText(t, m, "new-outer")
	m = enter(t, m)
	rootID := selectedID
	for id, item := range expected {
		if id != rootID {
			item.Path = []string{"new-outer"}
			expected[id] = item
			selectedID = id
		}
	}
	// Folder rename rewrites its descendants, not the root account.
	check("rename folder", before, true)
	m = enter(t, m)
	before = spaceSnapshot(t, root)
	m = key(t, m, 'd')
	m = key(t, m, tea.KeyEsc)
	check("cancel delete item", before, false)
	before = spaceSnapshot(t, root)
	m = key(t, m, 'd')
	m = key(t, m, 'y')
	delete(expected, selectedID)
	check("delete nested item and folder", before, true)
	selectedID = rootID
	before = spaceSnapshot(t, root)
	m = key(t, m, 'd')
	m = key(t, m, 'y')
	delete(expected, selectedID)
	check("delete root item", before, true)
}

func TestWideInspectorHeadingFitsTwoPanes(t *testing.T) {
	for _, tc := range []struct {
		name string
		path []string
		cut  bool
	}{
		{"exact width", []string{strings.Repeat("a", 35)}, false},
		{"width plus one", []string{strings.Repeat("a", 36)}, true},
		{"deep unicode", []string{"école-🙂-long-long", "mañana-🙂-long-long", "élève-🙂-long-long", "último-🙂-destino"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := spacesSession(t, English)
			_, err := m.vlt.Add(vault.Item{Path: tc.path, Title: "sample", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "value", Value: []byte("fake")}}})
			if err != nil {
				t.Fatal(err)
			}
			m.refreshSpaces()
			m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
			m = enter(t, m)
			for range tc.path {
				m = enter(t, m)
			}
			line := ""
			for _, row := range strings.Split(framed(m), "\n") {
				if strings.Contains(row, "[2] ") {
					line = row
					break
				}
			}
			if line == "" {
				t.Fatal("missing heading")
			}
			parts := strings.Split(line, "│")
			if len(parts) != 2 {
				t.Fatalf("heading columns: %q", line)
			}
			heading := strings.TrimSpace(parts[1])
			if !utf8.ValidString(heading) || lipgloss.Width(parts[1]) != 76 || lipgloss.Width(heading) > 74 || !strings.HasPrefix(heading, "[2] ACCOUNT // Personal / ") {
				t.Fatalf("invalid or overflowing inspector heading %q", heading)
			}
			if tc.cut && !strings.Contains(heading, "…") && !strings.Contains(heading, "sample") {
				t.Fatalf("long location was not represented: %q", heading)
			}
		})
	}
}
