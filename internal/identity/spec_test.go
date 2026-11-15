package identity_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/hpke"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
	"github.com/fabianramirez-code/qrypt/internal/identity"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// Fixed seed 000102...1f, salt a0a1...af, nonce b0b1...c7,
// and the seven-word passphrase "abandon ability able about above absent absorb".
const goldenFile = "5152595054494401a0a1a2a3a4a5a6a7a8a9aaabacadaeafb0b1b2b3b4b5b6b7b8b9babbbcbdbebfc0c1c2c3c4c5c6c713861e71bae6ac5a4e8415e95078abf7e129613fc8e67824dc680d6800b81cba46a3119dd4c4da2177bd1966edc0661b"

// TestSpecConformance independently applies the fixed constructions in docs/identity.md.
// It intentionally shares no derivation, file-layout, or passphrase helpers with the package.
func TestSpecConformance(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	id, err := identity.FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	public := id.PublicKeys()

	kemSeed, err := hkdf.Key(sha256.New, seed, nil, "qrypt/v1/identity/kem/mlkem768-x25519", 32)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(kemSeed)
	actualPrivate, err := id.KEMPrivateKey().Bytes()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(actualPrivate)
	if !bytes.Equal(actualPrivate, kemSeed) {
		t.Fatal("X-Wing private key differs from the specified HKDF output")
	}
	kemKey, err := hpke.MLKEM768X25519().NewPrivateKey(kemSeed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(kemKey.PublicKey().Bytes(), public.KEM) || len(public.KEM) != 1216 {
		t.Fatal("X-Wing public key differs from independent derivation")
	}

	edSeed, err := hkdf.Key(sha256.New, seed, nil, "qrypt/v1/identity/sig/ed25519", 32)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(edSeed)
	edKey := ed25519.NewKeyFromSeed(edSeed)
	defer clear(edKey)
	if !bytes.Equal(edKey.Public().(ed25519.PublicKey), public.Ed25519) || len(public.Ed25519) != 32 {
		t.Fatal("Ed25519 public key differs from independent derivation")
	}

	mlSeed, err := hkdf.Key(sha256.New, seed, nil, "qrypt/v1/identity/sig/mldsa65", 32)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(mlSeed)
	var mlSeedArray [32]byte
	copy(mlSeedArray[:], mlSeed)
	defer clear(mlSeedArray[:])
	mlPublic, _ := mldsa65.NewKeyFromSeed(&mlSeedArray)
	if !bytes.Equal(mlPublic.Bytes(), public.MLDSA65) || len(public.MLDSA65) != 1952 {
		t.Fatal("ML-DSA-65 public key differs from independent derivation")
	}

	salt := make([]byte, 16)
	for i := range salt {
		salt[i] = byte(0xa0 + i)
	}
	nonce := make([]byte, 24)
	for i := range nonce {
		nonce[i] = byte(0xb0 + i)
	}
	file := make([]byte, 0, 96)
	file = append(file, []byte("QRYPTID")...)
	file = append(file, 0x01)
	file = append(file, salt...)
	file = append(file, nonce...)
	key := argon2.IDKey([]byte("abandon ability able about above absent absorb"), salt, 3, 262144, 4, 32)
	aead, err := chacha20poly1305.NewX(key)
	clear(key)
	if err != nil {
		t.Fatal(err)
	}
	file = aead.Seal(file, nonce, seed, file[:24])
	expected, err := hex.DecodeString(goldenFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(file) != 96 || !bytes.Equal(file, expected) {
		t.Fatalf("independently constructed 96-byte identity file differs: got %x, want %x", file, expected)
	}
}
