package identity_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/hpke"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
	"github.com/fabianramirez-code/qrypt/internal/identity"
)

func seed() []byte {
	s := make([]byte, 32)
	for i := range s {
		s[i] = byte(i)
	}
	return s
}
func mustID(t *testing.T, s []byte) *identity.Identity {
	t.Helper()
	id, e := identity.FromSeed(s)
	if e != nil {
		t.Fatal(e)
	}
	return id
}
func hashes(p identity.PublicKeys) [3]string {
	out := [3]string{}
	for i, b := range [][]byte{p.KEM, p.Ed25519, p.MLDSA65} {
		h := sha256.Sum256(b)
		out[i] = hex.EncodeToString(h[:])
	}
	return out
}
func same(t *testing.T, a, b identity.PublicKeys) {
	t.Helper()
	if hashes(a) != hashes(b) {
		t.Fatal("public keys differ")
	}
}

func TestGenerateAndDerive(t *testing.T) {
	a, e := identity.Generate()
	if e != nil {
		t.Fatal(e)
	}
	b, e := identity.Generate()
	if e != nil {
		t.Fatal(e)
	}
	if hashes(a.PublicKeys()) == hashes(b.PublicKeys()) {
		t.Fatal("random identities match")
	}
	s := seed()
	id := mustID(t, s)
	same(t, id.PublicKeys(), mustID(t, s).PublicKeys())
	p := id.PublicKeys()
	if len(p.KEM) != 1216 || len(p.Ed25519) != 32 || len(p.MLDSA65) != 1952 {
		t.Fatalf("key sizes: %d %d %d", len(p.KEM), len(p.Ed25519), len(p.MLDSA65))
	}
	want := [3]string{"0471f26269ade8aa0e780d32cdbe747e59ef72cd4b90779d8761d2bc4e4e22f2", "c8722e4524b419e6c7ab7ee37ee5662263160e9e483cf3ab9350de92f4297ccc", "bb5d374ac31ba7f37544a5f87e68732948d6a272f3bfba3f648b4a8dbfc0bbd7"}
	if got := hashes(p); got != want {
		t.Fatalf("public-key hashes: got %v, want %v", got, want)
	}
	for i := range s {
		s[i] = 255
	}
	same(t, p, id.PublicKeys())
	p.KEM[0] ^= 1
	p.Ed25519[0] ^= 1
	p.MLDSA65[0] ^= 1
	same(t, id.PublicKeys(), mustID(t, seed()).PublicKeys())
	for _, n := range []int{0, 31, 33} {
		if _, e := identity.FromSeed(make([]byte, n)); !errors.Is(e, identity.ErrInvalidSeed) {
			t.Fatalf("length %d: %v", n, e)
		}
	}
}

func TestDerivedKeysWork(t *testing.T) {
	id := mustID(t, seed())
	p := id.PublicKeys()
	kemPub, e := hpke.MLKEM768X25519().NewPublicKey(p.KEM)
	if e != nil {
		t.Fatal(e)
	}
	derived, e := id.KEMPrivateKey().Bytes()
	if e != nil {
		t.Fatal(e)
	}
	expected, e := hkdf.Key(sha256.New, seed(), nil, "qrypt/v1/identity/kem/mlkem768-x25519", 32)
	if e != nil {
		t.Fatalf("derive X-Wing HKDF seed: %v", e)
	}
	if !bytes.Equal(derived, expected) {
		t.Fatal("X-Wing private key does not match HKDF seed")
	}
	recreated, e := hpke.MLKEM768X25519().NewPrivateKey(expected)
	if e != nil {
		t.Fatal(e)
	}
	serialized, e := recreated.Bytes()
	if e != nil || !bytes.Equal(serialized, expected) || len(serialized) != 32 {
		t.Fatalf("X-Wing private key does not round-trip: %v", e)
	}
	clear(expected)
	clear(derived)
	clear(serialized)
	msg := []byte("identity test")
	ct, e := hpke.Seal(kemPub, hpke.HKDFSHA256(), hpke.AES128GCM(), nil, msg)
	if e != nil {
		t.Fatal(e)
	}
	pt, e := hpke.Open(id.KEMPrivateKey(), hpke.HKDFSHA256(), hpke.AES128GCM(), nil, ct)
	if e != nil || !bytes.Equal(pt, msg) {
		t.Fatalf("KEM open: %v", e)
	}
	sig := ed25519.Sign(id.Ed25519PrivateKey(), msg)
	if !ed25519.Verify(ed25519.PublicKey(p.Ed25519), msg, sig) {
		t.Fatal("ed25519")
	}
	var pk mldsa65.PublicKey
	var raw [mldsa65.PublicKeySize]byte
	copy(raw[:], p.MLDSA65)
	pk.Unpack(&raw)
	sig = make([]byte, mldsa65.SignatureSize)
	if e := mldsa65.SignTo(id.MLDSA65PrivateKey(), msg, nil, false, sig); e != nil || !mldsa65.Verify(&pk, msg, nil, sig) {
		t.Fatalf("ML-DSA verify: %v", e)
	}
}

