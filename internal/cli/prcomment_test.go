package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
)

// fakeCommentRunner stands in for gh, recording what was asked and answering
// with what a real `gh pr comment` prints: the comment's own URL.
type fakeCommentRunner struct {
	out   string
	err   error
	dir   string
	args  []string
	stdin string
}

func (f *fakeCommentRunner) Run(dir, name string, args ...string) (string, error) {
	f.dir, f.args = dir, args
	return f.out, f.err
}

func (f *fakeCommentRunner) RunWithStdin(dir string, stdin io.Reader, name string, args ...string) (string, error) {
	f.dir, f.args = dir, args
	b, _ := io.ReadAll(stdin)
	f.stdin = string(b)
	return f.out, f.err
}

var _ gh.StdinRunner = (*fakeCommentRunner)(nil)

func TestPRCommentPostsTheComment(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, out := testEnv(testConfig(), api)
	runner := &fakeCommentRunner{out: "https://github.test/craig/nat/pull/7#issuecomment-1\n"}
	env.NewGH = func() GH { return gh.NewWithRunner(runner) }

	err := Run(context.Background(), []string{
		"pr-comment", testSliceID, "--body", "Looks good.", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("pr-comment: %v", err)
	}
	if runner.dir != "/tmp/nat" {
		t.Errorf("ran gh in %q, want the project's working dir", runner.dir)
	}
	want := []string{"pr", "comment", "https://github.test/craig/nat/pull/7", "--body-file", "-"}
	if len(runner.args) != len(want) {
		t.Fatalf("args = %v, want %v", runner.args, want)
	}
	for i := range want {
		if runner.args[i] != want[i] {
			t.Errorf("args = %v, want %v", runner.args, want)
		}
	}
	if runner.stdin != "Looks good." {
		t.Errorf("stdin = %q, want the comment body", runner.stdin)
	}
	if !strings.Contains(out.String(), "https://github.test/craig/nat/pull/7#issuecomment-1") {
		t.Errorf("output missing the comment URL:\n%s", out.String())
	}
}

func TestPRCommentReadsTheBodyFromStdinByDefault(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(), api)
	env.In = strings.NewReader("Piped in comment.")
	runner := &fakeCommentRunner{}
	env.NewGH = func() GH { return gh.NewWithRunner(runner) }

	err := Run(context.Background(), []string{"pr-comment", testSliceID, "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-comment: %v", err)
	}
	if runner.stdin != "Piped in comment." {
		t.Errorf("stdin = %q, want the piped-in comment", runner.stdin)
	}
}

func TestPRCommentJSON(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, out := testEnv(testConfig(), api)
	runner := &fakeCommentRunner{out: "https://github.test/craig/nat/pull/7#issuecomment-1\n"}
	env.NewGH = func() GH { return gh.NewWithRunner(runner) }

	err := Run(context.Background(), []string{
		"pr-comment", testSliceID, "--body", "Looks good.", "--json", "--project", "project-1",
	}, env)
	if err != nil {
		t.Fatalf("pr-comment --json: %v", err)
	}
	var got prCommentedJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if got.PR != "https://github.test/craig/nat/pull/7" ||
		got.CommentURL != "https://github.test/craig/nat/pull/7#issuecomment-1" {
		t.Errorf("json = %+v", got)
	}
}

func TestPRCommentRefusesNoPullRequest(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress, "")},
		},
	}
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{
		"pr-comment", testSliceID, "--body", "Looks good.", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "no pull request recorded") {
		t.Errorf("err = %v, want 'no pull request recorded'", err)
	}
}

// TestPRCommentRefusesNoBodyAndNoStdin covers running with neither --body nor
// anything piped in at all: nothing to read, so it is the same empty-comment
// refusal rather than a wait on a stdin that will never arrive.
func TestPRCommentRefusesNoBodyAndNoStdin(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-comment", testSliceID, "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "no comment given") {
		t.Errorf("err = %v, want 'no comment given'", err)
	}
}

func TestPRCommentRefusesAnEmptyBody(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{
		"pr-comment", testSliceID, "--body", "   ", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "no comment given") {
		t.Errorf("err = %v, want 'no comment given'", err)
	}
}

func TestPRCommentReportsAGHFailure(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(), api)
	env.NewGH = func() GH {
		return gh.NewWithRunner(&fakeCommentRunner{err: &gh.ExitError{Code: 1, Stderr: "no such pull request"}})
	}

	err := Run(context.Background(), []string{
		"pr-comment", testSliceID, "--body", "Looks good.", "--project", "project-1",
	}, env)
	if err == nil || !strings.Contains(err.Error(), "no such pull request") {
		t.Errorf("err = %v, want gh's own reason", err)
	}
}

func TestPRCommentRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-comment", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "want exactly one") {
		t.Errorf("err = %v, want 'want exactly one'", err)
	}
}

func TestPRCommentRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-comment", testSliceID, "--bogus", "--project", "project-1"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestPRCommentRefusesAnInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-comment", "not-a-uuid", "--body", "x", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("err = %v, want 'not a slice'", err)
	}
}

func TestPRCommentRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-comment", testSliceID, "--body", "x", "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

// A stdin that cannot be read fails before Notion is touched at all, the same
// as every other command that may read the flag it takes off a pipe.
func TestPRCommentRefusesAnUnreadableStdin(t *testing.T) {
	env, _ := testEnv(testConfig(), &fakeAPI{})
	env.In = errReader{err: errors.New("boom")}

	err := Run(context.Background(), []string{"pr-comment", testSliceID, "--project", "project-1"}, env)
	if err == nil || !strings.Contains(err.Error(), "read the comment") {
		t.Errorf("err = %v, want the failed stdin read named", err)
	}
}

func TestPRCommentReportsAFailedRead(t *testing.T) {
	api := &fakeAPI{getErr: errors.New("notion is down")}
	env, _ := testEnv(testConfig(), api)

	err := Run(context.Background(), []string{"pr-comment", testSliceID, "--body", "x", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "load the slice") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}
