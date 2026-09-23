package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"

	// The pure-Go SQLite driver: SQLite's own C compiled to WebAssembly and run
	// in process, so `go install ...@latest` keeps working on a bare machine and
	// the release pipeline goes on cross-building per arch. It is registered as
	// "sqlite3" on database/sql, which is the only thing this file asks of it.
	_ "github.com/ncruces/go-sqlite3/driver"
)

// Local is a plan kept in a SQLite database of nat's own, one file per project,
// which is the store a project tracked in files rather than in Notion reads
// from — and, once Notion is demoted to a replica written through to, the store
// every project reads from.
//
// The reads are here and the writes in local_write.go, where every mutation is
// one transaction that re-reads the slice it is about before it writes it.
type Local struct {
	db *sql.DB
	// path is the database's own file, carried so that every failure can name
	// it: a store that will not open is a file on this machine and the path is
	// the whole of what there is to go and look at.
	path string
}

// Local is a Store.
var _ Store = (*Local)(nil)

// ErrSliceNotFound is what a read of one slice by ID wraps when the plan
// holds no such slice — the file's own "not found," wrapped rather than
// returned bare so the message still names the slice and the path, and
// matched with errors.Is by a caller that has to tell a slice the file has
// never seen from any other failure reading one, [Mirrored.Slice] chief among
// them.
var ErrSliceNotFound = errors.New("no such slice in the plan")

// LocalDir is the directory nat keeps its plans in: one database per project,
// under nat's own data directory — ~/Library/Application Support on macOS, and
// $XDG_DATA_HOME (or ~/.local/share) everywhere else. Plans are data rather
// than state or configuration: a log can be thrown away and a plan cannot.
func LocalDir() (string, error) { return localDirFor(runtime.GOOS) }

// localDirFor resolves that directory for an operating system, taking the OS as
// an argument so both answers are reachable from a test on either platform,
// exactly as logging.dirFor does.
func localDirFor(goos string) (string, error) {
	if goos != "darwin" {
		if x := os.Getenv("XDG_DATA_HOME"); x != "" {
			return filepath.Join(x, localAppDir, localPlansDir), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	if goos == "darwin" {
		return filepath.Join(home, "Library", "Application Support", localAppDir, localPlansDir), nil
	}
	return filepath.Join(home, ".local", "share", localAppDir, localPlansDir), nil
}

// The directory names under a user's data directory. They are nat's own name
// and one level for the plans, so anything else nat comes to keep as data has
// somewhere of its own to go.
const (
	localAppDir   = "notion-agent-tracker"
	localPlansDir = "plans"
)

// LocalPath is where a project's plan is kept: its ID, slugged, under
// [LocalDir]. A file per project keeps every project's blast radius its own — a
// database hand-edited into nonsense costs one project, a busy writer contends
// only with that project's own readers, and forgetting a project is deleting
// its file.
func LocalPath(projectID string) (string, error) {
	dir, err := LocalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, localSlug(projectID)+".db"), nil
}

// PlanPath is where a project's plan file is: LocalPath, or the same name in
// the directory the project's config chose for it.
func PlanPath(p Project) (string, error) {
	if p.PlanDir == "" {
		return LocalPath(p.ID)
	}
	return filepath.Join(p.PlanDir, localSlug(p.ID)+".db"), nil
}

// localSlug is a project ID as a filename: every run of anything but a letter,
// a digit, a dot, a hyphen or an underscore collapsed to one hyphen, the same
// rule internal/worktree slugs a branch into a path with. An ID that slugs away
// to nothing keeps a name of its own rather than becoming ".db".
func localSlug(id string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(id) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
			dash = false
		case !dash:
			b.WriteRune('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "unnamed"
	}
	return s
}

// OpenLocal opens the plan kept at the given path, creating the file and the
// schema if there is nothing there yet — which is what makes a project with no
// plan an empty plan rather than a failure. The directory is created too, so a
// machine that has never tracked a local project needs nothing set up first.
//
// A file that is there and is not a plan — hand-edited, truncated, some other
// file entirely — is reported with the path and what SQLite made of it, since
// the path is the whole of what there is to go and look at.
func OpenLocal(path string) (*Local, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("make the plan directory %s: %w", dir, err)
		}
	}
	db, err := sql.Open("sqlite3", localDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open the plan at %s: %w", path, err)
	}
	l := &Local{db: db, path: path}
	if err := l.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return l, nil
}

