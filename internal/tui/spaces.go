package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

type spaceRow struct {
	scope   *vault.Vault
	problem int // -1 for a healthy space
	name    string
	count   int
}

// selectSpace keeps the current opened scope and the selected list row together.
func (m *Model) selectSpace() {
	if len(m.spaceRows) == 0 {
		m.vlt = nil
		m.spaceCursor = 0
		return
	}
	m.spaceCursor = min(max(m.spaceCursor, 0), len(m.spaceRows)-1)
	m.vlt = m.spaceRows[m.spaceCursor].scope
}

func (m *Model) sortSpaces(selected *vault.Vault) error {
	m.refreshSpaces()
	if m.screen == vaultFailed {
		return errors.New(m.message)
	}
	for i, row := range m.spaceRows {
		if row.scope == selected {
			m.spaceCursor = i
			break
		}
	}
	m.selectSpace()
	return nil
}

// refreshSpaces verifies opened scopes and caches only space names and counts.
func (m *Model) refreshSpaces() {
	rows := make([]spaceRow, 0, len(m.scopes)+len(m.problems))
	selected := m.vlt
	selectedID := ""
	if m.spaceCursor < len(m.spaceRows) && m.spaceRows[m.spaceCursor].problem >= 0 {
		selectedID = m.problems[m.spaceRows[m.spaceCursor].problem].ScopeID
	}
	for _, scope := range m.scopes {
		name, err := scope.Name()
		if err != nil {
			m.failVault(err)
			return
		}
		items, err := scope.List()
		if err != nil {
			m.failVault(err)
			return
		}
		rows = append(rows, spaceRow{scope: scope, problem: -1, name: name, count: len(items)})
	}
	for i, p := range m.problems {
		rows = append(rows, spaceRow{problem: i, name: problemName(p, m.language.text())})
	}
	slices.SortFunc(rows, func(a, b spaceRow) int { return strings.Compare(a.name, b.name) })
	m.spaceRows = rows
	m.spaceNames = make([]string, len(rows))
	m.spaceCounts = make([]int, len(rows))
	for i, row := range rows {
		m.spaceNames[i], m.spaceCounts[i] = row.name, row.count
		if row.scope != nil && row.scope == selected || row.problem >= 0 && m.problems[row.problem].ScopeID == selectedID {
			m.spaceCursor = i
		}
	}
	m.selectSpace()
}

func (m *Model) spaceError(err error) {
	switch {
	case errors.Is(err, vault.ErrInvalidName):
		m.message = m.language.text().invalidName
	case errors.Is(err, vault.ErrScopeNameExists):
		m.message = m.language.text().spaceExists
	case errors.Is(err, vault.ErrScopeNotEmpty):
		m.message = m.language.text().spaceNotEmpty
	default:
		m.failVault(err)
	}
}

