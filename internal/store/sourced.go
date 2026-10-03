package store

import (
	"context"
	"fmt"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/source"
)

// Sourced is a plan of nat's own whose milestones are another tracker's
// containers — the Local file holds every task, a task-source plugin
// supplies the containers and hears what happens to the tasks.
//
// Three rules run through it.
//
// The containers are the plugin's. A milestone here is a container filed by
// its id ([Local.ensureMilestone]), taken into the plan the first time a task
// is filed under it and never changed after: nat does not add, rename, remove
// or reorder them, and does not move a task from one to another — each of
// those is refused, in Sourced's own words, before anything is written.
//
// Events follow the write. Each lifecycle write lands in the file first, and
// only once it has does the plugin hear of it — a write that fails tells it
// nothing. The event's own failure is logged and never returned, exactly as
// [Mirrored]'s push is: the file is the plan, the command already succeeded,
// and a plugin that did not hear is the plugin's to catch up on.
//
// Everything else is the file's. Reads, briefs, dependencies, notes, sessions:
// [Local] answers them, and the plugin is not told.
type Sourced struct {
	local  *Local
	client source.Client
	plugin source.Project
	// name is the plugin's own name — the <name> of nat-source-<name> — which
	// is what a refusal says the containers belong to.
	name string
}

// Sourced is a Store.
var _ Store = (*Sourced)(nil)

// NewSourced wraps a plan file in the task source its containers come from.
// plugin is the project as the plugin is told of it on every call, and name
// the plugin's own name.
func NewSourced(local *Local, client source.Client, plugin source.Project, name string) *Sourced {
	return &Sourced{local: local, client: client, plugin: plugin, name: name}
}

// Plugin is the project as the plugin is told of it.
func (s *Sourced) Plugin() source.Project { return s.plugin }

// fireEvent tells the plugin what just happened to a task, after the write
// that made it so. Its failure is logged — the plugin, the task, the event,
// never a body — and swallowed.
func (s *Sourced) fireEvent(ctx context.Context, sl domain.Slice, event string) {
	task := source.Task{ID: sl.ID, Title: sl.Name, Status: sl.StatusName, Branch: sl.Branch, PR: sl.PRURL}
	if err := s.client.Event(ctx, s.plugin, sl.MilestoneID, task, event); err != nil {
		logging.Error("the task source was not told of a task's event",
			"project", s.plugin.ID, "container", sl.MilestoneID, "slice", sl.ID, "event", event, "err", err)
	}
}

// fireEventByID is fireEvent for a write that answers with no slice: the
// slice is read back for it, and where that read fails the plugin is still
// told, with the ID that is always known.
func (s *Sourced) fireEventByID(ctx context.Context, id, event string) {
	sl, _, err := s.local.Slice(ctx, id)
	if err != nil {
		logging.Error("could not read a task back for its event, sending its ID alone",
			"project", s.plugin.ID, "slice", id, "event", event, "err", err)
		sl = domain.Slice{ID: id}
	}
	s.fireEvent(ctx, sl, event)
}

// refuseContainers is the refusal every container-shaping write gets.
func (s *Sourced) refuseContainers() error {
	return fmt.Errorf("%s: milestones here are %s's containers — nat does not add, rename, remove or move them",
		s.plugin.Name, s.name)
}

// refuseMove is the refusal a task moved between containers gets: the
// protocol has no event that would tell the plugin of it.
func (s *Sourced) refuseMove() error {
	return fmt.Errorf("%s: a task is not moved between %s's containers — it stays under the one it was filed under",
		s.plugin.Name, s.name)
}

// Shape reads the plan's shape from the file.
func (s *Sourced) Shape(ctx context.Context, p Project) (Shape, error) { return s.local.Shape(ctx, p) }

// Plan reads the whole plan from the file.
func (s *Sourced) Plan(ctx context.Context, p Project) (Plan, error) { return s.local.Plan(ctx, p) }

// Slice reads one slice from the file.
func (s *Sourced) Slice(ctx context.Context, id string) (domain.Slice, Shape, error) {
	return s.local.Slice(ctx, id)
}

// Body reads prose from the file.
func (s *Sourced) Body(ctx context.Context, id string) (string, error) { return s.local.Body(ctx, id) }

// PRDescription reads a hand-back's PR description from the file.
func (s *Sourced) PRDescription(ctx context.Context, id string) (string, error) {
	return s.local.PRDescription(ctx, id)
}

// ClaimSlice claims the slice in the file, then tells the plugin.
func (s *Sourced) ClaimSlice(ctx context.Context, id string, sh Shape, userID string) (domain.Slice, error) {
	sl, err := s.local.ClaimSlice(ctx, id, sh, userID)
	if err != nil {
		return domain.Slice{}, err
	}
	s.fireEvent(ctx, sl, source.EventClaimed)
	return sl, nil
}

