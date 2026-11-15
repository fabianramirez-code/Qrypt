package tui

import "strings"

// Language is the selected interface language.
type Language int

const (
	// English is the default interface language.
	English Language = iota
	// Spanish is the Spanish interface language.
	Spanish
)

// FromTag maps a system language tag to an interface language.
func FromTag(tag string) Language {
	if len(tag) >= 2 && strings.EqualFold(tag[:2], "es") && (len(tag) == 2 || strings.ContainsRune("-_\u002e", rune(tag[2]))) {
		return Spanish
	}
	return English
}

func (language Language) text() texts {
	if language == Spanish {
		return spanish
	}
	return english
}
