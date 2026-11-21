// Package mirror copies current signed vault scopes to an untrusted whole-file storage.
package mirror

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

type Storage interface {
	List() ([]string, error)
	Read(name string) ([]byte, error)
	Write(name string, data []byte) error
	Remove(name string) error
}
type Report struct {
	Updated, Unchanged      int
	Ahead, Foreign, Unknown []string
	LocalProblems           int
}

var (
	ErrNotEmpty   = errors.New("vault holds a secret")
	ErrIncomplete = errors.New("mirror scope incomplete")
	ErrForeign    = errors.New("mirror scope foreign")
	ErrEmpty      = errors.New("mirror has no scope")
)

func scopeID(s string) bool {
	if len(s) != 32 || strings.ToLower(s) != s {
		return false
	}
	_, e := hex.DecodeString(s)
	return e == nil
}
func prefix(id string) string { return "scopes/" + id + "/" }
func parse(name string) (string, bool) {
	parts := strings.Split(name, "/")
	if len(parts) < 3 || parts[0] != "scopes" || !scopeID(parts[1]) {
		return "", false
	}
	return parts[1], true
}
func expected(id string, m vault.Manifest) map[string]bool {
	p := prefix(id)
	out := map[string]bool{p + "scope": true, p + "manifest": true}
	if m.NameHash != ([32]byte{}) {
		out[p+"name"] = true
	}
	for k := range m.Files {
		out[p+"secrets/"+k+".qrs"] = true
	}
	return out
}
func scoped(names []string) map[string][]string {
	out := map[string][]string{}
	for _, n := range names {
		if id, ok := parse(n); ok {
			out[id] = append(out[id], n)
		}
	}
	for _, v := range out {
		sort.Strings(v)
	}
	return out
}
func Push(root, statePath string, id *identity.Identity, to Storage) (Report, error) {
	var report Report
	opened, problems, e := vault.OpenScopes(root, statePath, id)
	if e != nil {
		return report, e
	}
	defer func() {
		for _, v := range opened {
			v.Close()
		}
	}()
	report.LocalProblems = len(problems)
	names, e := to.List()
	if e != nil {
		return report, e
	}
	remote := scoped(names)
	// A mirror that holds a scope signed by someone else belongs to another vault.
	// Nothing is written to it, not even the scopes it does not have yet.
	for sid := range remote {
		raw, err := to.Read(prefix(sid) + "manifest")
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return report, err
		}
		if m, err := vault.VerifyManifest(id, raw); err != nil || m.ScopeID != sid {
			report.Foreign = append(report.Foreign, sid)
		}
	}
	if len(report.Foreign) > 0 {
		sort.Strings(report.Foreign)
		return report, nil
	}
	local := map[string]bool{}
	for _, p := range problems {
		local[p.ScopeID] = true
	}
	for _, v := range opened {
		sid := v.ID()
		local[sid] = true
		p := prefix(sid)
		manifest, e := os.ReadFile(filepath.Join(root, "scopes", sid, "manifest"))
		if e != nil {
			return report, e
		}
		lm, e := vault.VerifyManifest(id, manifest)
		if e != nil {
			return report, e
		}
		old, e := to.Read(p + "manifest")
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return report, e
		}
		if e == nil {
			rm, err := vault.VerifyManifest(id, old)
			if err != nil || rm.ScopeID != sid {
				report.Foreign = append(report.Foreign, sid)
				continue
			}
			if rm.Generation > lm.Generation || (rm.Generation == lm.Generation && rm.Hash != lm.Hash) {
				report.Ahead = append(report.Ahead, sid)
				continue
			}
		}
		wanted := expected(sid, lm)
		changed := false
		paths := make([]string, 0, len(wanted))
		for n := range wanted {
			if n != p+"manifest" {
				paths = append(paths, n)
			}
		}
		sort.Strings(paths)
		for _, n := range paths {
			blob, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(n)))
			if err != nil {
				return report, err
			}
			existing, err := to.Read(n)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return report, err
			}
			if err == nil && sha256.Sum256(existing) == sha256.Sum256(blob) {
				continue
			}
			if err = to.Write(n, blob); err != nil {
				return report, err
			}
			changed = true
		}
		if e != nil || sha256.Sum256(old) != lm.Hash {
			if err := to.Write(p+"manifest", manifest); err != nil {
				return report, err
			}
			changed = true
		}
		for _, n := range remote[sid] {
			if !wanted[n] {
				if err := to.Remove(n); err != nil {
					return report, err
				}
				changed = true
			}
		}
		if changed {
			report.Updated++
		} else {
			report.Unchanged++
		}
	}
	for sid := range remote {
		if !local[sid] {
			report.Unknown = append(report.Unknown, sid)
		}
	}
	sort.Strings(report.Unknown)
	sort.Strings(report.Foreign)
	sort.Strings(report.Ahead)
	return report, nil
}
func RemoveScope(to Storage, scopeID string) error {
	if !scopeIDValid(scopeID) {
		return fmt.Errorf("invalid scope id")
	}
	names, e := to.List()
	if e != nil {
		return e
	}
	for _, n := range names {
		if strings.HasPrefix(n, prefix(scopeID)) {
			if e = to.Remove(n); e != nil {
				return e
			}
		}
	}
	return nil
}
func scopeIDValid(s string) bool { return scopeID(s) }

