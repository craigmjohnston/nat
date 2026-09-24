package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
)

// Mirrored is the store a project tracked in Notion reads and writes
// through: a [Local] replica in front of a [Notion] workspace. Every read
// comes from the file — Mirror wires the two together and nothing above this
// package reaches it yet.
//
// Every slice write lands in the file first, dirty set inside the same
// transaction the write itself is, and is pushed to the workspace afterwards,
// [Local.MarkSent] clearing the flag on success. A push that fails does not
// fail the write: the work is recorded, because the file is the plan, and the
// flag the write already set is what the next sync sends. It is deliberately
// not the write itself that sets the flag — a process killed before the push
// ever ran must still leave a slice that reads as unsent, which is exactly
// what [Local]'s own transaction already guarantees.
//
// Milestone writes are the one write that cannot follow that rule: a
// milestone's identity is its name, so a rename cannot be replayed from the
// row it left behind, and [Local.Hydrate] replaces milestones wholesale on
// every pull — an unpushed local milestone would be silently reverted. They
// go to the workspace first instead, and a workspace refusal leaves the file
// untouched. [Mirrored.AddSlice] goes to the workspace first too, for a
// different reason: a slice's ID is its Notion page ID, and only the
// workspace can hand one back.
type Mirrored struct {
	local  *Local
	remote *Notion
	// project is what a Mirrored was wired for, which is what
	// [Mirrored.ensureHydrated] pulls against when a read reaches the file
	// before anything else ever has — a caller opening a store afresh and
	// reading straight from it, rather than by way of [ForProject], which
	// hydrates eagerly itself before handing one back.
	project Project

	// lastPull is when the file was last brought fully into line with the
	// workspace, which is what [Mirrored.Body] compares a page's own stamp
	// against to decide whether its copy is still worth trusting.
	lastPull time.Time
}

// Mirror wires a local replica to the workspace it mirrors, for the project p
// names — which [Mirrored.ensureHydrated] falls back on pulling against, since
// a read reaching the file may come before [ForProject]'s own hydrate ever
// has, or before anything has: an app that opens a project's store once and
// holds it for as long as it runs, rather than building a fresh one per
// command, has no second chance to hydrate eagerly up front the way
// [ForProject] does.
func Mirror(local *Local, remote *Notion, p Project) *Mirrored {
	return &Mirrored{local: local, remote: remote, project: p}
}

// Mirrored is a Store.
var _ Store = (*Mirrored)(nil)

// Puller is answered by a store with a workspace behind it, which pulling
// only ever means something for. A [Local] plan of its own answers no such
// interface — there is nowhere for it to pull from — so [Pull] treats its
// absence as nothing to do rather than an error.
type Puller interface {
	Pull(ctx context.Context, p Project) error
}

// Pull forces a store to bring its copy of a plan into line with whatever is
// behind it, for the two moments a caller knows better than the clock a
// read otherwise waits on — the refresh key and the background poll — rather
// than the staleness an ordinary read pulls for itself over; see
// [Mirrored.Plan]. A store with nothing behind it answers nil: there is
// nothing to do, and nothing wrong with asking.
func Pull(ctx context.Context, s Store, p Project) error {
	pl, ok := s.(Puller)
	if !ok {
		return nil
	}
	return pl.Pull(ctx, p)
}

// planStaleAfter is how old the file's own record of when it was last
// brought into line with the workspace can be before [Mirrored.Plan] pulls
// again rather than trusting it. It is long enough that the background poll
// and the refresh key — which force a pull well inside it — are what
// normally keeps a copy this young, and short enough that a plan opened
// after nat has been shut a while catches up on its own rather than waiting
// on either.
const planStaleAfter = 5 * time.Minute

// stale reports whether the file's copy of the plan is old enough that
// [Mirrored.Plan] should pull again before answering from it: never hydrated
// at all, or hydrated longer ago than planStaleAfter. A freshness reading
// that itself fails is read as stale — there is nothing safer to assume.
func (m *Mirrored) stale(ctx context.Context, p Project) bool {
	synced, err := m.local.SyncedAt(ctx, p.ID)
	if err != nil || synced.IsZero() {
		return true
	}
	return time.Since(synced) > planStaleAfter
}

