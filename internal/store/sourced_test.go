package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/source"
)

// sourcedPlugin is the project as the fake plugin is told of it.
var sourcedPlugin = source.Project{ID: "proj", Name: "Work", WorkingDir: "/w"}

// newSourced is a Sourced over a real plan file of the test's own and a fake
// plugin that records what it is told.
func newSourced(t *testing.T) (*Sourced, *Local, *source.Fake) {
	t.Helper()
	l, _ := openPlan(t)
	f := &source.Fake{}
	return NewSourced(l, f, sourcedPlugin, "shortcut"), l, f
}

// card is the container every test files its tasks under.
var card = domain.Milestone{ID: "sc-1", Name: "Fix the login page"}

// addTask files one task under the card and forgets the event it fired, so a
// test's assertions are about its own write alone.
func addTask(t *testing.T, s *Sourced, f *source.Fake, title string) domain.Slice {
	t.Helper()
	sl, err := s.AddSlice(context.Background(), Project{ID: "proj"}, NewSlice{Title: title, Milestone: card})
	if err != nil {
		t.Fatalf("AddSlice: %v", err)
	}
	f.Events = nil
	return sl
}

// onlyEvent asserts the plugin was told exactly one thing, and returns it.
func onlyEvent(t *testing.T, f *source.Fake) source.EventCall {
	t.Helper()
	if len(f.Events) != 1 {
		t.Fatalf("events = %+v, want exactly one", f.Events)
	}
	return f.Events[0]
}

func TestSourcedAddSliceFilesTheContainerAndTellsThePlugin(t *testing.T) {
	ctx := context.Background()
	s, l, f := newSourced(t)
	sl, err := s.AddSlice(ctx, Project{ID: "proj"}, NewSlice{Title: "Task one", Milestone: card})
	if err != nil {
		t.Fatalf("AddSlice: %v", err)
	}
	if sl.MilestoneID != "sc-1" {
		t.Errorf("filed under %q, want the container's id", sl.MilestoneID)
	}
	ms, err := l.milestones(ctx, l.db)
	if err != nil || len(ms) != 1 || ms[0].ID != "sc-1" || ms[0].Name != "Fix the login page" {
		t.Fatalf("milestones = %+v, %v", ms, err)
	}
	ev := onlyEvent(t, f)
	want := source.EventCall{Project: sourcedPlugin, Container: "sc-1", Event: source.EventCreated,
		Task: source.Task{ID: sl.ID, Title: "Task one", Status: "Todo"}}
	if !reflect.DeepEqual(ev, want) {
		t.Errorf("event = %+v, want %+v", ev, want)
	}

	// A second task under the same card files no second milestone.
	if _, err := s.AddSlice(ctx, Project{ID: "proj"}, NewSlice{Title: "Task two", Milestone: card}); err != nil {
		t.Fatalf("AddSlice again: %v", err)
	}
	if ms, _ := l.milestones(ctx, l.db); len(ms) != 1 {
		t.Errorf("milestones = %+v, want the one container", ms)
	}
}

// A new task with no repository of its own starts from the one its card's
// latest task was worked in; one under another card, or given its own, does
// not.
func TestSourcedAddSliceDefaultsTheRepoFromItsCard(t *testing.T) {
	ctx := context.Background()
	s, l, f := newSourced(t)
	first := addTask(t, s, f, "First")
	if first.Repo != "" {
		t.Fatalf("first task's repo = %q, want none to start from", first.Repo)
	}
	if err := s.SetSliceRepo(ctx, first.ID, "/src/app"); err != nil {
		t.Fatal(err)
	}
	if got := addTask(t, s, f, "Second"); got.Repo != "/src/app" {
		t.Errorf("second task's repo = %q, want the card's", got.Repo)
	}
	own, err := s.AddSlice(ctx, Project{ID: "proj"}, NewSlice{Title: "Own", Milestone: card, Repo: "/src/other"})
	if err != nil || own.Repo != "/src/other" {
		t.Errorf("a task given its own repo = %+v, %v", own, err)
	}
	other, err := s.AddSlice(ctx, Project{ID: "proj"}, NewSlice{Title: "Elsewhere", Milestone: domain.Milestone{ID: "sc-2", Name: "Other card"}})
	if err != nil || other.Repo != "" {
		t.Errorf("a task on another card = %+v, %v, want no repo", other, err)
	}
	// A plan that cannot be read concludes nothing: the task is filed with none.
	write(t, l, `DROP TABLE project`)
	if got, err := s.AddSlice(ctx, Project{ID: "proj"}, NewSlice{Title: "Blind", Milestone: card}); err != nil || got.Repo != "" {
		t.Errorf("with the plan unreadable = %+v, %v, want the task filed with no repo", got, err)
	}
}

