package vault_test

import (
	"fmt"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

func BenchmarkListAndRead500(b *testing.B) {
	id, err := identity.FromSeed(make([]byte, identity.SeedSize))
	if err != nil {
		b.Fatal(err)
	}
	defer id.Close()
	v, err := vault.NewScope(b.TempDir(), b.TempDir()+"/state", id, "Personal")
	if err != nil {
		b.Fatal(err)
	}
	defer v.Close()
	var itemID string
	for i := 0; i < 500; i++ {
		itemID, err = v.Add(vault.Item{Title: fmt.Sprintf("item-%03d", i), Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "value", Value: []byte("sample")}}})
		if err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entries, err := v.Entries()
		if err != nil || len(entries) != 500 {
			b.Fatalf("entries %d: %v", len(entries), err)
		}
		item, err := v.Read(itemID)
		if err != nil || len(item.Fields) != 1 {
			b.Fatalf("read: %v", err)
		}
		clear(item.Fields[0].Value)
	}
}
