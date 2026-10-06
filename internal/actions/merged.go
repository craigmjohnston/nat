package actions

import (
	"context"
	"fmt"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/logging"
)

// MarkDone moves a slice to Done: the one write that says its work is on
// main. Nothing but a merge reaches it — approving records the pull request
// and leaves the slice in progress, so the status on the page means the same
// thing everywhere the app reads it.
//
// The slice is read first for the shape it can be written in, which a project
// converted in the Notion UI may have changed under the app — the same read
// complete-slice makes for the same reason.
func MarkDone(ctx context.Context, st Store, s domain.Slice) error {
	_, shape, err := st.Slice(ctx, s.ID)
	if err != nil {
		return fmt.Errorf("mark %q Done: %w", s.Name, err)
	}
	if err := st.MarkDone(ctx, s.ID, shape); err != nil {
		return fmt.Errorf("mark %q Done: %w", s.Name, err)
	}
	logging.Action("slice marked Done", "slice", s.ID, "name", s.Name)
	return nil
}

// ReopenUnmerged writes a slice back to In progress: the mirror of
// [SettleMerged], for a slice Done under the old rule — Done written at
// approve, rather than at the merge — whose pull request a reading has found
// still open. Notion's Done no longer agrees with the work: the review, or
// the merge, is still to come, and In progress is what says so everywhere
// else the app reads a slice's status from — see the domain rule on
// StateOf. This converges the plan lazily, one slice at a time, as each is
// next read rather than all at once.
//
// The slice is read first for the shape it can be written in, exactly as
// [MarkDone]'s own read is, since a project converted in the Notion UI since
// this slice was marked Done may have changed under the app.
func ReopenUnmerged(ctx context.Context, st Store, s domain.Slice) error {
	_, shape, err := st.Slice(ctx, s.ID)
	if err != nil {
		return fmt.Errorf("reopen %q to In progress: %w", s.Name, err)
	}
	if err := st.ReopenSlice(ctx, s.ID, shape); err != nil {
		return fmt.Errorf("reopen %q to In progress: %w", s.Name, err)
	}
	logging.Action("slice reopened to In progress", "slice", s.ID, "name", s.Name)
	return nil
}

// SettleMerged marks the slice Done where a reading of its pull request says
// merged — how a merge made on GitHub itself, with nat not running to make it,
// still moves the slice. The reading is the batch's ([gh.CLI.ReadPRs]), which
// carries the pull request's state, so nothing more is asked of gh here. A
// pull request closed unmerged is work going round again rather than work
// landed, and an open one is still under review: the slice is left exactly as
// it is. Reports whether Done was written.
func SettleMerged(ctx context.Context, st Store, s domain.Slice, reading gh.PRStatus) (bool, error) {
	if reading.State != gh.PRStateMerged {
		return false, nil
	}
	if err := MarkDone(ctx, st, s); err != nil {
		return false, err
	}
	return true, nil
}
