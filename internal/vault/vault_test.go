package vault_test

import (
	"bytes"
	"crypto/hkdf"
	"crypto/hpke"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/vault"
	"golang.org/x/crypto/chacha20poly1305"
)

func newIdentity(t *testing.T, fill byte) *identity.Identity {
	t.Helper()
	seed := bytes.Repeat([]byte{fill}, 32)
	id, err := identity.FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newVault(t *testing.T) (*vault.Vault, string, *identity.Identity) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "vault")
	id := newIdentity(t, 1)
	v, err := vault.Create(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	return v, dir, id
}

func secretFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, filepath.Join(dir, "secrets", e.Name()))
	}
	return out
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		out[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRoundTripAndReopen(t *testing.T) {
	v, dir, id := newVault(t)
	if err := v.Put("github token", []byte("ghp_secret")); err != nil {
		t.Fatal(err)
	}
	if err := v.Put("bank", []byte{}); err != nil {
		t.Fatal(err)
	}
	reopened, err := vault.Open(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.Get("github token")
	if err != nil || string(got) != "ghp_secret" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	empty, err := reopened.Get("bank")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty Get = %q, %v", empty, err)
	}
	names, err := reopened.List()
	if err != nil || !slices.Equal(names, []string{"bank", "github token"}) {
		t.Fatalf("List = %v, %v", names, err)
	}
}

func TestValueSizeBoundaries(t *testing.T) {
	v, _, _ := newVault(t)
	for _, size := range []int{0, 1, 251, 252, 253, 508, vault.MaxValueSize} {
		value := bytes.Repeat([]byte{0xab}, size)
		name := "v" + string(rune('a'+size%26)) + hex.EncodeToString([]byte{byte(size), byte(size >> 8), byte(size >> 16)})
		if err := v.Put(name, value); err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		got, err := v.Get(name)
		if err != nil || !bytes.Equal(got, value) {
			t.Fatalf("size %d: round trip failed: %v", size, err)
		}
	}
	if err := v.Put("too big", make([]byte, vault.MaxValueSize+1)); !errors.Is(err, vault.ErrValueTooLarge) {
		t.Fatalf("oversized value = %v", err)
	}
}

func TestInvalidNamesAreRejectedAndLeaveTheVaultUnchanged(t *testing.T) {
	v, dir, _ := newVault(t)
	if err := v.Put("keep", []byte("1")); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, dir)
	for _, name := range []string{"", strings.Repeat("a", vault.MaxNameSize+1), "bad\x00name", "line\nbreak", "\xff\xfe"} {
		if err := v.Put(name, []byte("x")); !errors.Is(err, vault.ErrInvalidName) {
			t.Fatalf("Put(%q) = %v", name, err)
		}
	}
	if err := v.Put(strings.Repeat("é", 127), []byte("x")); err != nil {
		t.Fatalf("255-byte name rejected: %v", err)
	}
	if err := v.Put("keep", []byte("2")); !errors.Is(err, vault.ErrNameExists) {
		t.Fatalf("duplicate Put = %v", err)
	}
	delete(before, "")
	after := snapshot(t, dir)
	for path, content := range before {
		if after[path] != content {
			t.Fatalf("%s changed after rejected input", path)
		}
	}
	if len(after) != len(before)+1 {
		t.Fatalf("rejected input left files: %d before, %d after", len(before), len(after))
	}
}

func TestUpdateKeepsFileNameAndChangesBytes(t *testing.T) {
	v, dir, _ := newVault(t)
	if err := v.Put("api", []byte("old")); err != nil {
		t.Fatal(err)
	}
	files := secretFiles(t, dir)
	oldBytes, _ := os.ReadFile(files[0])
	if err := v.Update("api", []byte("new value")); err != nil {
		t.Fatal(err)
	}
	if after := secretFiles(t, dir); !slices.Equal(after, files) {
		t.Fatalf("files = %v, want %v", after, files)
	}
	newBytes, _ := os.ReadFile(files[0])
	if bytes.Equal(oldBytes, newBytes) || bytes.Equal(oldBytes[8:32], newBytes[8:32]) {
		t.Fatal("update must write new nonces and ciphertext")
	}
	if got, err := v.Get("api"); err != nil || string(got) != "new value" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if err := v.Update("missing", []byte("x")); !errors.Is(err, vault.ErrNotFound) {
		t.Fatalf("Update missing = %v", err)
	}
}

