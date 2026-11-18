package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

// wideColumn paints a fixed-cell opaque column without a surrounding box.
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
	if !m.plain {
		bg := m.styles.palette.Surface
		if bg.A == 0 {
			bg = m.styles.palette.Background
		}
		column = solidBackground(column, bg)
	}
	return column
}

type treeRow struct {
	space  int
	path   []string
	name   string
	id     string
	count  int
	depth  int
	branch string
}

// refreshTree uses the already decrypted listing; painting and animation never read the vault.
func (m *Model) refreshTree() {
	rows := make([]treeRow, 0, len(m.spaceRows))
	for i, space := range m.spaceRows {
		spaceStart := len(rows)
		rows = append(rows, treeRow{space: i, name: space.name, count: space.count})
		// Yazi-like levels: a space or folder shows its children only once entered.
		if i != m.spaceCursor || space.problem >= 0 || m.screen != vaultList && m.screen != vaultItem {
			continue
		}
		// Walk every folder and its direct items; only the current space is expanded.
		var walk func([]string, string, int)
		walk = func(parent []string, indent string, depth int) {
			counts := map[string]int{}
			var items []vault.Entry
			for _, entry := range m.entries {
				if !hasPath(entry.Path, parent) {
					continue
				}
				if len(entry.Path) > len(parent) {
					counts[entry.Path[len(parent)]]++
				} else {
					items = append(items, entry)
				}
			}
			names := make([]string, 0, len(counts))
			for name := range counts {
				names = append(names, name)
			}
			slices.Sort(names)
			slices.SortFunc(items, func(a, b vault.Entry) int { return strings.Compare(a.Title, b.Title) })
			for j, name := range names {
				path := append(slices.Clone(parent), name)
				last := j == len(names)-1 && len(items) == 0
				branch, continuation := "├─ ", "│  "
				if last {
					branch, continuation = "└─ ", "   "
				}
				rows = append(rows, treeRow{space: i, path: path, name: name, count: counts[name], depth: depth, branch: indent + branch})
				if hasPath(m.folderPath, path) {
					walk(path, indent+continuation, depth+1)
				}
			}
			for j, item := range items {
				branch := "├─ "
				if j == len(items)-1 {
					branch = "└─ "
				}
				rows = append(rows, treeRow{space: i, path: slices.Clone(parent), name: item.Title, id: item.ID, depth: depth, branch: indent + branch})
			}
		}
		walk(nil, "", 1)
		// A folder exists only while it holds an item; show the one being created too.
		if i == m.spaceCursor && m.vlt != nil {
			known := 0
			at := len(rows)
			for known < len(m.folderPath) {
				last := -1
				for j := spaceStart; j < len(rows); j++ {
					if rows[j].depth > 0 && hasPath(rows[j].path, m.folderPath[:known+1]) {
						last = j
					}
				}
				if last < 0 {
					break
				}
				known, at = known+1, last+1
			}
			var extra []treeRow
			for depth := known + 1; depth <= len(m.folderPath); depth++ {
				extra = append(extra, treeRow{space: i, path: slices.Clone(m.folderPath[:depth]), name: m.folderPath[depth-1], depth: depth, branch: strings.Repeat("   ", depth-1) + "└─ "})
			}
			rows = slices.Insert(rows, at, extra...)
		}
	}
	m.treeRows = rows
	// Keep the selection on the location after navigation, but preserve a manually
	// selected sibling while it remains visible.
	if m.treeCursor >= len(rows) {
		m.treeCursor = max(0, len(rows)-1)
	}
}

// treeFollow derives the tree cursor from the navigation state, which stays the
// single source of truth for every validated vault action.
func (m *Model) treeFollow() {
	m.refreshTree()
	m.treeCursor = m.treeIndex()
}

func (m Model) treeIndex() int {
	spaceRow := 0
	for i, row := range m.treeRows {
		if row.depth == 0 && row.space == m.spaceCursor {
			spaceRow = i
			break
		}
	}
	if m.screen != vaultList && m.screen != vaultItem {
		return spaceRow
	}
	path, id := m.folderPath, ""
	if m.screen == vaultItem {
		id = m.itemID
	} else if row := m.selectedRow(); row.folder {
		path = append(slices.Clone(m.folderPath), row.name)
	} else {
		id = row.id
	}
	for i, row := range m.treeRows {
		if row.space == m.spaceCursor && row.depth > 0 && row.id == id && samePath(row.path, path) {
			return i
		}
	}
	return spaceRow
}

