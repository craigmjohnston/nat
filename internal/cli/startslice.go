package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// startSlice claims one named slice and prints the same brief next-slice does.
// It is the command an agent launched from the board runs: the board already
// knows which slice the agent is for, so there is nothing to choose — only the
// claim to take and the brief to hand over.
//
// Only a slice nobody has started can be taken. A slice already in progress, or
// Done, is somebody's work or somebody's finished work, and either way an
// agent must not be told to start it again; that refusal happens before any
// write, so a mistaken invocation leaves the plan exactly as it was.
func startSlice(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("start-slice", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("start-slice: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("start-slice", rest[0])
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
	page, err := client.GetPage(ctx, id)
	if err != nil {
		return fmt.Errorf("load the slice: %w", err)
	}
	if err := takeable(*page); err != nil {
		return err
	}
	claimed, err := claim(ctx, client, page.ID, shape, cfg.AssigneeUserID)
	if err != nil {
		return err
	}

	milestone := milestoneOf(claimed, shape)
	brief, err := body(ctx, client, claimed.ID)
	if err != nil {
		return fmt.Errorf("claimed %q but could not read its brief: %w", claimed.Name, err)
	}
	conventions, err := body(ctx, client, cfg.ActiveProjectID)
	if err != nil {
		return fmt.Errorf("claimed %q but could not read the project conventions: %w", claimed.Name, err)
	}

	b := briefOf(claimed, milestone, project, cfg.AssigneeUserName, brief, conventions)
	if *asJSON {
		return writeBriefJSON(env.Out, b, cfg.ActiveProjectID, project.Name)
	}
	_, err = io.WriteString(env.Out, briefMarkdown(b, project.Name))
	return err
}

// takeable reports why a slice cannot be started, or nil when it can. Being
// Todo is not enough on its own: a Todo slice already assigned to somebody is
// one next-slice would pass over too, and taking it would step on their work.
func takeable(page notion.Page) error {
	s := domain.SliceFromPage(page)
	if s.Status != domain.SliceTodo {
		return fmt.Errorf("%q is %s, not Todo: only a slice nobody has started can be claimed",
			s.Name, blank(s.StatusName))
	}
	if s.AssigneeName != "" {
		return fmt.Errorf("%q is Todo but assigned to %s: leave it to them", s.Name, s.AssigneeName)
	}
	return nil
}

// milestoneOf is the milestone a slice belongs to, so the brief can name it. A
// slice belonging to none is not an error — the board shows those too — it
// simply has no milestone to print.
//
// The slice's Milestone value names one of that column's options, which the
// schema already carries, so there is nothing to fetch. A name the plan no
// longer offers — an option deleted out from under a slice — leaves the brief
// without a milestone rather than failing over one line of it.
func milestoneOf(s domain.Slice, shape notion.SliceShape) domain.Milestone {
	for _, m := range milestonesOf(shape) {
		if m.ID == s.MilestoneID && s.MilestoneID != "" {
			return m
		}
	}
	return domain.Milestone{}
}
