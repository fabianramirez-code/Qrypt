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

	"crypto/ed25519"
	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/secmem"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	keySize = 32

	scopeMagic     = "QRYPTSK"
	scopeNameMagic = "QRYPTSN"
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

	secretExt     = ".qrs"
	scopeName     = "scope"
	scopeNameFile = "name"
	secretDir     = "secrets"

	wrapLabel      = "qrypt/v1/vault/scope-key"
	nameLabel      = "qrypt/v1/vault/secret/name"
	valueLabel     = "qrypt/v1/vault/secret/value"
	scopeNameLabel = "qrypt/v1/vault/scope/name"

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
	dir          string
	keys         *secmem.Buffer
	scopeKey     []byte
	scopeID      []byte
	nameKey      []byte
	valueKey     []byte
	scopeNameKey []byte
	edSigning    ed25519.PrivateKey
	mlSeed       []byte
	edPublic     []byte
	mlPublic     []byte
	manifest     Manifest
	statePath    string
}

func derive(scopeKey []byte, label string) []byte {
	key, err := hkdf.Key(sha256.New, scopeKey, nil, label, keySize)
	if err != nil {
		panic("fixed HKDF length rejected")
	}
	return key
}

func newVault(dir string, scopeID, scopeKey []byte, id *identity.Identity) *Vault {
	ed := id.Ed25519PrivateKey()
	ml := id.MLDSA65PrivateKey()
	public := id.PublicKeys()
	keys := secmem.New(keySize*5 + ed25519.PrivateKeySize)
	packed := keys.Bytes()
	v := &Vault{
		dir: dir, keys: keys, scopeID: scopeID,
		scopeKey:     packed[:keySize],
		nameKey:      packed[keySize : 2*keySize],
		valueKey:     packed[2*keySize : 3*keySize],
		scopeNameKey: packed[3*keySize : 4*keySize],
		edSigning:    packed[4*keySize : 4*keySize+ed25519.PrivateKeySize],
		mlSeed:       packed[4*keySize+ed25519.PrivateKeySize:],
		edPublic:     public.Ed25519, mlPublic: public.MLDSA65,
	}
	copy(v.scopeKey, scopeKey)
	name := derive(v.scopeKey, nameLabel)
	copy(v.nameKey, name)
	clear(name)
	value := derive(v.scopeKey, valueLabel)
	copy(v.valueKey, value)
	clear(value)
	scopeName := derive(v.scopeKey, scopeNameLabel)
	copy(v.scopeNameKey, scopeName)
	clear(scopeName)
	copy(v.edSigning, ed)
	clear(ed)
	mlSeed := ml.Seed()
	copy(v.mlSeed, mlSeed)
	clear(mlSeed)
	*ml = mldsa65.PrivateKey{}
	return v
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
	if id == nil || id.KEMPrivateKey() == nil {
		return nil, identity.ErrClosed
	}
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
	v := newVault(dir, scopeID, scopeKey, id)
	if err := v.openManifest(); err != nil {
		v.Close()
		return nil, err
	}
	return v, nil
}

// Open reads the scope file in dir and unwraps the scope key with the identity.
func Open(dir string, id *identity.Identity) (*Vault, error) {
	return openScope(dir, id, true)
}

func openScope(dir string, id *identity.Identity, verify bool) (*Vault, error) {
	if id == nil || id.KEMPrivateKey() == nil {
		return nil, identity.ErrClosed
	}
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
	v := newVault(dir, scopeID, scopeKey, id)
	if verify {
		if err := v.openManifest(); err != nil {
			v.Close()
			return nil, err
		}
	}
	return v, nil
}

