package agent

import (
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/notion"
)

// PromptContext is everything a fresh agent session needs to be told about the
// slice it is picking up. WorkingDir is the directory the agent will start in,
// resolved by the caller — the slice's Repo override, the project default, or
// whatever the launch form was edited to, and the worktree cut from any of them
// where there is one.
//
// Branch and Repo describe that worktree: the branch it is already on, and the
// checkout it was cut from. Both are empty where the session runs in the
// checkout itself, which is what the agent is told to branch for itself.
//
// ProjectID is the project's own page ID, which is what every `nat` command in
// the prompt names with --project. It comes from the caller rather than from
// Project, since a ProjectConfig is a value of the config file's Projects map
// and the ID is the key it is filed under.
//
// Fix says the session is not working the slice but the review of the pull
// request it already produced: the slice is Done, that pull request is still
// open, and the agent is being sent at the comments and the failing checks on
// it. It is the launch's own word rather than something read back off the
// slice, since it is the launch that established the pull request is open —
// see [fixPrompt].
type PromptContext struct {
	Slice        domain.Slice
	Project      config.ProjectConfig
	ProjectID    string
	WorkingDir   string
	Branch       string
	Repo         string
	AssigneeName string
	Fix          bool
}

// Prompt is the opening message for an agent session working one slice.
//
// The agent starts with no history, so the prompt has to carry the whole
// contract: which slice, how to claim it before touching anything, where to
// read the brief and the project's conventions, and how to record the outcome.
// It deliberately does not restate the brief itself — the slice page is the
// single source of truth for that, and copying it here would let the two drift.
//
// A relaunch is told so: a slice already in progress is one `start-slice`
// re-opens rather than refuses, and a session placed on the very branch the
// slice records is continuing work that is already there — see [resuming]. Both
// are said rather than left to be discovered, since an agent that read the
// ordinary prompt would take a refusal for a stop and a branch full of commits
// for somebody else's.
//
// Every step that touches the tracker is a `nat` command. The agent is told
// nothing about Notion — not the data sources, not the properties, not even
// that Notion is what is behind the commands — because the commands are the
// only writes it is allowed to make, and a prompt that also explained the
// underlying pages would be describing a second way to do the same thing.
//
// The ending it is told to reach is a hand-back: the branch pushed and recorded
// with `complete-slice --branch`, the slice left in progress for the user to
// review on the board, which is where the pull request is opened from. So the
// prompt tells the agent not to run `gh` at all — an agent that opened its own
// pull request would put the work past the review the approve key answers,
// and `gh pr create` on a branch that already has one refuses anyway.
//
// Every one of those commands names the project it acts on with --project,
// which they require: a command given none is refused rather than falling back
// to the project the board is on, since the user changes that while an agent
// runs. The prompt pins them to the project of the launch, which is the one the
// slice is in and cannot change under a session, and says why, since the
// commands an agent runs of its own accord are the ones no template can spell
// out.
// A session sent at an open pull request rather than at the slice is told
// something else entirely — see [fixPrompt] — so it is dispatched here rather
// than woven through the sections below: nothing about claiming, working or
// handing back a slice applies to work that has already been published.
func Prompt(c PromptContext) string {
	if c.Fix {
		return fixPrompt(c)
	}
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
	if c.Branch != "" {
		fmt.Fprintf(&b, "- Branch: %s (the working directory is a worktree already on it)\n", c.Branch)
	}
	if resuming(c) {
		b.WriteString("- There is work on that branch already: an earlier session pushed it and\n")
		b.WriteString("  handed it back. You are continuing that work, not starting again.\n")
	}

	b.WriteString("\n## Claim it first\n\n")
	b.WriteString("Before doing any work, run:\n\n")
	fmt.Fprintf(&b, "    nat start-slice %s --project %s\n\n", c.Slice.ID, c.ProjectID)
	if c.Slice.Status == domain.SliceClaimed {
		fmt.Fprintf(&b, "The slice is already in progress and held by %s, so that\n", c.AssigneeName)
		b.WriteString("command re-opens it for you and writes nothing at all. Either way it\n")
		b.WriteString("prints your brief: the slice's own body and acceptance criteria,\n")
		b.WriteString("followed by the project's conventions. If it refuses — somebody else\n")
		b.WriteString("holds the slice, or it is already done — stop and say so rather than\n")
		b.WriteString("working it anyway.\n\n")
	} else {
		fmt.Fprintf(&b, "That claims the slice for %s and prints your brief: the slice's own\n", c.AssigneeName)
		b.WriteString("body and acceptance criteria, followed by the project's conventions.\n")
		b.WriteString("If it refuses — the slice is already claimed, or already done — stop and\n")
		b.WriteString("say so rather than working the slice anyway.\n\n")
	}
	b.WriteString("Every `nat` command below names the project this slice is in:\n\n")
	fmt.Fprintf(&b, "    --project %s\n\n", c.ProjectID)
	b.WriteString("Put it on any other one you run too.\n")
	b.WriteString("A command given no project is refused: there is nothing for it to fall\n")
	b.WriteString("back to, and in particular not the project the user's board is on,\n")
	b.WriteString("which they can switch while you work.\n")

	b.WriteString("\n## Then read\n\n")
	b.WriteString("1. The brief the command printed — the slice, then the conventions that\n")
	b.WriteString("   apply to every slice of the project.\n")
	b.WriteString("2. `CLAUDE.md` in the working directory — architecture and the\n")
	b.WriteString("   verification gate.\n")

	b.WriteString("\n## Do the work\n\n")
	b.WriteString("Work in the working directory above; if that is not where this session\n")
	b.WriteString("started, use absolute paths or `git -C`. Honour the brief's acceptance\n")
	b.WriteString("criteria and the project's verification gate before calling it done.\n\n")
	switch {
	case resuming(c):
		b.WriteString("That directory is a git worktree cut for this slice alone, already on\n")
		fmt.Fprintf(&b, "the branch %s and shared with nobody, and the work an earlier\n", c.Branch)
		b.WriteString("session pushed is already on it. Read what is there before adding to\n")
		b.WriteString("it — the commits on the branch, and the summary the slice page carries\n")
		fmt.Fprintf(&b, "of what that session did. Commit your own work there and push %s\n", c.Branch)
		b.WriteString("again: the same branch, which is the one the review is against. Do not\n")
		b.WriteString("create a branch of your own and do not switch to another; this one is\n")
		b.WriteString("yours and is what you hand back. Do not run `gh`, and do not open a\n")
		b.WriteString("pull request: you hand the branch back and the user opens the pull\n")
		b.WriteString("request from the board once they have reviewed it.\n\n")
	case c.Branch != "":
		fmt.Fprintf(&b, "That directory is a git worktree cut for this slice alone, already on\n")
		fmt.Fprintf(&b, "the branch %s and shared with nobody. If the work is code: commit\n", c.Branch)
		fmt.Fprintf(&b, "there — exactly ONE change, this slice's — and push %s. Do not\n", c.Branch)
		b.WriteString("create a branch of your own and do not switch to another; this one is\n")
		b.WriteString("yours and is what you hand back. Do not run `gh`, and do not open a\n")
		b.WriteString("pull request: you hand the branch back and the user opens the pull\n")
		b.WriteString("request from the board once they have reviewed it.\n\n")
	default:
		b.WriteString("If the work is code: branch for the slice — one branch, and exactly ONE\n")
		b.WriteString("change on it — commit, and push the branch. Do not run `gh`, and do not\n")
		b.WriteString("open a pull request: you hand the branch back and the user opens the\n")
		b.WriteString("pull request from the board once they have reviewed it.\n\n")
	}
	b.WriteString("If the work is not code — docs, research, written-up findings — produce\n")
	b.WriteString("the deliverable the brief asks for and link it in the summary below.\n")

	b.WriteString("\n## Finish\n\n")
	b.WriteString("On completion, record the outcome:\n\n")
	fmt.Fprintf(&b, "    nat complete-slice %s --project %s \\\n", c.Slice.ID, c.ProjectID)
	fmt.Fprintf(&b, "        --branch %s --summary '<what you did>' \\\n", branchArg(c))
	b.WriteString("        --pr-description '<title line>\n\n<what the PR does and why>'\n\n")
	b.WriteString("That records the branch you pushed and hands the slice back for review,\n")
	b.WriteString("writing the summary onto its page: what you did, key decisions, follow-ups\n")
	b.WriteString("worth queueing. It leaves the slice in progress on purpose — approving it\n")
	b.WriteString("on the board is what opens the pull request and marks it Done.\n\n")
	b.WriteString("`--pr-description` is what that pull request is opened with: its first\n")
	b.WriteString("line is the title and the rest the body, so write it ready to publish —\n")
	b.WriteString("what the change does and why, for whoever reviews it on GitHub, not a\n")
	b.WriteString("report of your session. Pass `--pr-description -` to read it from stdin\n")
	b.WriteString("when it is too long for an argument, and give `--summary` as a flag then.\n\n")
	b.WriteString("Leave `--branch` off when the slice produced no branch — a docs or\n")
	b.WriteString("research slice — and it is marked Done there and then, with no pull\n")
	b.WriteString("request to describe. A summary too long for one argument can be piped in\n")
	b.WriteString("on stdin instead of passing `--summary`.\n\n")
	b.WriteString("If you cannot complete it, say what stopped you and leave the slice\n")
	b.WriteString("in progress, so nobody else picks it up on top of your work:\n\n")
	fmt.Fprintf(&b, "    nat complete-slice %s --project %s \\\n", c.Slice.ID, c.ProjectID)
	b.WriteString("        --blocked --summary '<what is blocking>'\n")

	b.WriteString("\n## Guardrails\n\n")
	b.WriteString("- One slice per session. Never pick up another when this one is done.\n")
	b.WriteString("- The `nat` commands are the only way to record anything about the slice.\n")
	fmt.Fprintf(&b, "- Every one of them carries `--project %s`.\n", c.ProjectID)
	b.WriteString("- Never touch other slices, other milestones, or the plan itself.\n")
	b.WriteString("- Never open or merge a pull request, and never push to the main branch.\n")

	return b.String()
}

