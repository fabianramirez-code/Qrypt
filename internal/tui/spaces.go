package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

// selectSpace keeps the current opened scope and the selected list row together.
func (m *Model) selectSpace() {
	if len(m.scopes) == 0 {
		m.vlt = nil
		return
	}
	m.spaceCursor = min(max(m.spaceCursor, 0), len(m.scopes)-1)
	m.vlt = m.scopes[m.spaceCursor]
}

func (m *Model) sortSpaces(selected *vault.Vault) error {
	names := make(map[*vault.Vault]string, len(m.scopes))
	for _, v := range m.scopes {
		n, err := v.Name()
		if err != nil {
			return err
		}
		names[v] = n
	}
	slices.SortFunc(m.scopes, func(a, b *vault.Vault) int { return strings.Compare(names[a], names[b]) })
	m.spaceCursor = max(slices.Index(m.scopes, selected), 0)
	m.selectSpace()
	return nil
}

// refreshSpaces verifies every opened scope and caches only public names and counts.
func (m *Model) refreshSpaces() {
	names := make([]string, 0, len(m.scopes))
	counts := make([]int, 0, len(m.scopes))
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
		names = append(names, name)
		counts = append(counts, len(items))
	}
	m.spaceNames, m.spaceCounts = names, counts
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
			m.spaceCursor = min(m.spaceCursor+1, len(m.scopes)-1)
			m.selectSpace()
		case v.Code == tea.KeyEnter && m.vlt != nil:
			m.cursor = 0
			m.message = ""
			m.screen = vaultList
			m.refresh()
		case plainKey(v, 'a'):
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
	case spacesDelete:
		if plainKey(v, 'y') {
			deleted := m.vlt
			if err := vault.RemoveScope(deleted); err != nil {
				m.spaceError(err)
				break
			}
			m.scopes = slices.Delete(m.scopes, m.spaceCursor, m.spaceCursor+1)
			m.selectSpace()
		}
		m.message = ""
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
	first := max(min(m.spaceCursor-rows/2, len(m.scopes)-rows), 0)
	var lines []string
	for i := first; i < min(first+rows, len(m.scopes)); i++ {
		if i >= len(m.spaceNames) || i >= len(m.spaceCounts) {
			continue
		}
		prefix := "  "
		if i == m.spaceCursor {
			prefix = "> "
		}
		suffix := fmt.Sprintf(" (%d)", m.spaceCounts[i])
		name := []rune(m.spaceNames[i])
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
