package vault_test

import (
	"crypto/hpke"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

// swapScopeKey replaces a scope file with a valid one for the same identity and
// scope id that wraps a different key, as someone who can write to the vault
// and knows the public key could.
func swapScopeKey(t *testing.T, id *identity.Identity, dir, scopeID string) {
	t.Helper()
	rawID, err := hex.DecodeString(scopeID)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	public := id.KEMPrivateKey().PublicKey()
	info := append([]byte("qrypt/v1/vault/scope-key"), rawID...)
	wrapped, err := hpke.Seal(public, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), info, key)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(public.Bytes())
	file := append([]byte("QRYPTSK\x01"), rawID...)
	file = append(file, 1)
	file = append(file, sum[:]...)
	file = append(file, wrapped...)
	if err := os.WriteFile(filepath.Join(dir, "scope"), file, 0600); err != nil {
		t.Fatal(err)
	}
}

// A replaced scope key makes every file undecryptable. Accepting must not then
// remove the files: they are still valid under the original key.
func TestAcceptNeverEmptiesAScopeWhoseKeyWasReplaced(t *testing.T) {
	for _, named := range []bool{true, false} {
		root := filepath.Join(t.TempDir(), "vault")
		state := filepath.Join(filepath.Dir(root), "state")
		id := newIdentity(t, 1)
		v, err := vault.NewScope(root, state, id, "Work")
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"a", "b", "c"} {
			if err := putValue(v, name, []byte("value of "+name)); err != nil {
				t.Fatal(err)
			}
		}
		scopeID := v.ID()
		dir := filepath.Join(root, "scopes", scopeID)
		v.Close()
		original, err := os.ReadFile(filepath.Join(dir, "scope"))
		if err != nil {
			t.Fatal(err)
		}
		if !named {
			// Without a name file nothing but the items can prove the key.
			if err := os.Remove(filepath.Join(dir, "name")); err != nil {
				t.Fatal(err)
			}
			if err := vault.Accept(root, state, id, scopeID); err != nil {
				t.Fatal(err)
			}
		}
		swapScopeKey(t, id, dir, scopeID)
		before := snapshot(t, root)

		scopes, problems, err := vault.OpenScopes(root, state, id)
		if err != nil || len(scopes) != 0 || len(problems) != 1 || problems[0].Kind != vault.ProblemUnreadable {
			t.Fatalf("named=%v: OpenScopes = %v, %+v, %v", named, scopes, problems, err)
		}
		if err := vault.Accept(root, state, id, scopeID); !errors.Is(err, vault.ErrScopeUnreadable) {
			t.Fatalf("named=%v: Accept = %v", named, err)
		}
		if !sameSnapshot(before, snapshot(t, root)) {
			t.Fatalf("named=%v: files changed although the key decrypts nothing", named)
		}

		// Putting the original scope file back recovers everything.
		if err := os.WriteFile(filepath.Join(dir, "scope"), original, 0600); err != nil {
			t.Fatal(err)
		}
		scopes, problems, err = vault.OpenScopes(root, state, id)
		if err != nil || len(scopes) != 1 || len(problems) != 0 {
			t.Fatalf("named=%v: after restoring the scope file = %v, %+v, %v", named, scopes, problems, err)
		}
		if got, err := readValue(scopes[0], "b"); err != nil || string(got) != "value of b" {
			t.Fatalf("named=%v: Get = %q, %v", named, got, err)
		}
		scopes[0].Close()
	}
}

// Removing a scope leaves no record of it, so the next open reports nothing.
func TestRemoveScopeLeavesNoRecord(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	state := filepath.Join(filepath.Dir(root), "state")
	id := newIdentity(t, 1)
	keep, err := vault.NewScope(root, state, id, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	keep.Close()
	gone, err := vault.NewScope(root, state, id, "Work")
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.RemoveScope(gone); err != nil {
		t.Fatal(err)
	}
	scopes, problems, err := vault.OpenScopes(root, state, id)
	if err != nil || len(scopes) != 1 || len(problems) != 0 {
		t.Fatalf("OpenScopes = %v, %+v, %v", scopes, problems, err)
	}
	scopes[0].Close()
}

// A scope with a problem must not stop new scopes from being created or
// renamed, and its readable name stays taken.
func TestScopesCanBeAddedAndRenamedBesideAProblem(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	state := filepath.Join(filepath.Dir(root), "state")
	id := newIdentity(t, 1)
	broken, err := vault.NewScope(root, state, id, "Work")
	if err != nil {
		t.Fatal(err)
	}
	if err := putValue(broken, "a", []byte("1")); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "scopes", broken.ID())
	broken.Close()
	if err := os.Remove(filepath.Join(dir, "manifest")); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.NewScope(root, state, id, "Work"); !errors.Is(err, vault.ErrScopeNameExists) {
		t.Fatalf("NewScope with the name of a problem scope = %v", err)
	}
	fresh, err := vault.NewScope(root, state, id, "Personal")
	if err != nil {
		t.Fatalf("NewScope beside a problem = %v", err)
	}
	defer fresh.Close()
	if err := vault.RenameScope(root, state, id, fresh, "Work"); !errors.Is(err, vault.ErrScopeNameExists) {
		t.Fatalf("RenameScope to the name of a problem scope = %v", err)
	}
	if err := vault.RenameScope(root, state, id, fresh, "Home"); err != nil {
		t.Fatalf("RenameScope beside a problem = %v", err)
	}
	scopes, problems, err := vault.OpenScopes(root, state, id)
	if err != nil || len(scopes) != 1 || len(problems) != 1 {
		t.Fatalf("OpenScopes = %v, %+v, %v", scopes, problems, err)
	}
	scopes[0].Close()
}
