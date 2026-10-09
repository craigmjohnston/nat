package actions

import (
	"context"
	"fmt"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/store"
)

// AgentStopper is what stopping a slice's agent asks of tmux: which slices
// have an agent running, by slice ID, and the kill. [agent.Tmux] answers it,
// and so does the board's launcher.
type AgentStopper interface {
	LiveSlices() (map[string]string, error)
	Kill(session string) error
}

// StopAgent ends the agent session working slice id, where there is one —
// what a cancel and the delete of a slice in progress run before they write,
// since an agent left working on a slice its user threw away, or on a page in
// the trash, is the one outcome neither may leave. A tmux that cannot say
// which sessions are live, and a kill that fails, are both refusals, before
// anything is written; a slice with no agent, or a session already gone
// ([agent.Tmux.Kill] reads that as success), is nothing to stop.
func StopAgent(t AgentStopper, id string) error {
	live, err := t.LiveSlices()
	if err != nil {
		return fmt.Errorf("could not read live sessions to stop its agent first: %w", err)
	}
	session, ok := live[id]
	if !ok {
		return nil
	}
	if err := t.Kill(session); err != nil {
		return fmt.Errorf("stop its agent: %w", err)
	}
	logging.Action("slice agent stopped", "slice", id, "session", session)
	return nil
}

// CancelStore is what a cancel does to a plan: read the slice and the
// project's shape, then [store.Store.CancelSlice].
type CancelStore interface {
	Slice(ctx context.Context, id string) (domain.Slice, store.Shape, error)
	Shape(ctx context.Context, p store.Project) (store.Shape, error)
	CancelSlice(ctx context.Context, id string, sh store.Shape, by string) (domain.Slice, error)
}

// Cancel takes a slice in progress back to Todo and throws its work away —
// `nat slice-cancel` and the board's cancel key alike. It is release's
// destructive sibling: where a release keeps everything and refuses a live
// agent, a cancel stops the agent and discards the work, on the user's own
// confirmation, so there is no ownership check.
//
// In order: the slice and the project's shape are read (a slice not in
// progress refused by name — Todo has nothing to cancel, and Done is merged);
// the live agent stopped ([StopAgent], refusing before any write where it
// cannot be); the store's cancel written ([store.Store.CancelSlice]: its line,
// then Todo with the Branch and pull request cleared); and only then the
// worktree and branch discarded ([DiscardSliceWorktree], off the slice as it
// stood before the write, whose Branch says which branch was the work's). The
// branch has to go, not only the worktree: a launch reuses an existing branch,
// so a surviving one would put the next agent straight back on the work.
//
// GitHub is not touched: an open pull request is left as it is, for the user
// to close.
func Cancel(ctx context.Context, st CancelStore, t AgentStopper, w Worktrees, sp store.Project,
	p config.ProjectConfig, id, by string) (domain.Slice, error) {
	s, page, err := st.Slice(ctx, id)
	if err != nil {
		return domain.Slice{}, fmt.Errorf("load the slice: %w", err)
	}
	switch s.Status {
	case domain.SliceClaimed:
	case domain.SliceDone:
		return domain.Slice{}, fmt.Errorf("%q is Done: its work is merged, and new work on it is a new slice", s.Name)
	case domain.SliceTodo:
		return domain.Slice{}, fmt.Errorf("%q is Todo: nothing has been started on it, so there is nothing to cancel", s.Name)
	default:
		return domain.Slice{}, fmt.Errorf("%q is %s: only a slice in progress can be cancelled", s.Name, s.StatusName)
	}
	shape, err := st.Shape(ctx, sp)
	if err != nil {
		return domain.Slice{}, fmt.Errorf("read the project's shape: %w", err)
	}
	if err := StopAgent(t, s.ID); err != nil {
		return domain.Slice{}, err
	}
	cancelled, err := st.CancelSlice(ctx, s.ID, shape.On(page), by)
	if err != nil {
		return domain.Slice{}, err
	}
	DiscardSliceWorktree(w, s, p)
	return cancelled, nil
}

// DeleteStore is what a delete does to a plan: the trash, then the prune of
// the milestone it may have emptied.
type DeleteStore interface {
	DeleteSlice(ctx context.Context, id string) error
	MilestonePruner
}

// Delete trashes slice s — `nat slice-delete` and the board's delete key
// alike — and answers the milestones the delete left empty and so removed
// ([PruneEmptied]).
//
// A slice in progress is no longer refused: the user's confirmation is the
// gate, and what a delete of one must never leave is an agent working on a
// page in the trash, so its live agent is stopped first ([StopAgent],
// refusing before any write where it cannot be). After the trash its worktree
// goes: discarded, uncommitted work, branch and all, for a slice in progress
// ([DiscardSliceWorktree]) — the user threw that work away with the page — and
// removed the safe way for any other ([RemoveSliceWorktree]), a Todo or Done
// slice having no work in flight to lose. Either removal git refuses is
// logged, and the delete has happened regardless.
func Delete(ctx context.Context, st DeleteStore, t AgentStopper, w Worktrees, sp store.Project,
	p config.ProjectConfig, s domain.Slice) ([]string, error) {
	inProgress := s.Status == domain.SliceClaimed
	if inProgress {
		if err := StopAgent(t, s.ID); err != nil {
			return nil, err
		}
	}
	if err := st.DeleteSlice(ctx, s.ID); err != nil {
		return nil, fmt.Errorf("delete the slice: %w", err)
	}
	if inProgress {
		DiscardSliceWorktree(w, s, p)
	} else {
		RemoveSliceWorktree(w, s, p)
	}
	return PruneEmptied(ctx, st, sp, s.MilestoneID), nil
}
