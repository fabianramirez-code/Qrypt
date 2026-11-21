package mirror

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type directory struct{ root string }

func Dir(path string) Storage { return directory{filepath.Clean(path)} }
func (d directory) path(name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return "", fmt.Errorf("invalid mirror path %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == ".." || part == "." || strings.Contains(part, ":") {
			return "", fmt.Errorf("invalid mirror path %q", name)
		}
	}
	current := d.root
	for _, part := range strings.Split(name, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("mirror path is a symlink: %q", name)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return current, nil
}
func (d directory) List() ([]string, error) {
	out := []string{}
	err := filepath.WalkDir(d.root, func(path string, entry fs.DirEntry, e error) error {
		if errors.Is(e, os.ErrNotExist) && path == d.root {
			return nil
		}
		if e != nil {
			return e
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".mirror-") {
			return nil
		}
		rel, e := filepath.Rel(d.root, path)
		if e != nil {
			return e
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out, err
}
func (d directory) Read(name string) ([]byte, error) {
	p, e := d.path(name)
	if e != nil {
		return nil, e
	}
	return os.ReadFile(p)
}
func (d directory) Write(name string, data []byte) error {
	p, e := d.path(name)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p), ".mirror-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e != nil {
		f.Close()
		return e
	}
	_, e = f.Write(data)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), p)
}
func (d directory) Remove(name string) error {
	p, e := d.path(name)
	if e != nil {
		return e
	}
	if e = os.Remove(p); e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	for parent := filepath.Dir(p); parent != d.root && parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
		if e = os.Remove(parent); e != nil {
			if errors.Is(e, os.ErrNotExist) {
				continue
			}
			break
		}
	}
	return nil
}
