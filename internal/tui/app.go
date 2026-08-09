// Package tui is the Bubble Tea v2 interface: a root model that routes between
// screens, and the screens themselves.
package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/craigmjohnston/notion-agent-tracker/internal/config"
)

// App is the root model. It owns whichever screen is on show and routes
// messages to it. Only the onboarding screen exists so far; the board replaces
// the placeholder in a later milestone.
type App struct {
	cfg        config.Config
	onboarding *Onboarding
	note       string
}

var _ tea.Model = (*App)(nil)

// NewApp returns the root model showing the board, for a workspace that is
// already set up.
func NewApp(cfg config.Config) *App {
	return &App{cfg: cfg}
}

// NewAppWithOnboarding returns the root model showing the first-run wizard.
func NewAppWithOnboarding(o *Onboarding) *App {
	return &App{onboarding: o}
}

// Init starts the screen on show.
func (a *App) Init() tea.Cmd {
	if a.onboarding != nil {
		return a.onboarding.Init()
	}
	return nil
}

// Update routes messages to the current screen, and handles the screen
// switches: onboarding hands over to the board when it finishes.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// ctrl+c always quits: the wizard's forms handle it themselves, but a
		// wizard that failed has no form left to catch it. "q" and esc only
		// quit outside the wizard, so that typing into a form field does not
		// exit the program.
		if msg.String() == "ctrl+c" || (a.onboarding == nil && isQuit(msg)) {
			return a, tea.Quit
		}
	case OnboardingDoneMsg:
		a.cfg, a.onboarding = msg.Config, nil
		a.note = "Setup complete."
		if msg.NeedsProject {
			a.note = "Setup complete. No projects yet — the new-project flow lands in a later milestone."
		}
		return a, nil
	}

	if a.onboarding != nil {
		o, cmd := a.onboarding.Update(msg)
		a.onboarding = o
		return a, cmd
	}
	return a, nil
}

// View renders the current screen.
func (a *App) View() tea.View {
	if a.onboarding != nil {
		return tea.NewView(a.onboarding.View())
	}
	return tea.NewView(a.boardPlaceholder())
}

// boardPlaceholder stands in for the board screen until it exists.
func (a *App) boardPlaceholder() string {
	s := ""
	if a.note != "" {
		s = a.note + "\n\n"
	}
	return s + fmt.Sprintf(
		"notion-agent-tracker\n\nProject database: %s\nAssignee: %s\nProjects configured: %d\n\nThe board is not built yet. Press q to quit.\n",
		orDash(a.cfg.ProjectDBDataSourceID), orDash(a.cfg.AssigneeUserName), len(a.cfg.Projects))
}

// isQuit reports whether a key press should exit the program when no screen
// wants to handle it first.
func isQuit(msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "q", "esc":
		return true
	}
	return false
}

// orDash renders an unset value as a dash.
func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
