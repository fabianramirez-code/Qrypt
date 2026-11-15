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
	welcome screen = iota
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
func New(path string) Model {
	m := Model{path: path, status: 1, generate: identity.Generate, pick: randomPositions}
	if _, err := os.Stat(path); err == nil {
		m.screen = existing
		m.status = 0
	} else if !errors.Is(err, os.ErrNotExist) {
		m.screen = notSaved
		m.message = err.Error()
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
			m.message = passError(v.err)
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
			m.message = "Pasting is not accepted. Type the words from your paper."
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
					m.message = fmt.Sprintf("That is not word %d. Check your paper.", position+1)
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
func passError(err error) string {
	switch {
	case errors.Is(err, identity.ErrPassphraseWordCount):
		return "Type exactly 7 words."
	case errors.Is(err, identity.ErrPassphraseUnknownWord):
		return "One of the words is not valid. Check the spelling."
	default:
		return "That is not the passphrase shown. Check your paper."
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
	m.message = err.Error()
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
		return "Enlarge the window to at least 60 columns by 20 rows.\n"
	}
	switch m.screen {
	case existing:
		return title.Render("Identity exists") + "\nIdentity file: " + m.path + "\nUnlocking is not available yet.\n" + hint.Render("Any key: exit")
	case welcome:
		return title.Render("Creating an identity") + "\n\nQrypt will give you two secrets.\n\nPassphrase (7 words)\n  You type it every time you open Qrypt on this device.\n\nRecovery phrase (24 words)\n  It rebuilds your identity if you lose this device\n  or forget the passphrase.\n\nWrite both on paper. They are shown only now,\nand nobody can reset them.\n\n" + hint.Render("Enter: create   Esc: quit")
	case waitingSeal:
		return title.Render("Creating an identity") + "\nGenerating and sealing your identity. Please wait."
	case passphrase:
		return m.passphraseView()
	case confirmPassphrase:
		return title.Render("Confirm passphrase") + "\nType the 7 words in order (visible):\n" + wrapLine(m.input) + "\n" + m.message + "\n" + hint.Render("Enter: check  Esc: back")
	case waitingOpen:
		return title.Render("Confirm passphrase") + "\nChecking your passphrase. Please wait."
	case recovery:
		return m.recoveryView()
	case confirmRecovery:
		return title.Render("Confirm recovery phrase") + fmt.Sprintf("\nWord %d of 3\nType word %d:\n", m.index+1, m.positions[m.index]+1) + wrapLine(m.input) + "\n" + m.message + "\n" + hint.Render("Enter: check  Esc: back")
	case saving:
		return title.Render("Saving") + "\nSaving your identity. Please wait."
	case done:
		return title.Render("Done") + "\nYour identity is ready.\n" + wrapLine("Identity file: "+m.path) + "\nKeep both papers safe.\n" + hint.Render("Any key: exit")
	case notSaved:
		return title.Render("Not saved") + "\nQrypt could not save your identity:\n" + wrapLine(m.message) + "\nNothing was saved.\nThe words you wrote down belong to no identity.\nDiscard them.\n" + hint.Render("Any key: exit")
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
	return title.Render("Passphrase") + "\nWrite these 7 words on paper, in order.\n" + secret.Render(strings.Join(lines, "\n")) + "\nYou will type them on the next screen.\n" + hint.Render("Enter: continue  Ctrl+C: quit")
}
func (m Model) recoveryView() string {
	var lines []string
	for i := 0; i < 8; i++ {
		lines = append(lines, fmt.Sprintf("%-17s %-17s %s", fmt.Sprintf("%d. %s", i+1, m.words[i]), fmt.Sprintf("%d. %s", i+9, m.words[i+8]), fmt.Sprintf("%d. %s", i+17, m.words[i+16])))
	}
	return title.Render("Recovery phrase") + "\nWrite these 24 words on paper, in order.\n" + secret.Render(strings.Join(lines, "\n")) + "\nKeep the paper somewhere safe, away from this device.\nDo not store it in a photo, a note or a file.\n" + hint.Render("Enter: continue  Ctrl+C: quit")
}
