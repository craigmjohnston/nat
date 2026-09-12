package store

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// readBack is the one thing every write test asks afterwards: the slice as the
// file now holds it, read through a fresh call rather than from what the write
// returned, so the assertion is about what was stored and not about what was
// handed back.
func readBack(t *testing.T, l *Local, id string) domain.Slice {
	t.Helper()
	s, _, err := l.Slice(context.Background(), id)
	if err != nil {
		t.Fatalf("read back %s: %v", id, err)
	}
	return s
}

// body is the slice's prose as the plan holds it.
func body(t *testing.T, l *Local, id string) string {
	t.Helper()
	b, err := l.Body(context.Background(), id)
	if err != nil {
		t.Fatalf("read the body of %s: %v", id, err)
	}
	return b
}

// localShape is what every local plan can record, which is everything: the
// caller of a write holds a shape read from the plan, and this is that shape.
var wholeShape = Shape{HasAssignee: true, HasBranch: true}

func TestLocalClaimAndRelease(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	claimed, err := l.ClaimSlice(ctx, "writes", wholeShape, "Craig Johnston")
	if err != nil {
		t.Fatalf("ClaimSlice: %v", err)
	}
	if claimed.Status != domain.SliceClaimed || claimed.AssigneeName != "Craig Johnston" {
		t.Errorf("claimed = %+v, want in progress and held by Craig Johnston", claimed)
	}
	if !Holds(claimed, wholeShape, "Craig Johnston") {
		t.Errorf("Holds(%+v) = false, want the claim to read as ownership", claimed)
	}
	if got := readBack(t, l, "writes"); !reflect.DeepEqual(got, claimed) {
		t.Errorf("stored = %+v, want what the claim answered with %+v", got, claimed)
	}

	released, err := l.ReleaseSlice(ctx, "writes", wholeShape, "Craig Johnston")
	if err != nil {
		t.Fatalf("ReleaseSlice: %v", err)
	}
	if released.Status != domain.SliceTodo || released.AssigneeName != "" {
		t.Errorf("released = %+v, want Todo and held by nobody", released)
	}
	if want := releasedLine("Craig Johnston"); !strings.Contains(body(t, l, "writes"), want) {
		t.Errorf("body = %q, want it to carry %q", body(t, l, "writes"), want)
	}
	// The brief is the work so far the next session wants, and a release is not
	// the place it goes.
	if got := body(t, l, "writes"); !strings.HasPrefix(got, "Write the plan.") {
		t.Errorf("body = %q, want the brief still at the top of it", got)
	}
}

// A shape that records no ownership decides it on status alone, so neither half
// of a claim-and-release writes an assignee — which is what the Notion store
// does with a project whose table has no such column.
func TestLocalClaimAndReleaseWithoutAnAssigneeColumn(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	claimed, err := l.ClaimSlice(ctx, "reads", Shape{}, "Somebody Else")
	if err != nil {
		t.Fatalf("ClaimSlice: %v", err)
	}
	if claimed.AssigneeName != "Craig Johnston" {
		t.Errorf("assignee = %q, want the column left exactly as it was", claimed.AssigneeName)
	}
	released, err := l.ReleaseSlice(ctx, "reads", Shape{}, "Craig Johnston")
	if err != nil {
		t.Fatalf("ReleaseSlice: %v", err)
	}
	if released.Status != domain.SliceTodo || released.AssigneeName != "Craig Johnston" {
		t.Errorf("released = %+v, want Todo with the assignee untouched", released)
	}
}

// A claim with nobody to name records nobody: the status is the claim, and
// writing an empty assignee over the one already there would be a release
// nobody asked for.
func TestLocalClaimWithNoUserLeavesTheAssignee(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)

	claimed, err := l.ClaimSlice(context.Background(), "reads", wholeShape, "")
	if err != nil {
		t.Fatalf("ClaimSlice: %v", err)
	}
	if claimed.AssigneeName != "Craig Johnston" {
		t.Errorf("assignee = %q, want it left as it was", claimed.AssigneeName)
	}
}

// The three endings a session has, each writing what it says and nothing else.
func TestLocalCompleteSlice(t *testing.T) {
	ctx := context.Background()
	cases := map[string]struct {
		outcome Outcome
		status  domain.SliceStatus
		branch  string
		pr      string
		heading string
	}{
		"handed back": {
			outcome: Outcome{Summary: "Wrote it.", Branch: "slice/writes", PRDescription: "Write a local plan\n\nWhat it does."},
			status:  domain.SliceClaimed, branch: "slice/writes", heading: handedBackHeading,
		},
		"pull request": {
			outcome: Outcome{Summary: "Opened it.", PR: "https://example.test/pr/9"},
			status:  domain.SliceClaimed, pr: "https://example.test/pr/9", heading: summaryHeading,
		},
		"blocked": {
			outcome: Outcome{Summary: "Waiting on the reads.", Blocked: true},
			status:  domain.SliceClaimed, heading: blockedHeading,
		},
		"done": {
			outcome: Outcome{Summary: "No pull request to come."},
			status:  domain.SliceDone, heading: summaryHeading,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			l, _ := openPlan(t)
			fillPlan(t, l)

			got, err := l.CompleteSlice(ctx, "reads", wholeShape, c.outcome)
			if err != nil {
				t.Fatalf("CompleteSlice: %v", err)
			}
			if got.Status != c.status {
				t.Errorf("status = %q, want %q", got.Status, c.status)
			}
			// The branch and the pull request the slice already carried stand
			// where the ending says nothing about them.
			wantBranch, wantPR := c.branch, c.pr
			if wantBranch == "" {
				wantBranch = "slice/reads"
			}
			if got.Branch != wantBranch || got.PRURL != wantPR {
				t.Errorf("branch/pr = %q/%q, want %q/%q", got.Branch, got.PRURL, wantBranch, wantPR)
			}
			text := body(t, l, "reads")
			if !strings.Contains(text, "### "+c.heading+"\n\n"+c.outcome.Summary) {
				t.Errorf("body = %q, want the summary filed under %q", text, c.heading)
			}
			if !strings.HasPrefix(text, "Read the plan.") {
				t.Errorf("body = %q, want the brief still at the top of it", text)
			}
			if c.outcome.PRDescription == "" {
				return
			}
			// The description is filed by the very heading the read half finds
			// it back by, and the last such section is the one that counts — the
			// slice's brief already carried one from an earlier hand-back.
			desc, err := l.PRDescription(ctx, "reads")
			if err != nil {
				t.Fatalf("PRDescription: %v", err)
			}
			if desc != c.outcome.PRDescription {
				t.Errorf("PRDescription = %q, want %q", desc, c.outcome.PRDescription)
			}
		})
	}
}

