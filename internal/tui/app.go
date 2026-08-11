// Package tui is the Bubble Tea v2 interface: a root model that routes between
// screens, and the screens themselves.
package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// screen is one of the app's full-window views. The board is what the app is
// for; help and info are pushed over it and dismissed with esc.
type screen int

const (
	screenBoard screen = iota
	screenHelp
	screenInfo
	screenForm
)

// modal is a form shown over the board: it owns every key but esc, and once it
// completes the app dispatches the write it describes. save returns nil when a
// completed form asks for nothing — a confirm the user answered no to.
//
// SetSize is how a modal learns how much room it has. Left to itself huh sizes
// a form to the whole window, which is the wrong answer twice over: the app
// draws a frame and a heading around it, and the leftover columns are what the
// form actually gets.
type modal interface {
	Init() tea.Cmd
	Update(tea.Msg) tea.Cmd
	State() huh.FormState
	View() string
	Heading() string
	SetSize(width, height int)
	save(a *App) tea.Cmd
}

// The messages the root model's own Notion calls come back as. Every call is a
// tea.Cmd returning one of these, so nothing in Update blocks on the network.
type (
	// projectLoadedMsg carries a freshly loaded plan.
	projectLoadedMsg struct{ project domain.Project }
	// notionErrMsg carries a failed Notion call, already described.
	notionErrMsg struct{ err error }
	// infoLoadedMsg carries the project page body, already converted to
	// markdown; infoErrMsg the fetch that failed instead. The info screen
	// reports its own failures, so they do not go through notionErrMsg.
	infoLoadedMsg struct{ markdown string }
	infoErrMsg    struct{ err error }
)

// keyMap is the app's global key bindings. Screens own their own navigation
// keys; these work wherever the user is.
type keyMap struct {
	ForceQuit key.Binding
	Quit      key.Binding
	Refresh   key.Binding
	Help      key.Binding
	Info      key.Binding
	Back      key.Binding
	Dismiss   key.Binding
}

// defaultKeyMap returns the bindings the app runs with.
func defaultKeyMap() keyMap {
	return keyMap{
		ForceQuit: key.NewBinding(key.WithKeys("ctrl+c")),
		Quit:      key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "quit")),
		Refresh:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Info:      key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "info")),
		Back:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Dismiss:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "dismiss")),
	}
}

// hint is one key binding of the status bar, with the order it goes in as the
// bar runs out of room: rank 1 is dropped first.
type hint struct {
	binding key.Binding
	rank    int
}

// statusHints are the bindings the status bar draws, in the order they read.
// They are dropped by rank rather than truncated, so a narrow bar loses whole
// hints from the middle and the two that matter most — quit, and the refresh
// that is the only way to see what an agent has done — survive longest.
func (k keyMap) statusHints() []hint {
	return []hint{
		{k.Refresh, 4},
		{k.Info, 2},
		{k.Help, 3},
		{k.Back, 1},
		{k.Quit, 5},
	}
}

// helpBindings are the bindings listed to the user, in the order they read.
func (k keyMap) helpBindings() []key.Binding {
	hints := k.statusHints()
	bindings := make([]key.Binding, len(hints))
	for i, h := range hints {
		bindings[i] = h.binding
	}
	return bindings
}

// App is the root model. It owns the config, the Notion client, the loaded
// plan, and whichever screen is on show, routing messages to it.
//
// The first-run wizard is held separately rather than as a screen: it runs
// before there is a config to show a board for, and it hands over exactly once.
type App struct {
	cfg    config.Config
	client NotionAPI
	styles Styles
	keys   keyMap

	onboarding *Onboarding
	screen     screen
	board      Board
	info       Info
	form       modal

	// boardVP scrolls the board's rows, which the board itself draws in full: a
	// plan taller than the window is the layout's problem, not the board's.
	// helpVP does the same for the help screen, whose content never changes and
	// so is set once.
	boardVP viewport.Model
	helpVP  viewport.Model

	// launcher starts and attaches to the agents' tmux sessions, and live maps
	// each slice it last reported an agent running for to that agent's session.
	launcher AgentLauncher
	live     map[string]string
	// joined marks the slices whose agent pane is beside the board right now.
	// While any is, the status bar swaps its hints for the pane guidance.
	joined map[string]bool

	project *domain.Project
	loading bool
	// busy is a write, or the read that opens a form, in flight. Only one runs
	// at a time: a second would race the first over the same page.
	busy    bool
	spinner spinner.Model

	err error
	// note is progress in flight — "Saving…" — cleared when the work lands.
	// What lands reports itself as an inline confirmation on the board row it
	// was about, or, when it is not about a row, as a toast here on the bar;
	// both auto-dismiss, on the timer their id ties them to.
	note      string
	toast     string
	toastSev  severity
	toastID   int
	confirmID int

	width, height int
}

var _ tea.Model = (*App)(nil)

