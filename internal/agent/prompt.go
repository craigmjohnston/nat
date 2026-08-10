package agent

import (
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// PromptContext is everything a fresh agent session needs to be told about the
// slice it is picking up. WorkingDir is the directory the agent will start in,
// resolved by the caller — the slice's Repo override, the project default, or
// whatever the launch form was edited to.
type PromptContext struct {
	Slice        domain.Slice
	Project      config.ProjectConfig
	WorkingDir   string
	AssigneeName string
}

// Prompt is the opening message for an agent session working one slice.
//
// The agent starts with no history, so the prompt has to carry the whole
// contract: which slice, how to claim it before touching anything, where to
// read the brief and the project's conventions, and how to record the outcome.
// It deliberately does not restate the brief itself — the slice page is the
// single source of truth for that, and copying it here would let the two drift.
//
// Every step that touches the tracker is a `nat` command. The agent is told
// nothing about Notion — not the data sources, not the properties, not even
// that Notion is what is behind the commands — because the commands are the
// only writes it is allowed to make, and a prompt that also explained the
// underlying pages would be describing a second way to do the same thing.
func Prompt(c PromptContext) string {
	var b strings.Builder

	fmt.Fprintf(&b, "You are a Claude Code agent working exactly one slice of the %q project.\n\n", c.Project.Name)

	b.WriteString("## The slice\n\n")
	fmt.Fprintf(&b, "- Name: %s\n", c.Slice.Name)
	fmt.Fprintf(&b, "- Notion page ID: %s\n", c.Slice.ID)
	if c.Slice.URL != "" {
		fmt.Fprintf(&b, "- Notion URL: %s\n", c.Slice.URL)
	}
	fmt.Fprintf(&b, "- Working directory: %s\n", c.WorkingDir)
	if repoOverridden(c) {
		fmt.Fprintf(&b, "  (this slice overrides the project default of %s)\n", c.Project.WorkingDir)
	}

	b.WriteString("\n## Claim it first\n\n")
	b.WriteString("Before doing any work, run:\n\n")
	fmt.Fprintf(&b, "    nat start-slice %s\n\n", c.Slice.ID)
	fmt.Fprintf(&b, "That claims the slice for %s and prints your brief: the slice's own\n", c.AssigneeName)
	b.WriteString("body and acceptance criteria, followed by the project's conventions.\n")
	b.WriteString("If it refuses — the slice is already claimed, or already done — stop and\n")
	b.WriteString("say so rather than working the slice anyway.\n")

	b.WriteString("\n## Then read\n\n")
	b.WriteString("1. The brief the command printed — the slice, then the conventions that\n")
	b.WriteString("   apply to every slice of the project.\n")
	b.WriteString("2. `CLAUDE.md` in the working directory — architecture and the\n")
	b.WriteString("   verification gate.\n")

	b.WriteString("\n## Do the work\n\n")
	b.WriteString("Work in the working directory above; if that is not where this session\n")
	b.WriteString("started, use absolute paths or `git -C`. Honour the brief's acceptance\n")
	b.WriteString("criteria and the project's verification gate before calling it done.\n\n")
	b.WriteString("If the work is code: branch for the slice, keep it to exactly ONE pull\n")
	b.WriteString("request, commit, push the branch, and open the PR (do not merge it).\n\n")
	b.WriteString("If the work is not code — docs, research, written-up findings — produce\n")
	b.WriteString("the deliverable the brief asks for and link it in the summary below.\n")

	b.WriteString("\n## Finish\n\n")
	b.WriteString("On completion, record the outcome:\n\n")
	fmt.Fprintf(&b, "    nat complete-slice %s --pr <URL> --summary '<what you did>'\n\n", c.Slice.ID)
	b.WriteString("That marks the slice Done and writes the summary onto its page: what you\n")
	b.WriteString("did, key decisions, follow-ups worth queueing. Leave `--pr` off when the\n")
	b.WriteString("slice produced no pull request. A summary too long for one argument can\n")
	b.WriteString("be piped in on stdin instead of passing `--summary`.\n\n")
	b.WriteString("If you cannot complete it, say what stopped you and leave the slice\n")
	b.WriteString("claimed, so nobody else picks it up on top of your work:\n\n")
	fmt.Fprintf(&b, "    nat complete-slice %s --blocked --summary '<what is blocking>'\n", c.Slice.ID)

	b.WriteString("\n## Guardrails\n\n")
	b.WriteString("- One slice per session. Never pick up another when this one is done.\n")
	b.WriteString("- The `nat` commands are the only way to record anything about the slice.\n")
	b.WriteString("- Never touch other slices, other milestones, or the plan itself.\n")
	b.WriteString("- Never merge a PR or push to the main branch.\n")

	return b.String()
}

// repoOverridden reports whether the agent is being sent somewhere other than
// the project's default working directory, which is worth calling out in the
// prompt so the agent does not assume the default.
func repoOverridden(c PromptContext) bool {
	return c.Project.WorkingDir != "" && c.WorkingDir != c.Project.WorkingDir
}
