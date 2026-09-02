package gh

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fixture is one of gh's own answers, kept as a file because the shape being
// decoded is GitHub's rather than this package's and is worth reading whole.
func fixture(t *testing.T, name string) string {
	t.Helper()
	out, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("could not read the fixture: %v", err)
	}
	return string(out)
}

func at(t *testing.T, stamp string) time.Time {
	t.Helper()
	when, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		t.Fatalf("could not read the timestamp: %v", err)
	}
	return when
}

// TestViewPRRunsGh pins the invocation: gh, in the slice's repository, told
// which pull request to read and asked for the viewer's fields alone.
func TestViewPRRunsGh(t *testing.T) {
	runner := &fakeRunner{out: fixture(t, "pr-open.json")}
	if _, err := NewWithRunner(runner).ViewPR("/repos/nat", "slice/read-a-pull-request-through-gh"); err != nil {
		t.Fatalf("ViewPR() = %v, want a pull request", err)
	}
	if runner.dir != "/repos/nat" {
		t.Errorf("ran in %q, want the slice's repository", runner.dir)
	}
	if runner.name != Binary {
		t.Errorf("ran %q, want %q", runner.name, Binary)
	}
	want := []string{"pr", "view", "slice/read-a-pull-request-through-gh", "--json",
		"number,title,body,state,isDraft,author,baseRefName,headRefName,url," +
			"reviewDecision,mergeable,mergeStateStatus,statusCheckRollup,reviews,comments," +
			"additions,deletions,changedFiles,commits"}
	if !reflect.DeepEqual(runner.args, want) {
		t.Errorf("args = %v, want %v", runner.args, want)
	}
}

// The ref goes through as it stands, whichever of the three things gh reads a
// pull request by it is — the URL recorded on a slice's PR property included.
func TestViewPRTakesTheRefAsItStands(t *testing.T) {
	for _, ref := range []string{"7", "slice/read-a-pull-request-through-gh",
		"https://github.com/craigmjohnston/nat/pull/7"} {
		runner := &fakeRunner{out: fixture(t, "pr-open.json")}
		if _, err := NewWithRunner(runner).ViewPR("/repos/nat", ref); err != nil {
			t.Fatalf("ViewPR(%q) = %v, want a pull request", ref, err)
		}
		if got := runner.args[2]; got != ref {
			t.Errorf("asked gh for %q, want %q", got, ref)
		}
	}
}

// TestViewPROpen decodes gh's answer for a pull request still out for review,
// which is every field the viewer draws at once.
func TestViewPROpen(t *testing.T) {
	runner := &fakeRunner{out: fixture(t, "pr-open.json")}
	pr, err := NewWithRunner(runner).ViewPR("/repos/nat", "7")
	if err != nil {
		t.Fatalf("ViewPR() = %v, want a pull request", err)
	}
	want := PR{
		Number:           7,
		Title:            "Read a pull request through gh",
		Body:             "Extends internal/gh with a read of one pull request.\n\nNo TUI in this slice.",
		State:            "OPEN",
		IsDraft:          false,
		Author:           "craigmjohnston",
		BaseRefName:      "main",
		HeadRefName:      "slice/read-a-pull-request-through-gh",
		URL:              "https://github.com/craigmjohnston/nat/pull/7",
		ReviewDecision:   "REVIEW_REQUIRED",
		Mergeable:        "MERGEABLE",
		MergeStateStatus: "BLOCKED",
		Checks: []Check{
			// A finished run is worth its conclusion, one still going its
			// status, and a StatusContext has only ever had the one word.
			{Name: "test", State: "SUCCESS",
				URL: "https://github.com/craigmjohnston/nat/actions/runs/1/job/1"},
			{Name: "lint", State: "IN_PROGRESS",
				URL: "https://github.com/craigmjohnston/nat/actions/runs/1/job/2"},
			{Name: "ci/legacy", State: "PENDING", URL: "https://ci.test/build/1"},
		},
		Reviews: []Review{
			{Author: "reviewer", State: "COMMENTED", Body: "One nit inline, otherwise fine.",
				SubmittedAt: at(t, "2026-08-28T11:30:00Z")},
		},
		Comments: []Comment{
			{Author: "craigmjohnston", Body: "Rebased on main.",
				CreatedAt: at(t, "2026-08-28T09:15:00Z"),
				URL:       "https://github.com/craigmjohnston/nat/pull/7#issuecomment-1"},
			// An account since deleted is named by nobody at all.
			{Author: "", Body: "Looks good from here.",
				CreatedAt: at(t, "2026-08-28T10:00:00Z"),
				URL:       "https://github.com/craigmjohnston/nat/pull/7#issuecomment-2"},
		},
	}
	if !reflect.DeepEqual(pr, want) {
		t.Errorf("ViewPR() = %+v, want %+v", pr, want)
	}
}

