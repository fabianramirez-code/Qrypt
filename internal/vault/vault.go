// Package vault stores secrets as one encrypted file each, under a scope key
// wrapped for the identity. The fixed constructions and file formats are
// specified in docs/vault.md.
package vault

import (
	"bytes"
	"crypto/hkdf"
	"crypto/hpke"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/fabianramirez-code/qrypt/internal/identity"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	keySize = 32

	scopeMagic     = "QRYPTSK"
	secretMagic    = "QRYPTSE"
	magicSize      = 7
	formatVersion  = 0x01
	headerSize     = magicSize + 1
	scopeIDSize    = 16
	fileIDSize     = 16
	fingerprintLen = sha256.Size
	wrappedSize    = 1120 + keySize + chacha20poly1305.Overhead

	scopeIDOffset     = headerSize
	memberCountOffset = scopeIDOffset + scopeIDSize
	fingerprintOffset = memberCountOffset + 1
	wrappedOffset     = fingerprintOffset + fingerprintLen
	scopeFileSize     = wrappedOffset + wrappedSize

	nonceSize         = chacha20poly1305.NonceSizeX
	tagSize           = chacha20poly1305.Overhead
	nameNonceOffset   = headerSize
	nameSegmentOffset = nameNonceOffset + nonceSize
	namePadded        = 256
	nameSealedSize    = namePadded + tagSize
	valueNonceOffset  = nameSegmentOffset + nameSealedSize
	valueSizeOffset   = valueNonceOffset + nonceSize
	valueOffset       = valueSizeOffset + 4
	valueBlock        = 256
	lengthPrefixSize  = 4

	// MaxNameSize is the longest secret name, in bytes.
	MaxNameSize = namePadded - 1
	// MaxValueSize is the largest secret value, in bytes.
	MaxValueSize = 64 * 1024

	minValueSealed = valueBlock + tagSize
	maxValueSealed = ((lengthPrefixSize+MaxValueSize+valueBlock-1)/valueBlock)*valueBlock + tagSize
	maxSecretFile  = valueOffset + maxValueSealed

	secretExt = ".qrs"
	scopeName = "scope"
	secretDir = "secrets"

	wrapLabel  = "qrypt/v1/vault/scope-key"
	nameLabel  = "qrypt/v1/vault/secret/name"
	valueLabel = "qrypt/v1/vault/secret/value"

	segmentName  byte = 0x01
	segmentValue byte = 0x02
)

var (
	// ErrAlreadyExists means a scope file already exists, and was not overwritten.
	ErrAlreadyExists = errors.New("vault already exists")
	// ErrInvalidFormat means a vault file has the wrong size, magic, or layout.
	ErrInvalidFormat = errors.New("invalid vault file format")
	// ErrUnsupportedVersion means a vault file has an unknown version.
	ErrUnsupportedVersion = errors.New("unsupported vault version")
	// ErrDecryption means the identity is wrong or a vault file is damaged or foreign.
	ErrDecryption = errors.New("wrong identity or damaged vault file")
	// ErrInvalidName means a secret name is empty, too long, not UTF-8, or has control characters.
	ErrInvalidName = errors.New("invalid secret name")
	// ErrValueTooLarge means a secret value is larger than MaxValueSize.
	ErrValueTooLarge = errors.New("secret value too large")
	// ErrNameExists means a secret with that name already exists.
	ErrNameExists = errors.New("a secret with that name already exists")
	// ErrNotFound means no secret has that name.
	ErrNotFound = errors.New("secret not found")
	// ErrClosed means the vault was closed and its keys erased.
	ErrClosed = errors.New("vault is closed")
)

// DefaultDir returns the vault directory under the user's configuration directory without creating it.
func DefaultDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "qrypt", "vault"), nil
}

// Vault is an open vault holding the scope's derived keys in memory. Call Close when done.
type Vault struct {
	dir      string
	scopeID  []byte
	nameKey  []byte
	valueKey []byte
}