// ensureHydrated brings the file into line with the workspace the one time
// it has never been at all — a read reaching this Mirrored before it has
// been hydrated any other way, which is what a store opened once and held
// for the life of an app (rather than built afresh per command, the way
// [ForProject] itself already hydrates eagerly before ever handing one back)
// can no longer rule out: the very first read against it may be any of
// [Mirrored]'s own methods, not only [Mirrored.Plan]. Unlike the staleness
// pull [Mirrored.Plan] and [Mirrored.Body] otherwise run, a failure here is
// not swallowed — there is nothing in the file yet for either to fall back
// on.
//
// ordered says whether this first pull is read by the board's own view order
// rather than left as the query gave it — true for the board itself, whose
// domain rule this is, and false for [ForProject]'s own call, which nothing
// but a headless command's first-ever run against a project reaches and
// which has no board to read an order for.
func (m *Mirrored) ensureHydrated(ctx context.Context, ordered bool) error {
	hydrated, err := m.local.hydrated(ctx, m.project.ID)
	if err != nil {
		return err
	}
	if hydrated {
		return nil
	}
	if err := m.pull(ctx, m.project, ordered); err != nil {
		return fmt.Errorf("hydrate the plan: %w", err)
	}
	return nil
}

// Shape reads what can be recorded about a project's slices from the file —
// never a request, since the file already knows.
func (m *Mirrored) Shape(ctx context.Context, p Project) (Shape, error) {
	return m.local.Shape(ctx, p)
}

// Plan reads the whole plan from the file, hydrating it first if it has never
// been at all ([Mirrored.ensureHydrated], whose own failure this returns
// rather than a plan read off an empty file), then pulling again when the
// file's own copy has gone stale since (see [Mirrored.stale]) — the moments a
// caller knows better than the clock, [Pull]'s forced reading, are the
// refresh key and the background poll; every other read leaves that judgment
// here. A staleness pull that fails is logged and swallowed rather than
// returned: the file already has a plan in it, reads that fail conclude
// nothing, and what is on screen is worth more than an error over it.
func (m *Mirrored) Plan(ctx context.Context, p Project) (Plan, error) {
	if err := m.ensureHydrated(ctx, true); err != nil {
		return Plan{}, err
	}
	if m.stale(ctx, p) {
		if err := m.Pull(ctx, p); err != nil {
			logging.Error("could not refresh a stale plan, reading the file as it stands",
				"project", p.ID, "err", err)
		}
	}
	return m.local.Plan(ctx, p)
}

// Slice reads one slice from the file, and falls back to the workspace for an
// ID the file has not got — a slice created on another machine, or in Notion
// itself, since the last pull, which is exactly a slice the file has never
// seen. What the workspace answers is taken into the plan
// ([Local.TakeSlice]) before it is handed back, so a second read of the same
// ID is a file read like any other.
func (m *Mirrored) Slice(ctx context.Context, id string) (domain.Slice, Shape, error) {
	s, sh, err := m.local.Slice(ctx, id)
	if err == nil {
		return s, sh, nil
	}
	if !errors.Is(err, ErrSliceNotFound) {
		return domain.Slice{}, Shape{}, err
	}
	s, _, err = m.remote.Slice(ctx, id)
	if err != nil {
		return domain.Slice{}, Shape{}, err
	}
	body, err := m.remote.Body(ctx, id)
	if err != nil {
		return domain.Slice{}, Shape{}, err
	}
	// The file keeps edges between rows it holds, so anything s depends on has
	// to be taken in first — the same rule ensureHeld already enforces for a
	// dependency AddSlice writes, applied here because a slice read straight
	// off the workspace, rather than through the plan, may equally name a
	// dependency the file has never met.
	if err := m.ensureHeld(ctx, s.DependsOn); err != nil {
		return domain.Slice{}, Shape{}, err
	}
	if err := m.local.TakeSlice(ctx, s, body, time.Now()); err != nil {
		return domain.Slice{}, Shape{}, err
	}
	return m.local.Slice(ctx, id)
}

