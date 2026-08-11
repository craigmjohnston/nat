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
	// started for — with whether its pane should be shown straight away — or
	// the error that stopped it.
	agentLaunchedMsg struct {
		slice   domain.Slice
		session string
		attach  bool
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

// launchAction is the one choice the launch screen asks for. Launching on the
// defaults comes first, so a single enter starts the agent and shows its pane;
// editing and the background launch are the exceptions, a keystroke away.
type launchAction int

const (
	actionLaunch launchAction = iota
	actionEdit
	actionBackground
)

// LaunchForm is the modal behind l. The resolved defaults are on display and
// enter launches on them straight away; the directory is only opened for
// editing when the user asks, because the default — the slice's own repo, or
// the project's — is nearly always right.
type LaunchForm struct {
	form    *huh.Form
	heading string

	slice   domain.Slice
	workdir string
	action  launchAction
}

// newLaunchForm returns the form for launching an agent on a slice, showing
// the working directory the config resolves to.
func newLaunchForm(theme huh.Theme, s domain.Slice, workdir string) *LaunchForm {
	f := &LaunchForm{heading: "Launch an agent for " + s.Name, slice: s, workdir: workdir}
	f.form = newForm(theme,
		huh.NewGroup(
			huh.NewSelect[launchAction]().
				Title("Working directory: "+workdir).
				Options(
					huh.NewOption("Launch and show the agent", actionLaunch),
					huh.NewOption("Edit the working directory first", actionEdit),
					huh.NewOption("Launch in the background", actionBackground),
				).
				Value(&f.action).
				// A launch on the defaults skips the input and its check, so
				// the directory is validated here instead — an edit is headed
				// for the input, which checks what is actually typed.
				Validate(func(a launchAction) error {
					if a == actionEdit {
						return nil
					}
					return existingDir(f.workdir)
				}),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Working directory").
				Description("Where the agent's session starts; ~ is expanded.").
				Value(&f.workdir).
				Validate(existingDir),
		).WithHideFunc(func() bool { return f.action != actionEdit }),
	)
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

// launchNote is what the status bar says while a session starts, whichever way
// the launch was asked for.
const launchNote = "Launching the agent…"

// busyNote says what the status bar shows while the session starts.
func (f *LaunchForm) busyNote() string { return launchNote }

// save starts the session the completed form describes. Attaching is the
// default; only the background launch leaves the pane unshown.
func (f *LaunchForm) save(a *App) tea.Cmd {
	return a.startAgent(f.slice, f.workdir, f.action != actionBackground)
}

// startAgent is the launch itself, shared by the prompt's default choice and
// the options form behind its other one: the directory as resolved, expanded,
// and the project the flow was opened against — the flows only ever open on a
// configured one, so this is that project.
func (a *App) startAgent(s domain.Slice, workdir string, attach bool) tea.Cmd {
	project, _ := a.activeProject()
	return launchAgent(a.launcher, agent.PromptContext{
		Slice:        s,
		Project:      project,
		WorkingDir:   expandHome(strings.TrimSpace(workdir)),
		AssigneeName: a.cfg.AssigneeUserName,
	}, attach)
}

// launchAgent writes the agent's prompt out and starts the detached session
// that reads it. Nothing in Notion is touched: the agent claims its own slice,
// which is what keeps the claim honest when two of them race.
func launchAgent(l AgentLauncher, c agent.PromptContext, attach bool) tea.Cmd {
	return func() tea.Msg {
		session := agent.SessionName(c.Slice.ID)
		file, err := agent.WritePromptFile(session, agent.Prompt(c))
		if err != nil {
			return agentLaunchedMsg{err: fmt.Errorf("launch agent: %w", err)}
		}
		if err := l.Launch(session, c.WorkingDir, file, c.Slice.ID); err != nil {
			return agentLaunchedMsg{err: err}
		}
		return agentLaunchedMsg{slice: c.Slice, session: session, attach: attach}
	}
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

// The choices the launch prompt offers, in the order they read. Launching on
// the shown defaults comes first, so one enter starts the agent; configuring is
// a step to the right, and reaches the options form.
var launchChoices = []string{"launch", "configure & launch"}

const (
	choiceLaunch = iota
	choiceConfigure
)

// launchAgentFlow anchors the launch prompt to the slice the cursor is on. Only
// a Todo slice can be launched: a slice in progress is work an agent already
// holds, and Done is finished — a second agent on either would fight the first.
func (a *App) launchAgentFlow() tea.Cmd {
	project, ok := a.activeProject()
	if !ok || a.launcher == nil || a.busy {
		return nil
	}
	s, ok := a.board.SelectedSlice()
	if !ok {
		return a.showConfirm("Move to a slice to launch an agent for it.", sevWarning)
	}
	if a.live[s.ID] != "" {
		return a.showConfirm(fmt.Sprintf("An agent is already running for %q — press t to attach.", s.Name), sevWarning)
	}
	if s.Status != domain.SliceTodo {
		return a.showConfirm(fmt.Sprintf("%q is %s — only Todo slices can be launched.", s.Name, statusWord(s)), sevWarning)
	}
	workdir := workdirFor(s, project)
	return a.openPrompt(launchChoices, func(choice int) tea.Cmd {
		return a.launchChosen(s, workdir, choice)
	})
}

// launchChosen is what answering the prompt does: configuring opens the options
// form, and the default launches on the spot, on the directory the config
// resolved to.
//
// That directory is checked here rather than left to tmux, where a session
// started somewhere that is not there fails with nobody looking. The refusal
// takes the prompt's place on the row, so l and the other choice are the way
// past it.
func (a *App) launchChosen(s domain.Slice, workdir string, choice int) tea.Cmd {
	if choice == choiceConfigure {
		return a.openForm(newLaunchForm(a.styles.FormTheme, s, workdir))
	}
	if err := existingDir(workdir); err != nil {
		return a.showConfirm(fmt.Sprintf("Cannot launch an agent for %q: %v.", s.Name, err), sevError)
	}
	a.busy, a.note = true, launchNote
	return a.startAgent(s, workdir, true)
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
		return a.showConfirm("Move to a slice to attach to its agent.", sevWarning)
	}
	session := a.live[s.ID]
	if session == "" {
		return a.showConfirm(fmt.Sprintf("No agent session is running for %q.", s.Name), sevWarning)
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
// ordinary case and says nothing; a failure is a toast rather than an error
// banner, because the board is still worth looking at and the agents it could
// not move are all still running.
func (a *App) straysReclaimed(msg straysReclaimedMsg) tea.Cmd {
	switch {
	case msg.err != nil:
		return a.showToast(fmt.Sprintf("Could not re-home the agents left by an earlier run: %v", msg.err), sevError)
	case msg.count == 1:
		return a.showToast("Re-homed 1 agent left joined by an earlier run.", sevSuccess)
	case msg.count > 1:
		return a.showToast(fmt.Sprintf("Re-homed %d agents left joined by an earlier run.", msg.count), sevSuccess)
	}
	return nil
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

// liveLoaded takes the sessions that came back to the board, returning the
// plan reload a planning agent's exit calls for. A failed read is a toast
// rather than an error banner: it is a background poll, and the plan is still
// worth looking at without knowing what is running.
func (a *App) liveLoaded(msg liveSessionsMsg) tea.Cmd {
	planWasLive := a.live[agent.PlanSentinel] != ""
	var failed tea.Cmd
	if msg.err != nil {
		a.live = nil
		failed = a.showToast(fmt.Sprintf("Could not read tmux panes: %v", msg.err), sevError)
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
	// A planning agent that has exited has been editing the plan, so the board
	// re-reads it rather than showing what was there before the session.
	if msg.err == nil && planWasLive && a.live[agent.PlanSentinel] == "" {
		return a.startLoad()
	}
	return failed
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

// agentLaunched takes a finished launch to its pane: attaching is the default,
// so the session is shown straight away — t toggles it from there. A launch
// sent to the background is confirmed instead, naming the key that attaches,
// so the session is not left running unannounced.
func (a *App) agentLaunched(msg agentLaunchedMsg) (tea.Model, tea.Cmd) {
	a.busy = false
	if msg.err != nil {
		a.note, a.err = "", msg.err
		return a, nil
	}
	if !msg.attach {
		confirm := a.showConfirm(fmt.Sprintf("Launched %s for %q — t attaches.", msg.session, msg.slice.Name), sevSuccess)
		return a, tea.Batch(confirm, a.refreshLive())
	}
	a.busy, a.note = true, ""
	return a, tea.Batch(a.showAgent(msg.slice, msg.session), a.refreshLive())
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
