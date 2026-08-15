package notion

import (
	"context"
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/logging"
)

// MigrationAPI is the part of the client migrating a project needs, as an
// interface so the callers that already hold a narrower one — the TUI's, the
// CLI's — can pass it straight through.
type MigrationAPI interface {
	GetDataSource(ctx context.Context, id string) (*DataSource, error)
	QueryDataSource(ctx context.Context, id string, filter map[string]any, sorts []Sort) ([]Page, error)
	UpdateDataSourceProperties(ctx context.Context, id string, properties map[string]PropertySchema) (*DataSource, error)
	UpdatePageProperties(ctx context.Context, pageID string, properties map[string]PropertyValue) (*Page, error)
}

// Migration is what migrating one project changed, so the run can say. A
// migration that changed nothing — every project already in the one shape, which
// is every project from the second load on — is Empty.
type Migration struct {
	// Milestones are the names the Milestone column gained as options, in plan
	// order: the project's milestone pages, now the whole of its plan.
	Milestones []string
	// Slices counts the slices refiled from their relation onto that column.
	Slices int
	// StatusRenamed reports the Claimed status option renamed to In progress.
	StatusRenamed bool
}

// Empty reports whether nothing was migrated.
func (m Migration) Empty() bool {
	return len(m.Milestones) == 0 && m.Slices == 0 && !m.StatusRenamed
}

// Summary says what changed, in one line, for a status bar or a log.
func (m Migration) Summary() string {
	var parts []string
	if len(m.Milestones) > 0 {
		parts = append(parts, fmt.Sprintf("%d %s moved onto the slices", len(m.Milestones),
			pluralise("milestone", len(m.Milestones))))
	}
	if m.Slices > 0 {
		parts = append(parts, fmt.Sprintf("%d %s refiled", m.Slices, pluralise("slice", m.Slices)))
	}
	if m.StatusRenamed {
		parts = append(parts, fmt.Sprintf("%q renamed to %q", SliceClaimed, SliceInProgress))
	}
	if len(parts) == 0 {
		return "nothing to migrate"
	}
	return "Migrated this project: " + strings.Join(parts, ", ") + "."
}