func TestLocalRecordPRAndMarkDone(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	if err := l.RecordPR(ctx, "reads", "https://example.test/pr/2"); err != nil {
		t.Fatalf("RecordPR: %v", err)
	}
	got := readBack(t, l, "reads")
	if got.PRURL != "https://example.test/pr/2" {
		t.Errorf("PR = %q, want the one recorded", got.PRURL)
	}
	if got.Status != domain.SliceClaimed {
		t.Errorf("status = %q, want the slice left in progress: the merge is what marks it Done", got.Status)
	}

	if err := l.MarkDone(ctx, "reads", wholeShape); err != nil {
		t.Fatalf("MarkDone: %v", err)
	}
	if got := readBack(t, l, "reads"); got.Status != domain.SliceDone {
		t.Errorf("status = %q, want Done", got.Status)
	}
}

func TestLocalAddMilestones(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	if added, err := l.AddMilestones(ctx, Project{}, wholeShape, nil); err != nil || added != nil {
		t.Errorf("AddMilestones(nothing) = %v, %v, want nothing written", added, err)
	}

	added, err := l.AddMilestones(ctx, Project{}, wholeShape, []string{"M3: Writes", "M4: Sync"})
	if err != nil {
		t.Fatalf("AddMilestones: %v", err)
	}
	want := []domain.Milestone{
		{ID: "M3: Writes", Name: "M3: Writes", Order: 2, Status: domain.MilestoneQueued},
		{ID: "M4: Sync", Name: "M4: Sync", Order: 3, Status: domain.MilestoneQueued},
	}
	if !reflect.DeepEqual(added, want) {
		t.Errorf("added = %+v, want %+v", added, want)
	}

	sh, err := l.Shape(ctx, Project{})
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	names := make([]string, len(sh.Milestones))
	for i, m := range sh.Milestones {
		names[i] = m.Name
	}
	if got := strings.Join(names, ", "); got != "M1: The format, M2: Reads, M3: Writes, M4: Sync" {
		t.Errorf("milestones = %q, want the two appended at the end of the plan", got)
	}
}

// A milestone is nothing but its name, so a plan cannot hold two of one — and
// the names it is checked against are the plan's own as the write reads them,
// not the shape the caller was handed however long ago.
func TestLocalAddMilestonesRefusesANameThePlanHolds(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)

	_, err := l.AddMilestones(context.Background(), Project{}, Shape{}, []string{"M3", "  m2: reads  "})
	if err == nil {
		t.Fatal("AddMilestones: want a duplicate name refused")
	}
	if !strings.Contains(err.Error(), `"M2: Reads"`) || !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the milestone and the file named", err)
	}
	// All of them or none: the one that would have been fine is not there.
	sh, err := l.Shape(context.Background(), Project{})
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	if len(sh.Milestones) != 2 {
		t.Errorf("milestones = %+v, want the refused run to have written nothing", sh.Milestones)
	}
}

