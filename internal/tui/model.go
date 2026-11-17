// Package tui implements the alternate-screen identity creation, unlock, and restore flows.
package tui

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/clipboard"
	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/settings"
	"github.com/fabianramirez-code/qrypt/internal/tui/theme"
	"github.com/fabianramirez-code/qrypt/internal/vault"
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
	unlock
	checkingUnlock
	vaultList
	spacesList
	spacesAdd
	spacesRename
	spacesDelete
	problemScreen
	vaultItem
	vaultAddName
	vaultAddValue
	vaultEdit
	vaultDelete
	vaultFailed
	cannotOpen
	restoreIntro
	restoreWord
	rebuilding
	restored
	notRestored
	colorEditor
)

type sealedMsg struct {
	id     *identity.Identity
	sealed []byte
	pass   string
	err    error
}
type openedMsg struct{ err error }
type savedMsg struct {
	backup string
	err    error
}
type rebuiltMsg struct {
	sealed []byte
	pass   string
	err    error
}
type clipboardTickMsg struct{ token uint64 }
type idleTickMsg struct{}
type logoTickMsg struct{}

const clipboardLifetime = 30 * time.Second
const idleLifetime = 5 * time.Minute
const logoFrame = 66 * time.Millisecond

func logoTick() tea.Cmd {
	return tea.Tick(logoFrame, func(time.Time) tea.Msg { return logoTickMsg{} })
}

func idleCheck() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return idleTickMsg{} })
}

func quitAfter(cmd tea.Cmd) tea.Cmd {
	if cmd != nil {
		return tea.Sequence(cmd, tea.Quit)
	}
	return tea.Quit
}

type unlockedMsg struct {
	id  *identity.Identity
	err error
}

// Model holds the state of a single identity creation, unlock, or restore session.
type Model struct {
	path            string
	settingsPath    string
	settings        settings.Settings
	settingsError   bool
	darkBackground  bool
	themeName       string
	styles          styles
	themeList       bool
	themeIndex      int
	colorIndex      int
	beforeEdit      settings.Settings
	editorColors    map[string]string
	editorOrigin    string
	editorPalette   theme.Theme
	preview         bool
	previewID       *identity.Identity
	language        Language
	initialLanguage Language
	start           screen
	screen          screen
	width, height   int
	status          int
	id              *identity.Identity
	sealed          []byte
	pass            string
	words           []string
	input           string
	message         string
	visible         bool
	positions       [3]int
	index           int
	restoring       bool
	restoreOrigin   screen
	restoreWords    []string
	backup          string
	vlt             *vault.Vault
	scopes          []*vault.Vault
	problems        []vault.Problem
	spaceRows       []spaceRow
	spaceNames      []string
	spaceCounts     []int
	scopeID         *identity.Identity
	spaceCursor     int
	names           []string
	cursor          int
	item            string
	itemValue       []byte
	newName         string
	generate        func() (*identity.Identity, error)
	pick            func() [3]int
	copyFn          func(string) error
	clearFn         func(string) error
	now             func() time.Time
	lastKey         time.Time
	logoPhase       int
	copyValue       string
	copyActive      bool
	copyToken       uint64
	copyFallback    bool

	themeBeforeCustom string
}

