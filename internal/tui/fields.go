package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

func (m *Model) clearItem() {
	for i := range m.itemData.Fields {
		clear(m.itemData.Fields[i].Value)
	}
	m.itemData = vault.Item{}
	clear(m.itemValue)
	m.itemValue = nil
	m.itemID = ""
	m.hideRevealed()
	m.fieldCursor = 0
}

// hideRevealed invalidates any pending reveal timer without touching key activity.
func (m *Model) hideRevealed() {
	m.revealToken++
	m.revealed = nil
	m.visible = false
}

func (m *Model) toggleReveal() tea.Cmd {
	if m.revealed[m.fieldCursor] {
		m.hideRevealed()
		return nil
	}
	m.hideRevealed()
	m.revealed = map[int]bool{m.fieldCursor: true}
	m.visible = true
	token := m.revealToken
	// Batch keeps existing key helpers from running a 15-second timer inline.
	return tea.Batch(tea.Tick(revealLifetime, func(time.Time) tea.Msg { return revealTickMsg{token: token} }), func() tea.Msg { return nil })
}

func (m *Model) enterEntry(row folderRow) {
	item, err := m.vlt.Read(row.id)
	if err != nil {
		m.failVault(err)
		return
	}
	m.clearItem()
	m.itemID = row.id
	m.itemData = item
	m.item = item.Title
	m.fieldCursor = 0
	m.syncFirstField()
	m.message = ""
	m.screen = vaultItem
}
func (m *Model) syncFirstField() {
	clear(m.itemValue)
	m.itemValue = nil
	if len(m.itemData.Fields) > 0 {
		m.itemValue = slices.Clone(m.itemData.Fields[0].Value)
	}
	m.visible = m.revealed[m.fieldCursor]
}
func (m *Model) copyEntry(id string) tea.Cmd {
	item, err := m.vlt.Read(id)
	if err != nil {
		m.failVault(err)
		return nil
	}
	defer func() {
		for i := range item.Fields {
			clear(item.Fields[i].Value)
		}
	}()
	if len(item.Fields) == 0 {
		return nil
	}
	return m.copyValueOf(item.Fields[0].Value)
}
func (m *Model) saveItem(item vault.Item) bool {
	var err error
	if m.itemID == "" {
		m.itemID, err = m.vlt.Add(item)
	} else {
		err = m.vlt.Replace(m.itemID, item)
	}
	if err != nil {
		m.writeError(err)
		return false
	}
	m.message = ""
	m.mirrorUpdate()
	m.refresh()
	m.refreshSpaces()
	return true
}
func (m Model) fieldValue(field vault.Field, index int) string {
	t := m.language.text()
	if field.Kind == vault.FieldVisible || m.revealed[index] {
		value := string(field.Value)
		if len([]rune(value)) > maxShownRunes || strings.ContainsAny(value, "\r\n") {
			if !m.revealed[index] && field.Kind == vault.FieldVisible {
				return t.tooLong
			}
			return wideTrim(strings.ReplaceAll(value, "\n", " ↵ "), m.innerWidth()-len([]rune(field.Label))-5)
		}
		return value
	}
	if field.Kind == vault.FieldNote {
		return fmt.Sprintf("%s (%d)", t.noteWord, strings.Count(string(field.Value), "\n")+1)
	}
	return "••••••"
}
func (m Model) fieldsView(t texts) string {
	lines := []string{m.styles.title.Render(wideTrim(t.itemTitle+": "+m.item, m.innerWidth())), wrapLine(m.location())}
	rows := max(3, m.height-14)
	first := max(0, min(m.fieldCursor-rows/2, len(m.itemData.Fields)-rows))
	for i := first; i < len(m.itemData.Fields) && i < first+rows; i++ {
		field := m.itemData.Fields[i]
		prefix := "  "
		if i == m.fieldCursor {
			prefix = "> "
		}
		line := wideTrim(prefix+field.Label+": "+m.fieldValue(field, i), m.innerWidth())
		if i == m.fieldCursor {
			line = m.styles.selected.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, wrapLine(m.message), m.styles.hint.Render(t.fieldsHint))
	return strings.Join(lines, "\n")
}
func (m Model) formView(t texts) string {
	title, prompt, hint := t.addTitle, t.namePrompt, t.addNameHint
	content := m.input
	switch m.screen {
	case folderName:
		title, prompt = t.folderAddTitle, t.namePrompt
	case vaultRename:
		title = t.renameTitle
	case vaultAddName:
		prompt = t.titlePrompt
	case fieldLabel:
		title, prompt = t.fieldAddTitle, t.labelPrompt
	case fieldKind:
		title, prompt = t.fieldAddTitle, t.kindPrompt
		content = ""
	case vaultAddValue:
		prompt = t.valuePrompt
		if m.formKind == vault.FieldVisible {
			prompt = t.visiblePrompt
		}
		content = hide(m.input, m.visible || m.formKind == vault.FieldVisible)
		hint = t.valueHint
	case vaultEdit:
		title, prompt = t.editTitle, t.valuePrompt
		if m.formKind == vault.FieldVisible {
			prompt = t.visiblePrompt
		}
		content = hide(m.input, m.visible || m.formKind == vault.FieldVisible)
		hint = t.valueHint
	case fieldNote:
		title, prompt, hint = t.noteTitle, t.notePrompt, t.noteHint
		content = wideTrim(strings.ReplaceAll(m.input, "\n", "\n"), m.innerWidth())
	case vaultDelete:
		return m.styles.title.Render(t.deleteTitle) + "\n" + wrapLine(m.selectedRow().name) + "\n" + t.deleteQuestion + "\n" + m.styles.hint.Render(t.deleteHint)
	case fieldDelete:
		label := ""
		if m.fieldCursor >= 0 && m.fieldCursor < len(m.itemData.Fields) {
			label = m.itemData.Fields[m.fieldCursor].Label
		}
		return m.styles.title.Render(t.fieldDeleteTitle) + "\n" + wrapLine(label) + "\n" + m.styles.hint.Render(t.deleteHint)
	}
	if m.screen == vaultAddValue || m.screen == fieldNote || m.screen == vaultEdit {
		if m.formAction == vaultAddName {
			title = t.addTitle
		}
	}
	lines := []string{m.styles.title.Render(title), wrapLine(prompt)}
	if m.screen == fieldNote {
		parts := strings.Split(content, "\n")
		start := max(0, len(parts)-max(2, m.height-15))
		for _, line := range parts[start:] {
			lines = append(lines, wideTrim(line, m.innerWidth()))
		}
	} else {
		// Keep the full input in memory, but show only its last two rows.
		// A pasted oversized value must never overflow the terminal.
		runes := []rune(content)
		limit := max(1, m.innerWidth()*2-2)
		if len(runes) > limit {
			content = "…" + string(runes[len(runes)-limit:])
		}
		lines = append(lines, wrapWidth(content, m.innerWidth()))
	}
	lines = append(lines, wrapLine(m.message), m.styles.hint.Render(hint))
	return strings.Join(lines, "\n")
}
func (m *Model) beginField(action screen) {
	m.formAction = action
	m.formLabel = ""
	m.formKind = 0
	m.input, m.message = "", ""
	m.visible = false
	m.screen = fieldLabel
}
func (m *Model) finishValue() {
	field := vault.Field{Kind: m.formKind, Label: m.formLabel, Value: []byte(m.input)}
	var item vault.Item
	if m.formAction == vaultAddName {
		item = vault.Item{Path: slices.Clone(m.folderPath), Title: m.newName, Fields: []vault.Field{field}}
	} else {
		item = m.itemData
		item.Fields = append([]vault.Field(nil), item.Fields...)
		if m.formAction == vaultEdit {
			item.Fields[m.fieldCursor] = field
		} else {
			item.Fields = append(item.Fields, field)
		}
	}
	if !m.saveItem(item) {
		return
	}
	m.input, m.formLabel, m.newName = "", "", ""
	m.visible = false
	if m.formAction == vaultAddName {
		m.screen = vaultList
		for i, row := range m.folderRows {
			if row.id == m.itemID {
				m.cursor = i
				break
			}
		}
		m.itemID = ""
	} else {
		id := m.itemID
		m.clearItem()
		m.enterEntry(folderRow{id: id})
		if m.formAction == vaultItem {
			m.fieldCursor = len(m.itemData.Fields) - 1
		}
	}
}
func (m Model) fieldsKey(v tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	t := m.language.text()
	switch m.screen {
	case vaultItem:
		switch {
		case v.Code == tea.KeyEsc:
			m.leaveItem()
		case v.Code == tea.KeyUp:
			m.fieldCursor = max(0, m.fieldCursor-1)
			m.syncFirstField()
		case v.Code == tea.KeyDown:
			m.fieldCursor = min(max(0, len(m.itemData.Fields)-1), m.fieldCursor+1)
			m.syncFirstField()
		case v.Code == tea.KeyTab && len(m.itemData.Fields) > 0:
			return m, m.toggleReveal()
		case plainKey(v, 'c') && len(m.itemData.Fields) > 0:
			return m, m.copyValueOf(m.itemData.Fields[m.fieldCursor].Value)
		case plainKey(v, 'e') && len(m.itemData.Fields) > 0:
			m.input, m.message = "", ""
			m.visible = false
			m.formKind = m.itemData.Fields[m.fieldCursor].Kind
			m.formLabel = m.itemData.Fields[m.fieldCursor].Label
			m.formAction = vaultEdit
			if m.formKind == vault.FieldNote {
				m.screen = fieldNote
			} else {
				m.screen = vaultEdit
			}
		case plainKey(v, 'a'):
			m.beginField(vaultItem)
		case plainKey(v, 'r'):
			m.input, m.message = "", ""
			m.screen = vaultRename
		case plainKey(v, 'd') && len(m.itemData.Fields) > 0:
			m.screen = fieldDelete
		}
	case vaultAddName, folderName, vaultRename, fieldLabel:
		if v.Code == tea.KeyEsc {
			switch m.screen {
			case vaultAddName, folderName:
				vaultListScreen(&m)
			case vaultRename:
				if m.itemID != "" {
					m.screen = vaultItem
				} else {
					m.screen = vaultList
				}
			case fieldLabel:
				if m.formAction == vaultAddName {
					m.screen = vaultAddName
					m.input = m.newName
				} else {
					m.screen = vaultItem
				}
			}
			m.message = ""
			break
		}
		if v.Code != tea.KeyEnter {
			m.editInput(v)
			break
		}
		if err := vault.ValidateName(m.input); err != nil {
			m.writeError(err)
			break
		}
		switch m.screen {
		case folderName:
			name := m.input
			m.screen = vaultList
			m.enterFolder(name)
		case vaultAddName:
			duplicate := false
			for _, e := range m.entries {
				if samePath(e.Path, m.folderPath) && e.Title == m.input {
					duplicate = true
					break
				}
			}
			if duplicate {
				m.message = t.nameExists
				break
			}
			m.newName = m.input
			m.beginField(vaultAddName)
		case vaultRename:
			name := m.input
			if m.itemID != "" {
				item := m.itemData
				item.Title = name
				if !m.saveItem(item) {
					break
				}
				id := m.itemID
				m.clearItem()
				m.enterEntry(folderRow{id: id})
			} else {
				if err := m.renameFolder(name); err != nil {
					m.writeError(err)
					break
				}
				m.refresh()
				m.mirrorUpdate()
				m.screen = vaultList
			}
			m.input = ""
		case fieldLabel:
			for _, f := range m.itemData.Fields {
				if m.formAction != vaultAddName && f.Label == m.input {
					m.message = t.labelExists
					return m, nil
				}
			}
			m.formLabel = m.input
			m.input = ""
			m.screen = fieldKind
		}
	case fieldKind:
		if v.Code == tea.KeyEsc {
			m.screen = fieldLabel
			m.input = m.formLabel
			break
		}
		var kind vault.FieldKind
		switch {
		case plainKey(v, 'v'):
			kind = vault.FieldVisible
		case plainKey(v, 'n'):
			kind = vault.FieldNote
		case m.language == Spanish && plainKey(v, 'o'), m.language != Spanish && plainKey(v, 'h'):
			kind = vault.FieldHidden
		}
		if kind != 0 {
			m.formKind = kind
			m.input = ""
			m.visible = false
			if kind == vault.FieldNote {
				m.screen = fieldNote
			} else {
				m.screen = vaultAddValue
			}
		}
	case vaultAddValue, vaultEdit, fieldNote:
		switch {
		case v.Code == tea.KeyEsc:
			if m.screen == vaultEdit || m.formAction == vaultEdit || m.formAction == vaultItem && m.screen == fieldNote {
				m.screen = vaultItem
			} else {
				m.screen = fieldKind
			}
			m.input, m.message = "", ""
			m.visible = false
		case v.Code == tea.KeyTab && m.screen != fieldNote:
			m.visible = !m.visible
		case m.screen == fieldNote && v.Code == 's' && v.Mod&tea.ModCtrl != 0:
			m.finishValue()
		case m.screen == fieldNote && v.Code == tea.KeyEnter:
			m.input += "\n"
		case m.screen != fieldNote && v.Code == tea.KeyEnter:
			m.finishValue()
		default:
			m.editInput(v)
		}
	case vaultDelete:
		if plainKey(v, 'y') {
			if err := m.vlt.Remove(m.deleteID); err != nil {
				m.failVault(err)
				break
			}
			m.refresh()
			for len(m.folderPath) > 0 {
				present := false
				for _, entry := range m.entries {
					if hasPath(entry.Path, m.folderPath) {
						present = true
						break
					}
				}
				if present {
					break
				}
				m.folderPath = slices.Clone(m.folderPath[:len(m.folderPath)-1])
				m.cursor = 0
				m.refresh()
			}
			m.mirrorUpdate()
		}
		m.screen = vaultList
		m.message = ""
	case fieldDelete:
		if plainKey(v, 'y') {
			item := m.itemData
			item.Fields = append([]vault.Field(nil), item.Fields...)
			item.Fields = slices.Delete(item.Fields, m.fieldCursor, m.fieldCursor+1)
			if !m.saveItem(item) {
				break
			}
			id := m.itemID
			m.clearItem()
			m.enterEntry(folderRow{id: id})
		} else {
			m.screen = vaultItem
		}
	}
	return m, nil
}
func vaultListScreen(m *Model) { m.screen = vaultList; m.input = ""; m.refresh() }
