package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/actions"
)

// sliceDelete moves a slice's page to Notion's trash — the headless half of
// the board's d key, and what the macOS app's delete action runs. Notion has
// no hard delete, so a slice deleted by mistake is still recoverable in the
// Notion UI, which is also why a Done slice is allowed through: warning about
// dropping the record of finished work is the caller's confirm, not this
// command's refusal.
//
// A slice in progress is deleted too, on the same reasoning: the caller's
// confirm is the gate. Its live agent is stopped first, refusing before any
// write where tmux cannot be read or the kill fails, and its worktree and
// branch are discarded with the work in them after the trash — all
// [actions.Delete]'s, which the board's d runs too.
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
	pruned, err := actions.Delete(ctx, st, env.NewTmux(), env.NewWorktrees(), storeProject(projectID, project), project, s)
	if err != nil {
		return err
	}
	removed := firstOf(pruned)

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
