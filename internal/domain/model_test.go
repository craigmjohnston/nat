package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/notion"
)

// page decodes a Notion page from JSON, so the mapping tests exercise the same
// shapes a data source query actually returns rather than hand-built structs.
func page(t *testing.T, body string) notion.Page {
	t.Helper()
	var p notion.Page
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("decoding page: %v", err)
	}
	return p
}

func TestMilestoneFromPage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want Milestone
	}{
		{
			"select shape",
			`{
				"id": "m1",
				"url": "https://notion.test/m1",
				"properties": {
					"Name": {"type": "title", "title": [{"plain_text": "M4: Read-only board"}]},
					"Order": {"type": "number", "number": 4},
					"Status": {"type": "select", "select": {"name": "Active"}}
				}
			}`,
			Milestone{ID: "m1", Name: "M4: Read-only board", Order: 4, Status: MilestoneActive,
				StatusType: notion.TypeSelect, URL: "https://notion.test/m1"},
		},
		{
			"status shape",
			`{
				"id": "m2",
				"properties": {
					"Name": {"type": "title", "title": [{"plain_text": "M5"}]},
					"Order": {"type": "number", "number": 5},
					"Status": {"type": "status", "status": {"name": "Queued"}}
				}
			}`,
			Milestone{ID: "m2", Name: "M5", Order: 5, Status: MilestoneQueued, StatusType: notion.TypeStatus},
		},
		{
			"missing properties map to zero values",
			`{"id": "m3", "properties": {}}`,
			Milestone{ID: "m3"},
		},
		{
			"unset order is zero",
			`{
				"id": "m4",
				"properties": {
					"Name": {"type": "title", "title": [{"plain_text": "M6"}]},
					"Order": {"type": "number", "number": null},
					"Status": {"type": "select", "select": {"name": "Done"}}
				}
			}`,
			Milestone{ID: "m4", Name: "M6", Status: MilestoneDone, StatusType: notion.TypeSelect},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MilestoneFromPage(page(t, tt.body)); got != tt.want {
				t.Errorf("MilestoneFromPage() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSliceFromPage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want Slice
	}{
		{
			"select shape, fully populated",
			`{
				"id": "s1",
				"url": "https://notion.test/s1",
				"properties": {
					"Name": {"type": "title", "title": [{"plain_text": "Domain model"}]},
					"Status": {"type": "select", "select": {"name": "Claimed"}},
					"Milestone": {"type": "relation", "relation": [{"id": "m1"}]},
					"Assignee": {"type": "people", "people": [{"id": "u1", "name": "Craig Johnston"}]},
					"Repo": {"type": "rich_text", "rich_text": [{"plain_text": "/repos/other"}]},
					"PR": {"type": "url", "url": "https://github.test/pr/1"}
				}
			}`,
			Slice{
				ID: "s1", Name: "Domain model", Status: SliceClaimed, StatusName: "Claimed", MilestoneID: "m1",
				AssigneeName: "Craig Johnston", Repo: "/repos/other",
				PRURL: "https://github.test/pr/1", URL: "https://notion.test/s1",
			},
		},
		{
			"status shape",
			`{
				"id": "s2",
				"properties": {
					"Name": {"type": "title", "title": [{"plain_text": "Board screen"}]},
					"Status": {"type": "status", "status": {"name": "Todo"}}
				}
			}`,
			Slice{ID: "s2", Name: "Board screen", Status: SliceTodo, StatusName: "Todo"},
		},
		{
			"unclaimed and unrelated slice",
			`{
				"id": "s3",
				"properties": {
					"Name": {"type": "title", "title": [{"plain_text": "Loose"}]},
					"Status": {"type": "select", "select": {"name": "Todo"}},
					"Milestone": {"type": "relation", "relation": []},
					"Assignee": {"type": "people", "people": []}
				}
			}`,
			Slice{ID: "s3", Name: "Loose", Status: SliceTodo, StatusName: "Todo"},
		},
		{
			"in progress is the same status as claimed",
			`{
				"id": "s6",
				"properties": {
					"Name": {"type": "title", "title": [{"plain_text": "Newer project"}]},
					"Status": {"type": "select", "select": {"name": "In progress"}}
				}
			}`,
			Slice{ID: "s6", Name: "Newer project", Status: SliceClaimed, StatusName: "In progress"},
		},
		{
			"a status this build does not know is carried through",
			`{"id": "s7", "properties": {"Status": {"type": "select", "select": {"name": "Parked"}}}}`,
			Slice{ID: "s7", Status: SliceStatus("Parked"), StatusName: "Parked"},
		},
		{
			"only the first relation and assignee are read",
			`{
				"id": "s4",
				"properties": {
					"Milestone": {"type": "relation", "relation": [{"id": "m1"}, {"id": "m2"}]},
					"Assignee": {"type": "people", "people": [{"id": "u1", "name": "First"}, {"id": "u2", "name": "Second"}]}
				}
			}`,
			Slice{ID: "s4", MilestoneID: "m1", AssigneeName: "First"},
		},
		{
			"missing properties map to zero values",
			`{"id": "s5", "properties": {}}`,
			Slice{ID: "s5"},
		},
		{
			"a milestone named on a select is the milestone",
			`{
				"id": "s8",
				"properties": {
					"Name": {"type": "title", "title": [{"plain_text": "Single page"}]},
					"Status": {"type": "select", "select": {"name": "Todo"}},
					"Milestone": {"type": "select", "select": {"name": "M2: The board"}}
				}
			}`,
			Slice{ID: "s8", Name: "Single page", Status: SliceTodo, StatusName: "Todo", MilestoneID: "M2: The board"},
		},
		{
			"a Milestone select naming nothing leaves the slice unassigned",
			`{"id": "s9", "properties": {"Milestone": {"type": "select", "select": null}}}`,
			Slice{ID: "s9"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SliceFromPage(page(t, tt.body)); got != tt.want {
				t.Errorf("SliceFromPage() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMilestonesFromPages(t *testing.T) {
	pages := []notion.Page{page(t, `{"id":"m1"}`), page(t, `{"id":"m2"}`)}
	want := []Milestone{{ID: "m1"}, {ID: "m2"}}
	if got := MilestonesFromPages(pages); !reflect.DeepEqual(got, want) {
		t.Errorf("MilestonesFromPages() = %+v, want %+v", got, want)
	}
	if got := MilestonesFromPages(nil); len(got) != 0 {
		t.Errorf("MilestonesFromPages(nil) = %+v, want empty", got)
	}
}

func TestMilestonesFromOptions(t *testing.T) {
	want := []Milestone{
		{ID: "M1: Groundwork", Name: "M1: Groundwork", Order: 0, Derived: true},
		{ID: "M2: The board", Name: "M2: The board", Order: 1, Derived: true},
	}
	if got := MilestonesFromOptions([]string{"M1: Groundwork", "M2: The board"}); !reflect.DeepEqual(got, want) {
		t.Errorf("MilestonesFromOptions() = %+v, want %+v", got, want)
	}
	if got := MilestonesFromOptions(nil); len(got) != 0 {
		t.Errorf("MilestonesFromOptions(nil) = %+v, want empty", got)
	}
}

// TestMilestonesFromOptionsGroupSlices is the point of naming a milestone after
// its option: a plan read from a select groups exactly as one read from pages.
func TestMilestonesFromOptionsGroupSlices(t *testing.T) {
	p := Project{
		Milestones: MilestonesFromOptions([]string{"M1: Groundwork", "M2: The board"}),
		Slices: []Slice{
			{ID: "s1", MilestoneID: "M2: The board"},
			{ID: "s2", MilestoneID: "M1: Groundwork"},
			{ID: "s3", MilestoneID: "M9: Retired"},
			{ID: "s4"},
		},
	}
	var got []string
	for _, g := range p.Groups() {
		names := make([]string, len(g.Slices))
		for i, s := range g.Slices {
			names[i] = s.ID
		}
		got = append(got, g.Name()+": "+strings.Join(names, ","))
	}
	want := []string{"M1: Groundwork: s2", "M2: The board: s1", UnassignedName + ": s3,s4"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Groups() = %v, want %v", got, want)
	}
}

func TestMilestoneStatusOf(t *testing.T) {
	tests := []struct {
		name   string
		slices []Slice
		want   MilestoneStatus
	}{
		{"no slices at all", nil, MilestoneQueued},
		{"every slice Todo", []Slice{{Status: SliceTodo}, {Status: SliceTodo}}, MilestoneQueued},
		{"one slice in progress", []Slice{{Status: SliceTodo}, {Status: SliceClaimed}}, MilestoneActive},
		{"some but not all Done", []Slice{{Status: SliceTodo}, {Status: SliceDone}}, MilestoneActive},
		{"the last slice in flight", []Slice{{Status: SliceDone}, {Status: SliceClaimed}}, MilestoneActive},
		{"every slice Done", []Slice{{Status: SliceDone}, {Status: SliceDone}}, MilestoneDone},
		{"one slice, Done", []Slice{{Status: SliceDone}}, MilestoneDone},
		{"a status this build does not know", []Slice{{Status: SliceStatus("Parked")}}, MilestoneQueued},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MilestoneStatusOf(tt.slices); got != tt.want {
				t.Errorf("MilestoneStatusOf() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNewProjectDerivesStatuses covers a select-shaped plan: every milestone's
// status comes off the slices that name it, in the order the options were in.
func TestNewProjectDerivesStatuses(t *testing.T) {
	p := NewProject("p1", "Tracker",
		MilestonesFromOptions([]string{"M1: Groundwork", "M2: The board", "M3: Later", "M4: Empty"}),
		[]Slice{
			{ID: "s1", MilestoneID: "M1: Groundwork", Status: SliceDone},
			{ID: "s2", MilestoneID: "M2: The board", Status: SliceClaimed},
			{ID: "s3", MilestoneID: "M2: The board", Status: SliceTodo},
			{ID: "s4", MilestoneID: "M3: Later", Status: SliceTodo},
			{ID: "s5", MilestoneID: "M9: Retired", Status: SliceDone},
		})

	if p.ID != "p1" || p.Name != "Tracker" || len(p.Slices) != 5 {
		t.Fatalf("NewProject() = %+v, want the plan it was given", p)
	}
	want := []Milestone{
		{ID: "M1: Groundwork", Name: "M1: Groundwork", Order: 0, Status: MilestoneDone, Derived: true},
		{ID: "M2: The board", Name: "M2: The board", Order: 1, Status: MilestoneActive, Derived: true},
		{ID: "M3: Later", Name: "M3: Later", Order: 2, Status: MilestoneQueued, Derived: true},
		{ID: "M4: Empty", Name: "M4: Empty", Order: 3, Status: MilestoneQueued, Derived: true},
	}
	if !reflect.DeepEqual(p.Milestones, want) {
		t.Errorf("NewProject().Milestones = %+v, want %+v", p.Milestones, want)
	}
}

// TestNewProjectKeepsPagedMilestones is the other shape: a milestone with a page
// keeps the Order and Status read off it, whatever its slices are doing, and the
// milestones it was given are not modified in place.
func TestNewProjectKeepsPagedMilestones(t *testing.T) {
	given := []Milestone{
		{ID: "m1", Name: "M1: Groundwork", Order: 7, Status: MilestoneQueued, StatusType: "select"},
	}
	p := NewProject("p1", "Tracker", given, []Slice{{ID: "s1", MilestoneID: "m1", Status: SliceDone}})

	want := []Milestone{{ID: "m1", Name: "M1: Groundwork", Order: 7, Status: MilestoneQueued, StatusType: "select"}}
	if !reflect.DeepEqual(p.Milestones, want) {
		t.Errorf("NewProject().Milestones = %+v, want %+v", p.Milestones, want)
	}
	if !reflect.DeepEqual(given, want) {
		t.Errorf("NewProject() modified its argument: %+v", given)
	}
}

func TestSlicesFromPages(t *testing.T) {
	pages := []notion.Page{page(t, `{"id":"s1"}`), page(t, `{"id":"s2"}`)}
	want := []Slice{{ID: "s1"}, {ID: "s2"}}
	if got := SlicesFromPages(pages); !reflect.DeepEqual(got, want) {
		t.Errorf("SlicesFromPages() = %+v, want %+v", got, want)
	}
	if got := SlicesFromPages(nil); len(got) != 0 {
		t.Errorf("SlicesFromPages(nil) = %+v, want empty", got)
	}
}

func TestProgressOf(t *testing.T) {
	tests := []struct {
		name     string
		slices   []Slice
		want     Progress
		fraction float64
		empty    bool
	}{
		{"no slices", nil, Progress{}, 0, true},
		{
			"mixed statuses",
			[]Slice{
				{Status: SliceTodo}, {Status: SliceTodo},
				{Status: SliceClaimed},
				{Status: SliceDone},
			},
			Progress{Todo: 2, Claimed: 1, Done: 1, Total: 4},
			0.25,
			false,
		},
		{
			"all done",
			[]Slice{{Status: SliceDone}, {Status: SliceDone}},
			Progress{Done: 2, Total: 2},
			1,
			false,
		},
		{
			"unknown status counts towards the total only",
			[]Slice{{Status: SliceDone}, {Status: "Blocked"}},
			Progress{Done: 1, Total: 2},
			0.5,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProgressOf(tt.slices)
			if got != tt.want {
				t.Errorf("ProgressOf() = %+v, want %+v", got, tt.want)
			}
			if got.Fraction() != tt.fraction {
				t.Errorf("Fraction() = %v, want %v", got.Fraction(), tt.fraction)
			}
			if got.Empty() != tt.empty {
				t.Errorf("Empty() = %v, want %v", got.Empty(), tt.empty)
			}
		})
	}
}

func TestProjectProgress(t *testing.T) {
	p := Project{Slices: []Slice{{Status: SliceDone}, {Status: SliceTodo}}}
	want := Progress{Todo: 1, Done: 1, Total: 2}
	if got := p.Progress(); got != want {
		t.Errorf("Progress() = %+v, want %+v", got, want)
	}
	if got := (Project{}).Progress(); got != (Progress{}) {
		t.Errorf("Progress() of an empty project = %+v, want zero", got)
	}
}

func TestGroupName(t *testing.T) {
	m := Milestone{Name: "M4"}
	if got := (Group{Milestone: &m}).Name(); got != "M4" {
		t.Errorf("Name() = %q, want %q", got, "M4")
	}
	if got := (Group{}).Name(); got != UnassignedName {
		t.Errorf("Name() = %q, want %q", got, UnassignedName)
	}
}

func TestGroupProgress(t *testing.T) {
	g := Group{Slices: []Slice{{Status: SliceClaimed}}}
	want := Progress{Claimed: 1, Total: 1}
	if got := g.Progress(); got != want {
		t.Errorf("Progress() = %+v, want %+v", got, want)
	}
}

// groupShape is what the Groups tests assert on: names are enough to pin down
// ordering and bucketing without restating every field.
type groupShape struct {
	Name   string
	Slices []string
}

func shapes(groups []Group) []groupShape {
	out := make([]groupShape, len(groups))
	for i, g := range groups {
		out[i] = groupShape{Name: g.Name()}
		for _, s := range g.Slices {
			out[i].Slices = append(out[i].Slices, s.Name)
		}
	}
	return out
}

func TestProjectGroups(t *testing.T) {
	tests := []struct {
		name    string
		project Project
		want    []groupShape
	}{
		{"zero-slice, zero-milestone project", Project{}, []groupShape{}},
		{
			"milestones are ordered by Order ascending",
			Project{
				Milestones: []Milestone{
					{ID: "m2", Name: "Second", Order: 2},
					{ID: "m1", Name: "First", Order: 1},
				},
				Slices: []Slice{
					{Name: "b", MilestoneID: "m2"},
					{Name: "a", MilestoneID: "m1"},
				},
			},
			[]groupShape{
				{Name: "First", Slices: []string{"a"}},
				{Name: "Second", Slices: []string{"b"}},
			},
		},
		{
			"equal orders break ties by name",
			Project{Milestones: []Milestone{
				{ID: "m2", Name: "Beta", Order: 1},
				{ID: "m1", Name: "Alpha", Order: 1},
			}},
			[]groupShape{{Name: "Alpha"}, {Name: "Beta"}},
		},
		{
			"empty milestones still get a group",
			Project{Milestones: []Milestone{{ID: "m1", Name: "Empty", Order: 1}}},
			[]groupShape{{Name: "Empty"}},
		},
		{
			"slices keep their query order within a milestone",
			Project{
				Milestones: []Milestone{{ID: "m1", Name: "Only", Order: 1}},
				Slices: []Slice{
					{Name: "first", MilestoneID: "m1"},
					{Name: "second", MilestoneID: "m1"},
					{Name: "third", MilestoneID: "m1"},
				},
			},
			[]groupShape{{Name: "Only", Slices: []string{"first", "second", "third"}}},
		},
		{
			"slices with no milestone land in a trailing Unassigned group",
			Project{
				Milestones: []Milestone{{ID: "m1", Name: "Only", Order: 1}},
				Slices: []Slice{
					{Name: "loose"},
					{Name: "placed", MilestoneID: "m1"},
				},
			},
			[]groupShape{
				{Name: "Only", Slices: []string{"placed"}},
				{Name: UnassignedName, Slices: []string{"loose"}},
			},
		},
		{
			"a milestone outside the project is treated as unassigned",
			Project{
				Milestones: []Milestone{{ID: "m1", Name: "Only", Order: 1}},
				Slices:     []Slice{{Name: "orphan", MilestoneID: "gone"}},
			},
			[]groupShape{
				{Name: "Only"},
				{Name: UnassignedName, Slices: []string{"orphan"}},
			},
		},
		{
			"a milestone with no ID does not swallow unassigned slices",
			Project{
				Milestones: []Milestone{{Name: "Nameless", Order: 1}},
				Slices:     []Slice{{Name: "loose"}},
			},
			[]groupShape{
				{Name: "Nameless"},
				{Name: UnassignedName, Slices: []string{"loose"}},
			},
		},
		{
			"slices with no milestones at all",
			Project{Slices: []Slice{{Name: "loose"}}},
			[]groupShape{{Name: UnassignedName, Slices: []string{"loose"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shapes(tt.project.Groups())
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Groups() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// The board reads each group's milestone to colour it by status, so Groups must
// hand back the milestone itself, not a copy of a loop variable.
func TestProjectGroupsCarryTheirMilestone(t *testing.T) {
	p := Project{Milestones: []Milestone{
		{ID: "m1", Name: "First", Order: 1, Status: MilestoneDone},
		{ID: "m2", Name: "Second", Order: 2, Status: MilestoneActive},
	}}
	groups := p.Groups()
	if len(groups) != 2 {
		t.Fatalf("Groups() returned %d groups, want 2", len(groups))
	}
	for i, want := range p.Milestones {
		if got := groups[i].Milestone; got == nil || *got != want {
			t.Errorf("groups[%d].Milestone = %+v, want %+v", i, got, want)
		}
	}
}

// Groups sorts a copy: the project it was called on keeps the order the query
// returned, so a caller holding onto it is not surprised.
func TestProjectGroupsDoesNotReorderTheProject(t *testing.T) {
	p := Project{Milestones: []Milestone{
		{ID: "m2", Name: "Second", Order: 2},
		{ID: "m1", Name: "First", Order: 1},
	}}
	p.Groups()
	if p.Milestones[0].ID != "m2" || p.Milestones[1].ID != "m1" {
		t.Errorf("Groups() reordered the project's milestones: %+v", p.Milestones)
	}
}

// ids names the slices in order, for an assertion that reads like the board.
func ids(slices []Slice) string {
	names := make([]string, len(slices))
	for i, s := range slices {
		names[i] = s.ID
	}
	return strings.Join(names, ",")
}

func TestInViewOrder(t *testing.T) {
	slices := []Slice{{ID: "s1"}, {ID: "s2"}, {ID: "s3"}}

	t.Run("puts the slices in the order given", func(t *testing.T) {
		got := InViewOrder(slices, []string{"s3", "s1", "s2"})
		if ids(got) != "s3,s1,s2" {
			t.Errorf("order = %s, want s3,s1,s2", ids(got))
		}
	})

	t.Run("keeps the slices it was given where there is no order", func(t *testing.T) {
		if got := InViewOrder(slices, nil); ids(got) != "s1,s2,s3" {
			t.Errorf("order = %s, want the input order", ids(got))
		}
	})

	t.Run("trails the slices the order does not name, in the order read", func(t *testing.T) {
		got := InViewOrder(slices, []string{"s3"})
		if ids(got) != "s3,s1,s2" {
			t.Errorf("order = %s, want the named slice first", ids(got))
		}
	})

	t.Run("ignores an ID the order names but the plan does not hold", func(t *testing.T) {
		got := InViewOrder(slices, []string{"gone", "s2"})
		if ids(got) != "s2,s1,s3" {
			t.Errorf("order = %s, want s2 first", ids(got))
		}
	})

	t.Run("matches IDs however they are written", func(t *testing.T) {
		dashed := []Slice{{ID: "3bd38308-f654-812e-abde-e5b4160e2717"}, {ID: "s2"}}
		got := InViewOrder(dashed, []string{"s2", "3BD38308F654812EABDEE5B4160E2717"})
		if got[0].ID != "s2" || got[1].ID != dashed[0].ID {
			t.Errorf("order = %s, want the undashed ID to have matched", ids(got))
		}
	})

	t.Run("does not reorder what it was given", func(t *testing.T) {
		in := []Slice{{ID: "s1"}, {ID: "s2"}}
		InViewOrder(in, []string{"s2", "s1"})
		if ids(in) != "s1,s2" {
			t.Errorf("the input was reordered: %s", ids(in))
		}
	})
}
