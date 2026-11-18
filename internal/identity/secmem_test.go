package identity_test

import (
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/secmem"
)

func TestIdentityClosesBuffer(t *testing.T) {
	before := secmem.Live()
	id, err := identity.FromSeed(make([]byte, identity.SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	if got := secmem.Live(); got != before+1 {
		t.Fatalf("live buffers: %d want %d", got, before+1)
	}
	id.Close()
	id.Close()
	var nilID *identity.Identity
	nilID.Close()
	if got := secmem.Live(); got != before {
		t.Fatalf("live buffers after close: %d want %d", got, before)
	}
}