func TestSourcedAddSliceRefusesATaskUnderNoContainer(t *testing.T) {
	s, _, f := newSourced(t)
	_, err := s.AddSlice(context.Background(), Project{}, NewSlice{Title: "Loose"})
	if err == nil || !strings.Contains(err.Error(), "Work: every task here is filed under one of shortcut's containers") {
		t.Fatalf("err = %v", err)
	}
	if len(f.Events) != 0 {
		t.Errorf("events = %+v, want none", f.Events)
	}
}

func TestSourcedAddSliceTellsNothingWhenTheWriteFails(t *testing.T) {
	ctx := context.Background()
	s, l, f := newSourced(t)
	// A dependency on a slice the plan does not hold fails the local write.
	if _, err := s.AddSlice(ctx, Project{}, NewSlice{Title: "x", Milestone: card, DependsOn: []string{"nope"}}); err == nil {
		t.Fatal("want the write refused")
	}
	if len(f.Events) != 0 {
		t.Errorf("events = %+v, want none", f.Events)
	}

	// And where the container cannot be filed, nothing is written at all.
	write(t, l, `CREATE TRIGGER no_milestones BEFORE INSERT ON milestones BEGIN SELECT RAISE(ABORT, 'refused'); END`)
	if _, err := s.AddSlice(ctx, Project{}, NewSlice{Title: "y", Milestone: domain.Milestone{ID: "sc-2", Name: "Other"}}); err == nil {
		t.Fatal("want the container refused")
	}
	if len(f.Events) != 0 {
		t.Errorf("events = %+v, want none", f.Events)
	}
}

func TestSourcedLifecycleEventsFollowTheirWrites(t *testing.T) {
	ctx := context.Background()
	s, _, f := newSourced(t)
	sl := addTask(t, s, f, "Task")
	sh := Shape{HasAssignee: true, HasBranch: true}

	if _, err := s.ClaimSlice(ctx, sl.ID, sh, "Craig"); err != nil {
		t.Fatal(err)
	}
	if ev := onlyEvent(t, f); ev.Event != source.EventClaimed || ev.Task.Status != "In progress" || ev.Container != "sc-1" {
		t.Errorf("claim event = %+v", ev)
	}
	f.Events = nil

	if _, err := s.ReleaseSlice(ctx, sl.ID, sh, "Craig"); err != nil {
		t.Fatal(err)
	}
	if ev := onlyEvent(t, f); ev.Event != source.EventReleased || ev.Task.Status != "Todo" {
		t.Errorf("release event = %+v", ev)
	}
	f.Events = nil

	if _, err := s.CompleteSlice(ctx, sl.ID, sh, Outcome{Summary: "stuck", Blocked: true}); err != nil {
		t.Fatal(err)
	}
	if len(f.Events) != 0 {
		t.Errorf("a blocked ending told the plugin %+v", f.Events)
	}

	if _, err := s.CompleteSlice(ctx, sl.ID, sh, Outcome{Summary: "done", Branch: "slice/task"}); err != nil {
		t.Fatal(err)
	}
	if ev := onlyEvent(t, f); ev.Event != source.EventHandedBack || ev.Task.Branch != "slice/task" {
		t.Errorf("hand-back event = %+v", ev)
	}
	f.Events = nil

	if err := s.RecordPR(ctx, sl.ID, "https://example.test/pr/1"); err != nil {
		t.Fatal(err)
	}
	if ev := onlyEvent(t, f); ev.Event != source.EventApproved || ev.Task.PR != "https://example.test/pr/1" || ev.Task.Title != "Task" {
		t.Errorf("approve event = %+v", ev)
	}
	f.Events = nil

	if err := s.MarkDone(ctx, sl.ID, sh); err != nil {
		t.Fatal(err)
	}
	if ev := onlyEvent(t, f); ev.Event != source.EventMerged || ev.Task.Status != "Done" {
		t.Errorf("merge event = %+v", ev)
	}
	f.Events = nil

	if err := s.DeleteSlice(ctx, sl.ID); err != nil {
		t.Fatal(err)
	}
	if ev := onlyEvent(t, f); ev.Event != source.EventDeleted || ev.Task.Title != "Task" || ev.Container != "sc-1" {
		t.Errorf("delete event = %+v", ev)
	}
}

