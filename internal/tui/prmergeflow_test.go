package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/gh"
)

// mergeCall is one pull request the screen asked gh to merge.
type mergeCall struct{ dir, ref string }

// fakePRMerger stands in for the GitHub CLI: it records what it was asked to
// merge and answers with the refusal — or the silence — the test wants gh to
// have given.
type fakePRMerger struct {
	err  error
	made []mergeCall
}

var _ PRMerger = (*fakePRMerger)(nil)

func (f *fakePRMerger) MergePR(dir, ref string) error {
	f.made = append(f.made, mergeCall{dir, ref})
	return f.err
}

// mergeablePR is a pull request whose merge box says yes on every line: it is
// approved, its checks are green and it sits cleanly on its base.
func mergeablePR() gh.PR {
	pr := samplePR()
	pr.ReviewDecision = reviewApproved
	pr.Mergeable = mergeableYes
	pr.MergeStateStatus = "CLEAN"
	pr.Checks = []gh.Check{{Name: "build", State: "SUCCESS"}}
	return pr
}

// mergeApp returns an app with the pull request screen already up on a slice's
// pull request, gh's answer being pr, and a fake merger wired in.
func mergeApp(t *testing.T, pr gh.PR) (*App, *fakePRMerger, *fakePRViewer, string) {
	t.Helper()
	app, viewer, workdir := prViewApp(t)
	viewer.pr = pr
	merger := &fakePRMerger{}
	app.prMerger = merger
	cursorOn(t, app, withPR)
	app.Update(first[prViewLoadedMsg](t, run(press(app, "V"))))
	if app.screen != screenPR {
		t.Fatalf("screen = %v, want the pull request screen", app.screen)
	}
	return app, merger, viewer, workdir
}

// TestMergeKeyAsksBeforeMerging is the whole of m on a pull request the merge
// box says can go in: the question is asked on the merge box, nothing has been
// merged yet, and answering it runs gh in the slice's repository on the very
// ref the screen was opened with.
func TestMergeKeyAsksBeforeMerging(t *testing.T) {
	app, merger, _, workdir := mergeApp(t, mergeablePR())

	press(app, "m")
	if !app.prview.Prompting() {
		t.Fatal("m did not ask before merging")
	}
	if len(merger.made) != 0 {
		t.Fatalf("gh was asked to merge %+v before the question was answered", merger.made)
	}
	if body := app.body(); !strings.Contains(body, "merge") || !strings.Contains(body, "cancel") {
		t.Errorf("body = %q, want the merge question drawn on the merge box", body)
	}

	cmd := press(app, "enter")
	if app.prview.Prompting() {
		t.Error("the question is still up after it was answered")
	}
	if !app.busy {
		t.Error("the merge is in flight, so the app should be busy")
	}
	msg := first[prMergedMsg](t, run(cmd))
	want := mergeCall{workdir, "https://github.test/craig/nat/pull/12"}
	if len(merger.made) != 1 || merger.made[0] != want {
		t.Fatalf("gh was asked to merge %+v, want %+v", merger.made, want)
	}
	if msg.err != nil || msg.number != 12 {
		t.Errorf("msg = %+v, want the merge of #12 to have happened", msg)
	}
}

// A merge that succeeded reads the pull request again, so the screen says
// merged rather than going on offering the key that merged it. Nothing is
// written to Notion: the slice was Done as its pull request was opened.
func TestMergeRereadsThePullRequest(t *testing.T) {
	app, _, viewer, _ := mergeApp(t, mergeablePR())
	client := app.client.(*fakeNotion)

	press(app, "m")
	msgs := run(press(app, "enter"))
	merged := mergeablePR()
	merged.State = gh.PRStateMerged
	viewer.pr = merged

	_, cmd := app.Update(first[prMergedMsg](t, msgs))
	if app.busy {
		t.Error("the app is still busy after the merge landed")
	}
	if !strings.Contains(app.toast, "Merged #12") {
		t.Errorf("toast = %q, want the merge reported", app.toast)
	}
	if app.toastSev != sevSuccess {
		t.Errorf("toast severity = %v, want a success", app.toastSev)
	}
	app.Update(first[prViewLoadedMsg](t, run(cmd)))
	if len(viewer.made) != 2 {
		t.Fatalf("gh was asked for %d readings, want the pull request read again", len(viewer.made))
	}
	if body := app.body(); !strings.Contains(body, "merged into main") {
		t.Errorf("body = %q, want the merge box replaced by the ending", body)
	}
	if len(client.updated) != 0 {
		t.Errorf("wrote %v to Notion, want nothing — the slice was Done already", client.updated)
	}
}

