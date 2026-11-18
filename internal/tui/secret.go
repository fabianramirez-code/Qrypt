package tui

import "bytes"

// secretText holds text the user typed or the app generated (passphrases, words,
// field values) as bytes that are overwritten as soon as they are discarded.
// Copies of a Model share the backing array, so wiping one wipes all of them.
// String and Bytes return transient copies for display and comparison; callers
// must not keep them.
type secretText struct{ b []byte }

func (s *secretText) Set(text string) { s.Wipe(); s.b = append(s.b, text...) }

func (s *secretText) Append(text string) { s.b = append(s.b, text...) }

// Backspace removes the last rune and overwrites its bytes.
func (s *secretText) Backspace() {
	end := len(s.b)
	if end == 0 {
		return
	}
	start := end - 1
	for start > 0 && s.b[start]&0xC0 == 0x80 {
		start--
	}
	clear(s.b[start:end])
	s.b = s.b[:start]
}

func (s *secretText) Wipe() {
	clear(s.b[:cap(s.b)])
	s.b = s.b[:0]
}

func (s secretText) String() string { return string(s.b) }
func (s secretText) Len() int       { return len(s.b) }
func (s secretText) Empty() bool    { return len(s.b) == 0 }
func (s secretText) Is(text string) bool {
	return bytes.Equal(s.b, []byte(text))
}

// Copy returns an independent byte copy; the caller clears it.
func (s secretText) Copy() []byte { return append([]byte(nil), s.b...) }