// PlanPrompt is the opening message for a planning agent session: an agent
// launched from the board to workshop the plan itself — milestones and slices —
// with the user, rather than to execute a slice.
//
// Like Prompt, it routes every write through the `nat` commands and says
// nothing about Notion: the planning commands are the only writes the agent is
// allowed, and they enforce the drafting rules themselves. The workflow lives
// in the /queue-work skill rather than being restated here, for the same
// reason the slice prompt does not restate the brief.
//
// request is what the user typed into the launch input: the thing they want to
// workshop, carried in the prompt so the agent starts on it rather than
// opening with a question. Empty means a plain planning session.
//
// projectID is the project's own page ID, which every command in the prompt
// names with --project: a planning session outlives the board's idea of which
// project is active, and a plan written into the project the user has since
// switched to is the one mistake none of the drafting rules would catch.
func PlanPrompt(projectID, projectName, workingDir, request string) string {
	b := planBody(projectID, projectName, workingDir)

	if request != "" {
		b.WriteString("\n## The request\n\n")
		b.WriteString("The user launched you with this in hand — start on it straight away,\n")
		b.WriteString("rather than asking what they want to work on:\n\n")
		b.WriteString(request + "\n")
	}

	return b.String()
}

// WishlistPrompt is the opening message for a planning agent launched on the
// project's wishlist: the same planning session PlanPrompt describes, with the
// items themselves in place of the question the user would otherwise have been
// asked. They are the request.
//
// The items are carried whole — sub-bullets and all, as the page holds them —
// and their block IDs come with them, because tidying up after the plan is
// written is part of the job and `nat wishlist-clear` addresses items by ID.
// The clear is deliberately spelled out as the last step rather than the first:
// an item cleared before the plan lands is an idea lost, and an item the agent
// never read is somebody's newer idea, typed while the session ran.
func WishlistPrompt(projectID, projectName, workingDir string, items []notion.WishlistItem) string {
	if len(items) == 0 {
		return PlanPrompt(projectID, projectName, workingDir, "")
	}
	b := planBody(projectID, projectName, workingDir)

	b.WriteString("\n## The request\n\n")
	b.WriteString("The user launched you on their wishlist — the ideas they have been\n")
	b.WriteString("jotting on the project page. These are the request: start on them\n")
	b.WriteString("straight away, rather than asking what they want to work on.\n\n")
	for _, item := range items {
		b.WriteString(item.Markdown + "\n")
	}

	b.WriteString("\n## Clearing them\n\n")
	b.WriteString("An item that the approved plan now covers has been captured, so take it\n")
	b.WriteString("off the wishlist — once the plan is written, and not before:\n\n")
	fmt.Fprintf(b, "    nat wishlist-clear %s \\\n        --project %s\n\n",
		strings.Join(itemIDs(items), " "), projectID)
	b.WriteString("Name only the items above, and only the ones the plan covers: an idea\n")
	b.WriteString("the user set aside stays on the wishlist, and so does one typed while\n")
	b.WriteString("this session ran — which is why the command names items rather than\n")
	b.WriteString("emptying the section.\n")

	return b.String()
}

