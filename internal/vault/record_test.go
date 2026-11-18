package vault_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/vault"
)

func TestRecordLifecycle(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	state := filepath.Join(filepath.Dir(root), "state")
	id := newIdentity(t, 1)
	v, err := vault.NewScope(root, state, id, "Work")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	check := func() {
		t.Helper()
		raw, e := os.ReadFile(state)
		if e != nil {
			t.Fatal(e)
		}
		h := v.ManifestHash()
		want := v.ID() + " " + fmt.Sprint(v.Generation()) + " " + hex.EncodeToString(h[:]) + "\n"
		if string(raw) != want {
			t.Fatalf("record %q want %q", raw, want)
		}
	}
	check()
	if info, e := os.Stat(state); e != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
		t.Fatalf("record permissions: %v %v", info, e)
	}
	for _, op := range []func() error{func() error { return putValue(v, "a", []byte("one")) }, func() error { return replaceValue(v, "a", []byte("two")) }, func() error { return removeValue(v, "a") }, func() error { return vault.RenameScope(root, state, id, v, "New") }, func() error { return v.SetName("Next") }} {
		if e := op(); e != nil {
			t.Fatal(e)
		}
		check()
	}
	if e := vault.RemoveScope(v); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(state)
	if e != nil || len(raw) != 0 {
		t.Fatalf("removed record %q %v", raw, e)
	}
}

func TestRecordComparisonAndAccept(t *testing.T) {
	for _, tc := range []struct {
		name   string
		damage func(*testing.T, string, *vault.Vault, string)
		kind   vault.ProblemKind
	}{
		{"missing", func(t *testing.T, d string, v *vault.Vault, s string) {
			t.Helper()
			if e := os.Remove(secretFiles(t, d)[0]); e != nil {
				t.Fatal(e)
			}
		}, vault.ProblemFiles},
		{"removed manifest", func(t *testing.T, d string, v *vault.Vault, s string) {
			t.Helper()
			if e := os.Remove(filepath.Join(d, "manifest")); e != nil {
				t.Fatal(e)
			}
		}, vault.ProblemSignature},
		{"rollback", func(t *testing.T, d string, v *vault.Vault, s string) {
			t.Helper()
			old := snapshot(t, d)
			if e := replaceValue(v, "a", []byte("new")); e != nil {
				t.Fatal(e)
			}
			for p, b := range old {
				if e := os.WriteFile(p, []byte(b), 0600); e != nil {
					t.Fatal(e)
				}
			}
		}, vault.ProblemRollback},
		{"fork", func(t *testing.T, d string, v *vault.Vault, s string) {
			t.Helper()
			old := snapshot(t, d)
			if e := replaceValue(v, "a", []byte("new")); e != nil {
				t.Fatal(e)
			}
			record, e := os.ReadFile(s)
			if e != nil {
				t.Fatal(e)
			}
			for p, b := range old {
				if e = os.WriteFile(p, []byte(b), 0600); e != nil {
					t.Fatal(e)
				}
			}
			id := newIdentity(t, 1)
			other, e := vault.Open(d, id)
			if e != nil {
				t.Fatal(e)
			}
			if e = replaceValue(other, "a", []byte("alternate")); e != nil {
				t.Fatal(e)
			}
			other.Close()
			if e = os.WriteFile(s, record, 0600); e != nil {
				t.Fatal(e)
			}
		}, vault.ProblemFork},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "vault")
			state := filepath.Join(filepath.Dir(root), "state")
			id := newIdentity(t, 1)
			v, e := vault.NewScope(root, state, id, "Work")
			if e != nil {
				t.Fatal(e)
			}
			defer v.Close()
			if e = putValue(v, "a", []byte("old")); e != nil {
				t.Fatal(e)
			}
			d := filepath.Join(root, "scopes", v.ID())
			tc.damage(t, d, v, state)
			scopes, problems, e := vault.OpenScopes(root, state, id)
			if e != nil || len(scopes) != 0 || len(problems) != 1 || problems[0].Kind != tc.kind {
				t.Fatalf("open %v %+v %v", scopes, problems, e)
			}
			recorded := problems[0].Recorded
			if e = vault.Accept(root, state, id, v.ID()); e != nil {
				t.Fatal(e)
			}
			scopes, problems, e = vault.OpenScopes(root, state, id)
			if e != nil || len(problems) != 0 || len(scopes) != 1 {
				t.Fatalf("accepted %v %+v %v", scopes, problems, e)
			}
			defer scopes[0].Close()
			if scopes[0].Generation() <= v.Generation() || scopes[0].Generation() <= recorded {
				t.Fatal("generation did not advance")
			}
			if tc.name != "missing" {
				want := "old"
				if tc.name == "fork" {
					want = "alternate"
				}
				if got, e := readValue(scopes[0], "a"); e != nil || string(got) != want {
					t.Fatalf("surviving value %q %v, want %q", got, e, want)
				}
			}
		})
	}
}