func TestDelete(t *testing.T) {
	v, dir, _ := newVault(t)
	_ = v.Put("a", []byte("1"))
	_ = v.Put("b", []byte("2"))
	if err := v.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if len(secretFiles(t, dir)) != 1 {
		t.Fatal("delete must remove exactly one file")
	}
	if _, err := v.Get("a"); !errors.Is(err, vault.ErrNotFound) {
		t.Fatalf("Get deleted = %v", err)
	}
	if err := v.Delete("a"); !errors.Is(err, vault.ErrNotFound) {
		t.Fatalf("Delete twice = %v", err)
	}
	if got, err := v.Get("b"); err != nil || string(got) != "2" {
		t.Fatalf("other secret = %q, %v", got, err)
	}
}

func TestFilesRevealNothingReadable(t *testing.T) {
	v, dir, _ := newVault(t)
	if err := v.Put("super visible name", []byte("super visible value")); err != nil {
		t.Fatal(err)
	}
	for path, content := range snapshot(t, dir) {
		if strings.Contains(path, "visible") || strings.Contains(content, "visible") {
			t.Fatalf("%s reveals a name or value", path)
		}
	}
	file := secretFiles(t, dir)[0]
	base := filepath.Base(file)
	if len(base) != 32+len(".qrs") || !strings.HasSuffix(base, ".qrs") {
		t.Fatalf("file name %q is not 32 hex characters and .qrs", base)
	}
	if _, err := hex.DecodeString(strings.TrimSuffix(base, ".qrs")); err != nil {
		t.Fatalf("file name is not hex: %v", err)
	}
}

func TestListDoesNotNeedValues(t *testing.T) {
	v, dir, _ := newVault(t)
	_ = v.Put("a", []byte("1"))
	file := secretFiles(t, dir)[0]
	data, _ := os.ReadFile(file)
	data[len(data)-1] ^= 1
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if names, err := v.List(); err != nil || !slices.Equal(names, []string{"a"}) {
		t.Fatalf("List with damaged value = %v, %v", names, err)
	}
	if _, err := v.Get("a"); !errors.Is(err, vault.ErrDecryption) {
		t.Fatalf("Get with damaged value = %v", err)
	}
}

func TestTamperingIsDetected(t *testing.T) {
	v, dir, _ := newVault(t)
	_ = v.Put("a", []byte("1"))
	file := secretFiles(t, dir)[0]
	original, _ := os.ReadFile(file)
	for _, offset := range []int{8, 40, 304, 340, len(original) - 1} {
		data := bytes.Clone(original)
		data[offset] ^= 1
		if err := os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
		_, listErr := v.List()
		_, getErr := v.Get("a")
		if listErr == nil && getErr == nil {
			t.Fatalf("flipping byte %d went unnoticed", offset)
		}
	}
	for _, data := range [][]byte{original[:len(original)-1], append(bytes.Clone(original), 0), original[:10], {}} {
		_ = os.WriteFile(file, data, 0600)
		if _, err := v.List(); !errors.Is(err, vault.ErrInvalidFormat) {
			t.Fatalf("size %d: List = %v", len(data), err)
		}
	}
	data := bytes.Clone(original)
	data[7] = 2
	_ = os.WriteFile(file, data, 0600)
	if _, err := v.List(); !errors.Is(err, vault.ErrUnsupportedVersion) {
		t.Fatalf("unknown version: %v", err)
	}
}

