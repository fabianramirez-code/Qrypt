package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

func problemName(p vault.Problem, t texts) string {
	if p.Name != "" {
		return p.Name
	}
	return t.unknownSpace + " " + p.ScopeID[:min(len(p.ScopeID), 8)]
}

func (m Model) selectedProblem() *vault.Problem {
	if m.spaceCursor < 0 || m.spaceCursor >= len(m.spaceRows) {
		return nil
	}
	i := m.spaceRows[m.spaceCursor].problem
	if i < 0 || i >= len(m.problems) {
		return nil
	}
	return &m.problems[i]
}

func (m *Model) openProblem() {
	m.input, m.message = "", ""
	m.names = nil
	m.screen = problemScreen
}

func (m Model) problemKey(v tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.selectedProblem()
	if v.Code == tea.KeyEsc {
		m.input, m.message = "", ""
		m.screen = spacesList
		return m, nil
	}
	if p == nil || p.Kind == vault.ProblemUnreadable {
		return m, nil
	}
	if v.Code != tea.KeyEnter {
		m.editInput(v)
		return m, nil
	}
	if m.input != m.language.text().problemWord {
		m.message = m.language.text().problemWrong
		return m, nil
	}
	id := p.ScopeID
	if err := vault.Accept(vaultDir(m.path), stateFile(m.path), m.scopeID, id); err != nil {
		m.message = err.Error()
		return m, nil
	}
	scopes, problems, err := vault.OpenScopes(vaultDir(m.path), stateFile(m.path), m.scopeID)
	if err != nil {
		m.failVault(err)
		return m, nil
	}
	for _, scope := range m.scopes {
		scope.Close()
	}
	m.scopes, m.problems = scopes, problems
	m.spaceRows = nil
	m.spaceCursor = 0
	m.vlt = nil
	m.input, m.message = "", ""
	m.screen = spacesList
	m.refreshSpaces()
	for i, row := range m.spaceRows {
		if row.scope != nil && row.scope.ID() == id {
			m.spaceCursor = i
			break
		}
	}
	m.selectSpace()
	if m.screen == spacesList {
		m.mirrorUpdate()
	}
	return m, nil
}

func (m Model) problemView(t texts) string {
	p := m.selectedProblem()
	if p == nil {
		return ""
	}
	lines := []string{m.styles.title.Render(t.problemTitle), wrapLine(problemName(*p, t))}
	switch p.Kind {
	case vault.ProblemSignature:
		lines = append(lines, wrapLine(t.problemSignature))
	case vault.ProblemFork:
		lines = append(lines, wrapLine(t.problemFork), t.problemGeneration(p.Generation), t.problemRecorded(p.Recorded))
	case vault.ProblemRollback:
		lines = append(lines, wrapLine(t.problemRollback), t.problemGeneration(p.Generation), t.problemRecorded(p.Recorded))
	case vault.ProblemRemoved:
		lines = append(lines, wrapLine(t.problemRemoved), wrapLine(t.problemRemoveRecord))
	case vault.ProblemUnreadable:
		lines = append(lines, wrapLine(t.problemUnreadable), wrapLine(m.message), m.styles.hint.Render(t.problemBack))
		return strings.Join(lines, "\n")
	}
	if p.Kind == vault.ProblemFiles || p.Kind == vault.ProblemSignature {
		lines = append(lines, t.problemMissing(uint64(p.Missing)), t.problemUnexpected(uint64(p.Unexpected)), t.problemChanged(uint64(p.Changed)), t.problemUndecryptable(uint64(p.Undecryptable)))
	}
	lines = append(lines, wrapLine(t.problemAccept), t.problemPrompt, wrapWidth(m.input, m.innerWidth()), wrapLine(m.message), m.styles.hint.Render(t.problemHint))
	return strings.Join(lines, "\n")
}