func TestLocalAddSlice(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	added, err := l.AddSlice(ctx, Project{}, NewSlice{
		Title:     "Write the plan through",
		Brief:     "Write it.",
		Repo:      "/tmp/other",
		Milestone: domain.Milestone{ID: "M2: Reads", Name: "M2: Reads"},
		DependsOn: []string{"writes", "design"},
	})
	if err != nil {
		t.Fatalf("AddSlice: %v", err)
	}
	if added.ID == "" {
		t.Fatal("AddSlice: want an ID for the new slice")
	}
	want := domain.Slice{
		ID: added.ID, Name: "Write the plan through",
		Status: domain.SliceTodo, StatusName: "Todo", MilestoneID: "M2: Reads",
		Repo: "/tmp/other", DependsOn: []string{"writes", "design"},
	}
	if !reflect.DeepEqual(added, want) {
		t.Errorf("added = %+v, want %+v", added, want)
	}
	if got := body(t, l, added.ID); got != "Write it." {
		t.Errorf("brief = %q, want the one filed", got)
	}

	// It goes at the end of the plan, which is where a newly filed slice goes.
	plan, err := l.Plan(ctx, Project{ID: "proj"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	last := plan.Project.Slices[len(plan.Project.Slices)-1]
	if last.ID != added.ID {
		t.Errorf("last slice = %q, want the one just added", last.ID)
	}
	// Two slices never share an ID, which is the whole of what the generated
	// one has to promise.
	second, err := l.AddSlice(ctx, Project{}, NewSlice{Title: "Another"})
	if err != nil {
		t.Fatalf("AddSlice: %v", err)
	}
	if second.ID == added.ID {
		t.Errorf("two slices share the ID %q", second.ID)
	}
	if second.MilestoneID != "" {
		t.Errorf("milestone = %q, want a slice filed under none to have none", second.MilestoneID)
	}
}

func TestLocalEditMoveAndDelete(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	if err := l.EditSlice(ctx, "writes", "Implement the writes", "/tmp/repo", "Write it properly."); err != nil {
		t.Fatalf("EditSlice: %v", err)
	}
	got := readBack(t, l, "writes")
	if got.Name != "Implement the writes" || got.Repo != "/tmp/repo" {
		t.Errorf("edited = %+v, want the title and repo rewritten", got)
	}
	if got.MilestoneID != "M2: Reads" || got.Status != domain.SliceTodo {
		t.Errorf("edited = %+v, want its milestone and status left alone", got)
	}
	if b := body(t, l, "writes"); b != "Write it properly." {
		t.Errorf("brief = %q, want the one written", b)
	}

	if err := l.SetSliceBrief(ctx, "writes", "Write it very properly."); err != nil {
		t.Fatalf("SetSliceBrief: %v", err)
	}
	if b := body(t, l, "writes"); b != "Write it very properly." {
		t.Errorf("brief = %q, want the one written", b)
	}

	if err := l.MoveSlice(ctx, "writes", domain.Milestone{ID: "M1: The format", Name: "M1: The format"}); err != nil {
		t.Fatalf("MoveSlice: %v", err)
	}
	if got := readBack(t, l, "writes"); got.MilestoneID != "M1: The format" {
		t.Errorf("milestone = %q, want the slice refiled", got.MilestoneID)
	}
	if err := l.MoveSlice(ctx, "writes", domain.Milestone{}); err != nil {
		t.Fatalf("MoveSlice(nowhere): %v", err)
	}
	if got := readBack(t, l, "writes"); got.MilestoneID != "" {
		t.Errorf("milestone = %q, want the slice under none", got.MilestoneID)
	}

	if err := l.DeleteSlice(ctx, "writes"); err != nil {
		t.Fatalf("DeleteSlice: %v", err)
	}
	if _, _, err := l.Slice(ctx, "writes"); err == nil {
		t.Error("Slice: want the deleted slice gone")
	}
	// The waits either side of it go too: a dependency on a slice that is not
	// there is a wait with no end.
	deps, err := l.dependencies(ctx, l.db)
	if err != nil {
		t.Fatalf("dependencies: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("dependencies = %+v, want the deleted slice's waits gone with it", deps)
	}
}

func TestLocalSetDependencies(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	got, err := l.SetDependencies(ctx, "writes", []string{"design"})
	if err != nil {
		t.Fatalf("SetDependencies: %v", err)
	}
	if !reflect.DeepEqual(got.DependsOn, []string{"design"}) {
		t.Errorf("depends on %v, want exactly the one recorded", got.DependsOn)
	}
	got, err = l.SetDependencies(ctx, "writes", nil)
	if err != nil {
		t.Fatalf("SetDependencies(nothing): %v", err)
	}
	if got.DependsOn != nil {
		t.Errorf("depends on %v, want the slice freed", got.DependsOn)
	}
}

// A wait on a slice the plan does not hold is a wait with no end, and the
// foreign keys are what make it impossible rather than merely wrong.
func TestLocalSetDependenciesRefusesASliceThatIsNotThere(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)

	_, err := l.SetDependencies(context.Background(), "writes", []string{"design", "ghost"})
	if err == nil {
		t.Fatal("SetDependencies: want a dependency on nothing refused")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the file named", err)
	}
	// The transaction took the old waits with it rather than leaving the slice
	// half rewritten.
	if got := readBack(t, l, "writes"); !reflect.DeepEqual(got.DependsOn, []string{"reads", "design"}) {
		t.Errorf("depends on %v, want the refused write to have changed nothing", got.DependsOn)
	}
}

// A slice filed under a milestone nothing knows about is one the board draws
// nowhere, which is what Notion's own select column refuses for the other
// store.
func TestLocalRefusesAMilestoneThePlanDoesNotHold(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	writes := map[string]func() error{
		"AddSlice": func() error {
			_, err := l.AddSlice(ctx, Project{}, NewSlice{Title: "x", Milestone: domain.Milestone{ID: "M9: Ghost"}})
			return err
		},
		"MoveSlice": func() error {
			return l.MoveSlice(ctx, "writes", domain.Milestone{ID: "M9: Ghost"})
		},
	}
	for name, w := range writes {
		err := w()
		if err == nil {
			t.Errorf("%s: want an unknown milestone refused", name)
			continue
		}
		if !strings.Contains(err.Error(), `"M9: Ghost"`) || !strings.Contains(err.Error(), path) {
			t.Errorf("%s: error = %q, want the milestone and the file named", name, err)
		}
	}
	if got := readBack(t, l, "writes"); got.MilestoneID != "M2: Reads" {
		t.Errorf("milestone = %q, want the refused move to have changed nothing", got.MilestoneID)
	}
}

// Every write is about a slice, and a slice that is not there is said so
// rather than quietly updating no rows — which is the failure a write against
// a slice somebody has deleted would otherwise be.
func TestLocalWritesRefuseASliceThatIsNotThere(t *testing.T) {
	l, _ := openPlan(t)
	ctx := context.Background()

	writes := map[string]func() error{
		"ClaimSlice":   func() error { _, err := l.ClaimSlice(ctx, "ghost", wholeShape, "u"); return err },
		"ReleaseSlice": func() error { _, err := l.ReleaseSlice(ctx, "ghost", wholeShape, "u"); return err },
		"CompleteSlice": func() error {
			_, err := l.CompleteSlice(ctx, "ghost", wholeShape, Outcome{Summary: "done"})
			return err
		},
		"RecordPR":        func() error { return l.RecordPR(ctx, "ghost", "url") },
		"MarkDone":        func() error { return l.MarkDone(ctx, "ghost", wholeShape) },
		"EditSlice":       func() error { return l.EditSlice(ctx, "ghost", "t", "r", "b") },
		"SetSliceBrief":   func() error { return l.SetSliceBrief(ctx, "ghost", "b") },
		"SetDependencies": func() error { _, err := l.SetDependencies(ctx, "ghost", nil); return err },
		"MoveSlice":       func() error { return l.MoveSlice(ctx, "ghost", domain.Milestone{}) },
		"DeleteSlice":     func() error { return l.DeleteSlice(ctx, "ghost") },
	}
	for name, w := range writes {
		err := w()
		if err == nil {
			t.Errorf("%s: want a write to a slice that is not there refused", name)
			continue
		}
		if !strings.Contains(err.Error(), "no slice ghost") {
			t.Errorf("%s: error = %q, want the slice named", name, err)
		}
	}
}

// The point of reading inside the write: a change another process made after
// the caller last read the slice is still there afterwards. The board's copy of
// a slice is as old as its last poll, and an agent has been writing since.
func TestLocalWriteDoesNotClobberAnotherProcessesChange(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	// What the caller holds: the slice as it was before anything else touched
	// it. Nothing below is allowed to write this back.
	stale := readBack(t, l, "reads")

	other, err := OpenLocal(path)
	if err != nil {
		t.Fatalf("open the plan again: %v", err)
	}
	defer func() { _ = other.Close() }()
	if err := other.SetSliceBrief(ctx, "reads", "A brief rewritten while the agent worked."); err != nil {
		t.Fatalf("SetSliceBrief: %v", err)
	}
	if err := other.RecordPR(ctx, "reads", "https://example.test/pr/3"); err != nil {
		t.Fatalf("RecordPR: %v", err)
	}

	if _, err := l.CompleteSlice(ctx, "reads", wholeShape,
		Outcome{Summary: "Handed it back.", Branch: "slice/reads"}); err != nil {
		t.Fatalf("CompleteSlice: %v", err)
	}

	got := readBack(t, l, "reads")
	if got.PRURL != "https://example.test/pr/3" {
		t.Errorf("PR = %q, want the other process's write kept", got.PRURL)
	}
	if got.Branch != "slice/reads" || got.Branch != stale.Branch {
		t.Errorf("branch = %q, want the hand-back's own", got.Branch)
	}
	text := body(t, l, "reads")
	if !strings.HasPrefix(text, "A brief rewritten while the agent worked.") {
		t.Errorf("body = %q, want the brief as it stood when the note was written", text)
	}
	if !strings.Contains(text, "### "+handedBackHeading) {
		t.Errorf("body = %q, want the note appended to it", text)
	}
}

// Several processes writing one file at once is what a board and its agents
// are, and what WAL and a busy timeout are for: every write lands, and none of
// them is lost to another.
func TestLocalTakesConcurrentWritesFromSeveralProcesses(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	ids := []string{"design", "reads", "writes", "stray"}
	var wg sync.WaitGroup
	errs := make([]error, len(ids))
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A store of its own, as a separate process would have.
			w, err := OpenLocal(path)
			if err != nil {
				errs[i] = err
				return
			}
			defer func() { _ = w.Close() }()
			_, errs[i] = w.CompleteSlice(ctx, id, wholeShape, Outcome{Summary: "Done by " + id})
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("CompleteSlice(%s): %v", ids[i], err)
		}
	}
	for _, id := range ids {
		if got := readBack(t, l, id); got.Status != domain.SliceDone {
			t.Errorf("%s = %q, want every concurrent write to have landed", id, got.Status)
		}
		if text := body(t, l, id); !strings.Contains(text, "Done by "+id) {
			t.Errorf("%s body = %q, want its own note", id, text)
		}
	}
}

