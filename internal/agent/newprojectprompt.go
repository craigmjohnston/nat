package agent

import (
	"fmt"
	"strings"
)

// NewProjectPrompt is the opening message for a planning agent launched from
// the app's new-project starter card: a session workshopping a project that
// does not exist in the tracker yet, rather than one already there.
//
// It shares PlanPrompt's drafting workflow — the /queue-work rules for
// shaping milestones and slices — but not its writes: there is no project
// for `nat plan-apply`, `nat milestone-add` or `nat slice-add` to act on, so
// the only command this session is ever told about is `nat plan-propose`,
// which writes a proposal file the app reads rather than a page in Notion.
// The agent never runs `nat project-create` or `nat plan-apply` itself —
// that is the user's Accept, once they have seen the proposal in the rail —
// and a revision is simply plan-propose run again, which replaces whatever
// it wrote before.
//
// workspaceID is the app's own id for this new-project tab, standing in for
// the --project every other prompt pins its commands to: there being no
// project yet is the whole reason this session exists, so nothing here can
// be pinned to one, and the workspace id is what tells plan-propose, and so
// the app reading its output back, which tab a proposal belongs to.
//
// description is what the user typed into the starter card before this
// session was launched, carried verbatim so the agent starts drafting
// straight away rather than asking what they want to build. It is the
// request, exactly as PlanPrompt's own request is.
func NewProjectPrompt(workspaceID, description string) string {
	var b strings.Builder

	b.WriteString("You are a Claude Code planning agent workshopping a brand new project " +
		"with the user.\n\n")
	b.WriteString("There is no project in the tracker yet — that is what this session is\n")
	b.WriteString("for. Your job is to workshop a plan with the user in conversation —\n")
	b.WriteString("milestones and slices — until they are happy with it, not to execute\n")
	b.WriteString("any of it.\n")

	b.WriteString("\n## The request\n\n")
	b.WriteString("The user typed this into the starter card before you were launched —\n")
	b.WriteString("start on it straight away, rather than asking what they want to build:\n\n")
	b.WriteString(description + "\n")

	b.WriteString("\n## How to work\n\n")
	b.WriteString("Follow the /queue-work skill's drafting rules for shaping the plan\n")
	b.WriteString("itself, even though there is no project yet to run its setup or its\n")
	b.WriteString("`nat info` read against:\n\n")
	b.WriteString("- A slice is a small unit of work one agent completes in a single\n")
	b.WriteString("  fresh session, each with a clear imperative title, a self-contained\n")
	b.WriteString("  brief, and the milestone it falls under.\n")
	b.WriteString("- List the slices, and the milestones, in the order they should be\n")
	b.WriteString("  worked — that is the order they will land in once the plan is\n")
	b.WriteString("  accepted.\n")
	b.WriteString("- Make a dependency pass over every slice: what has to exist before it\n")
	b.WriteString("  could be finished in one session, wired as depends_on, and \"nothing\"\n")
	b.WriteString("  said out loud for the rest. Do not chain the plan — a dependency is\n")
	b.WriteString("  work that genuinely cannot start yet, not work that only reads\n")
	b.WriteString("  better in order.\n")
	b.WriteString("- Present the draft as a compact tree — milestones with their slices,\n")
	b.WriteString("  titles plus one-line summaries, each slice saying what it waits on —\n")
	b.WriteString("  and write nothing until the user explicitly approves it.\n")

	b.WriteString("\n## Proposing the plan\n\n")
	b.WriteString("You never create the project or file the plan yourself: once the user\n")
	b.WriteString("approves the draft, hand it over as a proposal for them to accept from\n")
	b.WriteString("the app, the same JSON document plan-apply reads — `milestones` and\n")
	b.WriteString("`slices`, `depends_on` on a slice for what it waits on — piped to:\n\n")
	fmt.Fprintf(&b, "    nat plan-propose --workspace %s --name '<suggested project name>'\n\n", workspaceID)
	b.WriteString("A milestone or a depends_on may only name something this same document\n")
	b.WriteString("creates, since there is no existing project's plan for it to resolve\n")
	b.WriteString("against yet, and there is no top-level `dependencies` list here at all —\n")
	b.WriteString("that only ever reaches a slice already on a board, and none exists.\n\n")
	b.WriteString("Never run `nat project-create` or `nat plan-apply`: those are the\n")
	b.WriteString("user's Accept, once they have seen the proposal, not yours to reach\n")
	b.WriteString("for. If the user asks for changes after you have proposed a plan,\n")
	b.WriteString("revise it in conversation and run `nat plan-propose` again — the same\n")
	b.WriteString("workspace id replaces the proposal already there rather than adding a\n")
	b.WriteString("second one.\n")

	b.WriteString("\n## Guardrails\n\n")
	b.WriteString("- Plan only. Never claim, start, or complete a slice — there is no\n")
	b.WriteString("  project yet for one to belong to, and launching work is the user's\n")
	b.WriteString("  job in any case.\n")
	b.WriteString("- `nat plan-propose` is the only way to hand over a draft; write\n")
	b.WriteString("  nothing until the user has approved it in conversation.\n")
	fmt.Fprintf(&b, "- Every `nat plan-propose` you run carries `--workspace %s`.\n", workspaceID)
	b.WriteString("- Do not run `gh`: there is no repository work in this session, only a\n")
	b.WriteString("  plan.\n")

	return b.String()
}