// NewApp returns the root model showing the board, for a workspace that is
// already set up.
func NewApp(cfg config.Config, client NotionAPI) *App {
	s := DefaultStyles()
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(s.Spinner))
	a := &App{cfg: cfg, client: client, styles: s, keys: defaultKeyMap(), spinner: sp,
		board: NewBoard(s), info: NewInfo(s), launcher: newLauncher(), joined: map[string]bool{},
		boardVP: viewport.New(), helpVP: viewport.New()}
	a.helpVP.SetContent(a.helpBody())
	return a
}

// NewAppWithOnboarding returns the root model showing the first-run wizard,
// which hands over to the board when it finishes.
func NewAppWithOnboarding(cfg config.Config, client NotionAPI, o *Onboarding) *App {
	a := NewApp(cfg, client)
	a.onboarding = o
	return a
}

// setStyles swaps in a freshly built palette, reaching every widget that took
// a copy of the styles at construction. Modal forms are built from a.styles as
// they open, and the background answer arrives before any could, so they need
// no hand-me-down.
func (a *App) setStyles(s Styles) {
	a.styles = s
	a.spinner.Style = s.Spinner
	a.board.styles = s
	a.info.styles = s
	if a.onboarding != nil {
		a.onboarding.SetStyles(s)
	}
}

// Init starts the screen on show: the wizard's first call, or the first load of
// the active project's plan, alongside the poll that marks the slices an agent
// is already running on and the reconcile that re-homes the panes an earlier
// run left joined.
func (a *App) Init() tea.Cmd {
	// The background query goes out first: the styles start on the dark
	// palette, and the answer swaps in the light one when the terminal says so.
	if a.onboarding != nil {
		return tea.Batch(tea.RequestBackgroundColor, a.onboarding.Init())
	}
	return tea.Batch(tea.RequestBackgroundColor,
		a.startLoad(), a.refreshLive(), liveTick(), a.reclaimStrays())
}

// Update handles the global keys and the app's own messages, and routes
// everything else to the screen on show.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Recorded, then handed on: every band of the layout is sized from it.
		a.width, a.height = msg.Width, msg.Height
		a.resize()
	case tea.KeyPressMsg:
		return a.keyPressed(msg)
	case tea.BackgroundColorMsg:
		a.setStyles(NewStyles(msg.IsDark()))
		return a, nil
	case OnboardingDoneMsg:
		return a.onboardingDone(msg)
	case projectLoadedMsg:
		a.project, a.loading, a.err = &msg.project, false, nil
		a.board.SetProject(a.project)
		// The first plan brings the bar with it, which the board's viewport has
		// to give its lines up to; resize re-shares them and re-syncs the board.
		a.resize()
		return a, nil
	case notionErrMsg:
		a.loading, a.err = false, msg.err
		return a, nil
	case infoLoadedMsg:
		a.info.SetMarkdown(msg.markdown)
		return a, nil
	case infoErrMsg:
		a.info.Fail(msg.err)
		return a, nil
	case sliceBodyMsg:
		return a.sliceBodyLoaded(msg)
	case sliceSavedMsg:
		return a.saved(msg.note, msg.err)
	case milestoneSavedMsg:
		return a.saved(msg.note, msg.err)
	case projectCreatedMsg:
		return a.projectCreated(msg)
	case projectSwitchedMsg:
		return a.projectSwitched(msg)
	case agentLaunchedMsg:
		return a.agentLaunched(msg)
	case agentAttachedMsg:
		a.paneMoved(msg)
		// The agent has had the terminal to itself, so the plan it was working on
		// is reloaded rather than trusted. The planning agent's pane is about the
		// plan rather than any row, so its report is a toast, not a row confirm.
		if msg.slice == agent.PlanSentinel && msg.err == nil && msg.note != "" {
			a.busy = false
			return a, tea.Batch(a.startLoad(), a.refreshLive(), a.showToast(msg.note, sevSuccess))
		}
		model, cmd := a.saved(msg.note, msg.err)
		return model, tea.Batch(cmd, a.refreshLive())
	case liveSessionsMsg:
		return a, a.liveLoaded(msg)
	case straysReclaimedMsg:
		return a, a.straysReclaimed(msg)
	case liveTickMsg:
		return a, tea.Batch(a.refreshLive(), liveTick())
	case toastGoneMsg:
		a.toastGone(msg)
		return a, nil
	case confirmGoneMsg:
		a.confirmGone(msg)
		return a, nil
	case spinner.TickMsg:
		if !a.loading && !a.info.Busy() {
			return a, nil
		}
		sp, cmd := a.spinner.Update(msg)
		a.spinner = sp
		return a, cmd
	}

	if a.onboarding != nil {
		o, cmd := a.onboarding.Update(msg)
		a.onboarding = o
		return a, cmd
	}
	if a.form != nil {
		return a, a.formUpdate(msg)
	}
	return a, nil
}

