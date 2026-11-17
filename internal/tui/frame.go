package tui

import "strings"

// frame uses a light title and bottom rule so tall secret screens fit 60 by 20.
// The secret-word box retains its rounded border; no side border obscures words.
func (m Model) frame(content string) string {
	for len(strings.Split(content, "\n"))+3 > 20 && strings.Contains(content, "\n\n") {
		content = strings.Replace(content, "\n\n", "\n", 1)
	}
	if len(strings.Split(content, "\n"))+3 > m.height {
		// Never render only part of a secret when the available rows are exhausted.
		content = m.language.text().enlarge
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = m.styles.text.Render(line)
	}
	return m.styles.title.Render("◆ Qrypt") + "\n" + strings.Join(lines, "\n") + "\n" +
		m.styles.border.Render(strings.Repeat("─", 60)) + "\n" +
		m.styles.hint.Render("Ctrl+C: quit / salir")
}

// innerWidth is the usable width inside the light frame (no side padding).
func (m Model) innerWidth() int {
	return min(m.width, 60)
}
