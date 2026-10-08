package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

const readyToMergePRJSON = `{
  "number": 7,
  "title": "Ready to merge",
  "state": "OPEN",
  "headRefName": "slice/ready",
  "baseRefName": "main",
  "url": "https://github.test/craig/nat/pull/7",
  "reviewDecision": "APPROVED",
  "mergeable": "MERGEABLE",
  "mergeStateStatus": "CLEAN"
}`

const changesRequestedPRJSON = `{
  "number": 7,
  "title": "Needs work",
  "state": "OPEN",
  "headRefName": "slice/needs-work",
  "baseRefName": "main",
  "url": "https://github.test/craig/nat/pull/7",
  "reviewDecision": "CHANGES_REQUESTED",
  "mergeable": "MERGEABLE"
}`

const alreadyMergedPRJSON = `{
  "number": 7,
  "title": "Already merged",
  "state": "MERGED",
  "headRefName": "slice/done",
  "baseRefName": "main",
  "url": "https://github.test/craig/nat/pull/7"
}`

// multiRunner answers the batched reading (api graphql — viewOut as the one
// pull request's node, its fields gh pr view's own names) and MergePR (pr
// merge) with different
// canned responses, the way one gh.CLI answers both calls a merge makes.
type multiRunner struct {
	viewOut   string
	viewErr   error
	mergeErr  error
	mergeDirs []string
	mergeArgs [][]string
	// calls is every gh invocation's first two arguments ("api graphql",
	// "pr merge"), in order.
	calls []string
}

func (r *multiRunner) Run(dir, name string, args ...string) (string, error) {
	r.calls = append(r.calls, strings.Join(args[:2], " "))
	if len(args) > 0 && args[0] == "pr" && len(args) > 1 && args[1] == "merge" {
		r.mergeDirs = append(r.mergeDirs, dir)
		r.mergeArgs = append(r.mergeArgs, args)
		return "", r.mergeErr
	}
	if r.viewErr != nil {
		return "", r.viewErr
	}
	return `{"data":{"r0":{"p0":` + r.viewOut + `}}}`, nil
}

func TestPRMergeRefusesNoPullRequest(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress, "")},
		},
	}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "no pull request recorded") {
		t.Errorf("err = %v, want 'no pull request recorded'", err)
	}
}

func TestPRMergeMerges(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, out := testEnv(testConfig(t), api)
	runner := &multiRunner{viewOut: readyToMergePRJSON}
	env.NewGH = func() GH { return gh.NewWithRunner(runner) }

	var nudges int
	env.Nudge = func() { nudges++ }

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-merge: %v", err)
	}
	if !strings.Contains(out.String(), "Merged #7") {
		t.Errorf("output = %q, want it to say #7 was merged", out.String())
	}
	if len(runner.mergeDirs) != 1 || runner.mergeDirs[0] != "/tmp/nat" {
		t.Errorf("merge dirs = %v, want the project's working dir once", runner.mergeDirs)
	}
	// One batched reading of the pull request, then the merge: no gh pr view.
	if want := []string{"api graphql", "pr merge"}; strings.Join(runner.calls, ",") != strings.Join(want, ",") {
		t.Errorf("gh calls = %v, want %v", runner.calls, want)
	}
	// The merge is what marks the slice Done: the work is on main now, and
	// this is the write that says so.
	if len(api.updates) != 1 || api.updates[0].id != testSliceID {
		t.Fatalf("updates = %+v, want the slice marked Done", api.updates)
	}
	if name := api.updates[0].props[notion.PropStatus].SelectName(); name != notion.SliceDone {
		t.Errorf("status = %q, want %q", name, notion.SliceDone)
	}
	if nudges != 1 {
		t.Errorf("nudges = %d, want one for the write", nudges)
	}
}