// wide applies only to the vault navigation screens; modals use their own renderer.
func (m Model) wide() bool {
	return m.width >= 100 && m.height >= 20 && (m.screen == spacesList || m.screen == vaultList || m.screen == vaultItem)
}

// wideKey translates tree selection into the existing validated vault handlers.
func (m Model) wideKey(v tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Yazi navigation keys: h/j/k/l mirror the arrows, g/G and Home/End jump to
	// the first/last row of the level, Ctrl+U/D and PgUp/PgDn move half a page.
	if v.Mod & ^(tea.ModShift|tea.ModCapsLock) == 0 {
		switch {
		case v.Code == tea.KeyHome || v.Code == 'g' && v.Text != "G" && v.Mod&tea.ModShift == 0:
			return m.wideJump(tea.KeyUp, 1<<20)
		case v.Code == tea.KeyEnd || v.Code == 'g' || v.Code == 'G':
			return m.wideJump(tea.KeyDown, 1<<20)
		case v.Code == tea.KeyPgUp:
			return m.wideJump(tea.KeyUp, max(1, m.height/4))
		case v.Code == tea.KeyPgDown:
			return m.wideJump(tea.KeyDown, max(1, m.height/4))
		case v.Code == 'h':
			v = tea.KeyPressMsg{Code: tea.KeyLeft}
		case v.Code == 'j':
			v = tea.KeyPressMsg{Code: tea.KeyDown}
		case v.Code == 'k':
			v = tea.KeyPressMsg{Code: tea.KeyUp}
		case v.Code == 'l':
			v = tea.KeyPressMsg{Code: tea.KeyRight}
		}
	} else if v.Mod == tea.ModCtrl && (v.Code == 'u' || v.Code == 'd') {
		if v.Code == 'u' {
			return m.wideJump(tea.KeyUp, max(1, m.height/4))
		}
		return m.wideJump(tea.KeyDown, max(1, m.height/4))
	}
	if m.screen == vaultItem {
		if v.Code == tea.KeyLeft || v.Code == tea.KeyTab && v.Mod&tea.ModShift != 0 {
			m.leaveItem()
			m.treeFollow()
			return m, nil
		}
		if v.Code == tea.KeyRight {
			return m, nil
		}
	}
	if m.screen == spacesList || m.screen == vaultList {
		m.treeFollow()
		forward := v.Code == tea.KeyTab && v.Mod&tea.ModShift == 0 || v.Code == tea.KeyRight || v.Code == tea.KeyEnter
		switch {
		case v.Code == tea.KeyLeft || v.Code == tea.KeyTab && v.Mod&tea.ModShift != 0:
			// One level up: the folder just left stays selected in its parent.
			if m.screen == vaultList && len(m.folderPath) > 0 {
				name := m.folderPath[len(m.folderPath)-1]
				m.folderPath = slices.Clone(m.folderPath[:len(m.folderPath)-1])
				m.refresh()
				for i, row := range m.folderRows {
					if row.folder && row.name == name {
						m.cursor = i
					}
				}
			} else if m.screen == vaultList {
				m.screen = spacesList
			}
			m.message = ""
			m.treeFollow()
			return m, nil
		case forward && m.screen == vaultList:
			if row := m.selectedRow(); row.folder {
				m.enterFolder(row.name)
			} else if row.id != "" {
				m.enterEntry(row)
			}
			m.treeFollow()
			return m, nil
		case forward:
			v = tea.KeyPressMsg{Code: tea.KeyEnter}
		case m.screen == vaultList && (plainKey(v, 'a') || plainKey(v, 'f')) && m.selectedRow().folder:
			// New things are created inside the selected folder.
			m.enterFolder(m.selectedRow().name)
		}
	}

	if m.screen == vaultItem && plainKey(v, 'e') {
		delete(m.revealed, m.fieldCursor)
	}
	if m.screen == vaultItem && (v.Code == tea.KeyUp || v.Code == tea.KeyDown) {
		m.revealed = nil
		m.visible = false
	}
	{
		switch {
		case m.screen == vaultItem && v.Code == tea.KeyRight:
			// The detail pane is the last one: Right has nowhere to go and must never reveal the value.
			return m, nil
		case m.screen == vaultItem && v.Code == tea.KeyTab && v.Mod&tea.ModShift == 0:
			m.leaveItem()
			m.treeFollow()
			return m, nil

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
			m.treeFollow()
		}
		return m, cmd
	}
}

