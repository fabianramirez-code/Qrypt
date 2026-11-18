package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/mirror"
	"github.com/fabianramirez-code/qrypt/internal/settings"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

// mirrorTarget is replaceable in tests; the default is an owner-only local directory.
func (m Model) mirrorTarget() func(string) mirror.Storage {
	if m.mirrorStorage != nil {
		return m.mirrorStorage
	}
	return mirror.Dir
}

func (m *Model) mirrorPathAllowed(path string) bool {
	if !m.preview {
		return true
	}
	base, err := filepath.Abs(filepath.Dir(m.path))
	if err != nil {
		return false
	}
	candidate, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(base, candidate)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && candidate != base
}

func (m *Model) mirrorUpdate() {
	if m.settings.Mirror == "" || m.scopeID == nil {
		return
	}
	t := m.language.text()
	if fresh, err := m.mirrorIsForFreshDevice(); err != nil {
		m.mirrorFailure(err.Error())
		return
	} else if fresh {
		// Copying this device's empty spaces would mix them with the vault the
		// folder holds. The user restores first; nothing is written until then.
		m.mirrorWarning = false
		m.mirrorResult = t.mirrorHoldsVault
		return
	}
	report, err := mirror.Push(vaultDir(m.path), stateFile(m.path), m.scopeID, m.mirrorTarget()(m.settings.Mirror))
	if err != nil {
		m.mirrorFailure(err.Error())
		return
	}
	if len(report.Foreign) > 0 {
		m.mirrorFailure(t.mirrorForeign(len(report.Foreign)))
		return
	}
	if len(report.Ahead) > 0 {
		m.mirrorFailure(t.mirrorAhead(len(report.Ahead)))
		return
	}
	if m.mirrorWarning && m.message == m.mirrorResult {
		m.message = ""
	}
	m.mirrorWarning = false
	m.mirrorResult = t.mirrorUpdated(report.Updated, report.Unchanged)
	if len(report.Unknown) > 0 {
		m.mirrorResult += "\n" + t.mirrorUnknown(len(report.Unknown))
	}
	if report.LocalProblems > 0 {
		m.mirrorResult += "\n" + t.mirrorProblems(report.LocalProblems)
	}
}

// mirrorIsForFreshDevice reports whether the device holds no item while the
// mirror holds spaces, none of which the device has: a new device whose mirror
// was written elsewhere.
func (m *Model) mirrorIsForFreshDevice() (bool, error) {
	if len(m.problems) > 0 {
		return false, nil
	}
	local := map[string]bool{}
	for _, scope := range m.scopes {
		names, err := scope.Entries()
		if err != nil {
			return false, nil // The update itself reports a scope that cannot be listed.
		}
		if len(names) > 0 {
			return false, nil
		}
		local[scope.ID()] = true
	}
	files, err := m.mirrorTarget()(m.settings.Mirror).List()
	if err != nil {
		return false, err
	}
	remote := 0
	for _, name := range files {
		parts := strings.Split(name, "/")
		if len(parts) < 3 || parts[0] != "scopes" {
			continue
		}
		if local[parts[1]] {
			return false, nil
		}
		remote++
	}
	return remote > 0, nil
}