func TestRecovery(t *testing.T) {
	vectors := []struct {
		value  byte
		phrase string
	}{
		{0, "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon art"},
		{0x7f, "legal winner thank year wave sausage worth useful legal winner thank year wave sausage worth useful legal winner thank year wave sausage worth title"},
		{0x80, "letter advice cage absurd amount doctor acoustic avoid letter advice cage absurd amount doctor acoustic avoid letter advice cage absurd amount doctor acoustic bless"},
		{0xff, "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo vote"},
	}
	for _, v := range vectors {
		t.Run(v.phrase[:strings.IndexByte(v.phrase, ' ')], func(t *testing.T) {
			id := mustID(t, bytes.Repeat([]byte{v.value}, 32))
			if got := id.RecoveryPhrase(); got != v.phrase {
				t.Fatalf("phrase mismatch: %q", got)
			}
			back, e := identity.FromRecoveryPhrase(v.phrase)
			if e != nil {
				t.Fatal(e)
			}
			same(t, id.PublicKeys(), back.PublicKeys())
		})
	}
	id := mustID(t, seed())
	back, e := identity.FromRecoveryPhrase(id.RecoveryPhrase())
	if e != nil {
		t.Fatal(e)
	}
	same(t, id.PublicKeys(), back.PublicKeys())
	words := strings.Fields(id.RecoveryPhrase())
	mixed := " \t" + strings.ToUpper(strings.Join(words[:12], "  \n")) + "\n\t" + strings.Join(words[12:], "\t") + " \n"
	back, e = identity.FromRecoveryPhrase(mixed)
	if e != nil {
		t.Fatal(e)
	}
	same(t, id.PublicKeys(), back.PublicKeys())
	cases := []struct {
		phrase string
		want   error
	}{{"word", identity.ErrWordCount}, {strings.Replace(id.RecoveryPhrase(), words[3], "notaword", 1), identity.ErrUnknownWord}, {strings.Join(append(words[:23], "abandon"), " "), identity.ErrChecksum}}
	for _, tc := range cases {
		_, e := identity.FromRecoveryPhrase(tc.phrase)
		if !errors.Is(e, tc.want) {
			t.Fatalf("expected %v, got %v", tc.want, e)
		}
		for _, w := range strings.Fields(tc.phrase) {
			if strings.Contains(e.Error(), w) {
				t.Fatalf("error leaks word: %q", e)
			}
		}
	}
}