// Close erases the keys held in memory. The vault cannot be used afterward.
func (v *Vault) Close() {
	if v == nil {
		return
	}
	v.keys.Destroy()
	v.keys = nil
	v.scopeKey, v.edSigning, v.mlSeed = nil, nil, nil
	v.nameKey, v.valueKey, v.scopeNameKey = nil, nil, nil
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
	if len(file) < headerSize || !bytes.Equal(file[:magicSize], []byte(secretMagic)) {
		return ErrInvalidFormat
	}
	version := file[magicSize]
	if version != formatVersion && version != itemVersion {
		return ErrUnsupportedVersion
	}
	offset, sizeOffset := valueOffset, valueSizeOffset
	if version == itemVersion {
		offset, sizeOffset = bodyOffset, bodyNonceEnd
	}
	if len(file) < offset+minValueSealed || len(file) > offset+maxValueSealed {
		return ErrInvalidFormat
	}
	size := binary.BigEndian.Uint32(file[sizeOffset:offset])
	if int(size) != len(file)-offset || size < minValueSealed || (size-tagSize)%valueBlock != 0 {
		return ErrInvalidFormat
	}
	return nil
}

func (v *Vault) decodeName(fileID, file []byte) (string, error) {
	if err := checkSecretFile(file); err != nil {
		return "", err
	}
	if file[magicSize] == itemVersion {
		e, err := v.itemHead(fileID, file)
		return e.Title, err
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
	if file[magicSize] == itemVersion {
		plain, err := v.itemBody(fileID, file)
		if err != nil {
			return nil, err
		}
		defer clear(plain)
		fields, err := decodeFields(plain)
		if err != nil {
			return nil, err
		}
		var result []byte
		if len(fields) > 0 {
			result = fields[0].Value
		}
		for i := 1; i < len(fields); i++ {
			clear(fields[i].Value)
		}
		return result, nil
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
	if err := v.checkDisk(); err != nil {
		return nil, err
	}
	// A scope with no secret may have no directory for them, as one copied
	// from a mirror has: the manifest check above already vouches for it.
	files, err := os.ReadDir(filepath.Join(v.dir, secretDir))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var entries []entry
	for _, f := range files {
		id, ok := parseFileName(f.Name())
		if !ok || f.IsDir() {
			continue
		}
		file, err := readLimited(v.secretPath(id), maxItemFile)
		if err != nil {
			return nil, err
		}
		if err := v.verifyFile(id, file); err != nil {
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

// Put adds a root item with one value field.
func (v *Vault) Put(name string, value []byte) error {
	if err := checkSecret(name, value); err != nil {
		return err
	}
	kind := FieldHidden
	if bytes.ContainsAny(value, "\r\n") {
		kind = FieldNote
	}
	_, err := v.Add(Item{Title: name, Fields: []Field{{Kind: kind, Label: "value", Value: value}}})
	return err
}

// Update replaces the value of an existing root item, keeping its id.
func (v *Vault) Update(name string, value []byte) error {
	if err := checkSecret(name, value); err != nil {
		return err
	}
	id, err := v.rootID(name)
	if err != nil {
		return err
	}
	kind := FieldHidden
	if bytes.ContainsAny(value, "\r\n") {
		kind = FieldNote
	}
	return v.Replace(id, Item{Title: name, Fields: []Field{{Kind: kind, Label: "value", Value: value}}})
}

// Get returns the value field of a root item.
func (v *Vault) Get(name string) ([]byte, error) {
	id, err := v.rootID(name)
	if err != nil {
		return nil, err
	}
	item, err := v.Read(id)
	if err != nil {
		return nil, err
	}
	var value []byte
	for _, field := range item.Fields {
		if field.Label == "value" {
			value = field.Value
		} else {
			clear(field.Value)
		}
	}
	return value, nil
}
func (v *Vault) rootID(name string) (string, error) {
	entries, err := v.Entries()
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if len(e.Path) == 0 && e.Title == name {
			return e.ID, nil
		}
	}
	return "", ErrNotFound
}

// List returns root item titles in byte order.
func (v *Vault) List() ([]string, error) {
	entries, err := v.Entries()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if len(e.Path) == 0 {
			names = append(names, e.Title)
		}
	}
	sort.Strings(names)
	return names, nil
}

// Delete removes a root item.
func (v *Vault) Delete(name string) error {
	id, err := v.rootID(name)
	if err != nil {
		return err
	}
	return v.Remove(id)
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
