package tui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// wide applies only to the three vault navigation screens; modals use their own renderer.
func (m Model) wide() bool {
	return m.width >= 100 && m.height >= 20 && (m.screen == spacesList || m.screen == vaultList || m.screen == vaultItem)
}

// wideKey translates focus navigation to the existing screen handlers.
func (m Model) wideKey(v tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	{
		switch {
		case m.screen == vaultItem && v.Code == tea.KeyRight:
			// The detail pane is the last one: Right has nowhere to go and must never reveal the value.
			return m, nil
		case m.screen == vaultItem && v.Code == tea.KeyTab && v.Mod&tea.ModShift == 0:
			// Tab wraps to the spaces pane through the same steps as Esc twice,
			// so the value is cleared and the space is left as the narrow flow leaves it.
			m.leaveItem()
			v = tea.KeyPressMsg{Code: tea.KeyEsc}
		case v.Code == tea.KeyTab && v.Mod&tea.ModShift != 0, v.Code == tea.KeyLeft:
			if m.screen == spacesList {
				return m, nil
			}
			v = tea.KeyPressMsg{Code: tea.KeyEsc}
		case v.Code == tea.KeyTab, v.Code == tea.KeyRight:
			v = tea.KeyPressMsg{Code: tea.KeyEnter}
		case m.screen == vaultItem && v.Code == tea.KeyEnter:
			v = tea.KeyPressMsg{Code: tea.KeyTab}
		}
		var cmd tea.Cmd
		var next tea.Model
		if isSpaceScreen(m.screen) {
			next, cmd = m.spacesKey(v)
		} else {
			next, cmd = m.vaultKey(v)
		}
		m = next.(Model)
		if m.vlt == nil || m.screen == problemScreen {
			return m, cmd
		}
		if m.screen != spacesList && m.screen != vaultList && m.screen != vaultItem {
			return m, cmd
		}
		m.refreshSpaces()
		if m.vlt != nil {
			m.refresh()
		}
		return m, cmd
	}
}

// wideTrim cuts names and UI text by display cells, never secret values.
func wideTrim(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	var b strings.Builder
	for _, r := range s {
		if lipgloss.Width(b.String()+string(r)) > width-1 {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + "…"
}

// modalPane is the navigation screen whose selection remains behind a modal.
func (m Model) modalPane() screen {
	switch m.screen {
	case vaultAddName, vaultAddValue, vaultDelete:
		return vaultList
	case vaultEdit:
		return vaultItem
	default:
		return spacesList
	}
}

func (m Model) isWideModal() bool {
	if m.width < 100 || m.height < 20 {
		return false
	}
	switch m.screen {
	case vaultAddName, vaultAddValue, vaultEdit, vaultDelete,
		spacesAdd, spacesRename, spacesDelete, problemScreen,
		mirrorScreen, mirrorEdit, mirrorStop:
		return true
	}
	return false
}

// wideModal composes the existing screen's content over masked, faint panes.
// A too-tall panel returns false so View can use the unchanged single-panel flow.
func (m Model) wideModal() (string, bool) {
	content := m.viewContent()
	parts := strings.SplitN(content, "\n", 2)
	if len(parts) < 2 {
		return "", false
	}
	body := parts[1] // Including the screen's existing key hints inside the modal.
	panel := m.panel(stripANSI(parts[0]), body, m.panelWidth(), true, true)
	base := m
	base.screen = m.modalPane()
	base.visible = false // Never draw a revealed detail value behind any modal.
	background := base.wideView(true)
	paneTop := 2
	if m.height >= 30 {
		paneTop = 7
	}
	paneBottom := m.height - 2
	if m.preview {
		paneBottom--
	}
	h := lipgloss.Height(panel)
	if h > paneBottom-paneTop || lipgloss.Width(panel) > m.width {
		return "", false
	}
	x := (m.width - lipgloss.Width(panel)) / 2
	y := paneTop + (paneBottom-paneTop-h)/2
	canvas := lipgloss.NewCanvas(m.width, m.height)
	canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(background).X(0).Y(0).Z(0),
		lipgloss.NewLayer(panel).X(x).Y(y).Z(1),
	))
	return canvas.Render(), true
}