// Body reads the prose kept against an ID — a slice's brief, or a project's
// conventions — lazily: from the file when the copy was read since the last
// pull, otherwise from the workspace, cached back into the file stamped as of
// now. [Mirrored.Pull] itself never fetches a body — a plan is hundreds of
// pages, and fetching every one's brief on every pull would be hundreds of
// requests a tick, worse than the read this mirrors in the first place — so a
// pull leaves every copy exactly as stale as it was, and this is what catches
// it up the first time something asks.
//
// A workspace that will not answer keeps the stale copy as the answer to fall
// back on: the file is still the best anyone has, and a request that fails
// says nothing about whether it is still true.
func (m *Mirrored) Body(ctx context.Context, id string) (string, error) {
	if err := m.ensureHydrated(ctx, true); err != nil {
		return "", err
	}
	fresh, err := m.local.BodyFresh(ctx, id, m.lastPull)
	if err != nil {
		return "", err
	}
	if fresh {
		return m.local.Body(ctx, id)
	}
	body, err := m.remote.Body(ctx, id)
	if err != nil {
		return m.local.Body(ctx, id)
	}
	if err := m.local.SetBody(ctx, id, body, time.Now()); err != nil {
		return "", err
	}
	return body, nil
}

// PRDescription reads the pull request description a hand-back filed on a
// slice: [Mirrored.Body]'s own laziness, so a plan whose pull never fetches
// bodies still has this section fresh at the one moment it is asked for —
// approve — rather than reading whatever the file happened to cache last,
// which may be nothing at all.
func (m *Mirrored) PRDescription(ctx context.Context, id string) (string, error) {
	body, err := m.Body(ctx, id)
	if err != nil {
		return "", err
	}
	return lastMarkdownSection(body, notion.PRDescriptionHeading), nil
}

// pageShape reads the page's own shape from the workspace — what a push that
// writes a status needs, since a Status column converted in Notion's own UI
// takes a different value from the select every project this app made has,
// and that is a read of the page rather than something the caller's own
// [Shape] can answer. Every caller folds this into its own Shape with
// [Shape.On] rather than pushing it bare: a page that has never carried an
// Assignee or Branch value yet — the ordinary state of a slice about to be
// claimed or handed back for the first time — reads back with neither
// property present at all, and pushing that reading raw would silently drop
// the write [Local]'s own copy, built from the project's schema, already got
// right.
func (m *Mirrored) pageShape(ctx context.Context, id string) (Shape, error) {
	_, sh, err := m.remote.Slice(ctx, id)
	return sh, err
}

// push runs a write against the workspace once the same write has already
// landed in the file, and clears the slice's dirty flag on success. Its own
// failure is logged and swallowed rather than returned: the file already
// holds the write, the command already succeeded, and the flag the write set
// is exactly what the next sync will send.
func (m *Mirrored) push(ctx context.Context, id string, f func() error) {
	if err := f(); err != nil {
		logging.Error("push to the workspace failed, the file is ahead and will send it on the next sync",
			"slice", id, "err", err)
		return
	}
	if err := m.local.MarkSent(ctx, id, time.Now()); err != nil {
		logging.Error("could not mark the slice sent", "slice", id, "err", err)
	}
}

// ClaimSlice takes the slice locally, then pushes the claim to the workspace.
func (m *Mirrored) ClaimSlice(ctx context.Context, id string, sh Shape, userID string) (domain.Slice, error) {
	s, err := m.local.ClaimSlice(ctx, id, sh, userID)
	if err != nil {
		return domain.Slice{}, err
	}
	m.push(ctx, id, func() error {
		pageSh, err := m.pageShape(ctx, id)
		if err != nil {
			return err
		}
		_, err = m.remote.ClaimSlice(ctx, id, sh.On(pageSh), userID)
		return err
	})
	return s, nil
}

// ReleaseSlice hands the slice back locally, then pushes the release to the
// workspace.
func (m *Mirrored) ReleaseSlice(ctx context.Context, id string, sh Shape, by string) (domain.Slice, error) {
	s, err := m.local.ReleaseSlice(ctx, id, sh, by)
	if err != nil {
		return domain.Slice{}, err
	}
	m.push(ctx, id, func() error {
		pageSh, err := m.pageShape(ctx, id)
		if err != nil {
			return err
		}
		_, err = m.remote.ReleaseSlice(ctx, id, sh.On(pageSh), by)
		return err
	})
	return s, nil
}

