package vault_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

func TestManifestConformance(t *testing.T) {
	v, dir, id := newVault(t)
	if err := v.SetName("Work"); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a", "b"} {
		if err := putValue(v, n, []byte(n)); err != nil {
			t.Fatal(err)
		}
	}
	files := secretFiles(t, dir)
	sort.Strings(files)
	raw, err := os.ReadFile(filepath.Join(dir, "manifest"))
	if err != nil {
		t.Fatal(err)
	}
	expected := append([]byte("QRYPTSM\x01"), make([]byte, 16+8+32+4+48*2)...)
	scopeID, _ := hex.DecodeString(v.ID())
	copy(expected[8:24], scopeID)
	binary.BigEndian.PutUint64(expected[24:32], 4)
	name, _ := os.ReadFile(filepath.Join(dir, "name"))
	nh := sha256.Sum256(name)
	copy(expected[32:64], nh[:])
	binary.BigEndian.PutUint32(expected[64:68], 2)
	for i, path := range files {
		fid, _ := hex.DecodeString(strings.TrimSuffix(filepath.Base(path), ".qrs"))
		copy(expected[68+48*i:], fid)
		blob, _ := os.ReadFile(path)
		hash := sha256.Sum256(blob)
		copy(expected[84+48*i:], hash[:])
	}
	if !bytes.Equal(raw[:len(expected)], expected) {
		t.Fatal("unsigned manifest differs from documented format")
	}
	pub := id.PublicKeys()
	msg := append([]byte("qrypt/v1/vault/manifest"), expected...)
	if !ed25519.Verify(pub.Ed25519, msg, raw[len(expected):len(expected)+64]) {
		t.Fatal("Ed25519 signature")
	}
	var ml mldsa65.PublicKey
	if err := ml.UnmarshalBinary(pub.MLDSA65); err != nil {
		t.Fatal(err)
	}
	if !mldsa65.Verify(&ml, msg, nil, raw[len(expected)+64:]) {
		t.Fatal("ML-DSA signature")
	}
	parsed, err := vault.VerifyManifest(id, raw)
	if err != nil || parsed.ScopeID != v.ID() || parsed.Generation != 4 || parsed.NameHash != nh || len(parsed.Files) != 2 || parsed.Hash != sha256.Sum256(raw) {
		t.Fatalf("VerifyManifest: %+v %v", parsed, err)
	}
	for _, path := range files {
		blob, _ := os.ReadFile(path)
		key := strings.TrimSuffix(filepath.Base(path), ".qrs")
		if parsed.Files[key] != sha256.Sum256(blob) {
			t.Fatalf("hash %s", key)
		}
	}
	reopened, err := vault.Open(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	reopened.Close()
}

