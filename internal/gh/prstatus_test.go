package gh

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// TestOpenPRsRunsGh pins the invocation: gh, in the slice's repository, asked
// for the open pull requests alone and for the four fields alone, with a limit
// past gh's own default.
func TestOpenPRsRunsGh(t *testing.T) {
	runner := &fakeRunner{out: `[{"url":"https://github.test/craig/nat/pull/7",` +
		`"reviewDecision":"APPROVED","mergeable":"MERGEABLE"}]`}
	open, err := NewWithRunner(runner).OpenPRs("/repos/nat")
	if err != nil {
		t.Fatalf("OpenPRs() = %v, want a listing", err)
	}
	status, listed := open["https://github.test/craig/nat/pull/7"]
	if !listed {
		t.Fatalf("OpenPRs() = %+v, want the pull request keyed by its URL", open)
	}
	if !status.Approved || !status.Mergeable {
		t.Errorf("OpenPRs() = %+v, want it approved and mergeable", status)
	}
	if runner.dir != "/repos/nat" {
		t.Errorf("ran in %q, want the slice's repository", runner.dir)
	}
	if runner.name != Binary {
		t.Errorf("ran %q, want %q", runner.name, Binary)
	}
	want := []string{"pr", "list", "--state", "open", "--json", "url,reviewDecision,mergeable,statusCheckRollup",
		"--limit", "100"}
	if !reflect.DeepEqual(runner.args, want) {
		t.Errorf("args = %v, want %v", runner.args, want)
	}
}

// TestOpenPRsReadings walks what GitHub answers with: only the two affirmative
// words count, and every other value it uses — an unreviewed pull request,
// changes asked for, a conflicting merge, a mergeability GitHub is still
// working out — is read as the fact not being true.
func TestOpenPRsReadings(t *testing.T) {
	const url = "https://github.test/pr/7"
	tests := []struct {
		name          string
		fields        string
		wantApproved  bool
		wantMergeable bool
	}{
		{name: "approved and mergeable", fields: `"reviewDecision":"APPROVED","mergeable":"MERGEABLE"`,
			wantApproved: true, wantMergeable: true},
		// A repository that requires no review and has had none says nothing at
		// all, which is not an approval. Every other word gh can say instead of
		// APPROVED or MERGEABLE (REVIEW_REQUIRED, CHANGES_REQUESTED,
		// CONFLICTING, UNKNOWN, ...) reads as false the same way an empty
		// string does — the comparison is a plain equality, not a lookup with
		// its own case per word.
		{name: "no decision", fields: `"reviewDecision":"","mergeable":"MERGEABLE"`, wantMergeable: true},
		{name: "conflicting", fields: `"reviewDecision":"APPROVED","mergeable":"CONFLICTING"`,
			wantApproved: true},
		{name: "nothing said", fields: ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `"url":"` + url + `"`
			if tt.fields != "" {
				body += "," + tt.fields
			}
			runner := &fakeRunner{out: "[{" + body + "}]"}
			open, err := NewWithRunner(runner).OpenPRs("/repos/nat")
			if err != nil {
				t.Fatalf("OpenPRs() = %v, want a listing", err)
			}
			status := open[url]
			if status.Approved != tt.wantApproved || status.Mergeable != tt.wantMergeable {
				t.Errorf("OpenPRs() = %+v, want approved=%v mergeable=%v",
					status, tt.wantApproved, tt.wantMergeable)
			}
		})
	}
}

