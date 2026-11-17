package vault_test

import (
	"bytes"
	"crypto/hkdf"
	"crypto/hpke"
	"crypto/sha256"
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

func scopeAt(t *testing.T, root string, id *identity.Identity, name string) *vault.Vault {
	t.Helper()
	v, err := vault.NewScope(root, id, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	return v
}

func TestScopeNameFormatAndIntegrity(t *testing.T) {
	v, dir, id := newVault(t)
	if n, e := v.Name(); e != nil || n != "Personal" {
		t.Fatalf("default: %q %v", n, e)
	}
	if len(v.ID()) != 32 || v.ID() != strings.ToLower(v.ID()) {
		t.Fatalf("ID: %q", v.ID())
	}
	if _, e := hex.DecodeString(v.ID()); e != nil {
		t.Fatal(e)
	}
	if e := v.SetName("Trabajo"); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "name")
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if len(data) != 304 || string(data[:8]) != "QRYPTSN\x01" {
		t.Fatalf("name layout: %d %q", len(data), data[:8])
	}
	scope, e := os.ReadFile(filepath.Join(dir, "scope"))
	if e != nil {
		t.Fatal(e)
	}
	key, e := hpke.Open(id.KEMPrivateKey(), hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), append([]byte("qrypt/v1/vault/scope-key"), scope[8:24]...), scope[57:])
	if e != nil {
		t.Fatal(e)
	}
	nameKey, e := hkdf.Key(sha256.New, key, nil, "qrypt/v1/vault/scope/name", 32)
	if e != nil {
		t.Fatal(e)
	}
	aead, e := chacha20poly1305.NewX(nameKey)
	if e != nil {
		t.Fatal(e)
	}
	plain, e := aead.Open(nil, data[8:32], data[32:], append([]byte("QRYPTSN\x01"), scope[8:24]...))
	if e != nil {
		t.Fatal(e)
	}
	if len(plain) != 256 || plain[0] != 7 || string(plain[1:8]) != "Trabajo" || !bytes.Equal(plain[8:], make([]byte, 248)) {
		t.Fatal("name padding")
	}
	reopened, e := vault.Open(dir, id)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if n, e := reopened.Name(); e != nil || n != "Trabajo" {
		t.Fatalf("roundtrip: %q %v", n, e)
	}
	if e := v.SetName("Work"); e != nil {
		t.Fatal(e)
	}
	next, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Equal(data[8:32], next[8:32]) {
		t.Fatal("nonce reused")
	}
	entries, e := os.ReadDir(dir)
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range entries {
		if strings.HasSuffix(f.Name(), ".tmp") {
			t.Fatal("temp left")
		}
	}
	next[40] ^= 1
	if e := os.WriteFile(path, next, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := v.Name(); !errors.Is(e, vault.ErrIntegrity) {
		t.Fatalf("tamper: %v", e)
	}
}

func TestScopeNameValidationAndBinding(t *testing.T) {
	root := t.TempDir()
	id := newIdentity(t, 1)
	a := scopeAt(t, root, id, "First")
	b := scopeAt(t, root, id, "Second")
	before := snapshot(t, root)
	for _, n := range []string{"", strings.Repeat("a", 256), "bad\x00name", "line\nbreak", "\xff"} {
		if e := a.SetName(n); !errors.Is(e, vault.ErrInvalidName) {
			t.Fatalf("%q: %v", n, e)
		}
		if !reflect.DeepEqual(before, snapshot(t, root)) {
			t.Fatal("invalid name changed files")
		}
	}
	if e := a.SetName(strings.Repeat("é", 127)); e != nil {
		t.Fatal(e)
	}
	from, e := os.ReadFile(filepath.Join(root, "scopes", a.ID(), "name"))
	if e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, "scopes", b.ID(), "name"), from, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := b.Name(); !errors.Is(e, vault.ErrIntegrity) {
		t.Fatalf("foreign name: %v", e)
	}
}

