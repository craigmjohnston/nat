package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// ProjectsDBTitle is the title given to the database created to hold one row
// per tracked project.
const ProjectsDBTitle = "Agent Projects"

// createNewChoice is the sentinel value of the "create a new database" option
// in the project database picker. It cannot collide with a Notion ID.
const createNewChoice = "<new>"

// NotionAPI is the part of *notion.Client the interface uses. It is an
// interface so the screens can be driven by a fake in tests.
type NotionAPI interface {
	ListUsers(ctx context.Context) ([]notion.User, error)
	SearchPaged(ctx context.Context, query, filterType, startCursor string) ([]notion.SearchResult, string, error)
	PageEntries(ctx context.Context, id string) ([]notion.PageEntry, error)
	GetDatabase(ctx context.Context, id string) (*notion.Database, error)
	CreateProjectsDatabase(ctx context.Context, parentPageID, title string) (*notion.Database, error)
	CreateProject(ctx context.Context, projectsDSID, name string) (*notion.ProjectStructure, error)
	QueryDataSource(ctx context.Context, id string, filter map[string]any, sorts []notion.Sort) ([]notion.Page, error)
	GetBlockChildren(ctx context.Context, id string) ([]notion.Block, error)
	CreatePage(ctx context.Context, parent notion.Parent, properties map[string]notion.PropertyValue, children []map[string]any) (*notion.Page, error)
	UpdatePageProperties(ctx context.Context, pageID string, properties map[string]notion.PropertyValue) (*notion.Page, error)
	AppendBlockChildren(ctx context.Context, id string, children []map[string]any) ([]notion.Block, error)
	DeleteBlock(ctx context.Context, id string) error
	TrashPage(ctx context.Context, pageID string) error
}

// NewClientFunc builds a NotionAPI from a source of bearer tokens.
type NewClientFunc func(token notion.TokenFunc) NotionAPI

// DefaultNewClient builds a real Notion client that re-reads the token for
// every request, so a credential rotated outside the process is picked up
// without a restart.
func DefaultNewClient(token notion.TokenFunc) NotionAPI { return notion.NewWithToken(token) }

// OnboardingDoneMsg reports that onboarding finished and Config has been
// written. NeedsProject is set when the project database holds no projects yet,
// so the caller should run the new-project flow before showing the board.
type OnboardingDoneMsg struct {
	Config       config.Config
	NeedsProject bool
}

// Messages carrying the result of each Notion call the wizard makes. Every one
// of them may carry an error instead.
type (
	// rootPagesMsg is one page of the workspace search: the picker streams
	// roots in as they arrive rather than waiting for the whole workspace.
	rootPagesMsg struct {
		results []notion.SearchResult
		cursor  string
		err     error
	}
	// pageEntriesMsg is the contents of one page, fetched when it was expanded.
	pageEntriesMsg struct {
		node    *treeNode
		entries []notion.PageEntry
		err     error
	}
	// projectDBResolvedMsg is the chosen database, fetched to learn its data
	// source ID — the one GetDatabase call the picker ever costs.
	projectDBResolvedMsg struct {
		db  *notion.Database
		err error
	}
	projectDBCreatedMsg struct {
		db  *notion.Database
		err error
	}
	usersMsg struct {
		users []notion.User
		err   error
	}
	projectsCheckedMsg struct {
		count int
		err   error
	}
)

// onboardingStep is where the wizard has got to. Steps alternate between forms
// the user fills in and Notion calls made in between.
type onboardingStep int

const (
	stepPickProjectDB onboardingStep = iota
	stepNewProjectDB
	stepCreatingProjectDB
	stepResolvingProjectDB
	stepLoadingUsers
	stepPickAssignee
	stepCheckingProjects
	stepDone
)

// Onboarding is the first-run wizard, shown when the config file is missing. It
// walks the user through choosing the project database and picking the
// assignee, then writes config.
//
// Authentication is not its concern: the caller resolves the Notion CLI's token
// and hands over a ready client, because a credential problem is fixed with
// `ntn login` rather than anywhere in this flow.
type Onboarding struct {
	save func(config.Config) error

	cfg    config.Config
	client NotionAPI
	styles Styles

	step   onboardingStep
	form   *huh.Form
	tree   *treePicker
	status string
	err    error

	// Values bound to form fields.
	dbName       string
	parentPageID string
	assigneeID   string

	// The root pages streamed in so far — the pages a new database can be
	// created under — and whether the search has run to its end.
	pages     []notion.SearchResult
	rootsDone bool

	chosenDBID string
	users      []notion.User
}

