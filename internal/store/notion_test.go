package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// blockText reads the one span of plain text out of a block this package
// wrote, so a test can say what was appended rather than how it was encoded.
func blockText(t *testing.T, block map[string]any) (kind, text string) {
	t.Helper()
	kind, _ = block["type"].(string)
	body, ok := block[kind].(map[string]any)
	if !ok {
		t.Fatalf("block %+v has no body", block)
	}
	spans, ok := body["rich_text"].([]map[string]any)
	if !ok || len(spans) != 1 {
		t.Fatalf("block %+v has no single span", block)
	}
	content, ok := spans[0]["text"].(map[string]any)
	if !ok {
		t.Fatalf("span %+v has no text", spans[0])
	}
	text, _ = content["content"].(string)
	return kind, text
}

// texts is every block of an append, as kind-and-text pairs.
func texts(t *testing.T, blocks []map[string]any) [][2]string {
	t.Helper()
	out := make([][2]string, len(blocks))
	for i, b := range blocks {
		kind, text := blockText(t, b)
		out[i] = [2]string{kind, text}
	}
	return out
}

func TestClaimSliceTakesTheSliceForTheUser(t *testing.T) {
	api := &fakeAPI{updatePage: func(id string, _ map[string]notion.PropertyValue) (*notion.Page, error) {
		return slicePage(id, "Info view", notion.SliceInProgress, notion.User{ID: "u1", Name: "Craig"}), nil
	}}
	s, err := Over(api).ClaimSlice(context.Background(), "s5", Shape{HasAssignee: true}, "u1")
	if err != nil {
		t.Fatalf("ClaimSlice() error = %v", err)
	}
	if s.Status != domain.SliceClaimed || !reflect.DeepEqual(s.AssigneeIDs, []string{"u1"}) {
		t.Errorf("slice = %+v, want it claimed by u1", s)
	}
	props := api.updates[0]
	if props[notion.PropStatus].SelectName() != notion.SliceInProgress {
		t.Errorf("status = %+v, want In progress", props[notion.PropStatus])
	}
	if ids := props[notion.PropAssignee].PeopleIDs(); !reflect.DeepEqual(ids, []string{"u1"}) {
		t.Errorf("assignee = %v, want u1", ids)
	}
}

// A project with no Assignee column, and a claim with nobody to name, both
// write the status alone: there is nowhere to record ownership, or nobody to
// record.
func TestClaimSliceWritesNoAssigneeWhenThereIsNoneToWrite(t *testing.T) {
	tests := []struct {
		name   string
		shape  Shape
		userID string
	}{
		{"no column", Shape{}, "u1"},
		{"no user", Shape{HasAssignee: true}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{}
			if _, err := Over(api).ClaimSlice(context.Background(), "s5", tt.shape, tt.userID); err != nil {
				t.Fatalf("ClaimSlice() error = %v", err)
			}
			if _, wrote := api.updates[0][notion.PropAssignee]; wrote {
				t.Errorf("properties = %+v, want the status alone", api.updates[0])
			}
		})
	}
}

func TestClaimSliceCarriesTheWritesFailureUp(t *testing.T) {
	api := &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errBoom
	}}
	if _, err := Over(api).ClaimSlice(context.Background(), "s5", Shape{}, "u1"); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the write's failure", err)
	}
}

func TestReleaseSliceWritesTheLineThenTheStatus(t *testing.T) {
	api := &fakeAPI{updatePage: func(id string, _ map[string]notion.PropertyValue) (*notion.Page, error) {
		return slicePage(id, "Release action", notion.SliceTodo), nil
	}}
	s, err := Over(api).ReleaseSlice(context.Background(), "s5", Shape{HasAssignee: true}, "Craig Johnston")
	if err != nil {
		t.Fatalf("ReleaseSlice() error = %v", err)
	}
	if s.Status != domain.SliceTodo {
		t.Errorf("slice = %+v, want it back at Todo", s)
	}
	if got := api.calls; !reflect.DeepEqual(got, []string{"AppendBlockChildren", "UpdatePageProperties"}) {
		t.Errorf("calls = %v, want the line written before the status", got)
	}
	if _, text := blockText(t, api.appended[0][0]); !strings.Contains(text, "Craig Johnston") {
		t.Errorf("line = %q, want it to name who let the slice go", text)
	}
	if ids := api.updates[0][notion.PropAssignee].PeopleIDs(); len(ids) != 0 {
		t.Errorf("assignee = %v, want it cleared", ids)
	}
}

