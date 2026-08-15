// Package tui is the Bubble Tea v2 interface: a root model that routes between
// screens, and the screens themselves.
package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

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
	// projectLoadedMsg carries a freshly loaded plan, and what migrating the
	// project on the way to it changed — nothing at all, for every project
	// already in the one shape.
	projectLoadedMsg struct {
		project   domain.Project
		migration notion.Migration
	}
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
	// Workshop launches a planning agent on the project's wishlist. It is out
	// of the hints row on purpose: the wishlist indicator names it whenever
	// there is something to workshop, which is the only time the key does
	// anything, and a standing hint for it would take room from keys that
	// always work.
	Workshop key.Binding
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
		Workshop:  key.NewBinding(key.WithKeys("W"), key.WithHelp("W", "workshop")),
	}
}

// promptKeyMap is what an inline prompt anchored to a board row answers to:
// the choices side to side, the answer, and the way out. It is a map of its
// own rather than part of the board's, because while a prompt is up these are
// the only keys there are.
type promptKeyMap struct {
	Prev   key.Binding
	Next   key.Binding
	Pick   key.Binding
	Cancel key.Binding
}

// defaultPromptKeyMap returns the bindings a prompt runs with. The choices are
// stepped with the arrows rather than h/l, which on the board are a movement
// and the launch key: a prompt is a question about one row, and neither should
// half-work while it is up.
func defaultPromptKeyMap() promptKeyMap {
	return promptKeyMap{
		Prev:   key.NewBinding(key.WithKeys("left", "shift+tab"), key.WithHelp("←/→", "choose")),
		Next:   key.NewBinding(key.WithKeys("right", "tab")),
		Pick:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Cancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "dismiss")),
	}
}

// promptHints are the hints row's bindings while a prompt is up: how to answer
// it, in place of what the row's keys would otherwise do. The answer matters
// most, so the way out is the first to go as the row narrows.
func (k promptKeyMap) promptHints() []hint {
	return []hint{
		{k.Prev, 2},
		{k.Pick, 3},
		{k.Cancel, 1},
	}
}

// hint is one key binding of the hints row, with the order it goes in as the
// row runs out of room: rank 1 is dropped first.
type hint struct {
	binding key.Binding
	rank    int
}

// statusHints are the global bindings the hints row draws when nothing more
// specific is selected, in the order they read. They are dropped by rank
// rather than truncated, so a narrow row loses whole hints from the middle and
// the two that matter most — quit, and the refresh that is the only way to see
// what an agent has done — survive longest.
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
// The workshop key is among them although it is not among the hints: the help
// screen is where a key the hints row has no room for is still findable.
func (k keyMap) helpBindings() []key.Binding {
	hints := k.statusHints()
	bindings := make([]key.Binding, 0, len(hints)+1)
	for _, h := range hints {
		bindings = append(bindings, h.binding)
	}
	return append(bindings, k.Workshop)
}