// TestViewPRChangeStats covers the four fields gh's numbers pass straight
// through as: additions, deletions and changed files kept as gh reports them,
// and commits — a full object apiece on the wire — collapsed to how many
// there are, since that is all this package ever reads them for.
func TestViewPRChangeStats(t *testing.T) {
	runner := &fakeRunner{out: `{"number":7,"additions":42,"deletions":8,"changedFiles":5,` +
		`"commits":[{"oid":"a"},{"oid":"b"},{"oid":"c"}]}`}
	pr, err := NewWithRunner(runner).ViewPR("/repos/nat", "7")
	if err != nil {
		t.Fatalf("ViewPR() = %v, want a pull request", err)
	}
	if pr.Additions != 42 || pr.Deletions != 8 || pr.ChangedFiles != 5 || pr.Commits != 3 {
		t.Errorf("ViewPR() = %+v, want additions 42, deletions 8, changedFiles 5, commits 3", pr)
	}
}

// TestViewPRMerged is the other end of a slice's life: approved, merged, and
// with no checks and nothing said on it, which is what most of them look like.
func TestViewPRMerged(t *testing.T) {
	runner := &fakeRunner{out: fixture(t, "pr-merged.json")}
	pr, err := NewWithRunner(runner).ViewPR("/repos/nat", "168")
	if err != nil {
		t.Fatalf("ViewPR() = %v, want a pull request", err)
	}
	want := PR{
		Number:           168,
		Title:            "Claim the slice on the board before launching its agent",
		State:            "MERGED",
		Author:           "craigmjohnston",
		BaseRefName:      "main",
		HeadRefName:      "slice/claim-the-slice-on-the-board",
		URL:              "https://github.com/craigmjohnston/nat/pull/168",
		ReviewDecision:   "APPROVED",
		Mergeable:        "UNKNOWN",
		MergeStateStatus: "UNKNOWN",
		Reviews: []Review{
			{Author: "reviewer", State: "APPROVED", SubmittedAt: at(t, "2026-08-27T16:45:00Z")},
		},
	}
	if !reflect.DeepEqual(pr, want) {
		t.Errorf("ViewPR() = %+v, want %+v", pr, want)
	}
}

// TestViewPRChecksFailing is a draft with a failing check, a failing legacy
// status, a conflicting merge and changes asked for — every word GitHub uses
// for something being wrong, kept as GitHub wrote it. Its second review has
// never been submitted, and so carries no time at all.
func TestViewPRChecksFailing(t *testing.T) {
	runner := &fakeRunner{out: fixture(t, "pr-checks-failing.json")}
	pr, err := NewWithRunner(runner).ViewPR("/repos/nat", "9")
	if err != nil {
		t.Fatalf("ViewPR() = %v, want a pull request", err)
	}
	want := PR{
		Number:           9,
		Title:            "Draw a pull request on a screen of its own",
		Body:             "Work in progress.",
		State:            "OPEN",
		IsDraft:          true,
		Author:           "craigmjohnston",
		BaseRefName:      "main",
		HeadRefName:      "slice/draw-a-pull-request",
		URL:              "https://github.com/craigmjohnston/nat/pull/9",
		ReviewDecision:   "CHANGES_REQUESTED",
		Mergeable:        "CONFLICTING",
		MergeStateStatus: "DIRTY",
		Checks: []Check{
			{Name: "test", State: "FAILURE",
				URL: "https://github.com/craigmjohnston/nat/actions/runs/9/job/1"},
			{Name: "ci/legacy", State: "ERROR", URL: "https://ci.test/build/9"},
		},
		Reviews: []Review{
			{Author: "reviewer", State: "CHANGES_REQUESTED", Body: "The gate is failing.",
				SubmittedAt: at(t, "2026-08-29T08:00:00Z")},
			{Author: "reviewer", State: "PENDING", Body: "Still reading."},
		},
	}
	if !reflect.DeepEqual(pr, want) {
		t.Errorf("ViewPR() = %+v, want %+v", pr, want)
	}
}

