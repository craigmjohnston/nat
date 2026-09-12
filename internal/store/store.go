// Package store is the seam between nat and wherever a project's plan is
// kept. Everything the board and the headless commands do to a plan — read
// it, claim a slice, hand one back, file one, refile one, drop one — is a
// method of [Store], said in the app's own words rather than in the words of
// whatever is holding the plan: [domain.Slice] and [domain.Milestone] go in
// and come back, and no property type, page ID shape or request body crosses
// the line.
//
// [Notion] is the first implementation and [Local] the second — a plan kept in
// a SQLite database of nat's own rather than in a workspace. This is also the
// one place in the tree a Notion client is made ([NewClient]), which is what
// makes the seam real rather than declared: a backend plugs in here and
// nothing above it has to learn about it.
package store

import (
	"context"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// Project names the project an operation works on. It is the little a store
// needs to find a plan — the page the conventions live on, its name for the
// messages, and where the slices themselves are kept — and it is deliberately
// not config.ProjectConfig, which is this machine's idea of a project and
// holds a working directory no store has any business reading.
type Project struct {
	// ID is the project's own ID, which is what its slices are grouped under.
	ID string
	// Name is what the project is called, for what a caller prints.
	Name string
	// SlicesID names the collection of slices the plan is kept in.
	SlicesID string
}

// Shape is what a store can record about one project's slices, and the
// milestones there are to file one under. It is read rather than assumed,
// because a project set up before a column existed — or edited since in the
// backing store's own UI — cannot be written to as though it had one.
//
// A Shape also carries, unexported, whatever the store that read it needs to
// write with — here the property types this project's Status and Milestone
// columns are kept in, and the Milestone column itself, whose existing options
// a new milestone is appended to. A caller never sees any of it and never
// needs to: it takes a Shape from a read and hands the same Shape back to the
// write, which is the whole of what a Shape is for.
type Shape struct {
	// HasAssignee says whether ownership can be recorded on a slice at all.
	// Without it, ownership is decided on status alone.
	HasAssignee bool
	// HasBranch says whether a hand-back's branch has anywhere to be written.
	// A branch recorded nowhere is a hand-back lost, so the command that would
	// write one refuses instead.
	HasBranch bool
	// Milestones are the project's plan, in plan order.
	Milestones []domain.Milestone

	// statusType is the property type this project's Status column is written
	// in, and milestone is the Milestone column as it stands, which is what a
	// milestone is appended to — options, colours and all, since rebuilding one
	// from its names alone would quietly rewrite every option already there.
	statusType string
	milestone  notion.PropertySchema
}

// On returns the project's shape as it applies to one slice's own page, and is
// how a caller holding both writes: which columns exist stays the schema's
// answer, since a column holding nothing may simply not appear on a page,
// while the type a status is written in becomes the page's, because that is
// the value the write has to match — a Status column converted in the backing
// store's own UI takes a different value from the one every project this app
// made has.
func (sh Shape) On(page Shape) Shape {
	sh.statusType = page.statusType
	return sh
}

// Plan is a project's whole plan as a store answers it: the plan itself, the
// shape it is kept in, and whatever loading it changed on the way.
type Plan struct {
	// Project is the plan: milestones, slices, and the grouping over them.
	Project domain.Project
	// Shape is what can be recorded about the project's slices.
	Shape Shape
	// Migrated says what reading the plan changed about how it is stored, in
	// one line, and is empty when nothing changed — which is every read after
	// the first. It is a sentence rather than a structure because the only two
	// callers log it and toast it.
	Migrated string
}

// NewSlice is a slice to file: everything decided by whoever files it. Status
// and ownership are not in it, because a newly filed slice is Todo and
// unclaimed or it is not something the workflow can hand out.
type NewSlice struct {
	// Title is what the slice is called.
	Title string
	// Brief is the slice's own description, written as its page body.
	Brief string
	// Repo overrides the project's working directory for this slice alone.
	Repo string
	// Milestone is the phase of the plan the slice is filed under.
	Milestone domain.Milestone
	// DependsOn is the slices this one waits on, by ID.
	DependsOn []string
}

// Outcome is how a session working a slice ended, as [Store.CompleteSlice]
// takes it. The endings are exclusive and it is the caller that settles which
// one this is: a store writes what it is told.
type Outcome struct {
	// Summary is the note appended to the slice, filed under a heading naming
	// the ending. It is never empty — a slice closed out with nothing written
	// on it loses the only record of what was done.
	Summary string
	// Branch is the branch the work was pushed to and handed back on. Recording
	// one leaves the slice in progress: the work is done and the review is not.
	Branch string
	// PR is a pull request already open. Recording one leaves the slice in
	// progress too, since Done means the work is on main and the merge is what
	// writes it.
	PR string
	// PRDescription is the text the pull request a handed-back branch has yet
	// to open will be opened with, filed beside the summary under a heading of
	// its own so it outlives the session that wrote it.
	PRDescription string
	// Blocked leaves the slice in progress with the summary saying what
	// stopped it.
	Blocked bool
}

// done reports whether this ending marks the slice Done. Only an ending with
// no pull request at all does: a hand-back is waiting to be reviewed, a
// recorded pull request is waiting to merge, and blocked work has not
// finished, so none of the three is work on main.
func (o Outcome) done() bool {
	return !o.Blocked && o.Branch == "" && o.PR == ""
}

// Store is everything nat does to a plan. It is one interface rather than one
// per caller because a second backend has to answer all of it or none of it:
// the board and the headless commands are the same operations by two routes.
type Store interface {
	// Shape reads what can be recorded about a project's slices, and the
	// milestones the plan offers, without reading the slices themselves.
	Shape(ctx context.Context, p Project) (Shape, error)
	// Plan reads a project's whole plan, in the order it is kept in.
	Plan(ctx context.Context, p Project) (Plan, error)
	// Slice reads one slice by ID, with the shape it can be written in — which
	// is read off the slice itself, so a write follows what the page actually
	// holds rather than what the project's schema last said.
	Slice(ctx context.Context, id string) (domain.Slice, Shape, error)
	// Body reads the prose kept on a page — a slice's brief, a project's
	// conventions — as markdown. It takes an ID rather than a slice because
	// the two things written this way are a slice and a project, and reading
	// either is the same read.
	Body(ctx context.Context, id string) (string, error)
	// PRDescription reads the text a hand-back filed for the pull request its
	// branch has yet to open, and "" where it filed none — which is every
	// hand-back written before there was a flag for one.
	PRDescription(ctx context.Context, id string) (string, error)
	// ClaimSlice takes a slice for a user: in progress, and held by them where
	// the project records ownership at all. The slice as the store holds it
	// afterwards comes back, so a caller can check the claim stuck.
	ClaimSlice(ctx context.Context, id string, sh Shape, userID string) (domain.Slice, error)
	// ReleaseSlice hands a slice back to the plan: Todo, held by nobody, and a
	// line on it naming who let it go. Nothing else about the slice is touched.
	ReleaseSlice(ctx context.Context, id string, sh Shape, by string) (domain.Slice, error)
	// CompleteSlice closes a slice out: the summary written on it first, then
	// whichever properties the ending calls for.
	CompleteSlice(ctx context.Context, id string, sh Shape, o Outcome) (domain.Slice, error)
	// RecordPR writes a pull request's URL onto a slice and nothing else. The
	// slice stays in progress: the merge is what marks work landed.
	RecordPR(ctx context.Context, id, url string) error
	// MarkDone moves a slice to Done — the one write that says its work is on
	// main.
	MarkDone(ctx context.Context, id string, sh Shape) error
	// AddMilestones files milestones at the end of the plan, all of them or
	// none, and returns them in the order they were given.
	AddMilestones(ctx context.Context, p Project, sh Shape, names []string) ([]domain.Milestone, error)
	// RenameMilestone gives one milestone another name, in place: the plan
	// keeps its order and the milestone keeps its slices. A name the plan
	// already holds, and an old name it does not, are each refused before
	// anything is written.
	RenameMilestone(ctx context.Context, p Project, sh Shape, old, name string) (domain.Milestone, error)
	// RemoveMilestone drops a milestone from the plan and changes nothing else
	// about it. A name the plan does not hold, and a milestone with any slice
	// still filed under it, are each refused before anything is written: a
	// milestone is nothing but the name its slices carry, so dropping one out
	// from under them would leave them filed under a milestone the plan no
	// longer has. The milestone as it was — its place in the plan included —
	// comes back, since that place is what a caller's own record of the plan is
	// keyed by.
	RemoveMilestone(ctx context.Context, p Project, sh Shape, name string) (domain.Milestone, error)
	// MoveMilestone moves a milestone to sit directly before or after another,
	// and changes nothing else about the plan: a milestone's order is its place
	// among the others, so a move is the order of the plan and nothing more —
	// every milestone keeps its name and keeps its slices. A name the plan does
	// not hold, a target it does not hold, and a move relative to the milestone
	// itself are each refused before anything is written. The milestone as it
	// now stands and the one it was placed relative to both come back, since
	// each has a new place in the plan and that place is what a caller's own
	// record of the plan is keyed by.
	MoveMilestone(ctx context.Context, p Project, sh Shape, name, target string, before bool) (domain.Milestone, domain.Milestone, error)
	// AddSlice files one slice under a milestone, Todo and unclaimed.
	AddSlice(ctx context.Context, p Project, n NewSlice) (domain.Slice, error)
	// EditSlice rewrites a slice's title, working directory and brief, leaving
	// its milestone and its status alone: moving a slice is its own operation
	// and the status is the workflow's.
	EditSlice(ctx context.Context, id, title, repo, brief string) error
	// SetSliceBrief rewrites a slice's brief and nothing else about it.
	SetSliceBrief(ctx context.Context, id, brief string) error
	// SetDependencies records exactly the slices a slice waits on, replacing
	// whatever it waited on before — an empty list being how a slice is freed —
	// and answers with the slice as it stands afterwards.
	SetDependencies(ctx context.Context, id string, on []string) (domain.Slice, error)
	// MoveSlice refiles a slice under another milestone. The work itself is
	// untouched.
	MoveSlice(ctx context.Context, id string, m domain.Milestone) error
	// DeleteSlice drops a slice from the plan, as recoverably as the backing
	// store allows.
	DeleteSlice(ctx context.Context, id string) error
}

// Holds reports whether a slice is in progress and held by the given user,
// which is what a claim looks like from the outside — and so what every
// operation that may only touch the caller's own slice asks first.
//
// Without an Assignee column the status is the whole answer: there is nobody
// else the slice could belong to, so a project that records no ownership
// decides it on status alone.
func Holds(s domain.Slice, sh Shape, userID string) bool {
	if s.Status != domain.SliceClaimed {
		return false
	}
	if !sh.HasAssignee {
		return true
	}
	for _, id := range s.AssigneeIDs {
		if id == userID {
			return true
		}
	}
	return false
}
