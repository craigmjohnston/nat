package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

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
// its delta onto, the slice is marked dirty in that same transaction — a
// replica's file is now ahead of the workspace on this slice, and the two must
// never disagree about that — and the slice as the write left it is read back
// before the commit — which is what the caller gets, so what comes back is the
// plan's own answer rather than the caller's hopes for it.
//
// The dirty write is inside the transaction on purpose, rather than a second
// call made after this one returns: a process killed between the two would
// leave a change in the file, marked nothing, that no sync would ever send —
// where a crash on the other side, between a push and marking a slice sent, only
// resends a write the workspace already has.
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
		if err := l.markDirty(ctx, tx, id); err != nil {
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

// markDirty records that a slice's file copy has changed since it was last
// brought into line with the workspace, inside whichever transaction the
// change itself is being written in.
func (l *Local) markDirty(ctx context.Context, tx *sql.Tx, id string) error {
	return l.exec(ctx, tx, "mark the slice dirty",
		`INSERT INTO sync (slice_id, dirty) VALUES (?, 1)
		 ON CONFLICT(slice_id) DO UPDATE SET dirty = 1`, id)
}

// markSynced records that a slice's file copy was just brought into line with
// the workspace — a pull, which is clean by definition, rather than a push,
// which [Local.MarkSent] is the answer to.
func (l *Local) markSynced(ctx context.Context, tx *sql.Tx, id string, at time.Time) error {
	return l.exec(ctx, tx, "mark the slice synced",
		`INSERT INTO sync (slice_id, dirty, synced_at) VALUES (?, 0, ?)
		 ON CONFLICT(slice_id) DO UPDATE SET dirty = 0, synced_at = excluded.synced_at`,
		id, timeStamp(at))
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

// ReopenSlice writes a slice back to In progress, MarkDone undone.
func (l *Local) ReopenSlice(ctx context.Context, id string, _ Shape) error {
	if _, err := l.updateSlice(ctx, id, "reopen the slice to In progress", func(tx *sql.Tx, _ domain.Slice) error {
		return l.exec(ctx, tx, "reopen the slice to In progress",
			`UPDATE slices SET status = ? WHERE id = ?`, notion.SliceInProgress, id)
	}); err != nil {
		return err
	}
	logging.Action("slice reopened to In progress", "slice", id)
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

// MoveMilestone moves a milestone to sit directly before or after another, which
// here is the position column of every milestone rewritten in the plan's new
// order. A position is allocated from how many milestones there are, so the
// whole plan is restamped densely from zero rather than a gap being opened for
// the one that moved.
//
// It is one transaction and it reads inside it, so all three refusals — a name
// the plan does not hold, a target it does not hold, and a move relative to the
// milestone itself — are about the plan as of the write rather than whatever the
// caller was last handed: a milestone another agent renamed or removed since
// that read is one this would otherwise move something relative to.
//
// Nothing about any milestone but its place changes, so no slice is touched and
// none is read — which is why nothing here says what status the moved milestone
// is in, that being the slices' answer and nobody having asked them.
func (l *Local) MoveMilestone(ctx context.Context, _ Project, _ Shape, name, target string, before bool) (domain.Milestone, domain.Milestone, error) {
	var moved, to domain.Milestone
	err := l.withTx(ctx, "move the milestone", func(tx *sql.Tx) error {
		existing, err := l.milestones(ctx, tx)
		if err != nil {
			return err
		}
		plan, m, t, err := moveTargets(existing, name, target, before, func(given string) error {
			return fmt.Errorf("the plan at %s has no milestone named %q: its milestones are %s",
				l.path, given, milestoneList(existing))
		}, func(held string) error {
			return fmt.Errorf("%q in the plan at %s cannot be moved relative to itself: "+
				"name the milestone it is to sit beside", held, l.path)
		})
		if err != nil {
			return err
		}
		for _, ms := range plan {
			if err := l.exec(ctx, tx, "reorder the plan",
				`UPDATE milestones SET position = ? WHERE name = ?`, ms.Order, ms.Name); err != nil {
				return err
			}
		}
		moved, to = m, t
		return nil
	})
	if err != nil {
		return domain.Milestone{}, domain.Milestone{}, err
	}
	logging.Action("milestone moved", "milestone", moved.Name, "order", moved.Order,
		"placement", placementWord(before), "relative to", to.Name)
	return moved, to, nil
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
		if err := l.markDirty(ctx, tx, id); err != nil {
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
		return l.deleteSliceRows(ctx, tx, id)
	}); err != nil {
		return err
	}
	logging.Action("slice deleted", "slice", id)
	return nil
}

// deleteSliceRows takes every row a slice owns out of the plan: the waits
// either side of it, its sync state, and the slice itself. It is the row-level
// work [Local.DeleteSlice] does once a slice's presence has been checked, and
// [Local.Hydrate] does again for every slice a reading no longer names, which
// has already read the plan and has no second read to make of one slice.
func (l *Local) deleteSliceRows(ctx context.Context, tx *sql.Tx, id string) error {
	if err := l.exec(ctx, tx, "delete the slice",
		`DELETE FROM slice_deps WHERE slice_id = ? OR depends_on = ?`, id, id); err != nil {
		return err
	}
	if err := l.exec(ctx, tx, "delete the slice",
		`DELETE FROM sync WHERE slice_id = ?`, id); err != nil {
		return err
	}
	return l.exec(ctx, tx, "delete the slice", `DELETE FROM slices WHERE id = ?`, id)
}

// This is the replica half of [Local]: the writes a project read from Notion
// but kept locally needs, and nothing above this package calls any of it yet.
//
// sliceIdentity is what a domain.Slice carries onto a row's assignee and
// assignee_name columns — the workspace's own user ID, which is what a push
// compares equality against, and the name it resolves that ID to, which is
// what [Local.ApplyAssignee] alone fills in afterwards. A reading with no
// assignee at all writes neither.
func sliceIdentity(s domain.Slice) (assignee, name string) {
	if len(s.AssigneeIDs) > 0 {
		assignee = s.AssigneeIDs[0]
	}
	return assignee, s.AssigneeName
}

// sliceStatusName is the word a write puts in the status column: the
// project's own word for it where the reading carries one, and the workflow
// status otherwise — a reading may carry a [domain.Slice] with a Status and no
// StatusName, and a plan storing neither would read back with no status at
// all.
func sliceStatusName(s domain.Slice) string {
	if s.StatusName != "" {
		return s.StatusName
	}
	return string(s.Status)
}

// existingSlicePosition is what [Local.Hydrate] already knows about a slice
// before it writes anything: the place in the plan it holds, the milestone
// that place is measured within, and whether it is dirty — ahead of the
// workspace rather than behind it, and so not [Local.Hydrate]'s to overwrite.
type existingSlicePosition struct {
	position  float64
	milestone string
	dirty     bool
}

// existingSlicePositions reads every slice the plan already holds, and with
// each its sync state, as one query rather than one per slice — the same
// shape [Local.dependencies] reads a whole plan's waits in.
func (l *Local) existingSlicePositions(ctx context.Context, q localQuerier) (map[string]existingSlicePosition, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT s.id, s.position, COALESCE(s.milestone, ''), COALESCE(y.dirty, 0)
		 FROM slices s LEFT JOIN sync y ON y.slice_id = s.id`)
	if err != nil {
		return nil, l.errorf(err, "read the plan")
	}
	defer func() { _ = rows.Close() }()

	out := map[string]existingSlicePosition{}
	for rows.Next() {
		var id string
		var ex existingSlicePosition
		if err := rows.Scan(&id, &ex.position, &ex.milestone, &ex.dirty); err != nil {
			return nil, l.errorf(err, "read the plan")
		}
		out[id] = ex
	}
	if err := rows.Err(); err != nil {
		return nil, l.errorf(err, "read the plan")
	}
	return out, nil
}

// nextSlicePositionInMilestone is where a slice new to the plan goes within
// its milestone — past the highest position anything already filed there
// holds — which is [Local.nextSlicePosition]'s rule applied within a milestone
// rather than across the whole plan: [Local.AddSlice] puts a slice at the end
// of the plan because that is where a person filing one by hand expects it,
// and [Local.Hydrate] and [Local.TakeSlice] put one at the end of its
// milestone because a reading names the milestone and nothing about the rest
// of the plan.
func (l *Local) nextSlicePositionInMilestone(ctx context.Context, tx *sql.Tx, milestone string) (float64, error) {
	var last sql.NullFloat64
	if err := tx.QueryRowContext(ctx,
		`SELECT MAX(position) FROM slices WHERE COALESCE(milestone, '') = ?`, milestone).Scan(&last); err != nil {
		return 0, l.errorf(err, "read the plan order")
	}
	if !last.Valid {
		return 0, nil
	}
	return last.Float64 + 1, nil
}

// sliceExists reports whether a slice is already in the plan, which is what
// [Local.TakeSlice] asks before filing one: a slice the file already holds is
// not new to it, whatever page prompted the read.
func (l *Local) sliceExists(ctx context.Context, q localQuerier, id string) (bool, error) {
	var found string
	err := q.QueryRowContext(ctx, `SELECT id FROM slices WHERE id = ?`, id).Scan(&found)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, l.errorf(err, "read the slice")
	}
	return true, nil
}

// TakeSlice puts one slice the file has never seen into the plan, at the end
// of its milestone: the one way a slice enters the file outside
// [Local.Hydrate] — for a single slice read from the workspace by ID, and
// equally the page [Local.AddSlice] has just had a write-through layer create
// there — which is what keeps the write-through slice from having to grow a
// second entry point of its own for a workspace-chosen ID.
//
// A slice the file already holds is left exactly where it is, position
// included: it is not new to the plan, whatever page prompted the read. body
// is written as the slice's own, stamped fresh as of at; an empty body is
// still stamped, since "" is what a slice with no brief yet genuinely holds
// and the stamp is what says that reading is current.
func (l *Local) TakeSlice(ctx context.Context, s domain.Slice, body string, at time.Time) error {
	return l.withTx(ctx, "take the slice into the plan", func(tx *sql.Tx) error {
		held, err := l.sliceExists(ctx, tx, s.ID)
		if err != nil {
			return err
		}
		if held {
			return nil
		}
		position, err := l.nextSlicePositionInMilestone(ctx, tx, s.MilestoneID)
		if err != nil {
			return err
		}
		assignee, assigneeName := sliceIdentity(s)
		if err := l.exec(ctx, tx, "take the slice into the plan",
			`INSERT INTO slices
			   (id, title, status, milestone, position, assignee, assignee_name, repo, branch, pr, body, body_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.ID, s.Name, sliceStatusName(s), nullable(s.MilestoneID), position,
			assignee, assigneeName, s.Repo, s.Branch, s.PRURL, body, timeStamp(at)); err != nil {
			return err
		}
		if err := l.writeDependencies(ctx, tx, s.ID, s.DependsOn); err != nil {
			return err
		}
		return l.markSynced(ctx, tx, s.ID, at)
	})
}