// CompleteSlice closes the slice out locally, then pushes the same ending to
// the workspace. The page's own shape is only read when the ending writes a
// status ([Outcome.done]) — every other ending is a note and, maybe, a branch
// or a pull request URL, none of which needs to know what type the Status
// column is written in.
func (m *Mirrored) CompleteSlice(ctx context.Context, id string, sh Shape, o Outcome) (domain.Slice, error) {
	s, err := m.local.CompleteSlice(ctx, id, sh, o)
	if err != nil {
		return domain.Slice{}, err
	}
	m.push(ctx, id, func() error {
		pushSh := sh
		if o.done() {
			pageSh, err := m.pageShape(ctx, id)
			if err != nil {
				return err
			}
			pushSh = sh.On(pageSh)
		}
		_, err := m.remote.CompleteSlice(ctx, id, pushSh, o)
		return err
	})
	return s, nil
}

// RecordPR writes the pull request locally, then pushes it to the workspace.
// It writes no status, so no page shape is ever read for it.
func (m *Mirrored) RecordPR(ctx context.Context, id, url string) error {
	if err := m.local.RecordPR(ctx, id, url); err != nil {
		return err
	}
	m.push(ctx, id, func() error { return m.remote.RecordPR(ctx, id, url) })
	return nil
}

// MarkDone moves the slice to Done locally, then pushes it to the workspace.
func (m *Mirrored) MarkDone(ctx context.Context, id string, sh Shape) error {
	if err := m.local.MarkDone(ctx, id, sh); err != nil {
		return err
	}
	m.push(ctx, id, func() error {
		pageSh, err := m.pageShape(ctx, id)
		if err != nil {
			return err
		}
		return m.remote.MarkDone(ctx, id, sh.On(pageSh))
	})
	return nil
}

// ReopenSlice writes the slice back to In progress locally, then pushes it to
// the workspace.
func (m *Mirrored) ReopenSlice(ctx context.Context, id string, sh Shape) error {
	if err := m.local.ReopenSlice(ctx, id, sh); err != nil {
		return err
	}
	m.push(ctx, id, func() error {
		pageSh, err := m.pageShape(ctx, id)
		if err != nil {
			return err
		}
		return m.remote.ReopenSlice(ctx, id, sh.On(pageSh))
	})
	return nil
}

// ClearBranch empties the slice's branch locally, then pushes it to the
// workspace.
func (m *Mirrored) ClearBranch(ctx context.Context, id string) error {
	if err := m.local.ClearBranch(ctx, id); err != nil {
		return err
	}
	m.push(ctx, id, func() error {
		return m.remote.ClearBranch(ctx, id)
	})
	return nil
}

// remoteShape reads the project's actual schema from the workspace — what
// every milestone write needs to build its own request (the Milestone
// column's property type and its existing options), and which the caller's
// own [Shape] can never carry: [Mirrored.Shape] answers from the file, and
// the file has no schema of its own to keep that in.
func (m *Mirrored) remoteShape(ctx context.Context, p Project) (Shape, error) {
	return m.remote.Shape(ctx, p)
}

// AddMilestones writes the milestones to the workspace first, and only once
// that succeeds takes them into the file — a milestone write cannot ride the
// dirty flag the way a slice's can, so there is nothing for a failed write
// here to leave behind for a later sync to send: a workspace refusal leaves
// the file untouched and fails the command.
func (m *Mirrored) AddMilestones(ctx context.Context, p Project, _ Shape, names []string) ([]domain.Milestone, error) {
	rsh, err := m.remoteShape(ctx, p)
	if err != nil {
		return nil, err
	}
	added, err := m.remote.AddMilestones(ctx, p, rsh, names)
	if err != nil {
		return nil, err
	}
	if len(added) == 0 {
		return added, nil
	}
	if err := m.local.takeMilestones(ctx, added); err != nil {
		return nil, err
	}
	return added, nil
}