// gh refusing anyway — branch protection, a review dismissed by a push, a check
// that went red between the reading and the key — is a toast carrying its first
// line, not an error banner: the pull request is still there and still open.
func TestMergeReportsAGhRefusal(t *testing.T) {
	app, merger, viewer, _ := mergeApp(t, mergeablePR())
	merger.err = &gh.ExitError{Code: 1, Stderr: "Pull request is not mergeable: the base branch policy prohibits the merge.\n"}

	press(app, "m")
	app.Update(first[prMergedMsg](t, run(press(app, "enter"))))

	if !strings.Contains(app.toast, "base branch policy") {
		t.Errorf("toast = %q, want gh's own reason", app.toast)
	}
	if app.toastSev != sevError {
		t.Errorf("toast severity = %v, want an error", app.toastSev)
	}
	if app.err != nil {
		t.Errorf("err = %v, want gh's refusal left off the error banner", app.err)
	}
	if app.busy {
		t.Error("the app is still busy after gh refused")
	}
	if len(viewer.made) != 1 {
		t.Errorf("gh was asked for %d readings, want nothing read again after a refusal", len(viewer.made))
	}
}

// The question can be backed out of, both ways it is offered: esc, and the
// cancel beside the merge. Neither merges anything and neither says anything —
// nothing was in flight, and the pull request is as it was.
func TestMergeCanBeBackedOutOf(t *testing.T) {
	for _, tt := range []struct {
		name string
		back func(a *App)
	}{
		{"esc", func(a *App) { press(a, "esc") }},
		{"the cancel choice", func(a *App) {
			press(a, "right")
			press(a, "enter")
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, merger, _, _ := mergeApp(t, mergeablePR())

			press(app, "m")
			tt.back(app)

			if app.prview.Prompting() {
				t.Error("the question is still up after it was backed out of")
			}
			if len(merger.made) != 0 {
				t.Errorf("gh was asked to merge %+v after the question was backed out of", merger.made)
			}
			if app.busy {
				t.Error("the app is busy after a merge that never started")
			}
			if app.screen != screenPR {
				t.Errorf("screen = %v, want the pull request still on show", app.screen)
			}
			if app.toast != "" {
				t.Errorf("toast = %q, want nothing said about a merge that never started", app.toast)
			}
		})
	}
}

// The refusals are the merge box's own verdicts, in its own words: the key says
// no exactly where the box does, and names the line that says so.
func TestMergeIsRefusedByTheMergeBox(t *testing.T) {
	conflicting := mergeablePR()
	conflicting.Mergeable = mergeConflicting
	changes := mergeablePR()
	changes.ReviewDecision = reviewChangesRequested
	failing := mergeablePR()
	failing.Checks = []gh.Check{{Name: "build", State: "FAILURE"}}

	for _, tt := range []struct {
		name string
		pr   gh.PR
		want string
	}{
		{"changes requested", changes, "review: changes requested"},
		{"failing checks", failing, "checks: 1 failing"},
		{"a conflicting branch", conflicting, "mergeable: conflicting with main"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, merger, _, _ := mergeApp(t, tt.pr)

			press(app, "m")

			if app.prview.Prompting() {
				t.Fatal("the merge was offered on a pull request the merge box refuses")
			}
			if len(merger.made) != 0 {
				t.Fatalf("gh was asked to merge %+v", merger.made)
			}
			if !strings.Contains(app.toast, tt.want) {
				t.Errorf("toast = %q, want it to name %q", app.toast, tt.want)
			}
			if app.toastSev != sevWarning {
				t.Errorf("toast severity = %v, want a refusal", app.toastSev)
			}
		})
	}
}

// A verdict still to come is not a no: GitHub is the one to say whether it will
// take the merge anyway, so the key asks rather than refusing.
func TestMergeIsOfferedWhileAVerdictIsStillToCome(t *testing.T) {
	pr := mergeablePR()
	pr.ReviewDecision = reviewRequired
	app, _, _, _ := mergeApp(t, pr)

	press(app, "m")

	if !app.prview.Prompting() {
		t.Errorf("the merge was refused over a verdict still to come; toast = %q", app.toast)
	}
}

// A pull request that has already merged, or was closed without merging, has
// nothing left for the key to do — and neither has a screen with no reading on
// it at all.
func TestMergeIsRefusedWithNothingToMerge(t *testing.T) {
	merged := mergeablePR()
	merged.State = gh.PRStateMerged
	closed := mergeablePR()
	closed.State = gh.PRStateClosed

	for _, tt := range []struct {
		name string
		pr   gh.PR
	}{{"merged", merged}, {"closed", closed}} {
		t.Run(tt.name, func(t *testing.T) {
			app, merger, _, _ := mergeApp(t, tt.pr)

			press(app, "m")

			if app.prview.Prompting() {
				t.Fatal("the merge was offered on a pull request that is no longer open")
			}
			if len(merger.made) != 0 {
				t.Fatalf("gh was asked to merge %+v", merger.made)
			}
			if !strings.Contains(app.toast, "no open pull request") {
				t.Errorf("toast = %q, want the refusal to say there is nothing to merge", app.toast)
			}
		})
	}
}