func (m Model) spacesKey(v tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case spacesList:
		switch {
		case v.Code == tea.KeyEsc:
			cmd := m.closeVault()
			m.clearSecrets()
			return m, quitAfter(cmd)
		case v.Code == tea.KeyUp:
			m.spaceCursor = max(m.spaceCursor-1, 0)
			m.selectSpace()
		case v.Code == tea.KeyDown:
			m.spaceCursor = min(m.spaceCursor+1, max(len(m.spaceRows)-1, 0))
			m.selectSpace()
		case v.Code == tea.KeyEnter && m.selectedProblem() != nil:
			m.openProblem()
		case v.Code == tea.KeyEnter && m.vlt != nil:
			m.cursor = 0
			m.message = ""
			m.screen = vaultList
			m.refresh()
		case plainKey(v, 'm'):
			m.input = ""
			m.message = ""
			m.screen = mirrorScreen
		case plainKey(v, 'a') && m.selectedProblem() == nil:
			m.input = ""
			m.message = ""
			m.screen = spacesAdd
		case plainKey(v, 'r') && m.vlt != nil:
			m.input = ""
			m.message = ""
			m.screen = spacesRename
		case plainKey(v, 'd') && m.vlt != nil:
			m.message = ""
			m.screen = spacesDelete
		}
	case spacesAdd, spacesRename:
		if v.Code == tea.KeyEsc {
			m.input = ""
			m.message = ""
			m.screen = spacesList
			break
		}
		if v.Code != tea.KeyEnter {
			m.editInput(v)
			break
		}
		var err error
		if m.screen == spacesAdd {
			var newScope *vault.Vault
			newScope, err = vault.NewScope(vaultDir(m.path), stateFile(m.path), m.scopeID, m.input)
			if err == nil {
				m.scopes = append(m.scopes, newScope)
				err = m.sortSpaces(newScope)
			}
		} else {
			err = vault.RenameScope(vaultDir(m.path), stateFile(m.path), m.scopeID, m.vlt, m.input)
			if err == nil {
				err = m.sortSpaces(m.vlt)
			}
		}
		if err != nil {
			m.spaceError(err)
			break
		}
		m.input = ""
		m.message = ""
		m.screen = spacesList
		m.refreshSpaces()
		if m.screen == spacesList {
			m.mirrorUpdate()
		}
	case spacesDelete:
		if plainKey(v, 'y') {
			deleted := m.vlt
			if err := vault.RemoveScope(deleted); err != nil {
				m.spaceError(err)
				break
			}
			index := slices.Index(m.scopes, deleted)
			m.scopes = slices.Delete(m.scopes, index, index+1)
			m.mirrorRemoved(deleted.ID())
			m.spaceRows = nil
			m.selectSpace()
		}
		if !m.mirrorWarning {
			m.message = ""
		}
		m.screen = spacesList
		m.refreshSpaces()
	}
	return m, nil
}

func (m Model) spacesView(t texts) string {
	title, hint := m.styles.title, m.styles.hint
	switch m.screen {
	case spacesAdd:
		return title.Render(t.spaceAddTitle) + "\n" + t.namePrompt + "\n" + wrapWidth(m.input, m.innerWidth()) + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.spaceNameHint)
	case spacesRename:
		name := ""
		if m.vlt != nil {
			name, _ = m.vlt.Name()
		}
		return title.Render(t.spaceRenameTitle) + "\n" + wrapLine(name) + "\n" + t.namePrompt + "\n" + wrapWidth(m.input, m.innerWidth()) + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.spaceNameHint)
	case spacesDelete:
		name := ""
		if m.vlt != nil {
			name, _ = m.vlt.Name()
		}
		return title.Render(t.spaceDeleteTitle) + "\n" + wrapLine(name) + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.deleteHint)
	}
	rows := max(m.height-12, 3)
	first := max(min(m.spaceCursor-rows/2, len(m.spaceRows)-rows), 0)
	var lines []string
	for i := first; i < min(first+rows, len(m.spaceRows)); i++ {
		if i >= len(m.spaceNames) || i >= len(m.spaceCounts) {
			continue
		}
		prefix := "  "
		if i == m.spaceCursor {
			prefix = "> "
		}
		suffix := ""
		if m.spaceRows[i].problem < 0 {
			suffix = fmt.Sprintf(" (%d)", m.spaceCounts[i])
		}
		name := []rune(m.spaceNames[i])
		if m.spaceRows[i].problem >= 0 {
			name = []rune("! " + m.spaceNames[i])
		}
		limit := 58 - len([]rune(prefix)) - len([]rune(suffix))
		if len(name) > limit {
			name = append(name[:limit-1:limit-1], '…')
		}
		row := prefix + string(name) + suffix
		if i == m.spaceCursor {
			row = m.styles.selected.Render(row)
		}
		lines = append(lines, row)
	}
	return title.Render(t.spacesTitle) + "\n" + strings.Join(lines, "\n") + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.spacesHint)
}