// A project with no Assignee column has none to clear.
func TestReleaseSliceClearsNothingWhereThereIsNoAssignee(t *testing.T) {
	api := &fakeAPI{}
	if _, err := Over(api).ReleaseSlice(context.Background(), "s5", Shape{}, "Craig"); err != nil {
		t.Fatalf("ReleaseSlice() error = %v", err)
	}
	if _, wrote := api.updates[0][notion.PropAssignee]; wrote {
		t.Errorf("properties = %+v, want the status alone", api.updates[0])
	}
}

func TestReleaseSliceFailures(t *testing.T) {
	tests := []struct {
		name string
		api  *fakeAPI
		want string
	}{
		{"the line", &fakeAPI{appendBlocks: func(string, []map[string]any) ([]notion.Block, error) {
			return nil, errBoom
		}}, "note the release"},
		{"the status", &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
			return nil, errBoom
		}}, "release the slice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Over(tt.api).ReleaseSlice(context.Background(), "s5", Shape{}, "Craig")
			if !errors.Is(err, errBoom) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q and the failure", err, tt.want)
			}
		})
	}
}

// The four endings write four different things, and the note is filed under a
// heading naming which of them it was.
func TestCompleteSliceEndings(t *testing.T) {
	tests := []struct {
		name    string
		outcome Outcome
		heading string
		done    bool
	}{
		{"done", Outcome{Summary: "Wrote it."}, summaryHeading, true},
		{"handed back", Outcome{Summary: "Pushed it.", Branch: "slice/x"}, handedBackHeading, false},
		{"pull request recorded", Outcome{Summary: "Opened it.", PR: "https://gh.test/1"}, summaryHeading, false},
		{"blocked", Outcome{Summary: "Stuck.", Blocked: true}, blockedHeading, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{}
			if _, err := Over(api).CompleteSlice(context.Background(), "s5", Shape{}, tt.outcome); err != nil {
				t.Fatalf("CompleteSlice() error = %v", err)
			}
			if got := texts(t, api.appended[0]); got[0] != [2]string{"heading_3", tt.heading} {
				t.Errorf("note = %v, want it filed under %q", got, tt.heading)
			}
			var props map[string]notion.PropertyValue
			if len(api.updates) > 0 {
				props = api.updates[0]
			}
			if _, wrote := props[notion.PropStatus]; wrote != tt.done {
				t.Errorf("status written = %v, want %v", wrote, tt.done)
			}
			if want := tt.outcome.Branch; want != "" {
				if got := props[notion.PropBranch]; len(got.RichText) != 1 || got.RichText[0].Text.Content != want {
					t.Errorf("branch = %+v, want %q", got, want)
				}
			}
			if tt.outcome.PR != "" && props[notion.PropPR].URL != tt.outcome.PR {
				t.Errorf("pr = %+v, want %q", props[notion.PropPR], tt.outcome.PR)
			}
		})
	}
}

// The pull request description goes on in the same write as the summary, under
// a heading of its own, because the two are one hand-back.
func TestCompleteSliceFilesThePullRequestDescriptionBesideTheSummary(t *testing.T) {
	api := &fakeAPI{}
	_, err := Over(api).CompleteSlice(context.Background(), "s5", Shape{}, Outcome{
		Summary: "Pushed it.", Branch: "slice/x", PRDescription: "Add the store\n\nWhy it matters.",
	})
	if err != nil {
		t.Fatalf("CompleteSlice() error = %v", err)
	}
	want := [][2]string{
		{"heading_3", handedBackHeading},
		{"paragraph", "Pushed it."},
		{"heading_3", notion.PRDescriptionHeading},
		{"paragraph", "Add the store"},
		{"paragraph", "Why it matters."},
	}
	if got := texts(t, api.appended[0]); !reflect.DeepEqual(got, want) {
		t.Errorf("blocks = %v, want %v", got, want)
	}
}

