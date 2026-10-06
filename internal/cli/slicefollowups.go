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

// sliceFollowUps files the follow-ups an agent noticed beside its slice and did
// not do, for the user to triage before the agent hands back: each one queued
// as a slice of its own, folded into this one, or dropped. The agent stops once
// it has run this, and the user's decision reaches it as a message
// (`slice-triage`); complete-slice refuses until it has.
//
// Only a slice this user holds takes them, for the reason complete-slice is
// held to the same rule: the agent working it is the one with something to
// hand in.
func sliceFollowUps(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-followups", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var given stringList
	flags.Var(&given, "follow-up", "a follow-up: its first line the title, the rest its brief; repeat for more")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-followups: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-followups", rest[0])
	if err != nil {
		return err
	}
	items, err := followUpsOf(given)
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
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}
	shape, err := sliceShape(ctx, st, projectID, project)
	if err != nil {
		return err
	}
	s, pageShape, err := loadSlice(ctx, st, id)
	if err != nil {
		return err
	}
	if !store.Holds(s, shape.On(pageShape), cfg.AssigneeUserID) {
		return notOursError(s, cfg.AssigneeUserName, "given follow-ups")
	}

	if err := st.ProposeFollowUps(ctx, s.ID, items); err != nil {
		return fmt.Errorf("file the follow-ups: %w", err)
	}
	env.nudged()
	_, err = io.WriteString(env.Out, followUpsFiledMarkdown(s, items))
	return err
}

// followUpsOf reads each --follow-up as the follow-up it describes, settling
// every refusal before anything is read or written: none at all, one with no
// title, no brief or no `Done when:` line — a queued follow-up's brief is the
// new slice's, word for word, and one with no done-condition is a slice no
// agent can finish — and two of the same title — the title is what the triage
// record names each one by, so two alike could never be told apart in it.
func followUpsOf(given []string) ([]store.FollowUp, error) {
	if len(given) == 0 {
		return nil, usageErrorf("slice-followups: no follow-up given: pass --follow-up '<title>\\n\\n<brief>'")
	}
	seen := map[string]bool{}
	items := make([]store.FollowUp, 0, len(given))
	for _, g := range given {
		title, brief, _ := strings.Cut(strings.TrimSpace(strings.ReplaceAll(g, "\r\n", "\n")), "\n")
		title, brief = strings.TrimSpace(title), strings.TrimSpace(brief)
		switch {
		case title == "":
			return nil, usageErrorf("slice-followups: a follow-up has no title: its first line is the title")
		case brief == "":
			return nil, usageErrorf("slice-followups: %q has no brief: say the change after the title line, then a \"Done when:\" line", title)
		case !hasDoneWhen(brief):
			return nil, usageErrorf("slice-followups: %q has no \"Done when:\" line: say how anyone checks it is finished", title)
		case seen[title]:
			return nil, usageErrorf("slice-followups: two follow-ups are titled %q: give each its own title", title)
		}
		// A queued follow-up's title is the new slice's, so it is held to the
		// cap here, where the agent can still shorten it, rather than at the
		// user's queue.
		if err := domain.CheckSliceTitle(title); err != nil {
			return nil, usageErrorf("slice-followups: %v", err)
		}
		seen[title] = true
		items = append(items, store.FollowUp{Title: title, Brief: brief})
	}
	return items, nil
}

// hasDoneWhen is whether a brief has a line beginning `Done when:`.
func hasDoneWhen(brief string) bool {
	for _, line := range strings.Split(brief, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Done when:") {
			return true
		}
	}
	return false
}

// followUpsFiledMarkdown says what was filed and what the agent does now, which
// is nothing: the decision is the user's, and it arrives as a message.
func followUpsFiledMarkdown(s domain.Slice, items []store.FollowUp) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Name)
	fmt.Fprintf(&b, "%s filed on the slice page for the user to triage:\n\n", counted(len(items), "follow-up"))
	for i, it := range items {
		fmt.Fprintf(&b, "%d. %s\n", i+1, it.Title)
	}
	b.WriteString("\nWaiting for the user's decision — it arrives as a message; do not hand back before it does.\n")
	return b.String()
}

// counted is a count and its noun, in agreement.
func counted(n int, noun string) string {
	return fmt.Sprintf("%d %s", n, plural(noun, n))
}