// wideJump repeats one arrow step, stopping at the end of the current level.
func (m Model) wideJump(code rune, steps int) (tea.Model, tea.Cmd) {
	position := func(m Model) [3]int { return [3]int{m.spaceCursor, m.cursor, m.fieldCursor} }
	for range steps {
		next, _ := m.wideKey(tea.KeyPressMsg{Code: code})
		moved := next.(Model)
		if position(moved) == position(m) {
			break
		}
		m = moved
	}
	return m, nil
}

// wideTrim cuts names and UI text by display cells, never secret values.
func wideLeftTrim(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > width {
		r = r[1:]
	}
	return "…" + string(r)
}

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
	case vaultAddName, vaultAddValue, vaultDelete, folderName:
		return vaultList
	case vaultEdit, fieldLabel, fieldKind, fieldNote, fieldDelete, vaultRename:
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
	case vaultAddName, vaultAddValue, vaultEdit, vaultDelete, folderName,
		fieldLabel, fieldKind, fieldNote, vaultRename, fieldDelete,
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
	if m.screen == vaultAddName || m.screen == vaultAddValue || m.screen == folderName || m.screen == fieldLabel && m.formAction == vaultAddName {
		prefix := "Create in: "
		if m.language == Spanish {
			prefix = "Crear en: "
		}
		body = prefix + m.location() + "\n" + body
	} else if m.screen == spacesAdd {
		prefix := "Create in: Spaces"
		if m.language == Spanish {
			prefix = "Crear en: Espacios"
		}
		body = prefix + "\n" + body
	}
	panel := m.panel(stripANSI(parts[0]), body, m.panelWidth(), true, true)
	base := m
	base.screen = m.modalPane()
	base.visible = false // Never draw a revealed detail value behind any modal.
	base.revealed = nil
	return m.composeModal(panel, base.wideView(true))
}

