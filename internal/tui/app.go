// Package tui is the Bubble Tea v2 interface: a root model that routes between
// screens, and the screens themselves.
package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

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

// helpBindings are the bindings listed to the user, in the order they read.
func (k keyMap) helpBindings() []key.Binding {
	return []key.Binding{k.Refresh, k.Info, k.Help, k.Back, k.Quit}
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

	// launcher starts and attaches to the agents' tmux sessions, and live maps
	// each slice it last reported an agent running for to that agent's session.
	launcher AgentLauncher
	live     map[string]string

	project *domain.Project
	loading bool
	// busy is a write, or the read that opens a form, in flight. Only one runs
	// at a time: a second would race the first over the same page.
	busy    bool
	spinner spinner.Model

	err  error
	note string

	width, height int
}

var _ tea.Model = (*App)(nil)

// NewApp returns the root model showing the board, for a workspace that is
// already set up.
func NewApp(cfg config.Config, client NotionAPI) *App {
	s := DefaultStyles()
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(s.Spinner))
	return &App{cfg: cfg, client: client, styles: s, keys: defaultKeyMap(), spinner: sp,
		board: NewBoard(s), info: NewInfo(s), launcher: newLauncher()}
}

// NewAppWithOnboarding returns the root model showing the first-run wizard,
// which hands over to the board when it finishes.
func NewAppWithOnboarding(cfg config.Config, client NotionAPI, o *Onboarding) *App {
	a := NewApp(cfg, client)
	a.onboarding = o
	return a
}

// Init starts the screen on show: the wizard's first call, or the first load of
// the active project's plan, alongside the poll that marks the slices an agent
// is already running on and the reconcile that re-homes the panes an earlier
// run left joined.
func (a *App) Init() tea.Cmd {
	if a.onboarding != nil {
		return a.onboarding.Init()
	}
	return tea.Batch(a.startLoad(), a.refreshLive(), liveTick(), a.reclaimStrays())
}

// Update handles the global keys and the app's own messages, and routes
// everything else to the screen on show.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Recorded, then passed on: the screens size themselves too.
		a.width, a.height = msg.Width, msg.Height
		a.board.SetWidth(msg.Width - a.styles.App.GetHorizontalFrameSize())
		a.info.SetSize(msg.Width-a.styles.App.GetHorizontalFrameSize(),
			msg.Height-a.styles.App.GetVerticalFrameSize()-infoChromeHeight)
		if a.form != nil {
			a.form.SetSize(a.formSize())
		}
	case tea.KeyPressMsg:
		return a.keyPressed(msg)
	case OnboardingDoneMsg:
		return a.onboardingDone(msg)
	case projectLoadedMsg:
		a.project, a.loading, a.err = &msg.project, false, nil
		a.board.SetProject(a.project)
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
		// The agent has had the terminal to itself, so the plan it was working on
		// is reloaded rather than trusted.
		model, cmd := a.saved(msg.note, msg.err)
		return model, tea.Batch(cmd, a.refreshLive())
	case liveSessionsMsg:
		a.liveLoaded(msg)
		return a, nil
	case straysReclaimedMsg:
		a.straysReclaimed(msg)
		return a, nil
	case liveTickMsg:
		return a, tea.Batch(a.refreshLive(), liveTick())
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
			a.note = "Cancelled."
			// Coming back to the board is a chance to notice a session that
			// started, or ended, while the form was up.
			return a, a.refreshLive()
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
		a.note = ""
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
			return a, a.board.Update(msg)
		case screenInfo:
			return a, a.info.Update(msg)
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
		a.note = "Move to a milestone to add a slice under it."
		return nil
	}
	return a.openForm(newAddSliceForm(m))
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
		a.note = "Move to a slice to edit it."
		return nil
	}
	if s.Status != domain.SliceTodo {
		a.note = fmt.Sprintf("%q is %s — only Todo slices can be edited.", s.Name, s.Status)
		return nil
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
	return a, a.openForm(newEditSliceForm(msg.slice, msg.markdown))
}

// openForm shows a form over the board, at the size the window it is opening
// into leaves for it.
func (a *App) openForm(f modal) tea.Cmd {
	a.form, a.screen, a.note = f, screenForm, ""
	f.SetSize(a.formSize())
	return f.Init()
}

// formChromeHeight is how many lines an open form does not have to itself: the
// heading and the blank line under it, then the blank line and status bar
// below — and one more for the blank line huh draws above its own key hints,
// which it leaves out of the height it was given.
const formChromeHeight = 5

