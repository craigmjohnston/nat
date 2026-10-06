package agent

import (
	"regexp"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
)

// publishedContext is a relaunch on a slice whose work is already out: handed
// back and approved — in progress, its pull request recorded — then resumed,
// which cleared its branch, and placed back in the worktree its derived branch
// is checked out in.
func publishedContext() PromptContext {
	c := worktreeContext()
	c.Slice.Status = domain.SliceClaimed
	c.Slice.StatusName = "In progress"
	c.Slice.PRURL = "https://github.test/craig/nat/pull/12"
	return c
}

func TestPromptOnAPublishedSlice(t *testing.T) {
	golden(t, "prompt-published", Prompt(publishedContext()))
}

func TestPromptOnAPublishedSliceOnGnat(t *testing.T) {
	c := publishedContext()
	c.Frontend = FrontendGnat
	golden(t, "prompt-published-gnat", Prompt(c))
}

// A relaunch on a slice with a pull request recorded is an ordinary relaunch
// — told it is continuing, claimed like any other — and told besides that the
// pull request is open and a push updates it, never to open another.
func TestPromptTellsAPublishedSliceItsPullRequestIsOpen(t *testing.T) {
	c := publishedContext()
	got := Prompt(c)
	for _, want := range []string{
		"There is work on that branch already",
		"## The pull request",
		"its pull request is\nopen: " + c.Slice.PRURL + ". Pushing the branch updates it.",
		"Its pull request is open already, and\npushing the branch updates it",
		"    gh pr view " + c.Slice.PRURL + " --comments\n",
		"That is the only `gh` you may run",
		"Never open, merge, close or reopen a pull request",
		"--branch " + c.Branch + " --summary",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not say %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Do not run `gh`") {
		t.Error("prompt both permits the review read and bans every gh")
	}
	// The one gh read is the review's; CI is read with slice-checks, so no
	// template names a gh read of the checks.
	if regexp.MustCompile(`\bgh pr checks\b`).MatchString(got) {
		t.Error("prompt names a gh read of the checks")
	}
	if strings.Contains(strings.ToLower(got), "fix session") {
		t.Error("prompt mentions a fix session")
	}
}

// The merge sentence names the surface the user merges from, and says
// nothing about one where the launch did not say.
func TestPullRequestPassageNamesTheFrontend(t *testing.T) {
	c := publishedContext()
	if got := pullRequestPassage(c); strings.Contains(got, "app's PR tab") {
		t.Errorf("an unspecified launch names the app:\n%s", got)
	}
	c.Frontend = FrontendGnat
	if got := pullRequestPassage(c); !strings.Contains(got, "a button in the app's PR tab") {
		t.Errorf("a gnat launch does not name the PR tab:\n%s", got)
	}
}

// The review as it stood at launch is carried inline, each half on its own;
// with nothing gathered the section says only how to read it.
func TestPullRequestPassageCarriesTheGatheredReview(t *testing.T) {
	c := publishedContext()
	c.ReviewComments, c.ReviewChecks = "craig: nit on naming", "X build 1m"
	got := pullRequestPassage(c)
	for _, want := range []string{"Captured at launch", "craig: nit on naming", "The pull request's checks:\n\n```\nX build 1m\n```"} {
		if !strings.Contains(got, want) {
			t.Errorf("passage does not carry %q:\n%s", want, got)
		}
	}
	c.ReviewChecks = ""
	if got := pullRequestPassage(c); strings.Contains(got, "The pull request's checks") || !strings.Contains(got, "craig: nit on naming") {
		t.Errorf("comments alone:\n%s", got)
	}
	c.ReviewComments, c.ReviewChecks = "", "X build 1m"
	if got := pullRequestPassage(c); strings.Contains(got, "`gh pr view "+c.Slice.PRURL+" --comments`:") || !strings.Contains(got, "X build 1m") {
		t.Errorf("checks alone:\n%s", got)
	}
	c.ReviewChecks = ""
	if got := pullRequestPassage(c); strings.Contains(got, "Captured at launch") {
		t.Errorf("nothing gathered, yet a snapshot section:\n%s", got)
	}
	if got := pullRequestPassage(testContext()); got != "" {
		t.Errorf("no pull request recorded, yet a passage:\n%s", got)
	}
}

