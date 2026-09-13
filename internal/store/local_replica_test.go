package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/domain"
)

// rawSliceRow is a slice's row exactly as the file holds it, columns Slice
// and Plan never read — the replica columns this slice adds — so a test can
// say something about them without a second public method to read them back
// through.
type rawSliceRow struct {
	assignee, assigneeName string
	position               float64
	bodyAt                 sql.NullString
}

func readRawSlice(t *testing.T, l *Local, id string) rawSliceRow {
	t.Helper()
	var r rawSliceRow
	err := l.db.QueryRow(
		`SELECT assignee, assignee_name, position, body_at FROM slices WHERE id = ?`, id).
		Scan(&r.assignee, &r.assigneeName, &r.position, &r.bodyAt)
	if err != nil {
		t.Fatalf("read the raw row for %s: %v", id, err)
	}
	return r
}

// rawProjectRow is the project's row exactly as the file holds it, the same
// trick for the project's own replica columns.
type rawProjectRow struct {
	hasAssignee, hasBranch bool
	syncedAt, conventionAt sql.NullString
}

func readRawProject(t *testing.T, l *Local, id string) rawProjectRow {
	t.Helper()
	var r rawProjectRow
	var ha, hb int
	err := l.db.QueryRow(
		`SELECT has_assignee, has_branch, synced_at, conventions_at FROM project WHERE id = ?`, id).
		Scan(&ha, &hb, &r.syncedAt, &r.conventionAt)
	if err != nil {
		t.Fatalf("read the raw project row: %v", err)
	}
	r.hasAssignee, r.hasBranch = ha != 0, hb != 0
	return r
}

func readMilestoneSelectType(t *testing.T, l *Local, name string) string {
	t.Helper()
	var st string
	if err := l.db.QueryRow(`SELECT select_type FROM milestones WHERE name = ?`, name).Scan(&st); err != nil {
		t.Fatalf("read the milestone's select type: %v", err)
	}
	return st
}

