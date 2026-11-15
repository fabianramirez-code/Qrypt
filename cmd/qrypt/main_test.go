package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	if lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n"); len(lines) != 1 || !strings.Contains(lines[0], "interactive terminal") || !strings.Contains(lines[0], "Windows Terminal") {
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
