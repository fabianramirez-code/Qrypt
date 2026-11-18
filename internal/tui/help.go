package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

// textFieldActive distinguishes a typed question mark from the help shortcut.
func (m Model) textFieldActive() bool {
	switch m.screen {
	case unlock, confirmPassphrase, confirmRecovery, restoreWord, colorEditor,
		spacesAdd, spacesRename, mirrorEdit, vaultAddName, vaultAddValue,
		vaultEdit, folderName, fieldLabel, fieldNote, vaultRename:
		return true
	case problemScreen:
		return m.selectedProblem() != nil && m.selectedProblem().Kind != vault.ProblemUnreadable
	}
	return false
}

// helpKeys reuses the screen's translated key hints; only states without a
// regular hint and actions omitted from compact bars need additional copy.
func (m Model) helpKeys(t texts) string {
	switch m.screen {
	case languageScreen:
		return "↑/↓: move / mover  ←/→/Tab: list / lista\nI: icons / iconos  A: animation / animación\nEnter: select / elegir  Esc: quit / salir"
	case colorEditor:
		if m.language == Spanish {
			return "↑/↓: rol  Enter: aplicar\nCtrl+R: restablecer rol\nCtrl+A: restablecer todo  Esc: guardar"
		}
		return "↑/↓: role  Enter: apply\nCtrl+R: reset role\nCtrl+A: reset all  Esc: save"
	case welcome:
		return t.welcomeHint
	case waitingSeal, waitingOpen, checkingUnlock, rebuilding:
		return m.helpWaiting()
	case saving:
		return m.helpWaiting()
	case passphrase, recovery:
		return t.secretHint
	case confirmPassphrase, confirmRecovery:
		return t.checkHint
	case unlock:
		return t.unlockHint + "\n" + t.unlockRestoreHint
	case restoreIntro:
		return t.restoreIntroHint
	case restoreWord:
		return t.restoreWordHint
	case done, notSaved, restored, notRestored, cannotOpen, vaultFailed:
		return t.exitHint
	case spacesList:
		if m.width >= 100 {
			return t.wideSpacesHint
		}
		return t.spacesHint
	case spacesAdd, spacesRename:
		return t.spaceNameHint
	case spacesDelete:
		return t.deleteHint
	case problemScreen:
		if m.selectedProblem() != nil && m.selectedProblem().Kind == vault.ProblemUnreadable {
			return t.problemBack
		}
		return t.problemHint
	case mirrorScreen:
		return t.mirrorHint
	case mirrorEdit:
		return t.mirrorPathHint
	case mirrorStop:
		return t.mirrorStopHint
	case vaultList:
		if m.width >= 100 {
			return t.wideListHint + "\n" + t.folderHint
		}
		return t.folderHint
	case vaultItem:
		if m.width >= 100 {
			return t.wideItemHint + "\n" + t.fieldsHint
		}
		return t.fieldsHint
	case vaultAddName, folderName, vaultRename, fieldLabel:
		return t.addNameHint
	case fieldKind:
		if m.language == Spanish {
			return t.kindPrompt + "  Esc: volver"
		}
		return t.kindPrompt + "  Esc: back"
	case vaultAddValue, vaultEdit:
		return t.valueHint
	case fieldNote:
		return t.noteHint
	case vaultDelete, fieldDelete:
		return t.deleteHint
	}
	return ""
}

func (m Model) helpWaiting() string {
	if m.language == Spanish {
		return "F1: ayuda  Espera: teclas desactivadas"
	}
	return "F1: help  Wait: keys disabled"
}

func (m Model) helpView() string {
	title := "Help"
	closeHint := "Esc / ? / F1: close"
	more := "↑/↓: scroll"
	if m.screen == languageScreen {
		title = "Help / Ayuda"
		closeHint = "Esc / ? / F1: close / cerrar"
		more = "↑/↓: scroll / desplazar"
	} else if m.language == Spanish {
		title = "Ayuda"
		closeHint = "Esc / ? / F1: cerrar"
		more = "↑/↓: desplazar"
	}
	keys := m.helpKeys(m.language.text())
	var lines []string
	for _, line := range strings.Split(keys, "\n") {
		lines = append(lines, strings.Split(lipgloss.Wrap(line, m.innerWidth(), ""), "\n")...)
	}
	// Keep the modal short enough for the minimum window and let Up/Down
	// paginate instead of cutting off a key or the closing instruction.
	pageSize := max(1, min(9, m.height-9))
	pages := max(1, (len(lines)+pageSize-1)/pageSize)
	page := min(m.helpPage, pages-1)
	start := page * pageSize
	end := min(start+pageSize, len(lines))
	body := strings.Join(lines[start:end], "\n")
	if pages > 1 {
		body += "\n" + more
	}
	body += "\n" + closeHint
	panel := m.panel(title, body, m.panelWidth(), true, true)
	if m.width >= 100 && m.height >= 20 && (m.wide() || m.isWideModal()) {
		base := m
		base.helpOpen = false
		base.hideRevealed()
		if !base.wide() {
			base.screen = m.modalPane()
		}
		if background, ok := m.composeModal(panel, base.wideView(true)); ok {
			return background
		}
	}
	return m.styles.accent.Render("◆ Qrypt") + "\n" + panel
}