// TestLocalMigratesAV1PlanInPlace builds a plan in the shape the first build of
// this store wrote — no replica columns at all — the way a real one on disk
// from before this slice would read, and opens it with today's build.
func TestLocalMigratesAV1PlanInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.db")
	db, err := sql.Open("sqlite3", localDSN(path))
	if err != nil {
		t.Fatalf("open the raw file: %v", err)
	}
	if _, err := db.Exec(localSchemaV1 + "\nPRAGMA user_version = 1;\n"); err != nil {
		t.Fatalf("write the v1 schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO project (id, name) VALUES (?, ?)`, "proj", "v1 plan"); err != nil {
		t.Fatalf("seed the project: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO milestones (name, position) VALUES (?, ?)`, "M1", 0); err != nil {
		t.Fatalf("seed a milestone: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO slices (id, title, status, milestone, position, assignee) VALUES (?, ?, ?, ?, ?, ?)`,
		"s1", "Slice one", "Todo", "M1", 0, "Craig Johnston"); err != nil {
		t.Fatalf("seed a claimed slice: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO slices (id, title, status, position) VALUES (?, ?, ?, ?)`,
		"s2", "Slice two, unclaimed", "Todo", 1); err != nil {
		t.Fatalf("seed an unclaimed slice: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close the raw file: %v", err)
	}

	l, err := OpenLocal(path)
	if err != nil {
		t.Fatalf("open the v1 plan: %v", err)
	}
	defer func() { _ = l.Close() }()

	var version int
	if err := l.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != localSchemaVersion {
		t.Errorf("user_version = %d, want %d", version, localSchemaVersion)
	}

	claimed := readRawSlice(t, l, "s1")
	if claimed.assigneeName != "Craig Johnston" {
		t.Errorf("assignee_name = %q, want it back-filled from assignee", claimed.assigneeName)
	}
	unclaimed := readRawSlice(t, l, "s2")
	if unclaimed.assigneeName != "" {
		t.Errorf("assignee_name = %q, want nothing back-filled for a slice with none", unclaimed.assigneeName)
	}

	// Reading the plan through the store still works, exactly as it did before
	// this migration existed.
	plan, err := l.Plan(context.Background(), Project{ID: "proj"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Project.Slices) != 2 {
		t.Errorf("slices = %d, want the two migrated in", len(plan.Project.Slices))
	}
}

// A build reading a plan already at this version does nothing to it: opening
// twice is a read the second time, which is what migrate's version check is
// for and this asks of the replica columns specifically.
func TestLocalOpeningAV2PlanTwiceChangesNothing(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	at := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	if err := l.MarkSent(context.Background(), "writes", at); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	again, err := OpenLocal(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = again.Close() }()
	synced, ok, err := again.LastSynced(context.Background(), "writes")
	if err != nil || !ok || !synced.Equal(at) {
		t.Errorf("LastSynced after reopening = %v, %v, %v, want %v, true, nil", synced, ok, err, at)
	}
}

// A write [Local] already has marks the slice it touched dirty inside its own
// transaction, and [Local.MarkSent] is what clears it — the whole of the
// dirty flag's story, apart from a pull, which [TestLocalHydrateFreshPlan]
// and its siblings say about instead.
func TestLocalWritesMarkTheSliceDirty(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	if dirty, err := l.Dirty(ctx, "writes"); err != nil || dirty {
		t.Fatalf("Dirty before any write = %v, %v, want false", dirty, err)
	}
	if _, ok, err := l.LastSynced(ctx, "writes"); err != nil || ok {
		t.Fatalf("LastSynced before any write = ok %v, err %v, want false", ok, err)
	}

	if _, err := l.ClaimSlice(ctx, "writes", wholeShape, "Craig Johnston"); err != nil {
		t.Fatalf("ClaimSlice: %v", err)
	}
	if dirty, err := l.Dirty(ctx, "writes"); err != nil || !dirty {
		t.Errorf("Dirty after ClaimSlice = %v, %v, want true", dirty, err)
	}

	at := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	if err := l.MarkSent(ctx, "writes", at); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	if dirty, err := l.Dirty(ctx, "writes"); err != nil || dirty {
		t.Errorf("Dirty after MarkSent = %v, %v, want false", dirty, err)
	}
	synced, ok, err := l.LastSynced(ctx, "writes")
	if err != nil || !ok || !synced.Equal(at) {
		t.Errorf("LastSynced = %v, %v, %v, want %v, true, nil", synced, ok, err, at)
	}

	// A write can dirty a slice that has already been synced once.
	if _, err := l.ReleaseSlice(ctx, "writes", wholeShape, "Craig Johnston"); err != nil {
		t.Fatalf("ReleaseSlice: %v", err)
	}
	if dirty, err := l.Dirty(ctx, "writes"); err != nil || !dirty {
		t.Errorf("Dirty after a second write = %v, %v, want true again", dirty, err)
	}
}

// MarkSent is a step of its own precisely so a stamp of nothing — a push that
// carried no useful moment — clears dirty without pretending to know when the
// push happened.
func TestLocalMarkSentWithAZeroTimeRecordsNoStamp(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	if err := l.MarkSent(ctx, "writes", time.Time{}); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	if dirty, err := l.Dirty(ctx, "writes"); err != nil || dirty {
		t.Errorf("Dirty after MarkSent = %v, %v, want false", dirty, err)
	}
	if _, ok, err := l.LastSynced(ctx, "writes"); err != nil || ok {
		t.Errorf("LastSynced after a zero-time MarkSent = ok %v, err %v, want false", ok, err)
	}
}

// MarkSent is refused for a slice the file has never held, the same rule
// every other slice write already applies.
func TestLocalMarkSentRefusesASliceNotInThePlan(t *testing.T) {
	l, path := openPlan(t)
	if err := l.MarkSent(context.Background(), "ghost", time.Now()); err == nil {
		t.Fatal("MarkSent on a missing slice: want an error")
	} else if !strings.Contains(err.Error(), "ghost") || !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the slice and the file named", err)
	}
}

// AddSlice needs a push like any other local edit: a slice filed under no
// milestone the workspace has never seen has to go there.
func TestLocalAddSliceMarksTheNewSliceDirty(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	added, err := l.AddSlice(ctx, Project{}, NewSlice{Title: "New", Milestone: domain.Milestone{ID: "M2: Reads"}})
	if err != nil {
		t.Fatalf("AddSlice: %v", err)
	}
	if dirty, err := l.Dirty(ctx, added.ID); err != nil || !dirty {
		t.Errorf("Dirty on a newly added slice = %v, %v, want true", dirty, err)
	}
}

// ApplyAssignee is narrower than every other write in the file: it changes
// one column, the workspace's own name for whoever the identity column
// already names, and it does not mark the slice dirty, because it is telling
// the file what the workspace has just agreed to rather than something the
// file is ahead of it on.
func TestLocalApplyAssigneeRecordsTheNameAndNothingElse(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	before := readBack(t, l, "design")
	if err := l.ApplyAssignee(ctx, "design", "Craig Johnston (workspace)"); err != nil {
		t.Fatalf("ApplyAssignee: %v", err)
	}

	got := readRawSlice(t, l, "design")
	if got.assigneeName != "Craig Johnston (workspace)" {
		t.Errorf("assignee_name = %q, want the workspace's own name written", got.assigneeName)
	}
	if got.assignee != "Craig Johnston" {
		t.Errorf("assignee = %q, want the identity column untouched", got.assignee)
	}

	// Slice() reads AssigneeName off the identity column alone, so nothing
	// this write touched shows up there — which is exactly the point: this
	// slice adds the column without changing anything that already reads it.
	if after := readBack(t, l, "design"); !reflect.DeepEqual(before, after) {
		t.Errorf("slice = %+v, want nothing the existing reads see to have changed", after)
	}
	if dirty, err := l.Dirty(ctx, "design"); err != nil || dirty {
		t.Errorf("Dirty after ApplyAssignee = %v, %v, want it to leave the slice clean", dirty, err)
	}
}

func TestLocalApplyAssigneeRefusesASliceNotInThePlan(t *testing.T) {
	l, path := openPlan(t)
	ctx := context.Background()
	if err := l.ApplyAssignee(ctx, "ghost", "Someone"); err == nil {
		t.Fatal("ApplyAssignee on a missing slice: want an error")
	} else if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}
}

// SetBody is Body's write half: the same slice-or-project duality, and a
// stamp on whichever it wrote, which is what [Local.BodyFresh] reads back.
func TestLocalSetBodyWritesASliceOrAProjectAndStampsIt(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()
	at := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)

	if err := l.SetBody(ctx, "writes", "Fresh from the workspace.", at); err != nil {
		t.Fatalf("SetBody on a slice: %v", err)
	}
	if got := body(t, l, "writes"); got != "Fresh from the workspace." {
		t.Errorf("body = %q", got)
	}
	if fresh, err := l.BodyFresh(ctx, "writes", at); err != nil || !fresh {
		t.Errorf("BodyFresh at the stamp itself = %v, %v, want true", fresh, err)
	}
	if fresh, err := l.BodyFresh(ctx, "writes", at.Add(time.Minute)); err != nil || fresh {
		t.Errorf("BodyFresh after the stamp = %v, %v, want false", fresh, err)
	}

	if err := l.SetBody(ctx, "proj", "New conventions.", at); err != nil {
		t.Fatalf("SetBody on the project: %v", err)
	}
	if got := body(t, l, "proj"); got != "New conventions." {
		t.Errorf("conventions = %q", got)
	}
	if fresh, err := l.BodyFresh(ctx, "proj", at); err != nil || !fresh {
		t.Errorf("BodyFresh(proj) = %v, %v, want true", fresh, err)
	}
}

func TestLocalSetBodyRefusesAnIDNeitherASliceNorAProject(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	err := l.SetBody(ctx, "ghost", "text", time.Now())
	if err == nil {
		t.Fatal("SetBody on neither a slice nor a project: want an error")
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error = %q, want the path and the ID named", err)
	}
}

func TestLocalBodyFreshForAPageNeverStamped(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	if fresh, err := l.BodyFresh(ctx, "design", time.Now()); err != nil || fresh {
		t.Errorf("BodyFresh on a never-stamped slice = %v, %v, want false", fresh, err)
	}
	if fresh, err := l.BodyFresh(ctx, "proj", time.Now()); err != nil || fresh {
		t.Errorf("BodyFresh on never-stamped conventions = %v, %v, want false", fresh, err)
	}
	if fresh, err := l.BodyFresh(ctx, "ghost", time.Now()); err != nil || fresh {
		t.Errorf("BodyFresh on an ID nothing answers to = %v, %v, want false", fresh, err)
	}
}

// A stamp that will not parse is a corrupt plan the same as a row that will
// not scan, and is reported the same way.
func TestLocalReportsAStampItCannotParse(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	write(t, l, `UPDATE slices SET body_at = 'not a time' WHERE id = 'design'`)
	if _, err := l.BodyFresh(ctx, "design", time.Now()); err == nil {
		t.Fatal("BodyFresh over a stamp that will not parse: want an error")
	} else if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}

	write(t, l, `INSERT INTO sync (slice_id, dirty, synced_at) VALUES ('design', 0, 'not a time')`)
	if _, _, err := l.LastSynced(ctx, "design"); err == nil {
		t.Fatal("LastSynced over a stamp that will not parse: want an error")
	} else if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}
}

