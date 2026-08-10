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
// agent running and in which session, how to start one, how to show one beside
// the board, the command that attaches to it full-screen, and the two ways a
// joined pane is given back — on the way out, and on the way in after a run
// that never got to. It is an interface so the flow can be driven without a
// tmux server.
type AgentLauncher interface {
	LiveSlices() (map[string]string, error)
	Launch(session, workdir, promptFile, sliceID string) error
	ShowPane(sliceID, hostPane string, percent int) (bool, error)
	AttachCmd(session string) *exec.Cmd
	BreakOutJoined(hostPane string) (int, error)
	ReclaimStrays(hostPane string) (int, error)
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
	// When the message is a pane moving instead, slice names whose agent moved
	// and joined says whether it is now beside the board — which is what the
	// status bar's pane guidance follows.
	agentAttachedMsg struct {
		note   string
		err    error
		slice  string
		joined bool
	}
	// straysReclaimedMsg reports the startup reconcile: how many panes a
	// previous run had left joined, or the read that failed instead.
	straysReclaimedMsg struct {
		count int
		err   error
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
func newLaunchForm(theme huh.Theme, s domain.Slice, workdir string) *LaunchForm {
	f := &LaunchForm{heading: "Launch an agent for " + s.Name, slice: s, workdir: workdir}
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

// SetSize gives the form the room the window leaves it.
func (f *LaunchForm) SetSize(width, height int) {
	f.form = f.form.WithWidth(width).WithHeight(height)
}

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

	slice   domain.Slice
	session string

	confirmed bool
}

// newAttachForm returns the confirm for showing a freshly started agent.
func newAttachForm(theme huh.Theme, s domain.Slice, session string) *AttachForm {
	f := &AttachForm{heading: "Agent launched", slice: s, session: session}
	f.form = huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("Show the agent working %q now?", s.Name)).
			Description("It keeps running either way; t on the slice shows and hides it.").
			Value(&f.confirmed),
	)).WithTheme(theme)
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

// SetSize gives the form the room the window leaves it.
func (f *AttachForm) SetSize(width, height int) {
	f.form = f.form.WithWidth(width).WithHeight(height)
}

// busyNote is empty: attaching replaces the screen and declining writes
// nothing, so there is no progress worth announcing.
func (f *AttachForm) busyNote() string { return "" }

// save shows the agent, or — when the answer was no — says how to reach it
// later, so the session is not left running unnamed.
func (f *AttachForm) save(a *App) tea.Cmd {
	if !f.confirmed {
		note := "Running in the background — attach with: " + attachCommand(f.session)
		return func() tea.Msg { return agentAttachedMsg{note: note} }
	}
	return a.showAgent(f.slice, f.session)
}

// attachCommand is the shell command that attaches to a session from another
// terminal.
func attachCommand(session string) string {
	return fmt.Sprintf("%s attach-session -t %s", agent.TmuxBinary, session)
}

// showAgent is what the board does with a slice's agent: shows it in a pane
// beside the plan when the board is itself a tmux pane, and hands the whole
// terminal over when it is not.
//
// The full-screen attach is the fallback rather than the other way round
// because there is nowhere to put a second pane without a window to put it in —
// and attaching from inside tmux would be nesting a session in a pane, which
// tmux refuses.
func (a *App) showAgent(s domain.Slice, session string) tea.Cmd {
	host := agent.HostPane()
	if host == "" {
		return attach(a.launcher, session)
	}
	return showPane(a.launcher, s, host, a.cfg.SplitPercent())
}

// showPane joins the slice's agent in beside the board, or sends it back to a
// session of its own when it is already there. A join carries no note: the
// status bar's pane guidance takes over the moment the pane is beside the
// board, and says how to send it back.
func showPane(l AgentLauncher, s domain.Slice, host string, percent int) tea.Cmd {
	return func() tea.Msg {
		joined, err := l.ShowPane(s.ID, host, percent)
		switch {
		case err != nil:
			return agentAttachedMsg{err: fmt.Errorf("show the agent for %q: %w", s.Name, err)}
		case joined:
			return agentAttachedMsg{slice: s.ID, joined: true}
		default:
			return agentAttachedMsg{note: fmt.Sprintf("Sent the agent for %q back to %s.", s.Name, agent.SessionName(s.ID)),
				slice: s.ID}
		}
	}
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
	return a.openForm(newLaunchForm(a.styles.FormTheme, s, workdirFor(s, project)))
}