// RenameMilestone renames the milestone in the workspace first, and only once
// that succeeds renames it in the file — [Local.RenameMilestone] does that
// half itself, working from the file's own copy of the plan rather than from
// whatever the workspace just answered.
func (m *Mirrored) RenameMilestone(ctx context.Context, p Project, sh Shape, old, name string) (domain.Milestone, error) {
	rsh, err := m.remoteShape(ctx, p)
	if err != nil {
		return domain.Milestone{}, err
	}
	if _, err := m.remote.RenameMilestone(ctx, p, rsh, old, name); err != nil {
		return domain.Milestone{}, err
	}
	return m.local.RenameMilestone(ctx, p, sh, old, name)
}

// RemoveMilestone removes the milestone from the workspace first, and only
// once that succeeds removes it from the file.
func (m *Mirrored) RemoveMilestone(ctx context.Context, p Project, sh Shape, name string) (domain.Milestone, error) {
	rsh, err := m.remoteShape(ctx, p)
	if err != nil {
		return domain.Milestone{}, err
	}
	if _, err := m.remote.RemoveMilestone(ctx, p, rsh, name); err != nil {
		return domain.Milestone{}, err
	}
	return m.local.RemoveMilestone(ctx, p, sh, name)
}

// MoveMilestone reorders the milestone in the workspace first, and only once
// that succeeds reorders it in the file.
func (m *Mirrored) MoveMilestone(ctx context.Context, p Project, sh Shape, name, target string, before bool) (domain.Milestone, domain.Milestone, error) {
	rsh, err := m.remoteShape(ctx, p)
	if err != nil {
		return domain.Milestone{}, domain.Milestone{}, err
	}
	if _, _, err := m.remote.MoveMilestone(ctx, p, rsh, name, target, before); err != nil {
		return domain.Milestone{}, domain.Milestone{}, err
	}
	return m.local.MoveMilestone(ctx, p, sh, name, target, before)
}

// AddSlice files the slice in the workspace first — the one slice write that
// does, because a slice's ID is its Notion page ID and only the workspace can
// hand one back — and takes what it answers into the file with
// [Local.TakeSlice], the same write that takes in a slice found on the
// workspace by [Mirrored.Slice] rather than a second entry point of its own.
func (m *Mirrored) AddSlice(ctx context.Context, p Project, n NewSlice) (domain.Slice, error) {
	added, err := m.remote.AddSlice(ctx, p, n)
	if err != nil {
		return domain.Slice{}, err
	}
	if err := m.ensureHeld(ctx, n.DependsOn); err != nil {
		return domain.Slice{}, err
	}
	if err := m.local.TakeSlice(ctx, added, n.Brief, time.Now()); err != nil {
		return domain.Slice{}, err
	}
	return added, nil
}

