package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIconSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings")
	for _, set := range []string{"unicode", "emoji", "nerd"} {
		if err := Save(path, Settings{Language: "en", Icons: set}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		want := "language=en\n"
		if set != DefaultIcons {
			want += "icons=" + set + "\n"
		}
		if err != nil || string(data) != want {
			t.Fatalf("output %q: %v", data, err)
		}
		got, err := Load(path)
		if err != nil || got.Icons != set {
			t.Fatalf("roundtrip %+v: %v", got, err)
		}
	}
	for _, content := range []string{"language=en\nunknown=value\n", "language=en\nicons=invalid\nunknown=value\n"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if err != nil || got.Icons != DefaultIcons || got.Language != "en" {
			t.Fatalf("default %+v: %v", got, err)
		}
	}
	if err := Save(path, Settings{Icons: "bogus"}); err == nil {
		t.Fatal("invalid icons accepted")
	}
}

func TestMirrorSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings")
	got, err := Load(path)
	if err != nil || got.Mirror != "" {
		t.Fatalf("absent: %+v %v", got, err)
	}
	s := Settings{Language: "es", Theme: "green", Mirror: filepath.Join(t.TempDir(), "cópia con espacios")}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	want := "language=es\ntheme=green\nmirror=" + s.Mirror + "\n"
	if err != nil || string(data) != want {
		t.Fatalf("saved %q: %v", data, err)
	}
	got, err = Load(path)
	if err != nil || got.Mirror != s.Mirror {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if err := os.WriteFile(path, []byte("theme=green\nmirror=\nunknown=ignored\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = Load(path)
	if err != nil || got.Mirror != "" || got.Theme != "green" {
		t.Fatalf("empty: %+v %v", got, err)
	}
	s.Mirror = ""
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil || string(data) != "language=es\ntheme=green\n" {
		t.Fatalf("removed: %q %v", data, err)
	}
}

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

// A theme that an older version saved and this one removed is read as green,
// together with the rest of the file.
func TestLoadRetiredThemeIsGreen(t *testing.T) {
	for _, name := range []string{"violet", "mono", "contrast", "auto"} {
		path := filepath.Join(t.TempDir(), "settings")
		if err := os.WriteFile(path, []byte("language=es\ntheme="+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if err != nil || got.Theme != "green" || got.Language != "es" {
			t.Errorf("%s: %+v, %v", name, got, err)
		}
		if err := os.WriteFile(path, []byte("theme="+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got, err := Load(path); err != nil || got.Theme != "green" {
			t.Errorf("%s alone: %+v, %v", name, got, err)
		}
	}
	for _, name := range []string{"green", "carbon", "custom"} {
		path := filepath.Join(t.TempDir(), "settings")
		if err := os.WriteFile(path, []byte("theme="+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got, err := Load(path); err != nil || got.Theme != name {
			t.Errorf("%s: %+v, %v", name, got, err)
		}
	}
}