// A write that fails part way leaves nothing behind: the whole of it is one
// transaction, which is what a temp file and a rename bought the design this
// store was drawn up against, without the cost of publishing one writer's whole
// idea of the plan over another's.
func TestLocalAFailedWriteLeavesTheFileAsItWas(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	before, err := l.Plan(ctx, Project{ID: "proj"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, err := l.AddSlice(ctx, Project{}, NewSlice{
		Title:     "Waits on nothing that exists",
		Milestone: domain.Milestone{ID: "M2: Reads"},
		DependsOn: []string{"ghost"},
	}); err == nil {
		t.Fatal("AddSlice: want a wait on a slice that is not there refused")
	}
	after, err := l.Plan(ctx, Project{ID: "proj"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !reflect.DeepEqual(before.Project.Slices, after.Project.Slices) {
		t.Errorf("slices = %+v, want the refused write to have left the plan as it was", after.Project.Slices)
	}
}

// A commit that fails is the store's own failure said in the store's own words,
// naming the file: SQLite checks a deferred foreign key at the commit rather
// than at the statement, which is a cheap way of asking for one.
func TestLocalNamesItsFileWhenACommitFails(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	err := l.withTx(ctx, "test the commit", func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO slice_deps (slice_id, depends_on, position) VALUES ('writes', 'ghost', 9)`)
		return err
	})
	if err == nil {
		t.Fatal("withTx: want the commit refused")
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "test the commit") {
		t.Errorf("error = %q, want the file and what it was doing named", err)
	}
}

// Every write opens a transaction, and a database that will not answer at all
// fails there — with the file named, since a plan that will not open is a path
// on this machine.
func TestLocalNamesItsFileWhenATransactionWillNotBegin(t *testing.T) {
	l := &Local{db: brokenDB(t), path: "/plans/broken.db"}
	ctx := context.Background()

	writes := map[string]func() error{
		"ClaimSlice": func() error { _, err := l.ClaimSlice(ctx, "x", wholeShape, "u"); return err },
		"AddMilestones": func() error {
			_, err := l.AddMilestones(ctx, Project{}, Shape{}, []string{"M1"})
			return err
		},
		"AddSlice":    func() error { _, err := l.AddSlice(ctx, Project{}, NewSlice{Title: "x"}); return err },
		"DeleteSlice": func() error { return l.DeleteSlice(ctx, "x") },
	}
	for name, w := range writes {
		err := w()
		if err == nil {
			t.Errorf("%s: want a database that will not answer reported", name)
			continue
		}
		if !strings.Contains(err.Error(), "/plans/broken.db") {
			t.Errorf("%s: error = %q, want the file named", name, err)
		}
	}
}

// A plan hand-edited into something that reads and will not be written — a
// table replaced by a view is the cheapest such thing — is reported by every
// write, naming the file, rather than passed off as a write that happened.
func TestLocalNamesItsFileWhenAWriteIsRefused(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()
	// The rows the plan held, now readable and not writable.
	write(t, l, `ALTER TABLE slices RENAME TO slices_data`)
	write(t, l, `CREATE VIEW slices (id, title, status, milestone, position, assignee, repo, branch, pr, body)
		AS SELECT id, title, status, milestone, position, assignee, repo, branch, pr, body FROM slices_data`)

	writes := map[string]func() error{
		"ClaimSlice":   func() error { _, err := l.ClaimSlice(ctx, "writes", wholeShape, "u"); return err },
		"ReleaseSlice": func() error { _, err := l.ReleaseSlice(ctx, "writes", wholeShape, "u"); return err },
		"ReleaseSlice without an assignee column": func() error {
			_, err := l.ReleaseSlice(ctx, "writes", Shape{}, "u")
			return err
		},
		"CompleteSlice": func() error {
			_, err := l.CompleteSlice(ctx, "writes", wholeShape, Outcome{Summary: "s"})
			return err
		},
		"RecordPR":      func() error { return l.RecordPR(ctx, "writes", "url") },
		"MarkDone":      func() error { return l.MarkDone(ctx, "writes", wholeShape) },
		"EditSlice":     func() error { return l.EditSlice(ctx, "writes", "t", "r", "b") },
		"SetSliceBrief": func() error { return l.SetSliceBrief(ctx, "writes", "b") },
		"MoveSlice":     func() error { return l.MoveSlice(ctx, "writes", domain.Milestone{}) },
		"AddSlice":      func() error { _, err := l.AddSlice(ctx, Project{}, NewSlice{Title: "x"}); return err },
		"DeleteSlice":   func() error { return l.DeleteSlice(ctx, "writes") },
	}
	for name, w := range writes {
		err := w()
		if err == nil {
			t.Errorf("%s: want a write to a view refused", name)
			continue
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("%s: error = %q, want the file named", name, err)
		}
	}
}

// The reads a write takes inside its own transaction are reported the same way
// the reads outside one are: each names the file, since a plan that will not
// read is a path on this machine whichever read found out.
func TestLocalNamesItsFileWhenAWritesOwnReadFails(t *testing.T) {
	ctx := context.Background()
	cases := map[string]struct {
		break_ func(t *testing.T, l *Local)
		write  func(l *Local) error
	}{
		"the body a note is appended to": {
			break_: func(t *testing.T, l *Local) {
				// A body column that cannot be evaluated: the slice itself still
				// reads, since no read of one asks for its body.
				write(t, l, `ALTER TABLE slices RENAME TO slices_data`)
				write(t, l, `CREATE VIEW slices (id, title, status, milestone, position, assignee, repo, branch, pr, body)
					AS SELECT id, title, status, milestone, position, assignee, repo, branch, pr,
					abs(-9223372036854775808) FROM slices_data`)
			},
			write: func(l *Local) error {
				_, err := l.CompleteSlice(ctx, "writes", wholeShape, Outcome{Summary: "s"})
				return err
			},
		},
		"the body a release's line is appended to": {
			break_: func(t *testing.T, l *Local) {
				write(t, l, `ALTER TABLE slices RENAME TO slices_data`)
				write(t, l, `CREATE VIEW slices (id, title, status, milestone, position, assignee, repo, branch, pr, body)
					AS SELECT id, title, status, milestone, position, assignee, repo, branch, pr,
					abs(-9223372036854775808) FROM slices_data`)
			},
			write: func(l *Local) error {
				_, err := l.ReleaseSlice(ctx, "writes", wholeShape, "u")
				return err
			},
		},
		"the milestones a new one is appended after": {
			break_: func(t *testing.T, l *Local) { write(t, l, `DROP TABLE milestones`) },
			write: func(l *Local) error {
				_, err := l.AddMilestones(ctx, Project{}, Shape{}, []string{"M3"})
				return err
			},
		},
		"the milestone a slice is filed under": {
			break_: func(t *testing.T, l *Local) { write(t, l, `DROP TABLE milestones`) },
			write: func(l *Local) error {
				return l.MoveSlice(ctx, "writes", domain.Milestone{ID: "M1: The format"})
			},
		},
		"the end of the plan a slice is added at": {
			break_: func(t *testing.T, l *Local) {
				// A position column that cannot be evaluated: where the end of
				// the plan is is the one thing adding a slice reads.
				write(t, l, `ALTER TABLE slices RENAME TO slices_data`)
				write(t, l, `CREATE VIEW slices (id, title, status, milestone, position, assignee, repo, branch, pr, body)
					AS SELECT id, title, status, milestone, abs(-9223372036854775808),
					assignee, repo, branch, pr, body FROM slices_data`)
			},
			write: func(l *Local) error {
				_, err := l.AddSlice(ctx, Project{}, NewSlice{Title: "x"})
				return err
			},
		},
		"the waits a slice already records": {
			break_: func(t *testing.T, l *Local) {
				write(t, l, `ALTER TABLE slice_deps RENAME TO slice_deps_data`)
				write(t, l, `CREATE VIEW slice_deps (slice_id, depends_on, position)
					AS SELECT slice_id, depends_on, position FROM slice_deps_data`)
			},
			write: func(l *Local) error {
				_, err := l.SetDependencies(ctx, "writes", nil)
				return err
			},
		},
		"the waits either side of a deleted slice": {
			break_: func(t *testing.T, l *Local) {
				write(t, l, `ALTER TABLE slice_deps RENAME TO slice_deps_data`)
				write(t, l, `CREATE VIEW slice_deps (slice_id, depends_on, position)
					AS SELECT slice_id, depends_on, position FROM slice_deps_data`)
			},
			write: func(l *Local) error { return l.DeleteSlice(ctx, "writes") },
		},
		"a milestone appended to the plan": {
			break_: func(t *testing.T, l *Local) {
				write(t, l, `ALTER TABLE milestones RENAME TO milestones_data`)
				write(t, l, `CREATE VIEW milestones (name, position)
					AS SELECT name, position FROM milestones_data`)
			},
			write: func(l *Local) error {
				_, err := l.AddMilestones(ctx, Project{}, Shape{}, []string{"M3"})
				return err
			},
		},
		"the sync row a deleted slice leaves": {
			break_: func(t *testing.T, l *Local) {
				write(t, l, `DROP TABLE sync`)
				write(t, l, `CREATE VIEW sync (slice_id, dirty, synced_at) AS SELECT 'x', 0, NULL`)
			},
			write: func(l *Local) error { return l.DeleteSlice(ctx, "writes") },
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			l, path := openPlan(t)
			fillPlan(t, l)
			c.break_(t, l)
			err := c.write(l)
			if err == nil {
				t.Fatalf("%s: want the failed read reported", name)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error = %q, want the file named", err)
			}
		})
	}
}

// The slice a write reads back is read inside the transaction too, so a plan
// that stops answering half way through one is reported rather than answered
// with a slice nobody read.
func TestLocalNamesItsFileWhenTheReadBackFails(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	_, err := l.updateSlice(ctx, "writes", "test the read back", func(tx *sql.Tx, _ domain.Slice) error {
		_, err := tx.ExecContext(ctx, `DROP TABLE slice_deps`)
		return err
	})
	if err == nil {
		t.Fatal("updateSlice: want the read back's failure reported")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the file named", err)
	}
}

// sliceBody is a read of one column of one row, and says so with the file when
// there is no such row — which is every caller's own bug, since each reads the
// slice first.
func TestLocalSliceBodyNamesItsFile(t *testing.T) {
	l, path := openPlan(t)
	ctx := context.Background()

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := l.sliceBody(ctx, tx, "ghost"); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("sliceBody = %v, want the file named", err)
	}
}

// A note goes on the end of what is there, one blank line clear of it, and a
// body with nothing in it yet starts with the note rather than with a blank
// line of its own.
func TestAppendSection(t *testing.T) {
	cases := []struct {
		body, heading, text, want string
	}{
		{"A brief.", "Summary", "Wrote it.", "A brief.\n\n### Summary\n\nWrote it."},
		{"A brief.\n\n", "Summary", "  Wrote it.  ", "A brief.\n\n### Summary\n\nWrote it."},
		{"", "Summary", "Wrote it.", "### Summary\n\nWrote it."},
		{"A brief.", "Summary", "", "A brief.\n\n### Summary"},
	}
	for _, c := range cases {
		if got := appendSection(c.body, c.heading, c.text); got != c.want {
			t.Errorf("appendSection(%q, %q, %q) = %q, want %q", c.body, c.heading, c.text, got, c.want)
		}
	}
}

// The statuses a local plan writes are the ones domain names, which is what
// lets the structs this store produces be the structs everything above it
// already reads.
func TestLocalWritesTheStatusesDomainNames(t *testing.T) {
	for _, s := range []string{notion.SliceTodo, notion.SliceInProgress, notion.SliceDone} {
		switch domain.SliceStatus(s) {
		case domain.SliceTodo, domain.SliceClaimed, domain.SliceDone:
		default:
			t.Errorf("%q is not a status domain knows", s)
		}
	}
}

// A rename keeps the milestone where it is in the plan and carries its slices
// over with it, which here is one update of each of two tables.
func TestLocalRenameMilestone(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	m, err := l.RenameMilestone(ctx, Project{}, wholeShape, "  m2: reads  ", "M2: Reading")
	if err != nil {
		t.Fatalf("RenameMilestone: %v", err)
	}
	want := domain.Milestone{ID: "M2: Reading", Name: "M2: Reading", Order: 1, Status: domain.MilestoneActive}
	if m != want {
		t.Errorf("milestone = %+v, want %+v", m, want)
	}

	sh, err := l.Shape(ctx, Project{})
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	names := make([]string, len(sh.Milestones))
	for i, ms := range sh.Milestones {
		names[i] = ms.Name
	}
	if got := strings.Join(names, ", "); got != "M1: The format, M2: Reading" {
		t.Errorf("milestones = %q, want the one renamed where it was", got)
	}
	for _, id := range []string{"reads", "writes"} {
		if got := readBack(t, l, id).MilestoneID; got != "M2: Reading" {
			t.Errorf("slice %s is under %q, want the renamed milestone", id, got)
		}
	}
	if got := readBack(t, l, "design").MilestoneID; got != "M1: The format" {
		t.Errorf("slice design is under %q, want its own milestone untouched", got)
	}
}

func TestLocalRenameMilestoneRefusals(t *testing.T) {
	unchanged := func(t *testing.T, l *Local) {
		t.Helper()
		sh, err := l.Shape(context.Background(), Project{})
		if err != nil {
			t.Fatalf("Shape: %v", err)
		}
		if len(sh.Milestones) != 2 || sh.Milestones[1].Name != "M2: Reads" {
			t.Errorf("milestones = %+v, want the refused run to have written nothing", sh.Milestones)
		}
	}
	t.Run("a new name the plan already holds", func(t *testing.T) {
		l, path := openPlan(t)
		fillPlan(t, l)
		_, err := l.RenameMilestone(context.Background(), Project{}, wholeShape, "M2: Reads", "  m1: the format  ")
		if err == nil || !strings.Contains(err.Error(), `already has a milestone named "M1: The format"`) {
			t.Fatalf("err = %v, want the duplicate refused by name", err)
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("err = %q, want the file named", err)
		}
		unchanged(t, l)
	})
	t.Run("an old name the plan does not hold", func(t *testing.T) {
		l, path := openPlan(t)
		fillPlan(t, l)
		_, err := l.RenameMilestone(context.Background(), Project{}, wholeShape, "M9: Nothing", "M3: Sync")
		if err == nil || !strings.Contains(err.Error(), `no milestone named "M9: Nothing"`) {
			t.Fatalf("err = %v, want the missing name refused", err)
		}
		if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), `"M2: Reads"`) {
			t.Errorf("err = %q, want the file and the plan named", err)
		}
		unchanged(t, l)
	})
	t.Run("a plan that cannot be read", func(t *testing.T) {
		l, _ := openPlan(t)
		fillPlan(t, l)
		write(t, l, `DROP TABLE milestones`)
		_, err := l.RenameMilestone(context.Background(), Project{}, wholeShape, "M2: Reads", "M3: Sync")
		if err == nil || !strings.Contains(err.Error(), "read the milestones") {
			t.Errorf("err = %v, want the read reported", err)
		}
	})
	t.Run("a milestone that cannot be renamed", func(t *testing.T) {
		l, _ := openPlan(t)
		fillPlan(t, l)
		write(t, l, `CREATE TRIGGER refuse BEFORE UPDATE ON milestones
			BEGIN SELECT RAISE(ABORT, 'no renames here'); END`)
		_, err := l.RenameMilestone(context.Background(), Project{}, wholeShape, "M2: Reads", "M3: Sync")
		if err == nil || !strings.Contains(err.Error(), "rename the milestone") {
			t.Errorf("err = %v, want the write reported", err)
		}
	})
	t.Run("slices that cannot be refiled", func(t *testing.T) {
		l, _ := openPlan(t)
		fillPlan(t, l)
		write(t, l, `CREATE TRIGGER refuse BEFORE UPDATE ON slices
			BEGIN SELECT RAISE(ABORT, 'no refiling here'); END`)
		_, err := l.RenameMilestone(context.Background(), Project{}, wholeShape, "M2: Reads", "M3: Sync")
		if err == nil || !strings.Contains(err.Error(), "refile the milestone's slices") {
			t.Errorf("err = %v, want the write reported", err)
		}
		// The transaction is one write or none: the milestone is where it was.
		sh, err := l.Shape(context.Background(), Project{})
		if err != nil {
			t.Fatalf("Shape: %v", err)
		}
		if sh.Milestones[1].Name != "M2: Reads" {
			t.Errorf("milestones = %+v, want the rolled-back rename", sh.Milestones)
		}
	})
	t.Run("slices that cannot be read", func(t *testing.T) {
		l, _ := openPlan(t)
		fillPlan(t, l)
		write(t, l, `DELETE FROM slice_deps`)
		write(t, l, `DROP TABLE slices`)
		_, err := l.RenameMilestone(context.Background(), Project{}, wholeShape, "M2: Reads", "M3: Sync")
		if err == nil || !strings.Contains(err.Error(), "read the slices") {
			t.Errorf("err = %v, want the read reported", err)
		}
	})
}

// A removal takes the milestone off the plan and closes the gap behind it, so
// the next milestone added still lands at the end rather than on top of one
// already there.
func TestLocalRemoveMilestone(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()
	// The plan's second milestone holds slices; the first is emptied so there is
	// something to remove.
	write(t, l, `UPDATE slices SET milestone = ? WHERE milestone = ?`, "M2: Reads", "M1: The format")

	m, err := l.RemoveMilestone(ctx, Project{}, wholeShape, "  m1: the format  ")
	if err != nil {
		t.Fatalf("RemoveMilestone: %v", err)
	}
	want := domain.Milestone{ID: "M1: The format", Name: "M1: The format", Order: 0, Status: domain.MilestoneQueued}
	if m != want {
		t.Errorf("milestone = %+v, want %+v", m, want)
	}

	sh, err := l.Shape(ctx, Project{})
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	if len(sh.Milestones) != 1 || sh.Milestones[0].Name != "M2: Reads" || sh.Milestones[0].Order != 0 {
		t.Fatalf("milestones = %+v, want the survivor alone, closed up to the front", sh.Milestones)
	}
	// Closing the gap is what keeps the next one added landing at the end.
	added, err := l.AddMilestones(ctx, Project{}, wholeShape, []string{"M3: Sync"})
	if err != nil {
		t.Fatalf("AddMilestones: %v", err)
	}
	if added[0].Order != 1 {
		t.Errorf("added milestone = %+v, want it at the end of the plan", added[0])
	}
}

func TestLocalRemoveMilestoneRefusals(t *testing.T) {
	unchanged := func(t *testing.T, l *Local) {
		t.Helper()
		sh, err := l.Shape(context.Background(), Project{})
		if err != nil {
			t.Fatalf("Shape: %v", err)
		}
		if len(sh.Milestones) != 2 {
			t.Errorf("milestones = %+v, want the refused run to have written nothing", sh.Milestones)
		}
	}
	t.Run("a name the plan does not hold", func(t *testing.T) {
		l, path := openPlan(t)
		fillPlan(t, l)
		_, err := l.RemoveMilestone(context.Background(), Project{}, wholeShape, " M9: Nothing ")
		if err == nil || !strings.Contains(err.Error(), `no milestone named "M9: Nothing"`) {
			t.Fatalf("err = %v, want the missing name refused", err)
		}
		if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), `"M2: Reads"`) {
			t.Errorf("err = %q, want the file and the plan named", err)
		}
		unchanged(t, l)
	})
	t.Run("a milestone with slices still filed under it", func(t *testing.T) {
		l, path := openPlan(t)
		fillPlan(t, l)
		_, err := l.RemoveMilestone(context.Background(), Project{}, wholeShape, "M2: Reads")
		if err == nil || !strings.Contains(err.Error(),
			`still holds 2 slices ("Implement the local store: reads", "Implement the local store: writes")`) {
			t.Fatalf("err = %v, want the slices under it named", err)
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("err = %q, want the file named", err)
		}
		unchanged(t, l)
	})
	t.Run("a plan that cannot be read", func(t *testing.T) {
		l, _ := openPlan(t)
		fillPlan(t, l)
		write(t, l, `DROP TABLE milestones`)
		_, err := l.RemoveMilestone(context.Background(), Project{}, wholeShape, "M1: The format")
		if err == nil || !strings.Contains(err.Error(), "read the milestones") {
			t.Errorf("err = %v, want the read reported", err)
		}
	})
	t.Run("slices that cannot be read", func(t *testing.T) {
		l, _ := openPlan(t)
		fillPlan(t, l)
		write(t, l, `DELETE FROM slice_deps`)
		write(t, l, `DROP TABLE slices`)
		_, err := l.RemoveMilestone(context.Background(), Project{}, wholeShape, "M1: The format")
		if err == nil || !strings.Contains(err.Error(), "read the slices") {
			t.Errorf("err = %v, want the read reported", err)
		}
	})
	t.Run("a milestone that cannot be removed", func(t *testing.T) {
		l, _ := openPlan(t)
		fillPlan(t, l)
		write(t, l, `UPDATE slices SET milestone = ? WHERE milestone = ?`, "M2: Reads", "M1: The format")
		write(t, l, `CREATE TRIGGER refuse BEFORE DELETE ON milestones
			BEGIN SELECT RAISE(ABORT, 'no removals here'); END`)
		_, err := l.RemoveMilestone(context.Background(), Project{}, wholeShape, "M1: The format")
		if err == nil || !strings.Contains(err.Error(), "remove the milestone") {
			t.Errorf("err = %v, want the write reported", err)
		}
		unchanged(t, l)
	})
	t.Run("a plan whose order cannot be closed up", func(t *testing.T) {
		l, _ := openPlan(t)
		fillPlan(t, l)
		write(t, l, `UPDATE slices SET milestone = ? WHERE milestone = ?`, "M2: Reads", "M1: The format")
		write(t, l, `CREATE TRIGGER refuse BEFORE UPDATE ON milestones
			BEGIN SELECT RAISE(ABORT, 'no reordering here'); END`)
		_, err := l.RemoveMilestone(context.Background(), Project{}, wholeShape, "M1: The format")
		if err == nil || !strings.Contains(err.Error(), "close the milestone's place in the plan") {
			t.Errorf("err = %v, want the write reported", err)
		}
		// One transaction: the milestone the delete took is back.
		unchanged(t, l)
	})
}

// A move rewrites the plan's order and nothing else: every milestone keeps its
// name and its slices, and the positions are restamped densely from zero, so the
// next milestone added still lands at the end rather than on top of one already
// there.
func TestLocalMoveMilestone(t *testing.T) {
	tests := []struct {
		name, target string
		before       bool
		plan         []string
		order, to    float64
	}{
		{
			name: "before an earlier milestone", target: "M1: The format", before: true,
			plan: []string{"M3: Sync", "M1: The format", "M2: Reads"}, order: 0, to: 1,
		},
		{
			name: "after an earlier milestone", target: "M1: The format", before: false,
			plan: []string{"M1: The format", "M3: Sync", "M2: Reads"}, order: 1, to: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, _ := openPlan(t)
			fillPlan(t, l)
			ctx := context.Background()
			write(t, l, `INSERT INTO milestones (name, position) VALUES (?, ?)`, "M3: Sync", 2)

			m, to, err := l.MoveMilestone(ctx, Project{}, wholeShape, "  m3: sync  ", tt.target, tt.before)
			if err != nil {
				t.Fatalf("MoveMilestone: %v", err)
			}
			// No status: a milestone has none of its own and a move reads no slices.
			want := domain.Milestone{ID: "M3: Sync", Name: "M3: Sync", Order: tt.order}
			if m != want {
				t.Errorf("milestone = %+v, want %+v", m, want)
			}
			if to.Name != tt.target || to.Order != tt.to {
				t.Errorf("relative to = %+v, want %s at %v", to, tt.target, tt.to)
			}

			sh, err := l.Shape(ctx, Project{})
			if err != nil {
				t.Fatalf("Shape: %v", err)
			}
			var plan []string
			for i, held := range sh.Milestones {
				plan = append(plan, held.Name)
				if held.Order != float64(i) {
					t.Errorf("milestone %+v, want it at position %d", held, i)
				}
			}
			if !reflect.DeepEqual(plan, tt.plan) {
				t.Errorf("plan = %v, want %v", plan, tt.plan)
			}
			// Nothing was refiled: the slices are where the plan left them.
			s, _, err := l.Slice(ctx, "reads")
			if err != nil {
				t.Fatalf("Slice: %v", err)
			}
			if s.MilestoneID != "M2: Reads" {
				t.Errorf("slice filed under %q, want it untouched", s.MilestoneID)
			}
			// Densely from zero, which is what the next one added counts on.
			added, err := l.AddMilestones(ctx, Project{}, wholeShape, []string{"M4: The app"})
			if err != nil {
				t.Fatalf("AddMilestones: %v", err)
			}
			if added[0].Order != 3 {
				t.Errorf("added milestone = %+v, want it at the end of the plan", added[0])
			}
		})
	}
}

func TestLocalMoveMilestoneRefusals(t *testing.T) {
	unchanged := func(t *testing.T, l *Local) {
		t.Helper()
		sh, err := l.Shape(context.Background(), Project{})
		if err != nil {
			t.Fatalf("Shape: %v", err)
		}
		if len(sh.Milestones) != 2 || sh.Milestones[0].Name != "M1: The format" {
			t.Errorf("milestones = %+v, want the refused run to have written nothing", sh.Milestones)
		}
	}
	t.Run("a name the plan does not hold", func(t *testing.T) {
		l, path := openPlan(t)
		fillPlan(t, l)
		_, _, err := l.MoveMilestone(context.Background(), Project{}, wholeShape,
			" M9: Nothing ", "M1: The format", true)
		if err == nil || !strings.Contains(err.Error(), `no milestone named "M9: Nothing"`) {
			t.Fatalf("err = %v, want the missing name refused", err)
		}
		if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), `"M2: Reads"`) {
			t.Errorf("err = %q, want the file and the plan named", err)
		}
		unchanged(t, l)
	})
	t.Run("a target the plan does not hold", func(t *testing.T) {
		l, path := openPlan(t)
		fillPlan(t, l)
		_, _, err := l.MoveMilestone(context.Background(), Project{}, wholeShape,
			"M1: The format", " M9: Nothing ", false)
		if err == nil || !strings.Contains(err.Error(), `no milestone named "M9: Nothing"`) {
			t.Fatalf("err = %v, want the missing target refused", err)
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("err = %q, want the file named", err)
		}
		unchanged(t, l)
	})
	t.Run("a move relative to itself", func(t *testing.T) {
		l, path := openPlan(t)
		fillPlan(t, l)
		_, _, err := l.MoveMilestone(context.Background(), Project{}, wholeShape,
			"M2: Reads", " m2: reads ", true)
		if err == nil || !strings.Contains(err.Error(), `"M2: Reads" in the plan at `) {
			t.Fatalf("err = %v, want the self-move refused", err)
		}
		if !strings.Contains(err.Error(), path) ||
			!strings.Contains(err.Error(), "name the milestone it is to sit beside") {
			t.Errorf("err = %q, want the file named and the way out said", err)
		}
		unchanged(t, l)
	})
	t.Run("a plan that cannot be read", func(t *testing.T) {
		l, _ := openPlan(t)
		fillPlan(t, l)
		write(t, l, `DROP TABLE milestones`)
		_, _, err := l.MoveMilestone(context.Background(), Project{}, wholeShape,
			"M2: Reads", "M1: The format", true)
		if err == nil || !strings.Contains(err.Error(), "read the milestones") {
			t.Errorf("err = %v, want the read reported", err)
		}
	})
	t.Run("a plan whose order cannot be rewritten", func(t *testing.T) {
		l, _ := openPlan(t)
		fillPlan(t, l)
		write(t, l, `CREATE TRIGGER refuse BEFORE UPDATE ON milestones
			BEGIN SELECT RAISE(ABORT, 'no reordering here'); END`)
		_, _, err := l.MoveMilestone(context.Background(), Project{}, wholeShape,
			"M2: Reads", "M1: The format", true)
		if err == nil || !strings.Contains(err.Error(), "reorder the plan") {
			t.Errorf("err = %v, want the write reported", err)
		}
		// One transaction: whatever it had already restamped is back.
		unchanged(t, l)
	})
}