// A read still in flight has no pull request to merge either, which is the same
// refusal for the same reason.
func TestMergeIsRefusedWhileTheReadIsInFlight(t *testing.T) {
	app, viewer, _ := prViewApp(t)
	merger := &fakePRMerger{}
	app.prMerger = merger
	cursorOn(t, app, withPR)
	read := press(app, "V")
	if !app.prview.Busy() {
		t.Fatal("the read should be in flight")
	}

	press(app, "m")

	if app.prview.Prompting() || len(merger.made) != 0 {
		t.Errorf("the merge was offered before there was a pull request; gh got %+v", merger.made)
	}
	if run(read); len(viewer.made) != 1 {
		t.Errorf("gh was asked for %d readings, want just the one", len(viewer.made))
	}
}

// A build with no gh to merge with, and an app already busy with something
// else, both do nothing at all rather than reporting anything: neither is a
// refusal the user did something to earn.
func TestMergeDoesNothingWithoutAMergerOrWhileBusy(t *testing.T) {
	for _, tt := range []struct {
		name string
		set  func(a *App)
	}{
		{"no gh", func(a *App) { a.prMerger = nil }},
		{"already busy", func(a *App) { a.busy = true }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app, _, _, _ := mergeApp(t, mergeablePR())
			tt.set(app)

			if cmd := press(app, "m"); cmd != nil {
				t.Errorf("m returned %v, want nothing done", run(cmd))
			}
			if app.prview.Prompting() {
				t.Error("the merge was offered with nothing to merge it with")
			}
			if app.toast != "" {
				t.Errorf("toast = %q, want nothing said", app.toast)
			}
		})
	}
}

// The hints row names the merge while there is one to attempt, and says nothing
// about it once the pull request has landed.
func TestPRHintsNameTheMerge(t *testing.T) {
	app, _, _, _ := mergeApp(t, mergeablePR())
	if hints := app.contextHints(); len(hints) == 0 || hints[0].binding.Help().Key != "m" {
		t.Errorf("hints = %v, want the merge named first", hints)
	}

	merged := mergeablePR()
	merged.State = gh.PRStateMerged
	app.prview.SetPR(merged)
	for _, h := range app.contextHints() {
		if h.binding.Help().Key == "m" {
			t.Error("the merge is still offered on a pull request that has merged")
		}
	}

	// While the question is up, what answers it is all the row names.
	app.prview.SetPR(mergeablePR())
	press(app, "m")
	if hints := app.contextHints(); len(hints) != len(app.promptKeys.promptHints()) {
		t.Errorf("hints = %v, want the prompt's own", hints)
	}
}

// The help screen lists the key, which is where a key out of the hints row is
// found.
func TestHelpListsTheMergeKey(t *testing.T) {
	app := NewApp(testConfig(), &fakeNotion{})
	if body := app.helpBody(); !strings.Contains(body, "merge") {
		t.Errorf("help = %q, want the merge key listed", body)
	}
}

// The question steps between its choices and stops at either end, exactly as
// the board's own prompt does.
func TestPRPromptMovesBetweenItsChoices(t *testing.T) {
	app, _, _, _ := mergeApp(t, mergeablePR())
	press(app, "m")

	press(app, "left")
	if got := app.prview.PromptChoice(); got != choiceMerge {
		t.Errorf("choice = %d, want it held at the first", got)
	}
	press(app, "right")
	press(app, "right")
	if got := app.prview.PromptChoice(); got != choiceCancelMerge {
		t.Errorf("choice = %d, want it held at the last", got)
	}
}

// A prompt asked about a pull request survives a plan landing under it: it is a
// question about a pull request rather than about a row that may have moved.
func TestPRPromptSurvivesAPlanLanding(t *testing.T) {
	app, _, _, _ := mergeApp(t, mergeablePR())
	press(app, "m")

	app.Update(projectLoadedMsg{project: *app.project})

	if !app.prview.Prompting() {
		t.Error("the merge question was taken down by a plan landing under it")
	}
}

// Nothing is asked of a screen that has never been pointed at a pull request:
// the merge key on an idle screen has no target at all.
func TestPRViewMergeableIsEmptyUntilOneIsRead(t *testing.T) {
	view := NewPRView(DefaultStyles())
	if _, ok := view.Mergeable(); ok {
		t.Error("an empty screen offered a pull request to merge")
	}
	view.Fail(errors.New("no pull requests found for branch"))
	if _, ok := view.Mergeable(); ok {
		t.Error("a failed read offered a pull request to merge")
	}
	if view.Prompting() || view.PromptChoice() != 0 {
		t.Error("an empty screen has a prompt on it")
	}
	view.MovePrompt(1) // nothing to move, and nothing to panic over
}
