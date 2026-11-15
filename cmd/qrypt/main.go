// Command qrypt starts the interactive identity creation screen.
package main

import (
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/fabianramirez-code/qrypt/internal/identity"
	"github.com/fabianramirez-code/qrypt/internal/tui"
	"golang.org/x/term"
)

func main() {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		os.Exit(run(os.Stdin, os.Stdout, os.Stderr, ""))
	}
	path, err := identity.DefaultPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not find the identity location:", err)
		os.Exit(1)
	}
	os.Exit(run(os.Stdin, os.Stdout, os.Stderr, path))
}

func run(stdin, stdout *os.File, stderr io.Writer, path string) int {
	if !term.IsTerminal(int(stdin.Fd())) || !term.IsTerminal(int(stdout.Fd())) {
		fmt.Fprintln(stderr, "Qrypt needs an interactive terminal. On Windows use Windows Terminal, PowerShell or cmd.")
		return 1
	}
	result, err := tea.NewProgram(tui.New(path), tea.WithInput(stdin), tea.WithOutput(stdout)).Run()
	if err != nil {
		fmt.Fprintln(stderr, "Could not run the terminal interface:", err)
		return 1
	}
	return result.(tui.Model).ExitStatus()
}