// BodyFresh's second read — the project, once the slices table says an ID is
// not one of theirs — names the file exactly as the first does.
func TestLocalBodyFreshNamesItsFileWhenTheProjectReadFails(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	write(t, l, `DROP TABLE project`)
	ctx := context.Background()

	if _, err := l.BodyFresh(ctx, "neither", time.Now()); err == nil {
		t.Fatal("BodyFresh with no project table: want an error")
	} else if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}
}

// TakeSlice is the one way a slice enters the file outside Hydrate: it files
// one the plan has never held at the end of its milestone, clean and stamped
// as of when it was read.
func TestLocalTakeSliceFilesANewSliceAtTheEndOfItsMilestone(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()
	at := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)

	s := domain.Slice{
		ID: "taken", Name: "Taken from the workspace", Status: domain.SliceTodo, StatusName: "Todo",
		MilestoneID: "M2: Reads", AssigneeIDs: []string{"user-1"}, AssigneeName: "Someone",
	}
	if err := l.TakeSlice(ctx, s, "Its brief.", at); err != nil {
		t.Fatalf("TakeSlice: %v", err)
	}

	got := readBack(t, l, "taken")
	if got.Name != "Taken from the workspace" || got.MilestoneID != "M2: Reads" {
		t.Errorf("slice = %+v", got)
	}
	if b := body(t, l, "taken"); b != "Its brief." {
		t.Errorf("body = %q", b)
	}
	writesPos := readRawSlice(t, l, "writes").position
	takenRaw := readRawSlice(t, l, "taken")
	if takenRaw.position <= writesPos {
		t.Errorf("position = %v, want it appended past what M2: Reads already holds (%v)", takenRaw.position, writesPos)
	}
	if takenRaw.assignee != "user-1" || takenRaw.assigneeName != "Someone" {
		t.Errorf("identity = %+v, want the reading's own", takenRaw)
	}
	if dirty, err := l.Dirty(ctx, "taken"); err != nil || dirty {
		t.Errorf("Dirty on a slice just taken from the workspace = %v, %v, want clean", dirty, err)
	}
	synced, ok, err := l.LastSynced(ctx, "taken")
	if err != nil || !ok || !synced.Equal(at) {
		t.Errorf("LastSynced = %v, %v, %v, want %v, true, nil", synced, ok, err, at)
	}
}

