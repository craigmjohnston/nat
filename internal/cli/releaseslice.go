package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
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

	cfg, project, err := env.activeProject()
	if err != nil {
		return err
	}
	if cfg.AssigneeUserID == "" {
		return fmt.Errorf("no assignee in the config: open the board with `nat` and finish setting it up")
	}
	client := env.NewClient(env.Tokens.Token)

	shape, err := sliceShape(ctx, client, project)
	if err != nil {
		return err
	}
	page, err := client.GetPage(ctx, pageID)
	if err != nil {
		return fmt.Errorf("load the slice: %w", err)
	}
	if !holds(*page, shape, cfg.AssigneeUserID) {
		return notOursError(*page, shape, cfg.AssigneeUserName, "released")
	}

	// The line goes on before the status does, for the reason complete-slice
	// writes its note first: of the two half-finished states, a slice still in
	// progress carrying the line is the one a second run can finish, whereas a
	// slice already back at Todo is one this command would refuse to add a line
	// to — and a line written twice reads worse than no record at all reads.
	if _, err := client.AppendBlockChildren(ctx, page.ID,
		[]map[string]any{textBlock("paragraph", releasedLine(cfg.AssigneeUserName))}); err != nil {
		return fmt.Errorf("note the release on the slice: %w", err)
	}
	released, err := release(ctx, client, page, shape)
	if err != nil {
		return err
	}

	env.nudged()
	logging.Action("slice released", "slice", released.ID, "name", released.Name)
	_, err = io.WriteString(env.Out, releasedMarkdown(released))
	return err
}

// release is the write itself: back to Todo, and held by nobody where the
// project has an Assignee column to say so on. The status is written in the
// shape the page was read in, since a Status column converted in the Notion UI
// takes a different value from the select every project this app made has.
func release(ctx context.Context, client API, page *notion.Page, shape notion.SliceShape) (domain.Slice, error) {
	properties := map[string]notion.PropertyValue{
		notion.PropStatus: notion.NewChoice(page.Properties[notion.PropStatus].Type, notion.SliceTodo),
	}
	if shape.HasAssignee {
		properties[notion.PropAssignee] = notion.NewPeople()
	}
	updated, err := client.UpdatePageProperties(ctx, page.ID, properties)
	if err != nil {
		return domain.Slice{}, fmt.Errorf("release the slice: %w", err)
	}
	return domain.SliceFromPage(*updated), nil
}

// releasedLine is the one line a release leaves on the page, so a slice that
// went round twice reads as having done so rather than as having been worked
// once by somebody who wrote nothing down.
func releasedLine(assignee string) string {
	return fmt.Sprintf("Released back to Todo by %s: the session working it ended without finishing it.", assignee)
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
