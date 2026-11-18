// Package settings stores non-secret interface preferences in an owner-only text file.
package settings

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const maxSize = 4 * 1024

// Settings contains the chosen language, theme, and custom role colors.
type Settings struct {
	Language string
	Theme    string
	Icons    string
	Mirror   string
	Colors   map[string]string
}

// DefaultPath returns the settings file location without creating it.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "qrypt", "settings"), nil
}

func validLanguage(s string) bool { return s == "en" || s == "es" }
func validTheme(s string) bool {
	switch s {
	case "green", "carbon", "custom":
		return true
	}
	return false
}

// retiredTheme reports a theme name that older versions saved and this one no
// longer has; Load reads it as green.
func retiredTheme(s string) bool {
	switch s {
	case "violet", "mono", "contrast", "auto":
		return true
	}
	return false
}

// DefaultIcons is the icon set used when the settings file names none: folders
// drawn as emoji, which need no extra font.
const DefaultIcons = "emoji"

func validIcons(s string) bool { return s == "unicode" || s == "emoji" || s == "nerd" }
func validRole(s string) bool {
	switch s {
	case "accent", "text", "dim", "success", "error", "warning", "border":
		return true
	}
	return false
}
func validHex(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, b := range []byte(s[1:]) {
		if b < '0' || b > '9' {
			if b < 'a' || b > 'f' {
				return false
			}
		}
	}
	return true
}

// Load reads at most 4 KiB. Invalid lines are skipped; a file with content and
// no valid line is an error.
func Load(path string) (Settings, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return Settings{}, err
	}
	if len(data) > maxSize || !utf8.Valid(data) {
		return Settings{}, fmt.Errorf("invalid settings file")
	}
	s := Settings{Icons: DefaultIcons}
	// Save writes nothing for default values, so an empty file is a valid file
	// that holds only defaults.
	valid := len(data) == 0
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch {
		case key == "language" && validLanguage(value):
			s.Language = value
			valid = true
		case key == "theme" && validTheme(value):
			s.Theme = value
			valid = true
		case key == "theme" && retiredTheme(value):
			s.Theme = "green"
			valid = true
		case key == "icons" && validIcons(value):
			s.Icons = value
			valid = true
		case key == "mirror":
			if value != "" && !strings.ContainsAny(value, "\r\x00") {
				s.Mirror = value
			}
			valid = true
		case strings.HasPrefix(key, "color.") && validRole(strings.TrimPrefix(key, "color.")) && validHex(value):
			if s.Colors == nil {
				s.Colors = make(map[string]string)
			}
			s.Colors[strings.TrimPrefix(key, "color.")] = value
			valid = true
		}
	}
	if !valid {
		return Settings{}, fmt.Errorf("invalid settings file")
	}
	return s, nil
}

// Save syncs an owner-only temporary file then atomically replaces path.
func Save(path string, s Settings) error {
	if s.Language != "" && !validLanguage(s.Language) {
		return fmt.Errorf("invalid language")
	}
	if s.Theme != "" && !validTheme(s.Theme) {
		return fmt.Errorf("invalid theme")
	}
	if s.Icons != "" && !validIcons(s.Icons) {
		return fmt.Errorf("invalid icons")
	}
	if strings.ContainsAny(s.Mirror, "\r\n\x00") {
		return fmt.Errorf("invalid mirror path")
	}
	keys := make([]string, 0, len(s.Colors))
	for role, hex := range s.Colors {
		if !validRole(role) || !validHex(hex) {
			return fmt.Errorf("invalid color %q", role)
		}
		keys = append(keys, role)
	}
	sort.Strings(keys)
	var b strings.Builder
	if s.Language != "" {
		fmt.Fprintf(&b, "language=%s\n", s.Language)
	}
	if s.Theme != "" {
		fmt.Fprintf(&b, "theme=%s\n", s.Theme)
	}
	if s.Icons != "" && s.Icons != DefaultIcons {
		fmt.Fprintf(&b, "icons=%s\n", s.Icons)
	}
	if s.Mirror != "" {
		fmt.Fprintf(&b, "mirror=%s\n", s.Mirror)
	}
	for _, role := range keys {
		fmt.Fprintf(&b, "color.%s=%s\n", role, s.Colors[role])
	}
	if b.Len() > maxSize {
		return fmt.Errorf("settings file too large")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	_, writeErr := io.WriteString(f, b.String())
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
