package tui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/tui/theme"
	"github.com/fabianramirez-code/qrypt/internal/vault"
)

// problemFixture creates an actual manifest mismatch after locking the first session.
func problemFixture(t *testing.T, lang Language, withWorkItem bool, damage func(*testing.T, string)) (Model, string, string) {
	t.Helper()
	m, path, pass, _ := unlockFixture(t, lang)
	m, cmd := beginCheck(t, m, pass)
	m = finishCheck(t, m, cmd)
	m = enter(t, m)
	m = addVaultItem(t, m, "private-name", "private-value")
	m = key(t, m, tea.KeyEsc)
	m = key(t, m, 'a')
	m = typeText(t, m, "Work")
	m = enter(t, m)
	if withWorkItem {
		m = enter(t, m)
		m = addVaultItem(t, m, "work-item", "work-value")
		m = key(t, m, tea.KeyEsc)
	}
	root := vaultDir(path)
	id := m.vlt.ID()
	m = key(t, m, tea.KeyEsc)
	damage(t, filepath.Join(root, "scopes", id))
	m = send(t, New(path, lang), tea.WindowSizeMsg{Width: 80, Height: 24})
	m = enter(t, m)
	m, cmd = beginCheck(t, m, pass)
	m = finishCheck(t, m, cmd)
	m.copyFn = func(string) error { return nil }
	m.clearFn = func(string) error { return nil }
	return m, path, id
}

func TestProblemIsolationAcceptAndRejections(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		t.Run(lang.text().spacesTitle, func(t *testing.T) {
			m, path, id := problemFixture(t, lang, false, func(t *testing.T, dir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "secrets", "0123456789abcdef0123456789abcdef.qrs"), []byte("broken"), 0600); err != nil {
					t.Fatal(err)
				}
			})
			if m.screen != spacesList || m.ExitStatus() != 0 || !strings.Contains(plain(m), "Personal (1)") || !strings.Contains(plain(m), "! Work") {
				t.Fatal(plain(m))
			}
			m = enter(t, m)
			if m.screen != vaultList || !strings.Contains(plain(m), "private-name") {
				t.Fatal(plain(m))
			}
			m = enter(t, m)
			m = key(t, m, tea.KeyTab)
			if !strings.Contains(plain(m), "private-value") {
				t.Fatal(plain(m))
			}
			m = key(t, m, tea.KeyEsc)
			m = key(t, m, tea.KeyEsc)
			m = key(t, m, tea.KeyDown)
			before := spaceSnapshot(t, vaultDir(path))
			state, err := os.ReadFile(stateFile(path))
			if err != nil {
				t.Fatal(err)
			}
			for _, code := range []rune{'a', 'r', 'd'} {
				m = key(t, m, code)
				if m.screen != spacesList || !reflect.DeepEqual(before, spaceSnapshot(t, vaultDir(path))) {
					t.Fatal("problem action changed files")
				}
			}
			m = enter(t, m)
			if m.screen != problemScreen || !strings.Contains(plain(m), "Unexpected files: 1") && lang == English || strings.Contains(plain(m), "private-name") || strings.Contains(plain(m), "private-value") {
				t.Fatal(plain(m))
			}
			for _, word := range []string{"", "wrong", "accept"} {
				m = typeText(t, m, word)
				m = enter(t, m)
				if m.screen != problemScreen || !reflect.DeepEqual(before, spaceSnapshot(t, vaultDir(path))) {
					t.Fatal("rejected input changed vault", plain(m))
				}
				after, e := os.ReadFile(stateFile(path))
				if e != nil || !bytes.Equal(state, after) {
					t.Fatal("rejected input changed record", e)
				}
				for range []rune(word) {
					m = key(t, m, tea.KeyBackspace)
				}
			}
			m = send(t, m, tea.PasteMsg{Content: "ACCEPT"})
			if m.input.String() != "" || !strings.Contains(plain(m), lang.text().problemPaste) {
				t.Fatal(plain(m))
			}
			m = key(t, m, tea.KeyEsc)
			if m.screen != spacesList || !reflect.DeepEqual(before, spaceSnapshot(t, vaultDir(path))) {
				t.Fatal("Esc changed vault")
			}
			m = enter(t, m)
			m = typeText(t, m, lang.text().problemWord)
			m = enter(t, m)
			if m.screen != spacesList || len(m.problems) != 0 || !strings.Contains(plain(m), "Work (0)") {
				t.Fatal(plain(m))
			}
			scopes, problems, e := vault.OpenScopes(vaultDir(path), stateFile(path), m.scopeID)
			if e != nil || len(problems) != 0 || len(scopes) != 2 {
				t.Fatalf("reopen: %v %v", problems, e)
			}
			for _, v := range scopes {
				v.Close()
			}
			_ = id
		})
	}
}