// localDSN is how the database is opened: WAL, so the board's poll never
// stalls an agent's write and no reader ever blocks one, and a busy timeout, so
// two processes writing at once wait for each other rather than one of them
// failing. Foreign keys are on, which is what makes a dependency on a slice
// that is not there impossible rather than merely wrong.
//
// Every transaction is BEGIN IMMEDIATE (_txlock), because every transaction
// this store opens is a write that reads first: a deferred one takes its read
// lock at the first SELECT and asks for the write lock afterwards, which is the
// one upgrade SQLite refuses outright rather than waiting out the busy timeout
// for — so two agents writing at once would fail rather than queue, which is
// exactly what the timeout is there to prevent.
func localDSN(path string) string {
	return "file:" + path +
		"?_pragma=journal_mode(wal)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(on)" +
		"&_txlock=immediate"
}

// Close gives the database back. A Local is held open for as long as its caller
// runs — a board for a session, a headless command for one command — so this is
// the end of that and not something a read does.
func (l *Local) Close() error {
	if err := l.db.Close(); err != nil {
		return fmt.Errorf("close the plan at %s: %w", l.path, err)
	}
	return nil
}

// Path is the file the plan is kept in.
func (l *Local) Path() string { return l.path }

// localSchemaVersion is the schema this build writes and reads. It is kept in
// SQLite's own user_version, so opening a plan written by this build is one
// read and no writes, and a plan written by a later one can be refused rather
// than half understood.
const localSchemaVersion = 4

// localSchemaV1 is the plan as tables, exactly as the first build of this store
// created it. Every column maps one-to-one onto [domain.Slice] or
// [domain.Milestone], so this store produces the same structs the Notion
// mapper does and everything above them — domain.Blockers, StateOf, HandedBack,
// the board, next-slice — is untouched.
//
// body and conventions hold markdown verbatim, which is the brief exactly as an
// agent receives it: the one constraint the format could not trade away.
//
// The full-text index the design settles on is not here: nothing yet searches a
// plan, and an index kept in step by triggers is machinery to write when there
// is a query to serve. user_version is what makes adding it later one
// migration rather than a second format.
const localSchemaV1 = `
CREATE TABLE project (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  conventions TEXT NOT NULL DEFAULT ''
);

CREATE TABLE milestones (
  name        TEXT PRIMARY KEY,
  position    REAL NOT NULL
);

CREATE TABLE slices (
  id          TEXT PRIMARY KEY,
  title       TEXT NOT NULL,
  status      TEXT NOT NULL DEFAULT 'Todo',
  milestone   TEXT,
  position    REAL NOT NULL,
  assignee    TEXT NOT NULL DEFAULT '',
  repo        TEXT NOT NULL DEFAULT '',
  branch      TEXT NOT NULL DEFAULT '',
  pr          TEXT NOT NULL DEFAULT '',
  body        TEXT NOT NULL DEFAULT ''
);

CREATE TABLE slice_deps (
  slice_id    TEXT NOT NULL REFERENCES slices(id),
  depends_on  TEXT NOT NULL REFERENCES slices(id),
  position    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (slice_id, depends_on)
);

CREATE TABLE sync (
  slice_id    TEXT PRIMARY KEY REFERENCES slices(id),
  dirty       INTEGER NOT NULL DEFAULT 0,
  synced_at   TEXT
);
`