// Restore validates all mirror scopes in a private staging directory before replacing empty scopes.
func Restore(from Storage, root, statePath string, id *identity.Identity) (ret error) {
	names, e := from.List()
	if e != nil {
		return e
	}
	groups := scoped(names)
	if len(groups) == 0 {
		return ErrEmpty
	}
	ids := make([]string, 0, len(groups))
	for sid := range groups {
		ids = append(ids, sid)
	}
	sort.Strings(ids)
	// Check the device without allowing an open to rewrite its local record.
	current := []string{}
	dirs, e := os.ReadDir(filepath.Join(root, "scopes"))
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	for _, d := range dirs {
		if !d.IsDir() || !scopeID(d.Name()) {
			continue
		}
		v, err := vault.Open(filepath.Join(root, "scopes", d.Name()), id)
		if err != nil {
			return fmt.Errorf("device scope %s: %w", d.Name(), ErrNotEmpty)
		}
		items, err := v.Entries()
		v.Close()
		if err != nil || len(items) > 0 {
			return ErrNotEmpty
		}
		current = append(current, d.Name())
	}
	_, rootErr := os.Stat(root)
	if rootErr != nil && !errors.Is(rootErr, os.ErrNotExist) {
		return rootErr
	}
	createdRoot := errors.Is(rootErr, os.ErrNotExist)
	if e = os.MkdirAll(root, 0700); e != nil {
		return e
	}
	defer func() {
		if ret != nil && createdRoot {
			os.Remove(filepath.Join(root, "scopes"))
			os.Remove(root)
		}
	}()
	stage, e := os.MkdirTemp(root, ".restore-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	staged := []*vault.Vault{}
	defer func() {
		for _, v := range staged {
			v.Close()
		}
	}()
	for _, sid := range ids {
		p := prefix(sid)
		raw, err := from.Read(p + "manifest")
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("%s: %w", sid, ErrIncomplete)
			}
			return err
		}
		m, err := vault.VerifyManifest(id, raw)
		if err != nil || m.ScopeID != sid {
			return fmt.Errorf("%s: %w", sid, ErrForeign)
		}
		want := expected(sid, m)
		for _, n := range groups[sid] {
			if !want[n] {
				return fmt.Errorf("%s: %w", sid, ErrIncomplete)
			}
		}
		for n := range want {
			if n == p+"manifest" {
				continue
			}
			blob, err := from.Read(n)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("%s: %w", sid, ErrIncomplete)
				}
				return err
			}
			switch {
			case n == p+"name":
				if sha256.Sum256(blob) != m.NameHash {
					return fmt.Errorf("%s: %w", sid, ErrIncomplete)
				}
			case strings.HasSuffix(n, ".qrs"):
				key := strings.TrimSuffix(strings.TrimPrefix(n, p+"secrets/"), ".qrs")
				if sha256.Sum256(blob) != m.Files[key] {
					return fmt.Errorf("%s: %w", sid, ErrIncomplete)
				}
			}
			dest := filepath.Join(stage, filepath.FromSlash(n))
			if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
				return err
			}
			if err = os.WriteFile(dest, blob, 0600); err != nil {
				return err
			}
		}
		dest := filepath.Join(stage, "scopes", sid, "manifest")
		if err = os.WriteFile(dest, raw, 0600); err != nil {
			return err
		}
		v, err := vault.Open(filepath.Dir(dest), id)
		if err != nil {
			return fmt.Errorf("%s: %w", sid, ErrForeign)
		}
		if v.ID() != sid {
			v.Close()
			return fmt.Errorf("%s: %w", sid, ErrForeign)
		}
		// Listing decrypts the names, which proves the scope key belongs to this
		// identity. Values are not decrypted: their bytes already match the signed manifest.
		if _, err = v.Entries(); err != nil {
			v.Close()
			return fmt.Errorf("%s: %w", sid, ErrForeign)
		}
		staged = append(staged, v)
	}
	if e = vault.CheckRestoredScopes(statePath, staged); e != nil {
		return e
	}
	destRoot := filepath.Join(root, "scopes")
	if e = os.MkdirAll(destRoot, 0700); e != nil {
		return e
	}
	backup := filepath.Join(stage, "backup")
	if e = os.Mkdir(backup, 0700); e != nil {
		return e
	}
	movedOld := []string{}
	movedNew := []string{}
	defer func() {
		if ret != nil {
			for _, sid := range movedNew {
				os.Rename(filepath.Join(destRoot, sid), filepath.Join(stage, "scopes", sid))
			}
			for _, sid := range movedOld {
				os.Rename(filepath.Join(backup, sid), filepath.Join(destRoot, sid))
			}
		}
	}()
	for _, sid := range current {
		if e = os.Rename(filepath.Join(destRoot, sid), filepath.Join(backup, sid)); e != nil {
			return e
		}
		movedOld = append(movedOld, sid)
	}
	for _, sid := range ids {
		if e = os.Rename(filepath.Join(stage, "scopes", sid), filepath.Join(destRoot, sid)); e != nil {
			return e
		}
		movedNew = append(movedNew, sid)
	}
	// Keep the previous record so that a failure after this point restores it too.
	oldRecord, recordErr := os.ReadFile(statePath)
	if recordErr != nil && !errors.Is(recordErr, os.ErrNotExist) {
		return recordErr
	}
	defer func() {
		if ret == nil {
			return
		}
		if recordErr != nil {
			os.Remove(statePath)
		} else {
			os.WriteFile(statePath, oldRecord, 0600)
		}
	}()
	if e = vault.RecordRestoredScopes(statePath, staged); e != nil {
		return e
	}
	opened, problems, e := vault.OpenScopes(root, statePath, id)
	for _, v := range opened {
		v.Close()
	}
	if e != nil {
		return e
	}
	if len(problems) > 0 {
		return ErrIncomplete
	}
	return nil
}