func TestProblemKindsOnDisk(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   vault.ProblemKind
		damage func(*testing.T, string, string)
		check  func(*testing.T, vault.Problem, string)
	}{
		{"missing", vault.ProblemFiles, func(t *testing.T, dir, state string) {
			p := firstSecret(t, dir)
			if err := os.Rename(p, filepath.Join(dir, "secrets", "displaced.tmp")); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, p vault.Problem, view string) {
			if p.Missing != 1 || !strings.Contains(view, "Missing files: 1") {
				t.Fatal(p, view)
			}
		}},
		{"unexpected", vault.ProblemFiles, func(t *testing.T, dir, state string) {
			if err := os.WriteFile(filepath.Join(dir, "secrets", "0123456789abcdef0123456789abcdef.qrs"), []byte("bad"), 0600); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, p vault.Problem, view string) {
			if p.Unexpected != 1 || p.Undecryptable != 1 || !strings.Contains(view, "Undecryptable files: 1") {
				t.Fatal(p, view)
			}
		}},
		{"changed-undecryptable", vault.ProblemFiles, func(t *testing.T, dir, state string) {
			p := firstSecret(t, dir)
			b, e := os.ReadFile(p)
			if e != nil {
				t.Fatal(e)
			}
			b[40] ^= 0xff
			if e = os.WriteFile(p, b, 0600); e != nil {
				t.Fatal(e)
			}
		}, func(t *testing.T, p vault.Problem, view string) {
			if p.Changed != 1 || p.Undecryptable != 1 || !strings.Contains(view, "Changed files: 1") {
				t.Fatal(p, view)
			}
		}},
		{"signature", vault.ProblemSignature, func(t *testing.T, dir, state string) {
			p := filepath.Join(dir, "manifest")
			b, e := os.ReadFile(p)
			if e != nil {
				t.Fatal(e)
			}
			b[len(b)-1] ^= 0xff
			if e = os.WriteFile(p, b, 0600); e != nil {
				t.Fatal(e)
			}
		}, func(t *testing.T, p vault.Problem, view string) {
			if !strings.Contains(view, english.problemSignature) {
				t.Fatal(p, view)
			}
		}},
		{"unreadable", vault.ProblemUnreadable, func(t *testing.T, dir, state string) {
			if e := os.WriteFile(filepath.Join(dir, "scope"), []byte("invalid"), 0600); e != nil {
				t.Fatal(e)
			}
		}, func(t *testing.T, p vault.Problem, view string) {
			if !strings.Contains(strings.ReplaceAll(view, "\n ", " "), english.problemUnreadable) || strings.Contains(view, english.problemPrompt) {
				t.Fatal(p, view)
			}
		}},
		{"removed", vault.ProblemRemoved, func(t *testing.T, dir, state string) {
			if e := os.Rename(dir, filepath.Join(filepath.Dir(dir), "displaced-space")); e != nil {
				t.Fatal(e)
			}
		}, func(t *testing.T, p vault.Problem, view string) {
			if !strings.Contains(view, english.problemRemoved) {
				t.Fatal(p, view)
			}
		}},
		{"rollback", vault.ProblemRollback, func(t *testing.T, dir, state string) { changeRecord(t, state, filepath.Base(dir), "9", "") }, func(t *testing.T, p vault.Problem, view string) {
			if p.Recorded != 9 || !strings.Contains(view, "Recorded generation: 9") || !strings.Contains(view, fmt.Sprintf("Current generation: %d", p.Generation)) {
				t.Fatal(p, view)
			}
		}},
		{"fork", vault.ProblemFork, func(t *testing.T, dir, state string) {
			changeRecord(t, state, filepath.Base(dir), "", "0000000000000000000000000000000000000000000000000000000000000000")
		}, func(t *testing.T, p vault.Problem, view string) {
			if !strings.Contains(view, english.problemFork) {
				t.Fatal(p, view)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, path, id := problemFixture(t, English, true, func(t *testing.T, dir string) {
				tc.damage(t, dir, stateFile(filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(dir))), "identity")))
			})
			if m.screen != spacesList || len(m.problems) != 1 || m.problems[0].Kind != tc.kind {
				t.Fatal(m.screen, m.problems)
			}
			m = key(t, m, tea.KeyDown)
			m = enter(t, m)
			if m.screen != problemScreen {
				t.Fatal(plain(m))
			}
			tc.check(t, m.problems[0], plain(m))
			if strings.Contains(plain(m), "private-name") || strings.Contains(plain(m), "private-value") {
				t.Fatal("secret leaked", plain(m))
			}
			if tc.kind == vault.ProblemUnreadable {
				before := spaceSnapshot(t, vaultDir(path))
				m = typeText(t, m, "ACCEPT")
				m = enter(t, m)
				if m.screen != problemScreen || m.input.String() != "" || !reflect.DeepEqual(before, spaceSnapshot(t, vaultDir(path))) {
					t.Fatal("unreadable changed")
				}
			}
			if tc.kind == vault.ProblemRemoved {
				m = typeText(t, m, "ACCEPT")
				m = enter(t, m)
				if m.screen != spacesList || len(m.problems) != 0 || strings.Contains(plain(m), "! ") {
					t.Fatal("removed not accepted", plain(m))
				}
			}
			_ = id
		})
	}
}

