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
	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/notion-agent-tracker/internal/config"
	"github.com/craigmjohnston/notion-agent-tracker/internal/domain"
	"github.com/craigmjohnston/notion-agent-tracker/internal/notion"
)

// screen is one of the app's full-window views. The board is what the app is
// for; help and info are pushed over it and dismissed with esc.
type screen int

const (
	screenBoard screen = iota
	screenHelp
	screenInfo
)

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

	project *domain.Project
	loading bool
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
		board: NewBoard(s), info: NewInfo(s)}
}

// NewAppWithOnboarding returns the root model showing the first-run wizard,
// which hands over to the board when it finishes.
func NewAppWithOnboarding(cfg config.Config, client NotionAPI, o *Onboarding) *App {
	a := NewApp(cfg, client)
	a.onboarding = o
	return a
}

// Init starts the screen on show: the wizard's first call, or the first load of
// the active project's plan.
func (a *App) Init() tea.Cmd {
	if a.onboarding != nil {
		return a.onboarding.Init()
	}
	return a.startLoad()
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
		cmd := a.startLoad()
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
			return a, a.board.Update(msg)
		case screenInfo:
			return a, a.info.Update(msg)
		}
	}
	return a, nil
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
	a.note = "Setup complete."
	if msg.NeedsProject {
		a.note = "Setup complete. No projects yet — the new-project flow lands in a later milestone."
	}
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
	default:
		return a.boardView()
	}
}

// boardView is the main screen: the project's heading and tally, then the
// board itself. Loading and "there is nothing to show" are the root model's to
// report — the board only ever draws a plan.
func (a *App) boardView() string {
	header := a.styles.Title.Render("notion-agent-tracker")
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
		return "No project selected. The new-project flow lands in a later milestone."
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
	hints := make([]string, 0, len(a.keys.helpBindings()))
	for _, b := range a.keys.helpBindings() {
		h := b.Help()
		hints = append(hints, a.styles.HelpKey.Render(h.Key)+" "+a.styles.HelpDesc.Render(h.Desc))
	}
	return strings.Join(hints, a.styles.Faint.Render(" · "))
}
