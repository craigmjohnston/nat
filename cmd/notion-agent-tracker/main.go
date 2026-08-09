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
// is otherwise unexercisable without the real keychain, a terminal, and an
// exit that would take the test binary with it.
var (
	newSecrets           = keychain
	stdin      io.Reader = os.Stdin
	stdout     io.Writer = os.Stdout
	stderr     io.Writer = os.Stderr
	exit                 = os.Exit
)

// keychain is where the API key really lives.
func keychain() config.Secrets { return config.NewKeyring() }

func main() {
	if err := run(newSecrets(), stdin, stdout); err != nil {
		fmt.Fprintln(stderr, "notion-agent-tracker:", err)
		exit(1)
	}
}

// run builds the root model and hands it to Bubble Tea.
func run(secrets config.Secrets, in io.Reader, out io.Writer) error {
	app, err := buildApp(secrets)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(app, tea.WithInput(in), tea.WithOutput(out)).Run()
	return err
}

// buildApp decides which screen the app starts on: the first-run wizard when
// either the config file or the stored API key is missing, otherwise the board.
func buildApp(secrets config.Secrets) (*tui.App, error) {
	cfg, found, err := config.Load()
	if err != nil {
		return nil, err
	}
	onboard := !found
	if found {
		if _, err := secrets.GetAPIKey(); err != nil {
			if !errors.Is(err, config.ErrAPIKeyNotFound) {
				return nil, err
			}
			onboard = true
		}
	}
	if onboard {
		return tui.NewAppWithOnboarding(
			tui.NewOnboarding(cfg, tui.DefaultNewClient, secrets, config.Save)), nil
	}
	return tui.NewApp(cfg), nil
}
