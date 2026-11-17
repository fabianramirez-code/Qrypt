package tui

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fabianramirez-code/qrypt/internal/clipboard"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

// maxShownRunes is the longest value the Item screen can show without cutting it off.
const maxShownRunes = 300

func vaultDir(identityPath string) string {
	return filepath.Join(filepath.Dir(identityPath), "vault")
}
func stateFile(identityPath string) string { return filepath.Join(filepath.Dir(identityPath), "state") }

func isVaultScreen(s screen) bool {
	switch s {
	case vaultList, vaultItem, vaultAddName, vaultAddValue, vaultEdit, vaultDelete, spacesList, spacesAdd, spacesRename, spacesDelete:
		return true
	}
	return false
}

func isSpaceScreen(s screen) bool {
	return s == spacesList || s == spacesAdd || s == spacesRename || s == spacesDelete
}

// openVault opens every scope, or first creates Personal next to the identity file.
func (m *Model) openVault(id *identity.Identity) {
	dir := vaultDir(m.path)
	scopes, problems, err := vault.OpenScopes(dir, stateFile(m.path), id)
	if err == nil && len(problems) != 0 {
		err = &problems[0]
	}
	if err == nil && len(scopes) == 0 {
		var first *vault.Vault
		first, err = vault.NewScope(dir, stateFile(m.path), id, "Personal")
		if err == nil {
			scopes = append(scopes, first)
		}
	}
	if err != nil {
		for _, scope := range scopes {
			scope.Close()
		}
		m.failVault(err)
		return
	}
	m.scopes = scopes
	m.scopeID = id
	m.spaceCursor = 0
	m.selectSpace()
	m.status = 0
	m.cursor = 0
	m.screen = spacesList
	m.lastKey = m.now()
	m.refreshSpaces()
}

func (m *Model) failVault(err error) {
	m.closeVault()
	m.screen = vaultFailed
	m.status = 1
	switch {
	case errors.Is(err, vault.ErrDecryption), errors.Is(err, vault.ErrInvalidFormat), errors.Is(err, vault.ErrUnsupportedVersion):
		m.message = m.language.text().vaultDamaged
	default:
		m.message = err.Error()
	}
}

func (m *Model) closeVault() tea.Cmd {
	cmd := m.clearCopied()
	for _, scope := range m.scopes {
		scope.Close()
	}
	m.scopes = nil
	m.spaceNames = nil
	m.spaceCounts = nil
	m.vlt = nil
	m.scopeID = nil
	clear(m.itemValue)
	m.itemValue = nil
	m.names = nil
	return cmd
}

func (m *Model) clearCopied() tea.Cmd {
	if !m.copyActive {
		return nil
	}
	value, fallback := m.copyValue, m.copyFallback
	m.copyValue = ""
	m.copyActive = false
	m.copyFallback = false
	m.copyToken++
	if fallback {
		return tea.SetClipboard("")
	}
	_ = m.clearFn(value)
	return nil
}

func (m *Model) copyItem(name string) tea.Cmd {
	value := m.itemValue
	if m.screen == vaultList {
		var err error
		value, err = m.vlt.Get(name)
		if err != nil {
			m.failVault(err)
			return nil
		}
		defer clear(value)
	}
	text := string(value)
	err := m.copyFn(text)
	fallback := errors.Is(err, clipboard.ErrUnavailable)
	if err != nil && !fallback {
		m.message = err.Error()
		return nil
	}
	m.copyValue, m.copyFallback, m.copyActive = text, fallback, true
	m.copyToken++
	token := m.copyToken
	m.message = m.language.text().copied
	tick := tea.Tick(clipboardLifetime, func(time.Time) tea.Msg { return clipboardTickMsg{token: token} })
	if fallback {
		return tea.Batch(tea.SetClipboard(text), tick)
	}
	return tick
}

// refresh reloads the names, keeping the cursor inside the list.
func (m *Model) refresh() {
	names, err := m.vlt.List()
	if err != nil {
		m.failVault(err)
		return
	}
	m.names = names
	m.cursor = min(m.cursor, max(len(names)-1, 0))
}

func plainKey(v tea.KeyPressMsg, letter rune) bool {
	return (v.Code == letter || v.Code == letter-'a'+'A') && v.Mod & ^(tea.ModShift|tea.ModCapsLock) == 0
}

func (m *Model) enterItem(name string) {
	value, err := m.vlt.Get(name)
	if err != nil {
		m.failVault(err)
		return
	}
	m.item, m.itemValue = name, value
	m.visible = false
	m.message = ""
	m.screen = vaultItem
}

func (m *Model) leaveItem() {
	clear(m.itemValue)
	m.itemValue = nil
	m.visible = false
	m.input = ""
	m.message = ""
	m.screen = vaultList
}

// writeError turns a rejected write into an on-screen message, or a failure screen.
func (m *Model) writeError(err error) {
	t := m.language.text()
	switch {
	case errors.Is(err, vault.ErrInvalidName):
		m.message = t.invalidName
	case errors.Is(err, vault.ErrNameExists):
		m.message = t.nameExists
	case errors.Is(err, vault.ErrValueTooLarge):
		m.message = t.valueTooLarge
	default:
		m.failVault(err)
	}
}

