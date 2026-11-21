package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/fabianramirez-code/qrypt/internal/identity"
)

type recordEntry struct {
	generation uint64
	hash       [32]byte
}

// readRecord rejects malformed or unreadable state instead of treating it as a new device.
func readRecord(path string) (map[string]recordEntry, error) {
	entries := make(map[string]recordEntry)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		return nil, fmt.Errorf("invalid vault record")
	}
	previous := ""
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line == "" {
			if len(data) == 0 {
				break
			}
			return nil, fmt.Errorf("invalid vault record")
		}
		fields := strings.Split(line, " ")
		if len(fields) != 3 || !validScopeID(fields[0]) || fields[0] <= previous || len(fields[2]) != 64 || strings.ToLower(fields[2]) != fields[2] {
			return nil, fmt.Errorf("invalid vault record")
		}
		n, e := strconv.ParseUint(fields[1], 10, 64)
		if e != nil || n == 0 || strconv.FormatUint(n, 10) != fields[1] {
			return nil, fmt.Errorf("invalid vault record")
		}
		h, e := hex.DecodeString(fields[2])
		if e != nil {
			return nil, fmt.Errorf("invalid vault record")
		}
		var hash [32]byte
		copy(hash[:], h)
		entries[fields[0]] = recordEntry{n, hash}
		previous = fields[0]
	}
	return entries, nil
}
func writeRecord(path string, entries map[string]recordEntry) error {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		entry := entries[k]
		fmt.Fprintf(&b, "%s %d %x\n", k, entry.generation, entry.hash)
	}
	return writeReplacing(path, []byte(b.String()))
}

// ErrRollback means a restore would bring back a state older than the one the
// local record has already seen.
var ErrRollback = errors.New("the copy is older than the state this device has already seen")

// CheckRestoredScopes refuses scopes whose generation is lower than the one the
// local record holds for the same scope. A deleted secret must not come back
// through an older copy, and the record must not be reset to hide it.
func CheckRestoredScopes(statePath string, scopes []*Vault) error {
	entries, err := readRecord(statePath)
	if err != nil {
		return err
	}
	for _, v := range scopes {
		if seen, ok := entries[v.ID()]; ok && v.Generation() < seen.generation {
			return ErrRollback
		}
	}
	return nil
}

// RecordRestoredScopes replaces the local record after verified staged scopes have
// been moved into place. It uses the same validation and atomic writer as normal changes.
func RecordRestoredScopes(statePath string, scopes []*Vault) error {
	if err := CheckRestoredScopes(statePath, scopes); err != nil {
		return err
	}
	entries := make(map[string]recordEntry, len(scopes))
	for _, v := range scopes {
		entries[v.ID()] = recordEntry{v.Generation(), v.ManifestHash()}
	}
	return writeRecord(statePath, entries)
}
func recordVault(v *Vault) error {
	entries, err := readRecord(v.statePath)
	if err != nil {
		return err
	}
	entries[v.ID()] = recordEntry{v.Generation(), v.ManifestHash()}
	return writeRecord(v.statePath, entries)
}

// diskUndecryptable counts files that accepting would remove, including damaged value segments.
func diskUndecryptable(v *Vault) (int, error) {
	count := 0
	if _, err := v.nameUnchecked(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			count++
		}
	}
	files, err := os.ReadDir(filepath.Join(v.dir, secretDir))
	if errors.Is(err, os.ErrNotExist) {
		return count, nil
	}
	if err != nil {
		return 0, err
	}
	for _, f := range files {
		fid, ok := parseFileName(f.Name())
		if !ok || f.IsDir() {
			continue
		}
		blob, e := os.ReadFile(filepath.Join(v.dir, secretDir, f.Name()))
		if e != nil {
			return 0, e
		}
		if _, e = v.decodeName(fid, blob); e != nil {
			count++
			continue
		}
		value, e := v.decodeValue(fid, blob)
		clear(value) // Only whether it decrypts matters here.
		if e != nil {
			count++
		}
	}
	return count, nil
}