func TestFileIsBoundToItsNameScopeAndRole(t *testing.T) {
	v, dir, _ := newVault(t)
	_ = v.Put("a", []byte("1"))
	file := secretFiles(t, dir)[0]
	data, _ := os.ReadFile(file)

	renamed := filepath.Join(filepath.Dir(file), strings.Repeat("0", 32)+".qrs")
	if err := os.Rename(file, renamed); err != nil {
		t.Fatal(err)
	}
	if _, err := v.List(); !errors.Is(err, vault.ErrDecryption) {
		t.Fatalf("renamed file = %v", err)
	}
	_ = os.Remove(renamed)

	otherDir := filepath.Join(t.TempDir(), "other")
	other, err := vault.Create(otherDir, newIdentity(t, 1))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := os.WriteFile(filepath.Join(otherDir, "secrets", filepath.Base(file)), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := other.List(); !errors.Is(err, vault.ErrDecryption) {
		t.Fatalf("file from another scope = %v", err)
	}

	// Swapping the name segment for the value segment's role must fail.
	_ = os.WriteFile(file, data, 0600)
	swapped := bytes.Clone(data)
	copy(swapped[8:32], data[304:328])
	_ = os.WriteFile(file, swapped, 0600)
	if _, err := v.List(); !errors.Is(err, vault.ErrDecryption) {
		t.Fatalf("swapped nonce = %v", err)
	}
}

func TestOpenRejectsWrongIdentityAndBadScopeFile(t *testing.T) {
	_, dir, id := newVault(t)
	if _, err := vault.Open(dir, newIdentity(t, 2)); !errors.Is(err, vault.ErrDecryption) {
		t.Fatalf("wrong identity = %v", err)
	}
	scope := filepath.Join(dir, "scope")
	original, _ := os.ReadFile(scope)
	if len(original) != 1225 {
		t.Fatalf("scope file is %d bytes, want 1225", len(original))
	}
	for _, offset := range []int{8, 30, 60, 1224} {
		data := bytes.Clone(original)
		data[offset] ^= 1
		_ = os.WriteFile(scope, data, 0600)
		if _, err := vault.Open(dir, id); !errors.Is(err, vault.ErrDecryption) {
			t.Fatalf("flipping scope byte %d = %v", offset, err)
		}
	}
	for _, data := range [][]byte{original[:1224], append(bytes.Clone(original), 0)} {
		_ = os.WriteFile(scope, data, 0600)
		if _, err := vault.Open(dir, id); !errors.Is(err, vault.ErrInvalidFormat) {
			t.Fatalf("size %d = %v", len(data), err)
		}
	}
	bad := bytes.Clone(original)
	bad[7] = 9
	_ = os.WriteFile(scope, bad, 0600)
	if _, err := vault.Open(dir, id); !errors.Is(err, vault.ErrUnsupportedVersion) {
		t.Fatalf("version = %v", err)
	}
	bad = bytes.Clone(original)
	bad[24] = 2
	_ = os.WriteFile(scope, bad, 0600)
	if _, err := vault.Open(dir, id); !errors.Is(err, vault.ErrInvalidFormat) {
		t.Fatalf("member count = %v", err)
	}
	if _, err := vault.Open(filepath.Join(t.TempDir(), "nothing"), id); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing vault = %v", err)
	}
}

