package identity

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const goldenFile = "5152595054494401a0a1a2a3a4a5a6a7a8a9aaabacadaeafb0b1b2b3b4b5b6b7b8b9babbbcbdbebfc0c1c2c3c4c5c6c713861e71bae6ac5a4e8415e95078abf7e129613fc8e67824dc680d6800b81cba46a3119dd4c4da2177bd1966edc0661b"

func TestInWordList(t *testing.T) {
	for _, tc := range []struct {
		word string
		want bool
	}{
		{"abandon", true}, {" Zoo ", true}, {"ZEBRA", true},
		{"", false}, {"abandonx", false}, {"aband", false},
		{"zoo zoo", false}, {"notaword", false},
	} {
		if got := InWordList(tc.word); got != tc.want {
			t.Errorf("InWordList(%q) = %t, want %t", tc.word, got, tc.want)
		}
	}
}

func replacementFixture(t *testing.T) ([]byte, []byte) {
	t.Helper()
	old, err := hex.DecodeString(goldenFile)
	if err != nil {
		t.Fatal(err)
	}
	newData := bytes.Clone(old)
	newData[8] ^= 1
	return old, newData
}

func checkStored(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("file %q: read error %v, content matches: %t", path, err, bytes.Equal(got, want))
	}
}

func checkEntries(t *testing.T, dir string, names ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(names) {
		t.Fatalf("entries %v, want %v", entries, names)
	}
	for i, entry := range entries {
		if entry.Name() != names[i] {
			t.Fatalf("entries %v, want %v", entries, names)
		}
	}
}

func fixedBackupTime(t *testing.T) string {
	t.Helper()
	previous := now
	now = func() time.Time { return time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC) }
	t.Cleanup(func() { now = previous })
	return ".backup-20260115T093000Z"
}

func TestSaveReplacingNew(t *testing.T) {
	_, content := replacementFixture(t)
	dir := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(dir, "identity")
	backup, err := SaveReplacing(path, content)
	if err != nil || backup != "" {
		t.Fatalf("backup %q, error %v", backup, err)
	}
	checkStored(t, path, content)
	checkEntries(t, dir, "identity")
	if runtime.GOOS != "windows" {
		info, err := os.Stat(dir)
		if err != nil || info.Mode().Perm() != 0700 {
			t.Fatalf("directory mode: %v, %v", info, err)
		}
	}
}

func TestSaveReplacingExisting(t *testing.T) {
	suffix := fixedBackupTime(t)
	old, content := replacementFixture(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "identity")
	if err := Save(path, old); err != nil {
		t.Fatal(err)
	}
	backup, err := SaveReplacing(path, content)
	if err != nil || backup != path+suffix {
		t.Fatalf("backup %q, error %v", backup, err)
	}
	checkStored(t, path, content)
	checkStored(t, backup, old)
	checkEntries(t, dir, "identity", "identity"+suffix)
	if runtime.GOOS != "windows" {
		for _, name := range []string{path, backup} {
			info, err := os.Stat(name)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("file mode: %v, %v", info, err)
			}
		}
	}
}

func TestSaveReplacingInvalid(t *testing.T) {
	old, content := replacementFixture(t)
	wrongMagic := bytes.Clone(content)
	wrongMagic[0] ^= 1
	wrongVersion := bytes.Clone(content)
	wrongVersion[7] = 2
	for _, tc := range []struct {
		name string
		data []byte
		want error
	}{
		{"short", content[:95], ErrInvalidFormat},
		{"magic", wrongMagic, ErrInvalidFormat},
		{"version", wrongVersion, ErrUnsupportedVersion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "identity")
			if err := Save(path, old); err != nil {
				t.Fatal(err)
			}
			backup, err := SaveReplacing(path, tc.data)
			if backup != "" || !errors.Is(err, tc.want) {
				t.Fatalf("backup %q, error %v, want %v", backup, err, tc.want)
			}
			checkStored(t, path, old)
			checkEntries(t, dir, "identity")
		})
	}
}

func TestSaveReplacingBackupExists(t *testing.T) {
	suffix := fixedBackupTime(t)
	old, content := replacementFixture(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "identity")
	if err := Save(path, old); err != nil {
		t.Fatal(err)
	}
	preexisting := []byte("existing backup")
	if err := os.WriteFile(path+suffix, preexisting, 0600); err != nil {
		t.Fatal(err)
	}
	backup, err := SaveReplacing(path, content)
	if backup != "" || !errors.Is(err, ErrBackupExists) {
		t.Fatalf("backup %q, error %v", backup, err)
	}
	checkStored(t, path, old)
	checkStored(t, path+suffix, preexisting)
	checkEntries(t, dir, "identity", "identity"+suffix)
}

func TestSaveReplacingRoundTrip(t *testing.T) {
	suffix := fixedBackupTime(t)
	seed := make([]byte, SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	id, err := FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	old, oldPass, err := id.Seal()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "identity")
	if err := Save(path, old); err != nil {
		t.Fatal(err)
	}
	content, newPass, err := id.Seal()
	if err != nil {
		t.Fatal(err)
	}
	backup, err := SaveReplacing(path, content)
	if err != nil || backup != path+suffix {
		t.Fatalf("backup %q, error %v", backup, err)
	}
	if _, err := Load(path, newPass); err != nil {
		t.Fatalf("load new: %v", err)
	}
	if _, err := Load(path, oldPass); !errors.Is(err, ErrDecryption) {
		t.Fatalf("old passphrase: %v", err)
	}
	if _, err := Load(backup, oldPass); err != nil {
		t.Fatalf("load backup: %v", err)
	}
}

func TestPassphraseGeneratorDistribution(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 8572; i++ {
		phrase, err := generatePassphrase()
		if err != nil {
			t.Fatal(err)
		}
		for _, word := range strings.Fields(string(phrase)) {
			seen[word] = true
		}
	}
	if len(seen) != 2048 {
		t.Fatalf("generator reached %d of 2048 words", len(seen))
	}
}

func TestGoldenFile(t *testing.T) {
	file, err := hex.DecodeString(goldenFile)
	if err != nil || len(file) != 96 {
		t.Fatalf("fixture: %v", err)
	}
	salt := make([]byte, 16)
	for i := range salt {
		salt[i] = byte(0xa0 + i)
	}
	if !bytes.Equal(file[8:24], salt) {
		t.Fatal("salt changed")
	}
	nonce := make([]byte, 24)
	for i := range nonce {
		nonce[i] = byte(0xb0 + i)
	}
	if !bytes.Equal(file[24:48], nonce) {
		t.Fatal("nonce changed")
	}
	id, err := Open(file, []byte("abandon ability able about above absent absorb"))
	if err != nil {
		t.Fatal(err)
	}
	keys := id.PublicKeys()
	want := [3]string{"0471f26269ade8aa0e780d32cdbe747e59ef72cd4b90779d8761d2bc4e4e22f2", "c8722e4524b419e6c7ab7ee37ee5662263160e9e483cf3ab9350de92f4297ccc", "bb5d374ac31ba7f37544a5f87e68732948d6a272f3bfba3f648b4a8dbfc0bbd7"}
	for i, key := range [][]byte{keys.KEM, keys.Ed25519, keys.MLDSA65} {
		sum := sha256.Sum256(key)
		if got := hex.EncodeToString(sum[:]); got != want[i] {
			t.Fatalf("key %d hash: got %s, want %s", i, got, want[i])
		}
	}
}
