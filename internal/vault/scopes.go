package vault

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
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
	if err := v.checkDisk(); err != nil {
		return "", err
	}
	return v.nameUnchecked()
}

func (v *Vault) nameUnchecked() (string, error) {
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
	if err := v.ready(); err != nil {
		return err
	}
	m := v.nextManifest()
	m.NameHash = sha256.Sum256(file)
	return v.change(m, func() error { return writeReplacing(filepath.Join(v.dir, scopeNameFile), file) })
}

// moveEarlierLayout moves the original root-level scope and secrets directory,
// completing either order of an interrupted two-rename move.
func moveEarlierLayout(root string, id *identity.Identity) error {
	oldScope := filepath.Join(root, scopeName)
	_, err := os.Stat(oldScope)
	if err == nil {
		raw, err := readLimited(oldScope, scopeFileSize)
		if err != nil {
			return err
		}
		if len(raw) != scopeFileSize || string(raw[:7]) != scopeMagic {
			return ErrInvalidFormat
		}
		dest := filepath.Join(root, "scopes", hex.EncodeToString(raw[8:24]))
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
		for _, entry := range []string{scopeNameFile, "manifest", "manifest.next"} {
			from := filepath.Join(root, entry)
			if _, e := os.Stat(from); e == nil {
				if e = os.Rename(from, filepath.Join(dest, entry)); e != nil {
					return e
				}
			} else if !errors.Is(e, os.ErrNotExist) {
				return e
			}
		}
		oldSecrets := filepath.Join(root, secretDir)
		if _, e := os.Stat(oldSecrets); e == nil {
			return os.Rename(oldSecrets, filepath.Join(dest, secretDir))
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
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
			raw, e := readLimited(filepath.Join(path, scopeName), scopeFileSize)
			if e != nil {
				return e
			}
			if len(raw) != scopeFileSize || hex.EncodeToString(raw[8:24]) != d.Name() {
				return ErrInvalidFormat
			}
			dest = path
		} else if err != nil {
			return err
		}
	}
	if dest == "" {
		return ErrInvalidFormat
	}
	if err := os.Rename(oldSecrets, filepath.Join(dest, secretDir)); err != nil {
		return err
	}
	for _, entry := range []string{scopeNameFile, "manifest", "manifest.next"} {
		from := filepath.Join(root, entry)
		if _, e := os.Stat(from); e == nil {
			if e = os.Rename(from, filepath.Join(dest, entry)); e != nil {
				return e
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
		}
	}
	return nil
}

func validScopeID(s string) bool {
	if len(s) != scopeIDSize*2 || s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// OpenScopes opens all scopes, completing any earlier-layout migration first.
func OpenScopes(root, statePath string, id *identity.Identity) ([]*Vault, []Problem, error) {
	records, err := readRecord(statePath)
	if err != nil {
		return nil, nil, err
	}
	if _, err = os.Stat(root); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	if err == nil {
		if err = moveEarlierLayout(root, id); err != nil {
			return nil, nil, err
		}
	}
	dirs, err := os.ReadDir(filepath.Join(root, "scopes"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	result := []*Vault{}
	problems := []Problem{}
	seen := map[string]bool{}
	updated := false
	fail := func(e error) ([]*Vault, []Problem, error) {
		for _, v := range result {
			v.Close()
		}
		return nil, nil, e
	}
	for _, d := range dirs {
		if !d.IsDir() || !validScopeID(d.Name()) {
			continue
		}
		scopeID := d.Name()
		seen[scopeID] = true
		rec, has := records[scopeID]
		v, p, e := inspectScope(filepath.Join(root, "scopes", scopeID), scopeID, id, rec, has)
		if unreadableScope(e) {
			// One scope that cannot be opened must not take the others with it.
			problems = append(problems, Problem{ScopeID: scopeID, Kind: ProblemUnreadable, Recorded: rec.generation})
			continue
		}
		if e != nil {
			return fail(e)
		}
		if p != nil {
			problems = append(problems, *p)
			continue
		}
		v.statePath = statePath
		result = append(result, v)
		if !has || v.Generation() > rec.generation {
			records[scopeID] = recordEntry{v.Generation(), v.ManifestHash()}
			updated = true
		}
	}
	for scopeID, rec := range records {
		if !seen[scopeID] {
			problems = append(problems, Problem{ScopeID: scopeID, Kind: ProblemRemoved, Recorded: rec.generation})
		}
	}
	if updated {
		if err = writeRecord(statePath, records); err != nil {
			return fail(err)
		}
	}
	names := make(map[*Vault]string, len(result))
	for _, v := range result {
		names[v], _ = v.Name()
	}
	sort.Slice(result, func(i, j int) bool { return names[result[i]] < names[result[j]] })
	sort.Slice(problems, func(i, j int) bool { return problems[i].ScopeID < problems[j].ScopeID })
	return result, problems, nil
}

// unreadableScope reports whether a scope failed to open because of its own scope file.
func unreadableScope(err error) bool {
	return errors.Is(err, ErrDecryption) || errors.Is(err, ErrInvalidFormat) ||
		errors.Is(err, ErrUnsupportedVersion) || errors.Is(err, os.ErrNotExist)
}

// NewScope creates a named scope, refusing an existing name.
func NewScope(root, statePath string, id *identity.Identity, name string) (*Vault, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	opened, problems, err := OpenScopes(root, statePath, id)
	if err != nil {
		return nil, err
	}
	if len(problems) != 0 {
		for _, v := range opened {
			v.Close()
		}
		return nil, &problems[0]
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
	v.statePath = statePath
	if err = recordVault(v); err != nil {
		v.Close()
		return nil, err
	}
	return v, nil
}

// RenameScope changes a scope's name if no other scope has it.
func RenameScope(root, statePath string, id *identity.Identity, v *Vault, name string) error {
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
	opened, problems, err := OpenScopes(root, statePath, id)
	if err != nil {
		return err
	}
	if len(problems) != 0 {
		for _, other := range opened {
			other.Close()
		}
		return &problems[0]
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
	if err := v.checkDisk(); err != nil {
		return err
	}
	// One rename takes the scope out of the vault; a crash while its files are
	// being removed then leaves only an ignored directory, never half a scope.
	removed := filepath.Join(filepath.Dir(v.dir), ".scope-removed-"+v.ID())
	if err := os.Rename(v.dir, removed); err != nil {
		return err
	}
	v.Close()
	if err := os.RemoveAll(removed); err != nil {
		return err
	}
	if v.statePath != "" {
		entries, err := readRecord(v.statePath)
		if err != nil {
			return err
		}
		delete(entries, v.ID())
		return writeRecord(v.statePath, entries)
	}
	return nil
}
