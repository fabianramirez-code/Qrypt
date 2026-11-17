package vault_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/vault"
)

func TestRecordRestoredScopes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	id := newIdentity(t, 1)
	state := filepath.Join(filepath.Dir(root), "state")
	a := scopeAt(t, root, id, "A")
	b := scopeAt(t, root, id, "B")
	if err := vault.RecordRestoredScopes(state, []*vault.Vault{a}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), a.ID()+" ") || strings.Contains(string(raw), b.ID()) {
		t.Fatalf("record: %q", raw)
	}
	opened, problems, err := vault.OpenScopes(root, state, id)
	if err != nil || len(problems) != 0 || len(opened) != 2 {
		t.Fatalf("open: %v %+v %v", opened, problems, err)
	}
	for _, v := range opened {
		v.Close()
	}
}