// App is the root model. It owns the config, the Notion client, the loaded
// plan, and whichever screen is on show, routing messages to it.
//
// The first-run wizard is held separately rather than as a screen: it runs
// before there is a config to show a board for, and it hands over exactly once.
type App struct {
	cfg        config.Config
	client     NotionAPI
	styles     Styles
	keys       keyMap
	promptKeys promptKeyMap

	onboarding *Onboarding
	screen     screen
	board      Board
	info       Info
	form       modal
	// prompt is what answering the board's open row prompt does, held by the
	// flow that opened it; nil when no prompt is up.
	prompt func(choice int) tea.Cmd

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
	// While any is, the hints row swaps its keys for the pane guidance.
	joined map[string]bool

	project *domain.Project
	// wishlist is the pending items the project page's wishlist held when it
	// was last read: what the status line's indicator counts, and what the
	// workshop key launches a planning agent on. Empty is both an empty
	// wishlist and one that could not be read — neither is worth an indicator,
	// and neither is worth launching an agent on.
	wishlist []notion.WishlistItem
	loading  bool
	// syncedAt is when the plan on screen came back from Notion, which the
	// status line's freshness indicator counts from. Zero until the first load
	// lands, and left where it is by a load that fails: what is on the board is
	// still as old as it was.
	syncedAt time.Time
	// nudgeSeen is the nudge marker's mtime as last acted on — the file the
	// headless commands touch after writing to Notion. Zero until the first
	// reading, which is a baseline rather than a reason to reload.
	nudgeSeen time.Time
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
	a := &App{cfg: cfg, client: client, styles: s, keys: defaultKeyMap(),
		promptKeys: defaultPromptKeyMap(), spinner: sp,
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
// is already running on, the background poll that keeps the plan itself
// current, and the reconcile that re-homes the panes an earlier run left
// joined.
func (a *App) Init() tea.Cmd {
	// The background query goes out first: the styles start on the dark
	// palette, and the answer swaps in the light one when the terminal says so.
	if a.onboarding != nil {
		return tea.Batch(tea.RequestBackgroundColor, a.onboarding.Init())
	}
	return tea.Batch(tea.RequestBackgroundColor,
		a.startLoad(), a.refreshLive(), liveTick(), nudgeTick(),
		pollTick(a.cfg.PollInterval()), a.reclaimStrays())
}

// Update handles the global keys and the app's own messages, and routes
// everything else to the screen on show.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// The one message that falls out of the switch to the screen behind it also
	// has something to do on the way, so its command is carried down there.
	var bar tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Recorded, then handed on: every band of the layout is sized from it.
		a.width, a.height = msg.Width, msg.Height
		a.resize()
		// The panes under the tmux bar have changed width, and its sections are
		// drawn to the width they were.
		bar = a.refreshStatusBar()
	case tea.KeyPressMsg:
		return a.keyPressed(msg)
	case tea.BackgroundColorMsg:
		a.setStyles(NewStyles(msg.IsDark()))
		return a, nil
	case OnboardingDoneMsg:
		return a.onboardingDone(msg)
	case projectLoadedMsg:
		// A load that lands is what clears the failure before it: the board is
		// current again, so there is nothing left for the status line to warn
		// about.
		a.project, a.loading, a.err = &msg.project, false, nil
		a.syncedAt = timeNow()
		// A prompt is a question about a row of the plan that was on show, which
		// the reload may have moved or taken away entirely.
		a.closePrompt()
		a.board.SetProject(a.project)
		// The first plan brings the bar with it, which the board's viewport has
		// to give its lines up to; resize re-shares them and re-syncs the board.
		a.resize()
		// A project that had to be migrated to be shown says so: the plan on
		// screen is not quite the one Notion held a moment ago.
		if !msg.migration.Empty() {
			return a, a.showToast(msg.migration.Summary(), sevSuccess)
		}
		return a, nil
	case notionErrMsg:
		a.loading = false
		// A load that fails over a board already on screen is news, not a state:
		// the plan stays up — as old as it was, which the freshness indicator goes
		// on saying — and the failure passes as a toast rather than an error the
		// user has to dismiss before the app answers a key again. With no plan
		// loaded there is nothing to keep and nothing else to look at, so the
		// failure stands on the bar until it is dismissed.
		if a.project != nil {
			return a, a.showToast(fmt.Sprintf("Refresh failed: %v", msg.err), sevError)
		}
		a.err = msg.err
		return a, nil
	case wishlistLoadedMsg:
		a.wishlistLoaded(msg)
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
		return a.saved(msg)
	case sliceRefreshedMsg:
		return a.sliceRefreshed(msg)
	case slicesSyncedMsg:
		return a.slicesSynced(msg)
	case projectCreatedMsg:
		return a.projectCreated(msg)
	case projectSwitchedMsg:
		return a.projectSwitched(msg)
	case agentLaunchedMsg:
		return a.agentLaunched(msg)
	case agentAttachedMsg:
		a.paneMoved(msg)
		// The agent has had the pane to itself, so the slice it was working on
		// is refetched rather than trusted. The planning agent works the whole
		// plan, not a page of it, so its exit reloads the lot — and its report
		// is a toast, not a row confirm, since it is about no row.
		if msg.slice == agent.PlanSentinel {
			if msg.err == nil && msg.note != "" {
				a.busy = false
				return a, tea.Batch(a.startLoad(), a.refreshLive(), a.showToast(msg.note, sevSuccess))
			}
			model, cmd := a.saved(sliceSavedMsg{note: msg.note, err: msg.err})
			return model, tea.Batch(cmd, a.refreshLive())
		}
		model, cmd := a.saved(sliceSavedMsg{note: msg.note, err: msg.err, sliceID: msg.slice})
		return model, tea.Batch(cmd, a.refreshLive())
	case liveSessionsMsg:
		return a, a.liveLoaded(msg)
	case straysReclaimedMsg:
		return a, a.straysReclaimed(msg)
	case liveTickMsg:
		return a, tea.Batch(a.refreshLive(), liveTick())
	case nudgeTickMsg:
		return a, tea.Batch(checkNudge(), nudgeTick())
	case pollTickMsg:
		// The next tick is scheduled whatever this one does, so a poll passed over
		// while a form or a write is up resumes on the tick after.
		return a, tea.Batch(a.polled(), pollTick(a.cfg.PollInterval()))
	case nudgeMsg:
		return a, a.nudged(msg)
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
		return a, tea.Batch(bar, cmd)
	}
	if a.form != nil {
		return a, tea.Batch(bar, a.formUpdate(msg))
	}
	return a, bar
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
	// An open prompt owns every key too: it is a question about the row the
	// cursor is on, and the board should not move out from under it while it
	// goes unanswered. Keys it does not know are ignored rather than passed on.
	if a.board.Prompting() {
		return a, a.promptKey(msg)
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
	case key.Matches(msg, a.keys.Workshop):
		return a, a.workshopFlow()
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

// openPrompt anchors a question to the row the cursor is on and remembers what
// answering it does. It is the modal form's small sibling: one choice about one
// row, asked on the row itself, with the plan still on screen behind it.
func (a *App) openPrompt(options []string, answer func(choice int) tea.Cmd) tea.Cmd {
	a.prompt, a.note = answer, ""
	a.board.SetPrompt(options)
	a.syncBoard()
	return nil
}

// closePrompt takes the prompt down, answered or abandoned.
func (a *App) closePrompt() {
	a.prompt = nil
	a.board.ClearPrompt()
	a.syncBoard()
}

// promptKey answers the open prompt: the arrows step the choice, enter takes
// it, and esc leaves without one. Abandoning it says nothing — nothing was in
// flight to report on, and the row is as it was.
func (a *App) promptKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, a.promptKeys.Prev):
		a.board.MovePrompt(-1)
		a.syncBoard()
	case key.Matches(msg, a.promptKeys.Next):
		a.board.MovePrompt(1)
		a.syncBoard()
	case key.Matches(msg, a.promptKeys.Cancel):
		a.closePrompt()
	case key.Matches(msg, a.promptKeys.Pick):
		answer, choice := a.prompt, a.board.PromptChoice()
		a.closePrompt()
		return answer(choice)
	}
	return nil
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
// has been fetched to fill the brief in with. Slices in progress and Done ones
// are work in flight or finished, so they are refused rather than opened.
func (a *App) editSlice() tea.Cmd {
	if !a.canWrite() {
		return nil
	}
	s, ok := a.board.SelectedSlice()
	if !ok {
		return a.showConfirm("Move to a slice to edit it.", sevWarning)
	}
	if s.Status != domain.SliceTodo {
		return a.showConfirm(fmt.Sprintf("%q is %s — only Todo slices can be edited.", s.Name, statusWord(s)), sevWarning)
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
// the height it was given, and so the one line of the modal a form cannot
// be told about.
const formHintsHeight = 1

// A floating modal's measurements: the least columns and lines of board its
// frame is held away from the body's edges by, and the widest its interior may
// grow — a form run out across a wide window reads as a wall, not a dialog.
const (
	modalMarginX  = 3
	modalMarginY  = 1
	modalMaxWidth = 64
)

// formSize is the room an open form has: the interior of a modal floating over
// the board, held off the body's edges and capped rather than run out to a wide
// window. Before the first resize there is no window to measure, so the numbers
// come out non-positive — which is huh's own signal to size itself, and it does
// that from the resize that follows.
func (a *App) formSize() (width, height int) {
	if a.width <= 0 || a.height <= 0 {
		return 0, 0
	}
	width = min(a.innerWidth()-2*modalMarginX-a.styles.Modal.GetHorizontalFrameSize(), modalMaxWidth)
	height = a.bodyHeight() - 2*modalMarginY - a.styles.Modal.GetVerticalFrameSize() - formHintsHeight
	return width, height
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
// status line says what that is, or nothing at all when there is nothing worth
// announcing.
type busyNoter interface{ busyNote() string }

// busyNoteOf is what the status line shows while a modal's work is in flight.
func busyNoteOf(f modal) string {
	if n, ok := f.(busyNoter); ok {
		return n.busyNote()
	}
	return "Saving…"
}

// closeForm dismisses the form and goes back to the board.
func (a *App) closeForm() { a.form, a.screen = nil, screenBoard }

// saved reports a finished write and brings the board up to date with it: the
// one page the write touched is refetched and patched into the plan — a deleted
// one simply comes off it — rather than the whole plan reloaded. A write naming
// no page falls back to the full reload, as does one that lands before there is
// a plan to patch. The writes that come through here are all about the row the
// cursor is on, so the report is an inline confirmation anchored to it.
func (a *App) saved(msg sliceSavedMsg) (tea.Model, tea.Cmd) {
	a.busy = false
	if msg.err != nil {
		a.note, a.err = "", msg.err
		return a, nil
	}
	a.note = ""
	var cmds []tea.Cmd
	switch {
	case msg.deleted:
		a.removeSlice(msg.sliceID)
	case msg.sliceID != "" && a.project != nil:
		cmds = append(cmds, a.refreshSlice(msg.sliceID))
	default:
		cmds = append(cmds, a.startLoad())
	}
	cmds = append(cmds, a.refreshLive())
	if msg.note != "" {
		cmds = append(cmds, a.showConfirm(msg.note, sevSuccess))
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

// startLoad kicks off a load of the active project's plan and of the wishlist
// on its page, which the status line counts. It returns nil when there is
// nothing to load: an unconfigured or unknown active project is a state the
// board reports, not an error.
func (a *App) startLoad() tea.Cmd {
	project, ok := a.activeProject()
	if !ok || a.client == nil {
		return nil
	}
	// Whatever failed last time is left on the status line until this load says
	// otherwise: a refresh in flight is not yet news, and clearing the warning
	// on the way out would take it off a board still showing the stale plan.
	a.loading = true
	return tea.Batch(a.spinner.Tick, a.fetchProject(a.cfg.ActiveProjectID, project),
		a.fetchWishlist(a.cfg.ActiveProjectID))
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

// fetchProject loads a project's plan: its milestones, which are the options of
// its Slices data source's Milestone column and so come with the schema, and its
// slices, oldest first — which is the order agents pick them up in, until the
// board's own order re-sorts them. The domain groups them from there.
//
// A plan kept on one page has no Order to sort its slices by, and created time
// is no substitute — Notion records it to the minute, so a plan written in one
// go has no order at all. The order comes from where the slices sit in the
// project's own board instead, read from the view.
//
// A project still in the shape this app started with — milestones in a database
// of their own — is migrated on the way past, before its schema is read for the
// plan, so what comes back is a plan of the one shape however it was stored.
func (a *App) fetchProject(id string, cfg config.ProjectConfig) tea.Cmd {
	client := a.client
	return func() tea.Msg {
		ctx := context.Background()
		ds, migration, err := notion.MigrateProject(ctx, client, cfg.SlicesDSID)
		if err != nil {
			return notionErrMsg{err: err}
		}
		shape := notion.ShapeOf(ds)
		slices, err := client.QueryDataSource(ctx, cfg.SlicesDSID, nil,
			[]notion.Sort{{Timestamp: notion.TimestampCreated, Direction: notion.SortAscending}})
		if err != nil {
			return notionErrMsg{err: fmt.Errorf("load slices: %w", err)}
		}
		return projectLoadedMsg{
			project: domain.NewProject(
				id, cfg.Name,
				domain.MilestonesFromOptions(shape.MilestoneOptions, shape.MilestoneType),
				domain.InViewOrder(
					domain.SlicesFromPages(slices),
					notion.PlanOrder(ctx, client, cfg.SlicesDSID))),
			migration: migration,
		}
	}
}

// View renders the screen on show, full window, and sets what the terminal
// itself shows about the app: the window title, and the native progress bar
// terminals that speak OSC 9;4 draw in the tab or the dock.
func (a *App) View() tea.View {
	v := tea.NewView(a.content())
	v.AltScreen = true
	v.WindowTitle = a.windowTitle()
	v.ProgressBar = a.progressBar()
	return v
}

// windowTitle is what the app calls its terminal window: the line the in-TUI
// status bar used to draw — the mode chip, and beside it the error waiting to
// be dismissed, a transient note or an open form's prompt.
//
// The title is how those messages reach the screen now: inside tmux the title
// (OSC 2) is the pane's own, and the board's section of the status bar renders
// `#{pane_title}`, so tmux redraws the bar itself as the content changes and
// nat sends no tmux command per keystroke. The renderer writes the sequence
// only when the string changes, so an idle app writes nothing at all.
//
// The styling goes: a title is text, and tmux does not re-expand what it
// substitutes, so the chip's fill and the error's red are stripped here rather
// than reaching the bar as escape codes to be shown literally.
func (a *App) windowTitle() string {
	// The room is the whole window: the section drawing this is the pane's own
	// width, not the width inside the app's frame.
	return strings.TrimSpace(xansi.Strip(a.statusLeft(a.width)))
}

// progressBar is the terminal's own progress indicator, at the project's done
// fraction. It is cleared — nil, so nothing is emitted and any bar already up
// is reset — when there is no project or no slices to be done: a bar sitting at
// zero for want of a plan would read as work stalled.
func (a *App) progressBar() *tea.ProgressBar {
	if a.project == nil {
		return nil
	}
	p := a.project.Progress()
	if p.Empty() {
		return nil
	}
	return tea.NewProgressBar(tea.ProgressBarDefault, int(p.Fraction()*100))
}

// The layout's fixed measurements: the columns each band is held away from the
// window's edges by, the height of the heading bar and of the progress bar
// under it, the least lines the body's box is worth drawing in, and the least
// and most lines the key hints on the bottom row wrap onto.
const (
	framePadX = 2
	// headerHeight is the heading bar's own line, and headerBarHeight the
	// progress bar and its label beneath it inside the header's box.
	headerHeight    = 1
	headerBarHeight = 2
	headerBoxMin    = headerHeight + 2
	bodyBoxMin      = 3
	hintsHeight     = 1
	hintsMaxHeight  = 3
)

// content is the rendered screen, without the terminal-level settings: the
// heading and the progress bar boxed in a section of their own, the body of the
// screen on show boxed under it, and the key hints on the bottom row. The bands
// are cut and padded to fill the window exactly, so nothing a screen draws can
// push the hints off the bottom.
//
// There is no status bar of the app's own: what it carried goes out as the
// terminal title — see [App.windowTitle] — and is drawn by the tmux bar under
// the pane, so the screen ends at its own border.
func (a *App) content() string {
	if a.onboarding != nil {
		return a.onboarding.View()
	}
	if a.width <= 0 || a.height <= 0 {
		// Before the first resize there is no window to lay out to, so the bands
		// are simply drawn one after another at whatever size they come out.
		return a.headerView() + "\n" + a.body() + "\n" + a.hintsView()
	}
	if a.framed() {
		lines := a.headerRegion()
		lines = append(lines, a.bodyRegion()...)
		return strings.Join(append(lines, a.band(a.hintsView(), a.hintBandHeight())...), "\n")
	}
	var lines []string
	if a.headerBandHeight() > 0 {
		lines = append(lines, a.headerView())
	}
	lines = append(lines, a.band(a.body(), a.bodyHeight())...)
	return strings.Join(append(lines, a.band(a.hintsView(), a.hintBandHeight())...), "\n")
}

// framed reports whether the window is big enough for the bordered layout: a
// border costs two lines and two columns per region, and the layout boxes the
// header as well as the body, so there has to be room for the header's own box,
// a body box with a line in it, and the hints row under them. Below that the
// bands are drawn bare rather than boxed, so the content is never all frame.
func (a *App) framed() bool {
	return a.height >= headerBoxMin+bodyBoxMin+hintsHeight &&
		a.width >= 2*framePadX+1
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

// headerBandHeight and the body heights are how the window's lines are shared
// out. The header takes its own height first, because a window too short for
// everything is still worth telling the user where they are and what the keys
// do: its own box when the window is framed, one bare line when it is not. The
// box gives up the progress bar before the body gives up its rows, so a short
// window keeps the plan on show and loses the bar's label, then the bar, then
// nothing more — the heading itself always stays.
func (a *App) headerBandHeight() int {
	if !a.framed() {
		return min(headerHeight, max(a.height, 0))
	}
	spare := max(a.height-hintsHeight-bodyBoxMin, headerBoxMin)
	return min(a.headerContentHeight()+2, spare)
}

// headerContentHeight is the lines inside the header's box: the heading, and
// the progress bar and its label when there is a plan to sum and a width to
// draw it at.
func (a *App) headerContentHeight() int {
	if a.project == nil || a.innerWidth() <= 0 {
		return headerHeight
	}
	return headerHeight + headerBarHeight
}

// hintAllowance is the most lines the hints may wrap onto before they start
// dropping by rank: their own line whenever the window has one to give after
// the header has taken its — a short window loses the body before it loses the
// hints — and beyond that only lines the body can spare, never more than
// hintsMaxHeight. A tall window is not a reason to stack hints the width could
// have held on one line; the width decides that, and this only says how far the
// stack may grow.
func (a *App) hintAllowance() int {
	if a.height <= 0 {
		// Unmeasured: there are no lines to share out, and the bands are drawn one
		// after another at whatever size they come out.
		return hintsHeight
	}
	room := max(a.height-a.headerBandHeight(), 0)
	if room <= hintsHeight {
		return room
	}
	return min(hintsMaxHeight, max(hintsHeight, room-bodyBoxMin))
}

// hintBandHeight is the lines the hints actually take, which is what the body
// is left with the rest of.
func (a *App) hintBandHeight() int { return len(a.hintLines()) }

// bodyBoxHeight is the lines the body region occupies, border included;
// bodyHeight is the lines a screen can actually draw on inside it.
func (a *App) bodyBoxHeight() int {
	return max(a.height-a.headerBandHeight()-a.hintBandHeight(), 0)
}

func (a *App) bodyHeight() int {
	if a.framed() {
		return max(a.bodyBoxHeight()-2, 0)
	}
	return a.bodyBoxHeight()
}

// headerRegion is the header band inside its border: the heading bar and the
// progress bar under it, boxed in a section of their own above the body, and
// clipped to whatever lines the box was given.
func (a *App) headerRegion() []string {
	// framed has already made sure the box has its border lines and the heading.
	return a.boxRegion(a.headerContent(), a.headerBandHeight())
}

// headerContent is the header box's interior: the heading bar run out to the
// box's own width, and the project's segmented progress bar beneath it.
func (a *App) headerContent() string {
	width := a.innerWidth()
	heading := a.styles.Header.Width(width).Render(a.headerLeft(width))
	bar := a.progressBarView()
	if bar == "" {
		return heading
	}
	return heading + "\n" + bar
}

// headerLeft is what the heading says: the app's name as a segment of its own
// and the screen or project name beside it, cut to width.
func (a *App) headerLeft(width int) string {
	segment := a.styles.HeaderApp.Render(appName)
	name := a.headerName()
	if name == "" {
		return segment
	}
	if width <= 0 {
		// No window to spread across, so the segments simply sit together.
		return segment + " " + a.styles.HeaderTitle.Render(name)
	}
	return fit(segment+a.styles.HeaderTitle.Render(" "+name), width)
}

// headerView is the bare heading bar: what a window too small for the boxed
// header gets, and the unmeasured fallback. It is a full-width fill with the
// heading held in from the edge like every other bare band.
func (a *App) headerView() string {
	if a.width <= 0 {
		return a.headerLeft(0)
	}
	line := strings.Repeat(" ", framePadX) + a.headerLeft(a.innerWidth())
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
	return a.boxRegion(a.body(), a.bodyBoxHeight())
}

// boxRegion is content inside the layout's border, as exactly height lines of
// the window's width: clipped to the box's interior — content taller than the
// box would push the borders apart rather than scroll — and the box run out to
// the window's width.
func (a *App) boxRegion(content string, height int) []string {
	content = clipLines(fit(content, a.innerWidth()), max(height-2, 0))
	// Width and Height count the border, so the box is sized to the window.
	box := a.styles.Box.Width(a.width).Height(height).Render(content)
	// Cut to the band's lines however the box came out: a region that overran
	// would push the bands below it off the window.
	lines := strings.Split(box, "\n")
	return lines[:min(len(lines), height)]
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
		return a.modalView()
	default:
		return a.boardView()
	}
}

// modalView floats the open form over the board: the form in its own bordered
// box, centred on the body band, over the board faded behind it. Before the
// first resize there is no band to centre on, so the box is drawn alone.
func (a *App) modalView() string {
	box := a.styles.Modal.Render(a.form.View())
	width, height := a.innerWidth(), a.bodyHeight()
	if width <= 0 || height <= 0 {
		return box
	}
	// A Layer's position only counts through a Compositor — composing layers
	// straight onto a Canvas draws each at the origin.
	canvas := lipgloss.NewCanvas(width, height)
	canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(a.scrimView()),
		lipgloss.NewLayer(box).
			X(max((width-lipgloss.Width(box))/2, 0)).
			Y(max((height-lipgloss.Height(box))/2, 0)).
			Z(1),
	))
	return canvas.Render()
}

// scrimView is the board as a modal's backdrop: every colour it draws with
// stripped and the whole of it redrawn receded, so the form on top is the only
// thing at full strength.
func (a *App) scrimView() string {
	return a.styles.Scrim.Render(xansi.Strip(a.boardView()))
}

// boardView is the main screen: the plan, scrolled to the body band. Loading
// and "there is nothing to show" are the root model's to report — the board
// only ever draws a plan. The progress bar is the header's, not the board's.
//
// Only the first load takes the screen. A reload — the refresh key, a nudge
// from a headless write, the reload after an agent exits — leaves the plan it
// is replacing on show and swaps the new one in when it lands, so the board
// does not blink back to a spinner every time something changes behind it; the
// status line's freshness indicator says a fetch is in flight. A project switch
// clears the plan first, so that one does show the spinner: what is on screen
// is no longer what the app is loading.
func (a *App) boardView() string {
	switch {
	case a.loading && a.project == nil:
		return a.spinner.View() + " Loading the plan…"
	case a.project == nil:
		return a.styles.Faint.Render(a.noProjectReason())
	case a.boardVP.Width() <= 0 || a.boardVP.Height() <= 0:
		// No band to scroll in yet, so every row is drawn.
		return a.board.View()
	}
	return a.boardVP.View()
}

// progressBarView is the header's bar: the plan segmented by milestone, as
// wide as the header box's interior, label line included — or nothing when
// there is no plan to sum, or no width to size the bar to.
func (a *App) progressBarView() string {
	if a.project == nil {
		return ""
	}
	return RenderProgressBar(a.styles, a.innerWidth(), SegmentsOf(a.project.Groups()))
}

// syncBoard puts the board's rows into the body's viewport and scrolls it the
// least it can to bring the cursor back on screen. The board draws every row it
// has; holding a plan taller than the window to the window is the layout's job.
func (a *App) syncBoard() {
	// The hints band says what the row under the cursor can do, and a slice's
	// hints run to more lines than a milestone's — so the lines left for the
	// board change as the cursor moves, not only as the window resizes.
	a.boardVP.SetHeight(a.bodyHeight())
	a.boardVP.SetContent(a.board.View())
	h := a.boardVP.Height()
	if h <= 0 {
		return
	}
	// A row is not a line: a wrapped one spans several, and it is the whole of
	// it that has to come on screen. A row taller than the band cannot, so its
	// first line wins — that is the one carrying the cursor marker.
	cursor, rows := a.board.CursorSpan()
	switch top := a.boardVP.YOffset(); {
	case cursor < top:
		a.boardVP.SetYOffset(cursor)
	case cursor+rows > top+h:
		a.boardVP.SetYOffset(min(cursor+rows-h, cursor))
	}
}

// resize hands the window's new measurements to the bands that size themselves
// from them.
func (a *App) resize() {
	width, height := a.innerWidth(), a.bodyHeight()
	a.board.SetWidth(width)
	a.boardVP.SetWidth(width)
	a.boardVP.SetHeight(height)
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

// statusLeft is the status line's content: the mode chip, beside it the error
// waiting to be dismissed, a transient note, or an open form's prompt, and last
// the standing indicators — how fresh the board is, and the wishlist count when
// the project has items pending. It is drawn by the tmux bar under the pane
// rather than by the app — see [App.windowTitle] — and so is cut to one line at
// the width the pane has.
func (a *App) statusLeft(width int) string {
	chip := a.styles.ModeChip.Render(a.chipText())
	room := 0
	if width > 0 {
		// A window with no room beside the chip gets the chip alone, cut to fit.
		if room = width - lipgloss.Width(chip) - 1; room <= 0 {
			return fit(chip, width)
		}
	}
	// The indicators are standing readings rather than news, so they take what
	// the message leaves rather than the other way round. Freshness goes first
	// of the two: how current the board is says something on every board, where
	// the wishlist count only says something on some.
	content := chip
	if message := a.statusMessage(room); message != "" {
		content += " " + message
	}
	content, joined := a.withIndicator(content, a.freshnessIndicator(), width, false)
	content, _ = a.withIndicator(content, a.wishlistIndicator(), width, joined)
	return content
}

// withIndicator puts a standing indicator beside what the status line already
// says, within the room the two have between them, and reports whether it went
// on. An indicator goes entirely when there is no room for it: an error or a
// note is about what the user just did, and outranks a standing reading.
//
// joined says whether an indicator is already on the line, in which case the
// two are separated by the hints' dot rather than a space: side by side, two
// readings a space apart read as one sentence.
func (a *App) withIndicator(content, indicator string, width int, joined bool) (string, bool) {
	if indicator == "" {
		return content, joined
	}
	sep := " "
	if joined {
		sep = a.styles.HintSep.Render(" · ")
	}
	if width > 0 && lipgloss.Width(content)+lipgloss.Width(sep)+lipgloss.Width(indicator) > width {
		return content, joined
	}
	return content + sep + indicator, true
}

// chipText is what the status line's chip says: the screen's name, or the app's
// own on the board — the heading already names the project — cut to a third of
// the line so the message beside the chip keeps most of the room.
func (a *App) chipText() string {
	text := appName
	switch a.screen {
	case screenHelp:
		text = "help"
	case screenInfo:
		text = "info"
	case screenForm:
		text = "edit"
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

// hintLines are the contextual hints wrapped to the width the band has, and
// hintsView the same thing as the block the band draws.
func (a *App) hintLines() []string {
	return a.wrapHints(a.contextHints(), a.innerWidth(), a.hintAllowance())
}

func (a *App) hintsView() string { return strings.Join(a.hintLines(), "\n") }

// contextHints are the hints the window's bottom row draws: what acts on
// the selection — the slice's actions, the milestone's — and otherwise the
// global set. An open form owns every key, so naming any would be a lie, and
// an open prompt names what answers it instead; a
// joined agent pane swaps in the split guidance, since the pane has the keys
// until it is hidden again. Each contextual set carries the help key near the
// lowest rank, so the way to the full list is among the first hints a narrow
// row gives up — only the board-wide hide-done toggle goes before it.
func (a *App) contextHints() []hint {
	if a.form != nil {
		return nil
	}
	// A prompt has the keys until it is answered, so what answers it is all
	// there is to name.
	if a.board.Prompting() {
		return a.promptKeys.promptHints()
	}
	if len(a.joined) > 0 {
		return a.paneHints()
	}
	if a.screen == screenBoard {
		if _, ok := a.board.SelectedSlice(); ok {
			return append(a.board.sliceHints(), hint{a.keys.Help, 2})
		}
		if _, ok := a.board.SelectedMilestone(); ok {
			return append(a.board.milestoneHints(), hint{a.keys.Help, 2})
		}
	}
	return a.keys.statusHints()
}

// paneHints are the hints while an agent's pane is joined beside the board:
// how the split is handled, in place of hints for keys the user already knows.
// The key that hides the pane matters more than the zoom, so the zoom goes
// first when the row runs out of room. When the joined pane is the planning
// agent's, the guidance names its own key rather than t's — pressing t there
// would find no slice to act on.
func (a *App) paneHints() []hint {
	hideKey := a.board.keys.Attach.Help().Key
	if len(a.joined) == 1 && a.joined[agent.PlanSentinel] {
		hideKey = a.board.keys.Plan.Help().Key
	}
	return []hint{
		{key.NewBinding(key.WithHelp(hideKey, "hide agent pane")), 2},
		{key.NewBinding(key.WithHelp("prefix+z", "zoom the split")), 1},
	}
}

// wrapHints renders hints over as many lines as they need, up to maxLines: a
// window too narrow for them all on one line stacks them rather than losing
// them, and a hint is never split across two lines. Only once the stack would
// be taller than maxLines do hints start dropping by rank, and only if there is
// nothing left to drop is what remains truncated.
func (a *App) wrapHints(hints []hint, width, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	lines := a.packHints(hints, width)
	for rank := 1; len(hints) > 0 && len(lines) > maxLines; rank++ {
		hints = slices.DeleteFunc(hints, func(h hint) bool { return h.rank == rank })
		lines = a.packHints(hints, width)
	}
	for i, line := range lines {
		lines[i] = fit(line, width)
	}
	return lines[:min(len(lines), maxLines)]
}

// packHints lays the hints onto lines, each separated from the last by a dot
// and each line filled as far as it goes before the next is started. A hint
// wider than the whole width still takes a line of its own, where it is cut
// rather than wrapped: half a hint says less than a truncated one.
func (a *App) packHints(hints []hint, width int) []string {
	sep := a.styles.HintSep.Render(" · ")
	var lines []string
	var line string
	for i, h := range hints {
		part := a.renderHint(h)
		if i == 0 {
			line = part
			continue
		}
		if next := line + sep + part; width <= 0 || lipgloss.Width(next) <= width {
			line = next
			continue
		}
		lines, line = append(lines, line), part
	}
	// The last line is always emitted, so an empty set still comes out as the
	// one blank line the band draws.
	return append(lines, line)
}

// renderHint draws one hint — its key and what it does — in the hints row's own
// colours.
func (a *App) renderHint(h hint) string {
	help := h.binding.Help()
	return a.styles.HintKey.Render(help.Key) + " " + a.styles.HintDesc.Render(help.Desc)
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
