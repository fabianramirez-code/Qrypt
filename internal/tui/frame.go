package tui

import (
	_ "embed"
	"image/color"
	"math"
	"strconv"
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
	panel := strings.Join(lines, "\n")
	if !m.plain {
		bg := m.styles.palette.Surface
		if bg.A == 0 {
			bg = m.styles.palette.Background
		}
		return solidBackground(panel, bg)
	}
	return panel
}

// solidBackground reapplies the surface after nested styles reset SGR. A wrapper
// background alone leaves explicitly styled text and borders transparent.
func solidBackground(content string, bg color.RGBA) string {
	paint := "\x1b[48;2;" + strconv.Itoa(int(bg.R)) + ";" + strconv.Itoa(int(bg.G)) + ";" + strconv.Itoa(int(bg.B)) + "m"
	content = strings.ReplaceAll(content, "\x1b[m", "\x1b[m"+paint)
	content = strings.ReplaceAll(content, "\x1b[0m", "\x1b[0m"+paint)
	content = strings.ReplaceAll(content, "\x1b[49m", "\x1b[49m"+paint)
	return paint + strings.ReplaceAll(content, "\n", "\x1b[49m\n"+paint) + "\x1b[49m"
}

// logoPeriod is long enough for the band to leave the logo before it re-enters.
const logoPeriod = 80

// logoTint is the static left-to-right gradient plus a wide, near-white band of light that
// moves one column per phase, slanted by row.
func logoTint(accent color.RGBA, col, cols, row, phase int) color.RGBA {
	ratio := float64(col) / float64(max(cols-1, 1))
	centre := float64(phase%logoPeriod) - 20 + float64(row)*2
	if phase < 0 {
		centre = -1000 // No moving band; retain the static gradient.
	}
	d := (float64(col) - centre) / 6
	t := min(1, ratio*0.3+0.9*math.Exp(-d*d))
	mix := func(x uint8) uint8 { return uint8(min(255, math.Round(float64(x)*(1-t)+255*t))) }
	return color.RGBA{R: mix(accent.R), G: mix(accent.G), B: mix(accent.B), A: 255}
}

func (m Model) logoView() string {
	// A checkout may convert the embedded file to CRLF line endings.
	rows := strings.Split(strings.TrimRight(strings.ReplaceAll(logo, "\r", ""), "\n"), "\n")
	var result []string
	accent := m.styles.palette.Accent
	for y, row := range rows {
		row = strings.TrimRight(row, " ")
		if m.plain {
			result = append(result, m.styles.accent.Render(row))
			continue
		}
		var line strings.Builder
		for i, r := range []rune(row) {
			phase := m.logoPhase
			if m.motion == "reduced" {
				phase = -logoPeriod // Light band stays entirely outside the logo.
			}
			tint := logoTint(accent, i, len([]rune(row)), y, phase)
			line.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(hexColor(tint))).Render(string(r)))
		}
		result = append(result, line.String())
	}
	return strings.Join(result, "\n")
}

func (m Model) bottomBar() string {
	if m.screen == languageScreen {
		return m.styles.hint.Render("↑/↓: move / mover   ←/→/Tab: list / lista\nEnter: select / elegir   Esc: quit / salir  ? help/ayuda")
	}
	if m.language == Spanish {
		return m.styles.hint.Render("Ctrl+C: salir")
	}
	return m.styles.hint.Render("Ctrl+C: quit")
}

// frame draws the same headed panel and separate key hints on every full-size screen.
func (m Model) frame(content string) string {
	if m.screen == languageScreen {
		return m.preferenceFrame(content)
	}
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

// preferenceFrame keeps all first-screen content in one opaque island; Place
// pads outside it without a background style so the terminal stays visible.
func (m Model) preferenceFrame(content string) string {
	logo := m.logoView()
	hints := m.bottomBar()
	if !m.plain {
		hints = lipgloss.NewStyle().Foreground(lipgloss.Color(hexColor(m.styles.palette.Dim))).Render(stripANSI(hints))
	}
	makeBox := func(mark string) string {
		// Pad the rows to one width first so the mark is centered as a block.
		mark = lipgloss.JoinVertical(lipgloss.Left, strings.Split(mark, "\n")...)
		body := lipgloss.PlaceHorizontal(56, lipgloss.Center, mark) + "\n" + content + "\n" + hints
		bg := m.styles.palette.Surface
		if bg.A == 0 {
			bg = m.styles.palette.Background
		}
		style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
		if !m.plain {
			style = style.BorderForeground(lipgloss.Color(hexColor(m.styles.palette.Accent))).Background(lipgloss.Color(hexColor(bg)))
		}
		return style.Render(body)
	}
	box := makeBox(logo)
	if lipgloss.Height(box) > m.height {
		box = makeBox(m.styles.accent.Render("◆ Qrypt"))
	}
	if lipgloss.Height(box) > m.height {
		box = makeBox("")
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
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