// An ending with nothing to write on the properties still has its note filed,
// and the slice comes back as it stands rather than as a zero value.
func TestCompleteSliceReadsTheSliceBackWhenThereIsNoPropertyToWrite(t *testing.T) {
	api := &fakeAPI{page: func(id string) (*notion.Page, error) {
		return slicePage(id, "Stuck", notion.SliceInProgress), nil
	}}
	s, err := Over(api).CompleteSlice(context.Background(), "s5", Shape{},
		Outcome{Summary: "Stuck.", Blocked: true})
	if err != nil {
		t.Fatalf("CompleteSlice() error = %v", err)
	}
	if s.Name != "Stuck" {
		t.Errorf("slice = %+v, want it read back", s)
	}
	if got := api.calls; !reflect.DeepEqual(got, []string{"AppendBlockChildren", "GetPage"}) {
		t.Errorf("calls = %v, want the note and then the read", got)
	}
}

func TestCompleteSliceFailures(t *testing.T) {
	tests := []struct {
		name    string
		api     *fakeAPI
		outcome Outcome
		want    string
	}{
		{"the note", &fakeAPI{appendBlocks: func(string, []map[string]any) ([]notion.Block, error) {
			return nil, errBoom
		}}, Outcome{Summary: "Wrote it."}, "append the note"},
		{"the properties", &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
			return nil, errBoom
		}}, Outcome{Summary: "Wrote it."}, "close out the slice"},
		{"the read back", &fakeAPI{page: func(string) (*notion.Page, error) {
			return nil, errBoom
		}}, Outcome{Summary: "Stuck.", Blocked: true}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Over(tt.api).CompleteSlice(context.Background(), "s5", Shape{}, tt.outcome)
			if !errors.Is(err, errBoom) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q and the failure", err, tt.want)
			}
		})
	}
}

func TestRecordPRWritesTheURLAndNothingElse(t *testing.T) {
	api := &fakeAPI{}
	if err := Over(api).RecordPR(context.Background(), "s5", "https://gh.test/9"); err != nil {
		t.Fatalf("RecordPR() error = %v", err)
	}
	want := map[string]notion.PropertyValue{notion.PropPR: notion.NewURL("https://gh.test/9")}
	if !reflect.DeepEqual(api.updates[0], want) {
		t.Errorf("properties = %+v, want %+v", api.updates[0], want)
	}
}

func TestRecordPRCarriesTheWritesFailureUp(t *testing.T) {
	api := &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errBoom
	}}
	if err := Over(api).RecordPR(context.Background(), "s5", "https://gh.test/9"); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the write's failure", err)
	}
}

func TestMarkDoneWritesTheStatus(t *testing.T) {
	api := &fakeAPI{}
	if err := Over(api).MarkDone(context.Background(), "s5", Shape{}); err != nil {
		t.Fatalf("MarkDone() error = %v", err)
	}
	if got := api.updates[0][notion.PropStatus].SelectName(); got != notion.SliceDone {
		t.Errorf("status = %q, want Done", got)
	}
}

func TestMarkDoneCarriesTheWritesFailureUp(t *testing.T) {
	api := &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errBoom
	}}
	if err := Over(api).MarkDone(context.Background(), "s5", Shape{}); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the write's failure", err)
	}
}