func TestSourcedTellsNothingWhenTheWriteFails(t *testing.T) {
	ctx := context.Background()
	s, l, f := newSourced(t)
	sh := Shape{}
	if _, err := s.ClaimSlice(ctx, "nope", sh, "u"); err == nil {
		t.Error("ClaimSlice: want an error")
	}
	if _, err := s.ReleaseSlice(ctx, "nope", sh, "u"); err == nil {
		t.Error("ReleaseSlice: want an error")
	}
	if _, err := s.CompleteSlice(ctx, "nope", sh, Outcome{Summary: "x", Branch: "b"}); err == nil {
		t.Error("CompleteSlice: want an error")
	}
	if err := s.RecordPR(ctx, "nope", "u"); err == nil {
		t.Error("RecordPR: want an error")
	}
	if err := s.MarkDone(ctx, "nope", sh); err == nil {
		t.Error("MarkDone: want an error")
	}
	if err := s.DeleteSlice(ctx, "nope"); err == nil {
		t.Error("DeleteSlice: want an error")
	}
	// A slice that reads but whose delete is refused tells nothing either.
	sl := addTask(t, s, f, "Kept")
	write(t, l, `CREATE TRIGGER no_deletes BEFORE DELETE ON slices BEGIN SELECT RAISE(ABORT, 'refused'); END`)
	if err := s.DeleteSlice(ctx, sl.ID); err == nil {
		t.Error("DeleteSlice: want the refused delete")
	}
	if len(f.Events) != 0 {
		t.Errorf("events = %+v, want none", f.Events)
	}
}

func TestSourcedSwallowsAnEventThePluginRefuses(t *testing.T) {
	ctx := context.Background()
	s, _, f := newSourced(t)
	f.EventErr = errors.New("plugin down")
	sl, err := s.AddSlice(ctx, Project{}, NewSlice{Title: "Task", Milestone: card})
	if err != nil {
		t.Fatalf("AddSlice must succeed when only its event fails: %v", err)
	}
	if _, err := s.ClaimSlice(ctx, sl.ID, Shape{}, "u"); err != nil {
		t.Fatalf("ClaimSlice must succeed when only its event fails: %v", err)
	}
	if len(f.Events) != 2 {
		t.Errorf("events = %+v, want both still sent", f.Events)
	}
}

// RecordPR and MarkDone answer with no slice, so they read it back for the
// event; where that read fails the plugin is still told, by ID.
func TestSourcedSendsTheIDAloneWhereTheReadBackFails(t *testing.T) {
	ctx := context.Background()
	s, l, f := newSourced(t)
	sl := addTask(t, s, f, "Task")
	// The slice's own write never reads the project table; the read back does.
	write(t, l, `DROP TABLE project`)
	if err := s.RecordPR(ctx, sl.ID, "https://example.test/pr/2"); err != nil {
		t.Fatal(err)
	}
	ev := onlyEvent(t, f)
	if ev.Task != (source.Task{ID: sl.ID}) || ev.Event != source.EventApproved {
		t.Errorf("event = %+v, want the ID alone", ev)
	}
}

func TestSourcedRefusesToShapeTheContainers(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newSourced(t)
	want := "Work: milestones here are shortcut's containers — nat does not add, rename, remove or move them"
	_, err := s.AddMilestones(ctx, Project{}, Shape{}, []string{"x"})
	checkErr(t, "AddMilestones", err, want)
	_, err = s.RenameMilestone(ctx, Project{}, Shape{}, "a", "b")
	checkErr(t, "RenameMilestone", err, want)
	_, err = s.RemoveMilestone(ctx, Project{}, Shape{}, "a")
	checkErr(t, "RemoveMilestone", err, want)
	_, _, err = s.MoveMilestone(ctx, Project{}, Shape{}, "a", "b", true)
	checkErr(t, "MoveMilestone", err, want)
	err = s.MoveSlice(ctx, "x", card)
	checkErr(t, "MoveSlice", err, "Work: a task is not moved between shortcut's containers")
}

func checkErr(t *testing.T, op string, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("%s: err = %v, want it to say %q", op, err, want)
	}
}

