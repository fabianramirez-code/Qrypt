// Command qrypt starts the interactive identity creation screen.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/tui"
	"golang.org/x/term"
)

func main() {
	args := os.Args[1:]
	if len(args) > 1 || len(args) == 1 && args[0] != "--preview" {
		fmt.Fprintln(os.Stderr, "Usage: qrypt [--preview]")
		os.Exit(2)
	}
	if len(args) == 1 || !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		os.Exit(runArgs(os.Stdin, os.Stdout, os.Stderr, "", args))
	}
	path, err := identity.DefaultPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, pathFailure(err))
		os.Exit(1)
	}
	os.Exit(runArgs(os.Stdin, os.Stdout, os.Stderr, path, args))
}

func pathFailure(err error) string {
	return "Qrypt could not find the configuration directory / Qrypt no encontró el directorio de configuración: " + err.Error()
}

func run(stdin, stdout *os.File, stderr io.Writer, path string) int {
	if !term.IsTerminal(int(stdin.Fd())) || !term.IsTerminal(int(stdout.Fd())) {
		fmt.Fprintln(stderr, "Qrypt needs an interactive terminal (on Windows: Windows Terminal, PowerShell or cmd). / Qrypt necesita una terminal interactiva.")
		return 1
	}
	result, err := tea.NewProgram(tui.New(path, tui.FromTag(systemLanguageTag())), tea.WithInput(stdin), tea.WithOutput(stdout)).Run()
	if err != nil {
		fmt.Fprintln(stderr, "Could not run the terminal interface / No se pudo iniciar la interfaz de terminal:", err)
		return 1
	}
	return result.(tui.Model).ExitStatus()
}

func runArgs(stdin, stdout *os.File, stderr io.Writer, path string, args []string) int {
	if len(args) > 1 || len(args) == 1 && args[0] != "--preview" {
		fmt.Fprintln(stderr, "Usage: qrypt [--preview]")
		return 2
	}
	if len(args) == 0 {
		return run(stdin, stdout, stderr, path)
	}
	if !term.IsTerminal(int(stdin.Fd())) || !term.IsTerminal(int(stdout.Fd())) {
		fmt.Fprintln(stderr, "Qrypt needs an interactive terminal (on Windows: Windows Terminal, PowerShell or cmd). / Qrypt necesita una terminal interactiva.")
		return 1
	}
	code, err := runPreview(func(m tui.Model, _ string) (tui.Model, error) {
		result, err := tea.NewProgram(m, tea.WithInput(stdin), tea.WithOutput(stdout)).Run()
		if err != nil {
			return m, err
		}
		return result.(tui.Model), nil
	})
	if err != nil {
		fmt.Fprintln(stderr, "Could not run the terminal interface / No se pudo iniciar la interfaz de terminal:", err)
	}
	return code
}

// runPreview owns the throwaway directory through every exit, including setup errors.
func runPreview(runProgram func(tui.Model, string) (tui.Model, error)) (int, error) {
	dir, err := os.MkdirTemp("", "qrypt-preview-")
	if err != nil {
		return 1, err
	}
	defer os.RemoveAll(dir)
	id, err := identity.Generate()
	if err != nil {
		return 1, err
	}
	data, _, err := id.Seal()
	if err != nil {
		return 1, err
	}
	path := filepath.Join(dir, "identity")
	if err := identity.Save(path, data); err != nil {
		return 1, err
	}
	result, err := runProgram(tui.NewPreview(path, tui.FromTag(systemLanguageTag()), id), path)
	if err != nil {
		return 1, err
	}
	return result.ExitStatus(), nil
}
