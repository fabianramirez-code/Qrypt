// Package identity creates, recovers, encrypts, and stores Qrypt identities.
// The fixed constructions and file format are specified in docs/identity.md.
package identity

import (
	"bytes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/hpke"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// SeedSize is the length of the master seed in bytes.
const SeedSize = 32

const (
	magic            = "QRYPTID"
	magicSize        = 7
	version          = 0x01
	versionOffset    = magicSize
	saltOffset       = versionOffset + 1
	saltSize         = 16
	nonceOffset      = saltOffset + saltSize
	nonceSize        = 24
	ciphertextOffset = nonceOffset + nonceSize
	keySize          = 32
	tagOffset        = ciphertextOffset + SeedSize
	tagSize          = 16
	fileSize         = tagOffset + tagSize

	argonTime    = 3
	argonMemory  = 256 * 1024 // KiB (256 MiB).
	argonThreads = 4

	recoveryWordCount = 24
	wordListSize      = 2048
	bitsPerWord       = 11
	bitsPerByte       = 8
	checksumSize      = 1
	kemLabel          = "qrypt/v1/identity/kem/mlkem768-x25519"
	ed25519Label      = "qrypt/v1/identity/sig/ed25519"
	mldsa65Label      = "qrypt/v1/identity/sig/mldsa65"
)

var (
	// ErrInvalidSeed means the master seed is not SeedSize bytes long.
	ErrInvalidSeed = errors.New("invalid seed length")
	// ErrWordCount means the recovery input does not contain 24 entries.
	ErrWordCount = errors.New("recovery: expected exactly 24 entries")
	// ErrUnknownWord means a recovery entry is not in the English BIP39 list.
	ErrUnknownWord = errors.New("recovery: unrecognized value (not in BIP39 dictionary)")
	// ErrChecksum means the recovery input has an invalid BIP39 checksum.
	ErrChecksum = errors.New("recovery: checksum mismatch")
	// ErrEmptyPassphrase means a passphrase was required but not provided.
	ErrEmptyPassphrase = errors.New("empty passphrase")
	// ErrInvalidFormat means the identity file has the wrong size or magic.
	ErrInvalidFormat = errors.New("invalid identity file format")
	// ErrUnsupportedVersion means the identity file has an unknown version.
	ErrUnsupportedVersion = errors.New("unsupported identity version")
	// ErrDecryption means the passphrase is wrong or the identity file is damaged.
	ErrDecryption = errors.New("wrong passphrase or damaged identity file")
	// ErrAlreadyExists means an identity file was not overwritten.
	ErrAlreadyExists = errors.New("identity file already exists")
)

//go:embed bip39_english.txt
var wordList []byte

var words = strings.Fields(string(wordList))

var wordIndex = func() map[string]int {
	if len(words) != wordListSize {
		panic("invalid embedded BIP39 word-list length")
	}
	index := make(map[string]int, wordListSize)
	for i, word := range words {
		index[word] = i
	}
	return index
}()

// Identity holds the secret master seed from which its keys are derived.
type Identity struct{ seed []byte }

// PublicKeys contains independent copies of the identity's three public keys.
type PublicKeys struct {
	// KEM is the X-Wing public key.
	KEM []byte
	// Ed25519 is the classical signature public key.
	Ed25519 []byte
	// MLDSA65 is the post-quantum signature public key.
	MLDSA65 []byte
}

// Generate creates an identity from a fresh, random 32-byte seed.
func Generate() (*Identity, error) {
	seed := make([]byte, SeedSize)
	if _, err := rand.Read(seed); err != nil {
		clear(seed)
		return nil, err
	}
	return &Identity{seed: seed}, nil
}

// FromSeed creates an identity from a copy of exactly 32 seed bytes.
func FromSeed(seed []byte) (*Identity, error) {
	if len(seed) != SeedSize {
		return nil, ErrInvalidSeed
	}
	return &Identity{seed: bytes.Clone(seed)}, nil
}

func derive(seed []byte, label string) []byte {
	key, err := hkdf.Key(sha256.New, seed, nil, label, keySize)
	if err != nil {
		panic("fixed HKDF length rejected")
	}
	return key
}

// KEMPrivateKey returns the identity's secret X-Wing private key.
func (id *Identity) KEMPrivateKey() hpke.PrivateKey {
	seed := derive(id.seed, kemLabel)
	defer clear(seed)
	key, err := hpke.MLKEM768X25519().NewPrivateKey(seed)
	if err != nil {
		panic("derived X-Wing seed rejected")
	}
	return key
}

// Ed25519PrivateKey returns the identity's secret Ed25519 private key.
func (id *Identity) Ed25519PrivateKey() ed25519.PrivateKey {
	seed := derive(id.seed, ed25519Label)
	defer clear(seed)
	return ed25519.NewKeyFromSeed(seed)
}

// MLDSA65PrivateKey returns the identity's secret ML-DSA-65 private key.
func (id *Identity) MLDSA65PrivateKey() *mldsa65.PrivateKey {
	derivedSeed := derive(id.seed, mldsa65Label)
	defer clear(derivedSeed)
	var seed [mldsa65.SeedSize]byte
	copy(seed[:], derivedSeed)
	defer clear(seed[:])
	_, privateKey := mldsa65.NewKeyFromSeed(&seed)
	return privateKey
}

// PublicKeys returns copies of the X-Wing, Ed25519, and ML-DSA-65 public keys.
func (id *Identity) PublicKeys() PublicKeys {
	kemKey := id.KEMPrivateKey()
	edKey := id.Ed25519PrivateKey()
	defer clear(edKey)
	mlKey := id.MLDSA65PrivateKey()
	return PublicKeys{
		KEM:     kemKey.PublicKey().Bytes(),
		Ed25519: bytes.Clone(edKey[SeedSize:]),
		MLDSA65: mlKey.Public().(*mldsa65.PublicKey).Bytes(),
	}
}

// RecoveryPhrase returns the full secret as 24 BIP39 words; show it only to the user.
func (id *Identity) RecoveryPhrase() string {
	checksum := sha256.Sum256(id.seed)
	defer clear(checksum[:])
	bits := make([]byte, SeedSize+checksumSize)
	copy(bits, id.seed)
	bits[SeedSize] = checksum[0]
	defer clear(bits)

	// BIP39 reads the seed and its checksum as one stream of bits, most
	// significant bit first, and takes 11 bits per word. Each group of 11 bits
	// is the index of a word in the list. FromRecoveryPhrase is the exact
	// inverse of this loop: change both together.
	phraseWords := make([]string, recoveryWordCount)
	for i := range phraseWords {
		index := 0
		for bit := 0; bit < bitsPerWord; bit++ {
			position := i*bitsPerWord + bit
			index = (index << 1) | int((bits[position/bitsPerByte]>>(bitsPerByte-1-uint(position%bitsPerByte)))&1)
		}
		phraseWords[i] = words[index]
	}
	return strings.Join(phraseWords, " ")
}

// FromRecoveryPhrase recovers an identity from 24 BIP39 words, ignoring case and extra whitespace.
func FromRecoveryPhrase(phrase string) (*Identity, error) {
	phraseWords := strings.Fields(strings.ToLower(phrase))
	if len(phraseWords) != recoveryWordCount {
		return nil, ErrWordCount
	}
	// Inverse of the loop in RecoveryPhrase: write each word's 11-bit index
	// back into the bit stream, most significant bit first.
	bits := make([]byte, SeedSize+checksumSize)
	defer clear(bits)
	for i, word := range phraseWords {
		index, found := wordIndex[word]
		if !found {
			return nil, ErrUnknownWord
		}
		for bit := 0; bit < bitsPerWord; bit++ {
			position := i*bitsPerWord + bit
			bits[position/bitsPerByte] |= byte((index>>(bitsPerWord-1-bit))&1) << (bitsPerByte - 1 - uint(position%bitsPerByte))
		}
	}
	checksum := sha256.Sum256(bits[:SeedSize])
	defer clear(checksum[:])
	if bits[SeedSize] != checksum[0] {
		return nil, ErrChecksum
	}
	return FromSeed(bits[:SeedSize])
}

// passphraseAEAD derives the fixed Argon2id key and constructs XChaCha20-Poly1305.
func passphraseAEAD(passphrase, salt []byte) (cipher.AEAD, error) {
	key := argon2.IDKey(passphrase, salt, argonTime, argonMemory, argonThreads, keySize)
	defer clear(key)
	return chacha20poly1305.NewX(key)
}

// Seal encrypts the seed into a 96-byte version-1 identity file with fresh salt and nonce.
func (id *Identity) Seal(passphrase []byte) ([]byte, error) {
	if len(passphrase) == 0 {
		return nil, ErrEmptyPassphrase
	}
	file := make([]byte, fileSize)
	copy(file, magic)
	file[versionOffset] = version
	if _, err := rand.Read(file[saltOffset:ciphertextOffset]); err != nil {
		return nil, err
	}
	aead, err := passphraseAEAD(passphrase, file[saltOffset:nonceOffset])
	if err != nil {
		return nil, err
	}
	aead.Seal(file[ciphertextOffset:ciphertextOffset], file[nonceOffset:ciphertextOffset], id.seed, file[:nonceOffset])
	return file, nil
}

// Open decrypts a version-1 identity file or returns a format, version, or decryption error.
func Open(file, passphrase []byte) (*Identity, error) {
	if len(file) != fileSize || !bytes.Equal(file[:magicSize], []byte(magic)) {
		return nil, ErrInvalidFormat
	}
	if file[versionOffset] != version {
		return nil, ErrUnsupportedVersion
	}
	if len(passphrase) == 0 {
		return nil, ErrEmptyPassphrase
	}
	aead, err := passphraseAEAD(passphrase, file[saltOffset:nonceOffset])
	if err != nil {
		return nil, err
	}
	seed, err := aead.Open(nil, file[nonceOffset:ciphertextOffset], file[ciphertextOffset:], file[:nonceOffset])
	if err != nil {
		return nil, ErrDecryption
	}
	defer clear(seed)
	return FromSeed(seed)
}

// DefaultPath returns the identity path under the user's configuration directory without creating it.
func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "qrypt", "identity"), nil
}

// Save writes one valid identity file, creating its parent directory but never overwriting a file.
// A failed write or sync removes the new file; created files and directories are owner-only on Unix.
func Save(path string, sealed []byte) error {
	if len(sealed) != fileSize || !bytes.Equal(sealed[:magicSize], []byte(magic)) {
		return ErrInvalidFormat
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return ErrAlreadyExists
	}
	if err != nil {
		return err
	}
	_, writeErr := file.Write(sealed)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	return nil
}

// Load reads at most 97 bytes from disk and opens the identity file.
func Load(path string, passphrase []byte) (*Identity, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, fileSize+1))
	if err != nil {
		return nil, err
	}
	return Open(data, passphrase)
}
