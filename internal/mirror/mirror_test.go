package mirror_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/mirror"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

type memory struct {
	files        map[string][]byte
	writes, fail int
}

func (m *memory) List() ([]string, error) {
	out := []string{}
	for k := range m.files {
		out = append(out, k)
	}
	return out, nil
}
func (m *memory) Read(k string) ([]byte, error) {
	b, ok := m.files[k]
	if !ok {
		return nil, os.ErrNotExist
	}
	return bytes.Clone(b), nil
}
func (m *memory) Write(k string, b []byte) error {
	m.writes++
	if m.fail > 0 && m.writes == m.fail {
		return errors.New("injected write")
	}
	m.files[k] = bytes.Clone(b)
	return nil
}
func (m *memory) Remove(k string) error { delete(m.files, k); return nil }
func newMemory() *memory                { return &memory{files: map[string][]byte{}} }
func device(t *testing.T) (string, string, *identity.Identity) {
	t.Helper()
	base := t.TempDir()
	id, e := identity.Generate()
	if e != nil {
		t.Fatal(e)
	}
	return filepath.Join(base, "vault"), filepath.Join(base, "state"), id
}
func scope(t *testing.T, root, state string, id *identity.Identity, n string) *vault.Vault {
	t.Helper()
	v, e := vault.NewScope(root, state, id, n)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(v.Close)
	return v
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}