func derive(scopeKey []byte, label string) []byte {
	key, err := hkdf.Key(sha256.New, scopeKey, nil, label, keySize)
	if err != nil {
		panic("fixed HKDF length rejected")
	}
	return key
}

func newVault(dir string, scopeID, scopeKey []byte) *Vault {
	return &Vault{
		dir:      dir,
		scopeID:  scopeID,
		nameKey:  derive(scopeKey, nameLabel),
		valueKey: derive(scopeKey, valueLabel),
	}
}

func wrapInfo(scopeID []byte) []byte {
	return append([]byte(wrapLabel), scopeID...)
}

func fingerprint(publicKey []byte) []byte {
	sum := sha256.Sum256(publicKey)
	return sum[:]
}

// Create makes a new vault in dir for the identity, which becomes its only member.
// It refuses to overwrite an existing scope file.
func Create(dir string, id *identity.Identity) (*Vault, error) {
	scopeID := make([]byte, scopeIDSize)
	scopeKey := make([]byte, keySize)
	defer clear(scopeKey)
	if _, err := io.ReadFull(rand.Reader, scopeID); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(rand.Reader, scopeKey); err != nil {
		return nil, err
	}
	public := id.KEMPrivateKey().PublicKey()
	wrapped, err := hpke.Seal(public, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), wrapInfo(scopeID), scopeKey)
	if err != nil {
		return nil, err
	}
	file := make([]byte, 0, scopeFileSize)
	file = append(file, scopeMagic...)
	file = append(file, formatVersion)
	file = append(file, scopeID...)
	file = append(file, 1)
	file = append(file, fingerprint(public.Bytes())...)
	file = append(file, wrapped...)
	if len(file) != scopeFileSize {
		return nil, ErrInvalidFormat
	}
	if err := os.MkdirAll(filepath.Join(dir, secretDir), 0700); err != nil {
		return nil, err
	}
	if err := writeNew(filepath.Join(dir, scopeName), file); err != nil {
		return nil, err
	}
	return newVault(dir, scopeID, scopeKey), nil
}

// Open reads the scope file in dir and unwraps the scope key with the identity.
func Open(dir string, id *identity.Identity) (*Vault, error) {
	file, err := readLimited(filepath.Join(dir, scopeName), scopeFileSize)
	if err != nil {
		return nil, err
	}
	if len(file) != scopeFileSize || !bytes.Equal(file[:magicSize], []byte(scopeMagic)) {
		return nil, ErrInvalidFormat
	}
	if file[magicSize] != formatVersion {
		return nil, ErrUnsupportedVersion
	}
	if file[memberCountOffset] != 1 {
		return nil, ErrInvalidFormat
	}
	private := id.KEMPrivateKey()
	if !bytes.Equal(file[fingerprintOffset:wrappedOffset], fingerprint(private.PublicKey().Bytes())) {
		return nil, ErrDecryption
	}
	scopeID := bytes.Clone(file[scopeIDOffset:memberCountOffset])
	scopeKey, err := hpke.Open(private, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), wrapInfo(scopeID), file[wrappedOffset:])
	if err != nil || len(scopeKey) != keySize {
		return nil, ErrDecryption
	}
	defer clear(scopeKey)
	return newVault(dir, scopeID, scopeKey), nil
}

// Close erases the keys held in memory. The vault cannot be used afterward.
func (v *Vault) Close() {
	clear(v.nameKey)
	clear(v.valueKey)
	v.nameKey, v.valueKey = nil, nil
}

// ValidateName reports whether name can be used as a secret name.
func ValidateName(name string) error {
	return validateName(name)
}

func validateName(name string) error {
	if len(name) == 0 || len(name) > MaxNameSize || !utf8.ValidString(name) {
		return ErrInvalidName
	}
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return ErrInvalidName
	}
	return nil
}

// aad is the associated data of a segment: header, scope id, file id, and segment tag.
func (v *Vault) aad(fileID []byte, segment byte) []byte {
	out := make([]byte, 0, headerSize+scopeIDSize+fileIDSize+1)
	out = append(out, secretMagic...)
	out = append(out, formatVersion)
	out = append(out, v.scopeID...)
	out = append(out, fileID...)
	return append(out, segment)
}

