// Command notion-agent-tracker is a terminal UI over a Notion workspace for
// tracking project work executed by Claude Code agents.
package main

import (
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/notion-agent-tracker/internal/config"
	"github.com/craigmjohnston/notion-agent-tracker/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "notion-agent-tracker:", err)
		os.Exit(1)
	}
}

// run builds the root model and hands it to Bubble Tea.
func run() error {
	app, err := buildApp(config.NewKeyring())
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(app).Run()
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