func TestSourcedReorderStaysWithinAContainer(t *testing.T) {
	ctx := context.Background()
	s, _, f := newSourced(t)
	a := addTask(t, s, f, "A")
	b := addTask(t, s, f, "B")
	other, err := s.AddSlice(ctx, Project{}, NewSlice{Title: "C", Milestone: domain.Milestone{ID: "sc-2", Name: "Other card"}})
	if err != nil {
		t.Fatal(err)
	}

	moved, to, err := s.ReorderSlice(ctx, Shape{}, b.ID, a.ID, true)
	if err != nil || moved.ID != b.ID || to.ID != a.ID || moved.MilestoneID != "sc-1" {
		t.Fatalf("same-container reorder = %+v, %+v, %v", moved, to, err)
	}
	_, _, err = s.ReorderSlice(ctx, Shape{}, a.ID, other.ID, true)
	checkErr(t, "cross-container ReorderSlice", err, "Work: a task is not moved between shortcut's containers")

	if _, _, err := s.ReorderSlice(ctx, Shape{}, "nope", a.ID, true); err == nil {
		t.Error("an unknown slice: want an error")
	}
	if _, _, err := s.ReorderSlice(ctx, Shape{}, a.ID, "nope", true); err == nil {
		t.Error("an unknown target: want an error")
	}
}

// Every other write and read is the file's, told to nobody.
func TestSourcedDelegatesTheRestToTheFile(t *testing.T) {
	ctx := context.Background()
	s, _, f := newSourced(t)
	sl := addTask(t, s, f, "Task")
	dep := addTask(t, s, f, "Dep")
	p := Project{ID: "proj", Name: "Work"}

	if sh, err := s.Shape(ctx, p); err != nil || len(sh.Milestones) != 1 {
		t.Errorf("Shape = %+v, %v", sh, err)
	}
	if plan, err := s.Plan(ctx, p); err != nil || len(plan.Project.Slices) != 2 {
		t.Errorf("Plan = %+v, %v", plan, err)
	}
	if got, _, err := s.Slice(ctx, sl.ID); err != nil || got.ID != sl.ID {
		t.Errorf("Slice = %+v, %v", got, err)
	}
	if err := s.EditSlice(ctx, sl.ID, "Task", "", "brief"); err != nil {
		t.Error(err)
	}
	if err := s.SetSliceBrief(ctx, sl.ID, "brief\n\n### PR description\n\nthe PR"); err != nil {
		t.Error(err)
	}
	var _ RepoSetter = s
	if err := s.SetSliceRepo(ctx, sl.ID, "/src/app"); err != nil {
		t.Error(err)
	}
	if got, _, err := s.Slice(ctx, sl.ID); err != nil || got.Repo != "/src/app" || got.Name != "Task" {
		t.Errorf("after SetSliceRepo = %+v, %v, want only the repo changed", got, err)
	}
	if err := s.SetSliceRepo(ctx, "nope", "/src/app"); err == nil {
		t.Error("SetSliceRepo on no such slice: want an error")
	}
	if body, err := s.Body(ctx, sl.ID); err != nil || !strings.HasPrefix(body, "brief") {
		t.Errorf("Body = %q, %v", body, err)
	}
	if d, err := s.PRDescription(ctx, sl.ID); err != nil || d != "the PR" {
		t.Errorf("PRDescription = %q, %v", d, err)
	}
	if got, err := s.SetDependencies(ctx, sl.ID, []string{dep.ID}); err != nil || len(got.DependsOn) != 1 {
		t.Errorf("SetDependencies = %+v, %v", got, err)
	}
	if err := s.ProposeFollowUps(ctx, sl.ID, []FollowUp{{Index: 1, Title: "more", Brief: "b"}}); err != nil {
		t.Error(err)
	}
	if err := s.RecordTriage(ctx, sl.ID, []Triaged{{Title: "more"}}); err != nil {
		t.Error(err)
	}
	if err := s.RecordVisuals(ctx, sl.ID, []VisualChange{{Index: 1, Name: "n", URI: "file:///x.png"}}); err != nil {
		t.Error(err)
	}
	if err := s.RecordSentBack(ctx, sl.ID, "redo"); err != nil {
		t.Error(err)
	}
	if err := s.RecordRelaunch(ctx, sl.ID); err != nil {
		t.Error(err)
	}
	if err := s.RecordChecksFailed(ctx, sl.ID, "- test"); err != nil {
		t.Error(err)
	}
	if err := s.RecordNote(ctx, sl.ID, "From Craig", "mind the cache"); err != nil {
		t.Error(err)
	}
	if body, _ := s.Body(ctx, sl.ID); !strings.HasSuffix(strings.TrimRight(body, "\n"), "### Note\n\n"+testStamp+"\n\nFrom Craig\n\nmind the cache") {
		t.Errorf("Body = %q, want it to end in the Note section", body)
	}
	if err := s.ReopenSlice(ctx, sl.ID, Shape{}); err != nil {
		t.Error(err)
	}
	if err := s.ClearBranch(ctx, sl.ID); err != nil {
		t.Error(err)
	}
	if _, err := s.AddSession(ctx, p, NewSession{ID: "sess"}); err != nil {
		t.Error(err)
	}
	if ss, err := s.Sessions(ctx, p); err != nil || len(ss) != 1 {
		t.Errorf("Sessions = %+v, %v", ss, err)
	}
	if err := s.EndSession(ctx, "sess"); err != nil {
		t.Error(err)
	}
	if err := s.DeleteSession(ctx, "sess"); err != nil {
		t.Error(err)
	}
	if len(f.Events) != 0 {
		t.Errorf("events = %+v, want none for writes the protocol has no event for", f.Events)
	}
}

