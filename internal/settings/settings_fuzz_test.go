package settings

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func FuzzLoad(f *testing.F) {
	valid := []byte("language=es\ntheme=carbon\nicons=nerd\nmirror=/tmp/mirror\ncolor.accent=#abcdef\n")
	for _, seed := range [][]byte{nil, valid, valid[:len(valid)-1], append(bytes.Clone(valid), 0), []byte("theme=violet\n"), []byte("theme=mono\n"), []byte("theme=contrast\n"), []byte("theme=auto\n"), []byte("icons=emoji\n"), []byte("invalid\n"), bytes.Repeat([]byte{'x'}, maxSize+1)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "settings")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		s, err := Load(path)
		if err != nil {
			return
		}
		if (s.Language != "" && !validLanguage(s.Language)) || (s.Theme != "" && !validTheme(s.Theme)) || !validIcons(s.Icons) || !validMotion(s.Motion) || strings.ContainsAny(s.Mirror, "\r\n\x00") {
			t.Fatalf("loaded invalid settings: %+v", s)
		}
		for role, hex := range s.Colors {
			if !validRole(role) || !validHex(hex) {
				t.Fatalf("loaded invalid color %q=%q", role, hex)
			}
		}
		if err := Save(path, s); err != nil {
			t.Fatalf("save loaded settings: %v", err)
		}
		again, err := Load(path)
		if err != nil || !reflect.DeepEqual(s, again) {
			t.Fatalf("settings round trip: %+v, %+v, %v", s, again, err)
		}
	})
}