func TestManifestGenerationAndLegacy(t *testing.T) {
	v, dir, id := newVault(t)
	check := func(n uint64) {
		t.Helper()
		if v.Generation() != n {
			t.Fatalf("generation %d, want %d", v.Generation(), n)
		}
		raw, err := os.ReadFile(filepath.Join(dir, "manifest"))
		if err != nil || v.ManifestHash() != sha256.Sum256(raw) {
			t.Fatalf("hash: %v", err)
		}
	}
	check(1)
	if err := v.SetName("Home"); err != nil {
		t.Fatal(err)
	}
	check(2)
	if err := putValue(v, "x", []byte("one")); err != nil {
		t.Fatal(err)
	}
	check(3)
	if err := replaceValue(v, "x", []byte("two")); err != nil {
		t.Fatal(err)
	}
	check(4)
	if err := removeValue(v, "x"); err != nil {
		t.Fatal(err)
	}
	check(5)
	if err := os.Remove(filepath.Join(dir, "manifest")); err != nil {
		t.Fatal(err)
	}
	v.Close()
	opened, err := vault.Open(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if opened.Generation() != 1 {
		t.Fatalf("legacy generation: %d", opened.Generation())
	}
	opened.Close()
	if err := putValue(opened, "x", nil); !errors.Is(err, vault.ErrClosed) {
		t.Fatalf("closed Put: %v", err)
	}
	if err := opened.SetName("x"); !errors.Is(err, vault.ErrClosed) {
		t.Fatalf("closed SetName: %v", err)
	}
}

func TestManifestDamage(t *testing.T) {
	v, dir, id := newVault(t)
	if err := v.SetName("Work"); err != nil {
		t.Fatal(err)
	}
	if err := putValue(v, "a", []byte("old")); err != nil {
		t.Fatal(err)
	}
	file := secretFiles(t, dir)[0]
	old, _ := os.ReadFile(file)
	if err := replaceValue(v, "a", []byte("new")); err != nil {
		t.Fatal(err)
	}
	other, otherDir, _ := newVault(t)
	if err := putValue(other, "b", nil); err != nil {
		t.Fatal(err)
	}
	otherFile := secretFiles(t, otherDir)[0]
	foreign, _ := os.ReadFile(otherFile)
	manifest, _ := os.ReadFile(filepath.Join(dir, "manifest"))
	otherManifest, _ := os.ReadFile(filepath.Join(otherDir, "manifest"))
	name, _ := os.ReadFile(filepath.Join(dir, "name"))
	cases := []struct {
		name                         string
		path                         string
		data                         []byte
		remove                       bool
		kind                         vault.ProblemKind
		missing, unexpected, changed int
	}{
		{"missing", file, nil, true, vault.ProblemFiles, 1, 0, 0},
		{"rollback file", file, old, false, vault.ProblemFiles, 0, 0, 1},
		{"foreign file", file, foreign, false, vault.ProblemFiles, 0, 0, 1},
		{"extra file", filepath.Join(dir, "secrets", filepath.Base(otherFile)), foreign, false, vault.ProblemFiles, 0, 1, 0},
		{"flipped file", file, flip(old, 100), false, vault.ProblemFiles, 0, 0, 1},
		{"changed name", filepath.Join(dir, "name"), flip(name, 40), false, vault.ProblemFiles, 0, 0, 1},
		{"header", filepath.Join(dir, "manifest"), flip(manifest, 0), false, vault.ProblemSignature, 0, 0, 0},
		{"entries", filepath.Join(dir, "manifest"), flip(manifest, 70), false, vault.ProblemSignature, 0, 0, 0},
		{"Ed25519", filepath.Join(dir, "manifest"), flip(manifest, len(manifest)-3309-64), false, vault.ProblemSignature, 0, 0, 0},
		{"ML-DSA", filepath.Join(dir, "manifest"), flip(manifest, len(manifest)-1), false, vault.ProblemSignature, 0, 0, 0},
		{"other scope", filepath.Join(dir, "manifest"), otherManifest, false, vault.ProblemSignature, 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := snapshot(t, dir)
			if tc.kind == vault.ProblemSignature && tc.name != "other scope" {
				if _, err := vault.VerifyManifest(id, tc.data); !errors.Is(err, vault.ErrIntegrity) {
					t.Fatalf("VerifyManifest corrupt %s: %v", tc.name, err)
				}
			}
			if tc.remove {
				if err := os.Remove(tc.path); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(tc.path, tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			damaged := snapshot(t, dir)
			for _, probe := range []func() error{func() error { _, e := rootTitles(v); return e }, func() error { _, e := readValue(v, "a"); return e }, func() error { _, e := vault.Open(dir, id); return e }} {
				err := probe()
				var p *vault.Problem
				if !errors.As(err, &p) || !errors.Is(err, vault.ErrIntegrity) || p.Kind != tc.kind || p.ScopeID != v.ID() || p.Missing != tc.missing || p.Unexpected != tc.unexpected || p.Changed != tc.changed {
					t.Fatalf("problem: %+v %v", p, err)
				}
				if got := snapshot(t, dir); !sameSnapshot(got, damaged) {
					t.Fatal("rejected damage changed files")
				}
			}
			for path := range damaged {
				if _, ok := before[path]; !ok {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
			}
			for path, data := range before {
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	_ = other
}

func TestManifestInterruptedChanges(t *testing.T) {
	for _, op := range []string{"put", "update", "delete", "name"} {
		for _, step := range []int{1, 2} {
			t.Run(op+string(rune('0'+step)), func(t *testing.T) {
				v, dir, id := newVault(t)
				if err := putValue(v, "a", []byte("old")); err != nil {
					t.Fatal(err)
				}
				old := snapshot(t, dir)
				oldManifest := old[filepath.Join(dir, "manifest")]
				var err error
				switch op {
				case "put":
					err = putValue(v, "b", []byte("new"))
				case "update":
					err = replaceValue(v, "a", []byte("new"))
				case "delete":
					err = removeValue(v, "a")
				case "name":
					err = v.SetName("New")
				}
				if err != nil {
					t.Fatal(err)
				}
				after := snapshot(t, dir)
				next := after[filepath.Join(dir, "manifest")]
				for path := range after {
					if _, ok := old[path]; !ok {
						if err := os.Remove(path); err != nil {
							t.Fatal(err)
						}
					}
				}
				for path, data := range old {
					if err := os.WriteFile(path, []byte(data), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.WriteFile(filepath.Join(dir, "manifest.next"), []byte(next), 0600); err != nil {
					t.Fatal(err)
				}
				if step == 2 {
					for path := range old {
						if _, ok := after[path]; !ok {
							if err := os.Remove(path); err != nil {
								t.Fatal(err)
							}
						}
					}
					for path, data := range after {
						if path != filepath.Join(dir, "manifest") {
							if err := os.WriteFile(path, []byte(data), 0600); err != nil {
								t.Fatal(err)
							}
						}
					}
				}
				v.Close()
				opened, e := vault.Open(dir, id)
				if e != nil {
					t.Fatal(e)
				}
				defer opened.Close()
				want := oldManifest
				if step == 2 {
					want = next
				}
				if got := opened.ManifestHash(); got != sha256.Sum256([]byte(want)) {
					t.Fatal("wrong recovered generation")
				}
				if _, e := os.Stat(filepath.Join(dir, "manifest.next")); !errors.Is(e, os.ErrNotExist) {
					t.Fatalf("leftover next: %v", e)
				}
			})
		}
	}
}

func TestManifestInvalidNextIsNotApplied(t *testing.T) {
	v, dir, id := newVault(t)
	if err := putValue(v, "a", nil); err != nil {
		t.Fatal(err)
	}
	manifest, _ := os.ReadFile(filepath.Join(dir, "manifest"))
	for _, next := range [][]byte{[]byte("invalid"), manifest} {
		if err := os.WriteFile(filepath.Join(dir, "manifest.next"), next, 0600); err != nil {
			t.Fatal(err)
		}
		v.Close()
		opened, err := vault.Open(dir, id)
		if err != nil {
			t.Fatal(err)
		}
		v = opened
		if _, err := os.Stat(filepath.Join(dir, "manifest.next")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("invalid next retained: %v", err)
		}
	}
	v.Close()
}

func TestManifestForeignSigner(t *testing.T) {
	v, dir, _ := newVault(t)
	other := newIdentity(t, 2)
	raw, _ := os.ReadFile(filepath.Join(dir, "manifest"))
	unsigned := raw[:len(raw)-64-mldsa65.SignatureSize]
	foreign := bytes.Clone(raw)
	msg := append([]byte("qrypt/v1/vault/manifest"), unsigned...)
	copy(foreign[len(unsigned):], ed25519.Sign(other.Ed25519PrivateKey(), msg))
	if err := mldsa65.SignTo(other.MLDSA65PrivateKey(), msg, nil, false, foreign[len(unsigned)+64:]); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.VerifyManifest(other, foreign); err != nil {
		t.Fatalf("foreign signer must have valid signatures: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest"), foreign, 0600); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, dir)
	if _, err := vault.Open(dir, newIdentity(t, 1)); !errors.Is(err, vault.ErrIntegrity) {
		t.Fatalf("foreign signer Open: %v", err)
	}
	if !sameSnapshot(before, snapshot(t, dir)) {
		t.Fatal("foreign signer changed files")
	}
	v.Close()
}

func flip(in []byte, n int) []byte { b := bytes.Clone(in); b[n] ^= 1; return b }
func sameSnapshot(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
