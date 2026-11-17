package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// wide applies only to the three vault navigation screens. Forms retain their panel.
func (m Model) wide() bool {
	return m.width >= 100 && m.height >= 20 && (m.screen == spacesList || m.screen == vaultList || m.screen == vaultItem)
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

func (m Model) wideView() string {
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
			suffix = " (" + strconv.Itoa(m.spaceCounts[i]) + ")"
		}
		prefix := "  "
		if i == m.spaceCursor {
			prefix = "> "
		}
		name := wideTrim(m.spaceNames[i], left-4-lipgloss.Width(prefix)-lipgloss.Width(suffix))
		row := prefix + name + suffix
		if i == m.spaceCursor {
			row = m.styles.selected.Render(row)
		}
		spaces = append(spaces, row)
	}
	names := m.names
	if m.screen == spacesList && m.vlt != nil {
		// List decrypts names only. Never call Get on an unfocused pane.
		if listed, err := m.vlt.List(); err == nil {
			names = listed
		}
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
	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		m.panel(t.spacesTitle, pad(spaces), left, m.screen == spacesList, true), " ",
		m.panel(wideTrim(m.spaceTitle(), center-4), pad(items), center, m.screen == vaultList, true), " ",
		m.panel(t.itemTitle, pad(detail), right, m.screen == vaultItem, true))
	hints := t.spacesHint
	if m.screen == vaultList {
		if len(names) == 0 {
			hints = t.listEmptyHint
		} else {
			hints = t.listHint
		}
	}
	if m.screen == vaultItem {
		hints = t.itemHint
	}
	hints = strings.ReplaceAll(hints, "\n", "  ")
	hints = wideTrim(hints, m.width)
	status := m.message
	if status == "" {
		status = t.wideCount(len(names))
	}
	style := m.styles.hint
	if m.message != "" {
		style = m.styles.error
		if m.message == t.copied {
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