// keyPressed handles a key press. ctrl+c always quits — the wizard's forms
// handle it themselves, but a wizard that failed has no form left to catch it —
// and the rest belong to the wizard while it is on show, so that typing into a
// form field does not steer the app.
func (a *App) keyPressed(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, a.keys.ForceQuit) {
		return a, tea.Quit
	}
	if a.onboarding != nil {
		o, cmd := a.onboarding.Update(msg)
		a.onboarding = o
		return a, cmd
	}
	// An open form owns every key but esc, which abandons it: q and enter are
	// answers to the form, not instructions to the app behind it. huh's own
	// abort key is ctrl+c, which quits the app above, so cancelling is handled
	// here rather than left to the form.
	if a.form != nil {
		if key.Matches(msg, a.keys.Back) {
			a.closeForm()
			// Coming back to the board is a chance to notice a session that
			// started, or ended, while the form was up.
			return a, tea.Batch(a.showToast("Cancelled.", sevWarning), a.refreshLive())
		}
		return a, a.formUpdate(msg)
	}

	switch {
	// esc means the nearest "undo" there is: clear the error, else leave the
	// screen that was pushed over the board, else quit.
	case a.err != nil && key.Matches(msg, a.keys.Dismiss):
		a.err = nil
	case a.screen != screenBoard && key.Matches(msg, a.keys.Back):
		a.screen = screenBoard
	case key.Matches(msg, a.keys.Quit):
		return a, tea.Quit
	case key.Matches(msg, a.keys.Refresh):
		// A refresh is a fresh look, so whatever was being reported goes.
		a.note, a.toast = "", ""
		a.board.ClearConfirm()
		// The project page is refreshed too — at once if the user is reading it,
		// otherwise lazily, on the next visit to the info screen.
		a.info.Reset()
		cmd := tea.Batch(a.startLoad(), a.refreshLive())
		if a.screen == screenInfo {
			cmd = tea.Batch(cmd, a.startInfoLoad())
		}
		return a, cmd
	case key.Matches(msg, a.keys.Help):
		a.screen = toggle(a.screen, screenHelp)
	case key.Matches(msg, a.keys.Info):
		a.screen = toggle(a.screen, screenInfo)
		if a.screen == screenInfo {
			return a, a.startInfoLoad()
		}
	default:
		// Anything the app itself does not want belongs to the screen the user
		// is looking at.
		switch a.screen {
		case screenBoard:
			if cmd, ok := a.boardWrite(msg); ok {
				return a, cmd
			}
			cmd := a.board.Update(msg)
			// The cursor may have left the part of the plan on show.
			a.syncBoard()
			return a, cmd
		case screenInfo:
			return a, a.info.Update(msg)
		case screenHelp:
			// The key list is longer than a short window, so it scrolls.
			vp, cmd := a.helpVP.Update(msg)
			a.helpVP = vp
			return a, cmd
		}
	}
	return a, nil
}

// boardWrite handles the board keys that act on what the cursor is on rather
// than move it, reporting whether the key was one of them. They live here
// rather than on the board because they need the client, the project config, or
// the launcher.
func (a *App) boardWrite(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, a.board.keys.Add):
		return a.addSlice(), true
	case key.Matches(msg, a.board.keys.Edit):
		return a.editSlice(), true
	case key.Matches(msg, a.board.keys.Move):
		return a.moveSliceFlow(), true
	case key.Matches(msg, a.board.keys.Delete):
		return a.deleteSliceFlow(), true
	case key.Matches(msg, a.board.keys.Queue):
		return a.queueMilestone(), true
	case key.Matches(msg, a.board.keys.Launch):
		return a.launchAgentFlow(), true
	case key.Matches(msg, a.board.keys.Attach):
		return a.attachAgentFlow(), true
	case key.Matches(msg, a.board.keys.Plan):
		return a.planAgentFlow(), true
	case key.Matches(msg, a.board.keys.NewProject):
		return a.newProjectFlow(), true
	case key.Matches(msg, a.board.keys.SwitchProject):
		return a.switchProjectFlow(), true
	}
	return nil, false
}

// canWrite reports whether a write can be started: a client to make it with, a
// project to make it against, and nothing already in flight.
func (a *App) canWrite() bool {
	if a.client == nil || a.busy {
		return false
	}
	_, ok := a.activeProject()
	return ok
}

// addSlice opens the form for a new slice under the milestone the cursor is on.
func (a *App) addSlice() tea.Cmd {
	if !a.canWrite() {
		return nil
	}
	m, ok := a.board.SelectedMilestone()
	if !ok {
		return a.showConfirm("Move to a milestone to add a slice under it.", sevWarning)
	}
	return a.openForm(newAddSliceForm(a.styles.FormTheme, m))
}