// workdirFor is the directory a slice's agent starts in: its own repo
// override, or the project's default.
func workdirFor(s domain.Slice, p config.ProjectConfig) string {
	if s.Repo != "" {
		return s.Repo
	}
	return p.WorkingDir
}

// attachAgentFlow shows the agent of the slice the cursor is on, beside the
// board or full-screen. Any slice with a live session can be shown, whatever
// its status: an agent is worth watching from the moment it starts until it
// exits, and it spends nearly all of that time holding a slice it has already
// Claimed.
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
	return a.showAgent(s, session)
}

// Release sends the agents joined into the board's window back to sessions of
// their own. The window goes when the board does, and it would take a pane
// still in it along — so this is the last thing the app owes anything, and the
// caller runs it however it is leaving, quit or panic.
//
// A board that is not itself a tmux pane has never joined anything: there is
// nothing to give back, and nothing to ask tmux about.
func (a *App) Release() error {
	host := agent.HostPane()
	if a.launcher == nil || host == "" {
		return nil
	}
	if _, err := a.launcher.BreakOutJoined(host); err != nil {
		return fmt.Errorf("return the agents to their own sessions: %w", err)
	}
	return nil
}

// reclaimStrays kicks off the startup reconcile, re-homing the panes a run
// that died left joined. It is skipped when the board is not a pane itself:
// such a board joins nothing, and the panes it would find are another board's
// to look after.
func (a *App) reclaimStrays() tea.Cmd {
	host := agent.HostPane()
	if a.launcher == nil || host == "" {
		return nil
	}
	l := a.launcher
	return func() tea.Msg {
		count, err := l.ReclaimStrays(host)
		return straysReclaimedMsg{count: count, err: err}
	}
}

// straysReclaimed reports what the reconcile found. Finding nothing is the
// ordinary case and says nothing; a failure is a note rather than an error
// banner, because the board is still worth looking at and the agents it could
// not move are all still running.
func (a *App) straysReclaimed(msg straysReclaimedMsg) {
	switch {
	case msg.err != nil:
		a.note = fmt.Sprintf("Could not re-home the agents left by an earlier run: %v", msg.err)
	case msg.count == 1:
		a.note = "Re-homed 1 agent left joined by an earlier run."
	case msg.count > 1:
		a.note = fmt.Sprintf("Re-homed %d agents left joined by an earlier run.", msg.count)
	}
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
		// A joined agent that is no longer live has exited, taking its pane
		// with it — the pane guidance would be advice about nothing. A failed
		// read proves no such thing, so it does not touch the marks.
		for id := range a.joined {
			if a.live[id] == "" {
				delete(a.joined, id)
			}
		}
	}
	a.board.SetLive(a.live)
	a.syncBoard()
}

// paneMoved keeps the joined marks in step with a pane movement the message
// reports. Messages that moved nothing — an attach, a detach, a failure —
// name no slice and change nothing.
func (a *App) paneMoved(msg agentAttachedMsg) {
	if msg.slice == "" {
		return
	}
	if msg.joined {
		a.joined[msg.slice] = true
	} else {
		delete(a.joined, msg.slice)
	}
}

// agentLaunched reports a finished launch and offers to attach to what it
// started.
func (a *App) agentLaunched(msg agentLaunchedMsg) (tea.Model, tea.Cmd) {
	a.busy = false
	if msg.err != nil {
		a.note, a.err = "", msg.err
		return a, nil
	}
	cmd := a.openForm(newAttachForm(a.styles.FormTheme, msg.slice, msg.session))
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