// NewOnboarding returns the wizard, starting from whatever config already
// exists so that a partially written file survives a second run. The picker is
// on show from the first frame; root pages stream into it as the workspace
// search returns them.
func NewOnboarding(cfg config.Config, client NotionAPI, save func(config.Config) error) *Onboarding {
	m := &Onboarding{
		save:   save,
		cfg:    cfg,
		client: client,
		styles: DefaultStyles(),
		step:   stepPickProjectDB,
	}
	m.tree = newTreePicker(m.styles,
		"Which database holds your projects?",
		"Browse to the existing project database, or create one.",
		[]*treeNode{{label: "Create a new database…", value: createNewChoice}})
	return m
}

// Init starts the workspace search streaming into the picker.
func (m *Onboarding) Init() tea.Cmd { return m.searchRoots("") }

// Update advances the wizard. Our own messages drive the steps between forms;
// everything else is handed to the current form, and a completed form moves on
// to the next step.
func (m *Onboarding) Update(msg tea.Msg) (*Onboarding, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if m.tree != nil {
			m.tree.SetSize(msg.Width, msg.Height)
		}
		// The form takes its size from this same message, below.
	case rootPagesMsg:
		return m.rootPagesLoaded(msg)
	case pageEntriesMsg:
		return m.pageEntriesLoaded(msg)
	case projectDBResolvedMsg:
		return m.projectDBResolved(msg)
	case projectDBCreatedMsg:
		return m.projectDBCreated(msg)
	case usersMsg:
		return m.usersLoaded(msg)
	case projectsCheckedMsg:
		return m.projectsChecked(msg)
	}

	if m.tree != nil {
		if press, ok := msg.(tea.KeyPressMsg); ok {
			return m.treeInput(press)
		}
		return m, nil
	}
	if m.form == nil {
		return m, nil
	}
	form, cmd := m.form.Update(msg)
	m.form = form.(*huh.Form)
	switch m.form.State {
	case huh.StateCompleted:
		return m.formSubmitted()
	case huh.StateAborted:
		return m, tea.Quit
	}
	return m, cmd
}

// View renders the current form, the error that stopped the wizard, or a note
// about the Notion call in flight.
func (m *Onboarding) View() string {
	if m.err != nil {
		return fmt.Sprintf("Onboarding failed: %v\n\nPress ctrl+c to quit.\n", m.err)
	}
	if m.tree != nil {
		return m.tree.View() + "\n"
	}
	if m.form != nil {
		return m.form.View()
	}
	return m.status + "\n"
}

// treeInput hands one key press to the database picker. A press can open a
// page — costing a fetch of its contents — or land on a database, or the
// escape hatch into creating one.
func (m *Onboarding) treeInput(msg tea.KeyPressMsg) (*Onboarding, tea.Cmd) {
	ev := m.tree.Handle(msg)
	switch {
	case ev.aborted:
		return m, tea.Quit
	case ev.load != nil:
		return m, m.loadPageEntries(ev.load)
	case !ev.chosen:
		return m, nil
	}
	if ev.choice == createNewChoice {
		if len(m.pages) == 0 {
			// The search may yet surface a page to create the database under,
			// so only a finished search is a dead end.
			if !m.rootsDone {
				return m, nil
			}
			return m.fail(errNoPages)
		}
		m.tree = nil
		return m.show(stepNewProjectDB, m.newProjectDBForm(m.pages))
	}
	m.tree = nil
	m.chosenDBID = ev.choice
	return m.await(stepResolvingProjectDB, "Loading the database…", m.resolveProjectDB)
}

// formSubmitted handles the completed form for the current step and kicks off
// the Notion call that follows it.
func (m *Onboarding) formSubmitted() (*Onboarding, tea.Cmd) {
	m.form = nil
	switch m.step {
	case stepNewProjectDB:
		return m.await(stepCreatingProjectDB, "Creating the project database…", m.createProjectDB)
	case stepPickAssignee:
		m.setAssignee(m.assigneeID)
		return m.await(stepCheckingProjects, "Checking for existing projects…", m.checkProjects)
	}
	return m, nil
}

// await moves to an asynchronous step: no form, a status line, and the call in
// flight.
func (m *Onboarding) await(step onboardingStep, status string, cmd tea.Cmd) (*Onboarding, tea.Cmd) {
	m.step, m.status, m.form, m.tree = step, status, nil, nil
	return m, cmd
}

// show moves to a form step.
func (m *Onboarding) show(step onboardingStep, form *huh.Form) (*Onboarding, tea.Cmd) {
	m.step, m.form, m.status = step, form, ""
	return m, form.Init()
}