// Root-value conveniences for mirror tests; all writes use the item API.
func addValue(v *vault.Vault, name string, value []byte) error {
	if err := vault.ValidateName(name); err != nil {
		return err
	}
	if len(value) > vault.MaxValueSize {
		return vault.ErrValueTooLarge
	}
	kind := vault.FieldHidden
	if bytes.ContainsAny(value, "\r\n") {
		kind = vault.FieldNote
	}
	_, err := v.Add(vault.Item{Title: name, Fields: []vault.Field{{Kind: kind, Label: "value", Value: value}}})
	return err
}
func entryID(v *vault.Vault, name string) (string, error) {
	entries, err := v.Entries()
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if len(entry.Path) == 0 && entry.Title == name {
			return entry.ID, nil
		}
	}
	return "", vault.ErrNotFound
}
func changeValue(v *vault.Vault, name string, value []byte) error {
	if err := vault.ValidateName(name); err != nil {
		return err
	}
	if len(value) > vault.MaxValueSize {
		return vault.ErrValueTooLarge
	}
	id, err := entryID(v, name)
	if err != nil {
		return err
	}
	kind := vault.FieldHidden
	if bytes.ContainsAny(value, "\r\n") {
		kind = vault.FieldNote
	}
	return v.Replace(id, vault.Item{Title: name, Fields: []vault.Field{{Kind: kind, Label: "value", Value: value}}})
}
func dropValue(v *vault.Vault, name string) error {
	id, err := entryID(v, name)
	if err != nil {
		return err
	}
	return v.Remove(id)
}
func TestDir(t *testing.T) {
	base := filepath.Join(t.TempDir(), "mirror")
	s := mirror.Dir(base)
	must(t, s.Write("scopes/id/name", []byte("first")))
	b, e := s.Read("scopes/id/name")
	must(t, e)
	if string(b) != "first" {
		t.Fatal(string(b))
	}
	names, e := s.List()
	must(t, e)
	if !reflect.DeepEqual(names, []string{"scopes/id/name"}) {
		t.Fatal(names)
	}
	for _, n := range []string{"", "/absolute", "../outside", "x/../y", "x//y", "x\\y", "x/"} {
		if s.Write(n, nil) == nil {
			t.Fatalf("accepted %q", n)
		}
		if _, e := s.Read(n); e == nil {
			t.Fatalf("read %q", n)
		}
		if s.Remove(n) == nil {
			t.Fatalf("remove %q", n)
		}
	}
	must(t, s.Remove("scopes/id/name"))
	must(t, s.Remove("scopes/id/name"))
	if _, e := os.Stat(filepath.Join(base, "scopes")); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("empty dirs: %v", e)
	}
}
func TestDirAtomicAndOwnerOnly(t *testing.T) {
	base := filepath.Join(t.TempDir(), "mirror")
	s := mirror.Dir(base)
	name := "nested/file"
	must(t, s.Write(name, []byte("old")))
	if e := s.Write("nested/../file", []byte("bad")); e == nil {
		t.Fatal("accepted invalid path")
	}
	b, e := s.Read(name)
	must(t, e)
	if string(b) != "old" {
		t.Fatal("old content changed")
	}
	must(t, s.Write(name, []byte("new")))
	names, e := s.List()
	must(t, e)
	if !reflect.DeepEqual(names, []string{name}) {
		t.Fatalf("temporary file left: %v", names)
	}
	if runtime.GOOS != "windows" {
		for _, p := range []string{base, filepath.Join(base, "nested"), filepath.Join(base, "nested", "file")} {
			info, e := os.Stat(p)
			must(t, e)
			want := os.FileMode(0700)
			if !info.IsDir() {
				want = 0600
			}
			if info.Mode().Perm() != want {
				t.Fatalf("%s mode %o want %o", p, info.Mode().Perm(), want)
			}
		}
	}
}
func TestPushRestore(t *testing.T) {
	root, state, id := device(t)
	a := scope(t, root, state, id, "Work")
	must(t, addValue(a, "one", []byte("alpha")))
	b := scope(t, root, state, id, "Home")
	must(t, addValue(b, "two", []byte("beta")))
	m := newMemory()
	r, e := mirror.Push(root, state, id, m)
	must(t, e)
	if r.Updated != 2 {
		t.Fatalf("report: %+v", r)
	}
	if len(m.files) == 0 {
		t.Fatal("no files")
	}
	for k := range m.files {
		if strings.Contains(k, "state") || strings.Contains(k, "identity") || strings.Contains(k, "manifest.next") {
			t.Fatal(k)
		}
	}
	m.writes = 0
	r, e = mirror.Push(root, state, id, m)
	must(t, e)
	if r.Unchanged != 2 || m.writes != 0 {
		t.Fatalf("unchanged: %+v writes %d", r, m.writes)
	}
	fresh, record, _ := device(t)
	must(t, mirror.Restore(m, fresh, record, id))
	opened, problems, e := vault.OpenScopes(fresh, record, id)
	must(t, e)
	defer func() {
		for _, v := range opened {
			v.Close()
		}
	}()
	if len(problems) != 0 || len(opened) != 2 {
		t.Fatalf("restored: %v %v", opened, problems)
	}
	want := map[string]map[string]string{"Work": {"one": "alpha"}, "Home": {"two": "beta"}}
	for _, v := range opened {
		n, e := v.Name()
		must(t, e)
		entries, e := v.Entries()
		must(t, e)
		if len(entries) != 1 {
			t.Fatal(entries)
		}
		item, e := v.Read(entries[0].ID)
		must(t, e)
		if len(item.Fields) != 1 || item.Fields[0].Label != "value" || string(item.Fields[0].Value) != want[n][entries[0].Title] {
			t.Fatalf("%s %s %+v", n, entries[0].Title, item)
		}
	}
	if e := mirror.Restore(m, fresh, record, id); !errors.Is(e, mirror.ErrNotEmpty) {
		t.Fatalf("nonempty: %v", e)
	}
}
func TestPushChangesAndRefusals(t *testing.T) {
	root, state, id := device(t)
	v := scope(t, root, state, id, "A")
	must(t, addValue(v, "old", []byte("old")))
	m := newMemory()
	_, e := mirror.Push(root, state, id, m)
	must(t, e)
	before := map[string][]byte{}
	for k, b := range m.files {
		before[k] = bytes.Clone(b)
	}
	must(t, changeValue(v, "old", []byte("new")))
	must(t, addValue(v, "added", nil))
	must(t, dropValue(v, "old"))
	must(t, v.SetName("Renamed"))
	r, e := mirror.Push(root, state, id, m)
	must(t, e)
	if r.Updated != 1 {
		t.Fatal(r)
	}
	for k, b := range m.files {
		local, e := os.ReadFile(filepath.Join(root, filepath.FromSlash(k)))
		must(t, e)
		if !bytes.Equal(local, b) {
			t.Fatal(k)
		}
	}
	for k := range before {
		if _, ok := m.files[k]; !ok && strings.HasSuffix(k, "manifest") {
			t.Fatal("lost manifest")
		}
	}
	m.files["scopes/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/manifest"] = []byte("invalid")
	r, e = mirror.Push(root, state, id, m)
	must(t, e)
	// A scope in the mirror with a manifest this identity did not sign is foreign.
	if !reflect.DeepEqual(r.Foreign, []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}) || r.Updated != 0 {
		t.Fatal(r)
	}
	// A scope in the mirror with files and no manifest is only unknown.
	delete(m.files, "scopes/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/manifest")
	m.files["scopes/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/scope"] = []byte("x")
	r, e = mirror.Push(root, state, id, m)
	must(t, e)
	if !reflect.DeepEqual(r.Unknown, []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}) || len(r.Foreign) != 0 {
		t.Fatal(r)
	}
	must(t, mirror.RemoveScope(m, v.ID()))
	for k := range m.files {
		if strings.Contains(k, v.ID()) {
			t.Fatal(k)
		}
	}
}
func TestMirrorAheadForeignAndLocalProblem(t *testing.T) {
	root, state, id := device(t)
	v := scope(t, root, state, id, "A")
	must(t, addValue(v, "x", []byte("one")))
	m := newMemory()
	_, e := mirror.Push(root, state, id, m)
	must(t, e)
	old := newMemory()
	for k, b := range m.files {
		old.files[k] = bytes.Clone(b)
	}
	stale, staleState, _ := device(t)
	must(t, mirror.Restore(old, stale, staleState, id))
	must(t, changeValue(v, "x", []byte("two")))
	_, e = mirror.Push(root, state, id, m)
	must(t, e)
	snapshot := newMemory()
	for k, b := range m.files {
		snapshot.files[k] = bytes.Clone(b)
	}
	r, e := mirror.Push(stale, staleState, id, m)
	must(t, e)
	if !reflect.DeepEqual(r.Ahead, []string{v.ID()}) || !reflect.DeepEqual(snapshot.files, m.files) {
		t.Fatalf("ahead: %+v", r)
	}
	// A manifest signed for the same identity but another scope is foreign.
	other := scope(t, root, state, id, "B")
	_, e = mirror.Push(root, state, id, old)
	must(t, e)
	otherManifest := old.files["scopes/"+other.ID()+"/manifest"]
	m.files["scopes/"+v.ID()+"/manifest"] = bytes.Clone(otherManifest)
	r, e = mirror.Push(root, state, id, m)
	must(t, e)
	if !reflect.DeepEqual(r.Foreign, []string{v.ID()}) {
		t.Fatal(r)
	}
	// A missing manifest is treated as absent, even when files are present.
	// The refused push above wrote nothing, so scope B is copied now as well.
	delete(m.files, "scopes/"+v.ID()+"/manifest")
	r, e = mirror.Push(root, state, id, m)
	must(t, e)
	if r.Updated != 2 {
		t.Fatal(r)
	}
	// Damaged local content cannot overwrite the mirror.
	previous := newMemory()
	for k, b := range m.files {
		previous.files[k] = bytes.Clone(b)
	}
	path := filepath.Join(root, "scopes", v.ID(), "name")
	must(t, os.WriteFile(path, []byte("broken"), 0600))
	r, e = mirror.Push(root, state, id, m)
	must(t, e)
	if r.LocalProblems != 1 || !reflect.DeepEqual(previous.files, m.files) {
		t.Fatalf("local: %+v", r)
	}
}
func TestRestoreRejectsIncompleteWithoutChangingEmptyDevice(t *testing.T) {
	root, state, id := device(t)
	v := scope(t, root, state, id, "Source")
	must(t, addValue(v, "item", []byte("value")))
	m := newMemory()
	_, e := mirror.Push(root, state, id, m)
	must(t, e)
	dest, record, _ := device(t)
	empty := scope(t, dest, record, id, "Empty")
	recordBefore, e := os.ReadFile(record)
	must(t, e)
	dir := filepath.Join(dest, "scopes", empty.ID())
	scopeBefore, e := os.ReadFile(filepath.Join(dir, "scope"))
	must(t, e)
	nameBefore, e := os.ReadFile(filepath.Join(dir, "name"))
	must(t, e)
	manifestBefore, e := os.ReadFile(filepath.Join(dir, "manifest"))
	must(t, e)
	check := func(label string) {
		t.Helper()
		if e := mirror.Restore(m, dest, record, id); !errors.Is(e, mirror.ErrIncomplete) {
			t.Fatalf("%s: %v", label, e)
		}
		for p, want := range map[string][]byte{record: recordBefore, filepath.Join(dir, "scope"): scopeBefore, filepath.Join(dir, "name"): nameBefore, filepath.Join(dir, "manifest"): manifestBefore} {
			got, e := os.ReadFile(p)
			must(t, e)
			if !bytes.Equal(got, want) {
				t.Fatalf("%s changed %s", label, p)
			}
		}
		entries, e := os.ReadDir(dest)
		must(t, e)
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".restore-") {
				t.Fatal("stage remains")
			}
		}
	}
	for k, b := range m.files {
		delete(m.files, k)
		check("missing " + k)
		m.files[k] = b
		break
	}
	for k, b := range m.files {
		if !strings.HasSuffix(k, "/name") {
			continue
		}
		bad := bytes.Clone(b)
		bad[0] ^= 1
		m.files[k] = bad
		check("changed " + k)
		m.files[k] = b
		break
	}
	m.files["scopes/"+v.ID()+"/secrets/extra"] = []byte("bad")
	check("extra")
}
func TestForkAndForeignRestore(t *testing.T) {
	root, state, id := device(t)
	v := scope(t, root, state, id, "A")
	must(t, addValue(v, "x", []byte("base")))
	m := newMemory()
	_, e := mirror.Push(root, state, id, m)
	must(t, e)
	second, secondState, _ := device(t)
	must(t, mirror.Restore(m, second, secondState, id))
	must(t, changeValue(v, "x", []byte("first")))
	_, e = mirror.Push(root, state, id, m)
	must(t, e)
	scopes, problems, e := vault.OpenScopes(second, secondState, id)
	must(t, e)
	if len(problems) != 0 || len(scopes) != 1 {
		t.Fatal(problems)
	}
	must(t, changeValue(scopes[0], "x", []byte("different")))
	scopes[0].Close()
	snapshot := newMemory()
	for k, b := range m.files {
		snapshot.files[k] = bytes.Clone(b)
	}
	r, e := mirror.Push(second, secondState, id, m)
	must(t, e)
	if !reflect.DeepEqual(r.Ahead, []string{v.ID()}) || !reflect.DeepEqual(m.files, snapshot.files) {
		t.Fatalf("fork: %+v", r)
	}
	other := newMemory()
	foreignRoot, foreignState, foreignID := device(t)
	foreign := scope(t, foreignRoot, foreignState, foreignID, "Foreign")
	must(t, addValue(foreign, "item", []byte("value")))
	_, e = mirror.Push(foreignRoot, foreignState, foreignID, other)
	must(t, e)
	fresh, record, _ := device(t)
	if e = mirror.Restore(other, fresh, record, id); !errors.Is(e, mirror.ErrForeign) {
		t.Fatalf("foreign restore: %v", e)
	}
	if _, e = os.Stat(record); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("record modified: %v", e)
	}
	if _, e = os.Stat(fresh); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("vault modified: %v", e)
	}
}
func TestRestoreReplacesEmptyAndRecordsRollback(t *testing.T) {
	root, state, id := device(t)
	v := scope(t, root, state, id, "Real")
	must(t, addValue(v, "item", []byte("first")))
	m := newMemory()
	_, e := mirror.Push(root, state, id, m)
	must(t, e)
	dest, record, _ := device(t)
	empty := scope(t, dest, record, id, "Empty")
	oldID := empty.ID()
	must(t, mirror.Restore(m, dest, record, id))
	if _, e := os.Stat(filepath.Join(dest, "scopes", oldID)); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("empty scope remains: %v", e)
	}
	for _, k := range []string{"manifest", "name"} {
		blob, e := os.ReadFile(filepath.Join(dest, "scopes", v.ID(), k))
		must(t, e)
		if k == "manifest" {
			_ = blob
		}
	}
	restored, issues, e := vault.OpenScopes(dest, record, id)
	must(t, e)
	if len(issues) != 0 || len(restored) != 1 {
		t.Fatal(issues)
	}
	must(t, changeValue(restored[0], "item", []byte("second")))
	restored[0].Close()
	// Putting an older signed scope back after a higher generation was recorded is a rollback.
	for k, b := range m.files {
		must(t, os.WriteFile(filepath.Join(dest, filepath.FromSlash(k)), b, 0600))
	}
	opened, problems, e := vault.OpenScopes(dest, record, id)
	must(t, e)
	for _, s := range opened {
		s.Close()
	}
	if len(problems) != 1 || problems[0].Kind != vault.ProblemRollback {
		t.Fatalf("rollback: %+v", problems)
	}
}
func TestInterruptedPush(t *testing.T) {
	for _, nth := range []int{2, 3} {
		t.Run(string(rune('0'+nth)), func(t *testing.T) {
			root, state, id := device(t)
			v := scope(t, root, state, id, "A")
			must(t, addValue(v, "x", []byte("one")))
			m := newMemory()
			_, e := mirror.Push(root, state, id, m)
			must(t, e)
			must(t, changeValue(v, "x", []byte("two")))
			must(t, v.SetName("New"))
			m.writes = 0
			m.fail = nth
			_, e = mirror.Push(root, state, id, m)
			if e == nil {
				t.Fatal("expected write error")
			}
			fresh, record, _ := device(t)
			if e = mirror.Restore(m, fresh, record, id); !errors.Is(e, mirror.ErrIncomplete) {
				t.Fatalf("write %d restore: %v", nth, e)
			}
			m.fail = 0
			_, e = mirror.Push(root, state, id, m)
			must(t, e)
			must(t, mirror.Restore(m, fresh, record, id))
		})
	}
}
func TestRestoreRefusals(t *testing.T) {
	root, state, id := device(t)
	v := scope(t, root, state, id, "A")
	must(t, addValue(v, "x", []byte("x")))
	m := newMemory()
	_, e := mirror.Push(root, state, id, m)
	must(t, e)
	otherRoot, otherState, _ := device(t)
	if e := mirror.Restore(newMemory(), otherRoot, otherState, id); !errors.Is(e, mirror.ErrEmpty) {
		t.Fatal(e)
	}
	for k, b := range m.files {
		delete(m.files, k)
		if e := mirror.Restore(m, otherRoot, otherState, id); !errors.Is(e, mirror.ErrIncomplete) {
			t.Fatalf("missing %s: %v", k, e)
		}
		m.files[k] = b
	}
	if _, e := os.Stat(otherRoot); e == nil {
		entries, _ := os.ReadDir(otherRoot)
		for _, x := range entries {
			if strings.HasPrefix(x.Name(), ".restore-") {
				t.Fatal("staging left")
			}
		}
	}
}

