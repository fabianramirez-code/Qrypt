package tui

// IconGlyph returns the glyph for a named icon in a selected terminal icon set.
// Unknown sets use the portable Unicode symbols; unknown icons return an empty string.
func IconGlyph(set, name string) string {
	names := map[string]int{
		"spaceClosed": 0, "spaceOpen": 1, "item": 2, "locked": 3,
		"reveal": 4, "copy": 5, "problem": 6, "verified": 7,
	}
	index, ok := names[name]
	if !ok {
		return ""
	}
	switch set {
	case "emoji":
		return [...]string{"📁", "📂", "🔑", "🔒", "👁", "📋", "⚠", "✓"}[index]
	case "nerd":
		return [...]string{"\uf07b", "\uf07c", "\uf084", "\uf023", "\uf06e", "\uf0c5", "\uf071", "\uf00c"}[index]
	default:
		return [...]string{"▸", "▾", "◆", "●", "◉", "⎘", "!", "✓"}[index]
	}
}