// A slice new to no milestone at all still has somewhere to land.
func TestLocalTakeSliceFilesANewSliceUnderNoMilestone(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	s := domain.Slice{ID: "loose", Name: "No milestone", Status: domain.SliceTodo, StatusName: "Todo"}
	if err := l.TakeSlice(ctx, s, "", time.Now()); err != nil {
		t.Fatalf("TakeSlice: %v", err)
	}
	got := readBack(t, l, "loose")
	if got.MilestoneID != "" {
		t.Errorf("MilestoneID = %q, want none", got.MilestoneID)
	}
}

// A slice the file already holds is not new to it: TakeSlice leaves it
// exactly as it was, whatever the reading that prompted the call says.
func TestLocalTakeSliceLeavesASliceItAlreadyHolds(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	before := readBack(t, l, "writes")
	beforeRaw := readRawSlice(t, l, "writes")
	s := domain.Slice{ID: "writes", Name: "A different name entirely", MilestoneID: "M1: The format"}
	if err := l.TakeSlice(ctx, s, "a different body", time.Now()); err != nil {
		t.Fatalf("TakeSlice on a known slice: %v", err)
	}
	after := readBack(t, l, "writes")
	if !reflect.DeepEqual(before, after) {
		t.Errorf("slice = %+v, want a slice the file already holds left exactly as it was", after)
	}
	if afterRaw := readRawSlice(t, l, "writes"); afterRaw.position != beforeRaw.position {
		t.Errorf("position = %v, want it unchanged at %v", afterRaw.position, beforeRaw.position)
	}
}