// editSlice opens the form for the slice the cursor is on, once its page body
// has been fetched to fill the brief in with. Claimed and Done slices are work
// in flight or finished, so they are refused rather than opened.
func (a *App) editSlice() tea.Cmd {
	if !a.canWrite() {
		return nil
	}
	s, ok := a.board.SelectedSlice()
	if !ok {
		return a.showConfirm("Move to a slice to edit it.", sevWarning)
	}
	if s.Status != domain.SliceTodo {
		return a.showConfirm(fmt.Sprintf("%q is %s — only Todo slices can be edited.", s.Name, s.Status), sevWarning)
	}
	a.busy, a.note = true, "Loading the slice…"
	return loadSliceBody(a.client, s)
}

// sliceBodyLoaded opens the edit form over the body that came back.
func (a *App) sliceBodyLoaded(msg sliceBodyMsg) (tea.Model, tea.Cmd) {
	a.busy, a.note = false, ""
	if msg.err != nil {
		a.err = msg.err
		return a, nil
	}
	return a, a.openForm(newEditSliceForm(a.styles.FormTheme, msg.slice, msg.markdown))
}

// openForm shows a form over the board, at the size the window it is opening
// into leaves for it.
func (a *App) openForm(f modal) tea.Cmd {
	a.form, a.screen, a.note = f, screenForm, ""
	f.SetSize(a.formSize())
	return f.Init()
}

// formHintsHeight is the blank line huh draws above its own key hints on top of
// the height it was given, and so the one line of the body band a form cannot
// be told about.
const formHintsHeight = 1

// formSize is the room an open form has: the body band, less the line huh
// spends without counting it. Before the first resize there is no window to
// measure, so the numbers come out non-positive — which is huh's own signal to
// size itself, and it does that from the resize that follows.
func (a *App) formSize() (width, height int) {
	return a.innerWidth(), a.bodyHeight() - formHintsHeight
}

// formUpdate feeds a message to the open form, writing what it says to Notion
// once it is complete.
func (a *App) formUpdate(msg tea.Msg) tea.Cmd {
	cmd := a.form.Update(msg)
	if a.form.State() == huh.StateCompleted {
		return a.saveForm()
	}
	return cmd
}

// saveForm dispatches the write the completed form describes, returning to the
// board while it is in flight. A form that asks for no write — a confirm
// answered no — is simply dismissed.
func (a *App) saveForm() tea.Cmd {
	f := a.form
	a.closeForm()
	cmd := f.save(a)
	if cmd == nil {
		return a.showToast("Cancelled.", sevWarning)
	}
	a.busy, a.note = true, busyNoteOf(f)
	return cmd
}

// busyNoter is a modal whose completed form does something other than save: the
// status bar says what that is, or nothing at all when there is nothing worth
// announcing.
type busyNoter interface{ busyNote() string }

// busyNoteOf is what the status bar shows while a modal's work is in flight.
func busyNoteOf(f modal) string {
	if n, ok := f.(busyNoter); ok {
		return n.busyNote()
	}
	return "Saving…"
}

// closeForm dismisses the form and goes back to the board.
func (a *App) closeForm() { a.form, a.screen = nil, screenBoard }

// saved reports a finished write and reloads the plan, so the board shows what
// was just written rather than what was there before. The writes that come
// through here are all about the row the cursor is on, so the report is an
// inline confirmation anchored to it.
func (a *App) saved(note string, err error) (tea.Model, tea.Cmd) {
	a.busy = false
	if err != nil {
		a.note, a.err = "", err
		return a, nil
	}
	a.note = ""
	cmds := []tea.Cmd{a.startLoad(), a.refreshLive()}
	if note != "" {
		cmds = append(cmds, a.showConfirm(note, sevSuccess))
	}
	return a, tea.Batch(cmds...)
}

// toggle switches to want, or back to the board if it is already on show.
func toggle(current, want screen) screen {
	if current == want {
		return screenBoard
	}
	return want
}

// onboardingDone takes over from the wizard with the config it wrote, and
// loads the plan if there is a project to load.
func (a *App) onboardingDone(msg OnboardingDoneMsg) (tea.Model, tea.Cmd) {
	a.cfg, a.onboarding, a.screen = msg.Config, nil, screenBoard
	if msg.NeedsProject {
		// Nothing to load, and nothing to look at until there is: the wizard hands
		// straight over to the flow that makes the first project.
		cmd := a.newProjectFlow()
		return a, tea.Batch(cmd, a.showToast("Setup complete. No projects yet — let's make one.", sevSuccess))
	}
	return a, tea.Batch(a.startLoad(), a.showToast("Setup complete.", sevSuccess))
}

// startLoad kicks off a load of the active project's plan, returning nil when
// there is nothing to load: an unconfigured or unknown active project is a
// state the board reports, not an error.
func (a *App) startLoad() tea.Cmd {
	project, ok := a.activeProject()
	if !ok || a.client == nil {
		return nil
	}
	a.loading, a.err = true, nil
	return tea.Batch(a.spinner.Tick, a.fetchProject(a.cfg.ActiveProjectID, project))
}