func TestRecordFirstOpenAndCrashOrder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	state := filepath.Join(filepath.Dir(root), "state")
	id := newIdentity(t, 1)
	v, e := vault.NewScope(root, state, id, "Work")
	if e != nil {
		t.Fatal(e)
	}
	defer v.Close()
	if e = os.Remove(state); e != nil {
		t.Fatal(e)
	}
	scopes, problems, e := vault.OpenScopes(root, state, id)
	if e != nil || len(problems) != 0 || len(scopes) != 1 {
		t.Fatalf("new device: %v %+v %v", scopes, problems, e)
	}
	scopes[0].Close()
	before, e := os.ReadFile(state)
	if e != nil {
		t.Fatal(e)
	}
	plain, e := vault.Open(filepath.Join(root, "scopes", v.ID()), id)
	if e != nil {
		t.Fatal(e)
	}
	if e = putValue(plain, "a", []byte("value")); e != nil {
		t.Fatal(e)
	}
	plain.Close()
	unchanged, e := os.ReadFile(state)
	if e != nil || !bytes.Equal(before, unchanged) {
		t.Fatal("plain Open wrote record")
	}
	scopes, problems, e = vault.OpenScopes(root, state, id)
	if e != nil || len(problems) != 0 || len(scopes) != 1 {
		t.Fatalf("manifest ahead: %v %+v %v", scopes, problems, e)
	}
	if got, e := readValue(scopes[0], "a"); e != nil || string(got) != "value" {
		t.Fatalf("value %q %v", got, e)
	}
	scopes[0].Close()
	if after, _ := os.ReadFile(state); bytes.Equal(after, before) {
		t.Fatal("higher manifest not recorded")
	}
}

func TestRecordRemovedAndIsolated(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	state := filepath.Join(filepath.Dir(root), "state")
	id := newIdentity(t, 1)
	a, e := vault.NewScope(root, state, id, "A")
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, e := vault.NewScope(root, state, id, "B")
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	if e = putValue(b, "keep", []byte("alive")); e != nil {
		t.Fatal(e)
	}
	if e = os.RemoveAll(filepath.Join(root, "scopes", a.ID())); e != nil {
		t.Fatal(e)
	}
	scopes, problems, e := vault.OpenScopes(root, state, id)
	if e != nil || len(scopes) != 1 || len(problems) != 1 || problems[0].Kind != vault.ProblemRemoved || problems[0].ScopeID != a.ID() {
		t.Fatalf("isolated: %v %+v %v", scopes, problems, e)
	}
	if got, e := readValue(scopes[0], "keep"); e != nil || string(got) != "alive" {
		t.Fatalf("survivor: %q %v", got, e)
	}
	scopes[0].Close()
	if e = vault.Accept(root, state, id, a.ID()); e != nil {
		t.Fatal(e)
	}
	scopes, problems, e = vault.OpenScopes(root, state, id)
	if e != nil || len(scopes) != 1 || len(problems) != 0 {
		t.Fatalf("accepted removal: %v %+v %v", scopes, problems, e)
	}
	scopes[0].Close()
}

