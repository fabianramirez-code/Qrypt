package identity

import (
	"bytes"
	"sync"
	"testing"
)

// Closing while another goroutine derives keys must neither crash nor hand out
// keys from released memory: a use sees a whole identity or a closed one.
func TestCloseWhileInUse(t *testing.T) {
	for round := 0; round < 20; round++ {
		id, err := FromSeed(bytes.Repeat([]byte{7}, SeedSize))
		if err != nil {
			t.Fatal(err)
		}
		want := id.PublicKeys()
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 20; j++ {
					got := id.PublicKeys()
					if got.KEM != nil && !bytes.Equal(got.Ed25519, want.Ed25519) {
						t.Error("keys derived from a released seed")
						return
					}
					if phrase := id.RecoveryPhrase(); phrase != nil {
						clear(phrase)
					}
				}
			}()
		}
		id.Close()
		wg.Wait()
		if id.PublicKeys().KEM != nil || id.KEMPrivateKey() != nil {
			t.Fatal("closed identity still yields keys")
		}
	}
	var none *Identity
	none.Close()
	if none.KEMPrivateKey() != nil || none.RecoveryPhrase() != nil {
		t.Fatal("nil identity yields keys")
	}
}
