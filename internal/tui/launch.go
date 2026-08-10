package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// AgentLauncher is what the launch flow needs of tmux: which slices have an
// agent running and in which session, how to start one, and the command that
// attaches to it. It is an interface so the flow can be driven without a tmux
// server.
type AgentLauncher interface {
	LiveSlices() (map[string]string, error)
	Launch(session, workdir, promptFile, sliceID string) error
	AttachCmd(session string) *exec.Cmd
}

// liveInterval is how often the board re-reads which sessions are running. An
// agent's session outlives any one keystroke, so this is only there to keep a
// board nobody is touching from going stale.
const liveInterval = 30 * time.Second

// The launch flow's edges, held as variables so the tests can stand in for
// them: the real ones drive tmux and sleep for half a minute.
var (
	newLauncher = defaultLauncher
	liveTick    = defaultLiveTick
)

// defaultLauncher is the real tmux on PATH.
func defaultLauncher() AgentLauncher { return agent.NewTmux() }

// defaultLiveTick schedules the next re-read of the running sessions.
func defaultLiveTick() tea.Cmd { return tea.Tick(liveInterval, liveTicked) }

// liveTicked turns the timer going off into the prod to re-read the sessions.
func liveTicked(time.Time) tea.Msg { return liveTickMsg{} }

// The messages the launch flow comes back as.
type (
	// agentLaunchedMsg reports a finished launch: the slice its session was
	// started for, or the error that stopped it.
	agentLaunchedMsg struct {
		slice   domain.Slice
		session string
		err     error
	}
	// liveSessionsMsg carries the slices with an agent running, each mapped to
	// the session it is running in, or the read that failed instead.
	liveSessionsMsg struct {
		live map[string]string
		err  error
	}
	// liveTickMsg is the periodic prod to re-read them.
	liveTickMsg struct{}
	// agentAttachedMsg reports a terminal handed to a session and given back.
	agentAttachedMsg struct {
		note string
		err  error
	}
)

// LaunchForm is the modal behind l: where the agent's session should start.
// The directory is the one thing about a launch worth asking, and it is worth
// asking every time — a slice's brief may well be about somewhere else.
type LaunchForm struct {
	form    *huh.Form
	heading string

	slice   domain.Slice
	workdir string
}

// newLaunchForm returns the form for launching an agent on a slice, starting
// on the working directory the config resolves to.
func newLaunchForm(s domain.Slice, workdir string) *LaunchForm {
	f := &LaunchForm{heading: "Launch an agent for " + s.Name, slice: s, workdir: workdir}
	f.form = huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Working directory").
			Description("Where the agent's session starts; ~ is expanded.").
			Value(&f.workdir).
			Validate(existingDir),
	))
	return f
}

// Init starts the form.
func (f *LaunchForm) Init() tea.Cmd { return f.form.Init() }

// Update feeds a message to the form.
func (f *LaunchForm) Update(msg tea.Msg) tea.Cmd {
	form, cmd := f.form.Update(msg)
	f.form = form.(*huh.Form)
	return cmd
}

// State is how far the form has got.
func (f *LaunchForm) State() huh.FormState { return f.form.State }

// View renders the form.
func (f *LaunchForm) View() string { return f.form.View() }

// Heading is the title drawn over the form.
func (f *LaunchForm) Heading() string { return f.heading }

// busyNote says what the status bar shows while the session starts.
func (f *LaunchForm) busyNote() string { return "Launching the agent…" }

// save starts the session the completed form describes.
func (f *LaunchForm) save(a *App) tea.Cmd {
	// The form only ever opens on a configured project, so this is the one it
	// was opened against.
	project, _ := a.activeProject()
	return launchAgent(a.launcher, agent.PromptContext{
		Slice:         f.slice,
		Project:       project,
		ProjectPageID: a.cfg.ActiveProjectID,
		WorkingDir:    expandHome(strings.TrimSpace(f.workdir)),
		AssigneeName:  a.cfg.AssigneeUserName,
	})
}

// launchAgent writes the agent's prompt out and starts the detached session
// that reads it. Nothing in Notion is touched: the agent claims its own slice,
// which is what keeps the claim honest when two of them race.
func launchAgent(l AgentLauncher, c agent.PromptContext) tea.Cmd {
	return func() tea.Msg {
		session := agent.SessionName(c.Slice.ID)
		file, err := agent.WritePromptFile(session, agent.Prompt(c))
		if err != nil {
			return agentLaunchedMsg{err: fmt.Errorf("launch agent: %w", err)}
		}
		if err := l.Launch(session, c.WorkingDir, file, c.Slice.ID); err != nil {
			return agentLaunchedMsg{err: err}
		}
		return agentLaunchedMsg{slice: c.Slice, session: session}
	}
}

// AttachForm is the confirm offered once a session is up: the agent is running
// either way, and this only asks whether to watch it now.
type AttachForm struct {
	form    *huh.Form
	heading string

	session string

	confirmed bool
}

// newAttachForm returns the confirm for attaching to a freshly started session.
func newAttachForm(sliceName, session string) *AttachForm {
	f := &AttachForm{heading: "Agent launched", session: session}
	f.form = huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("Attach to the agent working %q now?", sliceName)).
			Description("Detaching later leaves it running; t on the slice comes back to it.").
			Value(&f.confirmed),
	))
	return f
}

// Init starts the form.
func (f *AttachForm) Init() tea.Cmd { return f.form.Init() }

