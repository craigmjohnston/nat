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

// prEditAPI holds one in-progress slice with a pull request recorded.
func prEditAPI() *fakeAPI {
	return &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
}

func TestPREditReplacesTheDescription(t *testing.T) {
	api := prEditAPI()
	env, out := testEnv(testConfig(t), api)
	runner := &fakeCommentRunner{}
	env.NewGH = func() GH { return gh.NewWithRunner(runner) }

	err := Run(context.Background(), []string{
		"pr-edit", testSliceID, "--body", "A new description.", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("pr-edit: %v", err)
	}
	if runner.dir != "/tmp/nat" {
		t.Errorf("ran gh in %q, want the project's working dir", runner.dir)
	}
	want := "pr edit https://github.test/craig/nat/pull/7 --body-file -"
	if got := strings.Join(runner.args, " "); got != want {
		t.Errorf("args = %q, want %q", got, want)
	}
	if runner.stdin != "A new description." {
		t.Errorf("stdin = %q, want the description", runner.stdin)
	}
	if len(api.appends) != 0 || len(api.updates) != 0 {
		t.Errorf("wrote to Notion (appends %v, updates %v), want nothing", api.appends, api.updates)
	}
	if !strings.Contains(out.String(), "Description edited") {
		t.Errorf("output =\n%s\nwant the edit reported", out.String())
	}
}

func TestPREditReadsTheBodyFromStdinByDefault(t *testing.T) {
	env, out := testEnv(testConfig(t), prEditAPI())
	env.In = strings.NewReader("Piped in description 🎉\n")
	runner := &fakeCommentRunner{}
	env.NewGH = func() GH { return gh.NewWithRunner(runner) }

	err := Run(context.Background(), []string{"pr-edit", testSliceID, "--json", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-edit: %v", err)
	}
	if runner.stdin != "Piped in description 🎉" {
		t.Errorf("stdin = %q, want the piped-in description", runner.stdin)
	}
	var got prEditedJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if got.PR != "https://github.test/craig/nat/pull/7" {
		t.Errorf("json = %+v", got)
	}
}

func TestPREditRefusesNoPullRequest(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress, "")},
		},
	}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{"pr-edit", testSliceID, "--body", "x", "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "no pull request recorded") {
		t.Errorf("err = %v, want 'no pull request recorded'", err)
	}
}

func TestPREditRefusesAnEmptyDescription(t *testing.T) {
	for _, args := range [][]string{
		{"pr-edit", testSliceID, "--body", "   ", "--project", "project-1"},
		{"pr-edit", testSliceID, "--project", "project-1"},
	} {
		env, _ := testEnv(testConfig(t), &fakeAPI{})
		err := Run(context.Background(), args, env)
		if err == nil || !strings.Contains(err.Error(), "no description given") {
			t.Errorf("%v: err = %v, want 'no description given'", args, err)
		}
	}
}

func TestPREditRefusesAnUnreadableStdin(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})
	env.In = errReader{err: errors.New("boom")}

	err := Run(context.Background(), []string{"pr-edit", testSliceID, "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "read the description") {
		t.Errorf("err = %v, want the failed stdin read named", err)
	}
}

func TestPREditReportsAGHFailure(t *testing.T) {
	env, _ := testEnv(testConfig(t), prEditAPI())
	env.NewGH = func() GH {
		return gh.NewWithRunner(&fakeCommentRunner{err: &gh.ExitError{Code: 1, Stderr: "no such pull request"}})
	}

	err := Run(context.Background(), []string{"pr-edit", testSliceID, "--body", "x", "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "no such pull request") {
		t.Errorf("err = %v, want gh's own reason", err)
	}
}

func TestPREditRefusesBadArguments(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"pr-edit", "--project", "project-1"}, "want exactly one"},
		{[]string{"pr-edit", testSliceID, "--bogus", "--project", "project-1"}, "pr-edit"},
		{[]string{"pr-edit", "not-a-uuid", "--body", "x", "--project", "project-1"}, "not a slice"},
		{[]string{"pr-edit", testSliceID, "--body", "x", "--project", "nope"}, "no project nope"},
	} {
		env, _ := testEnv(testConfig(t), &fakeAPI{})
		err := Run(context.Background(), tc.args, env)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestPREditReportsAFailedRead(t *testing.T) {
	for _, tc := range []struct {
		api  *fakeAPI
		want string
	}{
		{&fakeAPI{getErr: errors.New("notion is down")}, "load the slice"},
		{&fakeAPI{dataSourceErr: errors.New("notion is down")}, "hydrate the plan"},
	} {
		env, _ := testEnv(testConfig(t), tc.api)
		err := Run(context.Background(), []string{"pr-edit", testSliceID, "--body", "x", "--project", "project-1"}, env)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("err = %v, want %q", err, tc.want)
		}
	}
}
