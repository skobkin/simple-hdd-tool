package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

	"github.com/skobkin/simple-hdd-tool/internal/app"
	"github.com/skobkin/simple-hdd-tool/internal/buildinfo"
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
		if _, err := fmt.Println(buildinfo.Version); err != nil {
			exitWithMessage(1, os.Stderr, err.Error())
		}

		return
	}

	stdinIsTTY := term.IsTerminal(os.Stdin.Fd())
	stdoutIsTTY := term.IsTerminal(os.Stdout.Fd())
	if !stdinIsTTY && !stdoutIsTTY {
		exitWithMessage(1, os.Stderr, "interactive terminal required: no TTY attached to stdin or stdout")
	}

	model := app.NewModel(cfg)
	model.SetAltScreen(stdoutIsTTY)

	opts := []tea.ProgramOption{
		tea.WithInput(os.Stdin),
		tea.WithOutput(os.Stdout),
	}

	p := tea.NewProgram(model, opts...)
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
