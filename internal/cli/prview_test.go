package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/gh"
	"github.com/craigmjohnston/nat/internal/notion"
)

// fullPROpenJSON is gh's answer for an open pull request with a description,
// one check, one review and one comment — everything pr-view can draw.
const fullPROpenJSON = `{
  "number": 7,
  "title": "Read a pull request",
  "body": "The description.",
  "state": "OPEN",
  "isDraft": false,
  "author": {"login": "craig"},
  "baseRefName": "main",
  "headRefName": "slice/read-pr",
  "url": "https://github.test/craig/nat/pull/7",
  "reviewDecision": "APPROVED",
  "mergeable": "MERGEABLE",
  "mergeStateStatus": "CLEAN",
  "statusCheckRollup": [
    {"__typename": "CheckRun", "name": "test", "status": "COMPLETED", "conclusion": "SUCCESS", "detailsUrl": "https://ci.test/1"}
  ],
  "reviews": [
    {"author": {"login": "reviewer"}, "state": "APPROVED", "body": "LGTM", "submittedAt": "2026-08-28T11:30:00Z"}
  ],
  "comments": [
    {"author": {"login": "craig"}, "body": "Rebased.", "createdAt": "2026-08-28T09:15:00Z", "url": "https://github.test/craig/nat/pull/7#issuecomment-1"}
  ],
  "additions": 42,
  "deletions": 8,
  "changedFiles": 5,
  "commits": [{"oid": "a"}, {"oid": "b"}, {"oid": "c"}]
}`

// barePROpenJSON is gh's answer for a pull request with none of the optional
// facts filled in, which is what an empty repository, an unreviewed pull
// request with no description at all looks like.
const barePROpenJSON = `{
  "number": 9,
  "title": "A bare pull request",
  "state": "OPEN",
  "headRefName": "slice/bare",
  "baseRefName": "main",
  "url": "https://github.test/craig/nat/pull/9"
}`

func slicePageWithPR(id, name, status, pr string) notion.Page {
	props := map[string]notion.PropertyValue{
		notion.PropName:   title(name),
		notion.PropStatus: notion.NewSelect(status),
	}
	if pr != "" {
		props[notion.PropPR] = notion.NewURL(pr)
	}
	return notion.Page{ID: id, URL: "https://notion.so/" + id, Properties: props}
}

func TestPRViewRefusesNoPullRequest(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress, "")},
		},
	}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{"pr-view", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "no pull request recorded") {
		t.Errorf("err = %v, want 'no pull request recorded'", err)
	}
}

