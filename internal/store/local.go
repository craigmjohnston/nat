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
const localSchemaVersion = 1

// localSchema is the plan as tables. Every column maps one-to-one onto
// [domain.Slice] or [domain.Milestone], so this store produces the same structs
// the Notion mapper does and everything above them — domain.Blockers, StateOf,
// HandedBack, the board, next-slice — is untouched.
//
// body and conventions hold markdown verbatim, which is the brief exactly as an
// agent receives it: the one constraint the format could not trade away.
//
// The full-text index the design settles on is not here: nothing yet searches a
// plan, and an index kept in step by triggers is machinery to write when there
// is a query to serve. user_version is what makes adding it later one
// migration rather than a second format.
const localSchema = `
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

// migrate brings the file up to the schema this build speaks, and is what every
// open runs: an empty file becomes an empty plan, a plan already at this
// version is read and left alone, and a plan from a later build is refused
// rather than read through a schema that is not its own.
func (l *Local) migrate(ctx context.Context) error {
	var version int
	if err := l.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return l.errorf(err, "read the plan")
	}
	switch {
	case version == localSchemaVersion:
		return nil
	case version > localSchemaVersion:
		return fmt.Errorf("the plan at %s was written by a newer nat (schema %d, this build reads %d)",
			l.path, version, localSchemaVersion)
	}
	// The stamp goes on in the same statement as the tables, so a plan is never
	// left holding the schema without the version that says which schema it is.
	// PRAGMA takes no parameters, and the value is a constant of this build's
	// own rather than anything read from the file.
	if _, err := l.db.ExecContext(ctx,
		localSchema+fmt.Sprintf("\nPRAGMA user_version = %d;\n", localSchemaVersion)); err != nil {
		return l.errorf(err, "create the plan")
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

// localShape is what a local plan can record about a slice, which is
// everything: the columns are this store's own and there is no project old
// enough to be missing one, so the two questions a Notion shape exists to
// answer are both yes here.
func (l *Local) localShape(ms []domain.Milestone) Shape {
	return Shape{HasAssignee: true, HasBranch: true, Milestones: ms}
}

// Shape reads what can be recorded about a project's slices and the milestones
// there are to file one under, without reading the slices themselves.
func (l *Local) Shape(ctx context.Context, _ Project) (Shape, error) {
	ms, err := l.milestones(ctx, l.db)
	if err != nil {
		return Shape{}, err
	}
	return l.localShape(ms), nil
}

// milestones reads the plan's milestones in plan order. A milestone is nothing
// but a name and a place, exactly as domain already says: its status is
// computed from the slices under it, so there is nothing else to store.
func (l *Local) milestones(ctx context.Context, q localQuerier) ([]domain.Milestone, error) {
	rows, err := q.QueryContext(ctx, `SELECT name, position FROM milestones ORDER BY position, name`)
	if err != nil {
		return nil, l.errorf(err, "read the milestones")
	}
	defer func() { _ = rows.Close() }()

	var ms []domain.Milestone
	for rows.Next() {
		var m domain.Milestone
		if err := rows.Scan(&m.Name, &m.Order); err != nil {
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
	return Plan{
		Project: domain.NewProject(p.ID, name, ms, slices),
		Shape:   l.localShape(ms),
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

// localSliceColumns is the one list of columns every slice read selects, so the
// scan below can be one function rather than one per query.
const localSliceColumns = `id, title, status, COALESCE(milestone, ''), assignee, repo, branch, pr`

// scanLocalSlice reads one row of [localSliceColumns] into the app's own words.
// A local plan writes exactly the statuses domain names, so the status is both
// the workflow status and what the project calls it; the assignee is a name and
// an identity at once, since there is no directory of users behind a plan kept
// in a file and the string a claim wrote is the string an ownership check
// compares against.
func scanLocalSlice(scan func(...any) error) (domain.Slice, error) {
	var s domain.Slice
	var status, assignee string
	if err := scan(&s.ID, &s.Name, &status, &s.MilestoneID, &assignee, &s.Repo, &s.Branch, &s.PRURL); err != nil {
		return domain.Slice{}, err
	}
	s.Status, s.StatusName = domain.SliceStatus(status), status
	if assignee != "" {
		s.AssigneeName, s.AssigneeIDs = assignee, []string{assignee}
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
// for every slice in the file.
func (l *Local) Slice(ctx context.Context, id string) (domain.Slice, Shape, error) {
	s, err := l.slice(ctx, l.db, id)
	if err != nil {
		return domain.Slice{}, Shape{}, err
	}
	return s, Shape{HasAssignee: true, HasBranch: true}, nil
}

// slice reads one slice through whichever querier it is given, which is how a
// write re-reads the slice it is about inside its own transaction.
func (l *Local) slice(ctx context.Context, q localQuerier, id string) (domain.Slice, error) {
	row := q.QueryRowContext(ctx, `SELECT `+localSliceColumns+` FROM slices WHERE id = ?`, id)
	s, err := scanLocalSlice(row.Scan)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return domain.Slice{}, fmt.Errorf("no slice %s in the plan at %s", id, l.path)
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

// lastMarkdownSection is that rule. A fenced block is passed over whole, so a
// PR description quoting a diff or a shell session is not cut short by a line
// of its own that happens to start with a hash.
func lastMarkdownSection(body, heading string) string {
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
		if h > 0 && strings.EqualFold(text, heading) {
			level, section = h, nil
			continue
		}
		if level > 0 {
			section = append(section, line)
		}
	}
	return strings.TrimSpace(strings.Join(section, "\n"))
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