// A folder that already holds another identity's vault receives nothing, even
// though this device's scopes have different ids.
func TestPushWritesNothingIntoAnotherVaultsMirror(t *testing.T) {
	theirRoot, theirState, theirID := device(t)
	scope(t, theirRoot, theirState, theirID, "Theirs").Close()
	shared := newMemory()
	if _, err := mirror.Push(theirRoot, theirState, theirID, shared); err != nil {
		t.Fatal(err)
	}
	before := map[string]string{}
	for k, v := range shared.files {
		before[k] = string(v)
	}

	root := filepath.Join(t.TempDir(), "vault")
	state := filepath.Join(filepath.Dir(root), "state")
	seed := make([]byte, identity.SeedSize)
	seed[0] = 0x77
	id, err := identity.FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	scope(t, root, state, id, "Mine").Close()
	report, err := mirror.Push(root, state, id, shared)
	if err != nil || len(report.Foreign) != 1 || report.Updated != 0 {
		t.Fatalf("Push into another vault's mirror = %+v, %v", report, err)
	}
	if len(shared.files) != len(before) {
		t.Fatalf("files were added to another vault's mirror: %d -> %d", len(before), len(shared.files))
	}
	for k, v := range before {
		if string(shared.files[k]) != v {
			t.Fatalf("%s changed", k)
		}
	}
}