func TestPRViewReadsThePullRequest(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, out := testEnv(testConfig(t), api)
	runner := &fakeGHRunner{out: fullPROpenJSON}
	env.NewGH = func() GH { return gh.NewWithRunner(runner) }

	err := Run(context.Background(), []string{"pr-view", testSliceID, "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-view: %v", err)
	}
	if runner.dir != "/tmp/nat" {
		t.Errorf("ran gh in %q, want the project's working dir", runner.dir)
	}
	for _, want := range []string{
		"# #7 Read a pull request", "State: open", "Author: craig",
		"slice/read-pr → main", "The description.", "test: SUCCESS",
		"reviewer (APPROVED): LGTM", "craig: Rebased.",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestPRViewOfABarePullRequest(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/9")},
		},
	}
	env, out := testEnv(testConfig(t), api)
	env.NewGH = func() GH { return gh.NewWithRunner(&fakeGHRunner{out: barePROpenJSON}) }

	err := Run(context.Background(), []string{"pr-view", testSliceID, "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-view: %v", err)
	}
	for _, want := range []string{
		"Author: (no status)", "Review: (no status)", "Mergeable: (no status)", "Merge state: (no status)",
		"## Description\n\n_none_", "## Checks\n\n_none_", "## Reviews\n\n_none_", "## Comments\n\n_none_",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestPRViewJSON(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, out := testEnv(testConfig(t), api)
	env.NewGH = func() GH { return gh.NewWithRunner(&fakeGHRunner{out: fullPROpenJSON}) }

	err := Run(context.Background(), []string{"pr-view", testSliceID, "--json", "--project", "project-1"}, env)
	if err != nil {
		t.Fatalf("pr-view --json: %v", err)
	}

	var got prDoc
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if got.Number != 7 || got.Title != "Read a pull request" || got.State != "OPEN" {
		t.Errorf("json = %+v", got)
	}
	if len(got.Checks) != 1 || got.Checks[0] != (checkJSON{Name: "test", State: "SUCCESS", Link: "https://ci.test/1"}) {
		t.Errorf("checks = %+v", got.Checks)
	}
	if len(got.Reviews) != 1 || got.Reviews[0].Author != "reviewer" || got.Reviews[0].Body != "LGTM" {
		t.Errorf("reviews = %+v", got.Reviews)
	}
	if len(got.Comments) != 1 || got.Comments[0].Author != "craig" || got.Comments[0].Body != "Rebased." {
		t.Errorf("comments = %+v", got.Comments)
	}
	if got.Additions != 42 || got.Deletions != 8 || got.ChangedFiles != 5 || got.Commits != 3 {
		t.Errorf("change stats = +%d/-%d files=%d commits=%d, want +42/-8 files=5 commits=3",
			got.Additions, got.Deletions, got.ChangedFiles, got.Commits)
	}
}

func TestPRViewReportsAGHFailure(t *testing.T) {
	api := &fakeAPI{
		pages: map[string][]notion.Page{
			"slices-ds": {slicePageWithPR(testSliceID, "Write the UI", notion.SliceInProgress,
				"https://github.test/craig/nat/pull/7")},
		},
	}
	env, _ := testEnv(testConfig(t), api)
	env.NewGH = func() GH {
		return gh.NewWithRunner(&fakeGHRunner{err: &gh.ExitError{Code: 1, Stderr: "no such pull request"}})
	}

	err := Run(context.Background(), []string{"pr-view", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "no such pull request") {
		t.Errorf("err = %v, want gh's own reason", err)
	}
}

func TestPRViewRefusesWrongArgumentCount(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-view", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "want exactly one") {
		t.Errorf("err = %v, want 'want exactly one'", err)
	}
}

func TestPRViewRefusesAnUnknownFlag(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-view", testSliceID, "--bogus", "--project", "project-1"}, env)

	var usage *UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("err = %v (%T), want a *UsageError", err, err)
	}
}

func TestPRViewRefusesAnInvalidSliceID(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-view", "not-a-uuid", "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "not a slice") {
		t.Errorf("err = %v, want 'not a slice'", err)
	}
}

func TestPRViewRefusesAnUnknownProject(t *testing.T) {
	env, _ := testEnv(testConfig(t), &fakeAPI{})

	err := Run(context.Background(), []string{"pr-view", testSliceID, "--project", "nope"}, env)

	if err == nil || !strings.Contains(err.Error(), "no project nope") {
		t.Errorf("err = %v, want the unknown project named", err)
	}
}

func TestPRViewReportsAFailedRead(t *testing.T) {
	api := &fakeAPI{getErr: errors.New("notion is down")}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{"pr-view", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "load the slice") {
		t.Errorf("err = %v, want the failed read named", err)
	}
}

// A session has no slice to record a pull request on, so --session names the
// pull request directly and gh reads it from the session's own directory.
func TestPRViewOfASessionsPullRequest(t *testing.T) {
	env, out := sessionTestEnv(t)
	dir := t.TempDir()
	id := seedSession(t, env, dir, "session/one")
	runner := &fakeGHRunner{out: fullPROpenJSON}
	env.NewGH = func() GH { return gh.NewWithRunner(runner) }
	ref := "https://github.test/craig/nat/pull/7"

	if err := Run(context.Background(), []string{"pr-view", "--session", id, "--json", "--project", "project-1", ref}, env); err != nil {
		t.Fatalf("pr-view --session: %v", err)
	}
	if runner.dir != dir {
		t.Errorf("ran gh in %q, want the session's directory %q", runner.dir, dir)
	}
	var got prDoc
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	if got.Number != 7 || got.Title != "Read a pull request" {
		t.Errorf("json = %+v", got)
	}
}

