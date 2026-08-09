package notion

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Titles of the databases every tracked project owns.
const (
	MilestonesDBTitle = "Milestones"
	SlicesDBTitle     = "Slices"
)

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
)

// Milestone status options, in workflow order.
const (
	MilestoneQueued = "Queued"
	MilestoneActive = "Active"
	MilestoneDone   = "Done"
)

// Slice status options, in workflow order.
const (
	SliceTodo    = "Todo"
	SliceClaimed = "Claimed"
	SliceDone    = "Done"
)

// ProjectStructure is the set of Notion objects making up one tracked project:
// the project page, plus the two databases hanging off it. The data source IDs
// are what every later query addresses, so they are what local config stores.
type ProjectStructure struct {
	PageID  string
	PageURL string

	MilestonesDBID string
	MilestonesDSID string
	SlicesDBID     string
	SlicesDSID     string
}

// MilestonesSchema is the property schema of a project's Milestones data
// source.
func MilestonesSchema() map[string]PropertySchema {
	return map[string]PropertySchema{
		PropName:   SchemaTitle(),
		PropOrder:  SchemaNumber(),
		PropStatus: SchemaSelect(MilestoneQueued, MilestoneActive, MilestoneDone),
	}
}

// SlicesSchema is the property schema of a project's Slices data source, whose
// Milestone relation points at the project's Milestones data source.
func SlicesSchema(milestonesDSID string) map[string]PropertySchema {
	return map[string]PropertySchema{
		PropName:      SchemaTitle(),
		PropStatus:    SchemaSelect(SliceTodo, SliceClaimed, SliceDone),
		PropMilestone: SchemaRelation(milestonesDSID),
		PropAssignee:  SchemaPeople(),
		PropRepo:      SchemaRichText(),
		PropPR:        SchemaURL(),
	}
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
// that page, the project's Milestones and Slices databases — Milestones first,
// because the slice schema's relation points at it. The created schema is read
// back and verified before returning.
//
// A non-nil structure with a non-nil error means everything was created but
// verification failed: the caller can report the mismatch and still record
// what exists rather than orphaning it.
func (c *Client) CreateProject(ctx context.Context, projectsDSID, name string) (*ProjectStructure, error) {
	page, err := c.CreatePage(ctx, DataSourceParent(projectsDSID), map[string]PropertyValue{
		PropName: NewTitle(name),
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("create project page: %w", err)
	}

	milestones, milestonesDSID, err := c.createProjectDB(ctx, page.ID, MilestonesDBTitle, MilestonesSchema())
	if err != nil {
		return nil, err
	}
	slices, slicesDSID, err := c.createProjectDB(ctx, page.ID, SlicesDBTitle, SlicesSchema(milestonesDSID))
	if err != nil {
		return nil, err
	}

	s := &ProjectStructure{
		PageID:         page.ID,
		PageURL:        page.URL,
		MilestonesDBID: milestones.ID,
		MilestonesDSID: milestonesDSID,
		SlicesDBID:     slices.ID,
		SlicesDSID:     slicesDSID,
	}
	if err := c.VerifyProjectSchema(ctx, milestonesDSID, slicesDSID); err != nil {
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

// VerifyProjectSchema checks that a project's two data sources still carry the
// properties this app depends on, each with the expected type and — for the
// status selects — at least the expected options by exact name. Extra
// properties and extra options are left alone: a project may be customised in
// Notion, it just cannot drop what the app reads.
func (c *Client) VerifyProjectSchema(ctx context.Context, milestonesDSID, slicesDSID string) error {
	return errors.Join(
		c.verifyDataSource(ctx, MilestonesDBTitle, milestonesDSID, expectedMilestoneProperties()),
		c.verifyDataSource(ctx, SlicesDBTitle, slicesDSID, expectedSliceProperties(milestonesDSID)),
	)
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
// set for selects only, RelationDSID for relations only.
type expectedProperty struct {
	Name         string
	Type         string
	Options      []string
	RelationDSID string
}

func expectedMilestoneProperties() []expectedProperty {
	return []expectedProperty{
		{Name: PropName, Type: "title"},
		{Name: PropOrder, Type: "number"},
		{Name: PropStatus, Type: "select", Options: []string{MilestoneQueued, MilestoneActive, MilestoneDone}},
	}
}

func expectedSliceProperties(milestonesDSID string) []expectedProperty {
	return []expectedProperty{
		{Name: PropName, Type: "title"},
		{Name: PropStatus, Type: "select", Options: []string{SliceTodo, SliceClaimed, SliceDone}},
		{Name: PropMilestone, Type: "relation", RelationDSID: milestonesDSID},
		{Name: PropAssignee, Type: "people"},
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
		if e.RelationDSID != "" && (got.Relation == nil || !sameID(got.Relation.DataSourceID, e.RelationDSID)) {
			problems = append(problems, fmt.Sprintf("property %q does not relate to data source %s", e.Name, e.RelationDSID))
		}
	}
	return problems
}

// sameID compares two Notion IDs. The API returns them dashed, but config
// files and URLs carry them either way, so dashes and case are ignored.
func sameID(a, b string) bool {
	return strings.EqualFold(strings.ReplaceAll(a, "-", ""), strings.ReplaceAll(b, "-", ""))
}