// startInfoLoad kicks off a fetch of the project page body, unless there is
// nothing to fetch or it has been fetched already: the page is the project's
// conventions, which do not change between keystrokes.
func (a *App) startInfoLoad() tea.Cmd {
	if _, ok := a.activeProject(); !ok || !a.info.NeedsLoad() || a.client == nil {
		return nil
	}
	a.info.Start()
	return tea.Batch(a.spinner.Tick, a.fetchInfo(a.cfg.ActiveProjectID))
}

// fetchInfo loads a page's body and converts it to markdown for the info
// screen to render.
func (a *App) fetchInfo(pageID string) tea.Cmd {
	client := a.client
	return func() tea.Msg {
		blocks, err := client.GetBlockChildren(context.Background(), pageID)
		if err != nil {
			return infoErrMsg{err: fmt.Errorf("load project page: %w", err)}
		}
		return infoLoadedMsg{markdown: notion.Markdown(blocks)}
	}
}

// activeProject returns the configured project the board shows.
func (a *App) activeProject() (config.ProjectConfig, bool) {
	if a.cfg.ActiveProjectID == "" {
		return config.ProjectConfig{}, false
	}
	p, ok := a.cfg.Projects[a.cfg.ActiveProjectID]
	return p, ok
}

// fetchProject loads a project's milestones and slices. Milestones come back in
// plan order and slices oldest first, which is the order agents pick them up
// in; the domain groups them from there.
func (a *App) fetchProject(id string, cfg config.ProjectConfig) tea.Cmd {
	client := a.client
	return func() tea.Msg {
		ctx := context.Background()
		milestones, err := client.QueryDataSource(ctx, cfg.MilestonesDSID, nil,
			[]notion.Sort{{Property: notion.PropOrder, Direction: notion.SortAscending}})
		if err != nil {
			return notionErrMsg{err: fmt.Errorf("load milestones: %w", err)}
		}
		slices, err := client.QueryDataSource(ctx, cfg.SlicesDSID, nil,
			[]notion.Sort{{Timestamp: notion.TimestampCreated, Direction: notion.SortAscending}})
		if err != nil {
			return notionErrMsg{err: fmt.Errorf("load slices: %w", err)}
		}
		return projectLoadedMsg{project: domain.Project{
			ID:         id,
			Name:       cfg.Name,
			Milestones: domain.MilestonesFromPages(milestones),
			Slices:     domain.SlicesFromPages(slices),
		}}
	}
}

// View renders the screen on show, full window.
func (a *App) View() tea.View {
	v := tea.NewView(a.content())
	v.AltScreen = true
	return v
}

// The layout's fixed measurements: the columns each band is held away from the
// window's edges by, the height of the heading bar, and the height of the
// status bar — one line bare, or three inside its box.
const (
	framePadX       = 2
	headerHeight    = 1
	statusHeight    = 1
	statusBoxHeight = statusHeight + 2
)

// content is the rendered screen, without the terminal-level settings: the
// heading bar, the body of the screen on show boxed in its border, and the
// status bar in its own box docked to the window's bottom rows. The bands are
// cut and padded to fill the window exactly, so nothing a screen draws can
// push the bar off the bottom.
func (a *App) content() string {
	if a.onboarding != nil {
		return a.onboarding.View()
	}
	if a.width <= 0 || a.height <= 0 {
		// Before the first resize there is no window to lay out to, so the bands
		// are simply drawn one after another at whatever size they come out.
		return a.headerView() + "\n" + a.body() + "\n" + a.statusBar()
	}
	var lines []string
	if a.headerBandHeight() > 0 {
		lines = append(lines, a.headerView())
	}
	if a.framed() {
		lines = append(lines, a.bodyRegion()...)
		return strings.Join(append(lines, a.statusRegion()...), "\n")
	}
	lines = append(lines, a.band(a.body(), a.bodyHeight())...)
	return strings.Join(append(lines, a.statusBar()), "\n")
}

// framed reports whether the window is big enough for the bordered layout: a
// border costs two lines and two columns per region, and below that the bands
// are drawn bare rather than boxed, so the content is never all frame.
func (a *App) framed() bool {
	return a.height >= headerHeight+statusBoxHeight+2 && a.width >= 2*framePadX+1
}

// band lays s out as exactly height lines of the window's width: indented from
// the window's edges, padded out, and cut rather than allowed to push the bands
// below it off the window.
func (a *App) band(s string, height int) []string {
	if height <= 0 {
		return nil
	}
	// Cut to width before padding out to it, so a long line is truncated rather
	// than wrapped onto a line the band has no room for.
	s = a.styles.Frame.Render(s)
	fill := lipgloss.NewStyle().Width(a.width)
	out := strings.Split(fill.Render(fit(s, a.width)), "\n")
	for len(out) < height {
		out = append(out, fill.Render(""))
	}
	return out[:height]
}