// pluralise pluralises an English noun by the count it is used with.
func pluralise(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// MigrateProject reads a project's Slices data source and, where it is still in
// the shape this app started with, migrates it in place to the one shape
// everything now reads: milestones as the options of the slices' own Milestone
// column, and one name for the in-progress status.
//
// Two things are migrated, and both are idempotent — a project already in the
// one shape is read and left alone, which is what every load after the first
// does:
//
//   - A Milestone relation becomes a select whose options are the project's
//     milestones in plan order, and every slice is refiled onto that option from
//     the milestone page it related to. The Milestones database itself is left
//     exactly as it is, simply never read again: nothing of the user's plan is
//     destroyed, and it is still there to look at.
//   - A Claimed status option is renamed to In progress, by ID, so the slices
//     sitting on it keep their status.
//
// The whole migration is settled before the first write: a project whose Status
// column was converted in the Notion UI to a type this app cannot write options
// to is refused with the one edit to make there, rather than half-migrated.
//
// The plan is read in full before the schema changes, because converting the
// column is what discards the relations it is read from. A run that dies between
// the schema write and the last slice leaves those slices unfiled — their
// milestone pages are still in Notion to refile them from — so what was written
// is reported rather than assumed.
func MigrateProject(ctx context.Context, api MigrationAPI, slicesDSID string) (*DataSource, Migration, error) {
	ds, err := api.GetDataSource(ctx, slicesDSID)
	if err != nil {
		return nil, Migration{}, fmt.Errorf("load the slices schema: %w", err)
	}

	milestone, status := ds.Properties[PropMilestone], ds.Properties[PropStatus]
	renameStatus := renamesClaimed(status)
	if milestone.Relation == nil && !renameStatus {
		return ds, Migration{}, nil
	}
	if renameStatus && status.Select == nil {
		return nil, Migration{}, fmt.Errorf(
			"the %s column is a %s, whose options this app cannot write: "+
				"rename its %q option to %q in Notion and open the board again",
			PropStatus, status.Type, SliceClaimed, SliceInProgress)
	}

	var report Migration
	var filing []sliceFiling
	properties := map[string]PropertySchema{}
	if milestone.Relation != nil {
		names, byPage, err := milestonePlan(ctx, api, milestone.Relation.DataSourceID)
		if err != nil {
			return nil, Migration{}, err
		}
		if filing, err = sliceFilings(ctx, api, slicesDSID, byPage); err != nil {
			return nil, Migration{}, err
		}
		properties[PropMilestone] = SchemaSelect(names...)
		report.Milestones = names
	}
	if renameStatus {
		properties[PropStatus] = renamedOption(status, SliceClaimed, SliceInProgress)
		report.StatusRenamed = true
	}

	updated, err := api.UpdateDataSourceProperties(ctx, slicesDSID, properties)
	if err != nil {
		return nil, Migration{}, fmt.Errorf("migrate the slices schema: %w", err)
	}
	for _, f := range filing {
		if _, err := api.UpdatePageProperties(ctx, f.pageID,
			map[string]PropertyValue{PropMilestone: NewSelect(f.milestone)}); err != nil {
			return nil, Migration{}, fmt.Errorf("file slice %s under %q: %w", f.pageID, f.milestone, err)
		}
		report.Slices++
	}
	logging.Action("project migrated", "data_source", slicesDSID,
		"milestones", len(report.Milestones), "slices", report.Slices, "status_renamed", report.StatusRenamed)
	return updated, report, nil
}

// renamesClaimed reports whether a Status column still offers the old name for
// the in-progress option and nothing else. A column offering both names is left
// alone: renaming one onto the other would ask Notion for two options of a name,
// and merging them is a decision about somebody's slices rather than a rename.
func renamesClaimed(status PropertySchema) bool {
	var claimed, inProgress bool
	for _, name := range status.OptionNames() {
		switch name {
		case SliceClaimed:
			claimed = true
		case SliceInProgress:
			inProgress = true
		}
	}
	return claimed && !inProgress
}

// renamedOption is the select definition with one option renamed. Every option
// is sent back as it was read — ID, name and colour — because Notion replaces an
// option list wholesale; the one being renamed keeps its ID, which is what makes
// this a rename rather than a new option, so the pages sitting on it keep their
// value.
func renamedOption(status PropertySchema, from, to string) PropertySchema {
	options := make([]SelectOption, len(status.Select.Options))
	copy(options, status.Select.Options)
	for i := range options {
		if options[i].Name == from {
			options[i].Name = to
		}
	}
	return PropertySchema{Select: &OptionsConfig{Options: options}}
}

// milestonePlan reads a project's Milestones data source: the milestone names in
// plan order, and the page ID of each mapped onto the name it becomes an option
// under.
//
// A milestone with no name cannot be an option, and two sharing one cannot be
// told apart, so unnamed milestones are dropped and a repeated name is kept
// once — with every page of that name filing its slices under the one option.
func milestonePlan(ctx context.Context, api MigrationAPI, milestonesDSID string) ([]string, map[string]string, error) {
	if milestonesDSID == "" {
		return nil, nil, fmt.Errorf("the %s relation names no data source to migrate from", PropMilestone)
	}
	pages, err := api.QueryDataSource(ctx, milestonesDSID, nil,
		[]Sort{{Property: PropOrder, Direction: SortAscending}})
	if err != nil {
		return nil, nil, fmt.Errorf("load milestones: %w", err)
	}

	var names []string
	taken := map[string]string{}
	byPage := make(map[string]string, len(pages))
	for _, p := range pages {
		name := strings.TrimSpace(p.Properties[PropName].Text())
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		held, dup := taken[key]
		if !dup {
			held = name
			taken[key] = name
			names = append(names, name)
		}
		byPage[normalisedID(p.ID)] = held
	}
	return names, byPage, nil
}

// sliceFiling is one slice and the milestone option it is to be refiled under.
type sliceFiling struct {
	pageID    string
	milestone string
}

// sliceFilings reads which milestone each slice relates to, before the column
// that says so is converted. A slice relating to nothing, or to a page the
// milestones query did not return, has no option to file it under and is left
// with an empty Milestone — the board shows those as Unassigned rather than
// losing them.
func sliceFilings(ctx context.Context, api MigrationAPI, slicesDSID string, byPage map[string]string) ([]sliceFiling, error) {
	pages, err := api.QueryDataSource(ctx, slicesDSID, nil,
		[]Sort{{Timestamp: TimestampCreated, Direction: SortAscending}})
	if err != nil {
		return nil, fmt.Errorf("load slices: %w", err)
	}
	var filings []sliceFiling
	for _, p := range pages {
		ids := p.Properties[PropMilestone].RelationIDs()
		if len(ids) == 0 {
			continue
		}
		if name, ok := byPage[normalisedID(ids[0])]; ok {
			filings = append(filings, sliceFiling{pageID: p.ID, milestone: name})
		}
	}
	return filings, nil
}

// normalisedID puts a Notion ID in one form, so IDs read from different places
// compare equal however they were written.
func normalisedID(id string) string {
	return strings.ToLower(strings.ReplaceAll(id, "-", ""))
}