// Hydrate on a plan with nothing in it yet writes the whole reading: the
// project, its milestones and their select type, every slice clean and
// stamped, and any bodies the pull carried.
func TestLocalHydrateFreshPlanWritesEverything(t *testing.T) {
	l, _ := openPlan(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)

	reading := Plan{
		Project: domain.Project{
			ID:   "proj",
			Name: "A replica",
			Milestones: []domain.Milestone{
				{ID: "M1", Name: "M1", Order: 0, SelectType: "select"},
			},
			Slices: []domain.Slice{
				{ID: "a", Name: "First", Status: domain.SliceTodo, StatusName: "Todo", MilestoneID: "M1"},
				{
					ID: "b", Name: "Second", Status: domain.SliceTodo, StatusName: "Todo", MilestoneID: "M1",
					DependsOn: []string{"a"},
				},
			},
		},
		Shape: Shape{
			HasAssignee: true, HasBranch: true,
			Milestones: []domain.Milestone{{ID: "M1", Name: "M1", Order: 0, SelectType: "select"}},
		},
	}
	bodies := map[string]string{"proj": "Conventions.", "a": "Brief for a."}

	if err := l.Hydrate(ctx, Project{ID: "proj"}, reading, bodies, at); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	plan, err := l.Plan(ctx, Project{ID: "proj"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.Project.Name != "A replica" {
		t.Errorf("name = %q", plan.Project.Name)
	}
	if len(plan.Project.Milestones) != 1 || plan.Project.Milestones[0].Name != "M1" {
		t.Errorf("milestones = %+v", plan.Project.Milestones)
	}
	if st := readMilestoneSelectType(t, l, "M1"); st != "select" {
		t.Errorf("select_type = %q, want the reading's own", st)
	}
	wantSlices := []domain.Slice{
		{ID: "a", Name: "First", Status: domain.SliceTodo, StatusName: "Todo", MilestoneID: "M1"},
		{
			ID: "b", Name: "Second", Status: domain.SliceTodo, StatusName: "Todo", MilestoneID: "M1",
			DependsOn: []string{"a"},
		},
	}
	if !reflect.DeepEqual(plan.Project.Slices, wantSlices) {
		t.Errorf("slices = %+v, want %+v", plan.Project.Slices, wantSlices)
	}
	if got := body(t, l, "proj"); got != "Conventions." {
		t.Errorf("conventions = %q", got)
	}
	if got := body(t, l, "a"); got != "Brief for a." {
		t.Errorf("body of a = %q", got)
	}
	if got := body(t, l, "b"); got != "" {
		t.Errorf("body of b (no body given) = %q, want empty", got)
	}
	if dirty, err := l.Dirty(ctx, "a"); err != nil || dirty {
		t.Errorf("Dirty(a) after Hydrate = %v, %v, want false", dirty, err)
	}
	synced, ok, err := l.LastSynced(ctx, "a")
	if err != nil || !ok || !synced.Equal(at) {
		t.Errorf("LastSynced(a) = %v, %v, %v, want %v, true, nil", synced, ok, err, at)
	}
	row := readRawProject(t, l, "proj")
	if !row.hasAssignee || !row.hasBranch {
		t.Errorf("has_assignee/has_branch = %v/%v, want both true", row.hasAssignee, row.hasBranch)
	}
	if !row.syncedAt.Valid {
		t.Error("project.synced_at not set")
	}
	if !row.conventionAt.Valid {
		t.Error("project.conventions_at not set for a bodies entry that was given")
	}
}

// A Shape with neither column is the same write, the other way — a project
// this build reads from a workspace with no such columns at all.
func TestLocalHydrateRecordsAShapeWithNeitherColumn(t *testing.T) {
	l, _ := openPlan(t)
	ctx := context.Background()

	reading := Plan{Project: domain.Project{ID: "proj", Name: "Bare"}, Shape: Shape{}}
	if err := l.Hydrate(ctx, Project{ID: "proj"}, reading, nil, time.Now()); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	row := readRawProject(t, l, "proj")
	if row.hasAssignee || row.hasBranch {
		t.Errorf("has_assignee/has_branch = %v/%v, want both false", row.hasAssignee, row.hasBranch)
	}
}

// A re-pull updates a slice the file already holds, keeps the position it
// already has however the reading orders it, appends what is new to it past
// what a milestone already holds, drops what the reading no longer names, and
// leaves a dirty slice alone even where the reading changed it too.
func TestLocalHydrateRepullUpdatesReordersAndDrops(t *testing.T) {
	l, _ := openPlan(t)
	ctx := context.Background()
	at1 := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)

	milestones := []domain.Milestone{{ID: "M1", Name: "M1", Order: 0}}
	first := Plan{
		Project: domain.Project{
			ID: "proj", Name: "A replica", Milestones: milestones,
			Slices: []domain.Slice{
				{ID: "a", Name: "First", Status: domain.SliceTodo, StatusName: "Todo", MilestoneID: "M1"},
				{ID: "b", Name: "Second", Status: domain.SliceTodo, StatusName: "Todo", MilestoneID: "M1"},
				{ID: "c", Name: "Third", Status: domain.SliceTodo, StatusName: "Todo", MilestoneID: "M1"},
			},
		},
		Shape: Shape{HasAssignee: true, HasBranch: true, Milestones: milestones},
	}
	if err := l.Hydrate(ctx, Project{ID: "proj"}, first, nil, at1); err != nil {
		t.Fatalf("Hydrate (first): %v", err)
	}
	posB := readRawSlice(t, l, "b").position

	// "c" is worked locally since the pull, which is what makes it dirty.
	if _, err := l.ClaimSlice(ctx, "c", wholeShape, "Craig Johnston"); err != nil {
		t.Fatalf("ClaimSlice: %v", err)
	}
	dirtyBefore := readBack(t, l, "c")

	at2 := at1.Add(time.Hour)
	second := Plan{
		Project: domain.Project{
			ID: "proj", Name: "A replica", Milestones: milestones,
			Slices: []domain.Slice{
				// Reversed from the first reading, with "a" dropped, "b" changed,
				// "c" changed too even though the file is ahead of it, and a new
				// slice "d".
				{ID: "d", Name: "New from the workspace", Status: domain.SliceTodo, StatusName: "Todo", MilestoneID: "M1"},
				{ID: "c", Name: "Third, renamed upstream", Status: domain.SliceTodo, StatusName: "Todo", MilestoneID: "M1"},
				{ID: "b", Name: "Second, renamed", Status: domain.SliceClaimed, StatusName: "In progress", MilestoneID: "M1"},
			},
		},
		Shape: Shape{HasAssignee: true, HasBranch: true, Milestones: milestones},
	}
	if err := l.Hydrate(ctx, Project{ID: "proj"}, second, nil, at2); err != nil {
		t.Fatalf("Hydrate (second): %v", err)
	}

	if _, _, err := l.Slice(ctx, "a"); err == nil {
		t.Error("Slice(a) after a re-pull that drops it: want it gone")
	}

	gotB := readBack(t, l, "b")
	if gotB.Name != "Second, renamed" || gotB.Status != domain.SliceClaimed {
		t.Errorf("b = %+v, want the reading's own fields", gotB)
	}
	if got := readRawSlice(t, l, "b").position; got != posB {
		t.Errorf("position(b) = %v, want it unchanged at %v", got, posB)
	}

	gotC := readBack(t, l, "c")
	if !reflect.DeepEqual(gotC, dirtyBefore) {
		t.Errorf("c = %+v, want a dirty slice left exactly as it was (%+v)", gotC, dirtyBefore)
	}
	if dirty, err := l.Dirty(ctx, "c"); err != nil || !dirty {
		t.Errorf("Dirty(c) = %v, %v, want it to still be dirty", dirty, err)
	}

	gotD := readBack(t, l, "d")
	if gotD.Name != "New from the workspace" {
		t.Errorf("d = %+v", gotD)
	}
	if got := readRawSlice(t, l, "d").position; got <= posB {
		t.Errorf("position(d) = %v, want it appended past b's own %v", got, posB)
	}
}

// A pull that carries no bodies at all leaves every page's prose exactly as
// it was, which is the ordinary shape of a pull: bodies are fetched one at a
// time and a plan re-pull is not what fetches them.
func TestLocalHydrateAbsentBodiesLeaveProseAlone(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	milestones := []domain.Milestone{
		{ID: "M1: The format", Name: "M1: The format", Order: 0},
		{ID: "M2: Reads", Name: "M2: Reads", Order: 1},
	}
	reading := Plan{
		Project: domain.Project{
			ID: "proj", Name: "notion-agent-tracker", Milestones: milestones,
			Slices: []domain.Slice{
				{
					ID: "design", Name: "Design the local plan format",
					Status: domain.SliceDone, StatusName: "Done", MilestoneID: "M1: The format",
					PRURL: "https://example.test/pr/1",
				},
				{
					ID: "reads", Name: "Implement the local store: reads",
					Status: domain.SliceClaimed, StatusName: "In progress", MilestoneID: "M2: Reads",
					Repo: "/tmp/repo", Branch: "slice/reads",
				},
				{
					ID: "writes", Name: "Implement the local store: writes",
					Status: domain.SliceTodo, StatusName: "Todo", MilestoneID: "M2: Reads",
					DependsOn: []string{"reads", "design"},
				},
				{ID: "stray", Name: "A slice under no milestone", Status: domain.SliceTodo, StatusName: "Todo"},
			},
		},
		Shape: Shape{HasAssignee: true, HasBranch: true, Milestones: milestones},
	}
	if err := l.Hydrate(ctx, Project{ID: "proj"}, reading, nil, time.Now()); err != nil {
		t.Fatalf("Hydrate with no bodies at all: %v", err)
	}
	if got := body(t, l, "design"); got != "Settle the format." {
		t.Errorf("body(design) = %q, want the prose already on the page left alone", got)
	}
	if got := body(t, l, "proj"); got != "# Conventions\n\nBranch per slice." {
		t.Errorf("conventions = %q, want them left alone", got)
	}
}

// The reads Dirty, LastSynced and BodyFresh take outside a transaction each
// name the file the same way every other read here does.
func TestLocalReplicaReadsNameTheirFileWhenTheyFail(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	ctx := context.Background()

	reads := map[string]func() error{
		"Dirty":      func() error { _, err := l.Dirty(ctx, "writes"); return err },
		"LastSynced": func() error { _, _, err := l.LastSynced(ctx, "writes"); return err },
		"BodyFresh":  func() error { _, err := l.BodyFresh(ctx, "writes", time.Now()); return err },
	}
	for name, read := range reads {
		err := read()
		if err == nil {
			t.Errorf("%s on a closed plan: want an error", name)
			continue
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("%s error = %q, want the path named", name, err)
		}
	}
}

// Every write [Local] already has marks the slice it touched dirty inside its
// own transaction — the sync row it writes is one more row a hand-edited plan
// can refuse, and that refusal names the file the same way every other one
// does.
func TestLocalWritesNameTheirFileWhenTheSyncRowIsRefused(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	write(t, l, `DROP TABLE sync`)
	write(t, l, `CREATE VIEW sync (slice_id, dirty, synced_at) AS SELECT 'x', 0, NULL`)
	ctx := context.Background()

	writes := map[string]func() error{
		"ClaimSlice": func() error { _, err := l.ClaimSlice(ctx, "writes", wholeShape, "u"); return err },
		"AddSlice": func() error {
			_, err := l.AddSlice(ctx, Project{}, NewSlice{Title: "x", Milestone: domain.Milestone{ID: "M2: Reads"}})
			return err
		},
	}
	for name, w := range writes {
		err := w()
		if err == nil {
			t.Errorf("%s: want the sync row it marks dirty refused", name)
			continue
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("%s: error = %q, want the file named", name, err)
		}
	}
}

// A slice with no StatusName at all — a reading that carries a workflow
// status and nothing of the project's own word for it — still writes a
// status: the workflow one.
func TestLocalTakeSliceIntoAMilestoneWithNothingFiledYet(t *testing.T) {
	l, _ := openPlan(t)
	ctx := context.Background()

	s := domain.Slice{ID: "first", MilestoneID: "M1", Status: domain.SliceTodo}
	if err := l.TakeSlice(ctx, s, "", time.Now()); err != nil {
		t.Fatalf("TakeSlice into an empty milestone: %v", err)
	}
	if got := readRawSlice(t, l, "first").position; got != 0 {
		t.Errorf("position = %v, want 0 for the first slice filed under a milestone", got)
	}
	if got := readBack(t, l, "first").StatusName; got != "Todo" {
		t.Errorf("StatusName = %q, want the workflow status used as the project's own word", got)
	}
}

// Every read [Local.TakeSlice] takes inside its own transaction fails the
// same way every other write's own reads do.
func TestLocalTakeSliceNamesItsFileWhenItsOwnReadsFail(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func(t *testing.T, l *Local){
		"whether the slice is already held": func(t *testing.T, l *Local) {
			write(t, l, `DROP TABLE slice_deps`)
			write(t, l, `DROP TABLE sync`)
			write(t, l, `DROP TABLE slices`)
		},
		"the end of the milestone it is filed at": func(t *testing.T, l *Local) {
			write(t, l, `ALTER TABLE slices RENAME TO slices_data`)
			write(t, l, `CREATE VIEW slices
				(id, title, status, milestone, position, assignee, assignee_name, repo, branch, pr, body, body_at)
				AS SELECT id, title, status, milestone, abs(-9223372036854775808),
				assignee, assignee_name, repo, branch, pr, body, body_at FROM slices_data`)
		},
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			l, path := openPlan(t)
			fillPlan(t, l)
			breakIt(t, l)
			s := domain.Slice{ID: "new-one", MilestoneID: "M2: Reads"}
			if err := l.TakeSlice(ctx, s, "", time.Now()); err == nil {
				t.Fatalf("TakeSlice with %s broken: want an error", name)
			} else if !strings.Contains(err.Error(), path) {
				t.Errorf("error = %q, want the path named", err)
			}
		})
	}
}