func (m *Model) mirrorFailure(reason string) {
	m.mirrorWarning = true
	m.mirrorResult = m.language.text().mirrorFailed + reason
	m.message = m.mirrorResult
}
func (m *Model) mirrorRemoved(id string) {
	if m.settings.Mirror == "" {
		return
	}
	if err := mirror.RemoveScope(m.mirrorTarget()(m.settings.Mirror), id); err != nil {
		m.mirrorFailure(err.Error())
		return
	}
	m.mirrorUpdate()
}
func (m *Model) mirrorReload() error {
	scopes, problems, err := vault.OpenScopes(vaultDir(m.path), stateFile(m.path), m.scopeID)
	if err != nil {
		return err
	}
	for _, scope := range m.scopes {
		scope.Close()
	}
	m.scopes, m.problems = scopes, problems
	m.vlt = nil
	m.spaceRows = nil
	m.spaceCursor = 0
	m.cursor = 0
	m.names = nil
	m.refreshSpaces()
	return nil
}
func (m Model) mirrorKey(v tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	t := m.language.text()
	switch m.screen {
	case mirrorScreen:
		switch {
		case v.Code == tea.KeyEsc:
			m.screen = spacesList
			m.message = ""
		case plainKey(v, 'e'):
			m.screen = mirrorEdit
			m.input = m.settings.Mirror
			m.message = ""
		case plainKey(v, 'x'):
			m.screen = mirrorStop
			m.message = ""
		case plainKey(v, 'r'):
			if m.settings.Mirror == "" {
				break
			}
			err := mirror.Restore(m.mirrorTarget()(m.settings.Mirror), vaultDir(m.path), stateFile(m.path), m.scopeID)
			if err != nil {
				switch {
				case errors.Is(err, mirror.ErrNotEmpty):
					m.message = t.mirrorNotEmpty
				case errors.Is(err, mirror.ErrIncomplete):
					m.message = t.mirrorIncomplete
				case errors.Is(err, mirror.ErrForeign):
					m.message = t.mirrorRestoreForeign
				case errors.Is(err, mirror.ErrEmpty):
					m.message = t.mirrorEmpty
				default:
					m.message = err.Error()
				}
			} else if err = m.mirrorReload(); err != nil {
				m.failVault(err)
			} else {
				m.message = t.mirrorRestored
				m.mirrorUpdate()
				m.screen = spacesList
			}
		}
	case mirrorEdit:
		if v.Code == tea.KeyEsc {
			m.input = ""
			m.message = ""
			m.screen = mirrorScreen
			break
		}
		if v.Code != tea.KeyEnter {
			m.editInput(v)
			break
		}
		path := m.input
		if path == "" {
			m.message = t.mirrorNoFolder
			break
		}
		if !m.mirrorPathAllowed(path) {
			m.message = t.mirrorPreviewRefused
			break
		}
		err := os.MkdirAll(path, 0700)
		if err != nil {
			m.message = err.Error()
			break
		}
		old := m.settings
		m.settings.Mirror = path
		if err = settings.Save(m.settingsPath, m.settings); err != nil {
			m.settings = old
			m.message = err.Error()
			break
		}
		m.input = ""
		m.message = ""
		m.screen = mirrorScreen
		m.mirrorUpdate()
	case mirrorStop:
		if plainKey(v, 'y') {
			old := m.settings
			m.settings.Mirror = ""
			if err := settings.Save(m.settingsPath, m.settings); err != nil {
				m.settings = old
				m.message = err.Error()
			} else {
				m.mirrorResult = ""
				m.mirrorWarning = false
				m.message = ""
			}
		} else {
			m.message = ""
		}
		m.screen = mirrorScreen
	}
	return m, nil
}
func (m Model) mirrorView(t texts) string {
	lines := []string{m.styles.title.Render(t.mirrorTitle)}
	if m.screen == mirrorEdit {
		lines = append(lines, t.mirrorPathPrompt, wrapWidth(m.input, m.innerWidth()), wrapLine(m.message), m.styles.hint.Render(t.mirrorPathHint))
		return strings.Join(lines, "\n")
	}
	folder := m.settings.Mirror
	if folder == "" {
		folder = t.mirrorNoFolder
	}
	lines = append(lines, wrapLine(folder))
	if m.screen == mirrorStop {
		lines = append(lines, wrapLine(t.mirrorStopPrompt), wrapLine(m.message), m.styles.hint.Render(t.mirrorStopHint))
		return strings.Join(lines, "\n")
	}
	if m.mirrorResult != "" {
		lines = append(lines, wrapLine(m.mirrorResult))
	}
	if m.message != "" && m.message != m.mirrorResult {
		lines = append(lines, wrapLine(m.message))
	}
	lines = append(lines, m.styles.hint.Render(t.mirrorHint))
	return strings.Join(lines, "\n")
}