// statusBandHeight, headerBandHeight and the body heights are how the
// window's lines are shared out. The status bar takes the bottom rows first —
// boxed when the window is framed, bare when it is not — and the header what
// is left of its own height, because a window too short for all three is
// still worth telling the user where they are and what the keys do.
func (a *App) statusBandHeight() int {
	if a.framed() {
		return statusBoxHeight
	}
	return statusHeight
}

func (a *App) headerBandHeight() int {
	return min(headerHeight, max(a.height-a.statusBandHeight(), 0))
}

// bodyBoxHeight is the lines the body region occupies, border included;
// bodyHeight is the lines a screen can actually draw on inside it.
func (a *App) bodyBoxHeight() int {
	return max(a.height-a.statusBandHeight()-a.headerBandHeight(), 0)
}

func (a *App) bodyHeight() int {
	if a.framed() {
		return max(a.bodyBoxHeight()-2, 0)
	}
	return a.bodyBoxHeight()
}

// headerView is the top band: a full-width heading bar with the app's name as
// a segment of its own and the screen or project name beside it.
func (a *App) headerView() string {
	segment := a.styles.HeaderApp.Render(appName)
	name := a.headerName()
	if a.width <= 0 {
		// No window to spread across, so the segments simply sit together.
		if name == "" {
			return segment
		}
		return segment + " " + a.styles.HeaderTitle.Render(name)
	}
	left := segment
	if name != "" {
		left = fit(segment+a.styles.HeaderTitle.Render(" "+name), a.innerWidth())
	}
	line := strings.Repeat(" ", framePadX) + left
	return a.styles.Header.Width(a.width).Render(fit(line, a.width))
}

// appName is what the header and the board's mode chip call the app.
const appName = "nat"

// headerName is the header's second segment: the screen on show, or on the
// board the project it is showing. A board with no project loaded names
// nothing — the body says why.
func (a *App) headerName() string {
	switch a.screen {
	case screenHelp:
		return "Keys"
	case screenInfo:
		return "Info"
	case screenForm:
		return a.form.Heading()
	}
	if a.project == nil {
		return ""
	}
	return a.project.Name
}

// bodyRegion is the body band inside its border: the screen's content clipped
// to the box's interior — a body taller than the box would push the borders
// apart rather than scroll — and the box run out to the window's width.
func (a *App) bodyRegion() []string {
	// framed has already made sure the box has at least its own border lines.
	height := a.bodyBoxHeight()
	content := clipLines(fit(a.body(), a.innerWidth()), max(height-2, 0))
	// Width and Height count the border, so the box is sized to the window.
	box := a.styles.Box.Width(a.width).Height(height).Render(content)
	lines := strings.Split(box, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

// clipLines is the first n lines of s, or nothing at all when n is not
// positive.
func clipLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:max(n, 0)]
	}
	return strings.Join(lines, "\n")
}

// body renders the current screen.
func (a *App) body() string {
	switch a.screen {
	case screenHelp:
		return a.helpView()
	case screenInfo:
		return a.infoView()
	case screenForm:
		return a.form.View()
	default:
		return a.boardView()
	}
}

// boardView is the main screen: the project's segmented progress bar, and the
// plan under it scrolled to what is left of the body band. Loading and "there
// is nothing to show" are the root model's to report — the board only ever
// draws a plan, and those states have no plan for a bar to sum either.
func (a *App) boardView() string {
	switch {
	case a.loading:
		return a.spinner.View() + " Loading the plan…"
	case a.project == nil:
		return a.styles.Faint.Render(a.noProjectReason())
	case a.boardVP.Width() <= 0 || a.boardVP.Height() <= 0:
		// No band to scroll in yet, so every row is drawn.
		return a.progressBarView() + a.board.View()
	}
	return a.progressBarView() + a.boardVP.View()
}

// progressBarView is the bar over the board's rows: the plan segmented by
// milestone, as wide as the body's interior, label line included, and a blank
// line holding the rows off it — or nothing before the first resize, when
// there is no width to size the bar to.
func (a *App) progressBarView() string {
	bar := RenderProgressBar(a.styles, a.innerWidth(), SegmentsOf(a.project.Groups()))
	if bar == "" {
		return ""
	}
	return bar + "\n\n"
}

// progressBandLines is the body lines the bar takes from the board's viewport:
// the bar, its label and the blank line under them when there is a plan to
// sum, none without one — the other board states draw no bar — or before the
// first resize.
func (a *App) progressBandLines() int {
	if a.project == nil || a.innerWidth() <= 0 {
		return 0
	}
	return 3
}

// syncBoard puts the board's rows into the body's viewport and scrolls it the
// least it can to bring the cursor back on screen. The board draws every row it
// has; holding a plan taller than the window to the window is the layout's job.
func (a *App) syncBoard() {
	a.boardVP.SetContent(a.board.View())
	h := a.boardVP.Height()
	if h <= 0 {
		return
	}
	switch top, cursor := a.boardVP.YOffset(), a.board.Cursor(); {
	case cursor < top:
		a.boardVP.SetYOffset(cursor)
	case cursor >= top+h:
		a.boardVP.SetYOffset(cursor - h + 1)
	}
}

