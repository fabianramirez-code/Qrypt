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
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
	"github.com/fabianramirez-code/qrypt/internal/secmem"
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

	recoveryWordCount   = 24
	passphraseWordCount = 7
	wordListSize        = 2048
	bitsPerWord         = 11
	bitsPerByte         = 8
	checksumSize        = 1
	kemLabel            = "qrypt/v1/identity/kem/mlkem768-x25519"
	ed25519Label        = "qrypt/v1/identity/sig/ed25519"
	mldsa65Label        = "qrypt/v1/identity/sig/mldsa65"
)

var (
	// ErrInvalidSeed means the master seed is not SeedSize bytes long.
	ErrInvalidSeed = errors.New("invalid seed length")
	// ErrClosed means the identity's seed has been destroyed.
	ErrClosed = errors.New("identity is closed")
	// ErrWordCount means the recovery input does not contain 24 entries.
	ErrWordCount = errors.New("recovery: expected exactly 24 entries")
	// ErrUnknownWord means a recovery entry is not in the English BIP39 list.
	ErrUnknownWord = errors.New("recovery: unrecognized value (not in BIP39 dictionary)")
	// ErrChecksum means the recovery input has an invalid BIP39 checksum.
	ErrChecksum = errors.New("recovery: checksum mismatch")
	// ErrPassphraseWordCount means the passphrase does not contain exactly seven words.
	ErrPassphraseWordCount = errors.New("passphrase: expected exactly 7 words")
	// ErrPassphraseUnknownWord means a passphrase word is absent from the BIP39 list.
	ErrPassphraseUnknownWord = errors.New("passphrase: unrecognized word (not in BIP39 dictionary)")
	// ErrInvalidFormat means the identity file has the wrong size or magic.
	ErrInvalidFormat = errors.New("invalid identity file format")
	// ErrUnsupportedVersion means the identity file has an unknown version.
	ErrUnsupportedVersion = errors.New("unsupported identity version")
	// ErrDecryption means the passphrase is wrong or the identity file is damaged.
	ErrDecryption = errors.New("wrong passphrase or damaged identity file")
	// ErrAlreadyExists means an identity file was not overwritten.
	ErrAlreadyExists = errors.New("identity file already exists")
	// ErrBackupExists means replacement refused to overwrite an existing backup.
	ErrBackupExists = errors.New("identity backup already exists")
)

//go:embed bip39_english.txt
var wordList []byte

var words = strings.Fields(string(wordList))

var now = time.Now

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

// InWordList reports whether word is a complete BIP39 English word, ignoring
// letter case and surrounding whitespace. Inner whitespace is not accepted.
func InWordList(word string) bool {
	_, found := wordIndex[strings.ToLower(strings.TrimSpace(word))]
	return found
}

// Identity holds the secret master seed from which its keys are derived.
type Identity struct{ seed *secmem.Buffer }

// Close wipes and releases the identity seed. It may be called more than once.
func (id *Identity) Close() {
	if id == nil {
		return
	}
	id.seed.Destroy()
	id.seed = nil
}

func (id *Identity) seedBytes() []byte {
	if id == nil {
		return nil
	}
	return id.seed.Bytes()
}

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
	id, err := FromSeed(seed)
	clear(seed)
	return id, err
}

// FromSeed creates an identity from a copy of exactly 32 seed bytes.
func FromSeed(seed []byte) (*Identity, error) {
	if len(seed) != SeedSize {
		return nil, ErrInvalidSeed
	}
	buffer := secmem.New(SeedSize)
	copy(buffer.Bytes(), seed)
	return &Identity{seed: buffer}, nil
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
	if id.seedBytes() == nil {
		return nil
	}
	seed := derive(id.seedBytes(), kemLabel)
	defer clear(seed)
	key, err := hpke.MLKEM768X25519().NewPrivateKey(seed)
	if err != nil {
		panic("derived X-Wing seed rejected")
	}
	return key
}

// Ed25519PrivateKey returns the identity's secret Ed25519 private key.
func (id *Identity) Ed25519PrivateKey() ed25519.PrivateKey {
	if id.seedBytes() == nil {
		return nil
	}
	seed := derive(id.seedBytes(), ed25519Label)
	defer clear(seed)
	return ed25519.NewKeyFromSeed(seed)
}

