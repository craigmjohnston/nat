// Command notion-agent-tracker is a terminal UI over a Notion workspace for
// tracking project work executed by Claude Code agents.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/notion-agent-tracker/internal/config"
	"github.com/craigmjohnston/notion-agent-tracker/internal/tui"
)

// The process's edges, held as variables so tests can stand in for them: main
// is otherwise unexercisable without the real Notion CLI, a terminal, and an
// exit that would take the test binary with it.
var (
	newTokens                   = ntnCLI
	stdin     io.Reader         = os.Stdin
	stdout    io.Writer         = os.Stdout
	stderr    io.Writer         = os.Stderr
	exit                        = os.Exit
	newClient tui.NewClientFunc = tui.DefaultNewClient
)

// ntnCLI is where the Notion credential really comes from.
func ntnCLI() config.TokenSource { return config.NewNtnCLI() }

func main() {
	if err := run(newTokens(), stdin, stdout); err != nil {
		fmt.Fprintln(stderr, "notion-agent-tracker:", err)
		exit(1)
	}
}

// run builds the root model and hands it to Bubble Tea.
func run(tokens config.TokenSource, in io.Reader, out io.Writer) error {
	app, err := buildApp(tokens)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(app, tea.WithInput(in), tea.WithOutput(out)).Run()
	return err
}

// buildApp decides which screen the app starts on: the first-run wizard when
// there is no config file yet, otherwise the board.
//
// Authentication is settled before either screen. The token belongs to the
// Notion CLI, so a missing or expired one is fixed outside this program with
// `ntn login` — there is nothing the wizard could usefully do about it.
func buildApp(tokens config.TokenSource) (*tui.App, error) {
	cfg, found, err := config.Load()
	if err != nil {
		return nil, err
	}
	token, err := tokens.Token()
	if err != nil {
		return nil, authHint(err)
	}
	if !found {
		return tui.NewAppWithOnboarding(
			tui.NewOnboarding(cfg, newClient(token), config.Save)), nil
	}
	return tui.NewApp(cfg), nil
}

// authHint appends the command that fixes an authentication failure, so the
// user is not left to work out what "not logged in" wants from them.
func authHint(err error) error {
	switch {
	case errors.Is(err, config.ErrNtnNotInstalled):
		return fmt.Errorf("%w\ninstall it with: curl -fsSL https://ntn.dev | bash", err)
	case errors.Is(err, config.ErrNtnNotLoggedIn):
		return fmt.Errorf("%w\nlog in with: %s login", err, config.NtnBinary)
	default:
		return err
	}
}