// ReleaseSlice releases the slice in the file, then tells the plugin.
func (s *Sourced) ReleaseSlice(ctx context.Context, id string, sh Shape, by string) (domain.Slice, error) {
	sl, err := s.local.ReleaseSlice(ctx, id, sh, by)
	if err != nil {
		return domain.Slice{}, err
	}
	s.fireEvent(ctx, sl, source.EventReleased)
	return sl, nil
}

// CompleteSlice closes the slice out in the file, and tells the plugin only of
// a hand-back — an ending that recorded a branch. A blocked or plain ending is
// not an event the protocol has.
func (s *Sourced) CompleteSlice(ctx context.Context, id string, sh Shape, o Outcome) (domain.Slice, error) {
	sl, err := s.local.CompleteSlice(ctx, id, sh, o)
	if err != nil {
		return domain.Slice{}, err
	}
	if o.Branch != "" {
		s.fireEvent(ctx, sl, source.EventHandedBack)
	}
	return sl, nil
}

// ProposeFollowUps files follow-ups in the file.
func (s *Sourced) ProposeFollowUps(ctx context.Context, id string, items []FollowUp) error {
	return s.local.ProposeFollowUps(ctx, id, items)
}

// RecordTriage files a triage decision in the file.
func (s *Sourced) RecordTriage(ctx context.Context, id string, items []Triaged) error {
	return s.local.RecordTriage(ctx, id, items)
}

// RecordVisuals files visual changes in the file.
func (s *Sourced) RecordVisuals(ctx context.Context, id string, items []VisualChange) error {
	return s.local.RecordVisuals(ctx, id, items)
}

// RecordSentBack files review comments in the file.
func (s *Sourced) RecordSentBack(ctx context.Context, id, comments string) error {
	return s.local.RecordSentBack(ctx, id, comments)
}

// RecordNote files a note in the file. A note is body prose, so the plugin
// hears nothing of it.
func (s *Sourced) RecordNote(ctx context.Context, id, from, text string) error {
	return s.local.RecordNote(ctx, id, from, text)
}

// RecordChecksFailed files a pull request's failed checks in the file.
func (s *Sourced) RecordChecksFailed(ctx context.Context, id, checks string) error {
	return s.local.RecordChecksFailed(ctx, id, checks)
}

// RecordRelaunch files a relaunch line in the file.
func (s *Sourced) RecordRelaunch(ctx context.Context, id string) error {
	return s.local.RecordRelaunch(ctx, id)
}

// RecordPR records the pull request in the file, then tells the plugin the
// task was approved.
func (s *Sourced) RecordPR(ctx context.Context, id, url string) error {
	if err := s.local.RecordPR(ctx, id, url); err != nil {
		return err
	}
	s.fireEventByID(ctx, id, source.EventApproved)
	return nil
}

// MarkDone marks the slice Done in the file, then tells the plugin it merged.
func (s *Sourced) MarkDone(ctx context.Context, id string, sh Shape) error {
	if err := s.local.MarkDone(ctx, id, sh); err != nil {
		return err
	}
	s.fireEventByID(ctx, id, source.EventMerged)
	return nil
}

// ReopenSlice writes the slice back to In progress in the file.
func (s *Sourced) ReopenSlice(ctx context.Context, id string, sh Shape) error {
	return s.local.ReopenSlice(ctx, id, sh)
}

// ClearBranch empties the slice's branch in the file.
func (s *Sourced) ClearBranch(ctx context.Context, id string) error {
	return s.local.ClearBranch(ctx, id)
}

// AddMilestones is refused: the containers are the plugin's.
func (s *Sourced) AddMilestones(context.Context, Project, Shape, []string) ([]domain.Milestone, error) {
	return nil, s.refuseContainers()
}

// RenameMilestone is refused: the containers are the plugin's.
func (s *Sourced) RenameMilestone(context.Context, Project, Shape, string, string) (domain.Milestone, error) {
	return domain.Milestone{}, s.refuseContainers()
}

// RemoveMilestone is refused: the containers are the plugin's.
func (s *Sourced) RemoveMilestone(context.Context, Project, Shape, string) (domain.Milestone, error) {
	return domain.Milestone{}, s.refuseContainers()
}

// MoveMilestone is refused: the containers are the plugin's.
func (s *Sourced) MoveMilestone(context.Context, Project, Shape, string, string, bool) (domain.Milestone, domain.Milestone, error) {
	return domain.Milestone{}, domain.Milestone{}, s.refuseContainers()
}

// AddSlice takes the container into the plan if it is new to it, files the
// task under it, then tells the plugin. A task under no container is refused:
// there is nothing for the plugin to hang it off.
func (s *Sourced) AddSlice(ctx context.Context, p Project, n NewSlice) (domain.Slice, error) {
	if n.Milestone.ID == "" {
		return domain.Slice{}, fmt.Errorf("%s: every task here is filed under one of %s's containers — name the container",
			s.plugin.Name, s.name)
	}
	if err := s.local.ensureMilestone(ctx, n.Milestone.ID, n.Milestone.Name); err != nil {
		return domain.Slice{}, err
	}
	if n.Repo == "" {
		n.Repo = s.containerRepo(ctx, p, n.Milestone.ID)
	}
	sl, err := s.local.AddSlice(ctx, p, n)
	if err != nil {
		return domain.Slice{}, err
	}
	s.fireEvent(ctx, sl, source.EventCreated)
	return sl, nil
}

