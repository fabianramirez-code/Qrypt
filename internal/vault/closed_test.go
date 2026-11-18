package vault_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

func TestClosedVaultAndIdentity(t *testing.T) {
	id, err := identity.FromSeed(make([]byte, identity.SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	state := filepath.Join(root, "state")
	v, err := vault.NewScope(root, state, id, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	v.Close()
	id.Close()
	for name, call := range map[string]func() error{
		"Name":        func() error { _, e := v.Name(); return e },
		"SetName":     func() error { return v.SetName("Other") },
		"Entries":     func() error { _, e := v.Entries(); return e },
		"Read":        func() error { _, e := v.Read("00000000000000000000000000000000"); return e },
		"Add":         func() error { _, e := v.Add(vault.Item{Title: "item"}); return e },
		"Replace":     func() error { return v.Replace("00000000000000000000000000000000", vault.Item{Title: "item"}) },
		"Remove":      func() error { return v.Remove("00000000000000000000000000000000") },
		"MoveFolder":  func() error { return v.MoveFolder([]string{"from"}, []string{"to"}) },
		"RemoveScope": func() error { return vault.RemoveScope(v) },
	} {
		t.Run(name, func(t *testing.T) {
			if e := call(); !errors.Is(e, vault.ErrClosed) {
				t.Errorf("got %v, want ErrClosed", e)
			}
		})
	}
	if id.KEMPrivateKey() != nil || id.Ed25519PrivateKey() != nil || id.MLDSA65PrivateKey() != nil {
		t.Error("closed identity returned keys")
	}
	if got := id.PublicKeys(); len(got.KEM)+len(got.Ed25519)+len(got.MLDSA65) != 0 {
		t.Error("closed identity returned public keys")
	}
	if len(id.RecoveryPhrase()) != 0 {
		t.Error("closed identity returned recovery phrase")
	}
	if _, _, e := id.Seal(); !errors.Is(e, identity.ErrClosed) {
		t.Errorf("Seal: %v", e)
	}
	for name, call := range map[string]func() error{
		"Create":     func() error { _, e := vault.Create(filepath.Join(root, "new"), id); return e },
		"Open":       func() error { _, e := vault.Open(root, id); return e },
		"NewScope":   func() error { _, e := vault.NewScope(root, state, id, "Other"); return e },
		"OpenScopes": func() error { _, _, e := vault.OpenScopes(root, state, id); return e },
	} {
		t.Run(name, func(t *testing.T) {
			if e := call(); !errors.Is(e, identity.ErrClosed) {
				t.Errorf("got %v, want identity.ErrClosed", e)
			}
		})
	}
}