// MLDSA65PrivateKey returns the identity's secret ML-DSA-65 private key.
func (id *Identity) MLDSA65PrivateKey() *mldsa65.PrivateKey {
	if id.seedBytes() == nil {
		return nil
	}
	derivedSeed := derive(id.seedBytes(), mldsa65Label)
	defer clear(derivedSeed)
	var seed [mldsa65.SeedSize]byte
	copy(seed[:], derivedSeed)
	defer clear(seed[:])
	_, privateKey := mldsa65.NewKeyFromSeed(&seed)
	return privateKey
}

// PublicKeys returns copies of the X-Wing, Ed25519, and ML-DSA-65 public keys.
func (id *Identity) PublicKeys() PublicKeys {
	if id.seedBytes() == nil {
		return PublicKeys{}
	}
	kemKey := id.KEMPrivateKey()
	edKey := id.Ed25519PrivateKey()
	defer clear(edKey)
	mlKey := id.MLDSA65PrivateKey()
	defer func() { *mlKey = mldsa65.PrivateKey{} }()
	return PublicKeys{
		KEM:     kemKey.PublicKey().Bytes(),
		Ed25519: bytes.Clone(edKey[SeedSize:]),
		MLDSA65: mlKey.Public().(*mldsa65.PublicKey).Bytes(),
	}
}

// RecoveryPhrase returns the full secret as 24 BIP39 words; show it only to the
// user. The caller owns the returned bytes and should clear them after use.
func (id *Identity) RecoveryPhrase() []byte {
	if id.seedBytes() == nil {
		return nil
	}
	checksum := sha256.Sum256(id.seedBytes())
	defer clear(checksum[:])
	bits := make([]byte, SeedSize+checksumSize)
	copy(bits, id.seedBytes())
	bits[SeedSize] = checksum[0]
	defer clear(bits)

	// BIP39 reads the seed and its checksum as one stream of bits, most
	// significant bit first, and takes 11 bits per word. Each group of 11 bits
	// is the index of a word in the list. FromRecoveryPhrase is the exact
	// inverse of this loop: change both together.
	phrase := make([]byte, 0, recoveryWordCount*9)
	for i := 0; i < recoveryWordCount; i++ {
		index := 0
		for bit := 0; bit < bitsPerWord; bit++ {
			position := i*bitsPerWord + bit
			index = (index << 1) | int((bits[position/bitsPerByte]>>(bitsPerByte-1-uint(position%bitsPerByte)))&1)
		}
		if i != 0 {
			phrase = append(phrase, ' ')
		}
		phrase = append(phrase, words[index]...)
	}
	return phrase
}