// composeModal shares pane placement with forms and the help overlay.
func (m Model) composeModal(panel, background string) (string, bool) {
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
	left := 37 + extra*30/100
	right := m.width - left - 1
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
	m.treeFollow()
	// New items and folders go inside the selected folder, else the current one.
	focusedTree := m.screen != vaultItem && !modal
	creating := m.screen != spacesList && m.vlt != nil && m.selectedProblem() == nil
	container := slices.Clone(m.folderPath)
	if row := m.selectedRow(); creating && m.screen == vaultList && row.folder {
		container = append(container, row.name)
	}
	hintAfter := -1
	for i, tree := range m.treeRows {
		if creating && tree.space == m.spaceCursor && (tree.depth == 0 || hasPath(tree.path, container)) {
			hintAfter = i
		}
	}
	var all []string
	cursorLine := 0
	for i := range m.treeRows {
		tree := m.treeRows[i]
		spaceIndex := tree.space
		suffix := ""
		if tree.id == "" && spaceIndex < len(m.spaceRows) && m.spaceRows[spaceIndex].problem < 0 {
			suffix = " (" + strconv.Itoa(tree.count) + ")"
		}
		problem := spaceIndex < len(m.spaceRows) && m.spaceRows[spaceIndex].problem >= 0
		glyph := IconGlyph(m.icons, "spaceClosed")
		if tree.id != "" {
			glyph = IconGlyph(m.icons, "item")
		} else if tree.depth == 0 && spaceIndex == m.spaceCursor || tree.depth > 0 && tree.id == "" && hasPath(m.folderPath, tree.path) {
			glyph = IconGlyph(m.icons, "spaceOpen")
		}
		name := tree.name
		if problem {
			glyph = IconGlyph(m.icons, "problem")
			name = "! " + name
		}
		bar := " "
		if i == m.treeCursor {
			cursorLine = len(all)
			bar = m.styles.accent.Render("▌")
		}
		label := tree.branch + glyph + " " + name
		count := suffix
		if problem {
			count = ""
		}
		available := left - 3 - lipgloss.Width(bar) - lipgloss.Width(count)
		label = wideTrim(label, available)
		gap := 0 // The count, which starts with a space, stays beside the name: "Name (n)".
		if problem {
			all = append(all, bar+m.styles.error.Render(label)+strings.Repeat(" ", gap)+m.styles.error.Render(count))
		} else if i == m.treeCursor {
			all = append(all, m.styles.raised.Render(bar+m.styles.text.Bold(true).Render(label+strings.Repeat(" ", gap)+count)))
		} else if spaceIndex == m.spaceCursor && tree.depth == 0 || tree.depth > 0 && tree.id == "" && hasPath(m.folderPath, tree.path) {
			all = append(all, bar+m.styles.info.Render(label)+m.styles.muted.Render(count))
		} else {
			all = append(all, bar+label+strings.Repeat(" ", gap)+m.styles.muted.Render(count))
		}
		if i == hintAfter {
			indent := strings.Repeat("   ", len(container))
			if len(m.folderRows) == 0 && len(m.folderPath) > 0 {
				all = append(all, indent+"  "+m.hintStyle().Render(t.folderTemporary))
			}
			for _, hint := range [][2]string{{"A", t.wideNewItem}, {"F", t.wideNewFolder}} {
				if row := m.actionRow(hint[0], hint[1], focusedTree, left-lipgloss.Width(indent)); row != "" {
					all = append(all, indent+row)
				}
			}
		}
	}
	if m.selectedProblem() == nil {
		if row := m.actionRow("A", t.wideNewSpace, focusedTree && m.screen == spacesList, left); row != "" {
			all = append(all, row)
		}
	}
	start := max(0, min(cursorLine-spaceRows/2, len(all)-spaceRows))
	spaces = append(spaces, all[start:min(len(all), start+spaceRows)]...)
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
	listRows := m.folderRows
	if m.selectedProblem() != nil {
		listRows = nil
	}
	selected := min(m.cursor, max(0, len(listRows)-1))
	detail := []string{}
	if m.screen != spacesList && len(listRows) > 0 && !listRows[selected].folder {
		generation := uint64(0)
		if m.vlt != nil {
			generation = m.vlt.Generation()
		}
		w := right - 4 // The pane's inner width.
		integrity := fmt.Sprintf("%s %s · v%d", IconGlyph(m.icons, "verified"), t.wideVerified, generation)
		if lipgloss.Width(t.wideIntegrity)+1+lipgloss.Width(integrity) > w {
			integrity = fmt.Sprintf("✓ v%d", generation)
		}
		fields := [][2]string{{t.wideSpace, m.spaceTitle()}, {t.wideEncryption, "XChaCha20-Poly1305"}, {t.wideKey, "ML-KEM-768 + X25519"}, {t.wideSignature, "Ed25519 + ML-DSA-65"}, {t.wideIntegrity, integrity}}
		for _, field := range fields {
			// Label on the left, value against the right edge; a value that cannot fit is cut, never the label.
			value := wideTrim(field[1], w-1-lipgloss.Width(field[0]))
			gap := max(1, w-lipgloss.Width(field[0])-lipgloss.Width(value))
			detail = append(detail, m.hintStyle().Render(field[0])+strings.Repeat(" ", gap)+value)
		}
		if m.screen == vaultItem {
			// The column heading is decoration: a short window drops it first.
			visibleFields, heading := max(1, rows-12), rows >= 16
			if heading {
				visibleFields = max(1, rows-13)
			}
			// Two columns with a heading, so it is clear which part is the label.
			column := lipgloss.Width(t.wideLabel)
			for _, field := range m.itemData.Fields {
				column = max(column, lipgloss.Width(field.Label))
			}
			column = min(column, max(8, (w-2)/2)) + 2
			cell := func(text string) string {
				text = wideTrim(text, column-2)
				return text + strings.Repeat(" ", column-lipgloss.Width(text))
			}
			if heading {
				detail = append(detail, "  "+m.hintStyle().Render(cell(strings.ToUpper(t.wideLabel))+strings.ToUpper(t.wideValue)))
			}
			firstField := max(0, min(m.fieldCursor-visibleFields/2, len(m.itemData.Fields)-visibleFields))
			for i := firstField; i < len(m.itemData.Fields) && i < firstField+visibleFields; i++ {
				field := m.itemData.Fields[i]
				value := "••••••"
				if !modal {
					value = m.fieldValue(field, i)
					if field.Kind == vault.FieldNote {
						value = fmt.Sprintf("%s (%d)", t.noteWord, strings.Count(string(field.Value), "\n")+1)
					}
					if len([]rune(string(field.Value))) > maxShownRunes && (field.Kind == vault.FieldVisible || m.revealed[i]) {
						value = t.tooLong
					}
				}
				bar := " "
				if i == m.fieldCursor {
					bar = m.styles.accent.Render("▌")
				}
				line := wideTrim(cell(field.Label)+value, w-2)
				if i == m.fieldCursor {
					line = m.styles.text.Bold(true).Render(line)
				}
				detail = append(detail, bar+" "+line)
			}
		}
		label := t.wideValue
		if m.screen == vaultItem && m.fieldCursor < len(m.itemData.Fields) {
			// The card names the secret it shows.
			label = wideTrim(label+": "+m.itemData.Fields[m.fieldCursor].Label, w-8)
		}
		value := "• • • • • •"
		revealed := m.screen == vaultItem && !modal && m.fieldCursor < len(m.itemData.Fields) && (m.visible || m.itemData.Fields[m.fieldCursor].Kind == vault.FieldVisible)
		if revealed {
			value = string(m.itemData.Fields[m.fieldCursor].Value)
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
		sideBySide := lipgloss.Width(revealButton)+2+lipgloss.Width(copyChip) <= w
		edit, add := IconGlyph(m.icons, "edit")+" [E] "+t.wideEdit, IconGlyph(m.icons, "add")+" [A] "+t.wideAddField
		// Add field sits beside Edit, under Copy when the buttons above are side by side.
		addColumn := lipgloss.Width(edit) + 2
		if sideBySide {
			addColumn = max(addColumn, lipgloss.Width(revealButton)+2)
		}
		addBeside := addColumn+lipgloss.Width(add) <= w
		buttonRows := 2 // One row for Reveal and Copy, one for Edit.
		if !sideBySide {
			buttonRows++
		}
		if !addBeside {
			buttonRows++
			// Decoration goes first when the extra row has no room: the blank row above the buttons.
			if len(detail)+buttonRows > rows && detail[len(detail)-1] == "" {
				detail = detail[:len(detail)-1]
			}
		}
		if sideBySide {
			detail = append(detail, revealButton+"  "+copyChip)
		} else {
			// Side by side when they fit; one under the other in a narrow inspector.
			detail = append(detail, revealButton, copyChip)
		}
		if addBeside {
			detail = append(detail, m.styles.muted.Render(edit+strings.Repeat(" ", addColumn-lipgloss.Width(edit))+add))
		} else {
			detail = append(detail, m.styles.muted.Render(edit), m.styles.muted.Render(add))
		}
	}
	inspectorName := ""
	if len(listRows) > 0 && m.screen != spacesList && !listRows[selected].folder {
		inspectorName = listRows[selected].name
	}
	if m.screen == vaultItem {
		inspectorName = m.item
	}
	itemHeading := "[2] " + strings.ToUpper(t.itemTitle)
	if inspectorName != "" {
		itemHeading += " // " + m.location() + " / " + inspectorName
	}
	// One line between panes, crossing the rule under the headings.
	separator := m.styles.border.Render("│" + "\n" + "┼" + strings.Repeat("\n"+"│", max(0, paneHeight-2)))
	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		m.wideColumn("[1] "+strings.ToUpper(t.spacesTitle)+"  "+strconv.Itoa(len(m.spaceNames)), spaces, left, paneHeight, m.screen != vaultItem), separator,
		m.wideColumn(itemHeading, detail, right, paneHeight, m.screen == vaultItem))
	if modal {
		panes = lipgloss.NewStyle().Faint(true).Render(panes)
	}
	hints := ""
	if !modal {
		full, mandatory := t.wideSpacesHint, 2
		switch m.screen {
		case vaultList:
			full, mandatory = t.wideListHint, 3
		case vaultItem:
			full, mandatory = t.wideItemHint, 2
		}
		// Whole hints only: drop from the end of the priority order until the bar fits beside the counter.
		list := strings.Split(full, "  ")
		for len(list) > mandatory && lipgloss.Width(strings.Join(list, "  "))+2+lipgloss.Width(t.wideCount(len(listRows))) > m.width {
			list = list[:len(list)-1]
		}
		hints = strings.Join(list, "  ")
	}
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
	if warning := m.memoryWarning(); warning != "" {
		status = warning
	} else if status == "" && !modal {
		status = t.wideCount(len(listRows))
	}
	style := m.styles.hint
	if m.memoryWarning() != "" {
		style = m.styles.warning
	} else if m.message != "" {
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

// hintStyle is the role of the dim labels; the plain look has no hue, so it dims instead.
func (m Model) hintStyle() lipgloss.Style {
	if m.plain {
		return m.styles.dim
	}
	return m.styles.muted
}

// actionRow is a decoration that shows a creating key at the end of a list.
// It is never selectable. The key is painted like the bottom bar's keys only
// in the focused pane; elsewhere the whole row is muted.
func (m Model) actionRow(key, label string, focused bool, width int) string {
	text := "  + [" + key + "] " + label
	if lipgloss.Width(text) > width-2 {
		return ""
	}
	if !focused {
		return m.hintStyle().Render(text)
	}
	return "  " + m.hintStyle().Render("+ ") + m.styles.raised.Render(m.styles.info.Render("["+key+"]")) + m.hintStyle().Render(" "+label)
}