// A scope whose last secret was deleted has a higher recorded generation than
// an older mirror copy. Restoring that copy would bring the deleted secret back
// and reset the record, so the rollback would no longer be detected.
func TestRestoreRefusesAnOlderMirrorThanTheLocalRecord(t *testing.T) {
	root, state, id := device(t)
	v := scope(t, root, state, id, "Work")
	must(t, addValue(v, "gone", []byte("secret")))
	m := newMemory()
	_, e := mirror.Push(root, state, id, m)
	must(t, e)
	entries, e := v.Entries()
	must(t, e)
	must(t, v.Remove(entries[0].ID))
	recordBefore, e := os.ReadFile(state)
	must(t, e)
	if e := mirror.Restore(m, root, state, id); !errors.Is(e, vault.ErrRollback) {
		t.Fatalf("Restore = %v, want ErrRollback", e)
	}
	recordAfter, e := os.ReadFile(state)
	must(t, e)
	if !bytes.Equal(recordBefore, recordAfter) {
		t.Fatal("the record changed")
	}
	left, e := v.Entries()
	must(t, e)
	if len(left) != 0 {
		t.Fatalf("the deleted secret came back: %v", left)
	}
}

// A mirror folder typed with a trailing separator must still stop the removal
// of empty folders at the mirror folder itself.
func TestDirRemoveStopsAtRootWithTrailingSeparator(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "mirror")
	s := mirror.Dir(root + string(os.PathSeparator))
	must(t, s.Write("scopes/a/name", []byte("x")))
	must(t, s.Remove("scopes/a/name"))
	if _, e := os.Stat(parent); e != nil {
		t.Fatalf("the parent folder was removed: %v", e)
	}
}
