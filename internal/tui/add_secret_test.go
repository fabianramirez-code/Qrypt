package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Adding a secret to an account works in wide mode, in both languages and for every kind key.
func TestWideAddSecretToAccount(t *testing.T) {
	for _, tc := range []struct {
		language Language
		kind     rune
	}{{English, 'h'}, {English, 'v'}, {English, 'l'}, {Spanish, 'o'}, {Spanish, 'v'}, {Spanish, 't'}} {
		m := spacesSession(t, tc.language)
		m = enter(t, m)
		m = addVaultItem(t, m, "github", "first-payload")
		m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
		for m.screen != vaultItem {
			before := m.screen
			m = key(t, m, tea.KeyRight)
			if m.screen == before && m.screen != vaultList {
				t.Fatalf("%v %q: cannot reach the account, screen %d", tc.language, tc.kind, m.screen)
			}
		}
		m = send(t, m, tea.KeyPressMsg{Code: 'a', Text: "a"})
		if m.screen != fieldLabel {
			t.Fatalf("%v %q: A opened screen %d", tc.language, tc.kind, m.screen)
		}
		m = typeText(t, m, "high level goal")
		if m.input.String() != "high level goal" {
			t.Fatalf("%v %q: label typed as %q", tc.language, tc.kind, m.input.String())
		}
		m = enter(t, m)
		if m.screen != fieldKind {
			t.Fatalf("%v %q: after the label, screen %d", tc.language, tc.kind, m.screen)
		}
		m = send(t, m, tea.KeyPressMsg{Code: tc.kind, Text: string(tc.kind)})
		if m.screen == fieldKind {
			t.Fatalf("%v %q: kind key ignored", tc.language, tc.kind)
		}
		m = typeText(t, m, "jkl-payload")
		if tc.kind == 't' || tc.kind == 'l' {
			m = send(t, m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		} else {
			m = enter(t, m)
		}
		found := false
		for _, item := range diskItems(t, m) {
			for _, field := range item.Fields {
				if field.Label == "high level goal" && string(field.Value) == "jkl-payload" {
					found = true
				}
			}
		}
		if !found || m.screen != vaultItem {
			t.Fatalf("%v %q: secret not saved (screen %d, message %q)", tc.language, tc.kind, m.screen, m.message)
		}
	}
}