func TestSealOpen(t *testing.T) {
	id := mustID(t, seed())
	if _, e := id.Seal(nil); !errors.Is(e, identity.ErrEmptyPassphrase) {
		t.Fatalf("empty passphrase: %v", e)
	}
	pass := []byte("test-passphrase")
	start := time.Now()
	a, e := id.Seal(pass)
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("one Seal: %s", time.Since(start))
	b, e := id.Seal(pass)
	if e != nil {
		t.Fatal(e)
	}
	if len(a) != 96 || string(a[:7]) != "QRYPTID" || a[7] != 1 {
		t.Fatal("layout")
	}
	for _, span := range [][2]int{{8, 24}, {24, 48}, {48, 96}} {
		if bytes.Equal(a[span[0]:span[1]], b[span[0]:span[1]]) {
			t.Fatalf("reused random field: %v", span)
		}
	}
	opened, e := identity.Open(a, pass)
	if e != nil {
		t.Fatal(e)
	}
	same(t, id.PublicKeys(), opened.PublicKeys())
	if _, e := identity.Open(a, nil); !errors.Is(e, identity.ErrEmptyPassphrase) {
		t.Fatalf("empty passphrase: %v", e)
	}
	if _, e := identity.Open(a, []byte("wrong")); !errors.Is(e, identity.ErrDecryption) {
		t.Fatalf("wrong passphrase: %v", e)
	}
	for _, offset := range []int{8, 24, 48} {
		bad := bytes.Clone(a)
		bad[offset] ^= 1
		if _, e := identity.Open(bad, pass); !errors.Is(e, identity.ErrDecryption) {
			t.Fatalf("tamper %d: %v", offset, e)
		}
	}
	for _, n := range []int{0, 95, 97} {
		if _, e := identity.Open(make([]byte, n), pass); !errors.Is(e, identity.ErrInvalidFormat) {
			t.Fatalf("length %d: %v", n, e)
		}
	}
	bad := bytes.Clone(a)
	bad[0] ^= 1
	if _, e := identity.Open(bad, pass); !errors.Is(e, identity.ErrInvalidFormat) {
		t.Fatal(e)
	}
	bad = bytes.Clone(a)
	bad[7] = 2
	if _, e := identity.Open(bad, pass); !errors.Is(e, identity.ErrUnsupportedVersion) {
		t.Fatal(e)
	}
}

func TestStorage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "identity")
	content, err := hex.DecodeString(goldenFile)
	if err != nil {
		t.Fatal(err)
	}
	if e := identity.Save(path, content); e != nil {
		t.Fatal(e)
	}
	checkOnlyIdentity(t, path)
	got, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(got, content) {
		t.Fatalf("saved: %v", e)
	}
	other := bytes.Clone(content)
	other[8] ^= 1
	if e := identity.Save(path, other); !errors.Is(e, identity.ErrAlreadyExists) {
		t.Fatalf("overwrite: %v", e)
	}
	checkOnlyIdentity(t, path)
	got, e = os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(got, content) {
		t.Fatal("overwritten")
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(path)
		if fi.Mode().Perm() != 0600 {
			t.Fatal("file mode")
		}
		fi, _ = os.Stat(filepath.Dir(path))
		if fi.Mode().Perm() != 0700 {
			t.Fatal("directory mode")
		}
	}
	blocked := filepath.Join(t.TempDir(), "blocking-file")
	if e := os.WriteFile(blocked, []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := identity.Save(filepath.Join(blocked, "child"), content); e == nil {
		t.Fatal("expected failure")
	}
	got, _ = os.ReadFile(blocked)
	if string(got) != "keep" {
		t.Fatal("failed save modified existing file")
	}
	if _, e := os.Stat(filepath.Join(blocked, "child")); !errors.Is(e, fs.ErrNotExist) {
		t.Fatalf("failed save left a target: %v", e)
	}
	for name, invalid := range map[string][]byte{
		"short":       content[:95],
		"wrong magic": append([]byte("BADMAGC"), content[7:]...),
	} {
		t.Run(name, func(t *testing.T) {
			parent := filepath.Join(t.TempDir(), "uncreated")
			file := filepath.Join(parent, "identity")
			if err := identity.Save(file, invalid); !errors.Is(err, identity.ErrInvalidFormat) {
				t.Fatalf("Save invalid data: %v", err)
			}
			for _, path := range []string{file, parent} {
				if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("created %s: %v", path, err)
				}
			}
		})
	}
	t.Run("unsupported version", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "uncreated")
		path := filepath.Join(parent, "identity")
		invalid := bytes.Clone(content)
		invalid[7] = 0x02
		if err := identity.Save(path, invalid); !errors.Is(err, identity.ErrUnsupportedVersion) {
			t.Fatalf("Save unsupported version: %v", err)
		}
		for _, p := range []string{path, parent} {
			if _, err := os.Stat(p); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("created %s: %v", p, err)
			}
		}
	})
	if _, e := identity.Load(filepath.Join(t.TempDir(), "absent"), []byte("pass")); !errors.Is(e, fs.ErrNotExist) {
		t.Fatalf("missing: %v", e)
	}
	config, e := os.UserConfigDir()
	if e != nil {
		t.Fatal(e)
	}
	actual, e := identity.DefaultPath()
	if e != nil || actual != filepath.Join(config, "qrypt", "identity") {
		t.Fatalf("default path: %v", e)
	}
}

