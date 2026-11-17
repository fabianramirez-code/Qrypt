package vault

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fabianramirez-code/qrypt/internal/identity"
)

var (
	// ErrScopeNameExists means another scope already has the requested name.
	ErrScopeNameExists = errors.New("a scope with that name already exists")
	// ErrScopeNotEmpty means a scope still holds secrets.
	ErrScopeNotEmpty = errors.New("scope is not empty")
)

// ID returns the opaque hexadecimal scope identifier.
func (v *Vault) ID() string { return hex.EncodeToString(v.scopeID) }

func (v *Vault) scopeNameAAD() []byte {
	aad := make([]byte, 0, headerSize+scopeIDSize)
	aad = append(aad, scopeNameMagic...)
	aad = append(aad, formatVersion)
	return append(aad, v.scopeID...)
}

// Name returns the scope's decrypted name, or Personal for an earlier unnamed scope.
func (v *Vault) Name() (string, error) {
	if v.scopeNameKey == nil {
		return "", ErrClosed
	}
	file, err := readLimited(filepath.Join(v.dir, scopeNameFile), valueNonceOffset)
	if errors.Is(err, os.ErrNotExist) {
		return "Personal", nil
	}
	if err != nil {
		return "", err
	}
	if len(file) != valueNonceOffset || !bytes.Equal(file[:magicSize], []byte(scopeNameMagic)) {
		return "", ErrInvalidFormat
	}
	if file[magicSize] != formatVersion {
		return "", ErrUnsupportedVersion
	}
	plain, err := open(v.scopeNameKey, file[nameNonceOffset:nameSegmentOffset], file[nameSegmentOffset:], v.scopeNameAAD())
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

// SetName atomically replaces the encrypted name with a fresh nonce.
func (v *Vault) SetName(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	if v.scopeNameKey == nil {
		return ErrClosed
	}
	plain := make([]byte, namePadded)
	defer clear(plain)
	plain[0] = byte(len(name))
	copy(plain[1:], name)
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	sealed, err := seal(v.scopeNameKey, nonce, plain, v.scopeNameAAD())
	if err != nil {
		return err
	}
	file := make([]byte, 0, valueNonceOffset)
	file = append(file, scopeNameMagic...)
	file = append(file, formatVersion)
	file = append(file, nonce...)
	file = append(file, sealed...)
	return writeReplacing(filepath.Join(v.dir, scopeNameFile), file)
}

// moveEarlierLayout moves the original root-level scope and secrets directory,
// completing either order of an interrupted two-rename move.
func moveEarlierLayout(root string, id *identity.Identity) error {
	oldScope := filepath.Join(root, scopeName)
	_, err := os.Stat(oldScope)
	if err == nil {
		v, err := Open(root, id)
		if err != nil {
			return err
		}
		defer v.Close()
		dest := filepath.Join(root, "scopes", v.ID())
		if err := os.MkdirAll(dest, 0700); err != nil {
			return err
		}
		target := filepath.Join(dest, scopeName)
		if _, err := os.Stat(target); err == nil {
			return ErrAlreadyExists
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Rename(oldScope, target); err != nil {
			return err
		}
		oldSecrets := filepath.Join(root, secretDir)
		if _, err := os.Stat(oldSecrets); err == nil {
			return os.Rename(oldSecrets, filepath.Join(dest, secretDir))
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	oldSecrets := filepath.Join(root, secretDir)
	if _, err := os.Stat(oldSecrets); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	// An empty directory is what a vault whose creation failed leaves behind: it
	// holds nothing to move. Remove succeeds only on an empty directory.
	if os.Remove(oldSecrets) == nil {
		return nil
	}
	dirs, err := os.ReadDir(filepath.Join(root, "scopes"))
	if errors.Is(err, os.ErrNotExist) {
		return ErrInvalidFormat
	}
	if err != nil {
		return err
	}
	// A scope whose secrets directory is missing is the only possible destination.
	var dest string
	for _, d := range dirs {
		if !d.IsDir() || !validScopeID(d.Name()) {
			continue
		}
		path := filepath.Join(root, "scopes", d.Name())
		if _, err := os.Stat(filepath.Join(path, secretDir)); errors.Is(err, os.ErrNotExist) {
			if dest != "" {
				return ErrInvalidFormat
			}
			v, e := Open(path, id)
			if e != nil {
				return e
			}
			if v.ID() != d.Name() {
				v.Close()
				return ErrInvalidFormat
			}
			v.Close()
			dest = path
		} else if err != nil {
			return err
		}
	}
	if dest == "" {
		return ErrInvalidFormat
	}
	return os.Rename(oldSecrets, filepath.Join(dest, secretDir))
}

func validScopeID(s string) bool {
	if len(s) != scopeIDSize*2 || s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// OpenScopes opens all scopes, completing any earlier-layout migration first.
func OpenScopes(root string, id *identity.Identity) ([]*Vault, error) {
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return []*Vault{}, nil
	} else if err != nil {
		return nil, err
	}
	if err := moveEarlierLayout(root, id); err != nil {
		return nil, err
	}
	dirs, err := os.ReadDir(filepath.Join(root, "scopes"))
	if errors.Is(err, os.ErrNotExist) {
		return []*Vault{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]*Vault, 0, len(dirs))
	fail := func(err error) ([]*Vault, error) {
		for _, v := range result {
			v.Close()
		}
		return nil, err
	}
	for _, d := range dirs {
		// Anything that is not a scope directory is ignored: a scope being created or
		// removed (".scope-" prefix), or a file the system added, such as .DS_Store.
		if !d.IsDir() || !validScopeID(d.Name()) {
			continue
		}
		v, err := Open(filepath.Join(root, "scopes", d.Name()), id)
		if err != nil {
			return fail(err)
		}
		if v.ID() != d.Name() {
			v.Close()
			return fail(ErrInvalidFormat)
		}
		if _, err := v.Name(); err != nil {
			v.Close()
			return fail(err)
		}
		result = append(result, v)
	}
	names := make(map[*Vault]string, len(result))
	for _, v := range result {
		n, _ := v.Name()
		names[v] = n
	}
	sort.Slice(result, func(i, j int) bool { return names[result[i]] < names[result[j]] })
	return result, nil
}

// NewScope creates a named scope, refusing an existing name.
func NewScope(root string, id *identity.Identity, name string) (*Vault, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	opened, err := OpenScopes(root, id)
	if err != nil {
		return nil, err
	}
	exists := false
	for _, v := range opened {
		n, _ := v.Name()
		if n == name {
			exists = true
		}
		v.Close()
	}
	if exists {
		return nil, ErrScopeNameExists
	}
	dirRoot := filepath.Join(root, "scopes")
	if err := os.MkdirAll(dirRoot, 0700); err != nil {
		return nil, err
	}
	// Create owns key generation, wrapping, and non-overwriting of the scope file.
	pending, err := os.MkdirTemp(dirRoot, ".scope-*")
	if err != nil {
		return nil, err
	}
	// A scope that was not completed leaves nothing behind.
	created := false
	defer func() {
		if !created {
			os.RemoveAll(pending)
		}
	}()
	v, err := Create(pending, id)
	if err != nil {
		return nil, err
	}
	if err := v.SetName(name); err != nil {
		v.Close()
		return nil, err
	}
	dest := filepath.Join(dirRoot, v.ID())
	if _, err := os.Stat(dest); err == nil {
		v.Close()
		return nil, ErrAlreadyExists
	} else if !errors.Is(err, os.ErrNotExist) {
		v.Close()
		return nil, err
	}
	if err := os.Rename(pending, dest); err != nil {
		v.Close()
		return nil, err
	}
	created = true
	v.dir = dest
	return v, nil
}

// RenameScope changes a scope's name if no other scope has it.
func RenameScope(root string, id *identity.Identity, v *Vault, name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	current, err := v.Name()
	if err != nil {
		return err
	}
	if current == name {
		return nil
	}
	opened, err := OpenScopes(root, id)
	if err != nil {
		return err
	}
	defer func() {
		for _, other := range opened {
			other.Close()
		}
	}()
	for _, other := range opened {
		n, _ := other.Name()
		if other.ID() != v.ID() && n == name {
			return ErrScopeNameExists
		}
	}
	return v.SetName(name)
}

// RemoveScope removes an empty scope and closes it.
func RemoveScope(v *Vault) error {
	if v.scopeNameKey == nil {
		return ErrClosed
	}
	// Only secret files count: a leftover temporary file is not a secret, and a
	// scope whose directory of secrets is missing holds none.
	entries, err := os.ReadDir(filepath.Join(v.dir, secretDir))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	secrets := 0
	for _, entry := range entries {
		if _, ok := parseFileName(entry.Name()); ok {
			secrets++
		}
	}
	if secrets != 0 {
		return ErrScopeNotEmpty
	}
	// One rename takes the scope out of the vault; a crash while its files are
	// being removed then leaves only an ignored directory, never half a scope.
	removed := filepath.Join(filepath.Dir(v.dir), ".scope-removed-"+v.ID())
	if err := os.Rename(v.dir, removed); err != nil {
		return err
	}
	v.Close()
	return os.RemoveAll(removed)
}