func seal(key, nonce, plaintext, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	return aead.Seal(nil, nonce, plaintext, aad), nil
}

func open(key, nonce, sealed, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	plain, err := aead.Open(nil, nonce, sealed, aad)
	if err != nil {
		return nil, ErrDecryption
	}
	return plain, nil
}

// encode builds a whole secret file with fresh random nonces.
func (v *Vault) encode(fileID []byte, name string, value []byte) ([]byte, error) {
	namePlain := make([]byte, namePadded)
	defer clear(namePlain)
	namePlain[0] = byte(len(name))
	copy(namePlain[1:], name)

	valueLen := lengthPrefixSize + len(value)
	valuePlain := make([]byte, (valueLen+valueBlock-1)/valueBlock*valueBlock)
	defer clear(valuePlain)
	binary.BigEndian.PutUint32(valuePlain, uint32(len(value)))
	copy(valuePlain[lengthPrefixSize:], value)

	nameNonce := make([]byte, nonceSize)
	valueNonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nameNonce); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(rand.Reader, valueNonce); err != nil {
		return nil, err
	}
	sealedName, err := seal(v.nameKey, nameNonce, namePlain, v.aad(fileID, segmentName))
	if err != nil {
		return nil, err
	}
	sealedValue, err := seal(v.valueKey, valueNonce, valuePlain, v.aad(fileID, segmentValue))
	if err != nil {
		return nil, err
	}
	file := make([]byte, 0, valueOffset+len(sealedValue))
	file = append(file, secretMagic...)
	file = append(file, formatVersion)
	file = append(file, nameNonce...)
	file = append(file, sealedName...)
	file = append(file, valueNonce...)
	file = binary.BigEndian.AppendUint32(file, uint32(len(sealedValue)))
	return append(file, sealedValue...), nil
}

// checkSecretFile validates the layout of a secret file before any decryption.
func checkSecretFile(file []byte) error {
	if len(file) < valueOffset+minValueSealed || len(file) > maxSecretFile ||
		!bytes.Equal(file[:magicSize], []byte(secretMagic)) {
		return ErrInvalidFormat
	}
	if file[magicSize] != formatVersion {
		return ErrUnsupportedVersion
	}
	size := binary.BigEndian.Uint32(file[valueSizeOffset:valueOffset])
	if int(size) != len(file)-valueOffset || size < minValueSealed || (size-tagSize)%valueBlock != 0 {
		return ErrInvalidFormat
	}
	return nil
}

func (v *Vault) decodeName(fileID, file []byte) (string, error) {
	if err := checkSecretFile(file); err != nil {
		return "", err
	}
	plain, err := open(v.nameKey, file[nameNonceOffset:nameSegmentOffset],
		file[nameSegmentOffset:valueNonceOffset], v.aad(fileID, segmentName))
	if err != nil {
		return "", err
	}
	defer clear(plain)
	length := int(plain[0])
	if length == 0 || length > MaxNameSize || !allZero(plain[1+length:]) {
		return "", ErrInvalidFormat
	}
	name := string(plain[1 : 1+length])
	if validateName(name) != nil {
		return "", ErrInvalidFormat
	}
	return name, nil
}

