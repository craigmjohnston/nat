package agent

import (
	"fmt"
	"strings"

	"github.com/craigmjohnston/notion-agent-tracker/internal/config"
	"github.com/craigmjohnston/notion-agent-tracker/internal/domain"
)

// PromptContext is everything a fresh agent session needs to be told about the
// slice it is picking up. WorkingDir is the directory the agent will start in,
// resolved by the caller — the slice's Repo override, the project default, or
// whatever the launch form was edited to.
type PromptContext struct {
	Slice         domain.Slice
	Project       config.ProjectConfig
	ProjectPageID string
	WorkingDir    string
	AssigneeName  string
}

// Prompt is the opening message for an agent session working one slice.
//
// The agent starts with no history, so the prompt has to carry the whole
// contract: which slice, how to claim it before touching anything, where to
// read the brief and the project's conventions, and how to record the outcome.
// It deliberately does not restate the brief itself — the slice page is the
// single source of truth for that, and copying it here would let the two drift.
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
	b.WriteString("Before doing any work, use the Notion MCP to update the slice page:\n")
	fmt.Fprintf(&b, "set `Assignee` to %s and `Status` to `Claimed`.\n", c.AssigneeName)
	b.WriteString("Re-read the page afterwards to confirm the claim stuck; if another agent\n")
	b.WriteString("got there first, stop and say so rather than working the slice anyway.\n")

	b.WriteString("\n## Then read, in order\n\n")
	b.WriteString("1. The slice page body — that is your brief and its acceptance criteria.\n")
	if c.ProjectPageID != "" {
		fmt.Fprintf(&b, "2. The project page (%s) — conventions that apply to every slice.\n", c.ProjectPageID)
		b.WriteString("3. `CLAUDE.md` in the working directory — architecture and the verification gate.\n")
	} else {
		b.WriteString("2. `CLAUDE.md` in the working directory — architecture and the verification gate.\n")
	}

	b.WriteString("\n## Do the work\n\n")
	b.WriteString("Work in the working directory above; if that is not where this session\n")
	b.WriteString("started, use absolute paths or `git -C`. Honour the brief's acceptance\n")
	b.WriteString("criteria and the project's verification gate before calling it done.\n\n")
	b.WriteString("If the work is code: branch for the slice, keep it to exactly ONE pull\n")
	b.WriteString("request, commit, push the branch, open the PR (do not merge it), and write\n")
	b.WriteString("the PR URL to the slice's `PR` property.\n\n")
	b.WriteString("If the work is not code — docs, research, Notion content — produce the\n")
	b.WriteString("deliverable the brief asks for and link it on the slice page.\n")

	b.WriteString("\n## Finish\n\n")
	b.WriteString("On completion, set the slice `Status` to `Done` and append a short summary\n")
	b.WriteString("to the slice page: what you did, key decisions, follow-ups worth queueing.\n\n")
	b.WriteString("If you cannot complete it, leave `Status` as `Claimed` and append a note\n")
	b.WriteString("saying exactly what is blocking you.\n")

	b.WriteString("\n## Guardrails\n\n")
	b.WriteString("- One slice per session. Never pick up another when this one is done.\n")
	b.WriteString("- Never modify other slices, milestones, or the project page.\n")
	b.WriteString("- Never merge a PR or push to the main branch.\n")

	return b.String()
}

// repoOverridden reports whether the agent is being sent somewhere other than
// the project's default working directory, which is worth calling out in the
// prompt so the agent does not assume the default.
func repoOverridden(c PromptContext) bool {
	return c.Project.WorkingDir != "" && c.WorkingDir != c.Project.WorkingDir
}