// resize hands the window's new measurements to the bands that size themselves
// from them.
func (a *App) resize() {
	width, height := a.innerWidth(), a.bodyHeight()
	a.board.SetWidth(width)
	a.boardVP.SetWidth(width)
	a.boardVP.SetHeight(max(height-a.progressBandLines(), 0))
	a.helpVP.SetWidth(width)
	a.helpVP.SetHeight(height)
	a.info.SetSize(width, height)
	a.syncBoard()
	if a.form != nil {
		a.form.SetSize(a.formSize())
	}
}

// noProjectReason explains an empty board: either nothing is selected, or the
// selection points at a project the config no longer describes.
func (a *App) noProjectReason() string {
	if a.cfg.ActiveProjectID == "" {
		return "No project selected. Press N to create one."
	}
	return fmt.Sprintf("Active project %s is not in the config file.", a.cfg.ActiveProjectID)
}

// infoView is the project page. An unconfigured project has no page to fetch,
// which the screen reports the same way the board does.
func (a *App) infoView() string {
	if a.info.Idle() {
		return a.styles.Faint.Render(a.noProjectReason())
	}
	return a.info.View(a.spinner.View())
}

// helpView is the key list, scrolled to the body band: it is longer than a
// short window, and the keys at the bottom are worth reaching.
func (a *App) helpView() string {
	if a.helpVP.Width() <= 0 || a.helpVP.Height() <= 0 {
		return a.helpVP.GetContent()
	}
	return a.helpVP.View()
}

// helpBody lists the global keys, then each screen's own. The board's reserved
// keys are listed too: they do nothing yet, but the help is where the plan for
// them is visible.
func (a *App) helpBody() string {
	lines := a.helpLines(a.keys.helpBindings())
	lines = append(lines, "", a.styles.Subtitle.Render("Board"), "")
	lines = append(lines, a.helpLines(a.board.helpBindings())...)
	lines = append(lines, "", a.styles.Subtitle.Render("Scrolling"), "")
	lines = append(lines, a.helpLines(infoKeys())...)
	return strings.Join(lines, "\n")
}

// helpLines renders one indented line per binding.
func (a *App) helpLines(bindings []key.Binding) []string {
	lines := make([]string, len(bindings))
	for i, b := range bindings {
		h := b.Help()
		lines[i] = "  " + a.styles.HelpKey.Render(h.Key) + "  " + a.styles.HelpDesc.Render(h.Desc)
	}
	return lines
}

// statusRegion is the status bar inside its border, docked to the window's
// bottom rows: the bar keeps its own fill, and the box's columns come out of
// the indent the bare bar would have spent anyway.
func (a *App) statusRegion() []string {
	box := a.styles.StatusBox.Width(a.width).Render(a.statusBarAt(a.width-2, 1))
	return strings.Split(box, "\n")
}

// statusBar is the bare bar at the window's full width: what a window too
// small for the box gets, and the unmeasured fallback.
func (a *App) statusBar() string {
	return a.statusBarAt(a.width, framePadX)
}

// statusBarAt is the bar as one line of the given total width, its content
// held in from the left edge by indent, and the one band with a fill of its
// own: the mode chip and whatever the app has to say in a left segment, the
// key hints right-aligned in a right segment, and the bar's background between
// them. It is one line however narrow the window gets — a bar that wrapped
// would take a line the bands above it have already spent.
func (a *App) statusBarAt(total, indent int) string {
	room := a.innerWidth()
	left, right := a.statusLeft(room), ""
	if gap := room - lipgloss.Width(left) - statusSegmentGap; total <= 0 || gap > 0 {
		right = a.statusRight(max(gap, 0))
	}
	if total <= 0 {
		// No window to spread across, so the two segments simply sit together.
		return a.styles.StatusBar.Render(left + strings.Repeat(" ", statusSegmentGap) + right)
	}
	// The indents are the first thing a window too narrow for the bar loses, so
	// the line is cut to the window rather than to the room between them.
	pad := max(room-lipgloss.Width(left)-lipgloss.Width(right), 0)
	line := strings.Repeat(" ", indent) + left + strings.Repeat(" ", pad) + right
	return a.styles.StatusBar.Width(total).Render(fit(line, total))
}

// statusSegmentGap is the least space the bar keeps between its two segments,
// so a full bar does not read as one run of text.
const statusSegmentGap = 2

// statusLeft is the bar's left segment: the mode chip, and beside it the error
// waiting to be dismissed, a transient note, or an open form's prompt. The
// message never takes more than half the bar, so a long note cannot push the
// key hints out altogether.
func (a *App) statusLeft(width int) string {
	chip := a.styles.ModeChip.Render(a.chipText())
	room := 0
	if width > 0 {
		// A window with no room beside the chip gets the chip alone, cut to fit.
		if room = min(width-lipgloss.Width(chip)-1, width/2); room <= 0 {
			return fit(chip, width)
		}
	}
	message := a.statusMessage(room)
	if message == "" {
		return chip
	}
	return chip + " " + message
}