func TestPRViewOfASessionsPullRequestAsMarkdown(t *testing.T) {
	env, out := sessionTestEnv(t)
	id := seedSession(t, env, t.TempDir(), "session/one")
	env.NewGH = func() GH { return gh.NewWithRunner(&fakeGHRunner{out: fullPROpenJSON}) }

	if err := Run(context.Background(), []string{"pr-view", "--session", id, "--project", "project-1", "7"}, env); err != nil {
		t.Fatalf("pr-view --session: %v", err)
	}
	if !strings.Contains(out.String(), "# #7 Read a pull request") {
		t.Errorf("output = %s, want the pull request", out.String())
	}
}

func TestPRViewOfASessionRefusals(t *testing.T) {
	env, _ := sessionTestEnv(t)
	id := seedSession(t, env, t.TempDir(), "session/one")
	env.NewGH = func() GH {
		return gh.NewWithRunner(&fakeGHRunner{err: &gh.ExitError{Code: 1, Stderr: "no such pull request"}})
	}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no pull request named", []string{"pr-view", "--session", id, "--project", "project-1"}, "want exactly one pull request"},
		{"invalid session", []string{"pr-view", "--session", "not-a-uuid", "--project", "project-1", "7"}, "not a slice"},
		{"unknown session", []string{"pr-view", "--session", testSessionUUID, "--project", "project-1", "7"}, "no session"},
		{"unknown project", []string{"pr-view", "--session", id, "--project", "nope", "7"}, "no project nope"},
		{"gh refuses", []string{"pr-view", "--session", id, "--project", "project-1", "7"}, "no such pull request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Run(context.Background(), tt.args, env); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestPRViewOfASessionReportsAFailedStoreRead(t *testing.T) {
	api := &fakeAPI{queryErr: map[string]error{"slices-ds": errors.New("notion: 500")}}
	env, _ := testEnv(testClaimConfig(t), api)
	err := Run(context.Background(), []string{"pr-view", "--session", testSessionUUID, "--project", "project-1", "7"}, env)
	if err == nil || !strings.Contains(err.Error(), "notion: 500") {
		t.Errorf("err = %v, want the failed hydrate reported", err)
	}
}

func TestPRViewOfASessionReportsAFailedSessionsRead(t *testing.T) {
	env, _ := sessionTestEnv(t)
	seedHydratedProject(t, "project-1", func(db *sql.DB) { dropSessionsTable(t, db) })
	err := Run(context.Background(), []string{"pr-view", "--session", testSessionUUID, "--project", "project-1", "7"}, env)
	if err == nil || !strings.Contains(err.Error(), "read the sessions") {
		t.Errorf("err = %v, want 'read the sessions'", err)
	}
}

func TestPRStateWord(t *testing.T) {
	tests := []struct {
		pr   gh.PR
		want string
	}{
		{gh.PR{State: gh.PRStateMerged}, "merged"},
		{gh.PR{State: gh.PRStateClosed}, "closed"},
		{gh.PR{State: "OPEN", IsDraft: true}, "draft"},
		{gh.PR{State: "OPEN"}, "open"},
	}
	for _, tt := range tests {
		if got := prStateWord(tt.pr); got != tt.want {
			t.Errorf("prStateWord(%+v) = %q, want %q", tt.pr, got, tt.want)
		}
	}
}

// The plan file is hydrated from the workspace before anything else, and a
// workspace that will not answer that first read fails the command before
// it ever gets to the slice itself.
func TestPRViewReportsAFailedHydrate(t *testing.T) {
	api := &fakeAPI{dataSourceErr: errors.New("notion is down")}
	env, _ := testEnv(testConfig(t), api)

	err := Run(context.Background(), []string{"pr-view", testSliceID, "--project", "project-1"}, env)

	if err == nil || !strings.Contains(err.Error(), "hydrate the plan") {
		t.Errorf("err = %v, want the failed hydrate named", err)
	}
}
