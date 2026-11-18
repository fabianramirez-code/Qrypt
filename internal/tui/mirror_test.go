package tui

import (
	"charm.land/lipgloss/v2"
	"errors"
	"github.com/fabianramirez-code/qrypt/internal/mirror"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/settings"
)

func mirrorFixture(t *testing.T, lang Language) Model {
	t.Helper()
	seed := make([]byte, identity.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	id, err := identity.FromSeed(seed)
	if err != nil {
		t.Fatal(err)
	}
	m := send(t, New(filepath.Join(t.TempDir(), "identity"), lang), tea.WindowSizeMsg{Width: 60, Height: 20})
	m.copyFn = func(string) error { return nil }
	m.clearFn = func(string) error { return nil }
	m.openVault(id)
	t.Cleanup(func() { m.closeVault() })
	return m
}

func TestMirrorScreenAndSettings(t *testing.T) {
	for _, tc := range []struct {
		lang Language
		no   string
	}{{English, "No mirror"}, {Spanish, "Sin espejo"}} {
		t.Run(tc.no, func(t *testing.T) {
			m := mirrorFixture(t, tc.lang)
			m = key(t, m, 'm')
			if !strings.Contains(plain(m), tc.no) {
				t.Fatal(plain(m))
			}
			m = key(t, m, 'e')
			folder := filepath.Join(t.TempDir(), "cópia con espacios")
			m = send(t, m, tea.PasteMsg{Content: folder})
			m = enter(t, m)
			got, err := settings.Load(m.settingsPath)
			if err != nil || got.Mirror != folder {
				t.Fatalf("settings: %+v %v", got, err)
			}
			if _, err = os.Stat(folder); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.ReplaceAll(m.viewContent(), "\n", ""), folder) || !strings.Contains(plain(m), map[Language]string{English: "Updated: 1", Spanish: "Actualizados: 1"}[tc.lang]) {
				t.Fatal(plain(m))
			}
			m = key(t, m, 'x')
			m = key(t, m, 'n')
			if got, _ = settings.Load(m.settingsPath); got.Mirror != folder {
				t.Fatal("cancel removed setting")
			}
			m = key(t, m, 'x')
			m = key(t, m, 'y')
			if got, _ = settings.Load(m.settingsPath); got.Mirror != "" {
				t.Fatal("setting remains")
			}
			if _, err = os.Stat(folder); err != nil {
				t.Fatal("folder removed", err)
			}
		})
	}
}

func mirrorChoose(t *testing.T, m Model, path string) Model {
	t.Helper()
	m = key(t, m, 'm')
	m = key(t, m, 'e')
	m = send(t, m, tea.PasteMsg{Content: path})
	return enter(t, m)
}
func mirroredFiles(t *testing.T, m Model, folder string) {
	t.Helper()
	want := spaceSnapshot(t, vaultDir(m.path))
	got := spaceSnapshot(t, folder)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("mirror differs: local %v mirror %v", reflect.ValueOf(want).MapKeys(), reflect.ValueOf(got).MapKeys())
	}
}
func TestMirrorChangesAndRestore(t *testing.T) {
	m := mirrorFixture(t, English)
	folder := filepath.Join(t.TempDir(), "mirror")
	m = mirrorChoose(t, m, folder)
	mirroredFiles(t, m, folder)
	m = key(t, m, tea.KeyEsc)
	m = enter(t, m)
	m = addVaultItem(t, m, "account", "secret token")
	mirroredFiles(t, m, folder)
	m = enter(t, m)
	m = key(t, m, 'e')
	m = typeText(t, m, "changed token")
	m = enter(t, m)
	mirroredFiles(t, m, folder)
	m = key(t, m, tea.KeyEsc)
	m = key(t, m, 'd')
	m = key(t, m, 'y')
	mirroredFiles(t, m, folder)
	m = key(t, m, tea.KeyEsc)
	m = key(t, m, 'a')
	m = typeText(t, m, "Work")
	m = enter(t, m)
	mirroredFiles(t, m, folder)
	m = key(t, m, 'r')
	m = typeText(t, m, "Office")
	m = enter(t, m)
	mirroredFiles(t, m, folder)
	m = key(t, m, 'd')
	m = key(t, m, 'y')
	mirroredFiles(t, m, folder)
	m = key(t, m, 'm')
	m = key(t, m, 'r')
	if !strings.Contains(plain(m), "Mirror restored.") || !strings.Contains(plain(m), "Personal (0)") {
		t.Fatal(plain(m))
	}
	mirroredFiles(t, m, folder)
	m = enter(t, m)
	m = addVaultItem(t, m, "account", "cannot overwrite")
	m = key(t, m, tea.KeyEsc)
	m = key(t, m, 'm')
	before := spaceSnapshot(t, vaultDir(m.path))
	m = key(t, m, 'r')
	if !strings.Contains(plain(m), "Cannot restore: the vault holds a secret.") || !reflect.DeepEqual(before, spaceSnapshot(t, vaultDir(m.path))) {
		t.Fatal(plain(m))
	}
}

