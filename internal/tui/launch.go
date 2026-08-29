package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// AgentLauncher is what the launch flow needs of tmux: which slices have an
// agent running and in which session, how each of them is getting on, how to
// start one, how to say something to one — which is how the diff screen's review
// comments reach the agent that wrote the branch — the two commands that
// attach to one — the hidden client the embedded viewer runs, and the
// full-screen attach behind the hatch — and the reconcile that re-homes what an
// earlier run left joined beside a board. It is an interface so the flow can be
// driven without a tmux server.
type AgentLauncher interface {
	LiveSlices() (map[string]string, error)
	Activity() (map[string]agent.Activity, error)
	Launch(session, workdir, promptFile, sliceID string, model config.AgentModel) error
	SendPrompt(session, text string) error
	AttachClientCmd(session string) *exec.Cmd
	AttachCmd(session string) *exec.Cmd
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
	//
	// toast is what the status bar has to say about where the session was put:
	// the reason a worktree was not made, or the failure that stopped one being
	// made at all. A message carrying that failure names no session, since
	// nothing was launched.
	agentLaunchedMsg struct {
		slice   domain.Slice
		session string
		attach  bool
		err     error
		toast   string
		sev     severity
	}
	// liveSessionsMsg carries the slices with an agent running, each mapped to
	// the session it is running in, or the read that failed instead.
	liveSessionsMsg struct {
		live map[string]string
		err  error
	}
	// liveTickMsg is the periodic prod to re-read them.
	liveTickMsg struct{}
	// agentAttachedMsg reports the terminal handed to a session and given back,
	// naming the slice that session was about.
	agentAttachedMsg struct {
		note  string
		err   error
		slice string
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
// enter launches on them straight away; the directory and the model are only
// opened for editing when the user asks, because the defaults — the slice's own
// repo or the project's, and the model the config names for slice work — are
// nearly always right.
type LaunchForm struct {
	form    *huh.Form
	heading string

	slice   domain.Slice
	workdir string
	model   config.AgentModel
	action  launchAction
}

// newLaunchForm returns the form for launching an agent on a slice, showing
// the working directory the config resolves to and the model it names for
// slice work.
func newLaunchForm(theme huh.Theme, s domain.Slice, workdir string, m config.AgentModel) *LaunchForm {
	f := &LaunchForm{heading: "Launch an agent for " + s.Name, slice: s, workdir: workdir, model: m}
	f.form = newForm(theme,
		huh.NewGroup(
			huh.NewSelect[launchAction]().
				Title("Working directory: "+workdir).
				Description(modelSummary(m)).
				Options(
					huh.NewOption("Launch and show the agent", actionLaunch),
					huh.NewOption("Edit the options first", actionEdit),
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
		huh.NewGroup(append([]huh.Field{
			huh.NewInput().
				Title("Working directory").
				Description("Where the agent's session starts; ~ is expanded.").
				Value(&f.workdir).
				Validate(existingDir),
		}, modelFields(&f.model)...)...).
			WithHideFunc(func() bool { return f.action != actionEdit }),
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

// effortLevels are what Claude Code's --effort takes, in the order it lists
// them.
var effortLevels = []string{"low", "medium", "high", "xhigh", "max"}

// modelFields are the two fields every launch form carries: which Claude Code
// the session runs as. Both are prefilled from the config pair the flow is
// launching on and both may be left empty, which is the launch saying nothing
// and letting Claude Code decide.
//
// The model is typed because the aliases change faster than this binary does;
// the effort is chosen, because it is a fixed set the CLI rejects anything
// outside of. An effort the config names that is not in that set is offered
// too rather than dropped: it is what the user asked for, and this form is not
// the place to find out the CLI disagrees.
func modelFields(m *config.AgentModel) []huh.Field { return modelFieldsFor("", m) }

// modelFieldsFor is the same pair titled for a form that carries more than one
// of them: the settings form asks for the slice agent's and the planning
// agent's together, where "Model" twice over would say nothing about which is
// which. An empty label is the launch form's single pair, titled plainly.
func modelFieldsFor(what string, m *config.AgentModel) []huh.Field {
	model, effort := "Model", "Effort"
	if what != "" {
		model, effort = what+" model", what+" effort"
	}
	options := []huh.Option[string]{huh.NewOption("Claude Code's own default", "")}
	for _, level := range effortLevels {
		options = append(options, huh.NewOption(level, level))
	}
	if m.Effort != "" && !slices.Contains(effortLevels, m.Effort) {
		options = append(options, huh.NewOption(m.Effort, m.Effort))
	}
	return []huh.Field{
		huh.NewInput().
			Title(model).
			Description("An alias (sonnet, opus) or a full name; empty leaves it to Claude Code.").
			Value(&m.Model),
		huh.NewSelect[string]().
			Title(effort).
			Options(options...).
			Value(&m.Effort),
	}
}

// modelSummary is the model pair as the launch prompt shows it, so the user can
// see what enter is about to launch on without opening the options.
func modelSummary(m config.AgentModel) string {
	model, effort := m.Model, m.Effort
	if model == "" {
		model = "default"
	}
	if effort == "" {
		effort = "default"
	}
	return fmt.Sprintf("Model: %s · effort: %s", model, effort)
}

// launchNote is what the status bar says while a session starts, whichever way
// the launch was asked for.
const launchNote = "Launching the agent…"

// busyNote says what the status bar shows while the session starts.
func (f *LaunchForm) busyNote() string { return launchNote }

// save starts the session the completed form describes. Attaching is the
// default; only the background launch leaves the pane unshown.
func (f *LaunchForm) save(a *App) tea.Cmd {
	return a.startAgent(f.slice, f.workdir, f.model, f.action != actionBackground)
}

// startAgent is the launch itself, shared by the prompt's default choice and
// the options form behind its other one: the directory as resolved, expanded,
// and the project the flow was opened against — the flows only ever open on a
// configured one, so this is that project.
func (a *App) startAgent(s domain.Slice, workdir string, m config.AgentModel, attach bool) tea.Cmd {
	project, _ := a.activeProject()
	return launchAgent(a.launcher, newWorktrees(), newRepo(), a.client, a.cfg.AssigneeUserID, agent.PromptContext{
		Slice:        s,
		Project:      project,
		ProjectID:    a.cfg.ActiveProjectID,
		WorkingDir:   expandHome(strings.TrimSpace(workdir)),
		AssigneeName: a.cfg.AssigneeUserName,
	}, trimModel(m), attach)
}

// trimModel is the model pair as a launch sends it: what the user typed, with
// the spaces around it gone, since a flag value of " sonnet" is not one Claude
// Code answers to.
func trimModel(m config.AgentModel) config.AgentModel {
	return config.AgentModel{
		Model:  strings.TrimSpace(m.Model),
		Effort: strings.TrimSpace(m.Effort),
	}
}

// launchAgent gives the agent a worktree, writes its prompt out, claims the
// slice and starts the detached session that reads it. The claim is the board's
// rather than the agent's: a fresh Claude Code takes seconds to get as far as
// start-slice, and a row that reads Todo all the while is one the user can
// launch a second agent on. The prompt still tells the agent to run start-slice,
// which re-opens the slice it was launched on and prints the brief — and which
// refuses where somebody else holds it, so a race is settled before any agent is
// handed a brief.
//
// The claim goes last of the things that can fail before tmux is asked for
// anything, so a worktree that could not be cut or a prompt that could not be
// written leaves the slice where it was; a launch that fails after it leaves the
// slice in progress with no session, which is the state R releases. It is
// refused with a toast rather than the error banner, for the reason a worktree
// failure is: nothing has gone wrong with the board, and the slice is still
// there to launch.
//
// The worktree is resolved here rather than by the caller because it fetches
// origin and then cuts the worktree, which is a checkout and runs the
// repository's hooks over it: a launch is already the slow key, and this is the
// goroutine it is slow in — the board redraws throughout. Its
// answer is the working directory the prompt is written with and the session is
// started in, so the two never disagree about where the agent is.
func launchAgent(l AgentLauncher, w Worktrees, r Repo, client NotionAPI, assigneeID string,
	c agent.PromptContext, m config.AgentModel, attach bool) tea.Cmd {
	return func() tea.Msg {
		p := placeAgent(w, r, c.WorkingDir, c.Slice)
		if !p.ok {
			return agentLaunchedMsg{toast: p.toast, sev: p.sev}
		}
		c.WorkingDir, c.Branch, c.Repo = p.dir, p.branch, p.repo
		session := agent.SessionName(c.Slice.ID)
		file, err := agent.WritePromptFile(session, agent.Prompt(c))
		if err != nil {
			return agentLaunchedMsg{err: fmt.Errorf("launch agent: %w", err)}
		}
		if err := claimSlice(context.Background(), client, c.Slice, assigneeID); err != nil {
			return agentLaunchedMsg{toast: fmt.Sprintf("Could not %v — no agent was launched.", err), sev: sevError}
		}
		if err := l.Launch(session, c.WorkingDir, file, c.Slice.ID, m); err != nil {
			return agentLaunchedMsg{err: err}
		}
		return agentLaunchedMsg{slice: c.Slice, session: session, attach: attach, toast: p.toast, sev: p.sev}
	}
}

// attach hands the terminal to a session until the user detaches from it.
func attach(l AgentLauncher, sliceID, session string) tea.Cmd {
	return tea.ExecProcess(l.AttachCmd(session), attached(sliceID, session))
}

// attached is what getting the terminal back reports. Detaching is the ordinary
// way out of a session, and leaves the agent running. The message names the
// slice the session was about, so the board refetches the page the agent has
// been working on rather than the whole plan.
func attached(sliceID, session string) func(error) tea.Msg {
	return func(err error) tea.Msg {
		if err != nil {
			return agentAttachedMsg{err: fmt.Errorf("attach to %s: %w", session, err)}
		}
		return agentAttachedMsg{note: fmt.Sprintf("Detached from %s.", session), slice: sliceID}
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
//
// A blocked slice is refused too, and refused with a toast rather than the
// error banner a failure gets: nothing has gone wrong, and the slice is still
// there to launch once the slices it waits on are Done. The toast names them,
// since the chip on the row only says that there are some.
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
	if blockers := a.board.Blockers(s); len(blockers) > 0 {
		return a.showToast(fmt.Sprintf("%q waits on %s.", s.Name, blockerList(blockers)), sevWarning)
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
		return a.openForm(newLaunchForm(a.styles.FormTheme, s, workdir, a.cfg.SliceAgent))
	}
	if err := existingDir(workdir); err != nil {
		return a.showConfirm(fmt.Sprintf("Cannot launch an agent for %q: %v.", s.Name, err), sevError)
	}
	a.busy, a.note = true, launchNote
	return a.startAgent(s, workdir, a.cfg.SliceAgent, true)
}

// blockerList names the slices a blocked one is waiting on, in the order it
// names them and with the status each is at, so the toast says both what the
// wait is on and how far off it is.
func blockerList(blockers []domain.Slice) string {
	parts := make([]string, len(blockers))
	for i, b := range blockers {
		parts[i] = fmt.Sprintf("%q (%s)", b.Name, statusWord(b))
	}
	return strings.Join(parts, ", ")
}

// workdirFor is the directory a slice's agent starts in: its own repo
// override, or the project's default.
func workdirFor(s domain.Slice, p config.ProjectConfig) string {
	if s.Repo != "" {
		return s.Repo
	}
	return p.WorkingDir
}

// attachAgentFlow toggles the agent of the slice the cursor is on into the
// terminal beside the board. Any slice with a live session can be shown,
// whatever its status: an agent is worth watching from the moment it starts
// until it exits, and it spends nearly all of that time holding a slice it has
// already claimed.
//
// The close is checked before the session is, so a terminal already on the
// board hides on the same key that opened it rather than being refused because
// the live map has meanwhile stopped naming its session.
//
// Nothing is marked busy: the terminal is a widget on the board, not something
// the board is waiting on, and the plan behind it stays live throughout.
func (a *App) attachAgentFlow() tea.Cmd {
	if a.launcher == nil {
		return nil
	}
	s, ok := a.board.SelectedSlice()
	if !ok {
		return a.showConfirm("Move to a slice to attach to its agent.", sevWarning)
	}
	if a.viewer != nil && a.viewer.sliceID == s.ID {
		return a.closeViewer()
	}
	session := a.live[s.ID]
	if session == "" {
		return a.showConfirm(fmt.Sprintf("No agent session is running for %q.", s.Name), sevWarning)
	}
	return a.openAgentViewer(s.ID, s.Name, session)
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
	var cmds []tea.Cmd
	if msg.err != nil {
		a.live = nil
		cmds = append(cmds, a.showToast(fmt.Sprintf("Could not read tmux panes: %v", msg.err), sevError))
	} else {
		a.live = msg.live
		// An agent on show that is no longer live has exited, and the client in
		// the viewer is on its way out behind it: whichever of the two the board
		// hears about first closes the terminal, and the other finds nothing on
		// show. A failed read proves no such thing, so it leaves the viewer
		// alone.
		if a.viewer != nil && a.live[a.viewer.sliceID] == "" {
			cmds = append(cmds, a.viewerExited(nil))
		}
	}
	// A classification of an agent that has gone says nothing about the one
	// that takes its place on the slice, so it goes with the agent.
	for id := range a.activity {
		if a.live[id] == "" {
			delete(a.activity, id)
		}
	}
	a.board.SetLive(a.live)
	a.board.SetActivity(a.activity)
	// An agent found where there was none arms the watcher; it stops itself
	// again once the last one has gone.
	cmds = append(cmds, a.startWatch(), a.startPulse())
	a.syncBoard()
	// A planning agent that has exited has been editing the plan, so the board
	// re-reads it rather than showing what was there before the session.
	if msg.err == nil && planWasLive && a.live[agent.PlanSentinel] == "" {
		cmds = append(cmds, a.startLoad())
	}
	return tea.Batch(cmds...)
}

// agentLaunched takes a finished launch to the terminal beside the board:
// attaching is the default, so the session is shown straight away — t toggles
// it from there. A launch sent to the background is confirmed instead, naming
// the key that attaches, so the session is not left running unannounced.
//
// The toast about where the session was put goes up either way, since it is not
// about the launch succeeding: it is the difference between an agent on a
// branch of its own and one in the checkout the user is also working in. A
// launch that named no session is one the worktree or the claim stopped, and
// the toast is the whole report.
func (a *App) agentLaunched(msg agentLaunchedMsg) (tea.Model, tea.Cmd) {
	a.busy = false
	if msg.err != nil {
		a.note, a.err = "", msg.err
		return a, nil
	}
	var cmds []tea.Cmd
	if msg.toast != "" {
		cmds = append(cmds, a.showToast(msg.toast, msg.sev))
	}
	if msg.session == "" {
		a.note = ""
		return a, tea.Batch(cmds...)
	}
	// The launch claimed the slice, so the row it was pressed on is a page
	// behind: it is refetched the way any other write's row is, rather than the
	// whole plan reloaded. The planning agent comes through here too and has no
	// slice — the sentinel names no page there would be anything to read.
	if a.project != nil && msg.slice.ID != agent.PlanSentinel {
		cmds = append(cmds, a.refreshSlice(msg.slice.ID))
	}
	if !msg.attach {
		cmds = append(cmds, a.showConfirm(fmt.Sprintf("Launched %s for %q — t attaches.", msg.session, msg.slice.Name), sevSuccess))
		return a, tea.Batch(append(cmds, a.refreshLive())...)
	}
	a.note = ""
	cmds = append(cmds, a.openAgentViewer(msg.slice.ID, msg.slice.Name, msg.session), a.refreshLive())
	return a, tea.Batch(cmds...)
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