// localSchemaV2 is what a replica needs that a plan of its own never did: a
// name for whoever holds a slice, kept apart from the identity the existing
// assignee column already is — a plan of its own has no directory of users, so
// one string was both, and a replica of a Notion project cannot, Notion
// recording a person as a user ID with a name the workspace supplies; a stamp
// on every piece of prose fetched apart from the plan, so a re-pull knows
// whether its copy is worth trusting; and on the project, when it was last
// brought into line with the workspace and the shape read off it, so Shape can
// be answered from the file rather than putting a request back on every read.
// milestones.select_type is the one column [domain.Milestone] already carries
// that the file would otherwise drop — the property type a write to Notion's
// Milestone column has to be sent as.
//
// assignee_name is back-filled from assignee, which is exactly right for every
// plan written before this column existed: such a plan has no directory of
// users either, so the name a claim wrote is the only name there ever was.
const localSchemaV2 = `
ALTER TABLE slices ADD COLUMN assignee_name TEXT NOT NULL DEFAULT '';
ALTER TABLE slices ADD COLUMN body_at TEXT;
ALTER TABLE project ADD COLUMN conventions_at TEXT;
ALTER TABLE project ADD COLUMN synced_at TEXT;
ALTER TABLE project ADD COLUMN has_assignee INTEGER NOT NULL DEFAULT 0;
ALTER TABLE project ADD COLUMN has_branch INTEGER NOT NULL DEFAULT 0;
ALTER TABLE milestones ADD COLUMN select_type TEXT NOT NULL DEFAULT '';

UPDATE slices SET assignee_name = assignee WHERE assignee != '';
`

// localSchemaV3 adds the one column a replica needs that a plan of its own
// never did: a slice's own page URL, which a plan kept in a file of its own
// has no Notion page to have — and so is "" for every such slice, exactly
// the default a plan already at this schema would want for one written
// before this column existed too.
const localSchemaV3 = `
ALTER TABLE slices ADD COLUMN url TEXT NOT NULL DEFAULT '';
`

// localSchemaV4 adds the one table a plan of its own never needed until ad
// hoc sessions existed: a session belongs to this machine, never to the
// workspace a project's plan is otherwise kept in, so it is a table of its
// own rather than a slices row — nothing here maps onto [domain.Slice] or
// [domain.Milestone] at all.
//
// dir is where the session runs — a repository or a plain directory, in the
// same words a slice's own repo column names its project rather than a
// worktree path — and branch is the one it was cut on, empty for a session
// launched outside any git repository. ended_at is NULL until nat has seen
// the session gone, the same "never set" rule [timeStamp] already gives
// every other stamp in this file.
const localSchemaV4 = `
CREATE TABLE sessions (
  id          TEXT PRIMARY KEY,
  started_at  TEXT NOT NULL,
  dir         TEXT NOT NULL DEFAULT '',
  branch      TEXT NOT NULL DEFAULT '',
  ended_at    TEXT
);
`

// localMigrations is what [Local.migrate] walks version+1..[localSchemaVersion]
// through, so a plan lands on today's schema whichever version it started at —
// an empty file walking every migration there is, and a plan already at v1
// walking only the ones written since.
var localMigrations = map[int]string{
	1: localSchemaV1,
	2: localSchemaV2,
	3: localSchemaV3,
	4: localSchemaV4,
}

// migrate brings the file up to the schema this build speaks, and is what every
// open runs: an empty file becomes an empty plan, a plan already at this
// version is read and left alone, and a plan from a later build is refused
// rather than read through a schema that is not its own.
func (l *Local) migrate(ctx context.Context) error {
	var version int
	if err := l.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return l.errorf(err, "read the plan")
	}
	if version > localSchemaVersion {
		return fmt.Errorf("the plan at %s was written by a newer nat (schema %d, this build reads %d)",
			l.path, version, localSchemaVersion)
	}
	// The stamp goes on in the same statement as each step, so a plan is never
	// left holding a schema without the version that says which schema it is —
	// and a build killed part way through several steps resumes at the one it
	// never finished rather than repeating one already stamped in.
	for v := version + 1; v <= localSchemaVersion; v++ {
		stmt := localMigrations[v]
		if _, err := l.db.ExecContext(ctx, stmt+fmt.Sprintf("\nPRAGMA user_version = %d;\n", v)); err != nil {
			return l.errorf(err, "bring the plan to schema "+fmt.Sprint(v))
		}
	}
	return nil
}

