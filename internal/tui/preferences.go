package tui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/settings"
	"github.com/fabianramirez-code/qrypt/internal/tui/theme"
)

var roles = []string{"accent", "text", "dim", "success", "error", "warning", "border"}

func themeChoice(i int) string { return append(theme.Names(), "custom")[i] }

func hexColor(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func (m *Model) savePreferences() {
	s := m.settings
	language := "en"
	if m.language == Spanish {
		language = "es"
	}
	if (s.Language == language || s.Language == "" && m.language == m.initialLanguage) &&
		(s.Theme == m.themeName || s.Theme == "" && m.themeName == "green") &&
		(s.Icons == m.icons || (s.Icons == "" && m.icons == settings.DefaultIcons)) &&
		s.Motion == m.motion {
		return
	}
	s.Language, s.Theme, s.Icons, s.Motion = language, m.themeName, m.icons, m.motion
	if err := settings.Save(m.settingsPath, s); err != nil {
		m.message = err.Error()
		return
	}
	m.settings = s
	m.settingsError = false
}

func (m Model) languageView() string {
	// Terminal faint can reduce contrast below the palette's measured ratio.
	if !m.plain {
		m.styles.dim = lipgloss.NewStyle().Foreground(lipgloss.Color(hexColor(m.styles.palette.Dim)))
	}
	var languageRows, themeRows, iconRows, motionRows []string
	for i, name := range []string{"English", "Español"} {
		selected := (i == 0 && m.language == English) || (i == 1 && m.language == Spanish)
		languageRows = append(languageRows, m.choiceRow(name, i, selected, m.preferenceSection == 0))
	}
	for i := 0; i < len(theme.Names())+2; i++ {
		name := "Edit colors / Editar colores"
		if i < len(theme.Names())+1 {
			name = themeChoice(i)
		}
		themeRows = append(themeRows, m.choiceRow(name, i, i == m.themeIndex, m.preferenceSection == 1))
	}
	for i, name := range []string{"emoji", "nerd", "unicode"} {
		iconRows = append(iconRows, m.choiceRow(name, i, name == m.icons, m.preferenceSection == 2))
	}
	for i, name := range []string{"on / activa", "off / detenida"} {
		motionRows = append(motionRows, m.choiceRow(name, i, (i == 1) == (m.motion == "reduced"), m.preferenceSection == 3))
	}
	pair := func(a, b string, aw, bw int) string {
		left, right := strings.Split(a, "\n"), strings.Split(b, "\n")
		for len(left) < len(right) {
			left = append(left, strings.Repeat(" ", aw))
		}
		for len(right) < len(left) {
			right = append(right, strings.Repeat(" ", bw))
		}
		rows := make([]string, len(left))
		for i := range rows {
			rows[i] = left[i] + " " + right[i]
		}
		return strings.Join(rows, "\n")
	}
	join := func(rows []string) string { return strings.Join(rows, "\n") }
	first := pair(m.panel("LANGUAGE / IDIOMA", join(languageRows), 21, m.preferenceSection == 0, false), m.panel("THEME / TEMA", join(themeRows), 34, m.preferenceSection == 1, false), 21, 34)
	second := pair(m.panel("ICONS / ICONOS", join(iconRows), 21, m.preferenceSection == 2, false), m.panel("ANIMATION / ANIMACIÓN", join(motionRows), 34, m.preferenceSection == 3, false), 21, 34)
	body := first + "\n" + second
	if m.settingsError {
		body += "\n" + m.styles.dim.Render("Settings could not be read / No se pudieron leer los ajustes.")
	}
	return body
}

func (m *Model) cycleIcons(step int) {
	choices := []string{"emoji", "nerd", "unicode"}
	for i, name := range choices {
		if m.icons == name {
			m.icons = choices[(i+step+len(choices))%len(choices)]
			return
		}
	}
	m.icons = choices[0]
}

func (m Model) choiceRow(name string, index int, selected, focused bool) string {
	marker := "  "
	if selected {
		marker = "> "
	}
	row := marker + name
	if selected && focused {
		return m.styles.selected.Render(row)
	}
	if index%2 == 1 || !focused {
		return m.styles.dim.Render(row)
	}
	return m.styles.text.Render(row)
}

func cloneSettings(s settings.Settings) settings.Settings {
	copy := s
	copy.Colors = make(map[string]string, len(s.Colors))
	for k, v := range s.Colors {
		copy.Colors[k] = v
	}
	return copy
}

func (m *Model) openColorEditor() {
	m.beforeEdit = cloneSettings(m.settings)
	m.editorOrigin = m.themeName
	if m.themeName == "custom" && m.themeBeforeCustom != "" {
		m.editorOrigin = m.themeBeforeCustom
	}
	base, ok := theme.ByName(m.editorOrigin, m.darkBackground)
	if !ok {
		base, _ = theme.ByName("green", m.darkBackground)
		base = theme.Custom(base, m.settings.Colors)
	}
	m.editorPalette = base
	m.editorColors = make(map[string]string, 7)
	m.settings.Colors = make(map[string]string, 7)
	for _, role := range roles {
		c, _ := base.Lookup(role)
		m.editorColors[role] = hexColor(c)
		m.settings.Colors[role] = hexColor(c)
	}
	m.themeName = "custom"
	m.themeIndex = len(theme.Names())
	m.refreshStyles()
	m.input.Wipe()
	m.message, m.colorIndex, m.screen = "", 0, colorEditor
}

func (m *Model) colorKey(v tea.KeyPressMsg) {
	role := roles[m.colorIndex]
	switch v.Code {
	case tea.KeyEsc:
		m.input.Wipe()
		m.message, m.screen = "", languageScreen
		if sameColors(m.settings.Colors, m.editorColors) {
			m.settings = m.beforeEdit
			m.themeName = m.editorOrigin
		} else {
			m.settings.Theme = "custom"
			m.saveEditor()
		}
		m.themeBeforeCustom = ""
		for i := 0; i < len(theme.Names())+1; i++ {
			if themeChoice(i) == m.themeName {
				m.themeIndex = i
			}
		}
		m.refreshStyles()
	case tea.KeyUp, tea.KeyDown:
		if v.Code == tea.KeyUp {
			m.colorIndex = max(0, m.colorIndex-1)
		} else {
			m.colorIndex = min(6, m.colorIndex+1)
		}
		m.input.Wipe()
		m.message = ""
	case tea.KeyEnter:
		if err := theme.ValidateRole(m.editorPalette, role, m.input.String()); err != nil {
			if m.language == Spanish {
				m.message = "Color rechazado: hexadecimal inválido o contraste inferior a 3:1."
			} else {
				m.message = "Color rejected: invalid hex or contrast below 3:1."
			}
		} else {
			m.settings.Colors[role] = m.input.String()
			m.input.Wipe()
			m.message = ""
			m.refreshStyles()
		}
	default:
		switch {
		case v.Code == 'r' && v.Mod&tea.ModCtrl != 0:
			m.settings.Colors[role] = m.editorColors[role]
			m.refreshStyles()
			m.message = ""
		case v.Code == 'a' && v.Mod&tea.ModCtrl != 0:
			for _, r := range roles {
				m.settings.Colors[r] = m.editorColors[r]
			}
			m.refreshStyles()
			m.message = ""
		default:
			m.editInput(v)
		}
	}
}

func sameColors(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func (m *Model) saveEditor() {
	if err := settings.Save(m.settingsPath, m.settings); err != nil {
		m.message = err.Error()
		return
	}
	m.settingsError = false
}

func (m Model) colorView() string {
	heading := "Custom colors"
	if m.language == Spanish {
		heading = "Colores personalizados"
	}
	lines := []string{m.styles.title.Render(heading)}
	for i, role := range roles {
		marker := "  "
		if i == m.colorIndex {
			marker = "> "
		}
		value := m.settings.Colors[role]
		row := fmt.Sprintf("%s%-8s %s  ■ sample / muestra", marker, role, value)
		if !m.plain {
			row = lipgloss.NewStyle().Foreground(lipgloss.Color(value)).Render(row)
		}
		if i == m.colorIndex {
			row = m.styles.selected.Render(row)
		} else if i%2 == 1 {
			row = m.styles.dim.Render(row)
		}
		lines = append(lines, row)
	}
	lines = append(lines, "#rrggbb: "+wrapLine(m.input.String()), m.message,
		"↑/↓: role   Enter: apply   Ctrl+R: reset role",
		"Ctrl+A: reset all   Esc: save / back")
	return strings.Join(lines, "\n")
}