// FromRecoveryPhrase recovers an identity from 24 BIP39 words, ignoring case and
// extra whitespace. It does not keep or clear phrase; the caller owns it.
func FromRecoveryPhrase(phrase []byte) (*Identity, error) {
	lower := bytes.ToLower(phrase)
	defer clear(lower)
	phraseWords := bytes.Fields(lower)
	if len(phraseWords) != recoveryWordCount {
		return nil, ErrWordCount
	}
	// Inverse of the loop in RecoveryPhrase: write each word's 11-bit index
	// back into the bit stream, most significant bit first.
	bits := make([]byte, SeedSize+checksumSize)
	defer clear(bits)
	for i, word := range phraseWords {
		index, found := wordIndex[string(word)]
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

// generatePassphrase draws independent 11-bit indices from crypto/rand.
// Every index is equally likely because the BIP39 list has exactly 2^11 words.
func generatePassphrase() ([]byte, error) {
	phrase := make([]byte, 0, passphraseWordCount*9)
	var random [2]byte
	defer clear(random[:])
	for i := 0; i < passphraseWordCount; i++ {
		if _, err := rand.Read(random[:]); err != nil {
			clear(phrase)
			return nil, err
		}
		if i != 0 {
			phrase = append(phrase, ' ')
		}
		index := (int(random[0])<<8 | int(random[1])) & (wordListSize - 1)
		phrase = append(phrase, words[index]...)
	}
	return phrase, nil
}

// normalizePassphrase validates and canonicalizes typed words without retaining
// a string copy of the caller's secret. The caller clears the returned bytes.
func normalizePassphrase(passphrase []byte) ([]byte, error) {
	lower := bytes.ToLower(passphrase)
	defer clear(lower)
	parts := bytes.Fields(lower)
	if len(parts) != passphraseWordCount {
		return nil, ErrPassphraseWordCount
	}
	for _, word := range parts {
		// A map lookup keyed by string(word) does not allocate, so no string
		// copy of the typed word is created.
		if _, found := wordIndex[string(word)]; !found {
			return nil, ErrPassphraseUnknownWord
		}
	}
	return bytes.Join(parts, []byte(" ")), nil
}

// Seal encrypts the seed into a 96-byte version-1 identity file with fresh salt and nonce.
// It generates a new seven-word passphrase on every call; the returned passphrase
// is secret, must only be shown to the user, and should be cleared after use.
func (id *Identity) Seal() (sealed []byte, passphrase []byte, err error) {
	if id.seedBytes() == nil {
		return nil, nil, ErrClosed
	}
	passphrase, err = generatePassphrase()
	if err != nil {
		return nil, nil, err
	}
	canonical := passphrase
	fail := func(err error) ([]byte, []byte, error) {
		clear(passphrase)
		return nil, nil, err
	}
	file := make([]byte, fileSize)
	copy(file, magic)
	file[versionOffset] = version
	if _, err := rand.Read(file[saltOffset:ciphertextOffset]); err != nil {
		return fail(err)
	}
	aead, err := passphraseAEAD(canonical, file[saltOffset:nonceOffset])
	if err != nil {
		return fail(err)
	}
	aead.Seal(file[ciphertextOffset:ciphertextOffset], file[nonceOffset:ciphertextOffset], id.seedBytes(), file[:nonceOffset])
	return file, passphrase, nil
}

// Open checks file format first, then validates and canonicalizes seven passphrase
// words, ignoring case and extra whitespace, before deriving the decryption key.
func Open(file, passphrase []byte) (*Identity, error) {
	if len(file) != fileSize || !bytes.Equal(file[:magicSize], []byte(magic)) {
		return nil, ErrInvalidFormat
	}
	if file[versionOffset] != version {
		return nil, ErrUnsupportedVersion
	}
	canonical, err := normalizePassphrase(passphrase)
	if err != nil {
		return nil, err
	}
	defer clear(canonical)
	aead, err := passphraseAEAD(canonical, file[saltOffset:nonceOffset])
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

func validateSealed(sealed []byte) error {
	if len(sealed) != fileSize || !bytes.Equal(sealed[:magicSize], []byte(magic)) {
		return ErrInvalidFormat
	}
	if sealed[versionOffset] != version {
		return ErrUnsupportedVersion
	}
	return nil
}

// writeSealedTemp creates a synced, closed owner-only temporary file next to path.
// The caller removes the returned file after linking or renaming it.
func writeSealedTemp(path string, sealed []byte) (string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(dir, ".identity-*")
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(sealed)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(file.Name())
		if writeErr != nil {
			return "", writeErr
		}
		return "", closeErr
	}
	return file.Name(), nil
}

// Save validates and writes a whole identity file or none at the destination, never overwriting one.
// It creates the parent directory and writes to a same-directory temporary file before linking it.
func Save(path string, sealed []byte) error {
	if err := validateSealed(sealed); err != nil {
		return err
	}
	temp, err := writeSealedTemp(path, sealed)
	if err != nil {
		return err
	}
	defer os.Remove(temp)
	if err := os.Link(temp, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

// SaveReplacing writes a complete sealed identity, keeping any previous file as
// a timestamped backup before atomically replacing it. It never overwrites a
// backup, and removes its temporary file on every return path.
func SaveReplacing(path string, sealed []byte) (backup string, err error) {
	if err := validateSealed(sealed); err != nil {
		return "", err
	}
	temp, err := writeSealedTemp(path, sealed)
	if err != nil {
		return "", err
	}
	defer os.Remove(temp)

	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.Link(temp, path); err != nil {
			if errors.Is(err, os.ErrExist) {
				return "", ErrAlreadyExists
			}
			return "", err
		}
		return "", nil
	} else if err != nil {
		return "", err
	}

	backup = path + ".backup-" + now().UTC().Format("20060102T150405Z")
	if err := os.Link(path, backup); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", ErrBackupExists
		}
		return "", err
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(backup)
		return "", err
	}
	return backup, nil
}

// Load reads at most 97 bytes from disk and opens the identity file, checking
// its format before validating the seven passphrase words.
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