type failingMirror struct {
	mirror.Storage
	fail bool
}

func (f failingMirror) Write(name string, data []byte) error {
	if f.fail {
		return errors.New("storage unavailable")
	}
	return f.Storage.Write(name, data)
}
func TestMirrorFailureThenRepair(t *testing.T) {
	m := mirrorFixture(t, English)
	folder := filepath.Join(t.TempDir(), "mirror")
	m = mirrorChoose(t, m, folder)
	m = key(t, m, tea.KeyEsc)
	m.mirrorStorage = func(path string) mirror.Storage { return failingMirror{mirror.Dir(path), true} }
	m = enter(t, m)
	m = addVaultItem(t, m, "account", "private")
	if m.screen != vaultList || !strings.Contains(plain(m), "Mirror not updated: storage unavailable") || m.ExitStatus() != 0 {
		t.Fatal(plain(m))
	}
	if _, err := m.vlt.Get("account"); err != nil {
		t.Fatal(err)
	}
	m.mirrorStorage = func(path string) mirror.Storage { return mirror.Dir(path) }
	m = addVaultItem(t, m, "second", "token")
	mirroredFiles(t, m, folder)
	if m.mirrorWarning {
		t.Fatal("warning not cleared")
	}
}
func TestMirrorPreviewWideAndLock(t *testing.T) {
	id, err := identity.FromSeed(make([]byte, identity.SeedSize))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "mirror")
	m := enter(t, send(t, NewPreview(filepath.Join(root, "identity"), English, id), tea.WindowSizeMsg{Width: 100, Height: 20}))
	m = key(t, m, 'm')
	if m.screen != mirrorScreen || strings.Contains(framed(m), "DEMO-ACCOUNT-0000") || !strings.Contains(framed(m), "Mirror") {
		t.Fatal(framed(m))
	}
	m = key(t, m, 'e')
	m = typeText(t, m, outside)
	m = enter(t, m)
	if !strings.Contains(plain(m), "Preview mirror must stay in the temporary directory.") {
		t.Fatal(plain(m))
	}
	if _, err := os.Stat(outside); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	m = key(t, m, tea.KeyEsc)
	m = key(t, m, tea.KeyEsc)
	if m.screen != spacesList || !strings.Contains(framed(m), "Personal (4)") {
		t.Fatal(framed(m))
	}
	m = key(t, m, 'm')
	m.now = func() time.Time { return m.lastKey.Add(idleLifetime) }
	next, cmd := m.Update(idleTickMsg{})
	if cmd == nil || next.(Model).ExitStatus() != 0 {
		t.Fatal("idle lock")
	}
	m = ctrlC(t, m)
	if m.ExitStatus() != 0 {
		t.Fatal("Ctrl+C status")
	}
}
func TestMirrorRenderFits(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		for _, theme := range []string{"green", "carbon"} {
			for _, width := range []int{60, 100} {
				m := mirrorFixture(t, lang)
				m = send(t, m, tea.WindowSizeMsg{Width: width, Height: 20})
				m.themeName = theme
				m.refreshStyles()
				for _, state := range []screen{mirrorScreen, mirrorEdit, mirrorStop} {
					m.screen = state
					lines := strings.Split(framed(m), "\n")
					if len(lines) > 20 {
						t.Fatalf("%s %d %d: %d rows", theme, width, state, len(lines))
					}
					for _, line := range lines {
						if lipgloss.Width(line) > width {
							t.Fatalf("%s %d: %q", theme, width, line)
						}
					}
				}
			}
		}
	}
}
func TestMirrorReportAndRestoreErrors(t *testing.T) {
	m := mirrorFixture(t, English)
	folder := filepath.Join(t.TempDir(), "mirror")
	m = mirrorChoose(t, m, folder)
	// Error messages are emitted through the public R action, not direct Restore calls.
	m.mirrorStorage = func(string) mirror.Storage { return mirror.Dir(filepath.Join(t.TempDir(), "empty")) }
	m = key(t, m, 'r')
	if !strings.Contains(plain(m), "Cannot restore: the mirror is empty.") {
		t.Fatal(plain(m))
	}
}

