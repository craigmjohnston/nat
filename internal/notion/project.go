package notion

import (
	"context"
	"fmt"
	"strings"
)

// SlicesDBTitle is the title of the database a tracked project keeps its work
// in. A project's whole plan is that one database: its milestones are the
// options of the slices' own Milestone column.
const SlicesDBTitle = "Slices"

// Property names shared by the project, milestone and slice schemas. Every
// data source names its title property "Name".
const (
	PropName      = "Name"
	PropStatus    = "Status"
	PropOrder     = "Order"
	PropMilestone = "Milestone"
	PropAssignee  = "Assignee"
	PropRepo      = "Repo"
	PropPR        = "PR"
	// PropBranch is the branch an agent handed its work back on, before there
	// is a pull request to record. It is optional: a project whose Slices table
	// has no such column simply never has a branch to read, and reads back
	// empty rather than failing.
	PropBranch = "Branch"
)

// Milestone status options, in workflow order.
const (
	MilestoneQueued = "Queued"
	MilestoneActive = "Active"
	MilestoneDone   = "Done"
)

// Slice status options, in workflow order. Claimed is the name the middle one
// had in projects created before this app asked the question; it is not a status
// anything reads or writes any more, only the old name [MigrateProject] looks
// for and renames to In progress.
const (
	SliceTodo       = "Todo"
	SliceClaimed    = "Claimed"
	SliceInProgress = "In progress"
	SliceDone       = "Done"
)

// ProjectStructure is the set of Notion objects making up one tracked project:
// the project page, plus the Slices database hanging off it. The data source ID
// is what every later query addresses, so it is what local config stores.
type ProjectStructure struct {
	PageID  string
	PageURL string

	SlicesDBID string
	SlicesDSID string
}

// SlicesSchema is the property schema of a project's Slices data source. Its
// Milestone column is a select, with no options until the plan gives it some:
// a project's milestones are that column's options, in plan order, and it keeps
// no database of its own for them.
//
// The Assignee column is only there when asked for: a single-player project
// tracks work by status alone, and a people column nobody ever fills is a
// column of noise. Everything downstream reads the shape back rather than
// assuming it, so both shapes work.
func SlicesSchema(assignee bool) map[string]PropertySchema {
	schema := map[string]PropertySchema{
		PropName:      SchemaTitle(),
		PropStatus:    SchemaSelect(SliceTodo, SliceInProgress, SliceDone),
		PropMilestone: SchemaSelect(),
		PropRepo:      SchemaRichText(),
		PropPR:        SchemaURL(),
	}
	if assignee {
		schema[PropAssignee] = SchemaPeople()
	}
	return schema
}

// SliceShape is how one project's Slices data source is put together where
// projects differ: what type its Status and Milestone columns are, whether it
// tracks an assignee at all, and which milestones its Milestone column offers.
// It is read from the data source rather than assumed, so a project whose
// columns were converted in the Notion UI is read, claimed and completed the
// same way as one this app made.
type SliceShape struct {
	// StatusType is the property type a status is written in — select or status.
	StatusType string
	// HasAssignee says whether the Assignee people property is there to write.
	HasAssignee bool
	// MilestoneType is the property type of the Milestone column, so a slice can
	// be filed under a milestone in the shape that column was read in.
	MilestoneType string
	// MilestoneOptions are the milestone names the Milestone column offers, in
	// the order the schema lists them, which is the order of the plan.
	MilestoneOptions []string
}

// ShapeOf reads a Slices data source's shape.
func ShapeOf(ds *DataSource) SliceShape {
	shape := SliceShape{StatusType: ds.Properties[PropStatus].Type}
	if assignee, ok := ds.Properties[PropAssignee]; ok && assignee.Type == TypePeople {
		shape.HasAssignee = true
	}
	milestone := ds.Properties[PropMilestone]
	shape.MilestoneType = milestone.Type
	shape.MilestoneOptions = milestone.OptionNames()
	return shape
}

// ProjectsSchema is the property schema of the projects data source — one row
// per tracked project, with the project page's content as its conventions.
func ProjectsSchema() map[string]PropertySchema {
	return map[string]PropertySchema{PropName: SchemaTitle()}
}

// CreateProjectsDatabase creates the database holding one row per tracked
// project, as a child of the given page. Onboarding does this once; every
// project created afterwards is a row of its data source.
func (c *Client) CreateProjectsDatabase(ctx context.Context, parentPageID, title string) (*Database, error) {
	return c.CreateDatabase(ctx, parentPageID, title, ProjectsSchema())
}