// localQuerier is the little every read of a plan needs, which both a database
// and a transaction on one answer: that is what lets one set of read helpers
// serve a read taken on its own and the re-read a write takes inside its own
// transaction, so the two can never drift into reading a slice differently.
type localQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// errorf says what went wrong in the store's own words, naming the file it went
// wrong in: a plan that will not read is a path on this machine, and the path
// is what tells the difference between a plan nat wrote and a file something
// else left there.
func (l *Local) errorf(err error, doing string) error {
	return fmt.Errorf("%s at %s: %w", doing, l.path, err)
}

// localShape is what a local plan can record about a slice and the
// milestones there are to file one under. Whether it can record ownership or
// a branch is read off the project row — [Local.Hydrate]'s own copy of what
// the workspace it replicates actually offers — for a plan [Local.hydrated]
// once from one; a plan with no workspace behind it, or not yet pulled from
// one, answers both yes, because the columns are this store's own and there
// is no project old enough to be missing one.
func (l *Local) localShape(ctx context.Context, p Project, ms []domain.Milestone) (Shape, error) {
	hasAssignee, hasBranch := true, true
	hydrated, err := l.hydrated(ctx, p.ID)
	if err != nil {
		return Shape{}, err
	}
	if hydrated {
		var ha, hb int
		if err := l.db.QueryRowContext(ctx,
			`SELECT has_assignee, has_branch FROM project WHERE id = ?`, p.ID).Scan(&ha, &hb); err != nil {
			return Shape{}, l.errorf(err, "read the project")
		}
		hasAssignee, hasBranch = ha != 0, hb != 0
	}
	return Shape{HasAssignee: hasAssignee, HasBranch: hasBranch, Milestones: ms}, nil
}

// Shape reads what can be recorded about a project's slices and the milestones
// there are to file one under, without reading the slices themselves.
func (l *Local) Shape(ctx context.Context, p Project) (Shape, error) {
	ms, err := l.milestones(ctx, l.db)
	if err != nil {
		return Shape{}, err
	}
	return l.localShape(ctx, p, ms)
}

// milestones reads the plan's milestones in plan order. A milestone is nothing
// but a name and a place, exactly as domain already says: its status is
// computed from the slices under it, so there is nothing else to store.
func (l *Local) milestones(ctx context.Context, q localQuerier) ([]domain.Milestone, error) {
	rows, err := q.QueryContext(ctx, `SELECT name, position, select_type FROM milestones ORDER BY position, name`)
	if err != nil {
		return nil, l.errorf(err, "read the milestones")
	}
	defer func() { _ = rows.Close() }()

	var ms []domain.Milestone
	for rows.Next() {
		var m domain.Milestone
		if err := rows.Scan(&m.Name, &m.Order, &m.SelectType); err != nil {
			return nil, l.errorf(err, "read a milestone")
		}
		m.ID = m.Name
		ms = append(ms, m)
	}
	if err := rows.Err(); err != nil {
		return nil, l.errorf(err, "read the milestones")
	}
	return ms, nil
}

// Plan reads the whole plan: its milestones and its slices, each in the order
// the plan puts them in. The order is a column rather than a view's own row
// order, so there is no second round trip to read it and nothing to fall back
// to when that read fails.
func (l *Local) Plan(ctx context.Context, p Project) (Plan, error) {
	ms, err := l.milestones(ctx, l.db)
	if err != nil {
		return Plan{}, err
	}
	slices, err := l.slices(ctx, l.db)
	if err != nil {
		return Plan{}, err
	}
	name, err := l.projectName(ctx, p)
	if err != nil {
		return Plan{}, err
	}
	sh, err := l.localShape(ctx, p, ms)
	if err != nil {
		return Plan{}, err
	}
	return Plan{
		Project: domain.NewProject(p.ID, name, ms, slices),
		Shape:   sh,
	}, nil
}

// projectName is what the plan calls the project, and the caller's own name for
// it where the plan holds none — which is a plan nothing has been written to
// yet, and a project whose name the caller knows better than the file does
// because the caller has just read it from somewhere else.
func (l *Local) projectName(ctx context.Context, p Project) (string, error) {
	var name string
	err := l.db.QueryRowContext(ctx, `SELECT name FROM project LIMIT 1`).Scan(&name)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return p.Name, nil
	case err != nil:
		return "", l.errorf(err, "read the project")
	case name == "":
		return p.Name, nil
	}
	return name, nil
}

