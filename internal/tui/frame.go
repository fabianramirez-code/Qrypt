package tui

import (
	_ "embed"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

//go:embed logo.txt
var logo string

// panelWidth grows to 64 columns so that every 60-column text line fits unwrapped
// inside the borders; at the 60-column minimum, long lines wrap between words.
func (m Model) panelWidth() int { return min(max(m.width, 60), 64) }

func (m Model) panel(title, body string, width int, focused, rule bool) string {
	inner := width - 4
	border := m.styles.accent
	header := m.styles.accent
	if !focused {
		border, header = m.styles.dim, m.styles.dim
	}
	edges := lipgloss.RoundedBorder()
	if m.themeName == "contrast" {
		edges = lipgloss.ThickBorder()
	}
	var lines []string
	lines = append(lines, border.Render(edges.TopLeft+strings.Repeat(edges.Top, width-2)+edges.TopRight))
	heading := title
	if lipgloss.Width(heading) > inner {
		heading = wrapWidth(heading, inner)
	}
	heading = strings.Repeat(" ", (inner-lipgloss.Width(heading))/2) + heading
	lines = append(lines, border.Render(edges.Left)+" "+header.Render(lipgloss.NewStyle().Bold(true).Render(heading))+strings.Repeat(" ", inner-lipgloss.Width(heading))+" "+border.Render(edges.Right))
	if rule {
		lines = append(lines, border.Render("├"+strings.Repeat("─", width-2)+"┤"))
	}
	for _, original := range strings.Split(body, "\n") {
		if lipgloss.Width(original) > inner {
			original = lipgloss.Wrap(original, inner, "")
		}
		for _, line := range strings.Split(original, "\n") {
			lines = append(lines, border.Render(edges.Left)+" "+line+strings.Repeat(" ", max(0, inner-lipgloss.Width(line)))+" "+border.Render(edges.Right))
		}
	}
	lines = append(lines, border.Render(edges.BottomLeft+strings.Repeat(edges.Bottom, width-2)+edges.BottomRight))
	return strings.Join(lines, "\n")
}

func (m Model) logoView() string {
	rows := strings.Split(strings.TrimSuffix(logo, "\n"), "\n")
	var result []string
	accent := m.styles.palette.Accent
	for _, row := range rows {
		row = strings.TrimRight(row, " ")
		if m.themeName == "mono" {
			result = append(result, m.styles.accent.Render(row))
			continue
		}
		var line strings.Builder
		for i, r := range []rune(row) {
			ratio := float64(i) / float64(max(len([]rune(row))-1, 1))
			mix := func(x uint8) uint8 { return uint8(float64(x)*(1-ratio*0.42) + 255*ratio*0.42) }
			tint := color.RGBA{R: mix(accent.R), G: mix(accent.G), B: mix(accent.B), A: 255}
			line.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(hexColor(tint))).Render(string(r)))
		}
		result = append(result, line.String())
	}
	return strings.Join(result, "\n")
}

func (m Model) bottomBar() string {
	if m.screen == languageScreen {
		return m.styles.hint.Render("↑/↓: move / mover   ←/→/Tab: list / lista\nEnter: select / elegir   Esc: quit / salir")
	}
	if m.language == Spanish {
		return m.styles.hint.Render("Ctrl+C: salir")
	}
	return m.styles.hint.Render("Ctrl+C: quit")
}

// frame draws the same headed panel and separate key hints on every full-size screen.
func (m Model) frame(content string) string {
	width := m.panelWidth()
	var panel, hints string
	if m.screen == languageScreen {
		panel = content
	} else {
		parts := strings.SplitN(content, "\n", 2)
		title := parts[0]
		body := ""
		if len(parts) > 1 {
			body = parts[1]
		}
		// The final key line belongs beneath the panel, not inside its body.
		if at := strings.LastIndex(body, "\n"); at >= 0 {
			hints, body = body[at+1:], body[:at]
		}
		// A second hint is used by unlock; keep it beside the first below the panel.
		if m.screen == unlock {
			if at := strings.LastIndex(body, "\n"); at >= 0 {
				hints, body = body[at+1:]+"\n"+hints, body[:at]
			}
		}
		panel = m.panel(stripANSI(title), body, width, true, true)
	}
	// Shorter windows lose decoration, never content: logo, header rule, bottom bar, title line.
	render := func(level int) string {
		p := panel
		if level >= 2 && m.screen != languageScreen {
			rows := strings.Split(panel, "\n")
			if len(rows) >= 3 {
				p = strings.Join(append(rows[:2:2], rows[3:]...), "\n")
			}
		}
		var parts []string
		switch {
		case level == 0:
			parts = append(parts, m.logoView())
		case level < 4:
			parts = append(parts, m.styles.accent.Render("◆ Qrypt"))
		}
		parts = append(parts, p)
		if hints != "" {
			parts = append(parts, hints)
		}
		if level < 3 {
			parts = append(parts, m.bottomBar())
		}
		return strings.Join(parts, "\n")
	}
	for level := 0; level <= 4; level++ {
		if output := render(level); len(strings.Split(output, "\n")) <= m.height {
			return output
		}
	}
	return content
}

// stripANSI removes style sequences without touching the screen's text.
func stripANSI(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

// innerWidth reserves panel borders and one padding column on each side.
func (m Model) innerWidth() int { return m.panelWidth() - 4 }