// A plan hand-edited into something readable and not writable refuses
// TakeSlice's own writes the same way every other write here is refused.
func TestLocalTakeSliceNamesItsFileWhenAWriteIsRefused(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func(t *testing.T, l *Local){
		"the row it inserts": func(t *testing.T, l *Local) {
			write(t, l, `ALTER TABLE slices RENAME TO slices_data`)
			write(t, l, `CREATE VIEW slices
				(id, title, status, milestone, position, assignee, assignee_name, repo, branch, pr, body, body_at)
				AS SELECT id, title, status, milestone, position, assignee, assignee_name, repo, branch, pr, body, body_at
				FROM slices_data`)
		},
		"the waits it is given": func(t *testing.T, l *Local) {
			write(t, l, `ALTER TABLE slice_deps RENAME TO slice_deps_data`)
			write(t, l, `CREATE VIEW slice_deps (slice_id, depends_on, position)
				AS SELECT slice_id, depends_on, position FROM slice_deps_data`)
		},
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			l, path := openPlan(t)
			fillPlan(t, l)
			breakIt(t, l)
			s := domain.Slice{ID: "new-one", MilestoneID: "M2: Reads", DependsOn: []string{"design"}}
			if err := l.TakeSlice(ctx, s, "", time.Now()); err == nil {
				t.Fatalf("TakeSlice with %s refused: want an error", name)
			} else if !strings.Contains(err.Error(), path) {
				t.Errorf("error = %q, want the file named", name)
			}
		})
	}
}

