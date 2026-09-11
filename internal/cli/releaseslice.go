package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/store"
)

// releaseSlice hands a slice back to the plan: Status to Todo and the Assignee
// cleared, so the next session can claim it exactly as the last one did.
//
// It is the way out of the one state nothing else can leave. A session that
// dies — a crashed agent, a killed pane, a context that ran out — leaves its
// slice in progress and held, where next-slice steps over it and start-slice
// refuses it; complete-slice only goes forward, and --blocked is what leaves a
// slice held on purpose in the first place.
//
// Nothing else on the page is touched. The description, the dependencies, the
// repo and any branch already pushed stay as they are: the next session wants
// exactly the brief this one had, and a branch half-written is still the work
// so far.
//
// Only a slice this user already holds can be released — the same ownership
// rule complete-slice applies, and for the same reason: a slice somebody else
// is working is theirs, and pulling it out from under them is how two sessions
// end up on one branch.
func releaseSlice(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("release-slice", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("release-slice: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	pageID, err := pageID("release-slice", rest[0])
	if err != nil {
		return err
	}

	cfg, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	if cfg.AssigneeUserID == "" {
		return fmt.Errorf("no assignee in the config: open the board with `nat` and finish setting it up")
	}
	st := store.Over(env.NewClient(env.Tokens.Token))

	shape, err := sliceShape(ctx, st, projectID, project)
	if err != nil {
		return err
	}
	s, pageShape, err := loadSlice(ctx, st, pageID)
	if err != nil {
		return err
	}
	write := shape.On(pageShape)
	if !store.Holds(s, write, cfg.AssigneeUserID) {
		return notOursError(s, cfg.AssigneeUserName, "released")
	}

	released, err := st.ReleaseSlice(ctx, s.ID, write, cfg.AssigneeUserName)
	if err != nil {
		return err
	}

	env.nudged()
	_, err = io.WriteString(env.Out, releasedMarkdown(released))
	return err
}

// releasedMarkdown reports what moved, the way every other command that writes
// to a slice does.
func releasedMarkdown(s domain.Slice) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Name)
	b.WriteString("Released. It is Todo and unassigned, and the note is on its page.\n\n")
	fmt.Fprintf(&b, "- Notion page: %s\n", s.ID)
	if s.URL != "" {
		fmt.Fprintf(&b, "- Notion URL: %s\n", s.URL)
	}
	return b.String()
}