// Adding milestones appends options to the Milestone column, after the ones
// already there, in one write.
func TestAddMilestonesAppendsToThePlan(t *testing.T) {
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) {
		return settledSchema(true, "M1"), nil
	}}
	st := Over(api)
	sh, err := st.Shape(context.Background(), project())
	if err != nil {
		t.Fatalf("Shape() error = %v", err)
	}
	added, err := st.AddMilestones(context.Background(), project(), sh, []string{"M2", "M3"})
	if err != nil {
		t.Fatalf("AddMilestones() error = %v", err)
	}
	want := []domain.Milestone{
		{ID: "M2", Name: "M2", Order: 1, Status: domain.MilestoneQueued, SelectType: notion.TypeSelect},
		{ID: "M3", Name: "M3", Order: 2, Status: domain.MilestoneQueued, SelectType: notion.TypeSelect},
	}
	if !reflect.DeepEqual(added, want) {
		t.Errorf("added = %+v, want %+v", added, want)
	}
	options := api.schemas[0][notion.PropMilestone].OptionNames()
	if !reflect.DeepEqual(options, []string{"M1", "M2", "M3"}) {
		t.Errorf("options = %v, want the plan with the new ones after it", options)
	}
}

// A run adding no milestone writes nothing: replacing the option list with a
// copy of itself is a real edit for the sake of nothing.
func TestAddMilestonesWritesNothingWhenThereIsNothingToAdd(t *testing.T) {
	api := &fakeAPI{}
	added, err := Over(api).AddMilestones(context.Background(), project(), Shape{}, nil)
	if err != nil || added != nil {
		t.Fatalf("AddMilestones() = %v, %v, want nothing at all", added, err)
	}
	if len(api.calls) != 0 {
		t.Errorf("calls = %v, want none", api.calls)
	}
}

func TestAddMilestonesRefusals(t *testing.T) {
	selectShape := func(t *testing.T, api *fakeAPI) Shape {
		t.Helper()
		sh, err := Over(api).Shape(context.Background(), project())
		if err != nil {
			t.Fatalf("Shape() error = %v", err)
		}
		return sh
	}
	t.Run("a name the plan already holds", func(t *testing.T) {
		api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) {
			return settledSchema(true, "M1"), nil
		}}
		sh := selectShape(t, api)
		_, err := Over(api).AddMilestones(context.Background(), project(), sh, []string{"m1"})
		if err == nil || !strings.Contains(err.Error(), "already has a milestone") {
			t.Errorf("err = %v, want the duplicate refused", err)
		}
	})
	t.Run("two of a name in one run", func(t *testing.T) {
		api := &fakeAPI{}
		sh := selectShape(t, api)
		_, err := Over(api).AddMilestones(context.Background(), project(), sh, []string{"M2", "M2"})
		if err == nil || !strings.Contains(err.Error(), "already has a milestone") {
			t.Errorf("err = %v, want the duplicate refused", err)
		}
	})
	t.Run("a column that is not a select", func(t *testing.T) {
		api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) {
			ds := settledSchema(true)
			ds.Properties[notion.PropMilestone] = notion.PropertySchema{
				Name: notion.PropMilestone, Type: notion.TypeStatus,
				Status: &notion.OptionsConfig{},
			}
			return ds, nil
		}}
		sh := selectShape(t, api)
		_, err := Over(api).AddMilestones(context.Background(), project(), sh, []string{"M2"})
		if err == nil || !strings.Contains(err.Error(), "can only be added to it in Notion") {
			t.Errorf("err = %v, want the column refused", err)
		}
	})
	t.Run("a schema write that failed", func(t *testing.T) {
		api := &fakeAPI{updateSchema: func(string, map[string]notion.PropertySchema) (*notion.DataSource, error) {
			return nil, errBoom
		}}
		sh := selectShape(t, api)
		_, err := Over(api).AddMilestones(context.Background(), project(), sh, []string{"M2", "M3"})
		if !errors.Is(err, errBoom) || !strings.Contains(err.Error(), "create the milestones") {
			t.Errorf("err = %v, want the plural failure", err)
		}
	})
	t.Run("one milestone is named in the singular", func(t *testing.T) {
		api := &fakeAPI{updateSchema: func(string, map[string]notion.PropertySchema) (*notion.DataSource, error) {
			return nil, errBoom
		}}
		sh := selectShape(t, api)
		_, err := Over(api).AddMilestones(context.Background(), project(), sh, []string{"M2"})
		if !strings.Contains(err.Error(), "create the milestone:") {
			t.Errorf("err = %v, want the singular", err)
		}
	})
}

