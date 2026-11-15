package identity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"strings"
)

const goldenFile = "5152595054494401a0a1a2a3a4a5a6a7a8a9aaabacadaeafb0b1b2b3b4b5b6b7b8b9babbbcbdbebfc0c1c2c3c4c5c6c713861e71bae6ac5a4e8415e95078abf7e129613fc8e67824dc680d6800b81cba46a3119dd4c4da2177bd1966edc0661b"

func TestPassphraseGeneratorDistribution(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 8572; i++ {
		phrase, err := generatePassphrase()
		if err != nil {
			t.Fatal(err)
		}
		for _, word := range strings.Fields(phrase) {
			seen[word] = true
		}
	}
	if len(seen) != 2048 {
		t.Fatalf("generator reached %d of 2048 words", len(seen))
	}
}

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
	id, err := Open(file, []byte("abandon ability able about above absent absorb"))
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
