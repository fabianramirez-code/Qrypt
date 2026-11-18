package tui

import (
	"errors"
	"path/filepath"
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
	case vaultList, vaultItem, vaultAddName, vaultAddValue, vaultEdit, vaultDelete, folderName, fieldLabel, fieldKind, fieldNote, vaultRename, fieldDelete, spacesList, spacesAdd, spacesRename, spacesDelete, problemScreen, mirrorScreen, mirrorEdit, mirrorStop:
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
	if err == nil && len(scopes) == 0 && len(problems) == 0 {
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
		id.Close()
		return
	}
	m.scopes = scopes
	m.problems = problems
	m.scopeID = id
	m.spaceCursor = 0
	m.status = 0
	m.cursor = 0
	m.screen = spacesList
	m.lastKey = m.now()
	m.refreshSpaces()
	if m.screen == spacesList {
		m.mirrorUpdate()
	}
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
	m.helpOpen = false
	cmd := m.clearCopied()
	for _, scope := range m.scopes {
		scope.Close()
	}
	m.scopes = nil
	m.problems = nil
	m.spaceRows = nil
	m.spaceNames = nil
	m.spaceCounts = nil
	m.vlt = nil
	m.scopeID.Close()
	m.scopeID = nil
	clear(m.itemValue)
	m.itemValue = nil
	m.names = nil
	m.clearItem()
	m.folderPath = nil
	m.folderRows = nil
	m.entries = nil
	return cmd
}

func (m *Model) clearCopied() tea.Cmd {
	if !m.copyActive {
		return nil
	}
	value, fallback := m.copyValue, m.copyFallback
	m.copyValue = nil
	m.copyActive = false
	m.copyFallback = false
	m.copyToken++
	if fallback {
		return tea.SetClipboard("")
	}
	_ = m.clearFn(string(value))
	clear(value)
	return nil
}

func (m *Model) copyValueOf(value []byte) tea.Cmd {
	text := string(value)
	err := m.copyFn(text)
	fallback := errors.Is(err, clipboard.ErrUnavailable)
	if err != nil && !fallback {
		m.message = err.Error()
		return nil
	}
	m.copyValue, m.copyFallback, m.copyActive = []byte(text), fallback, true
	m.copyToken++
	token := m.copyToken
	m.message = m.language.text().copied
	tick := tea.Tick(clipboardLifetime, func(time.Time) tea.Msg { return clipboardTickMsg{token: token} })
	if fallback {
		return tea.Batch(tea.SetClipboard(text), tick)
	}
	return tick
}

func plainKey(v tea.KeyPressMsg, letter rune) bool {
	return (v.Code == letter || v.Code == letter-'a'+'A') && v.Mod & ^(tea.ModShift|tea.ModCapsLock) == 0
}

func (m *Model) leaveItem() {
	m.clearItem()
	m.input.Wipe()
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
		m.message = t.itemTooLarge
	case errors.Is(err, vault.ErrLabelExists):
		m.message = t.labelExists
	case errors.Is(err, vault.ErrTooManyFields):
		m.message = t.tooManyFields
	case errors.Is(err, vault.ErrPathTooLong):
		m.message = t.pathTooLong
	case errors.Is(err, vault.ErrInvalidField):
		m.message = t.lineBreak
	default:
		m.failVault(err)
	}
}

func (m Model) vaultKey(v tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.screen == vaultList {
		return m.folderKey(v)
	}
	return m.fieldsKey(v)
}

// vaultPaste accepts pasted text into a vault field, unless it breaks a line.
func (m *Model) vaultPaste(text string) {
	if strings.ContainsAny(text, "\r\n") {
		m.message = m.language.text().lineBreak
		return
	}
	m.message = ""
	m.input.Append(text)
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

func (m Model) spaceTitle() string {
	if m.spaceCursor >= 0 && m.spaceCursor < len(m.spaceNames) {
		return m.spaceNames[m.spaceCursor]
	}
	return m.language.text().listTitle
}
