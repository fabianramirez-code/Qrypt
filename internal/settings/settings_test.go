package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCarbonRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings")
	if !validTheme("carbon") {
		t.Error("carbon not accepted")
	}
	if err := Save(path, Settings{Theme: "carbon"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "theme=carbon\n" {
		t.Fatalf("stored %q: %v", data, err)
	}
	got, err := Load(path)
	if err != nil || got.Theme != "carbon" {
		t.Fatalf("loaded %+v: %v", got, err)
	}
}

func TestLoadMissingAndInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings")
	if got, err := Load(path); err != nil || got.Language != "" || got.Theme != "" || got.Colors != nil {
		t.Fatalf("missing: %+v, %v", got, err)
	}
	for name, data := range map[string]string{
		"oversized": strings.Repeat("x", 4097),
		"invalid":   "language=fr\ntheme=broken\ncolor.accent=bad\n",
	} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if got, err := Load(path); err == nil || got.Language != "" || got.Theme != "" || got.Colors != nil {
			t.Errorf("%s: %+v, %v", name, got, err)
		}
	}
}

func TestLoadSkipsBadLinesAndUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings")
	data := "unknown=value\nmalformed\nlanguage=fr\nlanguage=es\ntheme=custom\ncolor.text=nothex\ncolor.accent=#22d3ee\ncolor.unknown=#112233\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Language != "es" || got.Theme != "custom" || len(got.Colors) != 1 || got.Colors["accent"] != "#22d3ee" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestSaveRoundTripAtomicAndDeterministic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "settings")
	s := Settings{Language: "es", Theme: "custom", Colors: map[string]string{"text": "#ffffff", "accent": "#22d3ee"}}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "language=es\ntheme=custom\ncolor.accent=#22d3ee\ncolor.text=#ffffff\n"
	if string(first) != want {
		t.Fatalf("got %q, want %q", first, want)
	}
	got, err := Load(path)
	if err != nil || got.Language != s.Language || got.Theme != s.Theme || len(got.Colors) != 2 || got.Colors["text"] != s.Colors["text"] || got.Colors["accent"] != s.Colors["accent"] {
		t.Fatalf("round trip: %+v, %v", got, err)
	}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil || string(second) != want {
		t.Fatalf("second: %q, %v", second, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 || entries[0].Name() != "settings" {
		t.Fatalf("entries: %v, %v", entries, err)
	}
	// Windows exposes only read-only/writeable permission bits through os.Stat.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("file mode: %v, %v", info, err)
		}
		info, err = os.Stat(filepath.Dir(path))
		if err != nil || info.Mode().Perm() != 0700 {
			t.Fatalf("dir mode: %v, %v", info, err)
		}
	}
}