// TestOpenPRsChecksVerdict walks the rollup into its one verdict: no checks is
// no verdict, any failure fails the lot, anything unfinished or unknown is
// pending, and finished-without-failing — skipped and cancelled included — is
// passing. Both shapes a check arrives in are read, a CheckRun by its
// conclusion once it has completed and by its status until then.
func TestOpenPRsChecksVerdict(t *testing.T) {
	const url = "https://github.test/pr/7"
	const (
		run       = `{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"%s"}`
		running   = `{"__typename":"CheckRun","name":"test","status":"IN_PROGRESS","conclusion":""}`
		ctxStatus = `{"__typename":"StatusContext","context":"ci","state":"%s"}`
	)
	tests := []struct {
		name   string
		rollup string
		want   ChecksVerdict
	}{
		{name: "no rollup at all", rollup: ``, want: ChecksNone},
		{name: "an empty rollup", rollup: `[]`, want: ChecksNone},
		{name: "all green", rollup: `[` + fmt.Sprintf(run, "SUCCESS") + `,` +
			fmt.Sprintf(ctxStatus, "SUCCESS") + `]`, want: ChecksPassing},
		{name: "skipped and cancelled hold nothing up", rollup: `[` + fmt.Sprintf(run, "SKIPPED") + `,` +
			fmt.Sprintf(run, "cancelled") + `,` + fmt.Sprintf(run, "SUCCESS") + `]`, want: ChecksPassing},
		{name: "one still running", rollup: `[` + fmt.Sprintf(run, "SUCCESS") + `,` + running + `]`,
			want: ChecksPending},
		{name: "an unknown state reads as pending", rollup: `[` + fmt.Sprintf(ctxStatus, "SOMETHING_NEW") + `]`,
			want: ChecksPending},
		{name: "a failure beats a pending check", rollup: `[` + running + `,` + fmt.Sprintf(run, "FAILURE") + `]`,
			want: ChecksFailing},
		{name: "a failed status context", rollup: `[` + fmt.Sprintf(ctxStatus, "ERROR") + `]`,
			want: ChecksFailing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `"url":"` + url + `"`
			if tt.rollup != "" {
				body += `,"statusCheckRollup":` + tt.rollup
			}
			open, err := NewWithRunner(&fakeRunner{out: "[{" + body + "}]"}).OpenPRs("/repos/nat")
			if err != nil {
				t.Fatalf("OpenPRs() = %v, want a listing", err)
			}
			if got := open[url].Checks; got != tt.want {
				t.Errorf("Checks = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestOpenPRsFailingChecks names every failed check with its run URL, in the
// rollup's order, whatever else is pending beside them — a run's detailsUrl, a
// status context's targetUrl — and nothing for a pull request that is not red.
// A run is named under its workflow where it names one, bare where it does not.
func TestOpenPRsFailingChecks(t *testing.T) {
	const out = `[{"url":"https://github.test/pr/1","statusCheckRollup":[` +
		`{"__typename":"CheckRun","name":"lint","workflowName":"CI","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://github.test/runs/1"},` +
		`{"__typename":"CheckRun","name":"test","status":"IN_PROGRESS"},` +
		`{"__typename":"CheckRun","name":"bare","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://github.test/runs/3"},` +
		`{"__typename":"StatusContext","context":"deploy","state":"ERROR","targetUrl":"https://ci.test/9"},` +
		`{"__typename":"CheckRun","name":"vet","status":"COMPLETED","conclusion":"SUCCESS"}]},` +
		`{"url":"https://github.test/pr/2","statusCheckRollup":[` +
		`{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"SUCCESS"}]}]`
	open, err := NewWithRunner(&fakeRunner{out: out}).OpenPRs("/repos/nat")
	if err != nil {
		t.Fatalf("OpenPRs() = %v, want a listing", err)
	}
	red := open["https://github.test/pr/1"]
	want := []Check{
		{Name: "CI / lint", State: "FAILURE", URL: "https://github.test/runs/1"},
		{Name: "bare", State: "FAILURE", URL: "https://github.test/runs/3"},
		{Name: "deploy", State: "ERROR", URL: "https://ci.test/9"},
	}
	if red.Checks != ChecksFailing || !reflect.DeepEqual(red.Failing, want) {
		t.Errorf("red PR = %v %+v, want failing %+v", red.Checks, red.Failing, want)
	}
	if green := open["https://github.test/pr/2"]; green.Failing != nil {
		t.Errorf("green PR Failing = %+v, want none", green.Failing)
	}
}

// TestCheckOutcome is the one table of GitHub's check words: every finished
// word, read whatever its case or spacing, and anything else — an unfinished
// run, an empty state, a word GitHub adds later — as pending.
func TestCheckOutcome(t *testing.T) {
	for state, want := range map[string]CheckOutcome{
		"SUCCESS": CheckPassing, " success ": CheckPassing,
		"FAILURE": CheckFailing, "ERROR": CheckFailing, "TIMED_OUT": CheckFailing,
		"STARTUP_FAILURE": CheckFailing, "ACTION_REQUIRED": CheckFailing,
		"SKIPPED": CheckSkipped, "NEUTRAL": CheckSkipped, "CANCELLED": CheckSkipped, "STALE": CheckSkipped,
		"IN_PROGRESS": CheckPending, "QUEUED": CheckPending, "": CheckPending, "SOMETHING_NEW": CheckPending,
	} {
		if got := (Check{State: state}).Outcome(); got != want {
			t.Errorf("Check{State: %q}.Outcome() = %d, want %d", state, got, want)
		}
	}
}

// TestChecksVerdictString names every verdict, the zero value as none.
func TestChecksVerdictString(t *testing.T) {
	for v, want := range map[ChecksVerdict]string{
		ChecksNone: "none", ChecksPassing: "passing", ChecksPending: "pending", ChecksFailing: "failing",
	} {
		if got := v.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", int(v), got, want)
		}
	}
}

// A pull request that is not open is simply not in the listing, which is the
// whole fact the board reads off it: an empty listing names nothing, and no
// pull request is taken for open on the strength of nothing.
func TestOpenPRsListsOnlyWhatIsOpen(t *testing.T) {
	runner := &fakeRunner{out: "[]\n"}
	open, err := NewWithRunner(runner).OpenPRs("/repos/nat")
	if err != nil {
		t.Fatalf("OpenPRs() = %v, want a listing", err)
	}
	if len(open) != 0 {
		t.Errorf("OpenPRs() = %+v, want nothing named", open)
	}
}

// The listing is keyed the way a URL off a Notion page is looked up, so a link
// copied from a review page or typed with a trailing slash still finds it.
func TestOpenPRsKeysNormalisedURLs(t *testing.T) {
	runner := &fakeRunner{out: `[{"url":"https://github.test/Craig/Nat/pull/7/","mergeable":"MERGEABLE"}]`}
	open, err := NewWithRunner(runner).OpenPRs("/repos/nat")
	if err != nil {
		t.Fatalf("OpenPRs() = %v, want a listing", err)
	}
	if _, listed := open[NormaliseURL("https://github.test/craig/nat/pull/7?w=1")]; !listed {
		t.Errorf("OpenPRs() = %+v, want the URL keyed as it is looked up", open)
	}
}

// TestNormaliseURL walks what the same pull request can be written as.
func TestNormaliseURL(t *testing.T) {
	const want = "https://github.test/craig/nat/pull/7"
	tests := []string{
		"https://github.test/craig/nat/pull/7",
		"https://github.test/craig/nat/pull/7/",
		"  https://github.test/craig/nat/pull/7  ",
		"https://github.test/craig/nat/pull/7?w=1",
		"https://github.test/craig/nat/pull/7#issuecomment-1",
		"https://github.test/Craig/Nat/pull/7",
	}
	for _, url := range tests {
		if got := NormaliseURL(url); got != want {
			t.Errorf("NormaliseURL(%q) = %q, want %q", url, got, want)
		}
	}
}

// TestOpenPRsFailure passes gh's own refusal straight back — an unauthenticated
// gh, or a directory that is no repository — since the caller's answer to it is
// to conclude nothing at all.
func TestOpenPRsFailure(t *testing.T) {
	refusal := &ExitError{Code: 1, Stderr: "gh: Not Found (HTTP 404)\n"}
	runner := &fakeRunner{err: refusal}
	open, err := NewWithRunner(runner).OpenPRs("/repos/nat")
	if !errors.Is(err, error(refusal)) {
		t.Errorf("OpenPRs() = %v, want gh's own refusal", err)
	}
	if open != nil {
		t.Errorf("OpenPRs() = %+v, want nothing read", open)
	}
}

// TestOpenPRsUnreadableJSON covers a gh that exited zero and printed something
// that is not the JSON it was asked for: there is no listing in it, so it is a
// failure here rather than a repository read as having nothing open.
func TestOpenPRsUnreadableJSON(t *testing.T) {
	runner := &fakeRunner{out: "not JSON at all\n"}
	_, err := NewWithRunner(runner).OpenPRs("/repos/nat")
	if err == nil || !strings.Contains(err.Error(), "no readable JSON") {
		t.Errorf("OpenPRs() = %v, want it to report the unreadable output", err)
	}
}
