package tui

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

// maxShownRunes is the longest value the Item screen can show without cutting it off.
const maxShownRunes = 300

func vaultDir(identityPath string) string {
	return filepath.Join(filepath.Dir(identityPath), "vault")
}

func isVaultScreen(s screen) bool {
	switch s {
	case vaultList, vaultItem, vaultAddName, vaultAddValue, vaultEdit, vaultDelete:
		return true
	}
	return false
}

// openVault opens, or first creates, the vault next to the identity file.
func (m *Model) openVault(id *identity.Identity) {
	dir := vaultDir(m.path)
	v, err := vault.Open(dir, id)
	if errors.Is(err, os.ErrNotExist) {
		v, err = vault.Create(dir, id)
	}
	if err != nil {
		m.failVault(err)
		return
	}
	m.vlt = v
	m.status = 0
	m.cursor = 0
	m.screen = vaultList
	m.refresh()
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

func (m *Model) closeVault() {
	if m.vlt != nil {
		m.vlt.Close()
		m.vlt = nil
	}
	clear(m.itemValue)
	m.itemValue = nil
	m.names = nil
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
			m.closeVault()
			return m, tea.Quit
		case v.Code == tea.KeyUp:
			m.cursor = max(m.cursor-1, 0)
		case v.Code == tea.KeyDown:
			m.cursor = min(m.cursor+1, max(len(m.names)-1, 0))
		case v.Code == tea.KeyEnter && len(m.names) > 0:
			m.enterItem(m.names[m.cursor])
		case plainKey(v, 'a'):
			m.input, m.message, m.visible = "", "", false
			m.screen = vaultAddName
		case plainKey(v, 'd') && len(m.names) > 0:
			m.screen = vaultDelete
		}
	case vaultItem:
		switch {
		case v.Code == tea.KeyEsc:
			m.leaveItem()
		case v.Code == tea.KeyTab:
			m.visible = !m.visible
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
	return strings.Repeat("*", len([]rune(text)))
}

func (m Model) itemValueView(t texts) string {
	runes := len([]rune(string(m.itemValue)))
	if runes > maxShownRunes {
		return t.tooLong
	}
	return wrapWidth(hide(string(m.itemValue), m.visible), m.width)
}

func (m Model) listView(t texts) string {
	if len(m.names) == 0 {
		return title.Render(t.listTitle) + "\n" + wrapLine(t.noItems) + "\n" + hint.Render(t.listEmptyHint)
	}
	rows := max(m.height-8, 3)
	first := max(min(m.cursor-rows/2, len(m.names)-rows), 0)
	var lines []string
	for i := first; i < min(first+rows, len(m.names)); i++ {
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		lines = append(lines, firstLine(prefix+m.names[i]))
	}
	return title.Render(t.listTitle) + "\n" + strings.Join(lines, "\n") + "\n" + hint.Render(t.listHint)
}

// firstLine fits a name on one row by cutting it, since the full name is shown on the Item screen.
func firstLine(text string) string {
	runes := []rune(text)
	if len(runes) > 58 {
		return string(runes[:57]) + "…"
	}
	return text
}

func (m Model) selectedName() string {
	if m.cursor < 0 || m.cursor >= len(m.names) {
		return ""
	}
	return m.names[m.cursor]
}
