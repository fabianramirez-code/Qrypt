// Package tui implements the alternate-screen identity creation flow.
package tui

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
)

type screen int

const (
	languageScreen screen = iota
	welcome
	waitingSeal
	passphrase
	confirmPassphrase
	waitingOpen
	recovery
	confirmRecovery
	saving
	done
	notSaved
	existing
)

type sealedMsg struct {
	id     *identity.Identity
	sealed []byte
	pass   string
	err    error
}
type openedMsg struct{ err error }
type savedMsg struct{ err error }

// Model holds the state of a single identity-creation session.
type Model struct {
	path          string
	language      Language
	start         screen
	screen        screen
	width, height int
	status        int
	id            *identity.Identity
	sealed        []byte
	pass          string
	words         []string
	input         string
	message       string
	positions     [3]int
	index         int
	generate      func() (*identity.Identity, error)
	pick          func() [3]int
}

// New constructs a session for the explicit identity path without writing to disk.
func New(path string, preferred Language) Model {
	m := Model{path: path, language: preferred, status: 1, generate: identity.Generate, pick: randomPositions}
	if _, err := os.Stat(path); err == nil {
		m.start = existing
	} else if !errors.Is(err, os.ErrNotExist) {
		m.start = notSaved
		m.message = err.Error()
	} else {
		m.start = welcome
	}
	return m
}

func randomPositions() [3]int {
	order := rand.Perm(24)
	return [3]int{order[0], order[1], order[2]}
}

// ExitStatus reports the result to use after the Bubble Tea program ends.
func (m Model) ExitStatus() int { return m.status }

// Init returns no command: creation starts only after Enter.
func (m Model) Init() tea.Cmd { return nil }

// Update processes terminal input and completion of slow work.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
	case sealedMsg:
		if m.screen != waitingSeal {
			return m, nil
		}
		if v.err != nil {
			m.fail(v.err)
			return m, nil
		}
		m.id, m.sealed, m.pass = v.id, v.sealed, v.pass
		m.words = strings.Fields(v.id.RecoveryPhrase())
		m.screen = passphrase
	case openedMsg:
		if m.screen != waitingOpen {
			return m, nil
		}
		if v.err != nil {
			m.screen = confirmPassphrase
			m.message = passError(v.err, m.language.text())
		} else {
			m.screen = recovery
		}
		m.input = ""
	case savedMsg:
		if m.screen != saving {
			return m, nil
		}
		m.clearSecrets()
		if v.err != nil {
			m.fail(v.err)
		} else {
			m.screen = done
			m.status = 0
		}
	case tea.PasteMsg:
		if m.screen == confirmPassphrase || m.screen == confirmRecovery {
			m.message = m.language.text().pasted
		}
	case tea.KeyPressMsg:
		if m.screen == saving {
			return m, nil
		}
		if v.Code == 'c' && v.Mod&tea.ModCtrl != 0 {
			m.clearSecrets()
			if m.screen != done && m.screen != existing {
				m.status = 1
			}
			return m, tea.Quit
		}
		if m.screen == done || m.screen == notSaved || m.screen == existing {
			return m, tea.Quit
		}
		switch m.screen {
		case languageScreen:
			switch v.Code {
			case tea.KeyEsc:
				return m, tea.Quit
			case tea.KeyUp:
				m.language = English
			case tea.KeyDown:
				m.language = Spanish
			case tea.KeyEnter:
				m.screen = m.start
				if m.screen == existing {
					m.status = 0
				}
			}
		case welcome:
			if v.Code == tea.KeyEsc {
				return m, tea.Quit
			}
			if v.Code == tea.KeyEnter {
				m.screen = waitingSeal
				generate := m.generate
				return m, func() tea.Msg {
					id, err := generate()
					if err != nil {
						return sealedMsg{err: err}
					}
					data, pass, err := id.Seal()
					return sealedMsg{id: id, sealed: data, pass: pass, err: err}
				}
			}
		case passphrase:
			if v.Code == tea.KeyEnter {
				m.screen = confirmPassphrase
				m.input = ""
				m.message = ""
			}
		case confirmPassphrase:
			if v.Code == tea.KeyEsc {
				m.screen = passphrase
				m.input = ""
				m.message = ""
				return m, nil
			}
			if v.Code == tea.KeyEnter {
				typed := []byte(m.input)
				m.input = ""
				m.message = ""
				m.screen = waitingOpen
				sealed := m.sealed
				return m, func() tea.Msg { _, err := identity.Open(sealed, typed); clear(typed); return openedMsg{err: err} }
			}
			m.editInput(v)
		case recovery:
			if v.Code == tea.KeyEnter {
				m.screen = confirmRecovery
				m.positions = m.pick()
				m.index = 0
				m.input = ""
				m.message = ""
			}
		case confirmRecovery:
			if v.Code == tea.KeyEsc {
				m.screen = recovery
				m.input = ""
				m.message = ""
				return m, nil
			}
			if v.Code == tea.KeyEnter {
				position := m.positions[m.index]
				if !strings.EqualFold(strings.TrimSpace(m.input), m.words[position]) {
					m.message = m.language.text().wrongWord(position + 1)
				} else {
					m.index++
					m.message = ""
				}
				m.input = ""
				if m.index == 3 {
					m.screen = saving
					path, data := m.path, m.sealed
					return m, func() tea.Msg { return savedMsg{err: identity.Save(path, data)} }
				}
				return m, nil
			}
			m.editInput(v)
		}
	}
	return m, nil
}

