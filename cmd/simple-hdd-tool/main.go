package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"

	"github.com/skobkin/simple-hdd-tool/internal/app"
)

func main() {
	cfg, err := app.ParseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if cfg.ShowHelp {
		fmt.Print(app.HelpText())
		return
	}

	if cfg.ShowVersion {
		fmt.Println(app.Version)
		return
	}

	stdinIsTTY := term.IsTerminal(os.Stdin.Fd())
	stdoutIsTTY := term.IsTerminal(os.Stdout.Fd())
	if !stdinIsTTY && !stdoutIsTTY {
		fmt.Fprintln(os.Stderr, "interactive terminal required: no TTY attached to stdin or stdout")
		os.Exit(1)
	}

	opts := []tea.ProgramOption{
		tea.WithInput(os.Stdin),
		tea.WithOutput(os.Stdout),
	}
	if stdoutIsTTY {
		opts = append(opts, tea.WithAltScreen())
	}

	p := tea.NewProgram(app.NewModel(cfg), opts...)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