// ensureHeld makes sure the file holds every slice named before a dependency
// on it is written there — the file keeps edges between rows it holds, so the
// row has to be there first. A slice already known to the file is left alone;
// one it has never seen is taken in exactly as [Mirrored.Slice] takes in a
// slice read by ID.
func (m *Mirrored) ensureHeld(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if _, _, err := m.Slice(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// EditSlice rewrites the slice locally, then pushes the same rewrite to the
// workspace.
func (m *Mirrored) EditSlice(ctx context.Context, id, title, repo, brief string) error {
	if err := m.local.EditSlice(ctx, id, title, repo, brief); err != nil {
		return err
	}
	m.push(ctx, id, func() error { return m.remote.EditSlice(ctx, id, title, repo, brief) })
	return nil
}

// SetSliceBrief rewrites the slice's brief locally, then pushes it to the
// workspace.
func (m *Mirrored) SetSliceBrief(ctx context.Context, id, brief string) error {
	if err := m.local.SetSliceBrief(ctx, id, brief); err != nil {
		return err
	}
	m.push(ctx, id, func() error { return m.remote.SetSliceBrief(ctx, id, brief) })
	return nil
}

// SetDependencies makes sure the file holds every slice depended on, records
// them locally, then pushes the same dependencies to the workspace.
func (m *Mirrored) SetDependencies(ctx context.Context, id string, on []string) (domain.Slice, error) {
	if err := m.ensureHeld(ctx, on); err != nil {
		return domain.Slice{}, err
	}
	s, err := m.local.SetDependencies(ctx, id, on)
	if err != nil {
		return domain.Slice{}, err
	}
	m.push(ctx, id, func() error {
		_, err := m.remote.SetDependencies(ctx, id, on)
		return err
	})
	return s, nil
}

// MoveSlice refiles the slice locally, then pushes the same move to the
// workspace.
func (m *Mirrored) MoveSlice(ctx context.Context, id string, ms domain.Milestone) error {
	if err := m.local.MoveSlice(ctx, id, ms); err != nil {
		return err
	}
	m.push(ctx, id, func() error { return m.remote.MoveSlice(ctx, id, ms) })
	return nil
}

// ReorderSlice places the slice in the file, and pushes only what the workspace
// can hold: the refile, where the target is under another milestone. A reorder
// within a milestone is the file's alone — [Local.ReorderSlice] sets no dirty
// flag for it, and nothing is sent.
func (m *Mirrored) ReorderSlice(ctx context.Context, sh Shape, id, target string, before bool) (domain.Slice, domain.Slice, error) {
	was, _, err := m.local.Slice(ctx, id)
	if err != nil {
		return domain.Slice{}, domain.Slice{}, err
	}
	moved, to, err := m.local.ReorderSlice(ctx, sh, id, target, before)
	if err != nil {
		return domain.Slice{}, domain.Slice{}, err
	}
	if was.MilestoneID != moved.MilestoneID {
		m.push(ctx, id, func() error {
			_, _, err := m.remote.ReorderSlice(ctx, sh, id, target, before)
			return err
		})
	}
	return moved, to, nil
}

// DeleteSlice drops the slice from the file first, then asks the workspace to
// do the same. Unlike every other write, a failed push here is not something
// a later sync can retry: the row the dirty flag would have lived on is
// already gone, so the failure is only logged.
func (m *Mirrored) DeleteSlice(ctx context.Context, id string) error {
	if err := m.local.DeleteSlice(ctx, id); err != nil {
		return err
	}
	if err := m.remote.DeleteSlice(ctx, id); err != nil {
		logging.Error("push to the workspace failed and cannot be retried: the slice is already gone from the file",
			"slice", id, "err", err)
	}
	return nil
}

// AddSession files an ad hoc session in the file alone: a session belongs to
// this machine, never to the workspace this Mirrored otherwise pushes every
// slice write to, so there is nothing here for [Mirrored.push] to do.
func (m *Mirrored) AddSession(ctx context.Context, p Project, n NewSession) (domain.Session, error) {
	return m.local.AddSession(ctx, p, n)
}

// Sessions reads every ad hoc session from the file — never the workspace,
// which knows nothing of them.
func (m *Mirrored) Sessions(ctx context.Context, p Project) ([]domain.Session, error) {
	return m.local.Sessions(ctx, p)
}

// EndSession records a session's end in the file alone.
func (m *Mirrored) EndSession(ctx context.Context, id string) error {
	return m.local.EndSession(ctx, id)
}

// DeleteSession drops a session's row from the file alone.
func (m *Mirrored) DeleteSession(ctx context.Context, id string) error {
	return m.local.DeleteSession(ctx, id)
}

// Pull reads the whole plan from the workspace and hydrates the file with it
// ([Local.Hydrate]) — the whole plan rather than what changed, because "what
// changed" says nothing about what was deleted. It fetches no bodies, for the
// reason [Mirrored.Body] fetches them lazily instead, and reads the plan
// through [Notion.planForPull] rather than [Notion.Plan]: the reading's order
// only ever places a slice the file has never seen, so the board's own view
// order — a request of its own — is never even looked at on this path.
func (m *Mirrored) Pull(ctx context.Context, p Project) error {
	return m.pull(ctx, p, false)
}

// pull is [Mirrored.Pull]'s own body, and [Mirrored.ensureHydrated]'s: reading
// the plan through the workspace either way — ordered by the board's own
// view, or left as the query gave it — and taking it into the file.
func (m *Mirrored) pull(ctx context.Context, p Project, ordered bool) error {
	read := m.remote.planForPull
	if ordered {
		read = m.remote.Plan
	}
	plan, err := read(ctx, p)
	if err != nil {
		return err
	}
	at := time.Now()
	if err := m.local.Hydrate(ctx, p, plan, nil, at); err != nil {
		return err
	}
	m.lastPull = at
	return nil
}
