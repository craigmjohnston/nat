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
	DeleteBlock(ctx context.Context, id string) error
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
	// StatusRenamed reports the Claimed status option become In progress. The
	// API silently ignores renaming an option in place, so it is done the long
	// way — In progress appended, the slices sitting on Claimed moved over, and
	// Claimed dropped — and reported only once all three have happened.
	StatusRenamed bool
	// MilestonesTrashed reports the old Milestones database moved to Notion's
	// trash, its milestones now living entirely on the Milestone column.
	MilestonesTrashed bool
}

// Empty reports whether nothing was migrated.
func (m Migration) Empty() bool {
	return len(m.Milestones) == 0 && m.Slices == 0 && !m.StatusRenamed && !m.MilestonesTrashed
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
	if m.MilestonesTrashed {
		parts = append(parts, "the old Milestones database moved to Notion's trash")
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
// everything now reads: one page holding the whole plan, milestones as the
// options of the slices' own Milestone column, and one name for the in-progress
// status.
//
// The migrated things, each idempotent — a project already in the one shape is
// read and left alone, which is what every load after the first does:
//
//   - A Milestone relation becomes a select whose options are the project's
//     milestones in plan order, and every slice is refiled onto that option from
//     the milestone page it related to.
//   - The Milestones database, its milestones now wholly on the Milestone
//     column, is moved to Notion's trash — recoverable there, not destroyed.
//   - A Claimed status option becomes In progress. The API quietly ignores
//     renaming an option in place, so it is done the long way: In progress is
//     appended, every slice sitting on Claimed is moved onto it, and Claimed is
//     dropped once nothing holds it.
//
// The Slices database itself — a full-page child of the project page, its first
// view the table the plan's order is read from — is left exactly as it is.
//
// The whole migration is settled before the first write: a project whose Status
// column was converted in the Notion UI to a type this app cannot write options
// to, or whose milestones data source names no database to trash, is refused
// whole rather than half-migrated.
//
// The plan is read in full before the schema changes, because converting the
// column is what discards the relations it is read from. A run that dies partway
// leaves the milestone pages in Notion to refile from — the Milestones database
// goes to the trash last, after everything else has succeeded — so what was
// written is reported rather than assumed.
func MigrateProject(ctx context.Context, api MigrationAPI, slicesDSID string) (*DataSource, Migration, error) {
	ds, err := api.GetDataSource(ctx, slicesDSID)
	if err != nil {
		return nil, Migration{}, fmt.Errorf("load the slices schema: %w", err)
	}

	milestone, status := ds.Properties[PropMilestone], ds.Properties[PropStatus]
	replaceStatus := renamesClaimed(status)
	if milestone.Relation == nil && !replaceStatus {
		return ds, Migration{}, nil
	}
	if replaceStatus && status.Select == nil {
		return nil, Migration{}, fmt.Errorf(
			"the %s column is a %s, whose options this app cannot write: "+
				"rename its %q option to %q in Notion and open the board again",
			PropStatus, status.Type, SliceClaimed, SliceInProgress)
	}

	// Read everything before the first write: the schema change discards the
	// relations the plan is read from, and a refusal here leaves the project
	// exactly as it was.
	var report Migration
	var names []string
	var byPage map[string]string
	var milestonesDB string
	if milestone.Relation != nil {
		if names, byPage, err = milestonePlan(ctx, api, milestone.Relation.DataSourceID); err != nil {
			return nil, Migration{}, err
		}
		if milestonesDB, err = milestonesDatabase(ctx, api, milestone.Relation.DataSourceID); err != nil {
			return nil, Migration{}, err
		}
	}
	slices, err := api.QueryDataSource(ctx, slicesDSID, nil,
		[]Sort{{Timestamp: TimestampCreated, Direction: SortAscending}})
	if err != nil {
		return nil, Migration{}, fmt.Errorf("load slices: %w", err)
	}
	writes := sliceWrites(slices, byPage, replaceStatus)

	properties := map[string]PropertySchema{}
	if milestone.Relation != nil {
		properties[PropMilestone] = SchemaSelect(names...)
		report.Milestones = names
	}
	if replaceStatus {
		// Renaming Claimed in place is silently ignored by the API, so In
		// progress is appended here — alongside Claimed, which the slices still
		// sit on — and Claimed is dropped below once they have moved.
		properties[PropStatus], _ = status.AppendedOptions(SliceInProgress)
	}
	updated, err := api.UpdateDataSourceProperties(ctx, slicesDSID, properties)
	if err != nil {
		return nil, Migration{}, fmt.Errorf("migrate the slices schema: %w", err)
	}

	for _, w := range writes {
		if _, err := api.UpdatePageProperties(ctx, w.pageID, w.properties); err != nil {
			if w.milestone != "" {
				return nil, Migration{}, fmt.Errorf("file slice %s under %q: %w", w.pageID, w.milestone, err)
			}
			return nil, Migration{}, fmt.Errorf("move slice %s to %q: %w", w.pageID, SliceInProgress, err)
		}
		if w.milestone != "" {
			report.Slices++
		}
	}

	if replaceStatus {
		// Nothing sits on Claimed any more; drop it. The options are sent back
		// exactly as the schema write echoed them — In progress now has an ID —
		// minus the one being retired.
		without := withoutOption(updated.Properties[PropStatus], SliceClaimed)
		if updated, err = api.UpdateDataSourceProperties(ctx, slicesDSID,
			map[string]PropertySchema{PropStatus: without}); err != nil {
			return nil, Migration{}, fmt.Errorf("retire the %q option: %w", SliceClaimed, err)
		}
		report.StatusRenamed = true
	}

	if milestone.Relation != nil {
		// The Milestones database goes last: until here it was still in place
		// to refile from, and from here the plan needs nothing it holds.
		if err := api.DeleteBlock(ctx, milestonesDB); err != nil {
			return nil, Migration{}, fmt.Errorf("trash the Milestones database: %w", err)
		}
		report.MilestonesTrashed = true
	}

	logging.Action("project migrated", "data_source", slicesDSID,
		"milestones", len(report.Milestones), "slices", report.Slices,
		"status_renamed", report.StatusRenamed,
		"milestones_trashed", report.MilestonesTrashed)
	return updated, report, nil
}

// renamesClaimed reports whether a Status column still offers the old name for
// the in-progress option and nothing else. A column offering both names is left
// alone: moving one option's slices onto the other is a decision about
// somebody's slices rather than a rename.
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

// withoutOption is the select definition minus the named option. Every option
// kept is sent back exactly as it was read — ID, name and colour — because
// Notion replaces an option list wholesale: what the list omits is removed.
func withoutOption(status PropertySchema, name string) PropertySchema {
	options := make([]SelectOption, 0, len(status.Select.Options))
	for _, o := range status.Select.Options {
		if o.Name != name {
			options = append(options, o)
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

// milestonesDatabase names the database whose rows the Milestones data source
// holds — the child of the project page that is trashed once the plan has moved
// off it. It is resolved before anything is written, so a project it cannot be
// read for is refused whole.
func milestonesDatabase(ctx context.Context, api MigrationAPI, milestonesDSID string) (string, error) {
	ds, err := api.GetDataSource(ctx, milestonesDSID)
	if err != nil {
		return "", fmt.Errorf("load the milestones database: %w", err)
	}
	if ds.Parent.DatabaseID == "" {
		return "", fmt.Errorf("the milestones data source names no database to trash")
	}
	return ds.Parent.DatabaseID, nil
}

// pageWrite is the properties one slice is to be written during migration: the
// milestone option its relation named, the in-progress status for the old name,
// or both in the one write.
type pageWrite struct {
	pageID     string
	properties map[string]PropertyValue
	// milestone is the option the slice is refiled under, "" for a write that
	// only moves its status.
	milestone string
}

// sliceWrites reads what each slice is to be written, before the schema changes
// that would discard the relations it is read from. A slice relating to
// nothing, or to a page the milestones query did not return, has no option to
// file it under and is left with an empty Milestone — the board shows those as
// Unassigned rather than losing them.
func sliceWrites(pages []Page, byPage map[string]string, replaceStatus bool) []pageWrite {
	var writes []pageWrite
	for _, p := range pages {
		w := pageWrite{pageID: p.ID, properties: map[string]PropertyValue{}}
		if ids := p.Properties[PropMilestone].RelationIDs(); len(ids) > 0 {
			if name, ok := byPage[normalisedID(ids[0])]; ok {
				w.properties[PropMilestone] = NewSelect(name)
				w.milestone = name
			}
		}
		if replaceStatus && p.Properties[PropStatus].SelectName() == SliceClaimed {
			w.properties[PropStatus] = NewSelect(SliceInProgress)
		}
		if len(w.properties) > 0 {
			writes = append(writes, w)
		}
	}
	return writes
}

// normalisedID puts a Notion ID in one form, so IDs read from different places
// compare equal however they were written.
func normalisedID(id string) string {
	return strings.ToLower(strings.ReplaceAll(id, "-", ""))
}
