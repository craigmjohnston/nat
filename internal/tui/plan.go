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

// PlanForm is the modal behind w when no planning agent is running: where its
// session should start. The directory matters less than a slice agent's — the
// planning commands work wherever they are typed — but it is where the agent
// reads the repo's own conventions from, so it is still worth asking.
type PlanForm struct {
	form *huh.Form

	workdir string
}

// newPlanForm returns the form for launching a planning agent, starting on the
// project's default working directory.
func newPlanForm(theme huh.Theme, workdir string) *PlanForm {
	f := &PlanForm{workdir: workdir}
	f.form = huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Working directory").
			Description("Where the agent's session starts; ~ is expanded.").
			Value(&f.workdir).
			Validate(existingDir),
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

// save starts the session the completed form describes.
func (f *PlanForm) save(a *App) tea.Cmd {
	// The form only ever opens on a configured project, so this is the one it
	// was opened against.
	project, _ := a.activeProject()
	return launchPlanAgent(a.launcher, project.Name, expandHome(strings.TrimSpace(f.workdir)))
}

// launchPlanAgent writes the planning prompt out and starts the detached
// session that reads it, tagged with the sentinel rather than a slice ID. It
// comes back as the same message a slice launch does, so the offer to attach
// and the failure reporting are shared.
func launchPlanAgent(l AgentLauncher, projectName, workdir string) tea.Cmd {
	return func() tea.Msg {
		file, err := agent.WritePromptFile(agent.PlanSession, agent.PlanPrompt(projectName, workdir))
		if err != nil {
			return agentLaunchedMsg{err: fmt.Errorf("launch planning agent: %w", err)}
		}
		if err := l.Launch(agent.PlanSession, workdir, file, agent.PlanSentinel); err != nil {
			return agentLaunchedMsg{err: err}
		}
		return agentLaunchedMsg{slice: planSlice(), session: agent.PlanSession}
	}
}

// newPlanAttachForm is the offer to attach as it reads after a planning
// launch: the same confirm as newAttachForm, with the planning agent's own
// words — there is no slice for the usual ones to name, and the key that shows
// it later is w rather than t.
func newPlanAttachForm(theme huh.Theme, session string) *AttachForm {
	f := &AttachForm{heading: "Planning agent launched", slice: planSlice(), session: session}
	f.form = huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Show the planning agent now?").
			Description("It keeps running either way; w shows and hides it.").
			Value(&f.confirmed),
	)).WithTheme(theme)
	return f
}

// planAgentFlow is what w does: launches a planning agent when none is
// running, and shows or hides the one that is — the same toggle t is for a
// slice's agent. One planning agent is enough: a second would workshop the
// same plan the first is already holding in its head.
func (a *App) planAgentFlow() tea.Cmd {
	project, ok := a.activeProject()
	if !ok || a.launcher == nil || a.busy {
		return nil
	}
	if session := a.live[agent.PlanSentinel]; session != "" {
		a.busy, a.note = true, ""
		return a.showAgent(planSlice(), session)
	}
	return a.openForm(newPlanForm(a.styles.FormTheme, project.WorkingDir))
}
