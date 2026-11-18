package vault_test

import (
	"bytes"
	"crypto/hkdf"
	"crypto/hpke"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/vault"
	"golang.org/x/crypto/chacha20poly1305"
)

// writeLegacyFixture independently seals the documented version 1 segments.
// Reopening without a manifest uses the package's legacy manifest migration
// to sign the fixture through the ordinary writer.
func writeLegacyFixture(t *testing.T, dir string, id *identity.Identity, name string, value []byte) {
	t.Helper()
	scope, err := os.ReadFile(filepath.Join(dir, "scope"))
	if err != nil {
		t.Fatal(err)
	}
	scopeID := scope[8:24]
	key, err := hpke.Open(id.KEMPrivateKey(), hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), append([]byte("qrypt/v1/vault/scope-key"), scopeID...), scope[57:])
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	fid := make([]byte, 16)
	if _, err = rand.Read(fid); err != nil {
		t.Fatal(err)
	}
	aad := func(tag byte) []byte {
		a := append([]byte("QRYPTSE\x01"), scopeID...)
		a = append(a, fid...)
		return append(a, tag)
	}
	head := make([]byte, 256)
	head[0] = byte(len(name))
	copy(head[1:], name)
	body := make([]byte, (4+len(value)+255)/256*256)
	binary.BigEndian.PutUint32(body, uint32(len(value)))
	copy(body[4:], value)
	defer clear(head)
	defer clear(body)
	sealPart := func(label string, plain []byte, tag byte) ([]byte, []byte) {
		subkey, e := hkdf.Key(sha256.New, key, nil, label, 32)
		if e != nil {
			t.Fatal(e)
		}
		defer clear(subkey)
		cipher, e := chacha20poly1305.NewX(subkey)
		if e != nil {
			t.Fatal(e)
		}
		nonce := make([]byte, 24)
		if _, e = rand.Read(nonce); e != nil {
			t.Fatal(e)
		}
		return nonce, cipher.Seal(nil, nonce, plain, aad(tag))
	}
	hn, hs := sealPart("qrypt/v1/vault/secret/name", head, 1)
	bn, bs := sealPart("qrypt/v1/vault/secret/value", body, 2)
	file := append([]byte("QRYPTSE\x01"), hn...)
	file = append(file, hs...)
	file = append(file, bn...)
	file = binary.BigEndian.AppendUint32(file, uint32(len(bs)))
	file = append(file, bs...)
	if err = os.WriteFile(filepath.Join(dir, "secrets", hex.EncodeToString(fid)+".qrs"), file, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(dir, "manifest")); err != nil {
		t.Fatal(err)
	}
}

func TestItemRoundTripAndRefusals(t *testing.T) {
	v, dir, id := newVault(t)
	item := vault.Item{Path: []string{"Trabajo", "Cliente X"}, Title: "cuenta é", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "key", Value: []byte("secret")}, {Kind: vault.FieldVisible, Label: "user", Value: nil}, {Kind: vault.FieldNote, Label: "note", Value: []byte("one\ntwo")}}}
	fileID, err := v.Add(item)
	if err != nil {
		t.Fatal(err)
	}
	if len(fileID) != 32 {
		t.Fatalf("id %q", fileID)
	}
	if got, err := v.Read(fileID); err != nil || !reflect.DeepEqual(got, item) {
		t.Fatalf("Read: %+v %v", got, err)
	}
	if got, err := v.Entries(); err != nil || len(got) != 1 || got[0].Title != item.Title || !reflect.DeepEqual(got[0].Path, item.Path) {
		t.Fatalf("Entries: %+v %v", got, err)
	}
	before := snapshot(t, dir)
	for _, tc := range []struct {
		item vault.Item
		want error
	}{
		{vault.Item{Title: ""}, vault.ErrInvalidName},
		{vault.Item{Title: "bad\nname"}, vault.ErrInvalidName},
		{vault.Item{Title: "x", Path: []string{""}}, vault.ErrInvalidName},
		{vault.Item{Title: "x", Fields: []vault.Field{{Kind: 9, Label: "bad"}}}, vault.ErrInvalidField},
		{vault.Item{Title: "x", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "a", Value: []byte("x\ny")}}}, vault.ErrInvalidField},
		{vault.Item{Title: "x", Fields: []vault.Field{{Kind: vault.FieldNote, Label: "a"}, {Kind: vault.FieldNote, Label: "a"}}}, vault.ErrLabelExists},
		{vault.Item{Title: "x", Fields: make([]vault.Field, 65)}, vault.ErrTooManyFields},
		{vault.Item{Title: "x", Path: []string{strings.Repeat("a", 255), strings.Repeat("b", 255)}}, vault.ErrPathTooLong},
		{item, vault.ErrNameExists},
	} {
		if _, err := v.Add(tc.item); !errors.Is(err, tc.want) {
			t.Fatalf("Add: got %v want %v", err, tc.want)
		}
		if !sameSnapshot(before, snapshot(t, dir)) {
			t.Fatal("refusal changed disk")
		}
	}
	changed := vault.Item{Path: []string{"New"}, Title: "new", Fields: nil}
	if err := v.Replace(fileID, changed); err != nil {
		t.Fatal(err)
	}
	if got, err := v.Read(fileID); err != nil || !reflect.DeepEqual(got, changed) {
		t.Fatalf("Replace: %+v %v", got, err)
	}
	if err := v.MoveFolder([]string{"New"}, []string{"Deep", "Other"}); err != nil {
		t.Fatal(err)
	}
	if got, err := v.Read(fileID); err != nil || !reflect.DeepEqual(got.Path, []string{"Deep", "Other"}) {
		t.Fatalf("MoveFolder: %+v %v", got, err)
	}
	if err := v.Remove(fileID); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Read(fileID); !errors.Is(err, vault.ErrNotFound) {
		t.Fatalf("removed: %v", err)
	}
	reopened, err := vault.Open(filepath.Join(dir), id)
	if err != nil {
		t.Fatal(err)
	}
	reopened.Close()
	if len(secretFiles(t, dir)) != 0 {
		t.Fatal("file remains")
	}
}