// fail stops the wizard with an error the user can only quit out of. Nothing in
// the flow is recoverable in place: a credential problem is fixed with `ntn
// login` before the app starts, and the rest are workspace state.
func (m *Onboarding) fail(err error) (*Onboarding, tea.Cmd) {
	m.err, m.form, m.tree, m.status = err, nil, nil, ""
	return m, nil
}

// rootPagesLoaded feeds one page of search hits into the picker and asks for
// the next while there is one. Only pages parented by the workspace itself
// become roots — everything else, database rows included, is reached by
// expanding the page it lives in.
func (m *Onboarding) rootPagesLoaded(msg rootPagesMsg) (*Onboarding, tea.Cmd) {
	if msg.err != nil {
		return m.fail(msg.err)
	}
	if m.tree == nil {
		// The database is already picked; let the rest of the search lapse.
		return m, nil
	}
	var roots []*treeNode
	for _, p := range msg.results {
		if p.Parent.Type != "workspace" {
			continue
		}
		m.pages = append(m.pages, p)
		roots = append(roots, &treeNode{label: resultLabel(p), pageID: p.ID})
	}
	m.tree.AddRoots(roots...)
	if msg.cursor != "" {
		return m, m.searchRoots(msg.cursor)
	}
	m.rootsDone = true
	return m, nil
}

// pageEntriesLoaded hands an expanded page its contents: nested pages open
// further, databases can be chosen.
func (m *Onboarding) pageEntriesLoaded(msg pageEntriesMsg) (*Onboarding, tea.Cmd) {
	if msg.err != nil {
		return m.fail(fmt.Errorf("load the page's contents: %w", msg.err))
	}
	if m.tree == nil {
		return m, nil
	}
	children := make([]*treeNode, 0, len(msg.entries))
	for _, e := range msg.entries {
		n := &treeNode{label: entryLabel(e)}
		if e.Database {
			n.value = e.ID
		} else {
			n.pageID = e.ID
		}
		children = append(children, n)
	}
	m.tree.SetChildren(msg.node, children)
	return m, nil
}

// projectDBResolved records the chosen database once its data source ID has
// been fetched.
func (m *Onboarding) projectDBResolved(msg projectDBResolvedMsg) (*Onboarding, tea.Cmd) {
	if msg.err != nil {
		return m.fail(fmt.Errorf("load the chosen database: %w", msg.err))
	}
	dsID, ok := msg.db.DataSourceID()
	if !ok {
		return m.fail(fmt.Errorf("open the chosen database: %w", errNoDataSource))
	}
	m.cfg.ProjectDBID, m.cfg.ProjectDBDataSourceID = msg.db.ID, dsID
	return m.await(stepLoadingUsers, "Loading workspace users…", m.loadUsers)
}

func (m *Onboarding) projectDBCreated(msg projectDBCreatedMsg) (*Onboarding, tea.Cmd) {
	if msg.err != nil {
		return m.fail(fmt.Errorf("create project database: %w", msg.err))
	}
	dsID, ok := msg.db.DataSourceID()
	if !ok {
		return m.fail(fmt.Errorf("create project database: %w", errNoDataSource))
	}
	m.cfg.ProjectDBID, m.cfg.ProjectDBDataSourceID = msg.db.ID, dsID
	return m.await(stepLoadingUsers, "Loading workspace users…", m.loadUsers)
}

func (m *Onboarding) usersLoaded(msg usersMsg) (*Onboarding, tea.Cmd) {
	if msg.err != nil {
		return m.fail(fmt.Errorf("list users: %w", msg.err))
	}
	m.users = notion.Persons(msg.users)
	if len(m.users) == 0 {
		return m.fail(errNoPeople)
	}
	return m.show(stepPickAssignee, m.assigneeForm())
}

func (m *Onboarding) projectsChecked(msg projectsCheckedMsg) (*Onboarding, tea.Cmd) {
	if msg.err != nil {
		return m.fail(fmt.Errorf("query projects: %w", msg.err))
	}
	if m.cfg.Projects == nil {
		m.cfg.Projects = map[string]config.ProjectConfig{}
	}
	if err := m.save(m.cfg); err != nil {
		return m.fail(fmt.Errorf("save config: %w", err))
	}
	cfg, needsProject := m.cfg, msg.count == 0
	m.step, m.form, m.status = stepDone, nil, "Done."
	return m, func() tea.Msg {
		return OnboardingDoneMsg{Config: cfg, NeedsProject: needsProject}
	}
}