// Update feeds a message to the form.
func (f *AttachForm) Update(msg tea.Msg) tea.Cmd {
	form, cmd := f.form.Update(msg)
	f.form = form.(*huh.Form)
	return cmd
}

// State is how far the form has got.
func (f *AttachForm) State() huh.FormState { return f.form.State }

// View renders the form.
func (f *AttachForm) View() string { return f.form.View() }

// Heading is the title drawn over the form.
func (f *AttachForm) Heading() string { return f.heading }

// busyNote is empty: attaching replaces the screen and declining writes
// nothing, so there is no progress worth announcing.
func (f *AttachForm) busyNote() string { return "" }

// save attaches to the session, or — when the answer was no — says how to
// attach later, so the session is not left running unnamed.
func (f *AttachForm) save(a *App) tea.Cmd {
	if !f.confirmed {
		note := "Running in the background — attach with: " + attachCommand(f.session)
		return func() tea.Msg { return agentAttachedMsg{note: note} }
	}
	return attach(a.launcher, f.session)
}

// attachCommand is the shell command that attaches to a session from another
// terminal.
func attachCommand(session string) string {
	return fmt.Sprintf("%s attach-session -t %s", agent.TmuxBinary, session)
}

// attach hands the terminal to a session until the user detaches from it.
func attach(l AgentLauncher, session string) tea.Cmd {
	return tea.ExecProcess(l.AttachCmd(session), attached(session))
}

// attached is what getting the terminal back reports. Detaching is the ordinary
// way out of a session, and leaves the agent running.
func attached(session string) func(error) tea.Msg {
	return func(err error) tea.Msg {
		if err != nil {
			return agentAttachedMsg{err: fmt.Errorf("attach to %s: %w", session, err)}
		}
		return agentAttachedMsg{note: fmt.Sprintf("Detached from %s.", session)}
	}
}

// launchAgentFlow opens the launch form for the slice the cursor is on. Only a
// Todo slice can be launched: Claimed is work an agent already holds, and Done
// is finished — a second agent on either would fight the first.
func (a *App) launchAgentFlow() tea.Cmd {
	project, ok := a.activeProject()
	if !ok || a.launcher == nil || a.busy {
		return nil
	}
	s, ok := a.board.SelectedSlice()
	if !ok {
		a.note = "Move to a slice to launch an agent for it."
		return nil
	}
	if a.live[s.ID] != "" {
		a.note = fmt.Sprintf("An agent is already running for %q — press t to attach.", s.Name)
		return nil
	}
	if s.Status != domain.SliceTodo {
		a.note = fmt.Sprintf("%q is %s — only Todo slices can be launched.", s.Name, s.Status)
		return nil
	}
	return a.openForm(newLaunchForm(s, workdirFor(s, project)))
}

// workdirFor is the directory a slice's agent starts in: its own repo
// override, or the project's default.
func workdirFor(s domain.Slice, p config.ProjectConfig) string {
	if s.Repo != "" {
		return s.Repo
	}
	return p.WorkingDir
}

// attachAgentFlow hands the terminal to the session of the slice the cursor is
// on. Any slice with a live session can be attached to, whatever its status: an
// agent is worth watching from the moment it starts until it exits, and it
// spends nearly all of that time holding a slice it has already Claimed.
func (a *App) attachAgentFlow() tea.Cmd {
	if a.launcher == nil || a.busy {
		return nil
	}
	s, ok := a.board.SelectedSlice()
	if !ok {
		a.note = "Move to a slice to attach to its agent."
		return nil
	}
	session := a.live[s.ID]
	if session == "" {
		a.note = fmt.Sprintf("No agent session is running for %q.", s.Name)
		return nil
	}
	a.busy, a.note = true, ""
	return attach(a.launcher, session)
}

// refreshLive kicks off a read of the running sessions. It is skipped when
// there is no project on show, because there are no slices to mark.
func (a *App) refreshLive() tea.Cmd {
	if _, ok := a.activeProject(); !ok || a.launcher == nil {
		return nil
	}
	l := a.launcher
	return func() tea.Msg {
		live, err := l.LiveSlices()
		return liveSessionsMsg{live: live, err: err}
	}
}

// liveLoaded takes the sessions that came back to the board. A failed read is
// a note rather than an error banner: it is a background poll, and the plan is
// still worth looking at without knowing what is running.
func (a *App) liveLoaded(msg liveSessionsMsg) {
	if msg.err != nil {
		a.live, a.note = nil, fmt.Sprintf("Could not read tmux panes: %v", msg.err)
	} else {
		a.live = msg.live
	}
	a.board.SetLive(a.live)
}

// agentLaunched reports a finished launch and offers to attach to what it
// started.
func (a *App) agentLaunched(msg agentLaunchedMsg) (tea.Model, tea.Cmd) {
	a.busy = false
	if msg.err != nil {
		a.note, a.err = "", msg.err
		return a, nil
	}
	cmd := a.openForm(newAttachForm(msg.slice.Name, msg.session))
	a.note = fmt.Sprintf("Launched %s for %q.", msg.session, msg.slice.Name)
	return a, tea.Batch(cmd, a.refreshLive())
}

// expandHome expands a leading ~ to the user's home directory. tmux is handed
// the path as-is, and the shell that would otherwise expand it never sees it.
func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

// existingDir validates the launch form's working directory. A session started
// somewhere that is not there fails inside tmux, where nobody is looking, so
// the directory is checked while there is still a form to say so on.
func existingDir(path string) error {
	dir := expandHome(strings.TrimSpace(path))
	if dir == "" {
		return errors.New("the agent needs a working directory")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("%s is not there", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	return nil
}
