package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/domain"
)

// planSlice is the planning agent as the launch plumbing sees it: the sentinel
// where a slice's page ID would go, and a name for the strings the flow
// prints. It exists so the show/attach path — which is about panes and
// sessions, not slices — serves the planning agent unchanged.
func planSlice() domain.Slice {
	return domain.Slice{ID: agent.PlanSentinel, Name: "the plan"}
}

// PlanForm is the modal behind w when no planning agent is running: what the
// user wants to workshop, carried into the agent's prompt so the session
// starts on it. There is no directory question — the planning commands work
// wherever they are typed, so the project's default is always right — and an
// empty answer launches a plain planning session.
type PlanForm struct {
	form *huh.Form

	request string
}

// newPlanForm returns the form for launching a planning agent.
func newPlanForm(theme huh.Theme) *PlanForm {
	f := &PlanForm{}
	f.form = huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("What do you want to workshop?").
			Description("Goes into the agent's prompt; empty starts a plain session.").
			Value(&f.request),
	)).WithTheme(theme)
	return f
}

// Init starts the form.
func (f *PlanForm) Init() tea.Cmd { return f.form.Init() }

// Update feeds a message to the form.
func (f *PlanForm) Update(msg tea.Msg) tea.Cmd {
	form, cmd := f.form.Update(msg)
	f.form = form.(*huh.Form)
	return cmd
}

// State is how far the form has got.
func (f *PlanForm) State() huh.FormState { return f.form.State }

// View renders the form.
func (f *PlanForm) View() string { return f.form.View() }

// Heading is the title drawn over the form.
func (f *PlanForm) Heading() string { return "Launch a planning agent" }

// SetSize gives the form the room the window leaves it.
func (f *PlanForm) SetSize(width, height int) {
	f.form = f.form.WithWidth(width).WithHeight(height)
}

// busyNote says what the status bar shows while the session starts.
func (f *PlanForm) busyNote() string { return "Launching the planning agent…" }

// save starts the session the completed form describes, in the project's
// default working directory.
func (f *PlanForm) save(a *App) tea.Cmd {
	// The form only ever opens on a configured project, so this is the one it
	// was opened against.
	project, _ := a.activeProject()
	return launchPlanAgent(a.launcher, project.Name, expandHome(project.WorkingDir),
		strings.TrimSpace(f.request))
}

// launchPlanAgent writes the planning prompt out — the user's request folded
// in — and starts the detached session that reads it, tagged with the sentinel
// rather than a slice ID. It comes back as the same message a slice launch
// does, so the failure reporting is shared.
func launchPlanAgent(l AgentLauncher, projectName, workdir, request string) tea.Cmd {
	return func() tea.Msg {
		file, err := agent.WritePromptFile(agent.PlanSession, agent.PlanPrompt(projectName, workdir, request))
		if err != nil {
			return agentLaunchedMsg{err: fmt.Errorf("launch planning agent: %w", err)}
		}
		if err := l.Launch(agent.PlanSession, workdir, file, agent.PlanSentinel); err != nil {
			return agentLaunchedMsg{err: err}
		}
		// A planning launch always attaches: the user has just said what they
		// want to workshop, so the pane is shown straight away.
		return agentLaunchedMsg{slice: planSlice(), session: agent.PlanSession, attach: true}
	}
}

// planAgentFlow is what w does: launches a planning agent when none is
// running, and shows or hides the one that is — the same toggle t is for a
// slice's agent. One planning agent is enough: a second would workshop the
// same plan the first is already holding in its head.
func (a *App) planAgentFlow() tea.Cmd {
	_, ok := a.activeProject()
	if !ok || a.launcher == nil || a.busy {
		return nil
	}
	if session := a.live[agent.PlanSentinel]; session != "" {
		a.busy, a.note = true, ""
		return a.showAgent(planSlice(), session)
	}
	return a.openForm(newPlanForm(a.styles.FormTheme))
}
