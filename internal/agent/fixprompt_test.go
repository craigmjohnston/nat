package agent

import (
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
)

// fixContext is a launch on a slice whose work is already out: approved — in
// progress, with the pull request it produced recorded on it — and the
// worktree its branch is checked out in still there.
func fixContext() PromptContext {
	c := worktreeContext()
	c.Slice.Status = domain.SliceClaimed
	c.Slice.Branch = c.Branch
	c.Slice.PRURL = "https://github.test/craig/nat/pull/12"
	c.Fix = true
	return c
}

func TestFixPrompt(t *testing.T) {
	golden(t, "fix-prompt", Prompt(fixContext()))
}

func TestFixPromptOnTUI(t *testing.T) {
	c := fixContext()
	c.Frontend = FrontendTUI
	golden(t, "fix-prompt-tui", Prompt(c))
}

func TestFixPromptOnGnat(t *testing.T) {
	c := fixContext()
	c.Frontend = FrontendGnat
	golden(t, "fix-prompt-gnat", Prompt(c))
}

// The frontend note and the merge sentence are the only two places the fix
// prompt says anything about which surface the user is on; an unspecified
// launch says neither.
func TestFixPromptNamesTheFrontend(t *testing.T) {
	unset := Prompt(fixContext())
	for _, unwanted := range []string{"driving this from", "button in the app's PR"} {
		if strings.Contains(unset, unwanted) {
			t.Errorf("an unspecified launch's prompt says %q:\n%s", unwanted, unset)
		}
	}

	c := fixContext()
	c.Frontend = FrontendTUI
	tui := Prompt(c)
	if want := "The user is driving this from the TUI board.\n\n"; !strings.Contains(tui, want) {
		t.Errorf("tui prompt does not say %q:\n%s", want, tui)
	}
	if want := "merging this one is a key on the user's board"; !strings.Contains(tui, want) {
		t.Errorf("tui prompt does not say %q:\n%s", want, tui)
	}

	c = fixContext()
	c.Frontend = FrontendGnat
	gnat := Prompt(c)
	if want := "The user is driving this from gnat, the macOS app.\n\n"; !strings.Contains(gnat, want) {
		t.Errorf("gnat prompt does not say %q:\n%s", want, gnat)
	}
	if want := "merging this one is a button in the app's PR\ntab"; !strings.Contains(gnat, want) {
		t.Errorf("gnat prompt does not say %q:\n%s", want, gnat)
	}
	if strings.Contains(gnat, "a key on the user's board") {
		t.Errorf("gnat prompt still carries the TUI's board wording:\n%s", gnat)
	}
}

// The pull request is the whole brief, and it moves while the session runs, so
// the agent is told to read it again before it pushes: the comments with the
// one `gh` read the standing prohibition is relaxed for, the checks with
// `nat slice-checks` — never `gh pr checks` — and the prompt says what is still
// out of bounds in the same breath.
func TestFixPromptSendsTheAgentAtTheReview(t *testing.T) {
	c := fixContext()
	got := Prompt(c)
	for _, want := range []string{
		"- Pull request: " + c.Slice.PRURL,
		"gh pr view " + c.Slice.PRURL + " --comments",
		"nat slice-checks " + c.Slice.ID + " --log --project " + c.ProjectID,
		"shows what a check still running is doing",
		"nat slice-checks-rerun " + c.Slice.ID + " --check '<check name>' --project " + c.ProjectID,
		"answer the\ncomments left on the review, and fix whatever checks are failing",
		"That `gh pr view` is the only `gh` you may run.",
		"Never open, merge, close\nor reopen a pull request",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not say %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "gh pr checks") {
		t.Errorf("prompt names gh pr checks, which slice-checks replaces:\n%s", got)
	}
}

// A fix launch whose gh gather succeeded carries the review's comments and
// checks inline, framed as captured at launch — and still tells the agent to
// re-check both before it pushes.
func TestFixPromptCarriesTheGatheredReview(t *testing.T) {
	c := fixContext()
	c.ReviewComments = "craig: looks close, one nit on the error message"
	c.ReviewChecks = "X  build  1m3s"
	got := Prompt(c)
	for _, want := range []string{
		"Captured at launch — no need to re-run this",
		"craig: looks close, one nit on the error message",
		"X  build  1m3s",
		"Re-check before you push",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not carry the gathered review — missing %q:\n%s", want, got)
		}
	}
}

// A gh gather that came back empty leaves the section out: the agent is
// still told the two commands, and runs them itself.
func TestFixPromptOmitsTheReviewSnapshotWhenNothingWasGathered(t *testing.T) {
	got := Prompt(fixContext())
	if strings.Contains(got, "Captured at launch — no need to re-run this") {
		t.Errorf("prompt carries a review snapshot with nothing gathered:\n%s", got)
	}
}

// The launch already put the return to work on the record, so there is
// nothing to claim, and nothing else the prompt names would move the slice
// sideways: no start, no release, no blocking it.
func TestFixPromptClaimsNothing(t *testing.T) {
	got := Prompt(fixContext())
	if want := "there is nothing\nto claim"; !strings.Contains(got, want) {
		t.Errorf("prompt does not say %q:\n%s", want, got)
	}
	for _, unwanted := range []string{"start-slice", "release-slice", "--blocked", "recorded as done"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("prompt names %q, which a fix session has no business with", unwanted)
		}
	}
}

// The ending is a push to the same branch — the pull request is built from it
// — then a hand-back naming that branch, the same complete-slice the slice
// itself ended with, with no new pull request description.
func TestFixPromptEndsInAHandBack(t *testing.T) {
	c := fixContext()
	got := Prompt(c)
	for _, want := range []string{
		"- Branch: " + c.Branch + " (the working directory is a worktree already on it)",
		"push " + c.Branch + " again",
		"no second pull request to open",
		"nat complete-slice " + c.Slice.ID + " --project " + c.ProjectID + " \\\n        --branch " + c.Branch + " --summary",
		"Leave `--pr-description` off",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not say %q:\n%s", want, got)
		}
	}
	if i, j := strings.Index(got, "push "+c.Branch), strings.Index(got, "nat complete-slice"); i > j {
		t.Errorf("prompt hands back before it pushes:\n%s", got)
	}
}

// Launched from the app, a fix session hands in what it noticed but did not do
// before its hand-back, as a slice session does; from the board it is told
// nothing of follow-ups, since nothing there can triage them.
func TestFixPromptFollowUps(t *testing.T) {
	c := fixContext()
	c.Frontend = FrontendGnat
	if got := Prompt(c); !strings.Contains(got, "nat slice-followups "+c.Slice.ID+" --project "+c.ProjectID) {
		t.Errorf("gnat fix prompt does not hand follow-ups in:\n%s", got)
	}
	c.Frontend = FrontendTUI
	if got := Prompt(c); strings.Contains(got, "slice-followups") {
		t.Errorf("tui fix prompt names slice-followups:\n%s", got)
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
	for _, want := range []string{"push the branch the pull\nrequest is built from", "--branch <branch>"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt does not say %q:\n%s", want, got)
		}
	}
}

// The optional lines are the ones a slice may not carry: the page's URL, and
// the note that this slice works somewhere other than the project default.
func TestFixPromptWithoutOptionalContext(t *testing.T) {
	c := fixContext()
	c.Slice.URL = ""
	got := Prompt(c)
	if strings.Contains(got, "- Slice URL:") {
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
