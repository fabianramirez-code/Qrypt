package tui

import (
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
)

// resetRestore discards an abandoned restore attempt before another flow can start.
func (m *Model) resetRestore() {
	m.clearSecrets()
	m.restoring = false
	m.restoreOrigin = languageScreen
	m.index = 0
	m.visible = false
	m.message = ""
	m.backup = ""
}

func (m *Model) beginRestore(origin screen) {
	m.resetRestore()
	m.restoring = true
	m.restoreOrigin = origin
	m.screen = restoreIntro
}

func (m Model) restoreWordKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEsc:
		m.input = ""
		m.message = ""
		m.visible = false
		if m.index == 0 {
			m.screen = restoreIntro
		} else {
			m.index--
			m.restoreWords[m.index] = ""
			m.restoreWords = m.restoreWords[:m.index]
		}
	case tea.KeyTab:
		m.visible = !m.visible
	case tea.KeyEnter:
		word := strings.ToLower(strings.TrimSpace(m.input))
		if !identity.InWordList(word) {
			m.message = m.language.text().invalidWord
			return m, nil
		}
		m.restoreWords = append(m.restoreWords, word)
		m.index++
		m.input = ""
		m.message = ""
		m.visible = false
		if m.index == 24 {
			phrase := strings.Join(m.restoreWords, " ")
			m.screen = rebuilding
			return m, func() tea.Msg {
				id, err := identity.FromRecoveryPhrase(phrase)
				if err != nil {
					return rebuiltMsg{err: err}
				}
				sealed, pass, err := id.Seal()
				id.Close()
				return rebuiltMsg{sealed: sealed, pass: pass, err: err}
			}
		}
	default:
		m.editInput(key)
	}
	return m, nil
}

func restoreSaveError(err error, text texts) string {
	switch {
	case errors.Is(err, identity.ErrBackupExists):
		return text.backupExists
	case errors.Is(err, identity.ErrAlreadyExists):
		return text.alreadyExists
	default:
		return err.Error()
	}
}
