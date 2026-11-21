package mirror_test

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/mirror"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

// dump lists every scope, folder path, title, label, kind and value of a device.
func dump(t *testing.T, root, state string, id *identity.Identity) []string {
	t.Helper()
	scopes, problems, e := vault.OpenScopes(root, state, id)
	must(t, e)
	if len(problems) != 0 {
		t.Fatalf("problems: %+v", problems)
	}
	var out []string
	for _, v := range scopes {
		name, e := v.Name()
		must(t, e)
		entries, e := v.Entries()
		must(t, e)
		for _, en := range entries {
			item, e := v.Read(en.ID)
			must(t, e)
			for _, f := range item.Fields {
				out = append(out, fmt.Sprintf("%s|%v|%s|%s|%d|%s", name, item.Path, item.Title, f.Label, f.Kind, f.Value))
			}
		}
		v.Close()
	}
	sort.Strings(out)
	return out
}

// The restore drill: a device is lost, and everything comes back from the
// recovery phrase and the mirror folder alone.
func TestRestoreDrillFromRecoveryPhraseAndMirror(t *testing.T) {
	base := t.TempDir()
	root, state := filepath.Join(base, "old", "vault"), filepath.Join(base, "old", "state")
	idPath := filepath.Join(base, "old", "identity")
	id, e := identity.Generate()
	must(t, e)
	sealed, passphrase, e := id.Seal()
	must(t, e)
	must(t, identity.Save(idPath, sealed))
	phrase := id.RecoveryPhrase()

	work := scope(t, root, state, id, "Work")
	home := scope(t, root, state, id, "Home")
	_, e = work.Add(vault.Item{Path: []string{"Cloud", "AWS"}, Title: "Main account", Fields: []vault.Field{
		{Kind: vault.FieldVisible, Label: "user", Value: []byte("admin@example.test")},
		{Kind: vault.FieldHidden, Label: "key one", Value: []byte("AKIA-one")},
		{Kind: vault.FieldHidden, Label: "key two", Value: []byte("AKIA-two")},
		{Kind: vault.FieldNote, Label: "notes", Value: []byte("line 1\nline 2\n")},
	}})
	must(t, e)
	_, e = home.Add(vault.Item{Title: "Streaming", Fields: []vault.Field{{Kind: vault.FieldHidden, Label: "password", Value: []byte("pässwörd ✓")}}})
	must(t, e)
	want := dump(t, root, state, id)
	if len(want) != 5 {
		t.Fatalf("setup dump: %v", want)
	}
	mirrorDir := filepath.Join(base, "cloud", "qrypt-mirror")
	_, e = mirror.Push(root, state, id, mirror.Dir(mirrorDir))
	must(t, e)
	work.Close()
	home.Close()
	id.Close()

	// New device: nothing of the old one exists. The identity is rebuilt from the
	// recovery phrase; the first run has created one empty scope.
	newRoot, newState := filepath.Join(base, "new", "vault"), filepath.Join(base, "new", "state")
	id2, e := identity.FromRecoveryPhrase(phrase)
	must(t, e)
	defer id2.Close()
	scope(t, newRoot, newState, id2, "Personal")
	must(t, mirror.Restore(mirror.Dir(mirrorDir), newRoot, newState, id2))
	if got := dump(t, newRoot, newState, id2); !reflect.DeepEqual(got, want) {
		t.Fatalf("restored data differs:\n got %v\nwant %v", got, want)
	}

	// The identity file written before the loss opens with its passphrase, and
	// opens the same vault.
	id3, e := identity.Load(idPath, passphrase)
	must(t, e)
	defer id3.Close()
	if got := dump(t, newRoot, newState, id3); !reflect.DeepEqual(got, want) {
		t.Fatalf("data through the identity file differs:\n got %v\nwant %v", got, want)
	}
}