// containerRepo is the repository the container's latest task with one is
// worked in — what a new task on the same card starts from, since a source
// project has no working directory and the card's earlier tasks already
// worked out where its code is. Empty where none has one; a plan that cannot
// be read is logged and concludes nothing.
func (s *Sourced) containerRepo(ctx context.Context, p Project, container string) string {
	plan, err := s.local.Plan(ctx, p)
	if err != nil {
		logging.Error("could not read the plan for a container's repository", "container", container, "err", err)
		return ""
	}
	repo := ""
	for _, sl := range plan.Project.Slices {
		if sl.MilestoneID == container && sl.Repo != "" {
			repo = sl.Repo
		}
	}
	return repo
}

// EditSlice rewrites the slice in the file.
func (s *Sourced) EditSlice(ctx context.Context, id, title, repo, brief string) error {
	return s.local.EditSlice(ctx, id, title, repo, brief)
}

// SetSliceRepo records the slice's repository in the file. No event: the
// plugin is told of a task's lifecycle, and where it is worked is not part of
// it.
func (s *Sourced) SetSliceRepo(ctx context.Context, id, repo string) error {
	return s.local.SetSliceRepo(ctx, id, repo)
}

// SetSliceBrief rewrites the slice's brief in the file.
func (s *Sourced) SetSliceBrief(ctx context.Context, id, brief string) error {
	return s.local.SetSliceBrief(ctx, id, brief)
}

// SetDependencies records the slice's dependencies in the file.
func (s *Sourced) SetDependencies(ctx context.Context, id string, on []string) (domain.Slice, error) {
	return s.local.SetDependencies(ctx, id, on)
}

// MoveSlice is refused: a task stays under the container it was filed under.
func (s *Sourced) MoveSlice(context.Context, string, domain.Milestone) error {
	return s.refuseMove()
}

// ReorderSlice places a slice beside another under the same container, and
// refuses one beside a slice under another — that would be a move.
func (s *Sourced) ReorderSlice(ctx context.Context, sh Shape, id, target string, before bool) (domain.Slice, domain.Slice, error) {
	moving, _, err := s.local.Slice(ctx, id)
	if err != nil {
		return domain.Slice{}, domain.Slice{}, err
	}
	to, _, err := s.local.Slice(ctx, target)
	if err != nil {
		return domain.Slice{}, domain.Slice{}, err
	}
	if moving.MilestoneID != to.MilestoneID {
		return domain.Slice{}, domain.Slice{}, s.refuseMove()
	}
	return s.local.ReorderSlice(ctx, sh, id, target, before)
}

// DeleteSlice reads the slice, drops it from the file, then tells the plugin.
// The read comes first since there is nothing to read afterwards, and a slice
// that will not read is one [Local.DeleteSlice] would refuse in the same way,
// so its failure is the delete's.
func (s *Sourced) DeleteSlice(ctx context.Context, id string) error {
	sl, _, err := s.local.Slice(ctx, id)
	if err != nil {
		return err
	}
	if err := s.local.DeleteSlice(ctx, id); err != nil {
		return err
	}
	s.fireEvent(ctx, sl, source.EventDeleted)
	return nil
}

// AddSession files an ad hoc session in the file.
func (s *Sourced) AddSession(ctx context.Context, p Project, n NewSession) (domain.Session, error) {
	return s.local.AddSession(ctx, p, n)
}

// Sessions reads the ad hoc sessions from the file.
func (s *Sourced) Sessions(ctx context.Context, p Project) ([]domain.Session, error) {
	return s.local.Sessions(ctx, p)
}

// EndSession records a session's end in the file.
func (s *Sourced) EndSession(ctx context.Context, id string) error { return s.local.EndSession(ctx, id) }

// DeleteSession drops a session's row from the file.
func (s *Sourced) DeleteSession(ctx context.Context, id string) error {
	return s.local.DeleteSession(ctx, id)
}

// Describe asks the plugin what it says about itself.
func (s *Sourced) Describe(ctx context.Context) (source.Describe, error) {
	return s.client.Describe(ctx, s.plugin)
}

// Sidebar asks the plugin for its tree, and its header menu where it sends one.
func (s *Sourced) Sidebar(ctx context.Context, expand []string) (source.Sidebar, error) {
	return s.client.Sidebar(ctx, s.plugin, expand)
}

// Container asks the plugin for one container's detail.
func (s *Sourced) Container(ctx context.Context, id string) (source.ContainerDetail, error) {
	return s.client.Container(ctx, s.plugin, id)
}

// Action runs one of the plugin's actions.
func (s *Sourced) Action(ctx context.Context, action string, target source.Target, input string) (source.ActionResult, error) {
	return s.client.Action(ctx, s.plugin, action, target, input)
}
