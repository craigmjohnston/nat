package tui

import (
	"reflect"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/worktree"
)

// The slices of the plan below, by what became of the pull request their work
// went out on.
const (
	// landedMerged is a merged slice with its branch recorded, landedDerived a
	// merged one finished before there was a column to record one in, and
	// landedOwnRepo a merged one in a checkout of its own.
	landedMerged  = "lm"
	landedDerived = "ld"
	landedOwnRepo = "lo"
	// landedOpen is Done with its pull request still open, and landedClosed is
	// still in progress with a pull request that was closed rather than merged.
	landedOpen   = "lp"
	landedClosed = "lc"
)

// landedPlan is that plan. Every slice of it has a pull request out, so what
// tells them apart is only what GitHub says about each.
func landedPlan() domain.Project {
	return domain.NewProject(testProjectID, "tracker",
		domain.MilestonesFromOptions([]string{"M1: Review"}, notion.TypeSelect),
		[]domain.Slice{
			{ID: landedMerged, Name: "Landed work", Status: domain.SliceDone, StatusName: "Done",
				MilestoneID: "M1: Review", PRURL: "https://github.test/pr/landed",
				Branch: "slice/landed-work"},
			{ID: landedDerived, Name: "No branch recorded", Status: domain.SliceDone, StatusName: "Done",
				MilestoneID: "M1: Review", PRURL: "https://github.test/pr/derived"},
			{ID: landedOwnRepo, Name: "Own repo", Status: domain.SliceDone, StatusName: "Done",
				MilestoneID: "M1: Review", PRURL: "https://github.test/pr/own",
				Branch: "slice/own-repo", Repo: otherRepo},
			{ID: landedOpen, Name: "Still open", Status: domain.SliceDone, StatusName: "Done",
				MilestoneID: "M1: Review", PRURL: "https://github.test/pr/open",
				Branch: "slice/still-open"},
			{ID: landedClosed, Name: "Went round again", Status: domain.SliceClaimed,
				StatusName: "In progress", MilestoneID: "M1: Review",
				PRURL: "https://github.test/pr/closed", Branch: "slice/went-round-again"},
		})
}

// landedApp is an app showing that plan with a fake gh and a fake git behind
// it. GitHub has exactly one of the plan's pull requests open, so every other
// one reads as settled, and git has a worktree for every branch until a test
// says otherwise.
func landedApp(t *testing.T) (*App, *fakePRReader, *fakeWorktrees) {
	t.Helper()
	cfg := testConfig()
	project := cfg.Projects[testProjectID]
	project.WorkingDir = natRepo
	cfg.Projects[testProjectID] = project

	app := NewApp(cfg, &fakeNotion{})
	reader := &fakePRReader{open: map[string]map[string]gh.PRStatus{
		natRepo: {"https://github.test/pr/open": {Mergeable: true}},
	}}
	app.prReader = reader

	trees := approveWorktrees(t)
	trees.existing = map[string]string{
		"slice/landed-work":        "/worktrees/landed-work",
		"slice/no-branch-recorded": "/worktrees/no-branch-recorded",
		"slice/own-repo":           "/worktrees/own-repo",
		"slice/still-open":         "/worktrees/still-open",
		"slice/went-round-again":   "/worktrees/went-round-again",
	}
	return app, reader, trees
}

// TestAMergedPullRequestTakesItsWorktree is the whole rule: the removal rides
// the reading that finds a Done slice's pull request no longer open — the same
// edge that drops the slice from the Active panel — and it is taken off the
// checkout the slice belongs to, named by the branch the agent handed back on
// or, for a slice finished before there was a column to record one, by the
// branch a launch would have derived.
func TestAMergedPullRequestTakesItsWorktree(t *testing.T) {
	app, _, trees := landedApp(t)
	p := landedPlan()

	_, cmd := app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)

	want := []worktreeCall{
		{dir: natRepo, branch: "slice/landed-work"},
		{dir: natRepo, branch: "slice/no-branch-recorded"},
		{dir: otherRepo, branch: "slice/own-repo"},
	}
	if !reflect.DeepEqual(trees.removes, want) {
		t.Errorf("git was asked to remove %v, want %v", trees.removes, want)
	}
}

