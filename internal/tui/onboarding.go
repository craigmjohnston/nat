package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/craigmjohnston/notion-agent-tracker/internal/config"
	"github.com/craigmjohnston/notion-agent-tracker/internal/notion"
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
	Search(ctx context.Context, query, filterType string) ([]notion.SearchResult, error)
	CreateProjectsDatabase(ctx context.Context, parentPageID, title string) (*notion.Database, error)
	QueryDataSource(ctx context.Context, id string, filter map[string]any, sorts []notion.Sort) ([]notion.Page, error)
	GetBlockChildren(ctx context.Context, id string) ([]notion.Block, error)
	CreatePage(ctx context.Context, parent notion.Parent, properties map[string]notion.PropertyValue, children []map[string]any) (*notion.Page, error)
	UpdatePageProperties(ctx context.Context, pageID string, properties map[string]notion.PropertyValue) (*notion.Page, error)
	AppendBlockChildren(ctx context.Context, id string, children []map[string]any) ([]notion.Block, error)
	DeleteBlock(ctx context.Context, id string) error
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

// Messages carrying the result of each Notion call the wizard makes between
// steps. Every one of them may carry an error instead.
type (
	projectDBsMsg struct {
		results []notion.SearchResult
		err     error
	}
	parentPagesMsg struct {
		results []notion.SearchResult
		err     error
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
	stepLoadingProjectDBs onboardingStep = iota
	stepPickProjectDB
	stepLoadingParentPages
	stepNewProjectDB
	stepCreatingProjectDB
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

	step   onboardingStep
	form   *huh.Form
	status string
	err    error

	// Values bound to form fields.
	dbChoice     string
	dbName       string
	parentPageID string
	assigneeID   string

	projectDBs []notion.SearchResult
	users      []notion.User
}

// NewOnboarding returns the wizard, starting from whatever config already
// exists so that a partially written file survives a second run.
func NewOnboarding(cfg config.Config, client NotionAPI, save func(config.Config) error) *Onboarding {
	return &Onboarding{
		save:   save,
		cfg:    cfg,
		client: client,
		step:   stepLoadingProjectDBs,
		status: "Looking for databases in your workspace…",
	}
}

// Init starts the first Notion call: there is no form to fill in until we know
// what databases the workspace holds.
func (m *Onboarding) Init() tea.Cmd { return m.loadProjectDBs }

// Update advances the wizard. Our own messages drive the steps between forms;
// everything else is handed to the current form, and a completed form moves on
// to the next step.
func (m *Onboarding) Update(msg tea.Msg) (*Onboarding, tea.Cmd) {
	switch msg := msg.(type) {
	case projectDBsMsg:
		return m.projectDBsLoaded(msg)
	case parentPagesMsg:
		return m.parentPagesLoaded(msg)
	case projectDBCreatedMsg:
		return m.projectDBCreated(msg)
	case usersMsg:
		return m.usersLoaded(msg)
	case projectsCheckedMsg:
		return m.projectsChecked(msg)
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
	if m.form != nil {
		return m.form.View()
	}
	return m.status + "\n"
}

// formSubmitted handles the completed form for the current step and kicks off
// the Notion call that follows it.
func (m *Onboarding) formSubmitted() (*Onboarding, tea.Cmd) {
	m.form = nil
	switch m.step {
	case stepPickProjectDB:
		if m.dbChoice == createNewChoice {
			return m.await(stepLoadingParentPages, "Looking for pages to create the database under…", m.loadParentPages)
		}
		m.setProjectDB(m.dbChoice)
		return m.await(stepLoadingUsers, "Loading workspace users…", m.loadUsers)
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
	m.step, m.status, m.form = step, status, nil
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
	m.err, m.form, m.status = err, nil, ""
	return m, nil
}

func (m *Onboarding) projectDBsLoaded(msg projectDBsMsg) (*Onboarding, tea.Cmd) {
	if msg.err != nil {
		return m.fail(fmt.Errorf("search for databases: %w", msg.err))
	}
	m.projectDBs = msg.results
	return m.show(stepPickProjectDB, m.projectDBForm())
}

func (m *Onboarding) parentPagesLoaded(msg parentPagesMsg) (*Onboarding, tea.Cmd) {
	if msg.err != nil {
		return m.fail(fmt.Errorf("search for pages: %w", msg.err))
	}
	if len(msg.results) == 0 {
		return m.fail(errNoPages)
	}
	return m.show(stepNewProjectDB, m.newProjectDBForm(msg.results))
}

func (m *Onboarding) projectDBCreated(msg projectDBCreatedMsg) (*Onboarding, tea.Cmd) {
	if msg.err != nil {
		return m.fail(fmt.Errorf("create project database: %w", msg.err))
	}
	dsID, ok := msg.db.DataSourceID()
	if !ok {
		return m.fail(errNoDataSource)
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

// setProjectDB records the data source the user picked, along with the database
// it belongs to. A hit without a parent database still gives us the data source
// ID, which is what every query addresses.
func (m *Onboarding) setProjectDB(dsID string) {
	m.cfg.ProjectDBDataSourceID = dsID
	m.cfg.ProjectDBID = ""
	for _, r := range m.projectDBs {
		if r.ID == dsID {
			m.cfg.ProjectDBID = r.Parent.DatabaseID
			return
		}
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
	errNoDataSource = errors.New("create project database: no data source was returned")
)

// The Notion calls made between steps. Each returns a message carrying either
// the result or the error.

func (m *Onboarding) loadProjectDBs() tea.Msg {
	results, err := m.client.Search(context.Background(), "", notion.SearchDataSource)
	return projectDBsMsg{results: results, err: err}
}

func (m *Onboarding) loadParentPages() tea.Msg {
	results, err := m.client.Search(context.Background(), "", notion.SearchPage)
	return parentPagesMsg{results: results, err: err}
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

func (m *Onboarding) projectDBForm() *huh.Form {
	options := make([]huh.Option[string], 0, len(m.projectDBs)+1)
	for _, r := range m.projectDBs {
		options = append(options, huh.NewOption(resultLabel(r), r.ID))
	}
	options = append(options, huh.NewOption("Create a new database…", createNewChoice))
	m.dbChoice = options[0].Value
	return huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Which database holds your projects?").
			Description("Pick the existing project database, or create one.").
			Options(options...).
			Value(&m.dbChoice),
	))
}

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