func (m Model) wideView(modal bool) string {
	t := m.language.text()
	extra := m.width - 100
	left := 25 + extra*25/100
	center := 43 + extra*45/100
	right := m.width - left - center - 2
	header := m.styles.accent.Render("◆ Qrypt")
	if m.height >= 30 {
		header = m.logoView()
	}
	header += "\n" + m.styles.title.Render(wideTrim(m.spaceTitle(), m.width))
	banner := ""
	if m.preview {
		banner = m.styles.warning.Render(t.previewBanner)
	}
	paneHeight := m.height - strings.Count(header, "\n") - 1 - 2
	if banner != "" {
		paneHeight--
	}
	// A panel has a top, heading, divider and bottom border.
	rows := max(paneHeight-4, 0)
	spaces := make([]string, 0, rows)
	start := max(0, min(m.spaceCursor-rows/2, len(m.spaceNames)-rows))
	for i := start; i < len(m.spaceNames) && len(spaces) < rows; i++ {
		suffix := ""
		if i < len(m.spaceCounts) {
			if i < len(m.spaceRows) && m.spaceRows[i].problem < 0 {
				suffix = " (" + strconv.Itoa(m.spaceCounts[i]) + ")"
			}
		}
		prefix := "  "
		if i == m.spaceCursor {
			prefix = "> "
		}
		name := m.spaceNames[i]
		if i < len(m.spaceRows) && m.spaceRows[i].problem >= 0 {
			name = "! " + name
		}
		name = wideTrim(name, left-4-lipgloss.Width(prefix)-lipgloss.Width(suffix))
		row := prefix + name + suffix
		if i == m.spaceCursor {
			row = m.styles.selected.Render(row)
		}
		spaces = append(spaces, row)
	}
	names := m.names
	if m.selectedProblem() != nil {
		names = nil
	}
	items := make([]string, 0, rows)
	selected := min(m.cursor, max(0, len(names)-1))
	if m.screen == spacesList {
		selected = 0
	}
	first := max(0, min(selected-rows/2, len(names)-rows))
	for i := first; i < len(names) && len(items) < rows; i++ {
		prefix := "  "
		if i == selected {
			prefix = "> "
		}
		row := wideTrim(prefix+names[i], center-4)
		if i == selected {
			row = m.styles.selected.Render(row)
		}
		items = append(items, row)
	}
	detail := []string{}
	if m.screen != spacesList && len(names) > 0 {
		name := names[selected]
		if m.screen == vaultItem {
			name = m.item
		}
		detail = append(detail, wideTrim(name, right-4))
		if m.screen == vaultItem && m.visible {
			value := string(m.itemValue)
			if len([]rune(value)) > maxShownRunes || lipgloss.Width(value) > right-4 {
				detail = append(detail, strings.Split(lipgloss.Wrap(t.tooLong, right-4, ""), "\n")...)
			} else {
				detail = append(detail, value)
			}
		} else {
			detail = append(detail, "************")
		}
	}
	pad := func(lines []string) string {
		for len(lines) < rows {
			lines = append(lines, "")
		}
		return strings.Join(lines, "\n")
	}
	centerTitle := m.spaceTitle()
	if m.selectedProblem() != nil {
		centerTitle = ""
	}
	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		m.panel(t.spacesTitle, pad(spaces), left, m.screen == spacesList, true), " ",
		m.panel(wideTrim(centerTitle, center-4), pad(items), center, m.screen == vaultList, true), " ",
		m.panel(t.itemTitle, pad(detail), right, m.screen == vaultItem, true))
	if modal {
		panes = lipgloss.NewStyle().Faint(true).Render(panes)
	}
	hints := t.wideSpacesHint
	if m.screen == vaultList {
		hints = t.wideListHint
	}
	if m.screen == vaultItem {
		hints = t.wideItemHint
	}
	if modal {
		hints = ""
	}
	hints = strings.ReplaceAll(hints, "\n", "  ")
	hints = wideTrim(hints, m.width)
	status := m.message
	if status == "" && !modal {
		status = t.wideCount(len(names))
	}
	style := m.styles.hint
	if m.message != "" {
		style = m.styles.error
		if m.message == t.copied || m.mirrorWarning && m.message == m.mirrorResult {
			style = m.styles.warning
		}
	}
	status = wideTrim(status, m.width)
	footer := m.styles.hint.Render(hints)
	if lipgloss.Width(hints)+1+lipgloss.Width(status) <= m.width {
		footer += strings.Repeat(" ", m.width-lipgloss.Width(hints)-lipgloss.Width(status)) + style.Render(status) + "\n"
	} else {
		footer += "\n" + strings.Repeat(" ", m.width-lipgloss.Width(status)) + style.Render(status)
	}
	parts := []string{header, panes, footer}
	if banner != "" {
		parts = append(parts, banner)
	}
	return strings.Join(parts, "\n")
}
