package identity

import (
	"bytes"
	"testing"
)

func FuzzValidateSealed(f *testing.F) {
	valid := make([]byte, fileSize)
	copy(valid, magic)
	valid[versionOffset] = version
	for _, seed := range [][]byte{nil, []byte(magic), valid, valid[:len(valid)-1], append(bytes.Clone(valid), 0)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if validateSealed(data) == nil {
			if len(data) != fileSize || string(data[:magicSize]) != magic || data[versionOffset] != version {
				t.Fatal("accepted a wrong identity file layout")
			}
		}
	})
}

// FuzzNormalizePassphrase covers the typed words, the only text a person
// supplies to open an identity.
func FuzzNormalizePassphrase(f *testing.F) {
	for _, seed := range []string{"", "a b c d e f g", "  A\tB\nC  ", "\xff\xfe"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		before := bytes.Clone(data)
		out, err := normalizePassphrase(data)
		if !bytes.Equal(data, before) {
			t.Fatal("normalizePassphrase changed its input")
		}
		if err != nil {
			return
		}
		if len(bytes.Fields(out)) != passphraseWordCount || !bytes.Equal(out, bytes.ToLower(out)) {
			t.Fatalf("non canonical output %q", out)
		}
		again, err := normalizePassphrase(out)
		if err != nil || !bytes.Equal(again, out) {
			t.Fatalf("not idempotent: %q, %v", again, err)
		}
	})
}