func TestCreateNeverOverwrites(t *testing.T) {
	_, dir, id := newVault(t)
	before, _ := os.ReadFile(filepath.Join(dir, "scope"))
	if _, err := vault.Create(dir, id); !errors.Is(err, vault.ErrAlreadyExists) {
		t.Fatalf("second Create = %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "scope"))
	if !bytes.Equal(before, after) {
		t.Fatal("scope file changed")
	}
}

func TestTemporaryFilesAreIgnoredAndNoneAreLeft(t *testing.T) {
	v, dir, _ := newVault(t)
	_ = v.Put("a", []byte("1"))
	_ = v.Update("a", []byte("2"))
	for _, path := range secretFiles(t, dir) {
		if strings.HasSuffix(path, ".tmp") {
			t.Fatalf("temporary file left: %s", path)
		}
	}
	stray := filepath.Join(dir, "secrets", ".write-123.tmp")
	if err := os.WriteFile(stray, []byte("junk"), 0600); err != nil {
		t.Fatal(err)
	}
	if names, err := v.List(); err != nil || len(names) != 1 {
		t.Fatalf("List with stray temp = %v, %v", names, err)
	}
}

func TestClosedVaultRefusesWork(t *testing.T) {
	v, _, _ := newVault(t)
	v.Close()
	if _, err := v.List(); !errors.Is(err, vault.ErrClosed) {
		t.Fatalf("List after Close = %v", err)
	}
	if err := v.Put("a", nil); !errors.Is(err, vault.ErrClosed) {
		t.Fatalf("Put after Close = %v", err)
	}
}

// TestSpecConformance rebuilds both file kinds from docs/vault.md without
// sharing any layout or derivation code with the package.
func TestSpecConformance(t *testing.T) {
	v, dir, id := newVault(t)
	if err := v.Put("name", []byte("value")); err != nil {
		t.Fatal(err)
	}

	scope, _ := os.ReadFile(filepath.Join(dir, "scope"))
	if string(scope[:7]) != "QRYPTSK" || scope[7] != 1 || scope[24] != 1 {
		t.Fatal("scope header does not match the specification")
	}
	scopeID := scope[8:24]
	private := id.KEMPrivateKey()
	sum := sha256.Sum256(private.PublicKey().Bytes())
	if !bytes.Equal(scope[25:57], sum[:]) {
		t.Fatal("fingerprint is not SHA-256 of the X-Wing public key")
	}
	info := append([]byte("qrypt/v1/vault/scope-key"), scopeID...)
	scopeKey, err := hpke.Open(private, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), info, scope[57:])
	if err != nil || len(scopeKey) != 32 {
		t.Fatalf("scope key does not unwrap as specified: %v", err)
	}

	file, _ := os.ReadFile(secretFiles(t, dir)[0])
	fileID, _ := hex.DecodeString(strings.TrimSuffix(filepath.Base(secretFiles(t, dir)[0]), ".qrs"))
	if string(file[:7]) != "QRYPTSE" || file[7] != 1 {
		t.Fatal("secret header does not match the specification")
	}
	if len(file) != 332+256+16 || binary.BigEndian.Uint32(file[328:332]) != 256+16 {
		t.Fatalf("secret layout: size %d", len(file))
	}
	aad := func(segment byte) []byte {
		out := append([]byte("QRYPTSE\x01"), scopeID...)
		out = append(out, fileID...)
		return append(out, segment)
	}
	nameKey, _ := hkdf.Key(sha256.New, scopeKey, nil, "qrypt/v1/vault/secret/name", 32)
	valueKey, _ := hkdf.Key(sha256.New, scopeKey, nil, "qrypt/v1/vault/secret/value", 32)
	nameAEAD, _ := chacha20poly1305.NewX(nameKey)
	valueAEAD, _ := chacha20poly1305.NewX(valueKey)
	name, err := nameAEAD.Open(nil, file[8:32], file[32:304], aad(1))
	if err != nil {
		t.Fatalf("name segment: %v", err)
	}
	if name[0] != 4 || string(name[1:5]) != "name" || !bytes.Equal(name[5:], make([]byte, 251)) {
		t.Fatal("name padding does not match the specification")
	}
	value, err := valueAEAD.Open(nil, file[304:328], file[332:], aad(2))
	if err != nil {
		t.Fatalf("value segment: %v", err)
	}
	if binary.BigEndian.Uint32(value) != 5 || string(value[4:9]) != "value" ||
		!bytes.Equal(value[9:], make([]byte, 247)) {
		t.Fatal("value padding does not match the specification")
	}
}
