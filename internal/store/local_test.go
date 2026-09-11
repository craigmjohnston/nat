package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
)

// openPlan opens a plan in a directory of the test's own, and closes it when
// the test ends. The path is returned as well as the store, since half of what
// this package promises about a local plan is said about its file.
func openPlan(t *testing.T) (*Local, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.db")
	l, err := OpenLocal(path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Errorf("close the plan: %v", err)
		}
	})
	return l, path
}

// write runs one statement against the plan, which is how a test fills a plan
// in while the write half of the store is still the next slice's work.
func write(t *testing.T, l *Local, query string, args ...any) {
	t.Helper()
	if _, err := l.db.Exec(query, args...); err != nil {
		t.Fatalf("write %q: %v", query, err)
	}
}

// fillPlan writes a plan with two milestones and four slices: one Done, one in
// progress and handed back, one Todo waiting on the hand-back, and one under no
// milestone at all. That is enough of a plan to say something about every read.
func fillPlan(t *testing.T, l *Local) {
	t.Helper()
	write(t, l, `INSERT INTO project (id, name, conventions) VALUES (?, ?, ?)`,
		"proj", "notion-agent-tracker", "# Conventions\n\nBranch per slice.")
	write(t, l, `INSERT INTO milestones (name, position) VALUES (?, ?), (?, ?)`,
		"M2: Reads", 1, "M1: The format", 0)
	write(t, l, `INSERT INTO slices (id, title, status, milestone, position, assignee, repo, branch, pr, body)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"design", "Design the local plan format", "Done", "M1: The format", 0,
		"Craig Johnston", "", "", "https://example.test/pr/1", "Settle the format.")
	write(t, l, `INSERT INTO slices (id, title, status, milestone, position, assignee, repo, branch, pr, body)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"reads", "Implement the local store: reads", "In progress", "M2: Reads", 0,
		"Craig Johnston", "/tmp/repo", "slice/reads", "", "Read the plan.\n\n## PR description\n\nRead a local plan\n\nWhat it does.")
	write(t, l, `INSERT INTO slices (id, title, status, milestone, position, body)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"writes", "Implement the local store: writes", "Todo", "M2: Reads", 1, "Write the plan.")
	write(t, l, `INSERT INTO slices (id, title, status, position, body)
		VALUES (?, ?, ?, ?, ?)`,
		"stray", "A slice under no milestone", "Todo", 2, "")
	write(t, l, `INSERT INTO slice_deps (slice_id, depends_on, position) VALUES (?, ?, ?), (?, ?, ?)`,
		"writes", "reads", 0, "writes", "design", 1)
}

func TestOpenLocalReadsAPlanItHasWritten(t *testing.T) {
	l, path := openPlan(t)
	if l.Path() != path {
		t.Errorf("Path() = %q, want %q", l.Path(), path)
	}
	fillPlan(t, l)

	plan, err := l.Plan(context.Background(), Project{ID: "proj", Name: "whatever the caller calls it"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if plan.Project.ID != "proj" {
		t.Errorf("project ID = %q, want %q", plan.Project.ID, "proj")
	}
	if plan.Project.Name != "notion-agent-tracker" {
		t.Errorf("project name = %q, want the plan's own", plan.Project.Name)
	}
	if plan.Migrated != "" {
		t.Errorf("Migrated = %q, want nothing changed", plan.Migrated)
	}

	wantMilestones := []domain.Milestone{
		{ID: "M1: The format", Name: "M1: The format", Order: 0, Status: domain.MilestoneDone},
		{ID: "M2: Reads", Name: "M2: Reads", Order: 1, Status: domain.MilestoneActive},
	}
	if !reflect.DeepEqual(plan.Project.Milestones, wantMilestones) {
		t.Errorf("milestones = %+v, want %+v", plan.Project.Milestones, wantMilestones)
	}

	wantSlices := []domain.Slice{
		{
			ID: "design", Name: "Design the local plan format",
			Status: domain.SliceDone, StatusName: "Done", MilestoneID: "M1: The format",
			AssigneeName: "Craig Johnston", AssigneeIDs: []string{"Craig Johnston"},
			PRURL: "https://example.test/pr/1",
		},
		{
			ID: "reads", Name: "Implement the local store: reads",
			Status: domain.SliceClaimed, StatusName: "In progress", MilestoneID: "M2: Reads",
			AssigneeName: "Craig Johnston", AssigneeIDs: []string{"Craig Johnston"},
			Repo: "/tmp/repo", Branch: "slice/reads",
		},
		{
			ID: "writes", Name: "Implement the local store: writes",
			Status: domain.SliceTodo, StatusName: "Todo", MilestoneID: "M2: Reads",
			DependsOn: []string{"reads", "design"},
		},
		{ID: "stray", Name: "A slice under no milestone", Status: domain.SliceTodo, StatusName: "Todo"},
	}
	if !reflect.DeepEqual(plan.Project.Slices, wantSlices) {
		t.Errorf("slices =\n%+v\nwant\n%+v", plan.Project.Slices, wantSlices)
	}
}

// A plan read locally has to group and block exactly as one read from Notion
// does, since every rule above the store is written against domain alone.
func TestLocalPlanGroupsAndBlocksLikeAnyOther(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)

	plan, err := l.Plan(context.Background(), Project{ID: "proj"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	groups := plan.Project.Groups()
	var names []string
	for _, g := range groups {
		names = append(names, g.Name())
	}
	want := []string{"M1: The format", "M2: Reads", domain.UnassignedName}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("groups = %v, want %v", names, want)
	}

	byID := domain.SlicesByID(plan.Project.Slices)
	writes := byID["writes"]
	blockers, unknown := domain.Blockers(writes, byID)
	if len(unknown) != 0 {
		t.Errorf("unknown blockers = %v, want none", unknown)
	}
	if len(blockers) != 1 || blockers[0].ID != "reads" {
		t.Errorf("blockers = %+v, want only the unfinished one", blockers)
	}
}

// The shape of a local plan is every column there is: the file is nat's own and
// there is no project in it old enough to be missing one.
func TestLocalShapeRecordsEverything(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)

	sh, err := l.Shape(context.Background(), Project{ID: "proj"})
	if err != nil {
		t.Fatalf("Shape: %v", err)
	}
	if !sh.HasAssignee || !sh.HasBranch {
		t.Errorf("shape = %+v, want both columns", sh)
	}
	if len(sh.Milestones) != 2 || sh.Milestones[0].Name != "M1: The format" {
		t.Errorf("milestones = %+v, want the plan's own in order", sh.Milestones)
	}
}

func TestLocalSliceReadsOneSliceAndItsShape(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)

	s, sh, err := l.Slice(context.Background(), "writes")
	if err != nil {
		t.Fatalf("Slice: %v", err)
	}
	if s.Name != "Implement the local store: writes" {
		t.Errorf("name = %q", s.Name)
	}
	if !reflect.DeepEqual(s.DependsOn, []string{"reads", "design"}) {
		t.Errorf("DependsOn = %v, want what it waits on in order", s.DependsOn)
	}
	if !sh.HasAssignee || !sh.HasBranch {
		t.Errorf("shape = %+v, want both columns", sh)
	}
}

// Ownership is what a claim wrote, and Holds has to read it back: a local plan
// has no directory of users behind it, so the name is the identity.
func TestLocalSliceIsHeldByWhoeverIsNamedOnIt(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)

	s, sh, err := l.Slice(context.Background(), "reads")
	if err != nil {
		t.Fatalf("Slice: %v", err)
	}
	if !Holds(s, sh, "Craig Johnston") {
		t.Error("Holds = false for the user named on the slice")
	}
	if Holds(s, sh, "somebody else") {
		t.Error("Holds = true for somebody the slice does not name")
	}
}

func TestLocalSliceReportsOneItDoesNotHave(t *testing.T) {
	l, path := openPlan(t)

	_, _, err := l.Slice(context.Background(), "nobody")
	if err == nil {
		t.Fatal("Slice on a missing slice: want an error")
	}
	if !strings.Contains(err.Error(), "nobody") || !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the slice and the path named", err)
	}
}

func TestLocalBodyReadsASliceAndAProject(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	conventions, err := l.Body(ctx, "proj")
	if err != nil {
		t.Fatalf("Body of the project: %v", err)
	}
	if conventions != "# Conventions\n\nBranch per slice." {
		t.Errorf("conventions = %q", conventions)
	}

	brief, err := l.Body(ctx, "design")
	if err != nil {
		t.Fatalf("Body of a slice: %v", err)
	}
	if brief != "Settle the format." {
		t.Errorf("brief = %q", brief)
	}

	none, err := l.Body(ctx, "neither")
	if err != nil {
		t.Fatalf("Body of neither: %v", err)
	}
	if none != "" {
		t.Errorf("body of an unknown ID = %q, want nothing", none)
	}
}

func TestLocalPRDescriptionIsTheLastSectionFiled(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	ctx := context.Background()

	got, err := l.PRDescription(ctx, "reads")
	if err != nil {
		t.Fatalf("PRDescription: %v", err)
	}
	if got != "Read a local plan\n\nWhat it does." {
		t.Errorf("PRDescription = %q", got)
	}

	none, err := l.PRDescription(ctx, "design")
	if err != nil {
		t.Fatalf("PRDescription of a slice with none: %v", err)
	}
	if none != "" {
		t.Errorf("PRDescription = %q, want nothing for a hand-back that filed none", none)
	}
}

// An empty directory — a machine that has never tracked this project — is an
// empty plan and not a failure, which is what lets a board open a project
// before anything has been written to it.
func TestOpenLocalMakesAnEmptyPlanWhereThereIsNone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "never", "used")
	path := filepath.Join(dir, "plan.db")

	l, err := OpenLocal(path)
	if err != nil {
		t.Fatalf("open a plan that is not there: %v", err)
	}
	defer func() { _ = l.Close() }()

	plan, err := l.Plan(context.Background(), Project{ID: "proj", Name: "nothing written yet"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Project.Milestones) != 0 || len(plan.Project.Slices) != 0 {
		t.Errorf("plan = %+v, want it empty", plan.Project)
	}
	if plan.Project.Name != "nothing written yet" {
		t.Errorf("name = %q, want the caller's own", plan.Project.Name)
	}
	if !plan.Shape.HasAssignee || !plan.Shape.HasBranch {
		t.Errorf("shape = %+v, want both columns even on an empty plan", plan.Shape)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("stat the plan that was opened: %v", err)
	}
}

// Opening the same plan twice is a read and no writes: the schema is stamped
// once and recognised afterwards.
func TestOpenLocalLeavesAPlanItAlreadyWroteAlone(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	again, err := OpenLocal(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = again.Close() }()

	plan, err := again.Plan(context.Background(), Project{ID: "proj"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Project.Slices) != 4 {
		t.Errorf("slices = %d, want the four already written", len(plan.Project.Slices))
	}
}

// A file that is not a plan is reported with the path and with what SQLite made
// of it, rather than read as though it were one.
func TestOpenLocalReportsAFileThatIsNotAPlan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.db")
	if err := os.WriteFile(path, []byte("this is not a database, it is a note"), 0o600); err != nil {
		t.Fatalf("write the corrupt file: %v", err)
	}

	l, err := OpenLocal(path)
	if err == nil {
		_ = l.Close()
		t.Fatal("open a file that is not a plan: want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}
	if !strings.Contains(err.Error(), "not a database") {
		t.Errorf("error = %q, want SQLite's own words about the file", err)
	}
}

// A plan whose rows are not the shape this build reads is a corrupt plan too,
// and the failure names the file rather than arriving as a bare scan error.
func TestLocalReportsAMilestoneItCannotRead(t *testing.T) {
	l, path := openPlan(t)
	write(t, l, `INSERT INTO milestones (name, position) VALUES (?, ?)`, "M1", "not a number")

	_, err := l.Plan(context.Background(), Project{ID: "proj"})
	if err == nil {
		t.Fatal("Plan over a row that will not scan: want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}
}

// A plan written by a later build is refused rather than read through a schema
// that is not its own.
func TestOpenLocalRefusesANewerPlan(t *testing.T) {
	l, path := openPlan(t)
	write(t, l, `PRAGMA user_version = 99`)
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	again, err := OpenLocal(path)
	if err == nil {
		_ = again.Close()
		t.Fatal("open a newer plan: want an error")
	}
	if !strings.Contains(err.Error(), "newer nat") || !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path and what is wrong with it", err)
	}
}

// Every read says which file it failed on, whatever it was doing, which is what
// a closed database is a cheap way of showing.
func TestLocalReadsNameTheirFileWhenTheyFail(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	ctx := context.Background()

	reads := map[string]func() error{
		"Shape": func() error { _, err := l.Shape(ctx, Project{ID: "proj"}); return err },
		"Plan":  func() error { _, err := l.Plan(ctx, Project{ID: "proj"}); return err },
		"Slice": func() error { _, _, err := l.Slice(ctx, "reads"); return err },
		"Body":  func() error { _, err := l.Body(ctx, "design"); return err },
		"PRDescription": func() error {
			_, err := l.PRDescription(ctx, "reads")
			return err
		},
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

// The slices and the dependencies are two queries, so each has its own way of
// failing and each has to name the file.
func TestLocalNamesItsFileWhenTheDependenciesWillNotRead(t *testing.T) {
	l, path := openPlan(t)
	fillPlan(t, l)
	write(t, l, `DROP TABLE slice_deps`)
	ctx := context.Background()

	_, err := l.Plan(ctx, Project{ID: "proj"})
	if err == nil {
		t.Fatal("Plan with no dependency table: want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}
	if _, _, err := l.Slice(ctx, "reads"); err == nil {
		t.Error("Slice with no dependency table: want an error")
	}
}

// A row that will not scan is a corrupt plan too — whichever of the three reads
// is scanning it — and each says what it was reading and where.
func TestLocalNamesItsFileWhenARowWillNotScan(t *testing.T) {
	l, path := openPlan(t)
	// A plan hand-edited into a shape this build does not read: the columns are
	// there and what is in them is not what they are for.
	write(t, l, `DROP TABLE slices`)
	write(t, l, `CREATE TABLE slices (id TEXT, title TEXT, status TEXT, milestone TEXT,
		position REAL, assignee TEXT, repo TEXT, branch TEXT, pr TEXT, body TEXT)`)
	write(t, l, `INSERT INTO slices (id, position) VALUES ('one', 0)`)
	write(t, l, `DROP TABLE slice_deps`)
	write(t, l, `CREATE TABLE slice_deps (slice_id TEXT, depends_on TEXT, position INTEGER)`)
	write(t, l, `INSERT INTO slice_deps (slice_id, position) VALUES ('one', 0)`)
	ctx := context.Background()

	reads := map[string]func() error{
		"the slices":       func() error { _, err := l.slices(ctx); return err },
		"one slice":        func() error { _, _, err := l.Slice(ctx, "one"); return err },
		"the dependencies": func() error { _, err := l.dependencies(ctx); return err },
	}
	for name, read := range reads {
		err := read()
		if err == nil {
			t.Errorf("reading %s off a hand-edited plan: want an error", name)
			continue
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("reading %s: error = %q, want the path named", name, err)
		}
	}
}

// A read that fails part way through its rows is reported rather than passed
// off as a short plan, which is what rows.Err answers and what a view raising
// SQLite's own error at the second row is a cheap way of asking.
func TestLocalNamesItsFileWhenAReadFailsPartWayThrough(t *testing.T) {
	l, path := openPlan(t)
	overflow := `SELECT 'b', abs(-9223372036854775808)`
	write(t, l, `DROP TABLE milestones`)
	write(t, l, `CREATE VIEW milestones (name, position) AS SELECT 'a', 0.0 UNION ALL `+overflow)
	write(t, l, `DROP TABLE slices`)
	write(t, l, `CREATE VIEW slices (id, title, status, milestone, position, assignee, repo, branch, pr, body)
		AS SELECT 'a', 'A', 'Todo', NULL, 0.0, '', '', '', '', ''
		UNION ALL SELECT 'b', 'B', 'Todo', NULL, abs(-9223372036854775808), '', '', '', '', ''`)
	write(t, l, `DROP TABLE slice_deps`)
	write(t, l, `CREATE VIEW slice_deps (slice_id, depends_on, position) AS
		SELECT 'a', 'b', 0 UNION ALL SELECT 'b', 'a', abs(-9223372036854775808)`)
	ctx := context.Background()

	reads := map[string]func() error{
		"the milestones":   func() error { _, err := l.milestones(ctx); return err },
		"the slices":       func() error { _, err := l.slices(ctx); return err },
		"the dependencies": func() error { _, err := l.dependencies(ctx); return err },
	}
	for name, read := range reads {
		err := read()
		if err == nil {
			t.Errorf("reading %s that fails part way: want an error", name)
			continue
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("reading %s: error = %q, want the path named", name, err)
		}
	}
}

// A plan with no slices table at all is a file nat did not write, and the read
// says so by name rather than answering with an empty plan.
func TestLocalNamesItsFileWhenTheSlicesTableIsGone(t *testing.T) {
	l, path := openPlan(t)
	write(t, l, `DROP TABLE slice_deps`)
	write(t, l, `DROP TABLE slices`)

	_, err := l.Plan(context.Background(), Project{ID: "proj"})
	if err == nil {
		t.Fatal("Plan with no slices table: want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}
}

// A plan whose tables are already there under a version that says there are
// none is a file nat did not write, and creating over it is refused by name.
func TestOpenLocalReportsASchemaItCannotCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.db")
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open the file in the way: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE project (mine TEXT)`); err != nil {
		t.Fatalf("write the table in the way: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	l, err := OpenLocal(path)
	if err == nil {
		_ = l.Close()
		t.Fatal("open over somebody else's tables: want an error")
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want the path and what is in the way", err)
	}
}

// A path the driver cannot even be asked about fails before SQLite is opened,
// and names the path all the same.
func TestOpenLocalReportsAPathTheDriverRefuses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan\x7f.db")

	l, err := OpenLocal(path)
	if err == nil {
		_ = l.Close()
		t.Fatal("open a path the driver will not parse: want an error")
	}
	if !strings.Contains(err.Error(), "open the plan at") {
		t.Errorf("error = %q, want it said as opening a plan", err)
	}
}

// A plan that names no project falls back to what the caller calls it, and a
// project row that will not read is a failure rather than that fallback.
func TestLocalProjectNameReportsAReadItCannotMake(t *testing.T) {
	l, path := openPlan(t)
	write(t, l, `DROP TABLE project`)

	_, err := l.Plan(context.Background(), Project{ID: "proj", Name: "fallback"})
	if err == nil {
		t.Fatal("Plan with no project table: want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}
}

// A project row holding no name is a plan that has not been told what it is
// called, which is the caller's own name for it.
func TestLocalPlanFallsBackToTheCallersNameForAnUnnamedProject(t *testing.T) {
	l, _ := openPlan(t)
	write(t, l, `INSERT INTO project (id, name) VALUES (?, ?)`, "proj", "")

	plan, err := l.Plan(context.Background(), Project{ID: "proj", Name: "what the caller calls it"})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.Project.Name != "what the caller calls it" {
		t.Errorf("name = %q, want the caller's own", plan.Project.Name)
	}
}

// Body reads a slice first and the project second, so a project row that will
// not read has to be reported rather than passed over as no prose.
func TestLocalBodyReportsAProjectItCannotRead(t *testing.T) {
	l, path := openPlan(t)
	write(t, l, `DROP TABLE project`)

	_, err := l.Body(context.Background(), "proj")
	if err == nil {
		t.Fatal("Body with no project table: want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}
}

// A directory that cannot be made is the one failure that happens before
// SQLite is asked anything, and it names the directory it could not make.
func TestOpenLocalReportsADirectoryItCannotMake(t *testing.T) {
	file := filepath.Join(t.TempDir(), "in-the-way")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("write the file in the way: %v", err)
	}

	l, err := OpenLocal(filepath.Join(file, "plan.db"))
	if err == nil {
		_ = l.Close()
		t.Fatal("open under a file: want an error")
	}
	if !strings.Contains(err.Error(), file) {
		t.Errorf("error = %q, want the directory named", err)
	}
}

func TestCloseReportsAFailureByItsFile(t *testing.T) {
	l, path := openPlan(t)
	if err := l.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	// database/sql is happy to close twice, so the failure has to be made:
	// a Local over a database whose driver has gone is what a caller sees.
	broken := &Local{db: brokenDB(t), path: path}
	if err := broken.Close(); err == nil {
		t.Error("close a database that will not close: want an error")
	} else if !strings.Contains(err.Error(), path) {
		t.Errorf("error = %q, want the path named", err)
	}
}

// brokenDB is a database whose connector fails to close, which is the only way
// database/sql's Close reports anything at all.
func brokenDB(t *testing.T) *sql.DB {
	t.Helper()
	return sql.OpenDB(failingConnector{})
}

type failingConnector struct{}

func (failingConnector) Connect(context.Context) (driver.Conn, error) { return nil, errClosed }
func (failingConnector) Driver() driver.Driver                        { return nil }
func (failingConnector) Close() error                                 { return errClosed }

var errClosed = errors.New("this connector will not close")

// The writes are the next slice's, and until then they refuse rather than
// pretend: a Local is a Store from the read half onwards, so every write has to
// be there to be refused.
func TestLocalWritesRefuseUntilTheyAreImplemented(t *testing.T) {
	l, _ := openPlan(t)
	ctx := context.Background()

	writes := map[string]func() error{
		"ClaimSlice":   func() error { _, err := l.ClaimSlice(ctx, "x", Shape{}, "u"); return err },
		"ReleaseSlice": func() error { _, err := l.ReleaseSlice(ctx, "x", Shape{}, "u"); return err },
		"CompleteSlice": func() error {
			_, err := l.CompleteSlice(ctx, "x", Shape{}, Outcome{})
			return err
		},
		"RecordPR": func() error { return l.RecordPR(ctx, "x", "url") },
		"MarkDone": func() error { return l.MarkDone(ctx, "x", Shape{}) },
		"AddMilestones": func() error {
			_, err := l.AddMilestones(ctx, Project{}, Shape{}, []string{"M1"})
			return err
		},
		"AddSlice":        func() error { _, err := l.AddSlice(ctx, Project{}, NewSlice{}); return err },
		"EditSlice":       func() error { return l.EditSlice(ctx, "x", "t", "r", "b") },
		"SetSliceBrief":   func() error { return l.SetSliceBrief(ctx, "x", "b") },
		"SetDependencies": func() error { _, err := l.SetDependencies(ctx, "x", nil); return err },
		"MoveSlice":       func() error { return l.MoveSlice(ctx, "x", domain.Milestone{}) },
		"DeleteSlice":     func() error { return l.DeleteSlice(ctx, "x") },
	}
	for name, w := range writes {
		if err := w(); !errors.Is(err, errLocalReadOnly) {
			t.Errorf("%s = %v, want it refused as not implemented", name, err)
		}
	}
}

func TestLocalDirIsTheUsersDataDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	cases := map[string]string{
		"darwin": filepath.Join(home, "Library", "Application Support", localAppDir, localPlansDir),
		"linux":  filepath.Join(home, ".local", "share", localAppDir, localPlansDir),
	}
	for goos, want := range cases {
		got, err := localDirFor(goos)
		if err != nil {
			t.Fatalf("localDirFor(%q): %v", goos, err)
		}
		if got != want {
			t.Errorf("localDirFor(%q) = %q, want %q", goos, got, want)
		}
	}
}

// XDG_DATA_HOME is the user saying where data goes, and outranks the default
// everywhere it applies — which is everywhere but macOS, where the platform's
// own answer is the one nat's other directories take.
func TestLocalDirHonoursXDGDataHome(t *testing.T) {
	home, data := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", data)

	got, err := localDirFor("linux")
	if err != nil {
		t.Fatalf("localDirFor: %v", err)
	}
	if want := filepath.Join(data, localAppDir, localPlansDir); got != want {
		t.Errorf("localDirFor = %q, want %q", got, want)
	}
	mac, err := localDirFor("darwin")
	if err != nil {
		t.Fatalf("localDirFor(darwin): %v", err)
	}
	if strings.HasPrefix(mac, data) {
		t.Errorf("localDirFor(darwin) = %q, want the platform's own directory", mac)
	}
}

func TestLocalDirReportsAHomeItCannotResolve(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	if _, err := localDirFor("linux"); err == nil {
		t.Error("localDirFor with no home: want an error")
	}
	if _, err := LocalPath("proj"); err == nil {
		t.Error("LocalPath with no home: want an error")
	}
}

func TestLocalDirIsWhereAProjectsPlanIsKept(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	got, err := LocalPath("3b738308-F654-811c/948d")
	if err != nil {
		t.Fatalf("LocalPath: %v", err)
	}
	dir, err := LocalDir()
	if err != nil {
		t.Fatalf("LocalDir: %v", err)
	}
	if want := filepath.Join(dir, "3b738308-f654-811c-948d.db"); got != want {
		t.Errorf("LocalPath = %q, want %q", got, want)
	}
}

func TestLocalSlugKeepsANameForAnIDThatSlugsAwayToNothing(t *testing.T) {
	if got := localSlug("///"); got != "unnamed" {
		t.Errorf("localSlug(%q) = %q, want a name of its own", "///", got)
	}
}

func TestLastMarkdownSection(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"none", "Just a brief.", ""},
		{
			"the last of two",
			"## PR description\nfirst\n\n## Handed back\nnote\n\n## PR description\nsecond",
			"second",
		},
		{
			"closed by a heading of the same level",
			"# PR description\nkept\n# Summary\ndropped",
			"kept",
		},
		{
			"closed by a heading of a higher level",
			"## PR description\nkept\n# Summary\ndropped",
			"kept",
		},
		{
			"a deeper heading is part of the section",
			"## PR description\nkept\n### Detail\nalso kept",
			"kept\n### Detail\nalso kept",
		},
		{
			"a hash inside a fence is not a heading",
			"## PR description\n```sh\n# not a heading\n```\ndone",
			"```sh\n# not a heading\n```\ndone",
		},
		{
			"a tilde fence closes with tildes and not with backticks",
			"## PR description\n~~~\n# still fenced\n```\n# also fenced\n~~~\nout",
			"~~~\n# still fenced\n```\n# also fenced\n~~~\nout",
		},
		{
			"a longer fence closes a shorter one",
			"## PR description\n```\ncode\n````\nafter",
			"```\ncode\n````\nafter",
		},
		{"matched however it is written", "###   pr DESCRIPTION  ##\nkept", "kept"},
		{"hashes with no space are not a heading", "#PR description\nnot a section", ""},
		{"seven hashes are not a heading", "####### PR description\nnot a section", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := lastMarkdownSection(c.body, "PR description"); got != c.want {
				t.Errorf("lastMarkdownSection = %q, want %q", got, c.want)
			}
		})
	}
}