func firstSecret(t *testing.T, dir string) string {
	t.Helper()
	es, e := os.ReadDir(filepath.Join(dir, "secrets"))
	if e != nil || len(es) != 1 {
		t.Fatal(e, es)
	}
	return filepath.Join(dir, "secrets", es[0].Name())
}
func changeRecord(t *testing.T, path, id, generation, hash string) {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	for i, line := range lines {
		parts := strings.Split(line, " ")
		if parts[0] == id {
			if generation != "" {
				parts[1] = generation
			}
			if hash != "" {
				parts[2] = hash
			}
			lines[i] = strings.Join(parts, " ")
		}
	}
	if e = os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); e != nil {
		t.Fatal(e)
	}
}

func TestProblemFallbackAndFit(t *testing.T) {
	for _, lang := range []Language{English, Spanish} {
		m, _, id := problemFixture(t, lang, false, func(t *testing.T, dir string) {
			if e := os.WriteFile(filepath.Join(dir, "scope"), []byte("bad"), 0600); e != nil {
				t.Fatal(e)
			}
		})
		label := lang.text().unknownSpace + " " + id[:8]
		if !strings.Contains(plain(m), "! "+label) {
			t.Fatal(plain(m))
		}
		if m.selectedProblem() == nil {
			m = key(t, m, tea.KeyDown)
		}
		m = enter(t, m)
		if !strings.Contains(plain(m), label) {
			t.Fatal(plain(m))
		}
		for _, size := range []tea.WindowSizeMsg{{Width: 60, Height: 20}, {Width: 100, Height: 20}} {
			for _, name := range theme.Names() {
				x := send(t, m, size)
				x.themeName = name
				x.refreshStyles()
				wideFit(t, x, size.Width, size.Height)
			}
		}
		m = key(t, m, tea.KeyEsc)
	}
}

func TestProblemWideAndLock(t *testing.T) {
	m, _, _ := problemFixture(t, English, false, func(t *testing.T, dir string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "secrets", "0123456789abcdef0123456789abcdef.qrs"), []byte("bad"), 0600); err != nil {
			t.Fatal(err)
		}
	})
	m = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	for m.selectedProblem() == nil {
		m = key(t, m, tea.KeyDown)
	}
	for _, code := range []rune{tea.KeyRight, tea.KeyTab, tea.KeyEnter} {
		x := key(t, m, code)
		if x.screen != problemScreen || strings.Contains(framed(x), "private-name") {
			t.Fatal(framed(x))
		}
		wideFit(t, x, 120, 30)
		x = key(t, x, tea.KeyEsc)
		if x.screen != spacesList {
			t.Fatal("not returned")
		}
	}
	m = key(t, m, tea.KeyEnter)
	m.preview = true
	if !strings.Contains(framed(m), english.previewBanner) {
		t.Fatal("banner missing")
	}
	x := m
	x.now = func() time.Time { return x.lastKey.Add(idleLifetime) }
	next, cmd := x.Update(idleTickMsg{})
	if cmd == nil || next.(Model).ExitStatus() != 0 || next.(Model).scopeID != nil {
		t.Fatal("idle lock failed")
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil || next.(Model).ExitStatus() != 0 || next.(Model).scopeID != nil {
		t.Fatal("Ctrl+C failed")
	}
}

// A space with a problem does not stop new spaces, and accepting it keeps the
// items that can still be read.
func TestProblemAllowsNewSpacesAndAcceptKeepsReadableItems(t *testing.T) {
	m, _, _ := problemFixture(t, English, true, func(t *testing.T, dir string) {
		t.Helper()
		broken := filepath.Join(dir, "secrets", "0123456789abcdef0123456789abcdef.qrs")
		if err := os.WriteFile(broken, []byte("broken"), 0600); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(plain(m), "! Work") {
		t.Fatalf("no problem row:\n%s", plain(m))
	}
	m = key(t, m, 'a')
	m = typeText(t, m, "Home")
	m = enter(t, m)
	if m.screen != spacesList || !strings.Contains(plain(m), "Home (0)") || !strings.Contains(plain(m), "! Work") {
		t.Fatalf("adding a space beside a problem failed:\n%s", plain(m))
	}
	for range 5 {
		if strings.Contains(plain(m), "> ! Work") {
			break
		}
		m = key(t, m, tea.KeyDown)
	}
	if !strings.Contains(plain(m), "> ! Work") {
		t.Fatalf("problem row not selectable:\n%s", plain(m))
	}
	m = enter(t, m)
	m = typeText(t, m, "ACCEPT")
	m = enter(t, m)
	if m.screen != spacesList || strings.Contains(plain(m), "! Work") || !strings.Contains(plain(m), "Work (1)") {
		t.Fatalf("accepting did not restore the space:\n%s", plain(m))
	}
	for range 5 {
		if strings.Contains(plain(m), "> Work (1)") {
			break
		}
		m = key(t, m, tea.KeyUp)
	}
	m = enter(t, m)
	m = enter(t, m)
	m = key(t, m, tea.KeyTab)
	if !strings.Contains(plain(m), "work-item") || !strings.Contains(plain(m), "work-value") {
		t.Fatalf("surviving item not readable:\n%s", plain(m))
	}
}