// formSize is the room an open form has. Before the first resize there is no
// window to measure, so the numbers come out non-positive — which is huh's own
// signal to size itself, and it does that from the resize that follows.
func (a *App) formSize() (width, height int) {
	return a.width - a.styles.App.GetHorizontalFrameSize(),
		a.height - a.styles.App.GetVerticalFrameSize() - formChromeHeight
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
		a.note = "Cancelled."
		return nil
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
// was just written rather than what was there before.
func (a *App) saved(note string, err error) (tea.Model, tea.Cmd) {
	a.busy = false
	if err != nil {
		a.note, a.err = "", err
		return a, nil
	}
	a.note = note
	return a, tea.Batch(a.startLoad(), a.refreshLive())
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
		a.note = "Setup complete. No projects yet — let's make one."
		return a, cmd
	}
	a.note = "Setup complete."
	return a, a.startLoad()
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

// content is the rendered screen, without the terminal-level settings. The
// status bar is pushed to the bottom of the window once the terminal size is
// known, so it does not float under short screens.
func (a *App) content() string {
	if a.onboarding != nil {
		return a.onboarding.View()
	}
	frame := a.styles.App
	if a.width > 0 {
		frame = frame.Width(a.width)
	}
	body, status := a.body(), a.statusBar()
	gap := "\n\n"
	if inner := a.height - frame.GetVerticalFrameSize(); inner > 0 {
		// One newline butts the two together, so filling a window of `inner`
		// lines takes one more than the shortfall between them.
		if pad := inner - lipgloss.Height(body) - lipgloss.Height(status) + 1; pad > 2 {
			gap = strings.Repeat("\n", pad)
		}
	}
	return frame.Render(body + gap + status)
}

// body renders the current screen.
func (a *App) body() string {
	switch a.screen {
	case screenHelp:
		return a.helpView()
	case screenInfo:
		return a.infoView()
	case screenForm:
		return a.formView()
	default:
		return a.boardView()
	}
}

// formView is the open modal: its heading, then the form, which draws its own
// key hints.
func (a *App) formView() string {
	return a.styles.Title.Render(a.form.Heading()) + "\n\n" + a.form.View()
}

// boardView is the main screen: the project's heading and tally, then the
// board itself. Loading and "there is nothing to show" are the root model's to
// report — the board only ever draws a plan.
func (a *App) boardView() string {
	header := a.styles.Title.Render("nat")
	switch {
	case a.loading:
		return header + "\n\n" + a.spinner.View() + " Loading the plan…"
	case a.project == nil:
		return header + "\n\n" + a.styles.Faint.Render(a.noProjectReason())
	}

	p := a.project.Progress()
	return strings.Join([]string{
		header,
		a.styles.Subtitle.Render(a.project.Name),
		"",
		a.styles.Faint.Render(fmt.Sprintf("milestones: %d · slices done: %d/%d",
			len(a.project.Milestones), p.Done, p.Total)),
		"",
		a.board.View(),
	}, "\n")
}

// noProjectReason explains an empty board: either nothing is selected, or the
// selection points at a project the config no longer describes.
func (a *App) noProjectReason() string {
	if a.cfg.ActiveProjectID == "" {
		return "No project selected. Press N to create one."
	}
	return fmt.Sprintf("Active project %s is not in the config file.", a.cfg.ActiveProjectID)
}

// infoChromeHeight is how many lines the app draws around the info screen's
// viewport, inside the frame: the screen's heading and the blank line under it,
// then the blank line and status bar below. The viewport gets the rest.
const infoChromeHeight = 4

// infoView is the project page. An unconfigured project has no page to fetch,
// which the screen reports the same way the board does.
func (a *App) infoView() string {
	if a.info.Idle() {
		return a.styles.Title.Render("Info") + "\n\n" +
			a.styles.Faint.Render(a.noProjectReason())
	}
	return a.info.View(a.spinner.View())
}

// helpView lists the global keys, then each screen's own. The board's reserved
// keys are listed too: they do nothing yet, but the help is where the plan for
// them is visible.
func (a *App) helpView() string {
	lines := []string{a.styles.Title.Render("Keys"), ""}
	lines = append(lines, a.helpLines(a.keys.helpBindings())...)
	lines = append(lines, "", a.styles.Subtitle.Render("Board"), "")
	lines = append(lines, a.helpLines(a.board.helpBindings())...)
	lines = append(lines, "", a.styles.Subtitle.Render("Info"), "")
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

// statusBar is the bottom line: the error that is waiting to be dismissed, a
// note, or the key hints.
func (a *App) statusBar() string {
	if a.err != nil {
		return a.styles.Error.Render(fmt.Sprintf("%v — esc to dismiss", a.err))
	}
	if a.note != "" {
		return a.styles.Note.Render(a.note)
	}
	// An open form has the keys the global hints name, so all that is left to
	// say is the one key it does not handle itself.
	if a.form != nil {
		return a.styles.HelpKey.Render("esc") + " " + a.styles.HelpDesc.Render("cancel")
	}
	hints := make([]string, 0, len(a.keys.helpBindings()))
	for _, b := range a.keys.helpBindings() {
		h := b.Help()
		hints = append(hints, a.styles.HelpKey.Render(h.Key)+" "+a.styles.HelpDesc.Render(h.Desc))
	}
	return strings.Join(hints, a.styles.Faint.Render(" · "))
}
