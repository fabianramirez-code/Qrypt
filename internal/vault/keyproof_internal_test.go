package vault

import (
	"bytes"
	"crypto/hpke"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/identity"
)

// Someone who can write to the vault and knows the public key can replace the
// scope file with one that wraps a new key and add a name file sealed under
// that key. A name that decrypts proves only the new key, never the original
// one, so Accept must still refuse to remove the original secrets.
func TestAcceptKeepsFilesWhenOnlyAForgedNameDecrypts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	state := filepath.Join(filepath.Dir(root), "state")
	id := fuzzIdentity2(t)
	v, err := NewScope(root, state, id, "Work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Add(Item{Title: "a", Fields: []Field{{Kind: FieldHidden, Label: "k", Value: []byte("1")}}}); err != nil {
		t.Fatal(err)
	}
	scopeID := v.ID()
	dir := filepath.Join(root, "scopes", scopeID)
	v.Close()

	raw, _ := hex.DecodeString(scopeID)
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	public := id.KEMPrivateKey().PublicKey()
	wrapped, err := hpke.Seal(public, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), wrapInfo(raw), key)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(public.Bytes())
	file := append([]byte(scopeMagic), formatVersion)
	file = append(file, raw...)
	file = append(file, 1)
	file = append(file, sum[:]...)
	file = append(file, wrapped...)
	if err := os.WriteFile(filepath.Join(dir, scopeName), file, 0600); err != nil {
		t.Fatal(err)
	}
	forged, err := openScope(dir, id, false)
	if err != nil {
		t.Fatal(err)
	}
	plain := make([]byte, namePadded)
	plain[0] = byte(len("Evil"))
	copy(plain[1:], "Evil")
	nonce := make([]byte, nonceSize)
	sealed, err := seal(forged.scopeNameKey, nonce, plain, forged.scopeNameAAD())
	if err != nil {
		t.Fatal(err)
	}
	name := append([]byte(scopeNameMagic), formatVersion)
	name = append(name, nonce...)
	name = append(name, sealed...)
	forged.Close()
	if err := os.WriteFile(filepath.Join(dir, scopeNameFile), name, 0600); err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadDir(filepath.Join(dir, secretDir))
	if err != nil || len(before) != 1 {
		t.Fatalf("secret files before: %v, %v", before, err)
	}
	if e := Accept(root, state, id, scopeID); !errors.Is(e, ErrScopeUnreadable) {
		t.Fatalf("Accept = %v, want ErrScopeUnreadable", e)
	}
	after, err := os.ReadDir(filepath.Join(dir, secretDir))
	if err != nil || len(after) != 1 || !bytes.Equal([]byte(after[0].Name()), []byte(before[0].Name())) {
		t.Fatalf("the original secret file was removed: %v, %v", after, err)
	}
}

func fuzzIdentity2(t *testing.T) *identity.Identity {
	t.Helper()
	id, err := identity.FromSeed(bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return id
}