// The existing slices Hydrate reads before it writes anything fail the same
// two ways every other read of the slices table does: a row that will not
// scan, and one that fails once the read is under way.
func TestLocalHydrateNamesItsFileWhenTheExistingSlicesWillNotScan(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	write(t, l, `UPDATE slices SET position = 'not a number' WHERE id = 'design'`)
	ctx := context.Background()

	reading := Plan{Project: domain.Project{ID: "proj", Name: "x"}}
	if err := l.Hydrate(ctx, Project{ID: "proj"}, reading, nil, time.Now()); err == nil {
		t.Fatal("Hydrate over a position that will not scan: want an error")
	} else if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}
}

func TestLocalHydrateNamesItsFileWhenTheExistingSlicesFailPartWayThrough(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	write(t, l, `ALTER TABLE slices RENAME TO slices_data`)
	write(t, l, `CREATE VIEW slices
		(id, title, status, milestone, position, assignee, assignee_name, repo, branch, pr, body, body_at)
		AS SELECT 'a', 'A', 'Todo', NULL, 0.0, '', '', '', '', '', '', NULL
		UNION ALL SELECT 'b', 'B', 'Todo', NULL, abs(-9223372036854775808), '', '', '', '', '', '', NULL`)
	ctx := context.Background()

	reading := Plan{Project: domain.Project{ID: "proj", Name: "x"}}
	if err := l.Hydrate(ctx, Project{ID: "proj"}, reading, nil, time.Now()); err == nil {
		t.Fatal("Hydrate with the existing slices failing part way: want an error")
	} else if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}
}

