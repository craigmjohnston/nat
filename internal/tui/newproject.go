package tui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// saveConfig persists local config. It is held as a variable so tests can keep
// their hands off the user's real config file.
var saveConfig = config.Save

// The messages the project flows come back as.
type (
	// projectCreatedMsg reports a finished creation. A non-nil structure with a
	// non-nil error means the project exists but something after it went wrong —
	// a schema that did not verify, or a page body that did not land — so the
	// config still records what was made rather than orphaning it.
	projectCreatedMsg struct {
		structure *notion.ProjectStructure
		name      string
		workdir   string
		err       error
	}
	// projectSwitchedMsg reports the project the board should show instead.
	projectSwitchedMsg struct {
		id   string
		name string
	}
	// workspaceProjectsMsg is the projects database read back, so the picker can
	// offer the pages this machine has never opened alongside the ones it has.
	// It arrives while the picker is already up: switching between configured
	// projects waits on nothing.
	workspaceProjectsMsg struct {
		projects []workspaceProject
		err      error
	}
	// projectOpenedMsg reports a project page read as something openable — the
	// name and the plan behind it — or the reason it cannot be opened at all.
	projectOpenedMsg struct {
		id string
		// name is what the picker called the page, so a refusal can name it
		// without a read that failed being asked for a title.
		name    string
		project *notion.ResolvedProject
		err     error
	}
)

// workspaceProject is one row of the projects database as the picker lists it.
type workspaceProject struct {
	ID   string
	Name string
}

// NewProjectForm is the modal behind N: everything a tracked project needs that
// Notion cannot work out for itself. The working directory is asked for here
// and kept in local config only — it is this machine's answer, not the
// workspace's.
type NewProjectForm struct {
	form    *huh.Form
	heading string

	// The values bound to the form's fields.
	name     string
	info     string
	workdir  string
	assignee bool
}

// newNewProjectForm returns the empty form for a new project.
func newNewProjectForm(theme huh.Theme) *NewProjectForm {
	f := &NewProjectForm{heading: "New project"}
	f.form = newForm(theme, huh.NewGroup(
		huh.NewInput().
			Title("Name").
			Value(&f.name).
			Validate(required("a name")),
		huh.NewText().
			Title("Info").
			Description("Becomes the project page body — the conventions every agent reads.").
			Value(&f.info),
		huh.NewInput().
			Title("Working directory").
			Description("Where this project's agents start; kept in local config, not Notion.").
			Value(&f.workdir).
			Validate(existingDir),
		huh.NewConfirm().
			Title("Track an assignee?").
			Description("Adds an Assignee column to the Slices table. A single-player project needs none — status says whose turn it is.").
			Affirmative("Yes").
			Negative("No").
			Value(&f.assignee),
	))
	return f
}

// Init starts the form.
func (f *NewProjectForm) Init() tea.Cmd { return f.form.Init() }

// Update feeds a message to the form.
func (f *NewProjectForm) Update(msg tea.Msg) tea.Cmd {
	form, cmd := f.form.Update(msg)
	f.form = form.(*huh.Form)
	return cmd
}

// State is how far the form has got.
func (f *NewProjectForm) State() huh.FormState { return f.form.State }

// View renders the form.
func (f *NewProjectForm) View() string { return f.form.View() }

// Heading is the title drawn over the form.
func (f *NewProjectForm) Heading() string { return f.heading }

// SetSize gives the form the room the window leaves it.
func (f *NewProjectForm) SetSize(width, height int) {
	f.form = f.form.WithWidth(width).WithHeight(height)
}

// busyNote says what the status bar shows while the project is being built: it
// is a page, a database and a read-back, which is long enough to notice.
func (f *NewProjectForm) busyNote() string { return "Creating the project…" }

// save builds the project the completed form describes.
func (f *NewProjectForm) save(a *App) tea.Cmd {
	return createProject(a.client, a.cfg.ProjectDBDataSourceID,
		strings.TrimSpace(f.name), f.info, expandHome(strings.TrimSpace(f.workdir)), f.assignee)
}