// A rollup entry of a kind GitHub has added since is drawn as well as it can
// be rather than dropped: whichever of the two namings it carries is its name.
func TestViewPRUnknownCheckKind(t *testing.T) {
	runner := &fakeRunner{out: `{"statusCheckRollup":[` +
		`{"__typename":"Later","name":"newfangled","status":"QUEUED","detailsUrl":"https://gh.test/1"},` +
		`{"__typename":"Later","context":"old/style","state":"SUCCESS","targetUrl":"https://ci.test/2"}` +
		`]}`}
	pr, err := NewWithRunner(runner).ViewPR("/repos/nat", "7")
	if err != nil {
		t.Fatalf("ViewPR() = %v, want a pull request", err)
	}
	want := []Check{
		{Name: "newfangled", State: "QUEUED", URL: "https://gh.test/1"},
		{Name: "old/style", State: "SUCCESS", URL: "https://ci.test/2"},
	}
	if !reflect.DeepEqual(pr.Checks, want) {
		t.Errorf("checks = %+v, want %+v", pr.Checks, want)
	}
}

// TestViewPRNeedsARef refuses before gh is run at all: gh with no pull request
// named reads whichever one the directory's current branch has, and for a
// shared checkout that is nobody's slice in particular.
func TestViewPRNeedsARef(t *testing.T) {
	runner := &fakeRunner{}
	_, err := NewWithRunner(runner).ViewPR("/repos/nat", "")
	if err == nil || !strings.Contains(err.Error(), "needs a pull request") {
		t.Errorf("ViewPR() = %v, want it to refuse an unnamed pull request", err)
	}
	if runner.runs != 0 {
		t.Errorf("ran gh %d times, want it not run at all", runner.runs)
	}
}

// TestViewPRFailure hands gh's own words back — "no pull requests found for
// branch X" is the sentence the caller has to show.
func TestViewPRFailure(t *testing.T) {
	refusal := &ExitError{Code: 1, Stderr: "no pull requests found for branch \"slice/x\"\n"}
	runner := &fakeRunner{err: refusal}
	pr, err := NewWithRunner(runner).ViewPR("/repos/nat", "slice/x")
	if !errors.Is(err, error(refusal)) {
		t.Errorf("ViewPR() = %v, want gh's own refusal", err)
	}
	if err.Error() != `no pull requests found for branch "slice/x"` {
		t.Errorf("ViewPR() = %q, want gh's first line", err.Error())
	}
	if !reflect.DeepEqual(pr, PR{}) {
		t.Errorf("ViewPR() = %+v, want nothing read", pr)
	}
}

// A gh that exited zero and printed something that is not the JSON it was
// asked for is a failure rather than an empty pull request.
func TestViewPRUnreadableJSON(t *testing.T) {
	runner := &fakeRunner{out: "not JSON at all\n"}
	_, err := NewWithRunner(runner).ViewPR("/repos/nat", "7")
	if err == nil || !strings.Contains(err.Error(), "no readable JSON") {
		t.Errorf("ViewPR() = %v, want it to report the unreadable output", err)
	}
}