// statusRight is the bar's right segment: the key hints, or nothing at all once
// the left segment has taken the bar.
func (a *App) statusRight(width int) string {
	// An open form owns every key the hints name, so naming them would be a lie.
	// Its own prompt, on the left, is the whole story.
	if a.form != nil {
		return ""
	}
	if len(a.joined) > 0 {
		return a.paneHintLine(width)
	}
	return a.hintLine(width)
}

// chipText is what the status bar's chip says: the screen's name, or on the
// board the project's, cut to a third of the bar so the message beside the
// chip keeps most of the room.
func (a *App) chipText() string {
	text := appName
	switch {
	case a.screen == screenHelp:
		text = "help"
	case a.screen == screenInfo:
		text = "info"
	case a.screen == screenForm:
		text = "edit"
	case a.project != nil:
		text = a.project.Name
	}
	if w := a.innerWidth(); w > 0 {
		text = fit(text, w/3)
	}
	return text
}

// statusMessage is what the app has to say beside the chip, or nothing when it
// has nothing: the chip is then alone on the left and the hints have the bar.
func (a *App) statusMessage(width int) string {
	if a.err != nil {
		// The error style pads, so the text gets what the padding leaves. The
		// leading text is what names the call that failed, so the tail goes first.
		text := fit(oneLine(fmt.Sprintf("%v — esc to dismiss", a.err)),
			width-a.styles.Error.GetHorizontalFrameSize())
		return a.styles.Error.Render(text)
	}
	if a.note != "" {
		return a.styles.StatusNote.Render(fit(oneLine(a.note), width))
	}
	// An open form has the keys the hints name, so all that is left to say is
	// the one key it does not handle itself.
	if a.form != nil {
		return fit(a.styles.StatusKey.Render("esc")+" "+a.styles.StatusDesc.Render("cancel"), width)
	}
	if a.toast != "" {
		return a.styles.toastStyle(a.toastSev).Render(fit(oneLine(a.toast), width))
	}
	return ""
}

// hintLine is the ordinary hint line: the app's global keys.
func (a *App) hintLine(width int) string {
	return a.fitHints(a.keys.statusHints(), width)
}

// paneHintLine is the status bar while an agent's pane is joined beside the
// board: how the split is handled, in place of hints for keys the user already
// knows. The key that returns the pane matters more than the zoom, so the zoom
// goes first when the bar runs out of room. When the joined pane is the
// planning agent's, the guidance names its own key rather than t's — pressing
// t there would find no slice to act on.
func (a *App) paneHintLine(width int) string {
	returnKey := a.board.keys.Attach.Help().Key
	if len(a.joined) == 1 && a.joined[agent.PlanSentinel] {
		returnKey = a.board.keys.Plan.Help().Key
	}
	return a.fitHints([]hint{
		{key.NewBinding(key.WithHelp(returnKey, "return the agent")), 2},
		{key.NewBinding(key.WithHelp("prefix+z", "zoom the split")), 1},
	}, width)
}

// fitHints renders hints on one line, dropping them by rank until they fit.
// Only if there is nothing left to drop is what remains truncated.
func (a *App) fitHints(hints []hint, width int) string {
	line := a.renderHints(hints)
	for rank := 1; width > 0 && len(hints) > 0 && lipgloss.Width(line) > width; rank++ {
		hints = slices.DeleteFunc(hints, func(h hint) bool { return h.rank == rank })
		line = a.renderHints(hints)
	}
	return fit(line, width)
}

// renderHints draws one hint per binding, separated by a dot, in the status
// bar's own colours.
func (a *App) renderHints(hints []hint) string {
	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		help := h.binding.Help()
		parts = append(parts, a.styles.StatusKey.Render(help.Key)+" "+a.styles.StatusDesc.Render(help.Desc))
	}
	return strings.Join(parts, a.styles.StatusSep.Render(" · "))
}

// innerWidth is the columns a band has between its indents, or 0 before the
// first resize — which every caller reads as "unmeasured, draw it whole". A
// window too narrow to hold the indents comes out as 0 as well: there is no
// width to fit to, and they have overflowed it either way.
func (a *App) innerWidth() int {
	if a.width <= 0 {
		return 0
	}
	return max(0, a.width-a.styles.Frame.GetHorizontalFrameSize())
}

// fit truncates s to width columns, keeping the leading text. A width of zero
// or less is an unmeasured window, which is left alone.
func fit(s string, width int) string {
	if width <= 0 {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

// oneLine is s up to its first line break: an error or a note carrying one
// would otherwise take a line the layout has not left room for.
func oneLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
