package tui

import (
	"bytes"
	"testing"
)

func TestSecretTextWipesBackingBytes(t *testing.T) {
	var s secretText
	s.Append("añb")
	backing := s.b[:cap(s.b)]
	s.Backspace()
	if s.String() != "añ" || backing[3] != 0 {
		t.Fatalf("backspace left %q, freed byte %d", s.String(), backing[3])
	}
	s.Backspace()
	if s.String() != "a" || backing[1] != 0 || backing[2] != 0 {
		t.Fatalf("backspace did not remove the whole rune: %q", s.String())
	}
	s.Wipe()
	if !s.Empty() || !bytes.Equal(backing, make([]byte, len(backing))) {
		t.Fatalf("wipe left bytes behind: %v", backing)
	}
	s.Backspace()
}

func TestLockWipesSecrets(t *testing.T) {
	m, _, _ := fixture(t)
	m.pass.Set("alpha beta")
	m.phrase.Set("gamma delta")
	m.input.Set("typed")
	m.restoreWords = [][]byte{[]byte("word")}
	words := m.restoreWords[0]
	pass, phrase, input := m.pass.b[:cap(m.pass.b)], m.phrase.b[:cap(m.phrase.b)], m.input.b[:cap(m.input.b)]
	m.clearSecrets()
	for name, b := range map[string][]byte{"pass": pass, "phrase": phrase, "input": input, "restore word": words} {
		if !bytes.Equal(b, make([]byte, len(b))) {
			t.Fatalf("%s kept bytes after wipe: %q", name, b)
		}
	}
	if len(m.restoreWords) != 0 {
		t.Fatal("restore words kept")
	}
}
