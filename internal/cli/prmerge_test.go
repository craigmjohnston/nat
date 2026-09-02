package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
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

// multiRunner answers ViewPR (pr view) and MergePR (pr merge) with different
// canned responses, the way one gh.CLI answers both calls a merge makes.
type multiRunner struct {
	viewOut   string
	viewErr   error
	mergeErr  error
	mergeDirs []string
	mergeArgs [][]string
}

func (r *multiRunner) Run(dir, name string, args ...string) (string, error) {
	if len(args) > 0 && args[0] == "pr" && len(args) > 1 && args[1] == "merge" {
		r.mergeDirs = append(r.mergeDirs, dir)
		r.mergeArgs = append(r.mergeArgs, args)
		return "", r.mergeErr
	}
	return r.viewOut, r.viewErr
}

func TestPRMergeRefusesNoPullRequest(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress, "")},
		},
	}
	env, _ := testEnv(testConfig(), api)

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
	env, out := testEnv(testConfig(), api)
	runner := &multiRunner{viewOut: readyToMergePRJSON}
	env.NewGH = func() GH { return gh.NewWithRunner(runner) }

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
}

func TestPRMergeJSON(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, out := testEnv(testConfig(), api)
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
	env, _ := testEnv(testConfig(), api)
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
	env, _ := testEnv(testConfig(), api)
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
	env, _ := testEnv(testConfig(), api)
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
	env, _ := testEnv(testConfig(), api)
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

func TestPRMergeRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-merge", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "want exactly one") {
		t.Errorf("err = %v, want 'want exactly one'", err)
	}
}

func TestPRMergeRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--bogus", "--project", "project-1"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestPRMergeRefusesAnInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-merge", "not-a-uuid", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("err = %v, want 'not a slice'", err)
	}
}

func TestPRMergeRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestPRMergeReportsAFailedRead(t *testing.T) {
	api := &fakeAPI{getErr: errors.New("notion is down")}
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"pr-merge", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "load the slice") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}