func TestItemLimitsAndSorting(t *testing.T) {
	v, dir, _ := newVault(t)
	longPath := []string{strings.Repeat("a", 255), strings.Repeat("b", 253)}
	cases := []struct {
		name string
		item vault.Item
		want error
	}{
		{"title255", vault.Item{Title: strings.Repeat("t", 255)}, nil},
		{"title256", vault.Item{Title: strings.Repeat("t", 256)}, vault.ErrInvalidName},
		{"path511", vault.Item{Title: "path", Path: longPath}, nil},
		{"path512", vault.Item{Title: "path", Path: []string{strings.Repeat("a", 255), strings.Repeat("b", 254)}}, vault.ErrPathTooLong},
		{"fields64", vault.Item{Title: "many", Fields: func() []vault.Field {
			f := make([]vault.Field, 64)
			for i := range f {
				f[i] = vault.Field{Kind: vault.FieldVisible, Label: string(rune('a'+i/26)) + string(rune('a'+i%26))}
			}
			return f
		}()}, nil},
		{"fields65", vault.Item{Title: "too many", Fields: make([]vault.Field, 65)}, vault.ErrTooManyFields},
		{"label255", vault.Item{Title: "label", Fields: []vault.Field{{Kind: vault.FieldNote, Label: strings.Repeat("a", 255)}}}, nil},
		{"label256", vault.Item{Title: "bad label", Fields: []vault.Field{{Kind: vault.FieldNote, Label: strings.Repeat("a", 256)}}}, vault.ErrInvalidName},
		{"encoded65788", vault.Item{Title: "size", Fields: []vault.Field{{Kind: vault.FieldNote, Label: "a", Value: bytes.Repeat([]byte("x"), 65780)}}}, nil},
		{"encoded65789", vault.Item{Title: "sizebad", Fields: []vault.Field{{Kind: vault.FieldNote, Label: "a", Value: bytes.Repeat([]byte("x"), 65781)}}}, vault.ErrValueTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := snapshot(t, dir)
			id, err := v.Add(tc.item)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Add: %v want %v", err, tc.want)
			}
			if tc.want != nil {
				if !sameSnapshot(before, snapshot(t, dir)) {
					t.Fatal("refusal changed files")
				}
			} else if got, e := v.Read(id); e != nil || !reflect.DeepEqual(got, tc.item) {
				t.Fatalf("Read %+v %v", got, e)
			}
		})
	}
	if _, err := v.Add(vault.Item{Title: "path", Path: []string{"other"}}); err != nil {
		t.Fatal(err)
	}
	entries, err := v.Entries()
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(entries); i++ {
		if len(entries[i-1].Path) == 0 && len(entries[i].Path) != 0 {
			continue
		}
		if reflect.DeepEqual(entries[i-1].Path, entries[i].Path) && entries[i-1].Title > entries[i].Title {
			t.Fatal("unsorted titles")
		}
	}
}