func (m Model) vaultKey(v tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case vaultList:
		switch {
		case v.Code == tea.KeyEsc:
			m.screen = spacesList
			m.message = ""
			m.refreshSpaces()
		case v.Code == tea.KeyUp:
			m.cursor = max(m.cursor-1, 0)
		case v.Code == tea.KeyDown:
			m.cursor = min(m.cursor+1, max(len(m.names)-1, 0))
		case v.Code == tea.KeyEnter && len(m.names) > 0:
			m.enterItem(m.names[m.cursor])
		case plainKey(v, 'a'):
			m.input, m.message, m.visible = "", "", false
			m.screen = vaultAddName
		case plainKey(v, 'c') && len(m.names) > 0:
			return m, m.copyItem(m.selectedName())
		case plainKey(v, 'd') && len(m.names) > 0:
			m.screen = vaultDelete
		}
	case vaultItem:
		switch {
		case v.Code == tea.KeyEsc:
			m.leaveItem()
		case v.Code == tea.KeyTab:
			m.visible = !m.visible
		case plainKey(v, 'c'):
			return m, m.copyItem(m.item)
		case plainKey(v, 'e'):
			m.input, m.message, m.visible = "", "", false
			m.screen = vaultEdit
		}
	case vaultAddName:
		switch v.Code {
		case tea.KeyEsc:
			m.input, m.message = "", ""
			m.screen = vaultList
		case tea.KeyEnter:
			switch {
			case vault.ValidateName(m.input) != nil:
				m.message = m.language.text().invalidName
			case slices.Contains(m.names, m.input):
				m.message = m.language.text().nameExists
			default:
				m.newName, m.input, m.message = m.input, "", ""
				m.screen = vaultAddValue
			}
		default:
			m.editInput(v)
		}
	case vaultAddValue:
		switch v.Code {
		case tea.KeyEsc:
			m.input, m.message, m.visible = m.newName, "", false
			m.screen = vaultAddName
		case tea.KeyTab:
			m.visible = !m.visible
		case tea.KeyEnter:
			if err := m.vlt.Put(m.newName, []byte(m.input)); err != nil {
				m.writeError(err)
				return m, nil
			}
			name := m.newName
			m.input, m.newName, m.message, m.visible = "", "", "", false
			m.screen = vaultList
			m.refresh()
			if m.vlt != nil {
				m.cursor = max(slices.Index(m.names, name), 0)
			}
		default:
			m.editInput(v)
		}
	case vaultEdit:
		switch v.Code {
		case tea.KeyEsc:
			m.input, m.message, m.visible = "", "", false
			m.screen = vaultItem
		case tea.KeyTab:
			m.visible = !m.visible
		case tea.KeyEnter:
			if err := m.vlt.Update(m.item, []byte(m.input)); err != nil {
				m.writeError(err)
				return m, nil
			}
			clear(m.itemValue)
			m.itemValue = []byte(m.input)
			m.input, m.message, m.visible = "", "", false
			m.screen = vaultItem
		default:
			m.editInput(v)
		}
	case vaultDelete:
		if plainKey(v, 'y') {
			if err := m.vlt.Delete(m.selectedName()); err != nil {
				m.failVault(err)
				return m, nil
			}
			m.refresh()
		}
		if m.vlt != nil {
			m.screen = vaultList
		}
	}
	return m, nil
}

// vaultPaste accepts pasted text into a vault field, unless it breaks a line.
func (m *Model) vaultPaste(text string) {
	if strings.ContainsAny(text, "\r\n") {
		m.message = m.language.text().lineBreak
		return
	}
	m.message = ""
	m.input += text
}

func hide(text string, revealed bool) string {
	if revealed {
		return text
	}
	count := len([]rune(text))
	if count > maxShownRunes {
		// Only the mask is abbreviated; the entered value remains intact.
		return strings.Repeat("*", maxShownRunes) + "…"
	}
	return strings.Repeat("*", count)
}

func (m Model) itemValueView(t texts) string {
	runes := len([]rune(string(m.itemValue)))
	if runes > maxShownRunes {
		return t.tooLong
	}
	return wrapWidth(hide(string(m.itemValue), m.visible), m.innerWidth())
}

func (m Model) listView(t texts) string {
	title, hint := m.styles.title, m.styles.hint
	if len(m.names) == 0 {
		return title.Render(m.spaceTitle()) + "\n" + wrapLine(t.noItems) + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.listEmptyHint)
	}
	rows := max(m.height-12, 3)
	first := max(min(m.cursor-rows/2, len(m.names)-rows), 0)
	var lines []string
	for i := first; i < min(first+rows, len(m.names)); i++ {
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		row := firstLine(prefix + m.names[i])
		if i == m.cursor {
			row = m.styles.selected.Render(row)
		}
		lines = append(lines, row)
	}
	return title.Render(m.spaceTitle()) + "\n" + strings.Join(lines, "\n") + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.listHint)
}

// firstLine fits a name on one row by cutting it, since the full name is shown on the Item screen.
func firstLine(text string) string {
	runes := []rune(text)
	if len(runes) > 58 {
		return string(runes[:57]) + "…"
	}
	return text
}

func (m Model) spaceTitle() string {
	if m.spaceCursor >= 0 && m.spaceCursor < len(m.spaceNames) {
		return m.spaceNames[m.spaceCursor]
	}
	return m.language.text().listTitle
}

func (m Model) selectedName() string {
	if m.cursor < 0 || m.cursor >= len(m.names) {
		return ""
	}
	return m.names[m.cursor]
}
