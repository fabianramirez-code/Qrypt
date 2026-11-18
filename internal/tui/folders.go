package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type folderRow struct {
	name, id string
	folder   bool
	count    int
}

func samePath(a, b []string) bool { return slices.Equal(a, b) }
func hasPath(path, prefix []string) bool {
	return len(path) >= len(prefix) && samePath(path[:len(prefix)], prefix)
}
func (m Model) location() string {
	return strings.Join(append([]string{m.spaceTitle()}, m.folderPath...), " / ")
}

func (m *Model) refresh() {
	if m.vlt == nil {
		return
	}
	entries, err := m.vlt.Entries()
	if err != nil {
		m.failVault(err)
		return
	}
	m.entries = entries
	folderCounts := map[string]int{}
	rows := []folderRow{}
	m.names = nil
	for _, e := range entries {
		if !hasPath(e.Path, m.folderPath) {
			continue
		}
		if len(e.Path) > len(m.folderPath) {
			folderCounts[e.Path[len(m.folderPath)]]++
			continue
		}
		rows = append(rows, folderRow{name: e.Title, id: e.ID})
	}
	slices.SortFunc(rows, func(a, b folderRow) int { return strings.Compare(a.name, b.name) })
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if len(e.Path) == 0 && len(m.folderPath) == 0 {
			m.names = append(m.names, e.Title)
		}
		if len(e.Path) > 0 && len(m.folderPath) == 0 {
			continue
		}
	}
	slices.Reverse(m.names)
	folders := make([]folderRow, 0, len(folderCounts))
	for name, count := range folderCounts {
		folders = append(folders, folderRow{name: name, count: count, folder: true})
	}
	slices.SortFunc(folders, func(a, b folderRow) int { return strings.Compare(a.name, b.name) })
	m.folderRows = append(folders, rows...)
	m.cursor = min(m.cursor, max(0, len(m.folderRows)-1))
}

func (m Model) selectedRow() folderRow {
	if m.cursor < 0 || m.cursor >= len(m.folderRows) {
		return folderRow{}
	}
	return m.folderRows[m.cursor]
}
func (m Model) folderView(t texts) string {
	lines := []string{m.styles.title.Render(m.spaceTitle()), wrapLine(m.location())}
	if len(m.folderRows) == 0 {
		lines = append(lines, wrapLine(t.noItems))
	}
	rows := max(m.height-14, 3)
	first := max(0, min(m.cursor-rows/2, len(m.folderRows)-rows))
	for i := first; i < len(m.folderRows) && i < first+rows; i++ {
		row := m.folderRows[i]
		glyph := "item"
		if row.folder {
			glyph = "spaceClosed"
		}
		label := IconGlyph(m.icons, glyph) + " " + row.name
		if row.folder {
			label += fmt.Sprintf(" (%d)", row.count)
		}
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		line := wideTrim(prefix+label, m.innerWidth())
		if i == m.cursor {
			line = m.styles.selected.Render(line)
		}
		lines = append(lines, line)
	}
	if len(m.folderRows) == 0 && len(m.folderPath) > 0 {
		lines = append(lines, wrapLine(t.folderTemporary))
	}
	lines = append(lines, wrapLine(m.message), m.styles.hint.Render(t.folderHint))
	return strings.Join(lines, "\n")
}

func (m *Model) enterFolder(name string) {
	m.folderPath = append(slices.Clone(m.folderPath), name)
	m.cursor = 0
	m.message = ""
	m.refresh() // The view itself says that an empty folder is temporary.
}
func (m *Model) leaveFolder() {
	m.message = ""
	if len(m.folderPath) > 0 {
		m.folderPath = slices.Clone(m.folderPath[:len(m.folderPath)-1])
		m.cursor = 0
		m.refresh()
	} else {
		m.screen = spacesList
		m.refreshSpaces()
	}
}
func (m *Model) renameFolder(name string) error {
	from := append(slices.Clone(m.folderPath), m.selectedRow().name)
	to := append(slices.Clone(m.folderPath), name)
	return m.vlt.MoveFolder(from, to)
}

func (m Model) folderKey(v tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	row := m.selectedRow()
	switch {
	case v.Code == tea.KeyEsc:
		m.leaveFolder()
	case v.Code == tea.KeyUp:
		m.cursor = max(0, m.cursor-1)
	case v.Code == tea.KeyDown:
		m.cursor = min(max(0, len(m.folderRows)-1), m.cursor+1)
	case v.Code == tea.KeyEnter && row.folder:
		m.enterFolder(row.name)
	case v.Code == tea.KeyEnter && row.id != "":
		m.enterEntry(row)
	case plainKey(v, 'a'):
		m.input, m.newName, m.message = "", "", ""
		m.screen = vaultAddName
	case plainKey(v, 'f'):
		m.input, m.message = "", ""
		m.screen = folderName
	case plainKey(v, 'r') && (row.folder || row.id != ""):
		m.input, m.message = "", ""
		m.screen = vaultRename
	case plainKey(v, 'd') && row.folder:
		m.message = m.language.text().folderDeleteRefused
	case plainKey(v, 'd') && row.id != "":
		m.deleteID = row.id
		m.screen = vaultDelete
	case plainKey(v, 'c') && row.id != "":
		return m, m.copyEntry(row.id)
	}
	return m, nil
}