func TestAddSliceFilesItTodoAndUnclaimed(t *testing.T) {
	api := &fakeAPI{createPage: func(_ notion.Parent, _ map[string]notion.PropertyValue, _ []map[string]any) (*notion.Page, error) {
		return slicePage("s9", "Put a store in", notion.SliceTodo), nil
	}}
	m := domain.Milestone{ID: "M1", Name: "M1", SelectType: notion.TypeSelect}
	s, err := Over(api).AddSlice(context.Background(), project(), NewSlice{
		Title: "Put a store in", Brief: "Do it.\n\nCarefully.", Repo: "/repo", Milestone: m,
		DependsOn: []string{"s1"},
	})
	if err != nil {
		t.Fatalf("AddSlice() error = %v", err)
	}
	if s.ID != "s9" {
		t.Errorf("slice = %+v, want the page created", s)
	}
	props := api.created[0]["properties"].(map[string]notion.PropertyValue)
	if props[notion.PropStatus].SelectName() != notion.SliceTodo {
		t.Errorf("status = %+v, want Todo", props[notion.PropStatus])
	}
	if props[notion.PropMilestone].SelectName() != "M1" {
		t.Errorf("milestone = %+v, want M1", props[notion.PropMilestone])
	}
	if ids := props[notion.PropDependsOn].RelationIDs(); !reflect.DeepEqual(ids, []string{"s1"}) {
		t.Errorf("depends on = %v, want s1", ids)
	}
	want := [][2]string{{"paragraph", "Do it."}, {"paragraph", "Carefully."}}
	if got := texts(t, api.created[0]["children"].([]map[string]any)); !reflect.DeepEqual(got, want) {
		t.Errorf("body = %v, want %v", got, want)
	}
}

// A slice waiting on nothing is written without the relation at all: a project
// whose table has no dependency column can still have slices added to it.
func TestAddSliceLeavesTheRelationOffWhenThereIsNothingToWaitOn(t *testing.T) {
	api := &fakeAPI{}
	if _, err := Over(api).AddSlice(context.Background(), project(), NewSlice{Title: "Solo"}); err != nil {
		t.Fatalf("AddSlice() error = %v", err)
	}
	props := api.created[0]["properties"].(map[string]notion.PropertyValue)
	if _, wrote := props[notion.PropDependsOn]; wrote {
		t.Errorf("properties = %+v, want no dependency relation", props)
	}
}

func TestAddSliceCarriesTheWritesFailureUp(t *testing.T) {
	api := &fakeAPI{createPage: func(notion.Parent, map[string]notion.PropertyValue, []map[string]any) (*notion.Page, error) {
		return nil, errBoom
	}}
	_, err := Over(api).AddSlice(context.Background(), project(), NewSlice{Title: "Solo"})
	if !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the write's failure", err)
	}
}

func TestEditSliceRewritesThePropertiesThenTheBody(t *testing.T) {
	api := &fakeAPI{blocks: func(string) ([]notion.Block, error) {
		return []notion.Block{{ID: "b1"}, {ID: "b2"}}, nil
	}}
	if err := Over(api).EditSlice(context.Background(), "s5", "New title", "/repo", "New brief."); err != nil {
		t.Fatalf("EditSlice() error = %v", err)
	}
	want := []string{"UpdatePageProperties", "GetBlockChildren", "DeleteBlock", "DeleteBlock", "AppendBlockChildren"}
	if !reflect.DeepEqual(api.calls, want) {
		t.Errorf("calls = %v, want %v", api.calls, want)
	}
	if got := api.updates[0][notion.PropName]; len(got.Title) != 1 || got.Title[0].Text.Content != "New title" {
		t.Errorf("title = %+v, want the new one", got)
	}
	if !reflect.DeepEqual(api.deleted, []string{"b1", "b2"}) {
		t.Errorf("deleted = %v, want the old blocks", api.deleted)
	}
}

// A brief cleared to nothing leaves the page empty rather than appending an
// empty block.
func TestSetSliceBriefAppendsNothingForAnEmptyBrief(t *testing.T) {
	api := &fakeAPI{}
	if err := Over(api).SetSliceBrief(context.Background(), "s5", "   "); err != nil {
		t.Fatalf("SetSliceBrief() error = %v", err)
	}
	if len(api.appended) != 0 {
		t.Errorf("appended = %v, want nothing", api.appended)
	}
}