func (v *Vault) decodeValue(fileID, file []byte) ([]byte, error) {
	if err := checkSecretFile(file); err != nil {
		return nil, err
	}
	plain, err := open(v.valueKey, file[valueNonceOffset:valueSizeOffset],
		file[valueOffset:], v.aad(fileID, segmentValue))
	if err != nil {
		return nil, err
	}
	defer clear(plain)
	length := binary.BigEndian.Uint32(plain)
	if int64(length) > MaxValueSize || int(length) > len(plain)-lengthPrefixSize ||
		len(plain) != (lengthPrefixSize+int(length)+valueBlock-1)/valueBlock*valueBlock ||
		!allZero(plain[lengthPrefixSize+int(length):]) {
		return nil, ErrInvalidFormat
	}
	return bytes.Clone(plain[lengthPrefixSize : lengthPrefixSize+int(length)]), nil
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

type entry struct {
	id   []byte
	name string
}

// scan decrypts the name segment of every secret file, never a value.
func (v *Vault) scan() ([]entry, error) {
	if v.nameKey == nil {
		return nil, ErrClosed
	}
	files, err := os.ReadDir(filepath.Join(v.dir, secretDir))
	if err != nil {
		return nil, err
	}
	var entries []entry
	for _, f := range files {
		id, ok := parseFileName(f.Name())
		if !ok || f.IsDir() {
			continue
		}
		file, err := readLimited(v.secretPath(id), maxSecretFile)
		if err != nil {
			return nil, err
		}
		name, err := v.decodeName(id, file)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry{id, name})
	}
	return entries, nil
}

func parseFileName(name string) ([]byte, bool) {
	stem, ok := strings.CutSuffix(name, secretExt)
	if !ok || len(stem) != fileIDSize*2 || stem != strings.ToLower(stem) {
		return nil, false
	}
	id, err := hex.DecodeString(stem)
	return id, err == nil
}

func (v *Vault) secretPath(id []byte) string {
	return filepath.Join(v.dir, secretDir, hex.EncodeToString(id)+secretExt)
}

func (v *Vault) find(name string) ([]byte, error) {
	entries, err := v.scan()
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.name == name {
			return e.id, nil
		}
	}
	return nil, ErrNotFound
}

func checkSecret(name string, value []byte) error {
	if err := validateName(name); err != nil {
		return err
	}
	if len(value) > MaxValueSize {
		return ErrValueTooLarge
	}
	return nil
}

// Put adds a new secret. It refuses a name already in use.
func (v *Vault) Put(name string, value []byte) error {
	if err := checkSecret(name, value); err != nil {
		return err
	}
	switch _, err := v.find(name); {
	case err == nil:
		return ErrNameExists
	case !errors.Is(err, ErrNotFound):
		return err
	}
	id := make([]byte, fileIDSize)
	if _, err := io.ReadFull(rand.Reader, id); err != nil {
		return err
	}
	file, err := v.encode(id, name, value)
	if err != nil {
		return err
	}
	return writeNew(v.secretPath(id), file)
}

// Update replaces the value of an existing secret, keeping its file name.
func (v *Vault) Update(name string, value []byte) error {
	if err := checkSecret(name, value); err != nil {
		return err
	}
	id, err := v.find(name)
	if err != nil {
		return err
	}
	file, err := v.encode(id, name, value)
	if err != nil {
		return err
	}
	return writeReplacing(v.secretPath(id), file)
}

// Get returns the value of a secret, decrypting only that file's value.
func (v *Vault) Get(name string) ([]byte, error) {
	id, err := v.find(name)
	if err != nil {
		return nil, err
	}
	file, err := readLimited(v.secretPath(id), maxSecretFile)
	if err != nil {
		return nil, err
	}
	return v.decodeValue(id, file)
}

// List returns every secret name in byte order, decrypting names only.
func (v *Vault) List() ([]string, error) {
	entries, err := v.scan()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	sort.Strings(names)
	return names, nil
}

// Delete removes the file of a secret.
func (v *Vault) Delete(name string) error {
	id, err := v.find(name)
	if err != nil {
		return err
	}
	return os.Remove(v.secretPath(id))
}

func readLimited(path string, max int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, int64(max)+1))
}

// writeTemp creates a synced, closed owner-only temporary file next to path.
// Its extension is .tmp so that scans ignore it.
func writeTemp(path string, data []byte) (string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(dir, ".write-*.tmp")
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(data)
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

// writeNew writes a whole file or none, never overwriting an existing one.
func writeNew(path string, data []byte) error {
	temp, err := writeTemp(path, data)
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

// writeReplacing atomically replaces an existing file with a whole new one.
func writeReplacing(path string, data []byte) error {
	temp, err := writeTemp(path, data)
	if err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(temp)
		return err
	}
	return nil
}
