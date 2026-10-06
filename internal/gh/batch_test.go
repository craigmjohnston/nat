package gh

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// scriptRunner answers each run with the next of outs (the last repeating),
// failing each with the matching errs entry where there is one, and records
// every document it was asked.
type scriptRunner struct {
	outs []string
	errs []error
	docs []string
	dirs []string
}

func (s *scriptRunner) Run(dir, name string, args ...string) (string, error) {
	i := min(len(s.docs), len(s.outs)-1)
	s.dirs = append(s.dirs, dir)
	if name != Binary || len(args) != 4 || args[0] != "api" || args[1] != "graphql" || args[2] != "-f" ||
		!strings.HasPrefix(args[3], "query=") {
		return "", fmt.Errorf("unexpected invocation %s %v", name, args)
	}
	s.docs = append(s.docs, strings.TrimPrefix(args[3], "query="))
	var err error
	if i < len(s.errs) {
		err = s.errs[i]
	}
	return s.outs[i], err
}

var natPR = PRRef{Owner: "craig", Repo: "nat", Number: 7}

// TestReadPRsOpen decodes an open pull request's status: its review and
// mergeability, its base, and its rollup through the one check naming — a run
// led by its workflow, a run with none bare, a status context by its context.
func TestReadPRsOpen(t *testing.T) {
	runner := &scriptRunner{outs: []string{fixture(t, "graphql-open.json")}}
	batch, err := NewWithRunner(runner).ReadPRs(BatchQuery{PRs: []PRRef{natPR}})
	if err != nil {
		t.Fatalf("ReadPRs() = %v", err)
	}
	pr, read := batch.PRs[natPR]
	if !read {
		t.Fatalf("ReadPRs() = %+v, want #7 read", batch)
	}
	if pr.State != "OPEN" || !pr.MergedAt.IsZero() || pr.Number != 7 || pr.BaseRefName != "main" ||
		pr.MergeStateStatus != "CLEAN" {
		t.Errorf("pr = %+v, want #7 open against main, clean", pr)
	}
	wantChecks := []Check{
		{Name: "deploy/preview", State: "SUCCESS", URL: "https://ci.example/9"},
		{Name: "lint", State: "IN_PROGRESS", URL: "https://github.com/craig/nat/actions/runs/12/job/22"},
		{Name: "Pull request / Gate", State: "FAILURE", URL: "https://github.com/craig/nat/actions/runs/11/job/21"},
	}
	if !reflect.DeepEqual(pr.Checks, wantChecks) {
		t.Errorf("checks = %+v, want %+v", pr.Checks, wantChecks)
	}
	status := StatusOf(pr)
	if !status.Approved || !status.Mergeable || status.Checks != ChecksFailing || len(status.Failing) != 1 {
		t.Errorf("StatusOf() = %+v, want approved, mergeable, one failing check", status)
	}
	want := &RateLimit{Limit: 5000, Remaining: 4211, ResetAt: time.Date(2026, 10, 6, 13, 0, 0, 0, time.UTC)}
	if !reflect.DeepEqual(batch.RateLimit, want) {
		t.Errorf("RateLimit = %+v, want %+v", batch.RateLimit, want)
	}
	if runner.dirs[0] != "" {
		t.Errorf("ran in %q, want no repository directory", runner.dirs[0])
	}
}

// TestReadPRsMergedAndClosed reads the two ends a pull request comes to: a
// merge with its time, and a close with none.
func TestReadPRsMergedAndClosed(t *testing.T) {
	for _, tt := range []struct {
		fixture string
		state   string
		merged  time.Time
	}{
		{"graphql-merged.json", PRStateMerged, time.Date(2026, 10, 6, 12, 19, 0, 0, time.UTC)},
		{"graphql-closed.json", PRStateClosed, time.Time{}},
	} {
		t.Run(tt.state, func(t *testing.T) {
			batch, err := NewWithRunner(&scriptRunner{outs: []string{fixture(t, tt.fixture)}}).
				ReadPRs(BatchQuery{PRs: []PRRef{natPR}})
			if err != nil {
				t.Fatalf("ReadPRs() = %v", err)
			}
			status := StatusOf(batch.PRs[natPR])
			if status.State != tt.state || !status.MergedAt.Equal(tt.merged) || status.Checks != ChecksNone {
				t.Errorf("StatusOf() = %+v, want %s at %v with no checks", status, tt.state, tt.merged)
			}
		})
	}
}

