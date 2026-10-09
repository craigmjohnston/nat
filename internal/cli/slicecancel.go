package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/domain"
)

// sliceCancel takes a slice in progress back to Todo and throws its work away
// — release-slice's destructive sibling, and what the macOS app's Cancel and
// discard work runs once the user has confirmed. Where a release keeps the
// branch and refuses a live agent, a cancel stops the agent, clears the
// slice's Branch and pull request, and deletes its worktree and branch, so the
// next launch starts from the brief alone. [actions.Cancel] is the whole flow,
// shared with the board's cancel key.
//
// There is no ownership check, as there is none for slice-resume: the user's
// confirmation is the gate. A Todo slice (nothing to cancel), a Done one
// (merged — new work is a new slice) and one that cannot be read are refused
// before anything is written; so is a slice whose live agent cannot be
// stopped. Nothing touches GitHub: an open pull request is left as it is, for
// the user to close.
func sliceCancel(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-cancel", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-cancel: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-cancel", rest[0])
	if err != nil {
		return err
	}

	cfg, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	// The line names who cancelled it, and a name is all it needs: there is
	// no ownership to check.
	if cfg.AssigneeUserName == "" {
		return fmt.Errorf("no assignee in the config: open the board with `nat` and finish setting it up")
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}

	cancelled, err := actions.Cancel(ctx, st, env.NewTmux(), env.NewWorktrees(),
		storeProject(projectID, project), project, id, cfg.AssigneeUserName)
	if err != nil {
		return err
	}

	env.nudged()
	if *asJSON {
		return writeJSON(env.Out, sliceCancelledJSON{ID: cancelled.ID, Name: cancelled.Name, Cancelled: true})
	}
	_, err = io.WriteString(env.Out, cancelledMarkdown(cancelled))
	return err
}

// cancelledMarkdown reports what moved, as releasedMarkdown does for a
// release.
func cancelledMarkdown(s domain.Slice) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Name)
	b.WriteString("Cancelled. Its agent is stopped, its worktree and branch are discarded, and it is Todo and " +
		"unassigned, with the note on its page. Any pull request it had is left open on GitHub.\n\n")
	fmt.Fprintf(&b, "- Notion page: %s\n", s.ID)
	if s.URL != "" {
		fmt.Fprintf(&b, "- Notion URL: %s\n", s.URL)
	}
	return b.String()
}

// sliceCancelledJSON is the structured form of a successful cancel. Cancelled
// is always true — a refusal is a non-zero exit — and is there so the
// document says what happened rather than only naming a page, as
// [sliceDeletedJSON]'s Deleted is.
type sliceCancelledJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Cancelled bool   `json:"cancelled"`
}