func TestAcceptRetainsDecryptableAndRejectsWrongIdentity(t *testing.T) {
	for _, damage := range []string{"unexpected valid", "unexpected broken", "changed", "broken name", "broken value"} {
		t.Run(damage, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "vault")
			state := filepath.Join(filepath.Dir(root), "state")
			id := newIdentity(t, 1)
			v, e := vault.NewScope(root, state, id, "Work")
			if e != nil {
				t.Fatal(e)
			}
			defer v.Close()
			if e = putValue(v, "a", []byte("one")); e != nil {
				t.Fatal(e)
			}
			dir := filepath.Join(root, "scopes", v.ID())
			original := secretFiles(t, dir)[0]
			data, e := os.ReadFile(original)
			if e != nil {
				t.Fatal(e)
			}
			manifest, e := os.ReadFile(filepath.Join(dir, "manifest"))
			if e != nil {
				t.Fatal(e)
			}
			switch damage {
			case "unexpected valid", "unexpected broken":
				// A valid extra file is generated by this same scope's keys using a plain opener.
				plain, e := vault.Open(dir, id)
				if e != nil {
					t.Fatal(e)
				}
				if e = putValue(plain, "b", []byte("two")); e != nil {
					t.Fatal(e)
				}
				plain.Close()
				extra := secretFiles(t, dir)
				for _, p := range extra {
					if p != original {
						if damage == "unexpected broken" {
							blob, _ := os.ReadFile(p)
							blob[len(blob)-1] ^= 1
							if e = os.WriteFile(p, blob, 0600); e != nil {
								t.Fatal(e)
							}
						}
					}
				}
				if e = os.WriteFile(filepath.Join(dir, "manifest"), manifest, 0600); e != nil {
					t.Fatal(e)
				}
			case "changed":
				data[len(data)-1] ^= 1
				if e = os.WriteFile(original, data, 0600); e != nil {
					t.Fatal(e)
				}
			case "broken name":
				name := filepath.Join(dir, "name")
				blob, _ := os.ReadFile(name)
				blob[40] ^= 1
				if e = os.WriteFile(name, blob, 0600); e != nil {
					t.Fatal(e)
				}
			case "broken value":
				data[len(data)-1] ^= 1
				if e = os.WriteFile(original, data, 0600); e != nil {
					t.Fatal(e)
				}
			}
			before := snapshot(t, dir)
			if e = vault.Accept(root, state, newIdentity(t, 2), v.ID()); !errors.Is(e, vault.ErrScopeUnreadable) {
				t.Fatalf("wrong identity: %v", e)
			}
			if !sameSnapshot(before, snapshot(t, dir)) {
				t.Fatal("wrong identity changed scope")
			}
			scopes, problems, e := vault.OpenScopes(root, state, id)
			if e != nil || len(problems) != 1 || len(scopes) != 0 {
				t.Fatalf("problem: %v %+v %v", scopes, problems, e)
			}
			if damage == "broken name" || damage == "broken value" || damage == "unexpected broken" || damage == "changed" {
				if problems[0].Undecryptable != 1 {
					t.Fatalf("undecryptable: %+v", problems[0])
				}
			}
			if e = vault.Accept(root, state, id, v.ID()); e != nil {
				t.Fatal(e)
			}
			scopes, problems, e = vault.OpenScopes(root, state, id)
			if e != nil || len(problems) != 0 || len(scopes) != 1 {
				t.Fatalf("accepted: %v %+v %v", scopes, problems, e)
			}
			defer scopes[0].Close()
			if damage == "broken name" || damage == "unexpected valid" || damage == "unexpected broken" {
				if got, e := readValue(scopes[0], "a"); e != nil || string(got) != "one" {
					t.Fatalf("survivor %q %v", got, e)
				}
			}
			if damage == "unexpected valid" {
				if got, e := readValue(scopes[0], "b"); e != nil || string(got) != "two" {
					t.Fatalf("valid unexpected lost: %q %v", got, e)
				}
			}
		})
	}
}

func TestLegacyRecordAndHealthyAccept(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	state := filepath.Join(filepath.Dir(root), "state")
	id := newIdentity(t, 1)
	v, e := vault.Create(root, id)
	if e != nil {
		t.Fatal(e)
	}
	defer v.Close()
	if e = putValue(v, "old", []byte("readable")); e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(filepath.Join(root, "manifest")); e != nil {
		t.Fatal(e)
	}
	scopes, problems, e := vault.OpenScopes(root, state, id)
	if e != nil || len(problems) != 0 || len(scopes) != 1 {
		t.Fatalf("legacy: %v %+v %v", scopes, problems, e)
	}
	scope := scopes[0]
	defer scope.Close()
	if scope.Generation() != 1 {
		t.Fatalf("first generation %d", scope.Generation())
	}
	if got, e := readValue(scope, "old"); e != nil || string(got) != "readable" {
		t.Fatalf("legacy value %q %v", got, e)
	}
	before := snapshot(t, root)
	record, e := os.ReadFile(state)
	if e != nil {
		t.Fatal(e)
	}
	if e = vault.Accept(root, state, id, scope.ID()); e != nil {
		t.Fatal(e)
	}
	after, e := os.ReadFile(state)
	if e != nil || !bytes.Equal(record, after) || !sameSnapshot(before, snapshot(t, root)) {
		t.Fatal("healthy Accept changed disk")
	}
}

func TestInvalidRecordIsNeverOverwritten(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	state := filepath.Join(filepath.Dir(root), "state")
	id := newIdentity(t, 1)
	if e := os.WriteFile(state, []byte("invalid\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, _, e := vault.OpenScopes(root, state, id); e == nil || errors.Is(e, vault.ErrIntegrity) {
		t.Fatalf("invalid record: %v", e)
	}
	if _, e := vault.NewScope(root, state, id, "Work"); e == nil {
		t.Fatal("invalid record accepted")
	}
	raw, _ := os.ReadFile(state)
	if !bytes.Equal(raw, []byte("invalid\n")) {
		t.Fatal("record overwritten")
	}
}
