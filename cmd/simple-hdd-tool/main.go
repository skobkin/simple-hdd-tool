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
		exitWithMessage(2, os.Stderr, err.Error())
		return
	}

	if cfg.ShowHelp {
		if _, err := fmt.Print(app.HelpText()); err != nil {
			exitWithMessage(1, os.Stderr, err.Error())
		}
		return
	}

	if cfg.ShowVersion {
		if _, err := fmt.Println(app.Version); err != nil {
			exitWithMessage(1, os.Stderr, err.Error())
		}
		return
	}

	stdinIsTTY := term.IsTerminal(os.Stdin.Fd())
	stdoutIsTTY := term.IsTerminal(os.Stdout.Fd())
	if !stdinIsTTY && !stdoutIsTTY {
		exitWithMessage(1, os.Stderr, "interactive terminal required: no TTY attached to stdin or stdout")
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
		exitWithMessage(1, os.Stderr, err.Error())
	}
}

func exitWithMessage(code int, stream *os.File, message string) {
	if _, err := fmt.Fprintln(stream, message); err != nil {
		os.Exit(1)
	}
	os.Exit(code)
}