func TestMirrorRejectsFile(t *testing.T) {
	m := mirrorFixture(t, English)
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	m = key(t, m, 'm')
	m = key(t, m, 'e')
	m = typeText(t, m, file)
	m = enter(t, m)
	if !strings.Contains(plain(m), "file") {
		t.Fatal(plain(m))
	}
	if got, err := settings.Load(m.settingsPath); err != nil || got.Mirror != "" {
		t.Fatalf("saved bad path: %+v %v", got, err)
	}
}

// A new device with the same identity points at the mirror of another device,
// restores it, and sees everything. Its own empty space is never copied into
// the mirror first.
func TestMirrorRestoresOnAFreshDevice(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "mirror")

	a := mirrorFixture(t, English)
	a = mirrorChoose(t, a, folder)
	a = key(t, a, tea.KeyEsc)
	a = enter(t, a)
	a = addVaultItem(t, a, "account", "secret token")
	a = key(t, a, tea.KeyEsc)
	a = key(t, a, 'a')
	a = typeText(t, a, "Work")
	a = enter(t, a)
	a = enter(t, a)
	a = addVaultItem(t, a, "server", "work token")
	mirroredFiles(t, a, folder)
	before := spaceSnapshot(t, folder)

	b := mirrorFixture(t, English)
	if !strings.Contains(plain(b), "Personal (0)") {
		t.Fatalf("fresh device:\n%s", plain(b))
	}
	b = mirrorChoose(t, b, folder)
	// The fixture's window is 60 columns wide, so the line wraps between words.
	if !strings.Contains(strings.Join(strings.Fields(plain(b)), " "), "This folder already holds a vault. Press R to restore it.") {
		t.Fatalf("no restore hint: %q", plain(b))
	}
	if !reflect.DeepEqual(before, spaceSnapshot(t, folder)) {
		t.Fatal("the fresh device wrote to the mirror before restoring")
	}
	b = key(t, b, 'r')
	if b.screen != spacesList || !strings.Contains(plain(b), "Personal (1)") || !strings.Contains(plain(b), "Work (1)") || strings.Contains(plain(b), "Personal (0)") {
		t.Fatalf("after restore:\n%s", plain(b))
	}
	mirroredFiles(t, b, folder)
	for range 3 {
		if strings.Contains(plain(b), "> Work (1)") {
			break
		}
		b = key(t, b, tea.KeyDown)
	}
	b = enter(t, b)
	b = enter(t, b)
	b = key(t, b, tea.KeyTab)
	if !strings.Contains(plain(b), "server") || !strings.Contains(plain(b), "work token") {
		t.Fatalf("restored item not readable:\n%s", plain(b))
	}
}
