package cli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/worktree"
)

// removal is one worktree a command asked to take away: the repository and
// the branch.
type removal = struct{ dir, branch string }

// withWorktree gives env a worktree for every branch it is asked about, and
// answers with the fake so a test can read back what was removed.
func withWorktree(env *Env) *fakeSessionWorktrees {
	w := &fakeSessionWorktrees{existingPath: "/tmp/nat.worktrees/x"}
	env.NewWorktrees = func() actions.Worktrees { return w }
	return w
}

// refusedRemoval is git refusing a dirty worktree.
var refusedRemoval = &worktree.ExitError{Code: 128, Stderr: "fatal: contains modified or untracked files\n"}

// A merge takes the slice's worktree away: by its AgentBranch, in the
// repository WorkdirFor names.
func TestPRMergeRemovesTheSlicesWorktree(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	env.NewGH = func() GH { return gh.NewWithRunner(&multiRunner{viewOut: readyToMergePRJSON}) }
	w := withWorktree(&env)

	if err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-merge: %v", err)
	}
	if want := []removal{{"/tmp/nat", "slice/write-the-ui"}}; !reflect.DeepEqual(w.removed, want) {
		t.Errorf("removed = %+v, want %+v", w.removed, want)
	}
}

// A removal git refuses leaves the merge's exit status and output as they
// were: the merge has happened whatever became of the checkout.
func TestPRMergeSucceedsThoughTheRemovalIsRefused(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, out := testEnv(testConfig(t), api)
	env.NewGH = func() GH { return gh.NewWithRunner(&multiRunner{viewOut: readyToMergePRJSON}) }
	w := withWorktree(&env)
	w.removeErr = refusedRemoval

	if err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1", "--json"}, env); err != nil {
		t.Fatalf("pr-merge: %v", err)
	}
	if len(w.removed) != 1 {
		t.Errorf("removed = %+v, want the one attempt", w.removed)
	}
	if got := strings.TrimSpace(out.String()); got != "{\n  \"merged\": true\n}" {
		t.Errorf("output = %q, want the merge's own JSON", got)
	}
}

// A merge pr-status settles takes the slice's worktree with it.
func TestPRStatusRemovesASettledSlicesWorktree(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageForStatus("s1", "Merged on GitHub", notion.SliceInProgress, "",
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	env.NewGH = func() GH {
		return &fakePRReader{
			open: map[string]map[string]gh.PRStatus{"/tmp/nat": {}},
			view: map[string]gh.PR{"https://github.test/craig/nat/pull/7": {State: gh.PRStateMerged}},
		}
	}
	w := withWorktree(&env)

	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if want := []removal{{"/tmp/nat", "slice/merged-on-github"}}; !reflect.DeepEqual(w.removed, want) {
		t.Errorf("removed = %+v, want %+v", w.removed, want)
	}
}

// A pull request closed unmerged never makes its slice Done, and so removes
// nothing.
func TestPRStatusKeepsAClosedPRsWorktree(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageForStatus("s1", "Closed on GitHub", notion.SliceInProgress, "",
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	env.NewGH = func() GH {
		return &fakePRReader{
			open: map[string]map[string]gh.PRStatus{"/tmp/nat": {}},
			view: map[string]gh.PR{"https://github.test/craig/nat/pull/7": {State: gh.PRStateClosed}},
		}
	}
	w := withWorktree(&env)
	w.branches = map[string][]string{"/tmp/nat": {"slice/closed-on-github"}}

	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if len(w.removed) != 0 {
		t.Errorf("removed = %+v, want nothing", w.removed)
	}
}

// The sweep removes the worktree of every Done slice whose pull request is
// merged or absent, and leaves everything else: a Done slice whose pull
// request reads open, one whose repository could not be listed, slices in
// progress or still to do, a slice with a live agent, and a worktree no slice
// owns. Each repository is listed once.
func TestPRStatusSweepsLandedWorktrees(t *testing.T) {
	const pr = "https://github.test/craig/nat/pull/"
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {
				slicePageForStatus("merged", "Merged", notion.SliceDone, "", pr+"1"),
				slicePageForStatus("nopr", "No PR", notion.SliceDone, "", ""),
				slicePageForStatus("open", "Still open", notion.SliceDone, "", pr+"2"),
				slicePageWithAllFields("unread", "Unread", notion.SliceDone, "", "", "", pr+"3", "/repo/unread"),
				slicePageForStatus("working", "Working", notion.SliceInProgress, "", pr+"4"),
				slicePageForStatus("todo", "Todo", notion.SliceTodo, "", ""),
				slicePageForStatus("live", "Live", notion.SliceDone, "", ""),
			},
		},
	}
	env, out := testEnv(testConfig(t), api)
	env.NewGH = func() GH {
		return &fakePRReader{
			open: map[string]map[string]gh.PRStatus{"/tmp/nat": {
				pr + "2": {Approved: true, Mergeable: true},
				pr + "4": {Approved: true, Mergeable: true},
			}},
			err: map[string]error{"/repo/unread": errors.New("gh is down")},
		}
	}
	env.NewTmux = func() *agent.Tmux {
		return agent.NewTmuxWithRunner(&agentTestRunner{liveSessions: map[string]string{"live": "nat-live"}})
	}
	w := withWorktree(&env)
	w.branches = map[string][]string{
		"/tmp/nat": {"slice/merged", "slice/no-pr", "slice/still-open", "slice/working", "slice/todo",
			"slice/live", "slice/stranger"},
		"/repo/unread": {"slice/unread"},
	}

	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	want := []removal{{"/tmp/nat", "slice/merged"}, {"/tmp/nat", "slice/no-pr"}}
	if !reflect.DeepEqual(w.removed, want) {
		t.Errorf("removed = %+v, want %+v", w.removed, want)
	}
	if want := []string{"/tmp/nat"}; !reflect.DeepEqual(w.listed, want) {
		t.Errorf("listed = %v, want %v", w.listed, want)
	}
	if !strings.Contains(out.String(), "Merged — unread — "+pr+"1") {
		t.Errorf("output = %q, want the reading as before", out.String())
	}
}

