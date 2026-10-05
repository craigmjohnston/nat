package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/domain"
)

// sliceDelete moves a slice's page to Notion's trash — the headless half of
// the board's d key, and what the macOS app's delete action runs. Notion has
// no hard delete, so a slice deleted by mistake is still recoverable in the
// Notion UI, which is also why a Done slice is allowed through: warning about
// dropping the record of finished work is the caller's confirm, not this
// command's refusal.
//
// A slice in progress is refused, exactly as a move is: the page underneath
// belongs to whoever took it.
func sliceDelete(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-delete", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-delete: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-delete", rest[0])
	if err != nil {
		return err
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}

	s, _, err := loadSlice(ctx, st, id)
	if err != nil {
		return err
	}
	if s.Status == domain.SliceClaimed {
		return fmt.Errorf("%q is in progress: work in flight is not deleted under its agent", s.Name)
	}

	if err := st.DeleteSlice(ctx, s.ID); err != nil {
		return fmt.Errorf("delete the slice: %w", err)
	}
	// A trashed slice's worktree belongs to nothing any more. In progress is
	// refused above, so no agent is working in it; a removal git refuses is
	// logged and left, and the delete has happened regardless.
	actions.RemoveSliceWorktree(env.NewWorktrees(), s, project)
	removed := firstOf(actions.PruneEmptied(ctx, st, storeProject(projectID, project), s.MilestoneID))

	env.nudged()
	if *asJSON {
		return writeJSON(env.Out, sliceDeletedJSON{ID: s.ID, Name: s.Name, Deleted: true, RemovedMilestone: removed})
	}
	_, err = fmt.Fprintf(env.Out, "# %s\n\nMoved to Notion's trash — recoverable there.\n%s", s.Name, removedLine(removed))
	return err
}

// sliceDeletedJSON is the structured form of a successful delete. Deleted is
// always true — a refusal is a non-zero exit — and is there so the document
// says what happened rather than only naming a page.
type sliceDeletedJSON struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
	// RemovedMilestone names the milestone the delete left with no slice at
	// all, and so removed; omitted where it left none empty.
	RemovedMilestone string `json:"removed_milestone,omitempty"`
}