// hydrated reports whether the file holds a project row at all — which is
// exactly what a [Hydrate] pull writes and nothing else does, so its absence
// is what tells [ForProject] a plan has never been pulled from the workspace
// it mirrors.
func (l *Local) hydrated(ctx context.Context, id string) (bool, error) {
	var synced sql.NullString
	err := l.db.QueryRowContext(ctx, `SELECT synced_at FROM project WHERE id = ?`, id).Scan(&synced)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, l.errorf(err, "check whether the plan has been hydrated")
	}
	return synced.Valid, nil
}

// SyncedAt reads when the file was last brought fully into line with the
// workspace — [Hydrate]'s own stamp on the project row — which is what
// [Mirrored] compares against the clock to decide whether an ordinary read
// pulls for itself before answering. The zero time is a plan never hydrated
// at all, read back exactly as it went in rather than as an error: there is
// nothing wrong with a plan that has simply never been pulled.
func (l *Local) SyncedAt(ctx context.Context, id string) (time.Time, error) {
	var synced sql.NullString
	err := l.db.QueryRowContext(ctx, `SELECT synced_at FROM project WHERE id = ?`, id).Scan(&synced)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return time.Time{}, nil
	case err != nil:
		return time.Time{}, l.errorf(err, "read the plan's freshness")
	}
	if !synced.Valid || synced.String == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, synced.String)
	if err != nil {
		return time.Time{}, l.errorf(err, "parse the plan's freshness")
	}
	return t, nil
}

// localSliceColumns is the one list of columns every slice read selects, so the
// scan below can be one function rather than one per query.
const localSliceColumns = `id, title, status, COALESCE(milestone, ''), assignee, assignee_name, repo, branch, pr, url`

// scanLocalSlice reads one row of [localSliceColumns] into the app's own words.
// A local plan writes exactly the statuses domain names, so the status is both
// the workflow status and what the project calls it. assignee is the identity
// an ownership check compares against — a Notion person ID for a replica, or,
// for a plan of its own with no such directory, the same string as the name —
// and assignee_name is what is shown for it, back-filled from assignee by the
// v2 migration for exactly the plans that predate the two being different.
func scanLocalSlice(scan func(...any) error) (domain.Slice, error) {
	var s domain.Slice
	var status, assignee, assigneeName string
	if err := scan(&s.ID, &s.Name, &status, &s.MilestoneID, &assignee, &assigneeName, &s.Repo, &s.Branch, &s.PRURL, &s.URL); err != nil {
		return domain.Slice{}, err
	}
	s.Status, s.StatusName = domain.SliceStatus(status), status
	if assignee != "" {
		s.AssigneeIDs = []string{assignee}
		s.AssigneeName = assigneeName
		if s.AssigneeName == "" {
			s.AssigneeName = assignee
		}
	}
	return s, nil
}

// slices reads every slice of the plan in plan order: by position, and by ID
// where two share one, so the order is total and two writers who both picked
// the same position get a stable answer rather than a board that flaps between
// readings.
func (l *Local) slices(ctx context.Context, q localQuerier) ([]domain.Slice, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT `+localSliceColumns+` FROM slices ORDER BY position, id`)
	if err != nil {
		return nil, l.errorf(err, "read the slices")
	}
	defer func() { _ = rows.Close() }()

	var ss []domain.Slice
	for rows.Next() {
		s, err := scanLocalSlice(rows.Scan)
		if err != nil {
			return nil, l.errorf(err, "read a slice")
		}
		ss = append(ss, s)
	}
	if err := rows.Err(); err != nil {
		return nil, l.errorf(err, "read the slices")
	}
	deps, err := l.dependencies(ctx, q)
	if err != nil {
		return nil, err
	}
	for i := range ss {
		ss[i].DependsOn = deps[ss[i].ID]
	}
	return ss, nil
}

// dependencies reads what every slice waits on, in the order it was recorded
// in, as one query rather than one per slice: a plan's whole dependency graph
// is a few hundred rows at the very most and a read per slice would grow with
// the plan forever.
func (l *Local) dependencies(ctx context.Context, q localQuerier) (map[string][]string, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT slice_id, depends_on FROM slice_deps ORDER BY slice_id, position, depends_on`)
	if err != nil {
		return nil, l.errorf(err, "read the dependencies")
	}
	defer func() { _ = rows.Close() }()

	deps := map[string][]string{}
	for rows.Next() {
		var of, on string
		if err := rows.Scan(&of, &on); err != nil {
			return nil, l.errorf(err, "read a dependency")
		}
		deps[of] = append(deps[of], on)
	}
	if err := rows.Err(); err != nil {
		return nil, l.errorf(err, "read the dependencies")
	}
	return deps, nil
}

