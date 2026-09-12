package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// This is the write half of [Local]. Two rules run through all of it, and they
// are what the board and several agents' `nat` commands writing one file at
// once come down to.
//
// The first is that a mutation is one transaction. The design this store was
// drawn up against wrote the plan through a temp file and a rename, so that a
// half-written plan was never visible; a database gives that for nothing and
// gives it better — the write is a transaction, so nothing reads the half of it,
// and where a rename publishes one writer's whole idea of the plan over
// another's, a transaction touches only the rows it is about. Nothing here
// writes the plan: everything writes a row of it.
//
// The second is that a mutation reads before it writes, inside that same
// transaction, and writes what the reading says rather than what the caller was
// last told. A slice's body is the clearest case — a note is appended to the
// body as it stands, so a brief edited while an agent worked is still there
// afterwards — but it is the rule everywhere, because the caller's copy of a
// slice is as old as whenever it read one and an agent has been writing since.

// withTx runs one mutation as a single transaction. It is BEGIN IMMEDIATE by
// the DSN, so the write lock is taken before the first read and a second writer
// waits its turn out rather than failing; the rollback is unconditional, since
// rolling back a committed transaction is how database/sql says "nothing to
// undo" and the alternative is a transaction left open by an early return.
func (l *Local) withTx(ctx context.Context, doing string, f func(*sql.Tx) error) error {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return l.errorf(err, doing)
	}
	defer func() { _ = tx.Rollback() }()
	if err := f(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return l.errorf(err, doing)
	}
	return nil
}

// exec runs one statement, saying in the store's own words what it was doing
// and which file it was doing it in.
func (l *Local) exec(ctx context.Context, tx *sql.Tx, doing, query string, args ...any) error {
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return l.errorf(err, doing)
	}
	return nil
}

