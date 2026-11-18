package vault

import (
	"errors"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/identity"
	"path/filepath"
	"slices"
)

func TestEntriesDoNotOpenBodiesAndInterruptedMove(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = 3
	}
	id, err := identity.FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "vault")
	v, err := Create(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	for _, name := range []string{"a", "b", "c"} {
		if _, err = v.Add(Item{Path: []string{"old", "child"}, Title: name, Fields: []Field{{Kind: FieldHidden, Label: "value", Value: []byte(name)}}}); err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	itemBodyOpened = func() { count++ }
	defer func() { itemBodyOpened = nil }()
	entries, err := v.Entries()
	if err != nil || len(entries) != 3 || count != 0 {
		t.Fatalf("Entries opened %d bodies: %v", count, err)
	}
	interrupted := errors.New("interrupted")
	moveFolderStep = func(n int) error {
		if n == 2 {
			return interrupted
		}
		return nil
	}
	defer func() { moveFolderStep = nil }()
	if err = v.MoveFolder([]string{"old"}, []string{"new"}); !errors.Is(err, interrupted) {
		t.Fatalf("move: %v", err)
	}
	if v.Generation() != 6 {
		t.Fatalf("generation %d want 6", v.Generation())
	}
	reopened, err := Open(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	after, err := reopened.Entries()
	if err != nil || len(after) != 3 {
		t.Fatalf("reopen %+v %v", after, err)
	}
	for _, entry := range after {
		if !slices.Equal(entry.Path, []string{"old", "child"}) && !slices.Equal(entry.Path, []string{"new", "child"}) {
			t.Fatalf("path %+v", entry)
		}
		item, e := reopened.Read(entry.ID)
		if e != nil || string(item.Fields[0].Value) != entry.Title {
			t.Fatalf("read %+v %v", item, e)
		}
	}
}