// TestReadPRsHeads lists each branch's pull requests, a branch with none read
// as none rather than unread, and one the answer left out as unread.
func TestReadPRsHeads(t *testing.T) {
	first := HeadRef{Owner: "craig", Repo: "nat", Branch: "session/abc"}
	second := HeadRef{Owner: "craig", Repo: "nat", Branch: "scratch"}
	batch, err := NewWithRunner(&scriptRunner{outs: []string{fixture(t, "graphql-head.json")}}).
		ReadPRs(BatchQuery{Heads: []HeadRef{first, second, {Owner: "craig", Repo: "nat", Branch: "left-out"}}})
	if err != nil {
		t.Fatalf("ReadPRs() = %v", err)
	}
	want := []HeadPR{
		{Number: 12, Title: "Second go", URL: "https://github.com/craig/nat/pull/12", State: "OPEN"},
		{Number: 10, Title: "First go", URL: "https://github.com/craig/nat/pull/10", State: "MERGED",
			MergedAt: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)},
	}
	if !reflect.DeepEqual(batch.Heads[first], want) {
		t.Errorf("Heads[first] = %+v, want %+v", batch.Heads[first], want)
	}
	if got, read := batch.Heads[second]; !read || len(got) != 0 {
		t.Errorf("Heads[second] = %+v (read %v), want read and empty", got, read)
	}
	if len(batch.Heads) != 2 {
		t.Errorf("Heads = %+v, want the branch left out of the answer unread", batch.Heads)
	}
}

// TestReadPRsDetail decodes the full-detail selection into the PR pr-view
// prints: what it is, its stats, a commit count, reviews, comments (an
// account since deleted reading as no login) and who is asked to review.
func TestReadPRsDetail(t *testing.T) {
	batch, err := NewWithRunner(&scriptRunner{outs: []string{fixture(t, "graphql-detail.json")}}).
		ReadPRs(BatchQuery{Detail: &natPR})
	if err != nil {
		t.Fatalf("ReadPRs() = %v", err)
	}
	if batch.Detail == nil {
		t.Fatal("Detail = nil, want the pull request in full")
	}
	d := *batch.Detail
	if d.Title != "Read pull requests in one batch" || d.Body != "One document per tick." || d.Author != "craigmjohnston" ||
		!d.IsDraft || d.HeadRefName != "slice/batch" || d.HeadRefOid != "abc123" || d.Additions != 120 ||
		d.Deletions != 40 || d.ChangedFiles != 6 || d.Commits != 3 || d.ReviewDecision != "REVIEW_REQUIRED" {
		t.Errorf("Detail = %+v, want every field read", d)
	}
	if !reflect.DeepEqual(d.ReviewRequests, []string{"octocat", "core"}) {
		t.Errorf("ReviewRequests = %v, want a user by login and a team by slug", d.ReviewRequests)
	}
	wantReviews := []Review{{Author: "reviewer", State: "COMMENTED", Body: "Looks close.",
		SubmittedAt: time.Date(2026, 10, 6, 11, 0, 0, 0, time.UTC)}}
	if !reflect.DeepEqual(d.Reviews, wantReviews) {
		t.Errorf("Reviews = %+v, want %+v", d.Reviews, wantReviews)
	}
	if len(d.Comments) != 2 || d.Comments[0].Body != "Rebased." || d.Comments[1].Author != "" {
		t.Errorf("Comments = %+v, want both, the second with no author", d.Comments)
	}
	if !reflect.DeepEqual(d.Checks, []Check{{Name: "CI / test", State: "SUCCESS",
		URL: "https://github.com/craig/nat/actions/runs/13/job/23"}}) {
		t.Errorf("Checks = %+v, want the one run led by its workflow", d.Checks)
	}
	if len(batch.PRs) != 0 {
		t.Errorf("PRs = %+v, want the detail alone", batch.PRs)
	}
}