// A slice whose pull request is still open keeps its worktree, and so does one
// still in progress whose pull request was closed rather than merged: the work
// is going round again, and the checkout it is going round in is exactly what
// the next session wants.
func TestWorkStillInFlightKeepsItsWorktree(t *testing.T) {
	app, _, trees := landedApp(t)
	p := landedPlan()

	_, cmd := app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)

	for _, call := range trees.removes {
		if call.branch == "slice/still-open" || call.branch == "slice/went-round-again" {
			t.Errorf("git was asked to remove %v, want work still in flight left alone", call)
		}
	}
}

// A worktree goes exactly once. The plan lands again and again — the poll, the
// nudge, the refresh key — and the sweep each landing runs asks git nothing
// about a slice it has already settled.
func TestAWorktreeGoesOnlyOnce(t *testing.T) {
	app, _, trees := landedApp(t)
	p := landedPlan()

	_, cmd := app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)
	was := len(trees.removes)

	_, cmd = app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)

	if len(trees.removes) != was {
		t.Errorf("git was asked to remove %v, want the settled worktrees left out", trees.removes)
	}
}

// A removal git refused is one line in the log and nothing else — the pull
// request is merged and the slice is Done whatever became of the checkout — and
// the sweep the next plan load runs is what tries it again.
func TestARefusedRemovalIsRetriedOnTheNextLoad(t *testing.T) {
	app, _, trees := landedApp(t)
	trees.removeErr = &worktree.ExitError{Code: 1, Stderr: "worktree has uncommitted changes\n"}
	p := landedPlan()

	_, cmd := app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)

	was := len(trees.removes)
	if was == 0 {
		t.Fatal("git was asked to remove nothing at all")
	}
	if app.err != nil || app.toast != "" {
		t.Errorf("err = %v, toast = %q, want the refusal passed over", app.err, app.toast)
	}

	// The dirty worktree has been dealt with outside nat, and the next plan to
	// land tries the removal again.
	trees.removeErr = nil
	_, cmd = app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)

	if len(trees.removes) != 2*was {
		t.Errorf("git was asked to remove %v, want every refusal tried again", trees.removes)
	}

	// And having succeeded, they are done with.
	_, cmd = app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)
	if len(trees.removes) != 2*was {
		t.Errorf("git was asked to remove %v, want nothing left to remove", trees.removes)
	}
}

// A branch git names no worktree for is not a failure to retry: it is a slice
// whose worktree has already gone — or one that never had one — so nothing is
// removed and nothing is asked about it again.
func TestNoWorktreeToRemovePassesQuietly(t *testing.T) {
	app, _, trees := landedApp(t)
	trees.existing = nil
	p := landedPlan()

	_, cmd := app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)
	looks := len(trees.looks)

	if len(trees.removes) != 0 {
		t.Errorf("git was asked to remove %v, want nothing", trees.removes)
	}
	if looks == 0 {
		t.Fatal("git was asked about no branch at all")
	}

	_, cmd = app.Update(projectLoadedMsg{project: p})
	runPRRead(t, app, cmd)
	if len(trees.looks) != looks {
		t.Errorf("git was asked about %v, want the settled branches left out", trees.looks)
	}
}

// There is nothing to sweep before a plan has landed, and nothing to sweep for
// a project local config has no entry for: both are answered without git being
// started at all.
func TestNothingToRemoveStartsNoGit(t *testing.T) {
	app, _, _ := landedApp(t)
	if cmd := app.removeLanded([]string{landedMerged}); cmd != nil {
		t.Error("removeLanded() = a command with no plan loaded")
	}
	if cmd := app.removeLanded(nil); cmd != nil {
		t.Error("removeLanded(nil) = a command")
	}

	p := landedPlan()
	app.project = &p
	app.cfg.ActiveProjectID = "unknown"
	if cmd := app.removeLanded([]string{landedMerged}); cmd != nil {
		t.Error("removeLanded() = a command for a project config does not know")
	}
}