// A merge that landed but whose status write was refused is still a merge:
// the failure says which half needs anything more, and running the command
// again is not the recovery — the board's reading settles the slice instead.
// The merge already happened on GitHub by the time MarkDone runs, and MarkDone
// writes the local plan first — so a push to the workspace that fails no
// longer takes the whole command down with it: the local write is what
// succeeds or fails now, and it succeeds. The slice is left dirty for a later
// sync to send what the push could not.
func TestPRMergeMarksDoneLocallyEvenWhenThePushFails(t *testing.T) {
	cfg := testConfig(t)
	api := &fakeAPI{
		updateErr: errors.New("notion is down"),
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, out := testEnv(cfg, api)
	var nudges int
	env.Nudge = func() { nudges++ }
	env.NewGH = func() GH { return gh.NewWithRunner(&multiRunner{viewOut: readyToMergePRJSON}) }

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1"}, env)

	if err != nil {
		t.Fatalf("pr-merge: %v, want it to succeed: the merge landed and MarkDone's local write "+
			"succeeded even though the push to the workspace failed", err)
	}
	if !strings.Contains(out.String(), `Merged #7 and marked "Write the UI" Done`) {
		t.Errorf("output = %q, want the merge reported", out.String())
	}
	if nudges != 1 {
		t.Errorf("nudges = %d, want exactly one for the write that landed locally", nudges)
	}

	path, err := store.LocalPath("project-1")
	if err != nil {
		t.Fatalf("resolve the plan path: %v", err)
	}
	local, err := store.OpenLocal(path)
	if err != nil {
		t.Fatalf("open the plan: %v", err)
	}
	defer func() {
		if err := local.Close(); err != nil {
			t.Errorf("close the plan: %v", err)
		}
	}()
	dirty, err := local.Dirty(context.Background(), testSliceID)
	if err != nil {
		t.Fatalf("read whether the slice is dirty: %v", err)
	}
	if !dirty {
		t.Error("slice not marked dirty, want the failed push left for a later sync to send")
	}
}

func TestPRMergeJSON(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, out := testEnv(testConfig(t), api)
	env.NewGH = func() GH { return gh.NewWithRunner(&multiRunner{viewOut: readyToMergePRJSON}) }

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--json", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-merge --json: %v", err)
	}
	var got mergedJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if !got.Merged {
		t.Errorf("json = %+v, want merged true", got)
	}
}

func TestPRMergeRefusesAFailingVerdict(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	runner := &multiRunner{viewOut: changesRequestedPRJSON}
	env.NewGH = func() GH { return gh.NewWithRunner(runner) }

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "cannot merge #7 — review: changes requested") {
		t.Errorf("err = %v, want the merge box's own wording", err)
	}
	if len(runner.mergeDirs) != 0 {
		t.Error("want gh never asked to merge a refused pull request")
	}
}

func TestPRMergeRefusesAlreadyMerged(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	runner := &multiRunner{viewOut: alreadyMergedPRJSON}
	env.NewGH = func() GH { return gh.NewWithRunner(runner) }

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "nothing to merge") {
		t.Errorf("err = %v, want 'nothing to merge'", err)
	}
	if len(runner.mergeDirs) != 0 {
		t.Error("want gh never asked to merge an already-merged pull request")
	}
}

func TestPRMergeReportsAViewFailure(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	env.NewGH = func() GH {
		return gh.NewWithRunner(&multiRunner{viewErr: &gh.ExitError{Code: 1, Stderr: "no such pull request"}})
	}

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "no such pull request") {
		t.Errorf("err = %v, want gh's own reason", err)
	}
}

func TestPRMergeReportsAMergeFailure(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	env.NewGH = func() GH {
		return gh.NewWithRunner(&multiRunner{
			viewOut:  readyToMergePRJSON,
			mergeErr: &gh.ExitError{Code: 1, Stderr: "a branch protection rule blocks this merge"},
		})
	}

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "a branch protection rule blocks this merge") {
		t.Errorf("err = %v, want gh's own reason", err)
	}
}

// A mergeability GitHub is still working out refuses nothing on nat's side:
// the merge is attempted, and gh's own refusal, if it makes one, is what the
// caller reads, verbatim.
func TestPRMergeAttemptsAMergeOfUnknownMergeability(t *testing.T) {
	const unknownMergeabilityPRJSON = `{
  "number": 7,
  "title": "Still computing",
  "state": "OPEN",
  "headRefName": "slice/computing",
  "baseRefName": "main",
  "url": "https://github.test/craig/nat/pull/7",
  "reviewDecision": "APPROVED",
  "mergeable": "UNKNOWN",
  "mergeStateStatus": "UNKNOWN"
}`
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	const refusal = "Pull request craig/nat#7 is not mergeable: the merge commit cannot be cleanly created."
	runner := &multiRunner{viewOut: unknownMergeabilityPRJSON, mergeErr: &gh.ExitError{Code: 1, Stderr: refusal}}
	env.NewGH = func() GH { return gh.NewWithRunner(runner) }

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1"}, env)

	if len(runner.mergeArgs) != 1 {
		t.Fatalf("gh pr merge calls = %v, want the merge attempted once", runner.mergeArgs)
	}
	if err == nil || !strings.Contains(err.Error(), refusal) {
		t.Errorf("err = %v, want gh's refusal verbatim", err)
	}
	if len(api.updates) != 0 {
		t.Errorf("updates = %+v, want nothing written for a merge gh refused", api.updates)
	}
}

