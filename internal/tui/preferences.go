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
		(s.Theme == m.themeName || s.Theme == "" && m.themeName == "green") {
		return
	}
	s.Language, s.Theme = language, m.themeName
	if err := settings.Save(m.settingsPath, s); err != nil {
		m.message = err.Error()
		return
	}
	m.settings = s
	m.settingsError = false
}

func (m Model) languageView() string {
	first, second := "  English", "  Español"
	if m.language == Spanish {
		second = "> Español"
	} else {
		first = "> English"
	}
	rows := []string{m.styles.title.Render("Language / Idioma"), first, second, "Theme / Tema"}
	for i := 0; i < 7; i++ {
		name := "Edit colors / Editar colores"
		if i < 6 {
			name = themeChoice(i)
		}
		marker := "  "
		if i == m.themeIndex {
			marker = "> "
		}
		row := marker + name
		if m.themeList && i == m.themeIndex {
			row = m.styles.selected.Render(row)
		}
		rows = append(rows, row)
	}
	if m.settingsError {
		rows = append(rows, "Settings could not be read / No se pudieron leer los ajustes.")
	}
	rows = append(rows, m.styles.hint.Render("↑/↓: move / mover   Tab: list / lista"),
		m.styles.hint.Render("Enter: select / elegir   Esc: quit / salir"))
	return strings.Join(rows, "\n")
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
	base, ok := theme.ByName(m.themeName, true)
	if !ok {
		base, _ = theme.ByName("green", true)
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
	m.themeIndex = 5
	m.refreshStyles()
	m.input, m.message, m.colorIndex, m.screen = "", "", 0, colorEditor
}

func (m *Model) colorKey(v tea.KeyPressMsg) {
	role := roles[m.colorIndex]
	switch v.Code {
	case tea.KeyEsc:
		m.input, m.message, m.screen = "", "", languageScreen
		if sameColors(m.settings.Colors, m.editorColors) {
			m.settings = m.beforeEdit
			m.themeName = m.editorOrigin
		} else {
			m.settings.Theme = "custom"
			m.saveEditor()
		}
		for i := 0; i < 6; i++ {
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
		m.input, m.message = "", ""
	case tea.KeyEnter:
		if err := theme.ValidateRole(m.editorPalette, role, m.input); err != nil {
			if m.language == Spanish {
				m.message = "Color rechazado: hexadecimal inválido o contraste inferior a 3:1."
			} else {
				m.message = "Color rejected: invalid hex or contrast below 3:1."
			}
		} else {
			m.settings.Colors[role] = m.input
			m.input, m.message = "", ""
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
		if m.themeName != "mono" {
			row = lipgloss.NewStyle().Foreground(lipgloss.Color(value)).Render(row)
		}
		if i == m.colorIndex {
			row = m.styles.selected.Render(row)
		}
		lines = append(lines, row)
	}
	lines = append(lines, "#rrggbb: "+wrapLine(m.input), m.message,
		"↑/↓: role   Enter: apply   Ctrl+R: reset role",
		"Ctrl+A: reset all   Esc: save / back")
	return strings.Join(lines, "\n")
}