func checkOnlyIdentity(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("directory entries: %v, want only identity", entries)
	}
}

func TestSaveConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "identity")
	const workers = 8
	var data [workers][]byte
	var results [workers]error
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range data {
		data[i] = make([]byte, 96)
		copy(data[i], "QRYPTID")
		data[i][7] = 0x01
		data[i][8] = byte(i + 1)
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = identity.Save(path, data[i])
		}(i)
	}
	close(start)
	wg.Wait()
	winner := -1
	for i, err := range results {
		if err == nil {
			if winner != -1 {
				t.Fatalf("multiple successful saves: %d and %d", winner, i)
			}
			winner = i
		} else if !errors.Is(err, identity.ErrAlreadyExists) {
			t.Fatalf("save %d: %v, want ErrAlreadyExists", i, err)
		}
	}
	if winner == -1 {
		t.Fatal("no successful save")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data[winner]) {
		t.Fatal("saved bytes differ from successful caller")
	}
	checkOnlyIdentity(t, path)
}

//go:embed bip39_english.txt
var embeddedWords []byte

func TestOfficialWordList(t *testing.T) {
	list := strings.Fields(string(embeddedWords))
	if len(list) != 2048 {
		t.Fatalf("words: %d", len(list))
	}
	seen := make(map[string]bool)
	for _, word := range list {
		if seen[word] {
			t.Fatalf("duplicate word: %q", word)
		}
		seen[word] = true
	}
	canonical := strings.Join(list, "\n") + "\n"
	crlf := strings.Join(strings.Fields(strings.ReplaceAll(canonical, "\n", "\r\n")), "\n") + "\n"
	if crlf != canonical {
		t.Fatal("CRLF word-list parsing changed the canonical form")
	}
	sum := sha256.Sum256([]byte(canonical))
	if hex.EncodeToString(sum[:]) != "2f5eed53a4727b4bf8880d8f3f199efc90e58503646d9ff8eff3a2ed3b24dbda" {
		t.Fatal("official BIP39 word list hash differs")
	}
}

func TestLoadRoundTrip(t *testing.T) {
	id := mustID(t, seed())
	pass := []byte("test-passphrase")
	data, e := id.Seal(pass)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "identity")
	if e = identity.Save(path, data); e != nil {
		t.Fatal(e)
	}
	got, e := identity.Load(path, pass)
	if e != nil {
		t.Fatal(e)
	}
	same(t, id.PublicKeys(), got.PublicKeys())
	for _, n := range []int{95, 97, 1 << 20} {
		p := filepath.Join(t.TempDir(), "invalid")
		if e := os.WriteFile(p, make([]byte, n), 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := identity.Load(p, pass); !errors.Is(e, identity.ErrInvalidFormat) {
			t.Fatalf("length %d: %v", n, e)
		}
	}
}