// New constructs a session for the explicit identity path without writing to disk.
func New(path string, preferred Language) Model {
	m := Model{path: path, settingsPath: filepath.Join(filepath.Dir(path), "settings"), language: preferred, themeName: "green", darkBackground: true, status: 1, generate: identity.Generate, pick: randomPositions, copyFn: clipboard.Copy, clearFn: clipboard.ClearIfUnchanged, now: time.Now}
	loaded, err := settings.Load(m.settingsPath)
	m.settings = loaded
	m.settingsError = err != nil
	if m.settings.Language == "es" {
		m.language = Spanish
	} else if m.settings.Language == "en" {
		m.language = English
	}
	if m.settings.Theme != "" {
		m.themeName = m.settings.Theme
	}
	m.initialLanguage = m.language
	for i := 0; i < len(theme.Names())+1; i++ {
		if themeChoice(i) == m.themeName {
			m.themeIndex = i
		}
	}
	m.refreshStyles()
	if _, err := os.Stat(path); err == nil {
		m.start = unlock
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

// Init requests the terminal background; identity work starts only after Enter.
func (m Model) Init() tea.Cmd { return tea.Batch(tea.RequestBackgroundColor, logoTick()) }

// Update processes terminal input and completion of slow work.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
		if m.scopeID != nil && isVaultScreen(m.screen) {
			m.refreshSpaces()
			if m.vlt != nil {
				m.refresh()
			}
		}
	case tea.BackgroundColorMsg:
		m.darkBackground = v.IsDark()
		m.refreshStyles()
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
	case rebuiltMsg:
		if m.screen != rebuilding {
			return m, nil
		}
		clear(m.restoreWords)
		m.restoreWords = nil
		m.index = 0
		m.input = ""
		if v.err != nil {
			m.screen = restoreWord
			m.message = m.language.text().invalidPhrase
		} else {
			m.sealed, m.pass = v.sealed, v.pass
			m.screen = passphrase
			m.message = ""
		}
	case openedMsg:
		if m.screen != waitingOpen {
			return m, nil
		}
		if v.err != nil {
			m.screen = confirmPassphrase
			m.message = passError(v.err, m.language.text())
		} else if m.restoring {
			m.screen = saving
			path, data := m.path, m.sealed
			return m, func() tea.Msg {
				backup, err := identity.SaveReplacing(path, data)
				return savedMsg{backup: backup, err: err}
			}
		} else {
			m.screen = recovery
		}
		m.input = ""
	case unlockedMsg:
		if m.screen != checkingUnlock {
			return m, nil
		}
		switch {
		case v.err == nil:
			m.input = ""
			m.message = ""
			m.openVault(v.id)
			if m.scopeID != nil {
				return m, idleCheck()
			}
		case errors.Is(v.err, identity.ErrPassphraseWordCount), errors.Is(v.err, identity.ErrPassphraseUnknownWord), errors.Is(v.err, identity.ErrDecryption):
			m.screen = unlock
			m.message = unlockError(v.err, m.language.text())
		default:
			m.input = ""
			m.screen = cannotOpen
			m.message = openFailure(v.err, m.language.text())
		}
	case savedMsg:
		if m.screen != saving {
			return m, nil
		}
		m.clearSecrets()
		if m.restoring {
			if v.err != nil {
				m.screen = notRestored
				m.message = restoreSaveError(v.err, m.language.text())
			} else {
				m.backup = v.backup
				m.screen = restored
				m.status = 0
			}
		} else if v.err != nil {
			m.fail(v.err)
		} else {
			m.screen = done
			m.status = 0
		}
	case clipboardTickMsg:
		if m.scopeID != nil && m.copyActive && v.token == m.copyToken {
			return m, m.clearCopied()
		}
	case logoTickMsg:
		// Never touches lastKey: the animation is not user activity.
		m.logoPhase++
		return m, logoTick()
	case idleTickMsg:
		if m.scopeID != nil {
			if !m.now().Before(m.lastKey.Add(idleLifetime)) {
				cmd := m.closeVault()
				m.clearSecrets()
				return m, quitAfter(cmd)
			}
			return m, idleCheck()
		}
	case tea.PasteMsg:
		if m.screen == confirmPassphrase || m.screen == confirmRecovery || m.screen == unlock || m.screen == restoreWord {
			m.message = m.language.text().pasted
		} else if m.screen == problemScreen && m.selectedProblem() != nil && m.selectedProblem().Kind != vault.ProblemUnreadable {
			m.message = m.language.text().problemPaste
		} else if m.screen == vaultAddName || m.screen == vaultAddValue || m.screen == vaultEdit || m.screen == spacesAdd || m.screen == spacesRename {
			m.vaultPaste(v.Content)
		}
	case tea.KeyPressMsg:
		if m.scopeID != nil {
			m.lastKey = m.now()
		}
		if m.screen == saving || (m.screen == checkingUnlock || m.screen == rebuilding) && !(v.Code == 'c' && v.Mod&tea.ModCtrl != 0) {
			return m, nil
		}
		if v.Code == 'c' && v.Mod&tea.ModCtrl != 0 {
			opened := m.scopeID != nil
			var clearCmd tea.Cmd
			if opened {
				clearCmd = m.closeVault()
			}
			if m.restoring && m.screen != restored && m.screen != notRestored {
				m.resetRestore()
			} else {
				m.clearSecrets()
			}
			if !opened && m.screen != done && m.screen != restored {
				m.status = 1
			}
			return m, quitAfter(clearCmd)
		}
		if m.screen == done || m.screen == notSaved || m.screen == vaultFailed || m.screen == cannotOpen || m.screen == restored || m.screen == notRestored {
			return m, tea.Quit
		}
		if isVaultScreen(m.screen) {
			if m.wide() {
				return m.wideKey(v)
			}
			if m.screen == problemScreen {
				return m.problemKey(v)
			}
			if isSpaceScreen(m.screen) {
				return m.spacesKey(v)
			}
			next, cmd := m.vaultKey(v)
			m = next.(Model)
			if m.vlt != nil && m.wide() {
				m.refreshSpaces()
			}
			return m, cmd
		}
		switch m.screen {
		case languageScreen:
			switch v.Code {
			case tea.KeyEsc:
				return m, tea.Quit
			case tea.KeyTab:
				m.themeList = !m.themeList
			case tea.KeyLeft:
				m.themeList = false
			case tea.KeyRight:
				m.themeList = true
			case tea.KeyUp, tea.KeyDown:
				if m.themeList {
					step := 1
					if v.Code == tea.KeyUp {
						step = -1
					}
					m.themeIndex = max(0, min(len(theme.Names())+1, m.themeIndex+step))
					if m.themeIndex < len(theme.Names())+1 {
						if m.themeIndex == len(theme.Names()) && m.themeName != "custom" {
							m.themeBeforeCustom = m.themeName
						}
						m.themeName = themeChoice(m.themeIndex)
						m.refreshStyles()
					}
				} else if v.Code == tea.KeyUp {
					m.language = English
				} else {
					m.language = Spanish
				}
			case tea.KeyEnter:
				if m.themeList && m.themeIndex == len(theme.Names())+1 {
					m.openColorEditor()
					break
				}
				m.savePreferences()
				if m.preview {
					m.startPreview()
					if m.vlt != nil {
						return m, idleCheck()
					}
				} else {
					m.screen = m.start
				}
			}
		case colorEditor:
			m.colorKey(v)
		case unlock:
			if v.Code == 'r' && v.Mod&tea.ModCtrl != 0 {
				m.beginRestore(unlock)
				return m, nil
			}
			if v.Code == tea.KeyEsc {
				m.input = ""
				return m, tea.Quit
			}
			if v.Code == tea.KeyTab {
				m.visible = !m.visible
				return m, nil
			}
			if v.Code == tea.KeyEnter {
				typed := []byte(m.input)
				path := m.path
				m.message = ""
				m.screen = checkingUnlock
				return m, func() tea.Msg {
					id, err := identity.Load(path, typed)
					clear(typed)
					return unlockedMsg{id: id, err: err}
				}
			}
			m.editInput(v)
		case welcome:
			if (v.Code == 'r' || v.Code == 'R') && v.Mod & ^(tea.ModShift|tea.ModCapsLock) == 0 {
				m.beginRestore(welcome)
				return m, nil
			}
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
		case restoreIntro:
			if v.Code == tea.KeyEsc {
				origin := m.restoreOrigin
				m.resetRestore()
				m.screen = origin
			} else if v.Code == tea.KeyEnter {
				m.screen = restoreWord
				m.message = ""
			}
		case restoreWord:
			return m.restoreWordKey(v)
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
func unlockError(err error, text texts) string {
	switch {
	case errors.Is(err, identity.ErrPassphraseWordCount):
		return text.wordCount
	case errors.Is(err, identity.ErrPassphraseUnknownWord):
		return text.unknownWord
	default:
		return text.unlockWrongPass
	}
}
func openFailure(err error, text texts) string {
	switch {
	case errors.Is(err, identity.ErrInvalidFormat):
		return text.invalidIdentity
	case errors.Is(err, identity.ErrUnsupportedVersion):
		return text.newerIdentity
	default:
		return err.Error()
	}
}
func (m *Model) clearSecrets() {
	m.id = nil
	m.previewID = nil
	clear(m.sealed)
	m.sealed = nil
	m.pass = ""
	clear(m.words)
	m.words = nil
	clear(m.restoreWords)
	m.restoreWords = nil
	m.input = ""
	m.visible = false
	m.closeVault()
	m.newName = ""
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

type styles struct {
	text, title, hint, secret, selected, error, success, warning, border lipgloss.Style
	accent, dim                                                          lipgloss.Style
	palette                                                              theme.Theme
}

func (m *Model) refreshStyles() {
	palette, ok := theme.ByName(m.themeName, m.darkBackground)
	if !ok {
		palette, _ = theme.ByName("green", true)
		palette = theme.Custom(palette, m.settings.Colors)
	}
	m.styles = styles{
		text: lipgloss.NewStyle(), title: lipgloss.NewStyle().Bold(true),
		hint:     lipgloss.NewStyle().Faint(true),
		secret:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
		selected: lipgloss.NewStyle().Reverse(true),
		error:    lipgloss.NewStyle().Bold(true), success: lipgloss.NewStyle().Bold(true),
		warning: lipgloss.NewStyle().Bold(true), border: lipgloss.NewStyle(),
		accent: lipgloss.NewStyle().Bold(true), dim: lipgloss.NewStyle().Faint(true), palette: palette,
	}
	if m.themeName != "mono" {
		m.styles.text = m.styles.text.Foreground(lipgloss.Color(hexColor(palette.Text)))
		m.styles.title = m.styles.title.Foreground(lipgloss.Color(hexColor(palette.Accent)))
		m.styles.hint = m.styles.hint.Foreground(lipgloss.Color(hexColor(palette.Dim)))
		m.styles.selected = m.styles.selected.Foreground(lipgloss.Color(hexColor(palette.Accent)))
		m.styles.error = m.styles.error.Foreground(lipgloss.Color(hexColor(palette.Error)))
		m.styles.success = m.styles.success.Foreground(lipgloss.Color(hexColor(palette.Success)))
		m.styles.warning = m.styles.warning.Foreground(lipgloss.Color(hexColor(palette.Warning)))
		m.styles.border = m.styles.border.Foreground(lipgloss.Color(hexColor(palette.Border)))
		m.styles.accent = m.styles.accent.Foreground(lipgloss.Color(hexColor(palette.Accent)))
		m.styles.dim = m.styles.dim.Foreground(lipgloss.Color(hexColor(palette.Dim)))
		m.styles.secret = m.styles.secret.BorderForeground(lipgloss.Color(hexColor(palette.Border)))
	}
}

// View renders every state on the alternate screen, including small and waiting screens.
func (m Model) View() tea.View {
	if m.wide() {
		view := tea.NewView(m.wideView())
		if m.styles.palette.Surface.A != 0 {
			view.BackgroundColor = m.styles.palette.Background
		}
		view.AltScreen = true
		return view
	}
	content := m.viewContent()
	if m.preview && (isVaultScreen(m.screen) || m.screen == vaultFailed) {
		content += "\n" + m.language.text().previewBanner
	}
	if m.width >= 60 && m.height >= 20 {
		if m.message != "" && m.screen != languageScreen {
			messageStyle := m.styles.error
			if m.message == m.language.text().copied {
				messageStyle = m.styles.warning
			}
			content = strings.Replace(content, wrapLine(m.message), messageStyle.Render(wrapLine(m.message)), 1)
		}
		if m.preview && (isVaultScreen(m.screen) || m.screen == vaultFailed) {
			content = strings.Replace(content, m.language.text().previewBanner, m.styles.warning.Render(m.language.text().previewBanner), 1)
		}
		content = m.frame(content)
	}
	view := tea.NewView(content)
	if m.styles.palette.Surface.A != 0 {
		view.BackgroundColor = m.styles.palette.Background
	}
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
	title, hint := m.styles.title, m.styles.hint
	switch m.screen {
	case languageScreen:
		return m.languageView()
	case colorEditor:
		return m.colorView()
	case unlock:
		field := m.input
		if !m.visible {
			field = strings.Repeat("*", len([]rune(field)))
		}
		return title.Render(t.unlockTitle) + "\n" + t.unlockPrompt + "\n" + wrapWidth(field, m.innerWidth()) + "\n" + t.typedWords(len(strings.Fields(m.input))) + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.unlockHint) + "\n" + hint.Render(t.unlockRestoreHint)
	case checkingUnlock:
		return title.Render(t.unlockTitle) + "\n" + t.checking
	case vaultList:
		return m.listView(t)
	case spacesList, spacesAdd, spacesRename, spacesDelete:
		return m.spacesView(t)
	case problemScreen:
		return m.problemView(t)
	case vaultItem:
		return title.Render(t.itemTitle) + "\n" + wrapLine(m.item) + "\n" + m.itemValueView(t) + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.itemHint)
	case vaultAddName:
		return title.Render(t.addTitle) + "\n" + t.namePrompt + "\n" + wrapWidth(m.input, m.innerWidth()) + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.addNameHint)
	case vaultAddValue:
		return title.Render(t.addTitle) + "\n" + wrapLine(m.newName) + "\n" + t.valuePrompt + "\n" + wrapWidth(hide(m.input, m.visible), m.innerWidth()) + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.valueHint)
	case vaultEdit:
		return title.Render(t.editTitle) + "\n" + wrapLine(m.item) + "\n" + t.valuePrompt + "\n" + wrapWidth(hide(m.input, m.visible), m.innerWidth()) + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.valueHint)
	case vaultDelete:
		return title.Render(t.deleteTitle) + "\n" + wrapLine(m.selectedName()) + "\n" + t.deleteQuestion + "\n" + hint.Render(t.deleteHint)
	case vaultFailed:
		return title.Render(t.vaultFailedTitle) + "\n" + wrapLine(m.message) + "\n" + wrapLine(t.vaultLabel+vaultDir(m.path)) + "\n" + hint.Render(t.exitHint)
	case cannotOpen:
		return title.Render(t.cannotOpenTitle) + "\n" + wrapLine(m.message) + "\n" + wrapLine(t.fileLabel+m.path) + "\n" + hint.Render(t.exitHint)
	case welcome:
		return title.Render(t.welcomeTitle) + "\n\n" + t.welcomeBody + "\n\n" + t.welcomeRestore + "\n" + hint.Render(t.welcomeHint)
	case waitingSeal:
		return title.Render(t.welcomeTitle) + "\n" + t.generating
	case restoreIntro:
		body := t.restoreIntro
		if _, err := os.Stat(m.path); err == nil {
			body += "\n\n" + t.restoreBackup
		}
		return title.Render(t.restoreTitle) + "\n\n" + body + "\n\n" + hint.Render(t.restoreIntroHint)
	case restoreWord:
		field := m.input
		if !m.visible {
			field = strings.Repeat("*", len([]rune(field)))
		}
		return title.Render(t.restoreWordTitle) + "\n" + t.restoreProgress(m.index+1) + "\n" + t.restorePrompt(m.index+1) + "\n" + wrapWidth(field, m.innerWidth()) + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.restoreWordHint)
	case rebuilding:
		return title.Render(t.restoreTitle) + "\n" + t.rebuilding
	case passphrase:
		return m.passphraseView()
	case confirmPassphrase:
		return title.Render(t.confirmPassTitle) + "\n" + t.confirmPassPrompt + "\n" + wrapWidth(m.input, m.innerWidth()) + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.checkHint)
	case waitingOpen:
		return title.Render(t.confirmPassTitle) + "\n" + t.checking
	case recovery:
		return m.recoveryView()
	case confirmRecovery:
		return title.Render(t.confirmRecoveryTitle) + "\n" + t.wordProgress(m.index+1) + "\n" + t.wordPrompt(m.positions[m.index]+1) + "\n" + wrapWidth(m.input, m.innerWidth()) + "\n" + wrapLine(m.message) + "\n" + hint.Render(t.checkHint)
	case saving:
		return title.Render(t.savingTitle) + "\n" + t.savingBody
	case restored:
		body := title.Render(t.restoredTitle) + "\n" + t.restoredBody + "\n" + wrapLine(t.fileLabel+m.path)
		if m.backup != "" {
			body += "\n" + wrapLine(t.backupLabel+m.backup)
		}
		return body + "\n" + t.previousPass + "\n" + t.keepNewPass + "\n" + hint.Render(t.exitHint)
	case notRestored:
		return title.Render(t.notRestoredTitle) + "\n" + t.notRestoredBody + "\n" + wrapLine(m.message) + "\n" + t.notRestoredOutro + "\n" + t.discardNew + "\n" + t.phraseStillValid + "\n" + hint.Render(t.exitHint)
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

