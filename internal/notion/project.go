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

// Slice status options, in workflow order. There are two names for the middle
// one: projects created before this app asked the question call it Claimed, and
// newer ones call it In progress. Both mean the same thing — a slice an agent
// holds — and neither is migrated to the other, so every path that reads or
// writes a status resolves the name from the project's own schema.
const (
	SliceTodo       = "Todo"
	SliceClaimed    = "Claimed"
	SliceInProgress = "In progress"
	SliceDone       = "Done"
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
//
// The Assignee column is only there when asked for: a single-player project
// tracks work by status alone, and a people column nobody ever fills is a
// column of noise. Everything downstream reads the shape back rather than
// assuming it, so both shapes work.
func SlicesSchema(milestonesDSID string, assignee bool) map[string]PropertySchema {
	schema := map[string]PropertySchema{
		PropName:      SchemaTitle(),
		PropStatus:    SchemaSelect(SliceTodo, SliceInProgress, SliceDone),
		PropMilestone: SchemaRelation(milestonesDSID),
		PropRepo:      SchemaRichText(),
		PropPR:        SchemaURL(),
	}
	if assignee {
		schema[PropAssignee] = SchemaPeople()
	}
	return schema
}

// SliceShape is how one project's Slices data source is put together where the
// shapes differ: what its in-progress status option is called, what type that
// column is, whether it tracks an assignee at all, and how its Milestone column
// names a plan. It is read from the data source rather than assumed, so a
// project created under any of those shapes can be read, claimed and completed
// the same way.
type SliceShape struct {
	// InProgress is the status option name a claim writes.
	InProgress string
	// StatusType is the property type to write it in — select or status.
	StatusType string
	// HasAssignee says whether the Assignee people property is there to write.
	HasAssignee bool
	// MilestoneType is the property type of the Milestone column: a relation to
	// a Milestones data source, or a fixed-choice column naming the milestone
	// on the slice itself.
	MilestoneType string
	// MilestoneOptions are the milestone names a fixed-choice Milestone column
	// offers, in the order the schema lists them, which is the order the plan
	// is written in. It is nil under the relation shape, where the milestones
	// are pages of their own.
	MilestoneOptions []string
}

// MilestonesRelated reports whether the project keeps its milestones in a
// Milestones data source of its own. The question asked is whether the
// Milestone column offers milestones to choose from, not what type it says it
// is: a column this build cannot read options off — a relation, or one missing
// altogether — means the milestones are pages elsewhere, which is what every
// project read before the second shape existed had.
func (s SliceShape) MilestonesRelated() bool { return s.MilestoneOptions == nil }

// ShapeOf reads a Slices data source's shape. An in-progress option it cannot
// find a name for falls back to Claimed, which is what every project made
// before the option existed calls it — and what a Status column converted in
// the Notion UI to a type whose options this app cannot read is likeliest to
// be.
func ShapeOf(ds *DataSource) SliceShape {
	status := ds.Properties[PropStatus]
	shape := SliceShape{InProgress: SliceClaimed, StatusType: status.Type}
	for _, name := range status.OptionNames() {
		if name == SliceInProgress {
			shape.InProgress = SliceInProgress
			break
		}
	}
	if assignee, ok := ds.Properties[PropAssignee]; ok && assignee.Type == TypePeople {
		shape.HasAssignee = true
	}
	milestone := ds.Properties[PropMilestone]
	shape.MilestoneType = milestone.Type
	if milestone.Relation == nil {
		shape.MilestoneOptions = milestone.OptionNames()
	}
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
// that page, the project's Milestones and Slices databases — Milestones first,
// because the slice schema's relation points at it. The created schema is read
// back and verified before returning. assignee says whether the Slices table
// should carry an Assignee column at all.
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

	milestones, milestonesDSID, err := c.createProjectDB(ctx, page.ID, MilestonesDBTitle, MilestonesSchema())
	if err != nil {
		return nil, err
	}
	slices, slicesDSID, err := c.createProjectDB(ctx, page.ID, SlicesDBTitle, SlicesSchema(milestonesDSID, assignee))
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

// expectedProperty is one property VerifyProjectSchema insists on. Options and
// AnyOptions are set for selects only, RelationDSID for relations only. Every
// name in Options must be offered; at least one of AnyOptions must be, which is
// how a column that may legitimately be named either way is checked.
type expectedProperty struct {
	Name         string
	Type         string
	Options      []string
	AnyOptions   []string
	RelationDSID string
}

func expectedMilestoneProperties() []expectedProperty {
	return []expectedProperty{
		{Name: PropName, Type: "title"},
		{Name: PropOrder, Type: "number"},
		{Name: PropStatus, Type: "select", Options: []string{MilestoneQueued, MilestoneActive, MilestoneDone}},
	}
}

// expectedSliceProperties is what a project's Slices data source must carry,
// under either shape: Assignee is optional, and the in-progress status may be
// called by either name.
func expectedSliceProperties(milestonesDSID string) []expectedProperty {
	return []expectedProperty{
		{Name: PropName, Type: "title"},
		{
			Name:       PropStatus,
			Type:       "select",
			Options:    []string{SliceTodo, SliceDone},
			AnyOptions: []string{SliceInProgress, SliceClaimed},
		},
		{Name: PropMilestone, Type: "relation", RelationDSID: milestonesDSID},
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
		if len(e.AnyOptions) > 0 && !hasAny(have, e.AnyOptions) {
			problems = append(problems, fmt.Sprintf("property %q offers none of the options %s",
				e.Name, quoteList(e.AnyOptions)))
		}
		if e.RelationDSID != "" && (got.Relation == nil || !sameID(got.Relation.DataSourceID, e.RelationDSID)) {
			problems = append(problems, fmt.Sprintf("property %q does not relate to data source %s", e.Name, e.RelationDSID))
		}
	}
	return problems
}

// hasAny reports whether any of the wanted option names is offered.
func hasAny(have map[string]bool, wanted []string) bool {
	for _, want := range wanted {
		if have[want] {
			return true
		}
	}
	return false
}

// quoteList renders option names for a problem line: "a" or "b".
func quoteList(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = fmt.Sprintf("%q", n)
	}
	return strings.Join(quoted, " or ")
}

// sameID compares two Notion IDs. The API returns them dashed, but config
// files and URLs carry them either way, so dashes and case are ignored.
func sameID(a, b string) bool {
	return strings.EqualFold(strings.ReplaceAll(a, "-", ""), strings.ReplaceAll(b, "-", ""))
}