func TestItemVersion2Conformance(t *testing.T) {
	v, dir, id := newVault(t)
	item := vault.Item{Title: "cuenta é", Path: []string{"Trabajo", "Cliente X"}, Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "key", Value: []byte("secret")}, {Kind: vault.FieldVisible, Label: "user", Value: []byte("alice")}, {Kind: vault.FieldNote, Label: "note", Value: []byte("one\ntwo")}}}
	fileID, err := v.Add(item)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.ReadFile(filepath.Join(dir, "secrets", fileID+".qrs"))
	if err != nil {
		t.Fatal(err)
	}
	scope, err := os.ReadFile(filepath.Join(dir, "scope"))
	if err != nil {
		t.Fatal(err)
	}
	scopeKey, err := hpke.Open(id.KEMPrivateKey(), hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), append([]byte("qrypt/v1/vault/scope-key"), scope[8:24]...), scope[57:])
	if err != nil {
		t.Fatal(err)
	}
	defer clear(scopeKey)
	fid, err := hex.DecodeString(fileID)
	if err != nil {
		t.Fatal(err)
	}
	aad := func(tag byte) []byte {
		a := append([]byte("QRYPTSE\x02"), scope[8:24]...)
		a = append(a, fid...)
		return append(a, tag)
	}
	decrypt := func(label string, nonce, sealed []byte, tag byte) []byte {
		key, e := hkdf.Key(sha256.New, scopeKey, nil, label, 32)
		if e != nil {
			t.Fatal(e)
		}
		defer clear(key)
		cipher, e := chacha20poly1305.NewX(key)
		if e != nil {
			t.Fatal(e)
		}
		plain, e := cipher.Open(nil, nonce, sealed, aad(tag))
		if e != nil {
			t.Fatal(e)
		}
		return plain
	}
	if string(file[:8]) != "QRYPTSE\x02" || len(file) != 844+272 || binary.BigEndian.Uint32(file[840:844]) != 272 {
		t.Fatalf("layout %d", len(file))
	}
	head := decrypt("qrypt/v1/vault/secret/name", file[8:32], file[32:816], 1)
	defer clear(head)
	wantHead := make([]byte, 768)
	wantHead[0] = byte(len(item.Title))
	copy(wantHead[1:], item.Title)
	p := wantHead[256:]
	p[0] = 2
	p[1] = 7
	copy(p[2:], "Trabajo")
	p[9] = 9
	copy(p[10:], "Cliente X")
	if !bytes.Equal(head, wantHead) {
		t.Fatal("head layout or padding")
	}
	body := decrypt("qrypt/v1/vault/secret/value", file[816:840], file[844:], 2)
	defer clear(body)
	encoded := []byte{3}
	for _, field := range item.Fields {
		encoded = append(encoded, byte(field.Kind), byte(len(field.Label)))
		encoded = append(encoded, field.Label...)
		encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(field.Value)))
		encoded = append(encoded, field.Value...)
	}
	wantBody := make([]byte, 256)
	binary.BigEndian.PutUint32(wantBody, uint32(len(encoded)))
	copy(wantBody[4:], encoded)
	if !bytes.Equal(body, wantBody) {
		t.Fatal("body layout or padding")
	}
	before := bytes.Clone(file)
	if err := v.Replace(fileID, item); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "secrets", fileID+".qrs"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before[8:32], after[8:32]) || bytes.Equal(before[816:840], after[816:840]) {
		t.Fatal("nonce reused")
	}
}

func TestItemPadding(t *testing.T) {
	v, dir, _ := newVault(t)
	a, err := v.Add(vault.Item{Title: "a", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "key", Value: []byte("small")}}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := v.Add(vault.Item{Title: "b", Path: []string{"nested"}, Fields: []vault.Field{{Kind: vault.FieldVisible, Label: "x", Value: []byte("one")}, {Kind: vault.FieldNote, Label: "y", Value: []byte("two")}}})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(dir, "secrets", a+".qrs"))
	second, _ := os.ReadFile(filepath.Join(dir, "secrets", b+".qrs"))
	if len(first) != len(second) || len(first) != 1116 {
		t.Fatalf("bucket lengths %d %d", len(first), len(second))
	}
}