// TestReadPRsUnresolvedNodes leaves a node GitHub could not resolve unread —
// and only that node: the rest of the document stands.
func TestReadPRsUnresolvedNodes(t *testing.T) {
	missing := PRRef{Owner: "craig", Repo: "nat", Number: 999}
	gone := PRRef{Owner: "craig", Repo: "gone", Number: 1}
	batch, err := NewWithRunner(&scriptRunner{
		outs: []string{fixture(t, "graphql-errors.json")},
		errs: []error{&ExitError{Code: 1, Stderr: "gh: Could not resolve to a PullRequest\n"}},
	}).ReadPRs(BatchQuery{PRs: []PRRef{missing, natPR, gone}})
	if err != nil {
		t.Fatalf("ReadPRs() = %v, want the resolved nodes read", err)
	}
	if _, read := batch.PRs[missing]; read {
		t.Error("the unresolved pull request was read")
	}
	if _, read := batch.PRs[gone]; read {
		t.Error("the unresolved repository's pull request was read")
	}
	if batch.PRs[natPR].State != "OPEN" {
		t.Errorf("PRs[#7] = %+v, want it read open", batch.PRs[natPR])
	}
}

// TestReadPRsFailedDocument concludes nothing from a document that failed
// whole: a refusal of the budget, a gh that could not run, an answer that is
// no answer.
func TestReadPRsFailedDocument(t *testing.T) {
	for _, tt := range []struct {
		name string
		out  string
		err  error
		want string
	}{
		{"refused", fixture(t, "graphql-refused.json"), &ExitError{Code: 1}, "API rate limit already exceeded"},
		{"gh failed", "", &ExitError{Code: 1, Stderr: "gh: not logged in\n"}, "not logged in"},
		{"unreadable", "not JSON", nil, "no readable answer"},
		{"no data", `{"data": null}`, nil, "no data"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			batch, err := NewWithRunner(&scriptRunner{outs: []string{tt.out}, errs: []error{tt.err}}).
				ReadPRs(BatchQuery{PRs: []PRRef{natPR}, Heads: []HeadRef{{Owner: "craig", Repo: "nat", Branch: "b"}}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("ReadPRs() = %v, want %q", err, tt.want)
			}
			if len(batch.PRs) != 0 || len(batch.Heads) != 0 || batch.Detail != nil || batch.RateLimit != nil {
				t.Errorf("batch = %+v, want nothing read", batch)
			}
		})
	}
}

// TestReadPRsChunks splits a reading at twenty-five things per document, the
// detail first, and keeps what every other document read when one fails.
func TestReadPRsChunks(t *testing.T) {
	var prs []PRRef
	for n := 1; n <= 30; n++ {
		prs = append(prs, PRRef{Owner: "craig", Repo: "nat", Number: n})
	}
	runner := &scriptRunner{
		outs: []string{`{"data":{"r0":null}}`, ""},
		errs: []error{nil, errors.New("network down")},
	}
	_, err := NewWithRunner(runner).ReadPRs(BatchQuery{PRs: prs, Detail: &natPR})
	if err == nil || !strings.Contains(err.Error(), "network down") {
		t.Errorf("ReadPRs() = %v, want the failed document's error", err)
	}
	if len(runner.docs) != 2 {
		t.Fatalf("ran %d documents, want 2", len(runner.docs))
	}
	if !strings.Contains(runner.docs[0], "d: pullRequest(number: 7)") || strings.Count(runner.docs[0], "pullRequest(") != 25 {
		t.Errorf("first document = %s, want the detail and twenty-four more", runner.docs[0])
	}
	if strings.Count(runner.docs[1], "pullRequest(") != 6 {
		t.Errorf("second document = %s, want the last six", runner.docs[1])
	}
}