// undecryptable reports whether err means the content is not readable with these keys.
func undecryptable(err error) bool {
	return errors.Is(err, ErrDecryption) || errors.Is(err, ErrInvalidFormat) || errors.Is(err, ErrUnsupportedVersion)
}

// keyProven reports whether at least one file of the scope decrypts with the
// scope key, or the scope holds no item at all. When nothing decrypts, the key
// itself is in doubt (a replaced scope file), so nothing may be removed.
func keyProven(v *Vault) (bool, error) {
	files, err := os.ReadDir(filepath.Join(v.dir, secretDir))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	items := 0
	for _, f := range files {
		fid, ok := parseFileName(f.Name())
		if !ok || f.IsDir() {
			continue
		}
		items++
		blob, err := os.ReadFile(filepath.Join(v.dir, secretDir, f.Name()))
		if err != nil {
			return false, err
		}
		if _, err = v.decodeName(fid, blob); err == nil {
			return true, nil
		}
	}
	if items > 0 {
		// A name that decrypts proves only the key that sealed it, and someone who
		// can write to the vault can replace the scope file and the name together.
		// The name counts only when it is the one the signed manifest recorded.
		return v.nameMatchesSignedManifest(), nil
	}
	// With no item there is nothing of value to lose; a name that cannot be read
	// is the only thing Accept can remove.
	if _, err := os.Stat(filepath.Join(v.dir, scopeNameFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return true, nil
}

func scopeProblem(v *Vault, kind ProblemKind, recorded uint64) (Problem, error) {
	if proven, err := keyProven(v); err != nil {
		return Problem{}, err
	} else if !proven {
		return Problem{ScopeID: v.ID(), Kind: ProblemUnreadable, Recorded: recorded}, nil
	}
	p := *v.problem(kind)
	p.Recorded = recorded
	if _, err := os.Stat(filepath.Join(v.dir, scopeNameFile)); errors.Is(err, os.ErrNotExist) {
		p.Name = ""
	} else if err != nil {
		return p, err
	}
	n, e := diskUndecryptable(v)
	if e != nil {
		return p, e
	}
	p.Undecryptable = n
	return p, nil
}

// inspectScope verifies the scope before comparing the signed state with the local record.
func inspectScope(dir, scopeID string, id *identity.Identity, rec recordEntry, hasRecord bool) (*Vault, *Problem, error) {
	manifestPath := filepath.Join(dir, "manifest")
	_, statErr := os.Stat(manifestPath)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, nil, statErr
	}
	// A recorded manifest must not be silently recreated by Open.
	if hasRecord && errors.Is(statErr, os.ErrNotExist) {
		v, err := openScope(dir, id, false)
		if err != nil {
			return nil, nil, err
		}
		defer v.Close()
		p, e := scopeProblem(v, ProblemSignature, rec.generation)
		return nil, &p, e
	}
	v, err := Open(dir, id)
	if err != nil {
		var problem *Problem
		if !errors.As(err, &problem) {
			return nil, nil, err
		}
		v, e := openScope(dir, id, false)
		if e != nil {
			return nil, nil, e
		}
		defer v.Close()
		p, e := scopeProblem(v, problem.Kind, rec.generation)
		if e != nil {
			return nil, nil, e
		}
		p.Generation = problem.Generation
		p.Missing = problem.Missing
		p.Unexpected = problem.Unexpected
		p.Changed = problem.Changed
		// A valid manifest may have been read before a files mismatch.
		return nil, &p, nil
	}
	if v.ID() != scopeID {
		v.Close()
		return nil, nil, ErrInvalidFormat
	}
	if hasRecord {
		kind := ProblemKind("")
		switch {
		case v.Generation() < rec.generation:
			kind = ProblemRollback
		case v.Generation() == rec.generation && v.ManifestHash() != rec.hash:
			kind = ProblemFork
		}
		if kind != "" {
			p, e := scopeProblem(v, kind, rec.generation)
			v.Close()
			return nil, &p, e
		}
	}
	// Files that match the manifest but cannot be decrypted mean the scope key changed.
	proven, e := keyProven(v)
	if e != nil {
		v.Close()
		return nil, nil, e
	}
	if _, nameErr := v.nameUnchecked(); !proven || nameErr != nil {
		p, x := scopeProblem(v, ProblemFiles, rec.generation)
		v.Close()
		if x != nil {
			return nil, nil, x
		}
		return nil, &p, nil
	}
	return v, nil, nil
}

// Accept keeps only decryptable files and signs a fresh current manifest.
func Accept(root, statePath string, id *identity.Identity, scopeID string) error {
	if !validScopeID(scopeID) {
		return ErrInvalidFormat
	}
	entries, err := readRecord(statePath)
	if err != nil {
		return err
	}
	rec, has := entries[scopeID]
	dir := filepath.Join(root, "scopes", scopeID)
	if _, err = os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		if has {
			delete(entries, scopeID)
			return writeRecord(statePath, entries)
		}
		return os.ErrNotExist
	}
	if err != nil {
		return err
	}
	v, problem, err := inspectScope(dir, scopeID, id, rec, has)
	if unreadableScope(err) {
		return ErrScopeUnreadable
	}
	if err != nil {
		return err
	}
	if v != nil {
		v.Close()
		return nil
	}
	if problem == nil {
		return nil
	}
	v, err = openScope(dir, id, false)
	if err != nil {
		return err
	}
	defer v.Close()
	if v.ID() != scopeID {
		return ErrInvalidFormat
	}
	// Never remove anything on the word of a key that decrypts nothing.
	if proven, err := keyProven(v); err != nil {
		return err
	} else if !proven {
		return ErrScopeUnreadable
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	old, verifyErr := VerifyManifest(id, raw)
	generation := rec.generation
	if verifyErr == nil && old.ScopeID == scopeID && old.Generation > generation {
		generation = old.Generation
	}
	if generation == ^uint64(0) {
		return fmt.Errorf("manifest generation overflow")
	}
	m := Manifest{ScopeID: scopeID, Generation: generation + 1, Files: make(map[string][32]byte)}
	namePath := filepath.Join(dir, scopeNameFile)
	if name, e := os.ReadFile(namePath); e == nil {
		if _, e = v.nameUnchecked(); e == nil {
			m.NameHash = sha256.Sum256(name)
		} else if !undecryptable(e) {
			return e // A read error is not a reason to remove the file.
		} else if e = os.Remove(namePath); e != nil {
			return e
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	files, e := os.ReadDir(filepath.Join(dir, secretDir))
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	for _, f := range files {
		fid, ok := parseFileName(f.Name())
		if !ok || f.IsDir() {
			continue
		}
		path := filepath.Join(dir, secretDir, f.Name())
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		_, ne := v.decodeName(fid, data)
		value, ve := v.decodeValue(fid, data)
		clear(value) // Only whether it decrypts matters here.
		if ne != nil || ve != nil {
			if e = os.Remove(path); e != nil {
				return e
			}
			continue
		}
		m.Files[hex.EncodeToString(fid)] = sha256.Sum256(data)
	}
	signed, err := v.signManifest(m)
	if err != nil {
		return err
	}
	if err = writeReplacing(filepath.Join(dir, "manifest.next"), signed); err != nil {
		return err
	}
	if err = os.Rename(filepath.Join(dir, "manifest.next"), filepath.Join(dir, "manifest")); err != nil {
		return err
	}
	m.Hash = sha256.Sum256(signed)
	v.manifest = m
	v.statePath = statePath
	return recordVault(v)
}

// nameMatchesSignedManifest reports whether the name file is byte for byte the
// one recorded by the scope's signed manifest and decrypts with the scope key.
// Only this identity can sign that manifest, so a forged name cannot match it.
func (v *Vault) nameMatchesSignedManifest() bool {
	raw, err := os.ReadFile(filepath.Join(v.dir, "manifest"))
	if err != nil {
		return false
	}
	m, err := verifyManifest(v.edPublic, v.mlPublic, raw)
	if err != nil || m.NameHash == ([32]byte{}) {
		return false
	}
	name, err := os.ReadFile(filepath.Join(v.dir, scopeNameFile))
	if err != nil || sha256.Sum256(name) != m.NameHash {
		return false
	}
	_, err = v.nameUnchecked()
	return err == nil
}
