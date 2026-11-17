package vault_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/vault"
)

// A leftover temporary file is not a secret, so it does not keep a scope from being removed.
func TestRemoveScopeIgnoresTemporaryFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	id := newIdentity(t, 1)
	v := scopeAt(t, root, id, "Work")
	dir := filepath.Join(root, "scopes", v.ID())
	stray := filepath.Join(dir, "secrets", ".write-123.tmp")
	if err := os.WriteFile(stray, []byte("junk"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := vault.RemoveScope(v); err != nil {
		t.Fatalf("RemoveScope with a stray temporary file = %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("scope directory still present: %v", err)
	}
}

// A scope that lost its directory of secrets holds none and can still be removed.
func TestRemoveScopeWithoutItsSecretsDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	id := newIdentity(t, 1)
	v := scopeAt(t, root, id, "Work")
	dir := filepath.Join(root, "scopes", v.ID())
	if err := os.Remove(filepath.Join(dir, "secrets")); err != nil {
		t.Fatal(err)
	}
	if err := vault.RemoveScope(v); err != nil {
		t.Fatalf("RemoveScope = %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("scope directory still present: %v", err)
	}
}

// A removed scope must not keep the others from opening, and a real secret still blocks removal.
func TestRemoveScopeKeepsSecretsAndOtherScopes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	id := newIdentity(t, 1)
	keep := scopeAt(t, root, id, "Personal")
	if err := keep.Put("a", []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := vault.RemoveScope(keep); !errors.Is(err, vault.ErrScopeNotEmpty) {
		t.Fatalf("RemoveScope with a secret = %v", err)
	}
	gone := scopeAt(t, root, id, "Work")
	if err := vault.RemoveScope(gone); err != nil {
		t.Fatal(err)
	}
	list, problems, err := vault.OpenScopes(root, filepath.Join(filepath.Dir(root), "state"), id)
	if err != nil || len(problems) != 0 || len(list) != 1 {
		t.Fatalf("OpenScopes after a removal = %v, %v", list, err)
	}
	defer list[0].Close()
	if got, err := list[0].Get("a"); err != nil || string(got) != "1" {
		t.Fatalf("remaining secret = %q, %v", got, err)
	}
}

// What a failed creation in the earlier layout leaves behind must not make the vault unusable.
func TestEmptyEarlierSecretsDirectoryIsNotAnError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	id := newIdentity(t, 1)
	if err := os.MkdirAll(filepath.Join(root, "secrets"), 0700); err != nil {
		t.Fatal(err)
	}
	list, problems, err := vault.OpenScopes(root, filepath.Join(filepath.Dir(root), "state"), id)
	if err != nil || len(problems) != 0 || len(list) != 0 {
		t.Fatalf("OpenScopes = %v, %v", list, err)
	}
	v, err := vault.NewScope(root, filepath.Join(filepath.Dir(root), "state"), id, "Personal")
	if err != nil {
		t.Fatalf("NewScope = %v", err)
	}
	v.Close()
}

// Secret files without their scope file cannot be attached to any scope: Qrypt
// reports it and leaves every file where it is.
func TestOrphanEarlierSecretsAreKeptAndReported(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	id := newIdentity(t, 1)
	orphan := filepath.Join(root, "secrets", "00000000000000000000000000000000.qrs")
	if err := os.MkdirAll(filepath.Dir(orphan), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphan, []byte("unreadable"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := vault.OpenScopes(root, filepath.Join(filepath.Dir(root), "state"), id); !errors.Is(err, vault.ErrInvalidFormat) {
		t.Fatalf("OpenScopes = %v", err)
	}
	if data, err := os.ReadFile(orphan); err != nil || string(data) != "unreadable" {
		t.Fatalf("orphan file changed: %q, %v", data, err)
	}
}