// TestReadPRsNothingAsked runs no document at all.
func TestReadPRsNothingAsked(t *testing.T) {
	runner := &scriptRunner{outs: []string{""}}
	batch, err := NewWithRunner(runner).ReadPRs(BatchQuery{})
	if err != nil || len(runner.docs) != 0 || batch.PRs == nil || batch.Heads == nil {
		t.Errorf("ReadPRs() = %+v, %v after %d documents, want empty maps and none run", batch, err, len(runner.docs))
	}
}

// TestBatchDocument pins the document: one aliased repository per
// repository, an aliased field per thing asked, every pull request through
// the status fragment, the detail through the detail fragment too, a branch
// name quoted as a GraphQL string — and each fragment only where it is
// spread, since GraphQL refuses an unused one.
func TestBatchDocument(t *testing.T) {
	doc, _ := batchDocument([]batchItem{
		{pr: PRRef{Owner: "craig", Repo: "nat", Number: 3}, detail: true},
		{pr: PRRef{Owner: "craig", Repo: "nat", Number: 7}},
		{pr: PRRef{Owner: "craig", Repo: "brewery", Number: 2}},
		{head: HeadRef{Owner: "craig", Repo: "nat", Branch: `odd"branch`}, isHead: true},
	})
	want := `query {
  rateLimit { limit remaining resetAt }
  r0: repository(owner: "craig", name: "nat") {
    d: pullRequest(number: 3) { ...status ...detail }
    p1: pullRequest(number: 7) { ...status }
    h2: pullRequests(first: 10, headRefName: "odd\"branch", orderBy: {field: CREATED_AT, direction: DESC}) { nodes { number title url state mergedAt } }
  }
  r1: repository(owner: "craig", name: "brewery") {
    p3: pullRequest(number: 2) { ...status }
  }
}
` + statusFragment + detailFragment
	if doc != want {
		t.Errorf("document =\n%s\nwant\n%s", doc, want)
	}

	heads, _ := batchDocument([]batchItem{{head: HeadRef{Owner: "o", Repo: "r", Branch: "b"}, isHead: true}})
	if strings.Contains(heads, "fragment") {
		t.Errorf("a document of branches alone = %s, want no fragment", heads)
	}
}

// TestParsePRURL reads owner, repository and number off a PR URL in any shape
// it is pasted in, and nothing off anything else.
func TestParsePRURL(t *testing.T) {
	for url, want := range map[string]PRRef{
		"https://github.com/Craig/Nat/pull/7":             {Owner: "craig", Repo: "nat", Number: 7},
		"https://github.com/craig/nat/pull/7/files?w=1#x": {},
		"https://github.com/craig/nat/pull/7/":            {Owner: "craig", Repo: "nat", Number: 7},
		"github.test/craig/nat/pull/12":                   {Owner: "craig", Repo: "nat", Number: 12},
		"https://github.com/craig/nat/issues/7":           {},
		"https://github.com/craig/nat/pull/x":             {},
		"https://github.com/craig/nat/pull/0":             {},
		"https://github.com//nat/pull/7":                  {},
		"":                                                {},
	} {
		got, ok := ParsePRURL(url)
		if got != want || ok != (want != PRRef{}) {
			t.Errorf("ParsePRURL(%q) = %+v, %v, want %+v", url, got, ok, want)
		}
	}
}

// TestParseRemote reads the repository off each shape of remote URL.
func TestParseRemote(t *testing.T) {
	for url, want := range map[string][2]string{
		"https://github.com/Craig/Nat.git":   {"craig", "nat"},
		"https://github.com/craig/nat/":      {"craig", "nat"},
		"ssh://git@github.com/craig/nat.git": {"craig", "nat"},
		"git@github.com:craig/nat.git":       {"craig", "nat"},
		"https://github.com":                 {},
		"/local/path/nat":                    {},
		"git@github.com:nat":                 {},
	} {
		owner, repo, ok := ParseRemote(url)
		if owner != want[0] || repo != want[1] || ok != (want[0] != "") {
			t.Errorf("ParseRemote(%q) = %q, %q, %v, want %v", url, owner, repo, ok, want)
		}
	}
}
