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
)

// NewProjectForm is the modal behind N: everything a tracked project needs that
// Notion cannot work out for itself. The working directory is asked for here
// and kept in local config only — it is this machine's answer, not the
// workspace's.
type NewProjectForm struct {
	form    *huh.Form
	heading string

	// The values bound to the form's fields.
	name    string
	info    string
	workdir string
}

// newNewProjectForm returns the empty form for a new project.
func newNewProjectForm(theme huh.Theme) *NewProjectForm {
	f := &NewProjectForm{heading: "New project"}
	f.form = huh.NewForm(huh.NewGroup(
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
	)).WithTheme(theme)
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
// is three Notion calls and a read-back, which is long enough to notice.
func (f *NewProjectForm) busyNote() string { return "Creating the project…" }

// save builds the project the completed form describes.
func (f *NewProjectForm) save(a *App) tea.Cmd {
	return createProject(a.client, a.cfg.ProjectDBDataSourceID,
		strings.TrimSpace(f.name), f.info, expandHome(strings.TrimSpace(f.workdir)))
}

// createProject creates the project page and its two databases, then writes the
// blurb onto the page. The schema check the client runs on the way out is
// reported rather than swallowed, but not before the structure it verified: a
// project that exists is worth recording however its schema read back.
func createProject(client NotionAPI, projectsDSID, name, info, workdir string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		s, err := client.CreateProject(ctx, projectsDSID, name)
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
// about, so one workspace can hold more than one plan.
type SwitchProjectForm struct {
	form    *huh.Form
	heading string

	// names is each project's name by ID, so the switch can say where it went.
	names map[string]string
	// chosen is the ID of the project picked, bound to the select.
	chosen string
}

// newSwitchProjectForm returns the picker over the configured projects, which
// must not be empty, starting on the one already active.
func newSwitchProjectForm(theme huh.Theme, cfg config.Config) *SwitchProjectForm {
	f := &SwitchProjectForm{
		heading: "Switch project",
		names:   make(map[string]string, len(cfg.Projects)),
		chosen:  cfg.ActiveProjectID,
	}
	options := make([]huh.Option[string], 0, len(cfg.Projects))
	for _, id := range sortedProjectIDs(cfg.Projects) {
		name := cfg.Projects[id].Name
		if name == "" {
			name = id
		}
		options = append(options, huh.NewOption(name, id))
		f.names[id] = name
	}
	if _, ok := cfg.Projects[f.chosen]; !ok {
		f.chosen = options[0].Value
	}
	f.form = huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Project").
			Description("Which plan the board shows; every project stays where it is.").
			Options(options...).
			Value(&f.chosen),
	)).WithTheme(theme)
	return f
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

// SetSize gives the form the room the window leaves it.
func (f *SwitchProjectForm) SetSize(width, height int) {
	f.form = f.form.WithWidth(width).WithHeight(height)
}

// busyNote is empty: switching is a config write and a reload, which the board
// announces for itself.
func (f *SwitchProjectForm) busyNote() string { return "" }

// save points the app at the project that was picked.
func (f *SwitchProjectForm) save(*App) tea.Cmd {
	id, name := f.chosen, f.names[f.chosen]
	return func() tea.Msg { return projectSwitchedMsg{id: id, name: name} }
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

// switchProjectFlow opens the project picker. With one project there is nothing
// to pick, so the key says so rather than showing a list of one.
func (a *App) switchProjectFlow() tea.Cmd {
	if a.busy {
		return nil
	}
	if len(a.cfg.Projects) < 2 {
		return a.showToast("There is no other project to switch to — press N to add one.", sevWarning)
	}
	return a.openForm(newSwitchProjectForm(a.styles.FormTheme, a.cfg))
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
		Name:           msg.name,
		MilestonesDSID: msg.structure.MilestonesDSID,
		SlicesDSID:     msg.structure.SlicesDSID,
		WorkingDir:     msg.workdir,
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
	a.cfg.ActiveProjectID = msg.id
	err := a.persist()
	cmd := a.showActiveProject()
	if a.err = err; err != nil {
		return a, cmd
	}
	return a, tea.Batch(cmd, a.showToast(fmt.Sprintf("Switched to %q.", msg.name), sevSuccess))
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
	a.board.SetProject(nil)
	a.info.Reset()
	return tea.Batch(a.startLoad(), a.refreshLive())
}