// Slice reads one slice, and with it the shape it can be written in — which for
// a local plan is the project's shape, since the columns are the same columns
// for every slice in the file: a replica of a workspace with neither column
// reads back false for both here exactly as [Local.Shape] answers it, rather
// than assuming every plan carries them the way a plan of its own always does.
func (l *Local) Slice(ctx context.Context, id string) (domain.Slice, Shape, error) {
	s, err := l.slice(ctx, l.db, id)
	if err != nil {
		return domain.Slice{}, Shape{}, err
	}
	sh, err := l.sliceShape(ctx)
	if err != nil {
		return domain.Slice{}, Shape{}, err
	}
	return s, sh, nil
}

// sliceShape is the shape a write to any one slice takes, read off the file's
// single project row rather than a caller's own ID — a local plan holds
// exactly one project, so there is nothing else it could be — by the same
// rule [Local.localShape] reads the whole plan's shape by: both columns only
// where [Local.hydrated] says this is a replica of a workspace that has
// actually answered for them, true otherwise, since a plan of its own has no
// schema to be missing either from.
func (l *Local) sliceShape(ctx context.Context) (Shape, error) {
	var id string
	err := l.db.QueryRowContext(ctx, `SELECT id FROM project LIMIT 1`).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return Shape{HasAssignee: true, HasBranch: true}, nil
	case err != nil:
		return Shape{}, l.errorf(err, "read the project")
	}
	return l.localShape(ctx, Project{ID: id}, nil)
}

// slice reads one slice through whichever querier it is given, which is how a
// write re-reads the slice it is about inside its own transaction.
func (l *Local) slice(ctx context.Context, q localQuerier, id string) (domain.Slice, error) {
	row := q.QueryRowContext(ctx, `SELECT `+localSliceColumns+` FROM slices WHERE id = ?`, id)
	s, err := scanLocalSlice(row.Scan)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return domain.Slice{}, fmt.Errorf("no slice %s in the plan at %s: %w", id, l.path, ErrSliceNotFound)
	case err != nil:
		return domain.Slice{}, l.errorf(err, "read the slice")
	}
	deps, err := l.dependencies(ctx, q)
	if err != nil {
		return domain.Slice{}, err
	}
	s.DependsOn = deps[s.ID]
	return s, nil
}

// Body reads the prose kept against an ID as markdown — a slice's brief, or the
// conventions written on the project. Both are the same read, as they are in
// Notion, and the markdown is the markdown that was written: there is nothing
// to render, which is the whole point of holding it as markdown.
//
// An ID neither a slice nor the project answers to reads as no prose rather
// than as a failure: the callers that ask this of a project ask it of whatever
// project they are on, and a plan that has never been written to has no row for
// it yet.
func (l *Local) Body(ctx context.Context, id string) (string, error) {
	var body string
	err := l.db.QueryRowContext(ctx, `SELECT body FROM slices WHERE id = ?`, id).Scan(&body)
	if err == nil {
		return strings.TrimSpace(body), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", l.errorf(err, "read the slice body")
	}
	err = l.db.QueryRowContext(ctx, `SELECT conventions FROM project WHERE id = ?`, id).Scan(&body)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", nil
	case err != nil:
		return "", l.errorf(err, "read the project conventions")
	}
	return strings.TrimSpace(body), nil
}