// itemIDs are the wishlist items' block IDs, in the order they were read: what
// the clear command names.
func itemIDs(items []notion.WishlistItem) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}

// planBody is everything a planning session is told before the request it was
// launched on: the job, the workflow, the commands, the guardrails. Both
// planning prompts open with it, so a wishlist launch and a typed one differ
// only in what they are pointed at.
func planBody(projectID, projectName, workingDir string) *strings.Builder {
	b := &strings.Builder{}

	fmt.Fprintf(b, "You are a Claude Code planning agent for the %q project.\n\n", projectName)
	b.WriteString("Your job is to workshop the plan itself with the user — reshape\n")
	b.WriteString("milestones, draft new slices — not to execute any slice.\n")

	b.WriteString("\n## How to work\n\n")
	b.WriteString("Follow the /queue-work skill: it is the planning workflow for this\n")
	b.WriteString("tracker. Start by running:\n\n")
	fmt.Fprintf(b, "    nat info --project %s\n\n", projectID)
	b.WriteString("That prints the current plan: the project's conventions, its milestones\n")
	b.WriteString("in plan order, and the slices under them (`--json` to parse it instead).\n\n")
	b.WriteString("Every `nat` command you run names the project you are planning:\n\n")
	fmt.Fprintf(b, "    --project %s\n\n", projectID)
	b.WriteString("A command given no project is refused: there is nothing for it to fall\n")
	b.WriteString("back to, and in particular not the project the user's board is on,\n")
	b.WriteString("which they can switch while you work.\n")

	b.WriteString("\n## Applying changes\n\n")
	b.WriteString("Draft in conversation first, and write only after the user explicitly\n")
	b.WriteString("approves. The `nat` planning commands are the only way to change the\n")
	b.WriteString("plan:\n\n")
	fmt.Fprintf(b, "- `nat plan-apply [FILE] --project %s` — a whole drafted\n", projectID)
	b.WriteString("  plan of milestones and slices at once, from a JSON document (stdin\n")
	b.WriteString("  without FILE)\n")
	fmt.Fprintf(b, "- `nat milestone-add <name> --project %s` — one new\n", projectID)
	b.WriteString("  milestone, Queued, at the end of the plan\n")
	fmt.Fprintf(b, "- `nat slice-add <title> --milestone <name> [--description -] --project %s`\n", projectID)
	b.WriteString("  — one new Todo slice, its brief read from stdin\n")

	b.WriteString("\n## Guardrails\n\n")
	b.WriteString("- Plan only. Never claim, start, or complete a slice — launching work is\n")
	b.WriteString("  the board's job, not yours.\n")
	b.WriteString("- Never touch work in flight: slices in progress and Done slices, and the\n")
	b.WriteString("  milestones holding them, are records of what happened.\n")
	b.WriteString("- The commands above are the only way to change the plan; write nothing\n")
	b.WriteString("  until the user has approved the draft.\n")
	fmt.Fprintf(b, "- Every one of them carries `--project %s`.\n", projectID)
	fmt.Fprintf(b, "- This session starts in %s; the user's board picks up\n", workingDir)
	b.WriteString("  your changes when you exit, or on its refresh key.\n")

	return b
}