// A slice with a pull request is resumed whatever branch it records — resumed
// work has its branch cleared — so long as the session has a worktree.
func TestPromptResumesAPublishedSliceWithItsBranchCleared(t *testing.T) {
	if !Resuming(publishedContext()) {
		t.Error("a published slice placed in its worktree should resume")
	}
	c := publishedContext()
	c.Branch = ""
	if Resuming(c) {
		t.Error("a launch with no worktree has no branch to resume on")
	}
}

// Every slice prompt, whatever launched it, tells the agent to put a request
// for more after its hand-back on the record before changing anything, and
// what a Done refusal means.
func TestEverySlicePromptCarriesTheResumePassage(t *testing.T) {
	for name, c := range map[string]PromptContext{
		"slice":          testContext(),
		"slice worktree": worktreeContext(),
		"slice gnat":     gnatContext(),
		"slice resume":   resumeContext(),
		"slice no repo":  repoUnknownContext(),
		"published":      publishedContext(),
	} {
		got := Prompt(c)
		for _, want := range []string{
			"If the user asks for more or different work after you have handed back",
			"    nat slice-resume " + c.Slice.ID + " --project " + testProjectID + " --note '<what they asked for>'\n",
			"same\n`complete-slice --branch` command",
			"Done, the work is merged: say so to the user and stop.",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("the %s prompt does not say %q", name, want)
			}
		}
	}
}

// conflictedContext is a relaunch on a hand-back with no pull request whose
// branch the launch found conflicting with origin/main.
func conflictedContext() PromptContext {
	c := worktreeContext()
	c.Slice.Status = domain.SliceClaimed
	c.Slice.Branch = c.Branch
	c.ConflictBase = "origin/main"
	return c
}

func TestPromptOnAConflictedHandBack(t *testing.T) {
	golden(t, "prompt-conflicted", Prompt(conflictedContext()))
}

// A conflicted hand-back is told to rebase onto the base it conflicts with,
// resolve, push the rewritten branch with a lease, and hand back — on top of
// the ordinary relaunch, which still says it is continuing.
func TestPromptTasksAConflictedHandBackWithARebase(t *testing.T) {
	c := conflictedContext()
	got := Prompt(c)
	for _, want := range []string{
		"There is work on that branch already",
		"## The branch conflicts with origin/main",
		"on " + c.Branch + ":\n   `git rebase origin/main`",
		"Resolve every conflict",
		"`git push --force-with-lease origin " + c.Branch + "`",
		"--branch " + c.Branch + " --summary",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not say %q:\n%s", want, got)
		}
	}
}

// No conflict found, no passage: an ordinary relaunch says nothing of one.
func TestPromptSaysNothingOfAConflictWithoutOne(t *testing.T) {
	c := conflictedContext()
	c.ConflictBase = ""
	if got := Prompt(c); strings.Contains(got, "conflicts with") || strings.Contains(got, "git rebase") {
		t.Errorf("prompt speaks of a conflict nobody found:\n%s", got)
	}
}

// takenBackContext is a relaunch on a review sent back before any pull
// request: placed on its branch, the Branch cleared, no PR, a hand-back on
// its task log.
func takenBackContext() PromptContext {
	c := worktreeContext()
	c.Slice.Status = domain.SliceClaimed
	c.HandedBack = true
	return c
}

func TestPromptOnATakenBackReview(t *testing.T) {
	golden(t, "prompt-taken-back", Prompt(takenBackContext()))
}

// A review sent back before any pull request is continuing work, told so as
// any other relaunch is; the same placement with no hand-back is not.
func TestPromptTellsATakenBackReviewItIsContinuing(t *testing.T) {
	c := takenBackContext()
	got := Prompt(c)
	for _, want := range []string{
		"There is work on that branch already",
		"and the work an earlier\nsession pushed is already on it",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not say %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "has already loaded git status") {
		t.Error("prompt tells a resuming session its git status is already loaded")
	}
	c.HandedBack = false
	if Resuming(c) {
		t.Error("a placed branch with no record, no PR and no hand-back reads as resuming")
	}
}
