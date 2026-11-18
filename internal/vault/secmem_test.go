package vault_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/secmem"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

func TestSimulatedPageRefusalStillWorks(t *testing.T) {
	for _, alloc := range []bool{false, true} {
		restore := secmem.SimulateFailureForTest(alloc, !alloc)
		id, err := identity.FromSeed(make([]byte, identity.SeedSize))
		if err != nil {
			restore()
			t.Fatal(err)
		}
		v, err := vault.NewScope(t.TempDir(), filepath.Join(t.TempDir(), "state"), id, "Personal")
		if err != nil {
			id.Close()
			restore()
			t.Fatal(err)
		}
		source := []byte("secret value")
		itemID, err := v.Add(vault.Item{Title: "sample", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "value", Value: source}}})
		if err == nil {
			var item vault.Item
			item, err = v.Read(itemID)
			if err == nil && (len(item.Fields) != 1 || !bytes.Equal(item.Fields[0].Value, source)) {
				t.Error("value mismatch after refusal")
			}
			for _, field := range item.Fields {
				clear(field.Value)
			}
		}
		v.Close()
		id.Close()
		restore()
		if err != nil {
			t.Fatal(err)
		}
		if ok, why := secmem.Status(); ok || why == "" {
			t.Fatalf("missing refusal status: %v %q", ok, why)
		}
	}
}

func Test256OpenScopes(t *testing.T) {
	before := secmem.Live()
	id, err := identity.FromSeed(make([]byte, identity.SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	state := filepath.Join(root, "state")
	if err := os.Mkdir(filepath.Join(root, "scopes"), 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 256; i++ {
		pending, e := os.MkdirTemp(root, "pending-")
		if e != nil {
			t.Fatal(e)
		}
		v, e := vault.Create(pending, id)
		if e != nil {
			t.Fatalf("scope %d: %v", i, e)
		}
		target := filepath.Join(root, "scopes", v.ID())
		v.Close()
		if e = os.Rename(pending, target); e != nil {
			t.Fatal(e)
		}
	}
	opened, problems, err := vault.OpenScopes(root, state, id)
	if err != nil || len(problems) != 0 || len(opened) != 256 {
		t.Fatalf("opened %d problems %d: %v", len(opened), len(problems), err)
	}
	for _, v := range opened {
		v.Close()
	}
	id.Close()
	if got := secmem.Live(); got != before {
		t.Fatalf("leaked %d buffers", got-before)
	}
}

func TestScopeBuffersClosed(t *testing.T) {
	before := secmem.Live()
	id, err := identity.FromSeed(make([]byte, identity.SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	v, err := vault.NewScope(root, root+"/state", id, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	if got := secmem.Live(); got != before+2 {
		t.Fatalf("live buffers: %d want %d", got, before+2)
	}
	if _, err = v.Add(vault.Item{Title: "sample", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "value", Value: []byte("secret")}}}); err != nil {
		t.Fatal(err)
	}
	v.Close()
	v.Close()
	id.Close()
	if got := secmem.Live(); got != before {
		t.Fatalf("live buffers after close: %d want %d", got, before)
	}
}