// A pull request the reading could not find — a URL that names none, or one
// GitHub could not resolve — is refused before any merge is tried.
func TestPRMergeRefusesAnUnreadPullRequest(t *testing.T) {
	for _, tt := range []struct{ url, out, want string }{
		{"https://github.test/craig/nat/issues/7", "", "names no pull request"},
		{"https://github.test/craig/nat/pull/7", "null", "GitHub has no pull request at"},
	} {
		api := &fakeAPI{pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress, tt.url)},
		}}
		env, _ := testEnv(testConfig(t), api)
		runner := &multiRunner{viewOut: tt.out}
		env.NewGH = func() GH { return gh.NewWithRunner(runner) }

		err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1"}, env)

		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want %q", tt.url, err, tt.want)
		}
		if len(runner.mergeDirs) != 0 {
			t.Errorf("%s: want gh never asked to merge", tt.url)
		}
	}
}

func TestPRMergeRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-merge", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "want exactly one") {
		t.Errorf("err = %v, want 'want exactly one'", err)
	}
}

func TestPRMergeRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--bogus", "--project", "project-1"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestPRMergeRefusesAnInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-merge", "not-a-uuid", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("err = %v, want 'not a slice'", err)
	}
}

func TestPRMergeRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestPRMergeReportsAFailedRead(t *testing.T) {
	api := &fakeAPI{getErr: errors.New("notion is down")}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "load the slice") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}

// The plan file is hydrated from the workspace before anything else, and a
// workspace that will not answer that first read fails the command before
// it ever gets to the slice itself.
func TestPRMergeReportsAFailedHydrate(t *testing.T) {
	api := &fakeAPI{dataSourceErr: errors.New("notion is down")}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "hydrate the plan") {
		t.Errorf("err = %v, want the failed hydrate named", err)
	}
}

// The merge landed on GitHub whatever happens next, but the local write that
// marks the slice Done is still a write to the plan file — and a plan whose
// sync bookkeeping cannot be written fails that write, and the command says
// so rather than pretending the merge was never recorded.
func TestPRMergeReportsAFailedLocalMarkDone(t *testing.T) {
	cfg := testConfig(t)
	seedHydratedSlice(t, "project-1", testSliceID, "Write the UI", "In progress", func(db *sql.DB) {
		if _, err := db.Exec(`UPDATE slices SET pr = ? WHERE id = ?`,
			"https://github.test/craig/nat/pull/7", testSliceID); err != nil {
			t.Fatalf("seed the PR: %v", err)
		}
		if _, err := db.Exec(`DROP TABLE sync`); err != nil {
			t.Fatalf("break the plan's sync table: %v", err)
		}
	})
	env, _ := testEnv(cfg, &fakeAPI{})
	env.NewGH = func() GH { return gh.NewWithRunner(&multiRunner{viewOut: readyToMergePRJSON}) }

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "could not mark") {
		t.Errorf("err = %v, want the failed local MarkDone write named", err)
	}
}

// The merge is made as the project's config says: its strategy and, only
// where asked, --delete-branch.
func TestPRMergeUsesTheProjectsMergeSettings(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	cfg := testConfig(t)
	p := cfg.Projects["project-1"]
	p.MergeMethod, p.DeleteBranch = "squash", true
	cfg.Projects["project-1"] = p
	env, _ := testEnv(cfg, api)
	runner := &multiRunner{viewOut: readyToMergePRJSON}
	env.NewGH = func() GH { return gh.NewWithRunner(runner) }

	if err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-merge: %v", err)
	}
	want := []string{"pr", "merge", "https://github.test/craig/nat/pull/7", "--squash", "--delete-branch"}
	if len(runner.mergeArgs) != 1 || !reflect.DeepEqual(runner.mergeArgs[0], want) {
		t.Errorf("merge args = %v, want %v", runner.mergeArgs, want)
	}
}
