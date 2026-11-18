package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// wideColumn paints a fixed-cell column without a surrounding box.
// The focused column is the one whose heading and rule carry the accent.
func (m Model) wideColumn(heading string, lines []string, width, height int, focused bool) string {
	var result []string
	headStyle, ruleStyle := m.styles.muted, m.styles.dim
	if focused {
		headStyle, ruleStyle = m.styles.info.Bold(true), m.styles.accent
	}
	header := headStyle.Render(wideTrim(heading, width-2))
	result = append(result, " "+header+strings.Repeat(" ", max(0, width-1-lipgloss.Width(header))))
	result = append(result, ruleStyle.Render(strings.Repeat("─", width)))
	for i := 0; i < height-2; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		line = wideTrim(line, width-2)
		result = append(result, " "+line+strings.Repeat(" ", max(0, width-1-lipgloss.Width(line))))
	}
	column := strings.Join(result, "\n")
	if m.styles.palette.Surface.A != 0 {
		column = lipgloss.NewStyle().Background(lipgloss.Color(hexColor(m.styles.palette.Surface))).Render(column)
	}
	return column
}

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
	statusLeft := ">_ QRYPT // " + wideTrim(m.spaceTitle(), m.width-37)
	badge := "● PQC: ML-KEM-768 + X25519"
	header += "\n" + m.styles.accent.Render(statusLeft) + strings.Repeat(" ", max(1, m.width-lipgloss.Width(statusLeft)-lipgloss.Width(badge))) + m.styles.accent.Render(badge)
	banner := ""
	if m.preview {
		banner = m.styles.warning.Render(t.previewBanner)
	}
	paneHeight := m.height - strings.Count(header, "\n") - 1 - 2
	if banner != "" {
		paneHeight--
	}
	// Each column uses a header and rule, without an outer border.
	rows := max(paneHeight-2, 0)
	spaces := make([]string, 0, rows)
	spaceRows := rows
	if rows >= 5 && len(m.spaceNames) <= rows-2 {
		spaceRows -= 2
	}
	start := max(0, min(m.spaceCursor-spaceRows/2, len(m.spaceNames)-spaceRows))
	for i := start; i < len(m.spaceNames) && len(spaces) < spaceRows; i++ {
		suffix := ""
		if i < len(m.spaceCounts) {
			if i < len(m.spaceRows) && m.spaceRows[i].problem < 0 {
				suffix = " (" + strconv.Itoa(m.spaceCounts[i]) + ")"
			}
		}
		problem := i < len(m.spaceRows) && m.spaceRows[i].problem >= 0
		glyph := IconGlyph(m.icons, "spaceClosed")
		if i == m.spaceCursor {
			glyph = IconGlyph(m.icons, "spaceOpen")
		}
		name := m.spaceNames[i]
		if problem {
			glyph = IconGlyph(m.icons, "problem")
			name = "! " + name
		}
		bar := " "
		if i == m.spaceCursor {
			bar = m.styles.accent.Render("▌")
		}
		label := glyph + " " + name
		count := suffix
		if problem {
			count = ""
		}
		available := left - 3 - lipgloss.Width(bar) - lipgloss.Width(count)
		label = wideTrim(label, available)
		gap := 0 // The count, which starts with a space, stays beside the name: "Name (n)".
		if problem {
			spaces = append(spaces, bar+m.styles.error.Render(label)+strings.Repeat(" ", gap)+m.styles.error.Render(count))
		} else if i == m.spaceCursor {
			spaces = append(spaces, m.styles.raised.Render(bar+m.styles.text.Bold(true).Render(label+strings.Repeat(" ", gap)+count)))
		} else {
			spaces = append(spaces, bar+label+strings.Repeat(" ", gap)+m.styles.muted.Render(count))
		}
	}
	if spaceRows < rows {
		for len(spaces) < spaceRows {
			spaces = append(spaces, "")
		}
		mirrorState := t.mirrorNoFolder
		if m.settings.Mirror != "" {
			mirrorState = t.wideMirrorReady
			if m.mirrorWarning {
				mirrorState = t.wideMirrorFailed
			}
		}
		remaining := max(0, int((m.lastKey.Add(idleLifetime).Sub(m.now())+time.Second-1)/time.Second))
		spaces = append(spaces, m.styles.special.Render(wideTrim("["+mirrorState+"]", left-2)), m.styles.muted.Render(fmt.Sprintf("%s %d:%02d ", t.wideLock, remaining/60, remaining%60))+m.styles.accent.Render(strings.Repeat("━", remaining*8/300))+m.styles.muted.Render(strings.Repeat("─", 8-remaining*8/300)))
	}
	names := m.names
	if m.selectedProblem() != nil {
		names = nil
	}
	items := make([]string, 0, rows)
	count := strconv.Itoa(len(names))
	items = append(items, m.styles.info.Render(t.wideName)+strings.Repeat(" ", max(1, center-3-lipgloss.Width(t.wideName)-len(count)))+m.styles.muted.Render(count))
	selected := min(m.cursor, max(0, len(names)-1))
	if m.screen == spacesList {
		selected = 0
	}
	first := max(0, min(selected-(rows-1)/2, len(names)-(rows-1)))
	for i := first; i < len(names) && len(items) < rows; i++ {
		bar := " "
		if i == selected {
			bar = m.styles.accent.Render("▌")
		}
		name := wideTrim(names[i], center-7-lipgloss.Width(IconGlyph(m.icons, "item")))
		label := name
		if at := strings.LastIndex(name, " / "); at >= 0 && i != selected {
			label = m.styles.muted.Render(name[:at+3]) + m.styles.text.Render(name[at+3:])
		}
		row := bar + " " + IconGlyph(m.icons, "item") + " " + label
		if i == selected {
			row = m.styles.raised.Render(bar + m.styles.text.Bold(true).Render(" "+IconGlyph(m.icons, "item")+" "+name))
		}
		items = append(items, row)
	}
	detail := []string{}
	if m.screen != spacesList && len(names) > 0 {
		name := names[selected]
		if m.screen == vaultItem {
			name = m.item
		}
		_ = name // The inspector heading carries the selected item's name.
		generation := uint64(0)
		if m.vlt != nil {
			generation = m.vlt.Generation()
		}
		w := right - 4 // The pane's inner width.
		integrity := fmt.Sprintf("%s %d", t.wideVerified, generation)
		if lipgloss.Width(t.wideIntegrity)+1+lipgloss.Width(integrity) > w {
			integrity = fmt.Sprintf("✓ gen %d", generation)
		}
		fields := [][2]string{{t.wideSpace, m.spaceTitle()}, {t.wideEncryption, "XChaCha20-Poly1305"}, {t.wideKey, "ML-KEM-768 + X25519"}, {t.wideSignature, "Ed25519 + ML-DSA-65"}, {t.wideIntegrity, integrity}}
		for _, field := range fields {
			// Label on the left, value against the right edge; a value that cannot fit is cut, never the label.
			value := wideTrim(field[1], w-1-lipgloss.Width(field[0]))
			gap := max(1, w-lipgloss.Width(field[0])-lipgloss.Width(value))
			detail = append(detail, m.styles.dim.Render(field[0])+strings.Repeat(" ", gap)+value)
		}
		label := t.wideValue
		value := "• • • • • •"
		revealed := m.screen == vaultItem && m.visible && !modal
		if revealed {
			value = string(m.itemValue)
		}
		lines := strings.Split(lipgloss.Wrap(value, w-2, ""), "\n")
		// Three rows stay for the box borders and the buttons below it.
		if revealed && (len([]rune(value)) > maxShownRunes || len(lines) > max(1, rows-len(detail)-6)) {
			lines = strings.Split(lipgloss.Wrap(t.tooLong, w-2, ""), "\n")
		}
		// The value sits in a card drawn with two rules, the label on the upper one.
		detail = append(detail, "", m.styles.border.Render("── ")+m.styles.special.Render(label)+" "+m.styles.border.Render(strings.Repeat("─", max(0, w-4-lipgloss.Width(label)))))
		for _, line := range lines {
			detail = append(detail, "  "+line)
		}
		detail = append(detail, m.styles.border.Render(strings.Repeat("─", w)), "")
		reveal, copyButton := m.styles.info, m.styles.selected
		if m.screen != vaultItem || modal {
			reveal, copyButton = m.styles.dim, m.styles.dim
		}
		action := t.wideReveal
		if m.visible && !modal {
			action = t.wideHide
		}
		revealButton := reveal.Render(IconGlyph(m.icons, "reveal") + " [Enter] " + action)
		copyChip := copyButton.Render(" " + IconGlyph(m.icons, "copy") + " [C] " + t.wideCopy + " ")
		if lipgloss.Width(revealButton)+2+lipgloss.Width(copyChip) <= w {
			detail = append(detail, revealButton+"  "+copyChip)
		} else {
			// Side by side when they fit; one under the other in a narrow inspector.
			detail = append(detail, revealButton, copyChip)
		}
		detail = append(detail, m.styles.muted.Render("[E] "+t.wideEdit))
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
	_ = pad
	inspectorName := ""
	if len(names) > 0 && m.screen != spacesList {
		inspectorName = names[selected]
	}
	itemHeading := "[3] " + strings.ToUpper(t.itemTitle)
	if inspectorName != "" {
		itemHeading += " // " + inspectorName
	}
	// One line between panes, crossing the rule under the headings.
	separator := m.styles.border.Render("│" + "\n" + "┼" + strings.Repeat("\n"+"│", max(0, paneHeight-2)))
	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		m.wideColumn("[1] "+strings.ToUpper(t.spacesTitle)+"  "+strconv.Itoa(len(m.spaceNames)), spaces, left, paneHeight, m.screen == spacesList), separator,
		m.wideColumn(strings.TrimSpace("[2] "+centerTitle), items, center, paneHeight, m.screen == vaultList), separator,
		m.wideColumn(itemHeading, detail, right, paneHeight, m.screen == vaultItem))
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
	// Paint only bracketed keys; labels remain in the hint role.
	var hintParts []string
	for _, part := range strings.Split(hints, "  ") {
		at := strings.Index(part, "]")
		if strings.HasPrefix(part, "[") && at >= 0 {
			hintParts = append(hintParts, m.styles.raised.Render(m.styles.info.Render(part[:at+1]))+m.styles.muted.Render(part[at+1:]))
		} else {
			hintParts = append(hintParts, m.styles.hint.Render(part))
		}
	}
	coloredHints := strings.Join(hintParts, "  ")
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
	if m.copyActive && !m.copyUntil.IsZero() {
		seconds := max(0, int((m.copyUntil.Sub(m.now())+time.Second-1)/time.Second))
		if seconds > 0 {
			status += fmt.Sprintf("  %ds", seconds)
		}
	}
	status = wideTrim(status, m.width)
	footer := coloredHints
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
