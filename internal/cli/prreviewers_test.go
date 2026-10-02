package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
)

// fakeReviewersRunner answers gh by subcommand: pr view with view, the
// collaborators API with collaborators (or collabErr), and pr edit with
// editErr — recording every run.
type fakeReviewersRunner struct {
	view          string
	collaborators string
	collabErr     error
	editErr       error
	runs          [][]string
}

func (f *fakeReviewersRunner) Run(dir, name string, args ...string) (string, error) {
	f.runs = append(f.runs, args)
	switch {
	case args[0] == "api":
		return f.collaborators, f.collabErr
	case args[1] == "edit":
		return "", f.editErr
	default:
		return f.view, nil
	}
}

const reviewersPR = "https://github.test/craig/nat/pull/7"

func reviewersEnv(t *testing.T, runner *fakeReviewersRunner) (Env, *bytes.Buffer) {
	t.Helper()
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress, reviewersPR)},
		},
	}
	env, out := testEnv(testConfig(t), api)
	env.NewGH = func() GH { return gh.NewWithRunner(runner) }
	return env, out
}

func TestPRReviewersListsRequestedAndCandidates(t *testing.T) {
	runner := &fakeReviewersRunner{
		view:          `{"number":7,"author":{"login":"craig"},"reviewRequests":[{"login":"octocat"}]}`,
		collaborators: "craig\noctocat\nhubot\n",
	}
	env, out := reviewersEnv(t, runner)

	err := Run(context.Background(), []string{"pr-reviewers", testSliceID, "--json", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-reviewers: %v", err)
	}
	var doc prReviewersJSON
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	want := prReviewersJSON{PR: reviewersPR, Requested: []string{"octocat"}, Candidates: []string{"hubot"}}
	if !reflect.DeepEqual(doc, want) {
		t.Errorf("doc = %+v, want %+v", doc, want)
	}
	for _, run := range runner.runs {
		if run[0] == "pr" && run[1] == "edit" {
			t.Errorf("a plain read edited the pull request: %v", run)
		}
	}
}

func TestPRReviewersEditsBeforeReadingBack(t *testing.T) {
	runner := &fakeReviewersRunner{view: `{"number":7,"reviewRequests":[{"login":"hubot"}]}`}
	env, out := reviewersEnv(t, runner)

	err := Run(context.Background(), []string{
		"pr-reviewers", testSliceID, "--add", "hubot,org/core", "--add", "x", "--remove", "octocat",
		"--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("pr-reviewers: %v", err)
	}
	wantEdit := []string{"pr", "edit", reviewersPR, "--add-reviewer", "hubot,org/core,x", "--remove-reviewer", "octocat"}
	if !reflect.DeepEqual(runner.runs[0], wantEdit) {
		t.Errorf("first run = %v, want the edit %v", runner.runs[0], wantEdit)
	}
	if runner.runs[1][1] != "view" {
		t.Errorf("second run = %v, want the read back", runner.runs[1])
	}
	for _, line := range []string{"- Requested: hubot", "- Could also ask: none"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("markdown lacks %q:\n%s", line, out)
		}
	}
}

func TestPRReviewersReportsAFailedCandidateListWithoutFailing(t *testing.T) {
	runner := &fakeReviewersRunner{view: `{"number":7}`, collabErr: &gh.ExitError{Code: 1, Stderr: "HTTP 403"}}
	env, out := reviewersEnv(t, runner)

	if err := Run(context.Background(), []string{"pr-reviewers", testSliceID, "--json", "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-reviewers: %v", err)
	}
	var doc prReviewersJSON
	_ = json.Unmarshal(out.Bytes(), &doc)
	if doc.CandidatesError != "HTTP 403" || len(doc.Candidates) != 0 || doc.Requested == nil {
		t.Errorf("doc = %+v, want the listing's error beside empty lists", doc)
	}

	env, out = reviewersEnv(t, runner)
	if err := Run(context.Background(), []string{"pr-reviewers", testSliceID, "--project", "project-1"}, env); err != nil {
		t.Fatalf("pr-reviewers: %v", err)
	}
	if !strings.Contains(out.String(), "unknown (HTTP 403)") {
		t.Errorf("markdown = %s, want the listing's error", out)
	}
}

func TestPRReviewersRefusals(t *testing.T) {
	runner := &fakeReviewersRunner{editErr: &gh.ExitError{Code: 1, Stderr: "octocat is the author"}}
	env, _ := reviewersEnv(t, runner)
	err := Run(context.Background(), []string{"pr-reviewers", testSliceID, "--add", "octocat", "--project", "project-1"}, env)
	var exit *gh.ExitError
	if !errors.As(err, &exit) {
		t.Errorf("err = %v, want gh's refusal", err)
	}

	for _, args := range [][]string{
		{"pr-reviewers", "--project", "project-1"},
		{"pr-reviewers", "not-an-id", "--project", "project-1"},
		{"pr-reviewers", testSliceID, "--bogus", "--project", "project-1"},
		{"pr-reviewers", testSliceID},
	} {
		env, _ := reviewersEnv(t, &fakeReviewersRunner{})
		if err := Run(context.Background(), args, env); err == nil {
			t.Errorf("%v: no refusal", args)
		}
	}
}

func TestPRReviewersRefusesASliceWithNoPR(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePage(testSliceID, "Write the UI", notion.SliceTodo, "m1", "", "")},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	err := Run(context.Background(), []string{"pr-reviewers", testSliceID, "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "no pull request") {
		t.Errorf("err = %v, want the no-PR refusal", err)
	}
}

func TestPRReviewersFailedReadAndStore(t *testing.T) {
	failing := &failingViewRunner{}
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress, reviewersPR)},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	env.NewGH = func() GH { return gh.NewWithRunner(failing) }
	if err := Run(context.Background(), []string{"pr-reviewers", testSliceID, "--project", "project-1"}, env); err == nil {
		t.Error("a failed pull request read was not reported")
	}
}

type failingViewRunner struct{}

func (failingViewRunner) Run(dir, name string, args ...string) (string, error) {
	return "", &gh.ExitError{Code: 1, Stderr: "no pull request"}
}

func TestPRReviewersReportsAFailedReadAndHydrate(t *testing.T) {
	for _, tc := range []struct {
		api  *fakeAPI
		want string
	}{
		{&fakeAPI{getErr: errors.New("notion is down")}, "load the slice"},
		{&fakeAPI{dataSourceErr: errors.New("notion is down")}, "hydrate the plan"},
	} {
		env, _ := testEnv(testConfig(t), tc.api)
		err := Run(context.Background(), []string{"pr-reviewers", testSliceID, "--project", "project-1"}, env)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("err = %v, want %q", err, tc.want)
		}
	}
}