func TestScopeManagement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	id := newIdentity(t, 1)
	if list, e := vault.OpenScopes(root, id); e != nil || len(list) != 0 {
		t.Fatalf("missing: %v %v", list, e)
	}
	work := scopeAt(t, root, id, "Work")
	dir := filepath.Join(root, "scopes", work.ID())
	for _, p := range []string{"scope", "name", "secrets"} {
		if _, e := os.Stat(filepath.Join(dir, p)); e != nil {
			t.Fatal(e)
		}
	}
	before := snapshot(t, root)
	if _, e := vault.NewScope(root, id, "Work"); !errors.Is(e, vault.ErrScopeNameExists) {
		t.Fatalf("duplicate: %v", e)
	}
	personal := scopeAt(t, root, id, "Personal")
	beforeRename := snapshot(t, root)
	if e := vault.RenameScope(root, id, work, "Personal"); !errors.Is(e, vault.ErrScopeNameExists) {
		t.Fatalf("rename duplicate: %v", e)
	}
	if !reflect.DeepEqual(beforeRename, snapshot(t, root)) {
		t.Fatal("duplicate rename changed files")
	}
	if e := vault.RenameScope(root, id, work, "Work"); e != nil {
		t.Fatal(e)
	}
	if e := work.Put("item", []byte("private")); e != nil {
		t.Fatal(e)
	}
	if _, e := personal.Get("item"); !errors.Is(e, vault.ErrNotFound) {
		t.Fatalf("foreign secret: %v", e)
	}
	foreign := secretFiles(t, dir)[0]
	blob, e := os.ReadFile(foreign)
	if e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(root, "scopes", personal.ID(), "secrets", filepath.Base(foreign)), blob, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := personal.List(); !errors.Is(e, vault.ErrIntegrity) {
		t.Fatalf("foreign ciphertext: %v", e)
	}
	if e := os.Remove(filepath.Join(root, "scopes", personal.ID(), "secrets", filepath.Base(foreign))); e != nil {
		t.Fatal(e)
	}
	names, e := vault.OpenScopes(root, id)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		for _, v := range names {
			v.Close()
		}
	}()
	if len(names) != 2 || names[0].ID() != personal.ID() || names[1].ID() != work.ID() {
		t.Fatal("unsorted scopes")
	}
	nonempty := snapshot(t, root)
	if e := vault.RemoveScope(work); !errors.Is(e, vault.ErrScopeNotEmpty) {
		t.Fatalf("nonempty: %v", e)
	}
	if !reflect.DeepEqual(nonempty, snapshot(t, root)) {
		t.Fatal("nonempty removal changed files")
	}
	if e := vault.RemoveScope(personal); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(root, "scopes", personal.ID())); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("not removed: %v", e)
	}
	if _, e := personal.Name(); !errors.Is(e, vault.ErrClosed) {
		t.Fatalf("not closed: %v", e)
	}
	_ = before
}

func TestOpenScopesRejectsDamagedOrForeign(t *testing.T) {
	root := t.TempDir()
	id := newIdentity(t, 1)
	a := scopeAt(t, root, id, "A")
	b := scopeAt(t, root, id, "B")
	path := filepath.Join(root, "scopes", b.ID(), "name")
	original, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	broken := bytes.Clone(original)
	broken[40] ^= 1
	if e := os.WriteFile(path, broken, 0600); e != nil {
		t.Fatal(e)
	}
	if list, e := vault.OpenScopes(root, id); list != nil || !errors.Is(e, vault.ErrIntegrity) {
		t.Fatalf("damaged: %v %v", list, e)
	}
	if e := os.WriteFile(path, original, 0600); e != nil {
		t.Fatal(e)
	}
	other := newIdentity(t, 2)
	if list, e := vault.OpenScopes(root, other); list != nil || !errors.Is(e, vault.ErrDecryption) {
		t.Fatalf("other identity: %v %v", list, e)
	}
	if e := os.Mkdir(filepath.Join(root, "scopes", "damaged"), 0700); e != nil {
		t.Fatal(e)
	}
	// An entry that is not a scope directory, such as one the system adds, is ignored.
	if e := os.WriteFile(filepath.Join(root, "scopes", ".DS_Store"), []byte("x"), 0600); e != nil {
		t.Fatal(e)
	}
	list, e := vault.OpenScopes(root, id)
	if e != nil || len(list) != 2 {
		t.Fatalf("foreign entries: %v %v", list, e)
	}
	for _, v := range list {
		v.Close()
	}
	_ = a
}

func TestEarlierLayoutMoveAndRecovery(t *testing.T) {
	for _, partial := range []string{"none", "scope", "secrets"} {
		t.Run(partial, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "vault")
			id := newIdentity(t, 1)
			v, e := vault.Create(root, id)
			if e != nil {
				t.Fatal(e)
			}
			defer v.Close()
			if e := v.Put("old", []byte("still readable")); e != nil {
				t.Fatal(e)
			}
			prior := snapshot(t, root)
			if partial != "none" {
				dest := filepath.Join(root, "scopes", v.ID())
				if e := os.MkdirAll(dest, 0700); e != nil {
					t.Fatal(e)
				}
				if e := os.Rename(filepath.Join(root, partial), filepath.Join(dest, partial)); e != nil {
					t.Fatal(e)
				}
			}
			opened, e := vault.OpenScopes(root, id)
			if e != nil {
				t.Fatal(e)
			}
			defer func() {
				for _, x := range opened {
					x.Close()
				}
			}()
			if len(opened) != 1 {
				t.Fatalf("scopes: %d", len(opened))
			}
			n, e := opened[0].Name()
			if e != nil || n != "Personal" {
				t.Fatalf("name: %q %v", n, e)
			}
			value, e := opened[0].Get("old")
			if e != nil || string(value) != "still readable" {
				t.Fatalf("value: %q %v", value, e)
			}
			for path, data := range prior {
				relative, e := filepath.Rel(root, path)
				if e != nil {
					t.Fatal(e)
				}
				after, e := os.ReadFile(filepath.Join(root, "scopes", v.ID(), relative))
				if e != nil || string(after) != data {
					t.Fatalf("moved %s: %v", relative, e)
				}
			}
		})
	}
}