// Every write Hydrate makes is refused the same way a hand-edited plan
// refuses every other write here — a table swapped for a read-only view where
// nothing earlier in the same transaction needs to write it too, and a
// trigger where something earlier does.
func TestLocalHydrateNamesItsFileWhenAWriteIsRefused(t *testing.T) {
	ctx := context.Background()
	unwritable := func(table, cols string) func(t *testing.T, l *Local) {
		return func(t *testing.T, l *Local) {
			write(t, l, `ALTER TABLE `+table+` RENAME TO `+table+`_data`)
			write(t, l, `CREATE VIEW `+table+` (`+cols+`) AS SELECT `+cols+` FROM `+table+`_data`)
		}
	}
	slicesCols := "id, title, status, milestone, position, assignee, assignee_name, repo, branch, pr, body, body_at"

	cases := []struct {
		name    string
		breakIt func(t *testing.T, l *Local)
		reading Plan
		bodies  map[string]string
	}{
		{
			name:    "the project row",
			breakIt: unwritable("project", "id, name, conventions, conventions_at, synced_at, has_assignee, has_branch"),
			reading: Plan{Project: domain.Project{ID: "proj", Name: "x"}},
		},
		{
			name: "the project's conventions",
			breakIt: func(t *testing.T, l *Local) {
				write(t, l, `CREATE TRIGGER project_no_conventions BEFORE UPDATE OF conventions ON project
					BEGIN SELECT RAISE(ABORT, 'conventions locked'); END`)
			},
			reading: Plan{Project: domain.Project{ID: "proj", Name: "x"}},
			bodies:  map[string]string{"proj": "new conventions"},
		},
		{
			name: "a milestone it re-creates",
			breakIt: func(t *testing.T, l *Local) {
				write(t, l, `CREATE TRIGGER milestones_no_insert BEFORE INSERT ON milestones
					BEGIN SELECT RAISE(ABORT, 'no inserts'); END`)
			},
			reading: Plan{Project: domain.Project{
				ID: "proj", Name: "x",
				Milestones: []domain.Milestone{{ID: "M9", Name: "M9", Order: 0}},
			}},
		},
		{
			name:    "a slice it writes over",
			breakIt: unwritable("slices", slicesCols),
			reading: Plan{Project: domain.Project{
				ID: "proj", Name: "x",
				Slices: []domain.Slice{{ID: "brand-new", Name: "New", Status: domain.SliceTodo, StatusName: "Todo"}},
			}},
		},
		{
			name: "a slice's body",
			breakIt: func(t *testing.T, l *Local) {
				write(t, l, `CREATE TRIGGER slices_no_body BEFORE UPDATE OF body ON slices
					BEGIN SELECT RAISE(ABORT, 'body locked'); END`)
			},
			reading: Plan{Project: domain.Project{
				ID: "proj", Name: "x",
				Slices: []domain.Slice{{ID: "brand-new", Name: "New", Status: domain.SliceTodo, StatusName: "Todo"}},
			}},
			bodies: map[string]string{"brand-new": "its brief"},
		},
		{
			name:    "the waits a slice records",
			breakIt: unwritable("slice_deps", "slice_id, depends_on, position"),
			reading: Plan{Project: domain.Project{
				ID: "proj", Name: "x",
				Slices: []domain.Slice{{ID: "design", Name: "d", Status: domain.SliceDone, StatusName: "Done"}},
			}},
		},
		{
			name:    "a slice's sync state",
			breakIt: unwritable("sync", "slice_id, dirty, synced_at"),
			reading: Plan{Project: domain.Project{
				ID: "proj", Name: "x",
				Slices: []domain.Slice{{ID: "brand-new", Name: "New", Status: domain.SliceTodo, StatusName: "Todo"}},
			}},
		},
		{
			name:    "a slice the reading drops",
			breakIt: unwritable("slice_deps", "slice_id, depends_on, position"),
			reading: Plan{Project: domain.Project{ID: "proj", Name: "x"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, path := openPlan(t)
			fillPlan(t, l)
			c.breakIt(t, l)
			if err := l.Hydrate(ctx, Project{ID: "proj"}, c.reading, c.bodies, time.Now()); err == nil {
				t.Fatalf("Hydrate with %s refused: want an error", c.name)
			} else if !strings.Contains(err.Error(), path) {
				t.Errorf("error = %q, want the file named", err)
			}
		})
	}
}

// Every read [Local.Hydrate] takes inside its own transaction fails the same
// way every other write's own reads do: naming the file.
func TestLocalHydrateNamesItsFileWhenItsOwnReadsFail(t *testing.T) {
	ctx := context.Background()
	reading := Plan{Project: domain.Project{ID: "proj", Name: "x"}, Shape: Shape{}}

	cases := map[string]func(t *testing.T, l *Local){
		"the milestones it replaces": func(t *testing.T, l *Local) { write(t, l, `DROP TABLE milestones`) },
		"the slices it already holds": func(t *testing.T, l *Local) {
			write(t, l, `DROP TABLE sync`)
		},
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			l, path := openPlan(t)
			fillPlan(t, l)
			breakIt(t, l)
			if err := l.Hydrate(ctx, Project{ID: "proj"}, reading, nil, time.Now()); err == nil {
				t.Fatalf("Hydrate with %s broken: want an error", name)
			} else if !strings.Contains(err.Error(), path) {
				t.Errorf("error = %q, want the path named", err)
			}
		})
	}
}

// SetBody's second read — the project, once the slices table says an ID is
// not one of theirs — fails the same way the first does, and so does the
// exec each read's write is built on.
func TestLocalSetBodyNamesItsFileWhenAWriteIsRefused(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func(t *testing.T, l *Local){
		"the slice, tried first": func(t *testing.T, l *Local) {
			write(t, l, `ALTER TABLE slices RENAME TO slices_data`)
			write(t, l, `CREATE VIEW slices
				(id, title, status, milestone, position, assignee, assignee_name, repo, branch, pr, body, body_at)
				AS SELECT id, title, status, milestone, position, assignee, assignee_name, repo, branch, pr, body, body_at
				FROM slices_data`)
		},
		"the project, tried once the slice is not one": func(t *testing.T, l *Local) {
			write(t, l, `ALTER TABLE project RENAME TO project_data`)
			write(t, l, `CREATE VIEW project
				(id, name, conventions, conventions_at, synced_at, has_assignee, has_branch)
				AS SELECT id, name, conventions, conventions_at, synced_at, has_assignee, has_branch FROM project_data`)
		},
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			l, path := openPlan(t)
			fillPlan(t, l)
			breakIt(t, l)
			id := "writes"
			if name != "the slice, tried first" {
				id = "proj"
			}
			if err := l.SetBody(ctx, id, "text", time.Now()); err == nil {
				t.Fatalf("SetBody with %s refused: want an error", name)
			} else if !strings.Contains(err.Error(), path) {
				t.Errorf("error = %q, want the file named", name)
			}
		})
	}
}

// A database that will not answer at all fails every replica write the same
// way it already fails every other one.
func TestLocalNamesItsFileWhenAReplicaWriteCannotBeMade(t *testing.T) {
	l := &Local{db: brokenDB(t), path: "/plans/broken.db"}
	ctx := context.Background()

	writes := map[string]func() error{
		"TakeSlice": func() error {
			return l.TakeSlice(ctx, domain.Slice{ID: "x"}, "", time.Now())
		},
		"Hydrate": func() error {
			return l.Hydrate(ctx, Project{ID: "proj"}, Plan{Project: domain.Project{ID: "proj"}}, nil, time.Now())
		},
		"SetBody":       func() error { return l.SetBody(ctx, "x", "body", time.Now()) },
		"MarkSent":      func() error { return l.MarkSent(ctx, "x", time.Now()) },
		"ApplyAssignee": func() error { return l.ApplyAssignee(ctx, "x", "name") },
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