func TestEditSliceFailures(t *testing.T) {
	blocks := func(string) ([]notion.Block, error) { return []notion.Block{{ID: "b1"}}, nil }
	tests := []struct {
		name string
		api  *fakeAPI
		want string
	}{
		{"the properties", &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
			return nil, errBoom
		}}, "update the slice"},
		{"the read", &fakeAPI{blocks: func(string) ([]notion.Block, error) { return nil, errBoom }}, "read slice body"},
		{"the clear", &fakeAPI{blocks: blocks, deleteBlock: func(string) error { return errBoom }}, "clear slice body"},
		{"the write", &fakeAPI{appendBlocks: func(string, []map[string]any) ([]notion.Block, error) {
			return nil, errBoom
		}}, "write slice body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Over(tt.api).EditSlice(context.Background(), "s5", "T", "/repo", "Brief.")
			if !errors.Is(err, errBoom) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q and the failure", err, tt.want)
			}
		})
	}
}

func TestSetDependenciesReplacesWhatTheSliceWaitedOn(t *testing.T) {
	api := &fakeAPI{updatePage: func(id string, _ map[string]notion.PropertyValue) (*notion.Page, error) {
		p := slicePage(id, "Waiting", notion.SliceTodo)
		p.Properties[notion.PropDependsOn] = notion.NewRelation("s1", "s2")
		return p, nil
	}}
	s, err := Over(api).SetDependencies(context.Background(), "s5", []string{"s1", "s2"})
	if err != nil {
		t.Fatalf("SetDependencies() error = %v", err)
	}
	if !reflect.DeepEqual(s.DependsOn, []string{"s1", "s2"}) {
		t.Errorf("depends on = %v, want both", s.DependsOn)
	}
	if ids := api.updates[0][notion.PropDependsOn].RelationIDs(); !reflect.DeepEqual(ids, []string{"s1", "s2"}) {
		t.Errorf("written = %v, want both", ids)
	}
}

func TestSetDependenciesCarriesTheWritesFailureUp(t *testing.T) {
	api := &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errBoom
	}}
	if _, err := Over(api).SetDependencies(context.Background(), "s5", nil); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the write's failure", err)
	}
}

func TestMoveSliceWritesTheMilestoneAndNothingElse(t *testing.T) {
	api := &fakeAPI{}
	m := domain.Milestone{ID: "M2", Name: "M2", SelectType: notion.TypeSelect}
	if err := Over(api).MoveSlice(context.Background(), "s5", m); err != nil {
		t.Fatalf("MoveSlice() error = %v", err)
	}
	want := map[string]notion.PropertyValue{notion.PropMilestone: m.Ref()}
	if !reflect.DeepEqual(api.updates[0], want) {
		t.Errorf("properties = %+v, want %+v", api.updates[0], want)
	}
}

func TestMoveSliceCarriesTheWritesFailureUp(t *testing.T) {
	api := &fakeAPI{updatePage: func(string, map[string]notion.PropertyValue) (*notion.Page, error) {
		return nil, errBoom
	}}
	if err := Over(api).MoveSlice(context.Background(), "s5", domain.Milestone{}); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the write's failure", err)
	}
}

func TestDeleteSliceTrashesThePage(t *testing.T) {
	api := &fakeAPI{}
	if err := Over(api).DeleteSlice(context.Background(), "s5"); err != nil {
		t.Fatalf("DeleteSlice() error = %v", err)
	}
	if !reflect.DeepEqual(api.calls, []string{"TrashPage"}) {
		t.Errorf("calls = %v, want the one trash", api.calls)
	}
}

func TestDeleteSliceCarriesTheWritesFailureUp(t *testing.T) {
	api := &fakeAPI{trash: func(string) error { return errBoom }}
	if err := Over(api).DeleteSlice(context.Background(), "s5"); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the write's failure", err)
	}
}