// createProject creates the project page and its Slices database, then writes
// the blurb onto the page. The schema check the client runs on the way out is
// reported rather than swallowed, but not before the structure it verified: a
// project that exists is worth recording however its schema read back.
func createProject(client NotionAPI, projectsDSID, name, info, workdir string, assignee bool) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		s, err := client.CreateProject(ctx, projectsDSID, name, assignee)
		switch {
		case s != nil:
		case err != nil:
			return projectCreatedMsg{err: fmt.Errorf("create project: %w", err)}
		default:
			return projectCreatedMsg{err: errors.New("create project: no project was returned")}
		}
		if blocks := paragraphBlocks(info); len(blocks) > 0 {
			if _, aerr := client.AppendBlockChildren(ctx, s.PageID, blocks); aerr != nil {
				err = errors.Join(err, fmt.Errorf("write project page: %w", aerr))
			}
		}
		return projectCreatedMsg{structure: s, name: name, workdir: workdir, err: err}
	}
}

// SwitchProjectForm is the picker behind P: the projects local config knows
// about, and under them the ones the workspace's projects database holds that
// this machine has never opened — one picker for both, since which of the two
// a project is is an accident of where it was created rather than anything the
// user is thinking about when they press the key.
type SwitchProjectForm struct {
	form    *huh.Form
	heading string
	// theme, width and height are what the form is built from, kept because it
	// is built more than once: the workspace's own projects arrive after the
	// picker has opened, and huh measures a group's height when it is built —
	// options added to the field it already holds would be drawn cut off.
	theme  huh.Theme
	width  int
	height int
	// options are the ones offered so far, the configured projects first, so a
	// later offer appends to the list rather than replacing it.
	options []huh.Option[string]

	// names is each project's name by ID, so the switch can say where it went.
	names map[string]string
	// unopened is the IDs local config knows nothing about, which is what makes
	// picking one an open — resolved through Notion and recorded — rather than
	// the switch a configured project is.
	unopened map[string]bool
	// chosen is the ID of the project picked, bound to the select.
	chosen string
}

// openSuffix marks the projects the picker offers that this machine has not
// opened yet, so picking one says what it is about to do and two projects
// sharing a name are still told apart.
const openSuffix = " — open from Notion"

// newSwitchProjectForm returns the picker over the configured projects,
// starting on the one already active. That list may be empty: a machine that
// has opened nothing yet still gets a picker, because the workspace's own
// projects are on their way into it.
func newSwitchProjectForm(theme huh.Theme, cfg config.Config) *SwitchProjectForm {
	f := &SwitchProjectForm{
		heading:  "Switch project",
		theme:    theme,
		names:    make(map[string]string, len(cfg.Projects)),
		unopened: map[string]bool{},
		chosen:   cfg.ActiveProjectID,
		options:  make([]huh.Option[string], 0, len(cfg.Projects)),
	}
	for _, id := range sortedProjectIDs(cfg.Projects) {
		name := cfg.Projects[id].Name
		if name == "" {
			name = id
		}
		f.options = append(f.options, huh.NewOption(name, id))
		f.names[id] = name
	}
	if _, ok := cfg.Projects[f.chosen]; !ok {
		f.chosen = ""
		if len(f.options) > 0 {
			f.chosen = f.options[0].Value
		}
	}
	f.build()
	return f
}

// build makes the form over the options as they now stand, at the size the
// last one was given. huh measures a group's height as it is built, so a
// picker that has grown is built again rather than added to.
func (f *SwitchProjectForm) build() {
	f.form = newForm(f.theme, huh.NewGroup(huh.NewSelect[string]().
		Title("Project").
		Description("Which plan the board shows. One this machine has never opened is opened from Notion.").
		Options(f.options...).
		Value(&f.chosen)))
	f.SetSize(f.width, f.height)
}

// offer adds the workspace's own projects under the configured ones, skipping
// any the picker already lists: a project configured here is that same project,
// and offering it twice would only ask which of the two the user meant. The
// picker is on screen while this happens, which is the point of it — the
// configured projects are pickable from the moment the key is pressed.
func (f *SwitchProjectForm) offer(projects []workspaceProject) tea.Cmd {
	for _, p := range projects {
		if _, known := f.names[p.ID]; known {
			continue
		}
		name := p.Name
		if name == "" {
			name = p.ID
		}
		f.names[p.ID] = name
		f.unopened[p.ID] = true
		f.options = append(f.options, huh.NewOption(name+openSuffix, p.ID))
	}
	f.build()
	return f.Init()
}

// sortedProjectIDs orders the configured projects by name, falling back to the
// ID so that two projects sharing a name still list in a stable order.
func sortedProjectIDs(projects map[string]config.ProjectConfig) []string {
	ids := make([]string, 0, len(projects))
	for id := range projects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if a, b := projects[ids[i]].Name, projects[ids[j]].Name; a != b {
			return a < b
		}
		return ids[i] < ids[j]
	})
	return ids
}

