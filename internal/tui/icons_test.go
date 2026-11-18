package tui

import (
	"charm.land/lipgloss/v2"
	"testing"
)

func TestIconGlyphs(t *testing.T) {
	names := []string{"spaceClosed", "spaceOpen", "item", "locked", "reveal", "copy", "problem", "verified"}
	sets := map[string][]string{
		"unicode": {"▸", "▾", "◆", "●", "◉", "⎘", "!", "✓"},
		"emoji":   {"📁", "📂", "🔑", "🔒", "👁", "📋", "⚠", "✓"},
		"nerd":    {"\uf07b", "\uf07c", "\uf084", "\uf023", "\uf06e", "\uf0c5", "\uf071", "\uf00c"},
	}
	for set, glyphs := range sets {
		for i, name := range names {
			if got := IconGlyph(set, name); got != glyphs[i] {
				t.Errorf("%s %s = %q want %q", set, name, got, glyphs[i])
			}
			if set == "unicode" && lipgloss.Width(glyphs[i]) != 1 {
				t.Errorf("%s is not one cell", name)
			}
		}
	}
}