// resuming reports whether the session is picking work up rather than starting
// it: the worktree it is placed in is on the very branch the slice records, so
// there are commits there already and an earlier session put them there.
//
// It is the branch matching that says so rather than the slice's status alone,
// because a released slice is back at Todo with its branch still recorded and
// its work still exactly what the next session wants, and because a launch that
// fell back to the shared checkout has no worktree to have found the work in.
func resuming(c PromptContext) bool {
	return c.Branch != "" && c.Branch == strings.TrimSpace(c.Slice.Branch)
}

// branchArg is what the hand-back command names: the branch the session's
// worktree is already on, or the placeholder for an agent that will make one.
func branchArg(c PromptContext) string {
	if c.Branch != "" {
		return c.Branch
	}
	return "<branch>"
}

// repoOverridden reports whether the agent is being sent somewhere other than
// the project's default working directory, which is worth calling out in the
// prompt so the agent does not assume the default.
//
// The comparison is against the checkout rather than the session's own
// directory, because a worktree is never the default one: reporting every
// worktree launch as an override would say nothing about the slice.
func repoOverridden(c PromptContext) bool {
	dir := c.WorkingDir
	if c.Repo != "" {
		dir = c.Repo
	}
	return c.Project.WorkingDir != "" && dir != c.Project.WorkingDir
}