// Init starts the form.
func (f *SwitchProjectForm) Init() tea.Cmd { return f.form.Init() }

// Update feeds a message to the form.
func (f *SwitchProjectForm) Update(msg tea.Msg) tea.Cmd {
	form, cmd := f.form.Update(msg)
	f.form = form.(*huh.Form)
	return cmd
}

// State is how far the form has got.
func (f *SwitchProjectForm) State() huh.FormState { return f.form.State }

// View renders the form.
func (f *SwitchProjectForm) View() string { return f.form.View() }

// Heading is the title drawn over the form.
func (f *SwitchProjectForm) Heading() string { return f.heading }

// SetSize gives the form the room the window leaves it, remembering it for the
// form built when the workspace's own projects land.
func (f *SwitchProjectForm) SetSize(width, height int) {
	f.width, f.height = width, height
	f.form = f.form.WithWidth(width).WithHeight(height)
}

// busyNote is empty for a switch: it is a config write and a reload, which the
// board announces for itself. Opening a project this machine has not seen is a
// read of Notion first, which is long enough to say so.
func (f *SwitchProjectForm) busyNote() string {
	if f.unopened[f.chosen] {
		return "Opening the project…"
	}
	return ""
}

// save points the app at the project that was picked: a configured one is a
// switch and nothing more, and one the picker offered from Notion is read
// first, since what its plan lives in is the very thing config has no record
// of.
func (f *SwitchProjectForm) save(a *App) tea.Cmd {
	id, name := f.chosen, f.names[f.chosen]
	if f.unopened[id] {
		return openProject(a.client, id, name)
	}
	return func() tea.Msg { return projectSwitchedMsg{id: id, name: name} }
}

// listWorkspaceProjects reads the projects database into the rows the picker
// offers, by name so the list reads the way the configured half above it does.
func listWorkspaceProjects(client NotionAPI, projectsDSID string) tea.Cmd {
	return func() tea.Msg {
		pages, err := client.QueryDataSource(context.Background(), projectsDSID, nil, nil)
		if err != nil {
			return workspaceProjectsMsg{err: fmt.Errorf("read the projects database: %w", err)}
		}
		projects := make([]workspaceProject, 0, len(pages))
		for _, p := range pages {
			projects = append(projects, workspaceProject{ID: p.ID, Name: p.TitleText()})
		}
		sort.Slice(projects, func(i, j int) bool {
			if a, b := projects[i].Name, projects[j].Name; a != b {
				return a < b
			}
			return projects[i].ID < projects[j].ID
		})
		return workspaceProjectsMsg{projects: projects}
	}
}

// openProject reads a project page into what opening it needs. A page that is
// not a tracked project at all, or whose plan is missing what the app reads,
// comes back as the refusal the resolver made rather than a half-open project.
func openProject(client NotionAPI, id, name string) tea.Cmd {
	return func() tea.Msg {
		p, err := client.ResolveProject(context.Background(), id)
		if err == nil && p == nil {
			err = errors.New("no project came back")
		}
		return projectOpenedMsg{id: id, name: name, project: p, err: err}
	}
}

// newProjectFlow opens the new-project form. It needs no active project — it is
// how the first one comes to exist — only the projects database onboarding
// picked, and a client to create under it.
func (a *App) newProjectFlow() tea.Cmd {
	if a.client == nil || a.busy {
		return nil
	}
	if a.cfg.ProjectDBDataSourceID == "" {
		return a.showToast("No projects database is configured, so there is nowhere to create a project.", sevWarning)
	}
	return a.openForm(newNewProjectForm(a.styles.FormTheme))
}

// switchProjectFlow opens the project picker over the configured projects and
// sends the workspace's own after it, so the list fills in with the projects
// this machine has never opened while the ones it has are already there to
// pick: a projects database that is slow, or that cannot be read at all, holds
// up no switch. The key refuses only when there is nowhere for either half to
// come from — one configured project and no projects database is a list of one
// that can never grow.
func (a *App) switchProjectFlow() tea.Cmd {
	if a.busy {
		return nil
	}
	listable := a.client != nil && a.cfg.ProjectDBDataSourceID != ""
	if len(a.cfg.Projects) < 2 && !listable {
		return a.showToast("There is no other project to switch to — press N to add one.", sevWarning)
	}
	cmd := a.openForm(newSwitchProjectForm(a.styles.FormTheme, a.cfg))
	if !listable {
		return cmd
	}
	return tea.Batch(cmd, listWorkspaceProjects(a.client, a.cfg.ProjectDBDataSourceID))
}