// Hydrate brings the file into line with a reading taken from the workspace:
// the project row, its milestones replaced wholesale — a milestone is nothing
// but its name and its place, and its order lives in Notion's select options
// rather than on any page, so the reading's answer for it is always current —
// every slice the reading holds written over what was there except its
// position, and the slices the reading does not name taken off.
//
// A slice the file already holds keeps the position it has, clean or dirty,
// and only a slice the file has never seen takes one, appended at the end of
// its milestone in the reading's own order. A slice marked dirty — the file
// ahead of the workspace on it, not behind — is left alone entirely: neither
// its fields, its dependencies, nor its body are written over, since the file
// is right about it and the reading is what is stale.
//
// bodies is keyed by page ID — a slice's own, or the project's for its
// conventions — and an absent entry leaves the stored prose exactly as it
// was, because a pull often carries no bodies at all.
func (l *Local) Hydrate(ctx context.Context, p Project, plan Plan, bodies map[string]string, at time.Time) error {
	return l.withTx(ctx, "hydrate the plan", func(tx *sql.Tx) error {
		if err := l.exec(ctx, tx, "hydrate the plan",
			`INSERT INTO project (id, name, has_assignee, has_branch, synced_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET
			   name = excluded.name, has_assignee = excluded.has_assignee,
			   has_branch = excluded.has_branch, synced_at = excluded.synced_at`,
			p.ID, plan.Project.Name, boolColumn(plan.Shape.HasAssignee), boolColumn(plan.Shape.HasBranch),
			timeStamp(at)); err != nil {
			return err
		}
		if body, ok := bodies[p.ID]; ok {
			if err := l.exec(ctx, tx, "hydrate the plan",
				`UPDATE project SET conventions = ?, conventions_at = ? WHERE id = ?`,
				body, timeStamp(at), p.ID); err != nil {
				return err
			}
		}

		if err := l.exec(ctx, tx, "hydrate the plan", `DELETE FROM milestones`); err != nil {
			return err
		}
		for _, m := range plan.Project.Milestones {
			if err := l.exec(ctx, tx, "hydrate the plan",
				`INSERT INTO milestones (name, position, select_type) VALUES (?, ?, ?)`,
				m.Name, m.Order, m.SelectType); err != nil {
				return err
			}
		}

		existing, err := l.existingSlicePositions(ctx, tx)
		if err != nil {
			return err
		}
		// Where the next new slice under a milestone lands, seeded from what the
		// file already holds there so a reading's own new slices append after it
		// rather than colliding with it.
		nextPos := map[string]float64{}
		for _, ex := range existing {
			if n := ex.position + 1; n > nextPos[ex.milestone] {
				nextPos[ex.milestone] = n
			}
		}

		seen := map[string]bool{}
		for _, s := range plan.Project.Slices {
			seen[s.ID] = true
			ex, held := existing[s.ID]
			if held && ex.dirty {
				continue
			}
			position := ex.position
			if !held {
				position = nextPos[s.MilestoneID]
				nextPos[s.MilestoneID] = position + 1
			}
			assignee, assigneeName := sliceIdentity(s)
			if err := l.exec(ctx, tx, "hydrate the plan",
				`INSERT INTO slices
				   (id, title, status, milestone, position, assignee, assignee_name, repo, branch, pr)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				 ON CONFLICT(id) DO UPDATE SET
				   title = excluded.title, status = excluded.status, milestone = excluded.milestone,
				   position = excluded.position, assignee = excluded.assignee,
				   assignee_name = excluded.assignee_name, repo = excluded.repo,
				   branch = excluded.branch, pr = excluded.pr`,
				s.ID, s.Name, sliceStatusName(s), nullable(s.MilestoneID), position,
				assignee, assigneeName, s.Repo, s.Branch, s.PRURL); err != nil {
				return err
			}
			if body, ok := bodies[s.ID]; ok {
				if err := l.exec(ctx, tx, "hydrate the plan",
					`UPDATE slices SET body = ?, body_at = ? WHERE id = ?`,
					body, timeStamp(at), s.ID); err != nil {
					return err
				}
			}
			if err := l.writeDependencies(ctx, tx, s.ID, s.DependsOn); err != nil {
				return err
			}
			if err := l.markSynced(ctx, tx, s.ID, at); err != nil {
				return err
			}
		}

		for id := range existing {
			if seen[id] {
				continue
			}
			if err := l.deleteSliceRows(ctx, tx, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetBody writes the prose kept against an ID, stamped as read as of at — a
// slice's brief, or a project's conventions, whichever the ID names, the same
// duality [Local.Body] reads. An ID neither answers to is refused: unlike
// [Local.Body], which is read against whatever project happens to be open,
// this is always writing back a page just fetched by its own ID, and a page
// that fetch found is a page this file ought to have a row for already.
func (l *Local) SetBody(ctx context.Context, id, body string, at time.Time) error {
	return l.withTx(ctx, "write the page body", func(tx *sql.Tx) error {
		n, err := l.tryExec(ctx, tx, "write the page body",
			`UPDATE slices SET body = ?, body_at = ? WHERE id = ?`, body, timeStamp(at), id)
		if err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		n, err = l.tryExec(ctx, tx, "write the page body",
			`UPDATE project SET conventions = ?, conventions_at = ? WHERE id = ?`, body, timeStamp(at), id)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("no page %s in the plan at %s", id, l.path)
		}
		return nil
	})
}

// tryExec is [Local.exec] with the rows it changed handed back, which is how
// [Local.SetBody] tells a slice's page from a project's without reading either
// first. RowsAffected cannot fail here — every statement this is given is a
// plain UPDATE against this driver, which always answers it — so there is no
// failure for a caller to act on, the same reason newLocalID does not check
// crypto/rand's.
func (l *Local) tryExec(ctx context.Context, tx *sql.Tx, doing, query string, args ...any) (int64, error) {
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, l.errorf(err, doing)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// MarkSent records that a slice's file copy has just been pushed to the
// workspace: dirty cleared, and the sync stamp set to when the push was made.
//
// It is deliberately its own step rather than something folded into the push
// itself: a crash between the push landing and this call simply has the next
// sync attempt send the same write again, which the workspace reads as the
// write it already has — where the other direction, a local write committed
// and never marked dirty, would lose that write from every sync to come. See
// [Local.updateSlice].
func (l *Local) MarkSent(ctx context.Context, id string, at time.Time) error {
	return l.withTx(ctx, "mark the slice sent", func(tx *sql.Tx) error {
		if _, err := l.slice(ctx, tx, id); err != nil {
			return err
		}
		return l.markSynced(ctx, tx, id, at)
	})
}

// ApplyAssignee records the workspace's own name for whoever holds a slice,
// and nothing else about it. It is deliberately this narrow: an earlier
// attempt wrote the whole slice the workspace answered a push with back over
// the file's copy, which was wrong twice over — the file was written a moment
// ago and is already right about everything else, and a workspace answering a
// partial write with a partial page would blank whatever it left out.
//
// This is not a change the file is ahead of the workspace on, so unlike
// [Local.updateSlice]'s writes it does not mark the slice dirty: it is telling
// the file what the workspace already agrees to.
func (l *Local) ApplyAssignee(ctx context.Context, id, name string) error {
	return l.withTx(ctx, "record the assignee's name", func(tx *sql.Tx) error {
		if _, err := l.slice(ctx, tx, id); err != nil {
			return err
		}
		return l.exec(ctx, tx, "record the assignee's name",
			`UPDATE slices SET assignee_name = ? WHERE id = ?`, name, id)
	})
}

// takeMilestones puts milestones the workspace has just created into the
// plan, in the order handed back — the local half of a milestone write, which
// [Mirrored.AddMilestones] calls only once the workspace's own write has
// already succeeded. Unlike a slice's, a milestone write never rides the
// dirty flag (see the design's own reasoning, restated in root CLAUDE.md), so
// there is nothing here for a sync to send later and nothing to mark.
//
// A name the file already holds is left exactly as it is: it is not new to
// the plan, whatever the workspace's own answer says, the same rule
// [Local.TakeSlice] applies to a slice.
func (l *Local) takeMilestones(ctx context.Context, ms []domain.Milestone) error {
	return l.withTx(ctx, "take the milestones into the plan", func(tx *sql.Tx) error {
		for _, m := range ms {
			held, err := l.milestoneExists(ctx, tx, m.Name)
			if err != nil {
				return err
			}
			if held {
				continue
			}
			if err := l.exec(ctx, tx, "take the milestones into the plan",
				`INSERT INTO milestones (name, position, select_type) VALUES (?, ?, ?)`,
				m.Name, m.Order, m.SelectType); err != nil {
				return err
			}
		}
		return nil
	})
}

// milestoneExists reports whether a milestone is already in the plan, which
// is what [Local.takeMilestones] asks before filing one.
func (l *Local) milestoneExists(ctx context.Context, q localQuerier, name string) (bool, error) {
	var found string
	err := q.QueryRowContext(ctx, `SELECT name FROM milestones WHERE name = ?`, name).Scan(&found)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, l.errorf(err, "read the milestones")
	}
	return true, nil
}