// updateSlice is the shape of every write to one slice: the slice is read
// afresh inside the transaction, the mutation is handed that reading to write
// its delta onto, and the slice as the write left it is read back before the
// commit — which is what the caller gets, so what comes back is the plan's own
// answer rather than the caller's hopes for it.
//
// A slice that is not there is refused by the read, so a write to a slice
// somebody has deleted says so instead of quietly updating no rows.
func (l *Local) updateSlice(ctx context.Context, id, doing string,
	f func(tx *sql.Tx, before domain.Slice) error) (domain.Slice, error) {
	var after domain.Slice
	err := l.withTx(ctx, doing, func(tx *sql.Tx) error {
		before, err := l.slice(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := f(tx, before); err != nil {
			return err
		}
		after, err = l.slice(ctx, tx, id)
		return err
	})
	if err != nil {
		return domain.Slice{}, err
	}
	return after, nil
}

// sliceBody reads a slice's body as it stands, which is what a note is appended
// to. It is the raw column rather than [Local.Body]'s trimmed answer, because
// what is being written back is the body itself.
func (l *Local) sliceBody(ctx context.Context, tx *sql.Tx, id string) (string, error) {
	var body string
	if err := tx.QueryRowContext(ctx, `SELECT body FROM slices WHERE id = ?`, id).Scan(&body); err != nil {
		return "", l.errorf(err, "read the slice body")
	}
	return body, nil
}

// appendSection adds a note to a body under a heading of its own, which is how
// a local plan says what Notion says with a heading block and paragraphs under
// it: the headings are the same words, so a plan moved from one to the other
// reads the same and [lastMarkdownSection] finds a PR description either way.
func appendSection(body, heading, text string) string {
	return appendLines(body, "### "+heading, strings.TrimSpace(text))
}

// appendLines adds chunks to the end of a body, each separated from what was
// there by a blank line, and drops any that are empty.
func appendLines(body string, chunks ...string) string {
	out := strings.TrimRight(body, "\n")
	for _, c := range chunks {
		if c == "" {
			continue
		}
		if out != "" {
			out += "\n\n"
		}
		out += c
	}
	return out
}

// ClaimSlice takes the slice: in progress, and held by the given user where the
// caller's shape records ownership at all and there is a user to name. A shape
// that records none leaves whatever the assignee column already held, exactly
// as the Notion store leaves such a project's alone.
func (l *Local) ClaimSlice(ctx context.Context, id string, sh Shape, userID string) (domain.Slice, error) {
	s, err := l.updateSlice(ctx, id, "claim the slice", func(tx *sql.Tx, before domain.Slice) error {
		assignee := before.AssigneeName
		if sh.HasAssignee && userID != "" {
			assignee = userID
		}
		return l.exec(ctx, tx, "claim the slice",
			`UPDATE slices SET status = ?, assignee = ? WHERE id = ?`, notion.SliceInProgress, assignee, id)
	})
	if err != nil {
		return domain.Slice{}, err
	}
	logging.Action("slice claimed", "slice", s.ID, "name", s.Name, "user", userID)
	return s, nil
}

// ReleaseSlice hands the slice back to the plan: Todo, held by nobody where the
// shape says ownership is recorded, and a line on the page naming who let it
// go. Nothing else about the slice is touched — the brief, the dependencies,
// the repo and any branch are exactly the work so far the next session wants.
//
// Where the Notion store writes the line and the status as two requests and
// takes care which order they fail in, this is one transaction: either the
// slice is released and says so, or nothing happened at all.
func (l *Local) ReleaseSlice(ctx context.Context, id string, sh Shape, by string) (domain.Slice, error) {
	s, err := l.updateSlice(ctx, id, "release the slice", func(tx *sql.Tx, _ domain.Slice) error {
		body, err := l.sliceBody(ctx, tx, id)
		if err != nil {
			return err
		}
		if sh.HasAssignee {
			return l.exec(ctx, tx, "release the slice",
				`UPDATE slices SET status = ?, assignee = '', body = ? WHERE id = ?`,
				notion.SliceTodo, appendLines(body, releasedLine(by)), id)
		}
		return l.exec(ctx, tx, "release the slice",
			`UPDATE slices SET status = ?, body = ? WHERE id = ?`,
			notion.SliceTodo, appendLines(body, releasedLine(by)), id)
	})
	if err != nil {
		return domain.Slice{}, err
	}
	logging.Action("slice released", "slice", s.ID, "name", s.Name)
	return s, nil
}

// CompleteSlice closes the slice out: the summary filed on its body under a
// heading naming the ending, the pull request description beside it under one
// of its own where the hand-back carried one, and whichever properties the
// ending calls for.
//
// The body it appends to is the body as the transaction reads it, not as the
// caller last saw it, which is the whole of what re-reading buys: a brief the
// user edited on the board while the agent worked is still there under the
// note.
func (l *Local) CompleteSlice(ctx context.Context, id string, _ Shape, o Outcome) (domain.Slice, error) {
	s, err := l.updateSlice(ctx, id, "close out the slice", func(tx *sql.Tx, before domain.Slice) error {
		body, err := l.sliceBody(ctx, tx, id)
		if err != nil {
			return err
		}
		body = appendSection(body, noteHeading(o), o.Summary)
		if o.PRDescription != "" {
			body = appendSection(body, notion.PRDescriptionHeading, o.PRDescription)
		}
		status := string(before.Status)
		if o.done() {
			status = notion.SliceDone
		}
		branch, pr := before.Branch, before.PRURL
		if o.Branch != "" {
			branch = o.Branch
		}
		if o.PR != "" {
			pr = o.PR
		}
		return l.exec(ctx, tx, "close out the slice",
			`UPDATE slices SET status = ?, branch = ?, pr = ?, body = ? WHERE id = ?`,
			status, branch, pr, body, id)
	})
	if err != nil {
		return domain.Slice{}, err
	}
	logging.Action("slice closed out", "slice", s.ID, "blocked", o.Blocked, "pr", o.PR, "branch", o.Branch)
	return s, nil
}

// RecordPR writes a pull request onto a slice and nothing else. The slice stays
// where it is: the merge is what marks the work landed.
func (l *Local) RecordPR(ctx context.Context, id, url string) error {
	_, err := l.updateSlice(ctx, id, "record the pull request", func(tx *sql.Tx, _ domain.Slice) error {
		return l.exec(ctx, tx, "record the pull request",
			`UPDATE slices SET pr = ? WHERE id = ?`, url, id)
	})
	return err
}

// MarkDone moves a slice to Done, which is the one write that says its work is
// on main.
func (l *Local) MarkDone(ctx context.Context, id string, _ Shape) error {
	if _, err := l.updateSlice(ctx, id, "mark the slice Done", func(tx *sql.Tx, _ domain.Slice) error {
		return l.exec(ctx, tx, "mark the slice Done",
			`UPDATE slices SET status = ? WHERE id = ?`, notion.SliceDone, id)
	}); err != nil {
		return err
	}
	logging.Action("slice marked Done", "slice", id)
	return nil
}

// AddMilestones files milestones at the end of the plan, all of them or none.
//
// The names they are refused for clashing with are the plan's own as the
// transaction reads them rather than the shape the caller was handed, since a
// milestone added by an agent since that read is one this run would otherwise
// duplicate — and a milestone here, as in Notion, is nothing but its name, so
// two of a name could not be told apart.
func (l *Local) AddMilestones(ctx context.Context, _ Project, _ Shape, names []string) ([]domain.Milestone, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var added []domain.Milestone
	err := l.withTx(ctx, "create the "+pluralise("milestone", len(names)), func(tx *sql.Tx) error {
		existing, err := l.milestones(ctx, tx)
		if err != nil {
			return err
		}
		taken := map[string]string{}
		for _, m := range existing {
			taken[strings.ToLower(strings.TrimSpace(m.Name))] = m.Name
		}
		added = nil
		next := float64(len(existing))
		for _, name := range names {
			key := strings.ToLower(strings.TrimSpace(name))
			if held, dup := taken[key]; dup {
				return fmt.Errorf("the plan at %s already has a milestone named %q: "+
					"a milestone is nothing but its name, and a plan cannot hold two of one", l.path, held)
			}
			taken[key] = name
			if err := l.exec(ctx, tx, "create the milestone",
				`INSERT INTO milestones (name, position) VALUES (?, ?)`, name, next); err != nil {
				return err
			}
			added = append(added, domain.Milestone{
				ID: name, Name: name, Order: next, Status: domain.MilestoneStatusOf(nil),
			})
			next++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, m := range added {
		logging.Action("milestone added", "milestone", m.Name, "order", m.Order)
	}
	return added, nil
}

// RenameMilestone gives one milestone another name, in place: its position is
// the column the plan is ordered by and is left alone, and the slices filed
// under it are carried over with it.
//
// It is one transaction and it reads inside it, so the names it refuses over —
// a new name the plan already holds, an old name it does not — are the plan's
// own as of the write rather than whatever the caller was last handed. There is
// no long way round here: a milestone's name is a column of its own row and a
// foreign key nothing enforces, so renaming it is an update of two tables.
func (l *Local) RenameMilestone(ctx context.Context, _ Project, _ Shape, old, name string) (domain.Milestone, error) {
	var renamed domain.Milestone
	err := l.withTx(ctx, "rename the milestone", func(tx *sql.Tx) error {
		existing, err := l.milestones(ctx, tx)
		if err != nil {
			return err
		}
		from, err := renameTargets(existing, old, name, func(held string) error {
			return fmt.Errorf("the plan at %s already has a milestone named %q: "+
				"a milestone is nothing but its name, and a plan cannot hold two of one", l.path, held)
		}, func() error {
			return fmt.Errorf("the plan at %s has no milestone named %q: its milestones are %s",
				l.path, old, milestoneList(existing))
		})
		if err != nil {
			return err
		}
		// The slices are read before either write, so a plan that cannot be read
		// is a rename that has changed nothing — and they are what the renamed
		// milestone's status is computed from, a milestone having none of its own.
		all, err := l.slices(ctx, tx)
		if err != nil {
			return err
		}
		var under []domain.Slice
		for _, s := range all {
			if s.MilestoneID == from.Name {
				under = append(under, s)
			}
		}
		if err := l.exec(ctx, tx, "rename the milestone",
			`UPDATE milestones SET name = ? WHERE name = ?`, name, from.Name); err != nil {
			return err
		}
		if err := l.exec(ctx, tx, "refile the milestone's slices",
			`UPDATE slices SET milestone = ? WHERE milestone = ?`, name, from.Name); err != nil {
			return err
		}
		renamed = domain.Milestone{
			ID: name, Name: name, Order: from.Order, Status: domain.MilestoneStatusOf(under),
		}
		return nil
	})
	if err != nil {
		return domain.Milestone{}, err
	}
	logging.Action("milestone renamed", "from", old, "to", renamed.Name, "order", renamed.Order)
	return renamed, nil
}

// RemoveMilestone drops a milestone from the plan and nothing else, which here
// is one row gone and the rows after it closed up behind it: a position is
// allocated from how many milestones there are, so leaving a gap would have the
// next milestone added land on the position of one already there.
//
// It is one transaction and it reads inside it, so both refusals — a name the
// plan does not hold, and a milestone with slices still filed under it — are
// about the plan as of the write rather than whatever the caller was last
// handed, which for the second matters: a slice filed under the milestone by an
// agent since that read is exactly the one this must not orphan.
func (l *Local) RemoveMilestone(ctx context.Context, _ Project, _ Shape, name string) (domain.Milestone, error) {
	var removed domain.Milestone
	err := l.withTx(ctx, "remove the milestone", func(tx *sql.Tx) error {
		existing, err := l.milestones(ctx, tx)
		if err != nil {
			return err
		}
		from, found := milestoneNamed(existing, name)
		if !found {
			return fmt.Errorf("the plan at %s has no milestone named %q: its milestones are %s",
				l.path, strings.TrimSpace(name), milestoneList(existing))
		}
		all, err := l.slices(ctx, tx)
		if err != nil {
			return err
		}
		var under []string
		for _, s := range all {
			if s.MilestoneID == from.Name {
				under = append(under, s.Name)
			}
		}
		if len(under) > 0 {
			return fmt.Errorf("the milestone %q in the plan at %s %s", from.Name, l.path, stillFiledNote(under))
		}
		if err := l.exec(ctx, tx, "remove the milestone",
			`DELETE FROM milestones WHERE name = ?`, from.Name); err != nil {
			return err
		}
		if err := l.exec(ctx, tx, "close the milestone's place in the plan",
			`UPDATE milestones SET position = position - 1 WHERE position > ?`, from.Order); err != nil {
			return err
		}
		// Queued rather than computed: nothing is filed under it, which is the
		// whole of what this command will remove.
		removed = domain.Milestone{
			ID: from.Name, Name: from.Name, Order: from.Order, Status: domain.MilestoneStatusOf(nil),
		}
		return nil
	})
	if err != nil {
		return domain.Milestone{}, err
	}
	logging.Action("milestone removed", "milestone", removed.Name, "order", removed.Order)
	return removed, nil
}

// newLocalID is the ID a newly filed slice takes. Notion hands back a page ID
// and a local plan has nobody to ask, so one is made here, in the shape of the
// IDs everything above this package already passes about — a caller only ever
// reads one back and hands it in again.
func newLocalID() string {
	var b [16]byte
	// crypto/rand.Read fills the slice or stops the program; it has no failure
	// a caller could act on, which is why the error is not one.
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// AddSlice files one slice under a milestone, Todo and unclaimed: status and
// ownership are not the caller's to choose, or the slice is not something the
// workflow can hand out.
//
// The milestone has to be one the plan holds, which is what Notion's own
// select column enforces for the other store: a slice filed under a milestone
// nothing knows about is one the board draws nowhere.
func (l *Local) AddSlice(ctx context.Context, _ Project, n NewSlice) (domain.Slice, error) {
	id := newLocalID()
	var added domain.Slice
	err := l.withTx(ctx, "add the slice", func(tx *sql.Tx) error {
		if err := l.checkMilestone(ctx, tx, n.Milestone.ID); err != nil {
			return err
		}
		position, err := l.nextSlicePosition(ctx, tx)
		if err != nil {
			return err
		}
		if err := l.exec(ctx, tx, "add the slice",
			`INSERT INTO slices (id, title, status, milestone, position, repo, body)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, n.Title, notion.SliceTodo, nullable(n.Milestone.ID), position, n.Repo, n.Brief); err != nil {
			return err
		}
		if err := l.writeDependencies(ctx, tx, id, n.DependsOn); err != nil {
			return err
		}
		added, err = l.slice(ctx, tx, id)
		return err
	})
	if err != nil {
		return domain.Slice{}, err
	}
	logging.Action("slice added", "slice", added.ID, "name", added.Name, "milestone", added.MilestoneID)
	return added, nil
}

// nextSlicePosition is where a newly filed slice goes, which is the end of the
// plan. The column is a real number so that a slice can one day be put between
// two others without renumbering the plan; appending needs nothing of that but
// a number past the last one.
func (l *Local) nextSlicePosition(ctx context.Context, tx *sql.Tx) (float64, error) {
	var last float64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(position), -1) FROM slices`).Scan(&last); err != nil {
		return 0, l.errorf(err, "read the plan order")
	}
	return last + 1, nil
}

// checkMilestone refuses a milestone the plan does not hold. The empty name is
// a slice filed under no milestone at all, which the plan draws on its own and
// which the schema records as NULL.
func (l *Local) checkMilestone(ctx context.Context, tx *sql.Tx, name string) error {
	if name == "" {
		return nil
	}
	var held string
	err := tx.QueryRowContext(ctx, `SELECT name FROM milestones WHERE name = ?`, name).Scan(&held)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("the plan at %s has no milestone named %q", l.path, name)
	case err != nil:
		return l.errorf(err, "read the milestone")
	}
	return nil
}

// nullable is a milestone name as the column holds it: NULL for a slice under
// no milestone, so that "no milestone" is one value rather than two.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// writeDependencies records exactly the slices a slice waits on, in the order
// given, replacing whatever was recorded before. A slice that is not in the
// plan is refused by the foreign key, which is what makes a wait with no end
// impossible rather than merely wrong.
func (l *Local) writeDependencies(ctx context.Context, tx *sql.Tx, id string, on []string) error {
	if err := l.exec(ctx, tx, "clear the slice's dependencies",
		`DELETE FROM slice_deps WHERE slice_id = ?`, id); err != nil {
		return err
	}
	for i, dep := range on {
		if err := l.exec(ctx, tx, "record the slice's dependencies",
			`INSERT INTO slice_deps (slice_id, depends_on, position) VALUES (?, ?, ?)`,
			id, dep, i); err != nil {
			return err
		}
	}
	return nil
}

// EditSlice rewrites a slice's title, working directory and brief in one write.
// Its milestone is left alone — refiling a slice is its own operation — and so
// is its status, which only the workflow changes.
func (l *Local) EditSlice(ctx context.Context, id, title, repo, brief string) error {
	_, err := l.updateSlice(ctx, id, "update the slice", func(tx *sql.Tx, _ domain.Slice) error {
		return l.exec(ctx, tx, "update the slice",
			`UPDATE slices SET title = ?, repo = ?, body = ? WHERE id = ?`, title, repo, brief, id)
	})
	return err
}

// SetSliceBrief rewrites a slice's brief and nothing else about it.
func (l *Local) SetSliceBrief(ctx context.Context, id, brief string) error {
	_, err := l.updateSlice(ctx, id, "write the slice brief", func(tx *sql.Tx, _ domain.Slice) error {
		return l.exec(ctx, tx, "write the slice brief",
			`UPDATE slices SET body = ? WHERE id = ?`, brief, id)
	})
	return err
}

// SetDependencies records exactly the slices a slice waits on, replacing
// whatever it waited on before — an empty list being how a slice is freed.
func (l *Local) SetDependencies(ctx context.Context, id string, on []string) (domain.Slice, error) {
	s, err := l.updateSlice(ctx, id, "record the slice's dependencies", func(tx *sql.Tx, _ domain.Slice) error {
		return l.writeDependencies(ctx, tx, id, on)
	})
	if err != nil {
		return domain.Slice{}, err
	}
	logging.Action("slice dependencies recorded", "slice", s.ID, "depends_on", len(s.DependsOn))
	return s, nil
}

// MoveSlice refiles a slice under another milestone. The work itself — its
// brief, its status, its repo — says nothing about where in the plan it sits
// and is untouched.
func (l *Local) MoveSlice(ctx context.Context, id string, m domain.Milestone) error {
	if _, err := l.updateSlice(ctx, id, "move the slice", func(tx *sql.Tx, _ domain.Slice) error {
		if err := l.checkMilestone(ctx, tx, m.ID); err != nil {
			return err
		}
		return l.exec(ctx, tx, "move the slice",
			`UPDATE slices SET milestone = ? WHERE id = ?`, nullable(m.ID), id)
	}); err != nil {
		return err
	}
	logging.Action("slice moved", "slice", id, "milestone", m.ID)
	return nil
}

// DeleteSlice drops a slice from the plan, and with it every wait either side
// of it records: a dependency on a slice that is gone is a wait with no end,
// and one of the deleted slice's own is a row the foreign keys would not let
// go anyway.
//
// Notion's own delete is a move to a trash the user can undo from, and there is
// no such thing here. What a local plan offers instead is its file: one project
// per database, so a plan taken back is a file put back.
func (l *Local) DeleteSlice(ctx context.Context, id string) error {
	if err := l.withTx(ctx, "delete the slice", func(tx *sql.Tx) error {
		if _, err := l.slice(ctx, tx, id); err != nil {
			return err
		}
		if err := l.exec(ctx, tx, "delete the slice",
			`DELETE FROM slice_deps WHERE slice_id = ? OR depends_on = ?`, id, id); err != nil {
			return err
		}
		if err := l.exec(ctx, tx, "delete the slice",
			`DELETE FROM sync WHERE slice_id = ?`, id); err != nil {
			return err
		}
		return l.exec(ctx, tx, "delete the slice", `DELETE FROM slices WHERE id = ?`, id)
	}); err != nil {
		return err
	}
	logging.Action("slice deleted", "slice", id)
	return nil
}