// A sweep whose removal git refuses leaves the reading's output and exit
// status alone.
func TestPRStatusSweepSurvivesARefusedRemoval(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageForStatus("nopr", "No PR", notion.SliceDone, "", "")},
		},
	}
	env, out := testEnv(testConfig(t), api)
	env.NewGH = func() GH { return &fakePRReader{} }
	env.NewTmux = func() *agent.Tmux {
		return agent.NewTmuxWithRunner(&agentTestRunner{liveSessions: map[string]string{}})
	}
	w := withWorktree(&env)
	w.branches = map[string][]string{"/tmp/nat": {"slice/no-pr"}}
	w.removeErr = refusedRemoval

	if err := Run(context.Background(), []string{"pr-status", "--project", "project-1", "--json"}, env); err != nil {
		t.Fatalf("pr-status: %v", err)
	}
	if len(w.removed) != 1 {
		t.Errorf("removed = %+v, want the one attempt", w.removed)
	}
	if got := strings.TrimSpace(out.String()); got != "{\n  \"slices\": []\n}" {
		t.Errorf("output = %q, want the reading as before", got)
	}
}

// complete-slice closing a slice Done — no branch, no pull request — removes
// its worktree: no merge is coming to.
func TestCompleteSliceDoneRemovesTheWorktree(t *testing.T) {
	api := completableAPI()
	env, _ := completeEnv(t, api)
	w := withWorktree(&env)

	err := Run(context.Background(), []string{
		"complete-slice", sliceID, "--summary", "Wrote the docs.", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("complete-slice: %v", err)
	}
	if len(w.removed) != 1 || w.removed[0].dir != "/tmp/nat" ||
		!strings.HasPrefix(w.removed[0].branch, "slice/") {
		t.Errorf("removed = %+v, want the slice's worktree in /tmp/nat", w.removed)
	}
}

// A branch handed back is work still to review: its worktree stays.
func TestCompleteSliceHandBackKeepsTheWorktree(t *testing.T) {
	api := completableAPI()
	env, _ := completeEnv(t, api)
	w := withWorktree(&env)

	err := Run(context.Background(), []string{
		"complete-slice", sliceID, "--branch", "slice/render-the-board",
		"--summary", "Wrote the renderer.", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("complete-slice: %v", err)
	}
	if len(w.removed) != 0 {
		t.Errorf("removed = %+v, want nothing", w.removed)
	}
}

// A trashed slice's worktree goes with it.
func TestSliceDeleteRemovesTheWorktree(t *testing.T) {
	api := deletableAPI(notion.SliceDone)
	env, _ := testEnv(testConfig(t), api)
	w := withWorktree(&env)

	if err := Run(context.Background(), []string{"slice-delete", testSliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-delete: %v", err)
	}
	if want := []removal{{"/tmp/nat", "slice/render-the-board"}}; !reflect.DeepEqual(w.removed, want) {
		t.Errorf("removed = %+v, want %+v", w.removed, want)
	}
}

// A refused removal leaves the delete's output as it was.
func TestSliceDeleteSucceedsThoughTheRemovalIsRefused(t *testing.T) {
	api := deletableAPI(notion.SliceTodo)
	env, out := testEnv(testConfig(t), api)
	w := withWorktree(&env)
	w.removeErr = refusedRemoval

	if err := Run(context.Background(), []string{"slice-delete", testSliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("slice-delete: %v", err)
	}
	if len(w.removed) != 1 {
		t.Errorf("removed = %+v, want the one attempt", w.removed)
	}
	if !strings.Contains(out.String(), "Notion's trash") {
		t.Errorf("output = %q, want the delete's own", out.String())
	}
}