func wrapWidth(text string, maxWidth int) string {
	var result strings.Builder
	width := 0
	for _, char := range text {
		cellWidth := lipgloss.Width(string(char))
		if width+cellWidth > maxWidth {
			result.WriteByte('\n')
			width = 0
		}
		result.WriteRune(char)
		width += cellWidth
	}
	return result.String()
}

func (m Model) passphraseView() string {
	title, hint, secret := m.styles.title, m.styles.hint, lipgloss.NewStyle().Bold(true)
	var lines []string
	for i, word := range strings.Fields(m.pass) {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, word))
	}
	t := m.language.text()
	passTitle := t.passTitle
	if m.restoring {
		passTitle = t.newPassTitle
	}
	return title.Render(passTitle) + "\n" + t.passIntro + "\n" + secret.Render(strings.Join(lines, "\n")) + "\n" + t.passOutro + "\n" + hint.Render(t.secretHint)
}
func (m Model) recoveryView() string {
	title, hint, secret := m.styles.title, m.styles.hint, lipgloss.NewStyle().Bold(true)
	var lines []string
	for i := 0; i < 8; i++ {
		lines = append(lines, fmt.Sprintf("%-17s %-17s %s", fmt.Sprintf("%d. %s", i+1, m.words[i]), fmt.Sprintf("%d. %s", i+9, m.words[i+8]), fmt.Sprintf("%d. %s", i+17, m.words[i+16])))
	}
	t := m.language.text()
	return title.Render(t.recoveryTitle) + "\n" + t.recoveryIntro + "\n" + secret.Render(strings.Join(lines, "\n")) + "\n" + t.recoveryOutro + "\n" + hint.Render(t.secretHint)
}
