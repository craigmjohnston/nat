package actions

import (
	"context"
	"fmt"

	"github.com/craigmjohnston/nat/internal/domain"
)

// ResumeStore is what taking a handed-back slice back to work does to a plan:
// file the task-log section that says why, then clear the slice's Branch.
type ResumeStore interface {
	RecordResumed(ctx context.Context, id, note string) error
	ClearBranch(ctx context.Context, id string) error
}

// Resume takes the work on a handed-back slice back up: a Resumed section
// carrying note goes on its task log, then its Branch is cleared, so the
// slice reads as work in progress until the agent's next `complete-slice
// --branch` records the branch again. It is what `nat slice-resume` runs —
// an agent asked for more after its hand-back, or the app on the user's
// behalf — and what a checks nudge runs before it tells the agent.
//
// A slice that is not in progress is refused: a Done one by name, since its
// work is merged and new work is a new slice. One in progress with no Branch
// is already work in progress — never handed back, sent back, or resumed
// already — and Resume writes nothing for it, so an agent that runs it twice
// leaves one record. It reports whether it wrote.
func Resume(ctx context.Context, st ResumeStore, s domain.Slice, note string) (bool, error) {
	switch {
	case s.Status == domain.SliceDone:
		return false, fmt.Errorf("%q is Done: its work is merged, and new work on it is a new slice", s.Name)
	case s.Status != domain.SliceClaimed:
		return false, fmt.Errorf("%q is %s: only a slice in progress can be resumed", s.Name, s.StatusName)
	case s.Branch == "":
		return false, nil
	}
	return true, TakeBack(ctx, st, s.ID, func() error { return st.RecordResumed(ctx, s.ID, note) })
}

// BranchClearer is the one write [TakeBack] makes of its own.
type BranchClearer interface {
	ClearBranch(ctx context.Context, id string) error
}

// TakeBack takes a handed-back slice out of review: record files the event
// that says why — a Resumed, or `slice-rework`'s Sent back — and only once it
// has landed is the Branch cleared. The order is the one a hand-back keeps
// for its own note before its status write, and for the same reason: a slice
// already cleared out of review reads, to the refusal every such write opens
// with, as never handed back at all, so a record that failed after the clear
// would be lost rather than retried.
func TakeBack(ctx context.Context, st BranchClearer, id string, record func() error) error {
	if err := record(); err != nil {
		return err
	}
	return st.ClearBranch(ctx, id)
}