func TestSourcedAnswersThePluginsOwnQuestions(t *testing.T) {
	ctx := context.Background()
	s, _, f := newSourced(t)
	f.DescribeResult = source.Describe{Name: "shortcut"}
	f.Groups = []source.Group{{ID: "doing"}}
	f.Details = map[string]source.ContainerDetail{"sc-1": {ID: "sc-1", Title: "Fix"}}
	f.ActionResult = source.ActionResult{Message: "ok"}

	var st Store = s
	if d, err := st.(Describer).Describe(ctx); err != nil || d.Name != "shortcut" {
		t.Errorf("Describe = %+v, %v", d, err)
	}
	if g, err := st.(SidebarReader).Sidebar(ctx, []string{"done"}); err != nil || len(g.Groups) != 1 ||
		!reflect.DeepEqual(f.Expands, [][]string{{"done"}}) {
		t.Errorf("Sidebar = %+v, %v (expands %v)", g, err, f.Expands)
	}
	if c, err := st.(ContainerReader).Container(ctx, "sc-1"); err != nil || c.Title != "Fix" {
		t.Errorf("Container = %+v, %v", c, err)
	}
	r, err := st.(ActionRunner).Action(ctx, "refresh", source.Target{Group: "doing"}, "in")
	if err != nil || r.Message != "ok" {
		t.Errorf("Action = %+v, %v", r, err)
	}
	want := source.ActionCall{Project: sourcedPlugin, Action: "refresh", Target: source.Target{Group: "doing"}, Input: "in"}
	if len(f.Actions) != 1 || !reflect.DeepEqual(f.Actions[0], want) {
		t.Errorf("actions = %+v, want %+v", f.Actions, want)
	}
	if s.Plugin() != sourcedPlugin {
		t.Errorf("Plugin = %+v", s.Plugin())
	}

	// A plan of nat's own answers none of them.
	l, _ := openPlan(t)
	var plain Store = l
	if _, ok := plain.(Describer); ok {
		t.Error("a Local must not answer Describer")
	}
}

func TestForProjectWrapsASourceProject(t *testing.T) {
	isolatedHome(t)
	ctx := context.Background()
	p := ProjectOf("proj", config.ProjectConfig{Name: "Work", Backend: config.BackendSource, Source: "shortcut", PlanDir: t.TempDir()})
	if p.Source != "shortcut" || p.Local {
		t.Fatalf("ProjectOf = %+v", p)
	}
	f := &source.Fake{}
	st, err := ForProject(ctx, p, nil, sourcedPlugin, f)
	if err != nil {
		t.Fatal(err)
	}
	sd, ok := st.(*Sourced)
	if !ok {
		t.Fatalf("store = %T, want *Sourced", st)
	}
	if sd.Plugin() != sourcedPlugin || sd.name != "shortcut" || sd.client != f {
		t.Errorf("sourced = %+v", sd)
	}
	_ = sd.local.Close()

	_, err = ForProject(ctx, p, nil, sourcedPlugin, nil)
	checkErr(t, "ForProject with no client", err, `the "Work" project's plan is kept with the shortcut task source`)
}