func (m *Model) editInput(key tea.KeyPressMsg) {
	if key.Text != "" {
		m.message = ""
		m.input += key.Text
		return
	}
	switch key.Code {
	case tea.KeySpace:
		m.message = ""
		m.input += " "
	case tea.KeyBackspace, tea.KeyDelete:
		r := []rune(m.input)
		if len(r) > 0 {
			m.message = ""
			m.input = string(r[:len(r)-1])
		}
	}
}
func passError(err error, text texts) string {
	switch {
	case errors.Is(err, identity.ErrPassphraseWordCount):
		return text.wordCount
	case errors.Is(err, identity.ErrPassphraseUnknownWord):
		return text.unknownWord
	default:
		return text.wrongPass
	}
}
func (m *Model) clearSecrets() {
	m.id = nil
	clear(m.sealed)
	m.sealed = nil
	m.pass = ""
	clear(m.words)
	m.words = nil
	m.input = ""
}
func (m *Model) fail(err error) {
	m.screen = notSaved
	m.status = 1
	if errors.Is(err, identity.ErrAlreadyExists) {
		m.message = m.language.text().alreadyExists
	} else {
		m.message = err.Error()
	}
	m.clearSecrets()
}

var title = lipgloss.NewStyle().Bold(true)
var hint = lipgloss.NewStyle().Faint(true)
var secret = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)

// View renders every state on the alternate screen, including small and waiting screens.
func (m Model) View() tea.View {
	view := tea.NewView(m.viewContent())
	view.AltScreen = true
	return view
}

func (m Model) viewContent() string {
	if m.width < 60 || m.height < 20 {
		if m.screen == languageScreen {
			return wrapLine("Enlarge the window to at least 60 by 20. / Agranda la ventana a 60 por 20 como mínimo.") + "\n"
		}
		return m.language.text().enlarge + "\n"
	}
	t := m.language.text()
	switch m.screen {
	case languageScreen:
		first, second := "  English", "  Español"
		if m.language == Spanish {
			second = "> Español"
		} else {
			first = "> English"
		}
		instruction := "↑/↓: move / mover   Enter: select / elegir   Esc: quit / salir"
		if m.width == 60 {
			instruction = wrapLine(instruction)
		}
		return title.Render("Language / Idioma") + "\n" + first + "\n" + second + "\n" + hint.Render(instruction)
	case existing:
		return title.Render(t.existingTitle) + "\n" + wrapLine(t.fileLabel+m.path) + "\n" + t.existingBody + "\n" + hint.Render(t.exitHint)
	case welcome:
		return title.Render(t.welcomeTitle) + "\n\n" + t.welcomeBody + "\n\n" + hint.Render(t.welcomeHint)
	case waitingSeal:
		return title.Render(t.welcomeTitle) + "\n" + t.generating
	case passphrase:
		return m.passphraseView()
	case confirmPassphrase:
		return title.Render(t.confirmPassTitle) + "\n" + t.confirmPassPrompt + "\n" + wrapLine(m.input) + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.checkHint)
	case waitingOpen:
		return title.Render(t.confirmPassTitle) + "\n" + t.checking
	case recovery:
		return m.recoveryView()
	case confirmRecovery:
		return title.Render(t.confirmRecoveryTitle) + "\n" + t.wordProgress(m.index+1) + "\n" + t.wordPrompt(m.positions[m.index]+1) + "\n" + wrapLine(m.input) + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.checkHint)
	case saving:
		return title.Render(t.savingTitle) + "\n" + t.savingBody
	case done:
		return title.Render(t.doneTitle) + "\n" + t.doneBody + "\n" + wrapLine(t.fileLabel+m.path) + "\n" + t.doneOutro + "\n" + hint.Render(t.exitHint)
	case notSaved:
		return title.Render(t.failedTitle) + "\n" + t.failedBody + "\n" + wrapLine(m.message) + "\n" + t.failedOutro + "\n" + t.discard + "\n" + hint.Render(t.exitHint)
	}
	return ""
}

// wrapLine keeps an arbitrary path or filesystem error within the minimum window width.
func wrapLine(text string) string {
	var result strings.Builder
	width := 0
	for _, char := range text {
		cellWidth := lipgloss.Width(string(char))
		if width+cellWidth > 60 {
			result.WriteByte('\n')
			width = 0
		}
		result.WriteRune(char)
		width += cellWidth
	}
	return result.String()
}

func (m Model) passphraseView() string {
	var lines []string
	for i, word := range strings.Fields(m.pass) {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, word))
	}
	t := m.language.text()
	return title.Render(t.passTitle) + "\n" + t.passIntro + "\n" + secret.Render(strings.Join(lines, "\n")) + "\n" + t.passOutro + "\n" + hint.Render(t.secretHint)
}
func (m Model) recoveryView() string {
	var lines []string
	for i := 0; i < 8; i++ {
		lines = append(lines, fmt.Sprintf("%-17s %-17s %s", fmt.Sprintf("%d. %s", i+1, m.words[i]), fmt.Sprintf("%d. %s", i+9, m.words[i+8]), fmt.Sprintf("%d. %s", i+17, m.words[i+16])))
	}
	t := m.language.text()
	return title.Render(t.recoveryTitle) + "\n" + t.recoveryIntro + "\n" + secret.Render(strings.Join(lines, "\n")) + "\n" + t.recoveryOutro + "\n" + hint.Render(t.secretHint)
}
