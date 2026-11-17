package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/tui"
)

func TestArgumentsBeforeTerminalOrConfig(t *testing.T) {
	dir := t.TempDir()
	in, err := os.CreateTemp(dir, "in")
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.CreateTemp(dir, "out")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	for _, args := range [][]string{{"extra"}, {"--preview", "extra"}, {"--preview", "--preview"}} {
		var stderr bytes.Buffer
		if code := runArgs(in, out, &stderr, filepath.Join(dir, "real", "identity"), args); code != 2 || stderr.String() != "Usage: qrypt [--preview]\n" {
			t.Fatalf("args %v: code %d, stderr %q", args, code, stderr.String())
		}
		data, err := os.ReadFile(out.Name())
		if err != nil || len(data) != 0 {
			t.Fatalf("stdout %q: %v", data, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "real")); !os.IsNotExist(err) {
			t.Fatal("real path touched", err)
		}
	}
	var stderr bytes.Buffer
	if code := runArgs(in, out, &stderr, filepath.Join(dir, "real", "identity"), []string{"--preview"}); code != 1 || !strings.Contains(stderr.String(), "needs an interactive terminal") {
		t.Fatalf("preview without terminal: %d %q", code, stderr.String())
	}
}

func TestPreviewCleanupOnExitAndError(t *testing.T) {
	for _, fail := range []bool{false, true} {
		var temporary string
		code, runErr := runPreview(func(m tui.Model, path string) (tui.Model, error) {
			// This path is deliberately injected into the preview constructor, never the user's config.
			temporary = filepath.Dir(path)
			if _, err := os.Stat(temporary); err != nil {
				t.Fatal(err)
			}
			if fail {
				return m, os.ErrPermission
			}
			next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
			next, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			next, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
			return next.(tui.Model), nil
		})
		want := 0
		if fail {
			want = 1
		}
		if code != want || (runErr != nil) != fail {
			t.Fatalf("exit %d, error %v", code, runErr)
		}
		if temporary == "" {
			t.Fatal("runner not invoked")
		}
		if _, err := os.Stat(temporary); !os.IsNotExist(err) {
			t.Fatalf("temporary data remains: %v", err)
		}
	}
}

func TestPreviewSettingsStayInTemporaryDirectory(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "settings")
	if err := os.WriteFile(outside, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	var temporary string
	code, err := runPreview(func(m tui.Model, path string) (tui.Model, error) {
		temporary = filepath.Dir(path)
		next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
		next, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		next, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		next, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if _, err := os.Stat(filepath.Join(temporary, "settings")); err != nil {
			t.Errorf("preview settings not in temporary directory: %v", err)
		}
		return next.(tui.Model), nil
	})
	if err != nil || code != 0 {
		t.Fatalf("preview: code %d, error %v", code, err)
	}
	if _, err := os.Stat(temporary); !os.IsNotExist(err) {
		t.Fatalf("temporary directory remains: %v", err)
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "untouched" {
		t.Fatalf("outside settings changed: %q %v", data, err)
	}
}

func TestSystemLanguageTag(t *testing.T) {
	t.Logf("system language tag: %s", systemLanguageTag())
}

func TestPathFailure(t *testing.T) {
	got := pathFailure(os.ErrNotExist)
	want := "Qrypt could not find the configuration directory / Qrypt no encontró el directorio de configuración: file does not exist"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRefusal(t *testing.T) {
	dir := t.TempDir()
	in, err := os.CreateTemp(dir, "in")
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.CreateTemp(dir, "out")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	path := filepath.Join(dir, "config", "identity")
	var stderr bytes.Buffer
	if code := run(in, out, &stderr, path); code != 1 {
		t.Fatalf("exit status %d", code)
	}
	if lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n"); len(lines) != 1 || lines[0] != "Qrypt needs an interactive terminal (on Windows: Windows Terminal, PowerShell or cmd). / Qrypt necesita una terminal interactiva." {
		t.Fatalf("stderr %q", stderr.String())
	}
	data, err := os.ReadFile(out.Name())
	if err != nil || len(data) != 0 {
		t.Fatalf("stdout %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("directory exists: %v", err)
	}
}
