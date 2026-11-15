package identity_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/identity"
)

// Fixed seed 000102...1f, salt a0a1...af, nonce b0b1...c7,
// and the UTF-8 passphrase "golden-passphrase".
const goldenFile = "5152595054494401a0a1a2a3a4a5a6a7a8a9aaabacadaeafb0b1b2b3b4b5b6b7b8b9babbbcbdbebfc0c1c2c3c4c5c6c7ebbe5a42dad5572934945f3729176ee71ac8e5aa7e4c64d2daf5f365038ca77fec2c74d9f6b0094d0ab17264a237e244"

func TestGoldenFile(t *testing.T) {
	file, err := hex.DecodeString(goldenFile)
	if err != nil || len(file) != 96 {
		t.Fatalf("fixture: %v", err)
	}
	salt := make([]byte, 16)
	for i := range salt {
		salt[i] = byte(0xa0 + i)
	}
	if !bytes.Equal(file[8:24], salt) {
		t.Fatal("salt changed")
	}
	nonce := make([]byte, 24)
	for i := range nonce {
		nonce[i] = byte(0xb0 + i)
	}
	if !bytes.Equal(file[24:48], nonce) {
		t.Fatal("nonce changed")
	}
	id, err := identity.Open(file, []byte("golden-passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	keys := id.PublicKeys()
	want := [3]string{"0471f26269ade8aa0e780d32cdbe747e59ef72cd4b90779d8761d2bc4e4e22f2", "c8722e4524b419e6c7ab7ee37ee5662263160e9e483cf3ab9350de92f4297ccc", "bb5d374ac31ba7f37544a5f87e68732948d6a272f3bfba3f648b4a8dbfc0bbd7"}
	for i, key := range [][]byte{keys.KEM, keys.Ed25519, keys.MLDSA65} {
		sum := sha256.Sum256(key)
		if got := hex.EncodeToString(sum[:]); got != want[i] {
			t.Fatalf("key %d hash: got %s, want %s", i, got, want[i])
		}
	}
}
