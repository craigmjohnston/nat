package agent

import (
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
)

// fixContext is a launch on a slice whose work is already out: Done, with the
// pull request it produced recorded on it and the worktree its branch is
// checked out in still there.
func fixContext() PromptContext {
	c := worktreeContext()
	c.Slice.Status = domain.SliceDone
	c.Slice.Branch = c.Branch
	c.Slice.PRURL = "https://github.test/craig/nat/pull/12"
	c.Fix = true
	return c
}

func TestFixPrompt(t *testing.T) {
	golden(t, "fix-prompt", Prompt(fixContext()))
}

// The pull request is the whole brief, and it moves while the session runs, so
// the agent is told to read it from GitHub rather than handed a copy of what it
// said at launch. Those two reads are the one place the standing prohibition on
// gh is relaxed, and the prompt says what is still out of bounds in the same
// breath.
func TestFixPromptSendsTheAgentAtTheReview(t *testing.T) {
	c := fixContext()
	got := Prompt(c)
	for _, want := range []string{
		"- Pull request: " + c.Slice.PRURL,
		"gh pr view " + c.Slice.PRURL + " --comments",
		"gh pr checks " + c.Slice.PRURL,
		"answer the\ncomments left on the review, and fix whatever checks are failing",
		"Those two reads are the only `gh` you may run.",
		"Never open, merge, close\nor reopen a pull request",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not say %q:\n%s", want, got)
		}
	}
}

// Nothing about the slice is this session's to move: it is Done, the account of
// what was done is written, and the state the approve key left it in is what
// the merge box is read against. The prompt names none of the commands that
// would change it — a command an agent could copy is one it might run — and
// says outright that the record stands.
func TestFixPromptLeavesTheSliceAlone(t *testing.T) {
	got := Prompt(fixContext())
	for _, want := range []string{
		"nothing about the slice for you to claim, complete or record",
		"Never change the slice on the tracker",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not say %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"start-slice", "complete-slice", "release-slice", "--branch", "--blocked"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("prompt names %q, which would move a slice that is done", unwanted)
		}
	}
}

// The ending is a push and nothing else: the pull request is built from the
// branch, so it picks up what the session commits by itself. There is nothing
// to hand back and no second pull request to open.
func TestFixPromptEndsInAPushToTheSameBranch(t *testing.T) {
	c := fixContext()
	got := Prompt(c)
	for _, want := range []string{
		"- Branch: " + c.Branch + " (the working directory is a worktree already on it)",
		"push " + c.Branch + " again",
		"no second pull request to open, and no branch of your own to\ncreate or switch to",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not say %q:\n%s", want, got)
		}
	}
}

// A launch that could not place the agent in a worktree runs in the shared
// checkout, where nothing here knows which branch the pull request is built
// from — so the ending says to push the one it is on rather than naming a
// branch that may not be checked out at all.
func TestFixPromptWithoutAWorktreeNamesNoBranch(t *testing.T) {
	c := fixContext()
	c.Branch, c.Repo = "", ""
	c.WorkingDir = c.Project.WorkingDir
	got := Prompt(c)
	if strings.Contains(got, "- Branch:") {
		t.Errorf("prompt names a branch for a session that has none:\n%s", got)
	}
	if want := "push the branch the pull\nrequest is built from"; !strings.Contains(got, want) {
		t.Errorf("prompt does not say %q:\n%s", want, got)
	}
}

// The optional lines are the ones a slice may not carry: the page's URL, and
// the note that this slice works somewhere other than the project default.
func TestFixPromptWithoutOptionalContext(t *testing.T) {
	c := fixContext()
	c.Slice.URL = ""
	got := Prompt(c)
	if strings.Contains(got, "- Notion URL:") {
		t.Errorf("prompt names a URL the slice does not have:\n%s", got)
	}
}

func TestFixPromptWithRepoOverride(t *testing.T) {
	c := fixContext()
	c.Slice.Repo = "/Users/craig/Projects/other"
	c.Repo = c.Slice.Repo
	got := Prompt(c)
	if want := "(this slice overrides the project default of " + c.Project.WorkingDir + ")"; !strings.Contains(got, want) {
		t.Errorf("prompt does not say %q:\n%s", want, got)
	}
}