// setAssignee records the user slices are claimed as.
func (m *Onboarding) setAssignee(userID string) {
	m.cfg.AssigneeUserID = userID
	m.cfg.AssigneeUserName = ""
	for _, u := range m.users {
		if u.ID == userID {
			m.cfg.AssigneeUserName = u.Name
			return
		}
	}
}

// The failures worth explaining rather than reporting verbatim.
var (
	errNoPages = errors.New(
		"no pages found in this workspace to create the database under: create one in Notion, then start again")
	errNoPeople = errors.New(
		"no people found in this workspace, so there is nobody to claim slices as")
	errNoDataSource = errors.New("no data source was returned")
)

// The Notion calls the wizard makes. Each returns a message carrying either
// the result or the error.

// searchRoots fetches one page of the workspace search. Its result message
// carries the cursor, so each arrival queues the next fetch until the search
// is exhausted.
func (m *Onboarding) searchRoots(cursor string) tea.Cmd {
	return func() tea.Msg {
		results, next, err := m.client.SearchPaged(context.Background(), "", notion.SearchPage, cursor)
		if err != nil {
			return rootPagesMsg{err: fmt.Errorf("search for pages: %w", err)}
		}
		return rootPagesMsg{results: results, cursor: next}
	}
}

// loadPageEntries fetches what lives in one page, for the picker to show under
// its node.
func (m *Onboarding) loadPageEntries(node *treeNode) tea.Cmd {
	return func() tea.Msg {
		entries, err := m.client.PageEntries(context.Background(), node.pageID)
		return pageEntriesMsg{node: node, entries: entries, err: err}
	}
}

// resolveProjectDB fetches the chosen database for its data source ID — the
// selection is the first and only time a database is fetched.
func (m *Onboarding) resolveProjectDB() tea.Msg {
	db, err := m.client.GetDatabase(context.Background(), m.chosenDBID)
	return projectDBResolvedMsg{db: db, err: err}
}

func (m *Onboarding) createProjectDB() tea.Msg {
	db, err := m.client.CreateProjectsDatabase(context.Background(), m.parentPageID, strings.TrimSpace(m.dbName))
	return projectDBCreatedMsg{db: db, err: err}
}

func (m *Onboarding) loadUsers() tea.Msg {
	users, err := m.client.ListUsers(context.Background())
	return usersMsg{users: users, err: err}
}

func (m *Onboarding) checkProjects() tea.Msg {
	pages, err := m.client.QueryDataSource(context.Background(), m.cfg.ProjectDBDataSourceID, nil, nil)
	return projectsCheckedMsg{count: len(pages), err: err}
}

// The forms. Each binds straight to a field of the model, so a completed form
// leaves its answer where formSubmitted can read it.

func (m *Onboarding) newProjectDBForm(pages []notion.SearchResult) *huh.Form {
	options := make([]huh.Option[string], len(pages))
	for i, p := range pages {
		options[i] = huh.NewOption(resultLabel(p), p.ID)
	}
	m.dbName, m.parentPageID = ProjectsDBTitle, options[0].Value
	return huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Name for the new project database").
			Value(&m.dbName).
			Validate(required("a name")),
		huh.NewSelect[string]().
			Title("Which page should it live under?").
			Options(options...).
			Value(&m.parentPageID),
	))
}

func (m *Onboarding) assigneeForm() *huh.Form {
	options := make([]huh.Option[string], len(m.users))
	for i, u := range m.users {
		options[i] = huh.NewOption(userLabel(u), u.ID)
	}
	m.assigneeID = options[0].Value
	return huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Who claims slices?").
			Description("Agents set this person as the Assignee when they claim work.").
			Options(options...).
			Value(&m.assigneeID),
	))
}

// required rejects a blank answer, naming what was expected.
func required(what string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("please enter %s", what)
		}
		return nil
	}
}

// resultLabel is how a search hit is listed, falling back to its ID when Notion
// reports no title.
func resultLabel(r notion.SearchResult) string {
	if t := r.TitleText(); t != "" {
		return t
	}
	return "(untitled) " + r.ID
}

// entryLabel is how a page's child page or database is listed.
func entryLabel(e notion.PageEntry) string {
	if e.Title != "" {
		return e.Title
	}
	return "(untitled) " + e.ID
}

// userLabel is how a person is listed, with their email when the integration
// can read it.
func userLabel(u notion.User) string {
	name := u.Name
	if name == "" {
		name = u.ID
	}
	if email := u.Email(); email != "" {
		return fmt.Sprintf("%s <%s>", name, email)
	}
	return name
}