func TestMoveFolderPreflightAndIntegrity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	state := filepath.Join(filepath.Dir(root), "state")
	id := newIdentity(t, 1)
	v, err := vault.NewScope(root, state, id, "Work")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	scopeDir := filepath.Join(root, "scopes", v.ID())
	for _, item := range []vault.Item{{Path: []string{"old"}, Title: "a"}, {Path: []string{"old", "child"}, Title: "b", Fields: []vault.Field{{Kind: vault.FieldVisible, Label: "user", Value: []byte("name")}}}, {Path: []string{"new"}, Title: "a"}} {
		if _, err := v.Add(item); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshot(t, root)
	generation := v.Generation()
	for _, tc := range []struct {
		from, to []string
		want     error
	}{
		{nil, []string{"new"}, vault.ErrInvalidName},
		{[]string{"old"}, []string{"old", "child"}, vault.ErrInvalidField},
		{[]string{"old"}, []string{"new"}, vault.ErrNameExists},
		{[]string{"old"}, []string{strings.Repeat("a", 255), strings.Repeat("b", 253)}, vault.ErrPathTooLong},
		{[]string{"bad\n"}, nil, vault.ErrInvalidName},
	} {
		if err := v.MoveFolder(tc.from, tc.to); !errors.Is(err, tc.want) {
			t.Fatalf("MoveFolder: %v want %v", err, tc.want)
		}
		if !sameSnapshot(before, snapshot(t, root)) || v.Generation() != generation {
			t.Fatal("refusal changed state")
		}
	}
	if err := v.MoveFolder([]string{"old"}, []string{"renamed"}); err != nil {
		t.Fatal(err)
	}
	if v.Generation() != generation+2 {
		t.Fatalf("generation %d", v.Generation())
	}
	scopes, problems, err := vault.OpenScopes(root, state, id)
	if err != nil || len(problems) != 0 || len(scopes) != 1 {
		t.Fatalf("OpenScopes %+v %+v %v", scopes, problems, err)
	}
	scopes[0].Close()
	if err := v.MoveFolder([]string{"renamed"}, nil); err != nil {
		t.Fatal(err)
	}
	entries, err := v.Entries()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Title == "b" && !reflect.DeepEqual(entry.Path, []string{"child"}) {
			t.Fatalf("nested move %+v", entry)
		}
		if entry.Title == "a" && len(entry.Path) != 0 && !reflect.DeepEqual(entry.Path, []string{"new"}) {
			t.Fatalf("top move %+v", entry)
		}
	}
	files := secretFiles(t, scopeDir)
	blob, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	blob[len(blob)-1] ^= 1
	if err = os.WriteFile(files[0], blob, 0600); err != nil {
		t.Fatal(err)
	}
	_, problems, err = vault.OpenScopes(root, state, id)
	if err != nil || len(problems) != 1 || problems[0].Changed != 1 {
		t.Fatalf("tamper %+v %v", problems, err)
	}
}

func TestLegacyItemReadAndRewrite(t *testing.T) {
	_, dir, id := newVault(t)
	writeLegacyFixture(t, dir, id, "old", []byte("first\nsecond"))
	v, err := vault.Open(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	files := secretFiles(t, dir)
	original, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	entries, err := v.Entries()
	if err != nil || len(entries) != 1 || entries[0].Title != "old" || len(entries[0].Path) != 0 {
		t.Fatalf("Entries %+v %v", entries, err)
	}
	item, err := v.Read(entries[0].ID)
	if err != nil || len(item.Fields) != 1 || item.Fields[0].Kind != vault.FieldNote || item.Fields[0].Label != "value" || string(item.Fields[0].Value) != "first\nsecond" {
		t.Fatalf("Read %+v %v", item, err)
	}
	unchanged, err := os.ReadFile(files[0])
	if err != nil || !bytes.Equal(original, unchanged) {
		t.Fatal("read rewrote version 1")
	}
	if _, err = v.Add(vault.Item{Title: "old"}); !errors.Is(err, vault.ErrNameExists) {
		t.Fatalf("collision: %v", err)
	}
	if err = v.Replace(entries[0].ID, item); err != nil {
		t.Fatal(err)
	}
	converted, err := os.ReadFile(files[0])
	if err != nil || converted[7] != 2 {
		t.Fatalf("conversion: %v", err)
	}
}