// CreateProject creates a project row in the projects data source and, beneath
// that page, the project's Slices database — the whole of its plan, milestones
// included. The created schema is read back and verified before returning.
// assignee says whether the Slices table should carry an Assignee column at all.
//
// A non-nil structure with a non-nil error means everything was created but
// verification failed: the caller can report the mismatch and still record
// what exists rather than orphaning it.
func (c *Client) CreateProject(ctx context.Context, projectsDSID, name string, assignee bool) (*ProjectStructure, error) {
	page, err := c.CreatePage(ctx, DataSourceParent(projectsDSID), map[string]PropertyValue{
		PropName: NewTitle(name),
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("create project page: %w", err)
	}

	slices, slicesDSID, err := c.createProjectDB(ctx, page.ID, SlicesDBTitle, SlicesSchema(assignee))
	if err != nil {
		return nil, err
	}

	s := &ProjectStructure{
		PageID:     page.ID,
		PageURL:    page.URL,
		SlicesDBID: slices.ID,
		SlicesDSID: slicesDSID,
	}
	if err := c.VerifyProjectSchema(ctx, slicesDSID); err != nil {
		return s, err
	}
	return s, nil
}

// createProjectDB creates one of a project's databases and returns it with its
// single data source's ID.
func (c *Client) createProjectDB(ctx context.Context, parentPageID, title string, properties map[string]PropertySchema) (*Database, string, error) {
	db, err := c.CreateDatabase(ctx, parentPageID, title, properties)
	if err != nil {
		return nil, "", fmt.Errorf("create %s database: %w", title, err)
	}
	dsID, ok := db.DataSourceID()
	if !ok {
		return nil, "", fmt.Errorf("create %s database: no data source was returned", title)
	}
	return db, dsID, nil
}

// VerifyProjectSchema checks that a project's Slices data source still carries
// the properties this app depends on, each with the expected type and — for the
// status select — at least the expected options by exact name. Extra properties
// and extra options are left alone: a project may be customised in Notion, it
// just cannot drop what the app reads.
func (c *Client) VerifyProjectSchema(ctx context.Context, slicesDSID string) error {
	return c.verifyDataSource(ctx, SlicesDBTitle, slicesDSID, expectedSliceProperties())
}

// SchemaError reports the ways one data source fails to match the schema the
// app expects.
type SchemaError struct {
	// DataSource names the offending data source, e.g. "Slices".
	DataSource string
	// Problems describes each mismatch, one per missing or wrong property.
	Problems []string
}

// Error implements error.
func (e *SchemaError) Error() string {
	return fmt.Sprintf("%s schema: %s", e.DataSource, strings.Join(e.Problems, "; "))
}

// expectedProperty is one property VerifyProjectSchema insists on. Options is
// set for selects only, and every name in it must be offered.
type expectedProperty struct {
	Name    string
	Type    string
	Options []string
}

// expectedSliceProperties is what a project's Slices data source must carry.
// Assignee is left out: it is optional, and a project tracking work by status
// alone is not a broken one. The Milestone select is expected to exist but to
// offer nothing in particular — its options are the plan, which a project this
// new has none of yet.
func expectedSliceProperties() []expectedProperty {
	return []expectedProperty{
		{Name: PropName, Type: "title"},
		{Name: PropStatus, Type: "select", Options: []string{SliceTodo, SliceInProgress, SliceDone}},
		{Name: PropMilestone, Type: "select"},
		{Name: PropRepo, Type: "rich_text"},
		{Name: PropPR, Type: "url"},
	}
}

// verifyDataSource fetches a data source and checks it against expected,
// reporting every mismatch at once rather than stopping at the first.
func (c *Client) verifyDataSource(ctx context.Context, name, id string, expected []expectedProperty) error {
	ds, err := c.GetDataSource(ctx, id)
	if err != nil {
		return fmt.Errorf("verify %s schema: %w", name, err)
	}
	problems := schemaProblems(ds, expected)
	if len(problems) == 0 {
		return nil
	}
	return &SchemaError{DataSource: name, Problems: problems}
}

// schemaProblems lists how ds falls short of expected.
func schemaProblems(ds *DataSource, expected []expectedProperty) []string {
	var problems []string
	for _, e := range expected {
		got, ok := ds.Properties[e.Name]
		if !ok {
			problems = append(problems, fmt.Sprintf("missing property %q (%s)", e.Name, e.Type))
			continue
		}
		if got.Type != e.Type {
			problems = append(problems, fmt.Sprintf("property %q is a %s, want %s", e.Name, got.Type, e.Type))
			continue
		}
		options := got.OptionNames()
		have := make(map[string]bool, len(options))
		for _, o := range options {
			have[o] = true
		}
		for _, want := range e.Options {
			if !have[want] {
				problems = append(problems, fmt.Sprintf("property %q is missing option %q", e.Name, want))
			}
		}
	}
	return problems
}