// workspaceProjectsListed hands the picker the workspace's own projects, if it
// is still up: an answer that arrives after the user has picked, or given up,
// has nothing left to fill in. A read that failed is logged and otherwise
// passed over — the picker goes on offering the configured projects, which is
// what the key was pressed for.
func (a *App) workspaceProjectsListed(msg workspaceProjectsMsg) tea.Cmd {
	f, ok := a.form.(*SwitchProjectForm)
	if !ok {
		return nil
	}
	if msg.err != nil {
		logging.Error("could not list the workspace's projects", "error", msg.err)
		return nil
	}
	return f.offer(msg.projects)
}

// projectCreated records a freshly created project, makes it the active one,
// and reloads the board onto it.
func (a *App) projectCreated(msg projectCreatedMsg) (tea.Model, tea.Cmd) {
	a.busy = false
	if msg.structure == nil {
		a.note, a.err = "", msg.err
		return a, nil
	}
	if a.cfg.Projects == nil {
		a.cfg.Projects = map[string]config.ProjectConfig{}
	}
	a.cfg.Projects[msg.structure.PageID] = config.ProjectConfig{
		Name:       msg.name,
		SlicesDSID: msg.structure.SlicesDSID,
		WorkingDir: msg.workdir,
	}
	a.cfg.ActiveProjectID = msg.structure.PageID
	err := errors.Join(msg.err, a.persist())
	// The reload is started first: it clears the error banner, and anything that
	// went wrong on the way here is worth more than an empty one.
	cmd := a.showActiveProject()
	if a.err = err; err != nil {
		return a, cmd
	}
	return a, tea.Batch(cmd, a.showToast(fmt.Sprintf("Created %q.", msg.name), sevSuccess))
}

// projectSwitched points the board at another configured project.
func (a *App) projectSwitched(msg projectSwitchedMsg) (tea.Model, tea.Cmd) {
	a.busy = false
	return a, a.activateProject(msg.id, fmt.Sprintf("Switched to %q.", msg.name))
}

// projectOpened records a project page read out of Notion and switches to it,
// which is all opening one is: the config entry the switch had nothing to read
// from, and then the switch itself. The working directory is left unset — where
// the code lives is this machine's answer and no part of what was read — so the
// toast says where to give one. A page that would not resolve is a toast naming
// why, with local config untouched: half a project recorded is worse than none.
func (a *App) projectOpened(msg projectOpenedMsg) (tea.Model, tea.Cmd) {
	a.busy, a.note = false, ""
	if msg.err != nil {
		return a, a.showToast(fmt.Sprintf("Could not open %q: %v", msg.name, msg.err), sevError)
	}
	if a.cfg.Projects == nil {
		a.cfg.Projects = map[string]config.ProjectConfig{}
	}
	a.cfg.Projects[msg.id] = config.ProjectConfig{
		Name:       msg.project.Name,
		SlicesDSID: msg.project.SlicesDSID,
	}
	return a, a.activateProject(msg.id,
		fmt.Sprintf("Opened %q — press S to give it a working directory.", msg.project.Name))
}

// activateProject makes one of the configured projects the active one, saves
// that, and reloads the board onto its plan. A config that would not save is
// reported and the switch kept for the session, the bargain [App.persist]
// describes; the toast is the one thing the two ways in differ by.
func (a *App) activateProject(id, toast string) tea.Cmd {
	a.cfg.ActiveProjectID = id
	err := a.persist()
	cmd := a.showActiveProject()
	if a.err = err; err != nil {
		return cmd
	}
	return tea.Batch(cmd, a.showToast(toast, sevSuccess))
}

// persist writes the config as it now stands, describing a failure to do so.
// The change is kept for this session either way: the app is already showing
// what was asked for, and losing it on the next start is the smaller surprise.
func (a *App) persist() error {
	if err := saveConfig(a.cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// showActiveProject drops whatever the last project left on screen and loads
// the active one in its place.
func (a *App) showActiveProject() tea.Cmd {
	a.project = nil
	a.wishlist = nil
	a.board.SetProject(nil)
	a.info.Reset()
	// The diff on the review screen is of a branch in the project being left,
	// and so is what the refresh key would otherwise read again.
	a.diff.Reset()
	// And the pull request on the screen beside it is one of the project being
	// left, for the same reason.
	a.prview.Reset()
	return tea.Batch(a.startLoad(), a.refreshLive())
}
