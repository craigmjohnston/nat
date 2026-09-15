package agent

import (
	"fmt"
	"strings"
)

// fixPrompt is the opening message for a session launched on a slice that is
// already Done: the work it produced became a pull request, that pull request
// is still open, and what is left of the slice is the review on it — comments
// to answer and checks to get green.
//
// It is a prompt of its own rather than a branch of [Prompt] because almost
// none of the slice contract applies. There is nothing to claim: `start-slice`
// refuses a Done slice, and so does every other command that would move one, so
// the prompt names none of them and tells the agent the record stands. There is
// nothing to hand back either: the pull request is built from the branch, so a
// push to it is the whole of the ending, and the board's merge key is what the
// work lands with.
//
// The brief is the review rather than the slice page: `gh pr view --comments`
// and `gh pr checks`, read once at launch and carried straight into the
// prompt, the same as the slice's own brief is for an ordinary launch. This
// used to read the two live instead, on the theory that the review moves
// while the session runs and a launch-time copy would already be stale — but
// a snapshot taken seconds before the agent's own first read of the same two
// commands is exactly as fresh, and the prompt still tells the agent to run
// them again itself before it pushes, so nothing that moved in those few
// seconds goes uncaught. Those two reads are the one place the ordinary
// prohibition on `gh` is relaxed: opening, merging and closing a pull
// request are still the user's alone.
//
// Everything the two prompts do share is shared for the same reasons as ever:
// the working directory and the branch, so the session and the board never
// disagree about where the work is, and --project on the one `nat` command
// there is, since a session outlives the board's idea of which project is
// active.
func fixPrompt(c PromptContext) string {
	var b strings.Builder

	fmt.Fprintf(&b, "You are a Claude Code agent working the review of one already-published\nslice of the %q project.\n\n", c.Project.Name)
	b.WriteString(frontendNote(c.Frontend))

	b.WriteString("## The slice and its pull request\n\n")
	fmt.Fprintf(&b, "- Name: %s\n", c.Slice.Name)
	fmt.Fprintf(&b, "- Notion page ID: %s\n", c.Slice.ID)
	if c.Slice.URL != "" {
		fmt.Fprintf(&b, "- Notion URL: %s\n", c.Slice.URL)
	}
	fmt.Fprintf(&b, "- Pull request: %s\n", c.Slice.PRURL)
	fmt.Fprintf(&b, "- Working directory: %s\n", c.WorkingDir)
	if repoOverridden(c) {
		fmt.Fprintf(&b, "  (this slice overrides the project default of %s)\n", c.Project.WorkingDir)
	}
	if c.Branch != "" {
		fmt.Fprintf(&b, "- Branch: %s (the working directory is a worktree already on it)\n", c.Branch)
	}

	b.WriteString("\n## What is already true\n\n")
	b.WriteString("The slice is recorded as done and its pull request is open. The work was\n")
	b.WriteString("written, reviewed and published; none of that changes here, and there is\n")
	b.WriteString("nothing about the slice for you to claim, complete or record. Leave the\n")
	b.WriteString("tracker exactly as you found it: the account of what was done is written\n")
	b.WriteString("and it stands.\n")

	b.WriteString("\n## Your job\n\n")
	b.WriteString("Get that pull request to a state where it can be merged: answer the\n")
	b.WriteString("comments left on the review, and fix whatever checks are failing.\n\n")
	if c.ReviewComments != "" || c.ReviewChecks != "" {
		b.WriteString("Captured at launch — no need to re-run this to see where it stood then:\n\n")
		if c.ReviewComments != "" {
			fmt.Fprintf(&b, "`gh pr view %s --comments`:\n\n```\n%s\n```\n\n", c.Slice.PRURL, c.ReviewComments)
		}
		if c.ReviewChecks != "" {
			fmt.Fprintf(&b, "`gh pr checks %s`:\n\n```\n%s\n```\n\n", c.Slice.PRURL, c.ReviewChecks)
		}
	}
	b.WriteString("Re-check before you push, since either can have moved since launch:\n\n")
	fmt.Fprintf(&b, "    gh pr view %s --comments\n", c.Slice.PRURL)
	fmt.Fprintf(&b, "    gh pr checks %s\n\n", c.Slice.PRURL)
	b.WriteString("Those two reads are the only `gh` you may run. Never open, merge, close\n")
	if c.Frontend == FrontendGnat {
		b.WriteString("or reopen a pull request: merging this one is a button in the app's PR\n")
		b.WriteString("tab, pressed once they are satisfied with what you did.\n\n")
	} else {
		b.WriteString("or reopen a pull request: merging this one is a key on the user's board,\n")
		b.WriteString("pressed once they are satisfied with what you did.\n\n")
	}
	b.WriteString("Read files with the Read tool, not `cat`/`sed`/`head`, and edit with Edit\n")
	b.WriteString("or Write, not a shell heredoc — the shell is for running things, not for\n")
	b.WriteString("reading or editing files.\n")
	b.WriteString(gitSnapshotSection(c))

	b.WriteString("\n## Already in your context\n\n")
	b.WriteString("`CLAUDE.md` in the working directory — architecture and the verification\n")
	b.WriteString("gate a review fix has to pass exactly as the original change did — is\n")
	b.WriteString("auto-loaded by Claude Code; there is no need to read it again.\n")

	b.WriteString("\n## Then read\n\n")
	b.WriteString("The project's conventions, which is what the rest of the review will\n")
	b.WriteString("be measured against:\n\n")
	fmt.Fprintf(&b, "    nat info --project %s\n\n", c.ProjectID)
	b.WriteString("That is the only `nat` command this session has any business running, and\n")
	b.WriteString("it names the project the way every other one does:\n\n")
	fmt.Fprintf(&b, "    --project %s\n\n", c.ProjectID)
	b.WriteString("A command given no project is refused: there is nothing for it to fall\n")
	b.WriteString("back to, and in particular not the project the user's board is on,\n")
	b.WriteString("which they can switch while you work.\n")

	b.WriteString("\n## Finish\n\n")
	if c.Branch != "" {
		fmt.Fprintf(&b, "Commit in the working directory above and push %s again — the\n", c.Branch)
		b.WriteString("same branch, which is the one the pull request is built from. It picks\n")
		b.WriteString("up what you push by itself, so that is the whole of the ending: nothing\n")
		b.WriteString("to record, no second pull request to open, and no branch of your own to\n")
		b.WriteString("create or switch to.\n\n")
	} else {
		b.WriteString("Commit in the working directory above and push the branch the pull\n")
		b.WriteString("request is built from. It picks up what you push by itself, so that is\n")
		b.WriteString("the whole of the ending: nothing to record, no second pull request to\n")
		b.WriteString("open, and no branch of your own to create or switch to.\n\n")
	}
	b.WriteString("Then say what you changed and what is still outstanding, so the user can\n")
	b.WriteString("read the review's state off your last message.\n")

	b.WriteString("\n## Guardrails\n\n")
	b.WriteString("- One pull request per session. Never pick up another slice when this\n")
	b.WriteString("  one is done.\n")
	b.WriteString("- Never change the slice on the tracker: it is done, and its record of\n")
	b.WriteString("  what happened is not yours to rewrite.\n")
	b.WriteString("- `gh pr view` and `gh pr checks` are the only `gh` you may run.\n")
	b.WriteString("- Never open, merge, close or reopen a pull request, and never push to\n")
	b.WriteString("  the main branch.\n")

	return b.String()
}
