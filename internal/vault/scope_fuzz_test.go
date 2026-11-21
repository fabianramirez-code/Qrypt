package vault

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/identity"
)

func fuzzIdentity(f *testing.F) *identity.Identity {
	f.Helper()
	id, err := identity.FromSeed(bytes.Repeat([]byte{2}, 32))
	if err != nil {
		f.Fatal(err)
	}
	return id
}

func FuzzOpenScopeFile(f *testing.F) {
	id := fuzzIdentity(f)
	v, err := Create(f.TempDir(), id)
	if err != nil {
		f.Fatal(err)
	}
	valid, err := os.ReadFile(filepath.Join(v.dir, scopeName))
	v.Close()
	if err != nil {
		f.Fatal(err)
	}
	flipped := bytes.Clone(valid)
	flipped[len(flipped)-1] ^= 1
	for _, seed := range [][]byte{nil, []byte(scopeMagic), valid, valid[:len(valid)-1], append(bytes.Clone(valid), 0), flipped} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, scopeName), data, 0600); err != nil {
			t.Fatal(err)
		}
		opened, err := openScope(dir, id, false)
		if err != nil {
			if bytes.Equal(data, valid) {
				t.Fatalf("valid scope file rejected: %v", err)
			}
			return
		}
		opened.Close()
		if len(data) != scopeFileSize {
			t.Fatal("accepted a scope file of the wrong size")
		}
	})
}

func FuzzScopeNameFile(f *testing.F) {
	id := fuzzIdentity(f)
	v, err := Create(f.TempDir(), id)
	if err != nil {
		f.Fatal(err)
	}
	if err := v.SetName("Team"); err != nil {
		f.Fatal(err)
	}
	valid, err := os.ReadFile(filepath.Join(v.dir, scopeNameFile))
	if err != nil {
		f.Fatal(err)
	}
	flipped := bytes.Clone(valid)
	flipped[len(flipped)-1] ^= 1
	for _, seed := range [][]byte{nil, []byte(scopeNameMagic), valid, valid[:len(valid)-1], append(bytes.Clone(valid), 0), flipped} {
		f.Add(seed)
	}
	path := filepath.Join(v.dir, scopeNameFile)
	f.Fuzz(func(t *testing.T, data []byte) {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		name, err := v.nameUnchecked()
		if bytes.Equal(data, valid) {
			if err != nil || name != "Team" {
				t.Fatalf("valid name file: %q, %v", name, err)
			}
			return
		}
		if err == nil && validateName(name) != nil {
			t.Fatalf("accepted an invalid name %q", name)
		}
	})
}

func FuzzItemHead(f *testing.F) {
	id := fuzzIdentity(f)
	v, err := Create(f.TempDir(), id)
	if err != nil {
		f.Fatal(err)
	}
	hexID, err := v.Add(Item{Title: "Mail", Fields: []Field{{Kind: FieldHidden, Label: "key", Value: []byte("sample")}}})
	if err != nil {
		f.Fatal(err)
	}
	fid, err := decodeID(hexID)
	if err != nil {
		f.Fatal(err)
	}
	valid, err := os.ReadFile(v.secretPath(fid))
	if err != nil {
		f.Fatal(err)
	}
	flipped := bytes.Clone(valid)
	flipped[nameSegmentOffset] ^= 1
	for _, seed := range [][]byte{nil, []byte(secretMagic), valid, valid[:len(valid)-1], append(bytes.Clone(valid), 0), flipped} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		entry, err := v.itemHead(fid, data)
		if bytes.Equal(data, valid) {
			if err != nil || entry.Title != "Mail" {
				t.Fatalf("valid item head: %q, %v", entry.Title, err)
			}
			return
		}
		if err == nil && validateName(entry.Title) != nil {
			t.Fatalf("accepted an invalid title %q", entry.Title)
		}
	})
}