// PRDescription reads the pull request description a hand-back filed on a
// slice, which is a section of its body: the lines under a PR description
// heading, up to the next heading of the same or higher level. The rule is
// notion.PRDescriptionOf's exactly — the last such section wins, since a slice
// handed back twice carries one per hand-back and the one written last
// describes the work as it now stands — applied to markdown rather than to
// blocks, because here the markdown is what is stored.
func (l *Local) PRDescription(ctx context.Context, id string) (string, error) {
	body, err := l.Body(ctx, id)
	if err != nil {
		return "", err
	}
	return lastMarkdownSection(body, notion.PRDescriptionHeading), nil
}

// HandbackSummaryOf is the note a Done slice's last hand-back left on its
// page: the summary of what was done, filed under whichever heading closing
// it out wrote — see noteHeading in notion.go. Both headings are read here
// rather than one, because which of them a Done slice carries depends on how
// it got there: Summary for one closed straight to Done, Handed back for one
// closed via a merge that landed after its branch was reviewed. It works on
// the markdown [Store.Body] already reads, since the note is written there
// rather than as a property of its own, so a caller building a milestone
// digest needs nothing more of a store than the body it already has to fetch
// for every Done sibling.
func HandbackSummaryOf(body string) string {
	return lastMarkdownSection(body, summaryHeading, handedBackHeading)
}

// lastMarkdownSection is the rule [Local.PRDescription] and [HandbackSummaryOf]
// both apply: the blocks under the last heading matching any of the given
// names, up to the next heading of the same or higher level. A fenced block is
// passed over whole, so a section quoting a diff or a shell session is not cut
// short by a line of its own that happens to start with a hash.
func lastMarkdownSection(body string, headings ...string) string {
	var section []string
	level, fence := 0, ""
	for _, line := range strings.Split(body, "\n") {
		if f := fenceOf(line); f != "" {
			switch {
			case fence == "":
				fence = f
			case strings.HasPrefix(f, fence):
				fence = ""
			}
		}
		h, text := 0, ""
		if fence == "" {
			h, text = headingOf(line)
		}
		if h > 0 && h <= level {
			level = 0
		}
		if h > 0 && matchesHeading(text, headings) {
			level, section = h, nil
			continue
		}
		if level > 0 {
			section = append(section, line)
		}
	}
	return strings.TrimSpace(strings.Join(section, "\n"))
}

// matchesHeading reports whether text names one of the given headings,
// case-insensitively.
func matchesHeading(text string, headings []string) bool {
	for _, h := range headings {
		if strings.EqualFold(text, h) {
			return true
		}
	}
	return false
}

// headingOf is the level of an ATX heading and the text of it, and zero for a
// line that is not one. One to six hashes and then a space, which is what every
// heading nat writes is and what goldmark reads.
func headingOf(line string) (int, string) {
	t := strings.TrimLeft(line, " ")
	n := len(t) - len(strings.TrimLeft(t, "#"))
	if n < 1 || n > 6 || !strings.HasPrefix(t[n:], " ") {
		return 0, ""
	}
	return n, strings.TrimSpace(strings.TrimRight(t[n:], "#"))
}

// fenceOf is the run of backticks or tildes a fenced code block opens and
// closes with, and "" for a line that is neither. The closing fence must be at
// least as long as the one that opened it, which is what the caller compares.
func fenceOf(line string) string {
	t := strings.TrimLeft(line, " ")
	for _, r := range "`~" {
		f := strings.Repeat(string(r), 3)
		if strings.HasPrefix(t, f) {
			return t[:len(t)-len(strings.TrimLeft(t, string(r)))]
		}
	}
	return ""
}

// timeStamp is a moment as the plan stores one — a column of its own type
// rather than a driver's guess at one, since every stamp here is compared and
// sorted in Go and never in SQL. A zero moment, which is a stamp nothing has
// ever set, is stored as NULL rather than as a time that sorts before every
// other, so "never" cannot be mistaken for "long ago".
func timeStamp(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// parseTimeStamp reads a column [timeStamp] wrote, and false for a NULL one —
// which is either a page nothing has stamped yet, or one with no sync row at
// all, and the two callers that read this both treat them alike.
func parseTimeStamp(s sql.NullString) (time.Time, bool, error) {
	if !s.Valid {
		return time.Time{}, false, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s.String)
	if err != nil {
		return time.Time{}, false, err
	}
	return t, true, nil
}

// boolColumn is a bool as the plan stores one — SQLite has no boolean type of
// its own, so [domain.Milestone]-and-Slice-shaped columns already spell it as
// an integer (see slices.status's sibling columns), and the project's shape
// columns follow the same rule.
func boolColumn(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Sessions reads every ad hoc session filed against a project, in the order
// they were started — the project argument is unread, exactly as it is for
// [Local.slices]: a local file holds one project's sessions and there is
// nothing else they could belong to.
func (l *Local) Sessions(ctx context.Context, _ Project) ([]domain.Session, error) {
	rows, err := l.db.QueryContext(ctx,
		`SELECT id, started_at, dir, branch, ended_at FROM sessions ORDER BY started_at, id`)
	if err != nil {
		return nil, l.errorf(err, "read the sessions")
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Session
	for rows.Next() {
		var s domain.Session
		var started string
		var ended sql.NullString
		if err := rows.Scan(&s.ID, &started, &s.Dir, &s.Branch, &ended); err != nil {
			return nil, l.errorf(err, "read a session")
		}
		startedAt, _, err := parseTimeStamp(sql.NullString{String: started, Valid: started != ""})
		if err != nil {
			return nil, l.errorf(err, "read a session's start time")
		}
		s.StartedAt = startedAt
		endedAt, ok, err := parseTimeStamp(ended)
		if err != nil {
			return nil, l.errorf(err, "read a session's end time")
		}
		if ok {
			s.EndedAt = endedAt
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, l.errorf(err, "read the sessions")
	}
	return out, nil
}

// Dirty reports whether a slice's file copy is ahead of the workspace: written
// locally since it was last pushed, or never pushed at all. A slice with no
// sync row — every slice a plan of its own ever writes, since only a pull or a
// push touches that table — is clean: there is no workspace for it to be ahead
// of.
func (l *Local) Dirty(ctx context.Context, id string) (bool, error) {
	var dirty bool
	err := l.db.QueryRowContext(ctx, `SELECT dirty FROM sync WHERE slice_id = ?`, id).Scan(&dirty)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, l.errorf(err, "read the slice's sync state")
	}
	return dirty, nil
}

// LastSynced is when a slice's file copy was last brought into line with the
// workspace — read fresh by [Local.Hydrate] or [Local.TakeSlice], or pushed by
// [Local.MarkSent] — and false where it never has been.
func (l *Local) LastSynced(ctx context.Context, id string) (time.Time, bool, error) {
	var at sql.NullString
	err := l.db.QueryRowContext(ctx, `SELECT synced_at FROM sync WHERE slice_id = ?`, id).Scan(&at)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return time.Time{}, false, nil
	case err != nil:
		return time.Time{}, false, l.errorf(err, "read the slice's sync state")
	}
	t, ok, err := parseTimeStamp(at)
	if err != nil {
		return time.Time{}, false, l.errorf(err, "read the slice's sync state")
	}
	return t, ok, nil
}

// BodyFresh reports whether the prose held for a page — a slice's brief, or a
// project's conventions — was read since the given moment, and false both for
// prose that has never been stamped and for an ID neither a slice nor a
// project answers to: there is nothing fresh about a page that was never
// fetched, or that is not there at all.
func (l *Local) BodyFresh(ctx context.Context, id string, since time.Time) (bool, error) {
	var at sql.NullString
	err := l.db.QueryRowContext(ctx, `SELECT body_at FROM slices WHERE id = ?`, id).Scan(&at)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		err = l.db.QueryRowContext(ctx, `SELECT conventions_at FROM project WHERE id = ?`, id).Scan(&at)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, l.errorf(err, "read the page's freshness")
		}
	case err != nil:
		return false, l.errorf(err, "read the page's freshness")
	}
	t, ok, err := parseTimeStamp(at)
	if err != nil {
		return false, l.errorf(err, "read the page's freshness")
	}
	if !ok {
		return false, nil
	}
	return !t.Before(since), nil
}
