package gh

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

// TestStatusOfReadings walks what GitHub answers with: only the two affirmative
// words count, and every other value it uses — an unreviewed pull request,
// changes asked for, a conflicting merge, a mergeability GitHub is still
// working out — is read as the fact not being true.
func TestStatusOfReadings(t *testing.T) {
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
			status := statusOf(t, body)
			if status.Approved != tt.wantApproved || status.Mergeable != tt.wantMergeable {
				t.Errorf("StatusOf() = %+v, want approved=%v mergeable=%v",
					status, tt.wantApproved, tt.wantMergeable)
			}
		})
	}
}

// TestStatusOfConflicting walks GitHub's mergeability words, in the shape gh
// pr list prints them, into the one fact: only CONFLICTING, or a DIRTY merge
// state, is a conflict — a mergeability GitHub is still working out is not,
// and neither is one it never said.
func TestStatusOfConflicting(t *testing.T) {
	const url = "https://github.test/pr/7"
	tests := []struct {
		name   string
		fields string
		want   bool
	}{
		{name: "conflicting", fields: `,"mergeable":"CONFLICTING","mergeStateStatus":"DIRTY"`, want: true},
		{name: "dirty alone", fields: `,"mergeable":"UNKNOWN","mergeStateStatus":"dirty"`, want: true},
		{name: "conflicting alone", fields: `,"mergeable":" conflicting ","mergeStateStatus":"UNKNOWN"`, want: true},
		{name: "mergeable", fields: `,"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"`},
		{name: "behind", fields: `,"mergeable":"MERGEABLE","mergeStateStatus":"BEHIND"`},
		{name: "unknown", fields: `,"mergeable":"UNKNOWN","mergeStateStatus":"UNKNOWN"`},
		{name: "nothing said", fields: ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusOf(t, `"url":"`+url+`","baseRefName":" main "`+tt.fields)
			if got.Conflicting != tt.want || got.Base != "main" {
				t.Errorf("StatusOf() = %+v, want conflicting=%v base=main", got, tt.want)
			}
		})
	}
}

// TestStatusOfChecksVerdict walks the rollup into its one verdict: no checks is
// no verdict, any failure fails the lot, anything unfinished or unknown is
// pending, and finished-without-failing — skipped and cancelled included — is
// passing. Both shapes a check arrives in are read, a CheckRun by its
// conclusion once it has completed and by its status until then.
func TestStatusOfChecksVerdict(t *testing.T) {
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
			if got := statusOf(t, body).Checks; got != tt.want {
				t.Errorf("Checks = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestStatusOfFailingChecks names every failed check with its run URL, in
// GitHub's name order, whatever else is pending beside them — a run's
// detailsUrl, a status context's targetUrl — and nothing for a pull request
// that is not red. A run goes by its workflow and its job's own name — the
// last segment of a reusable workflow's caller-job path.
func TestStatusOfFailingChecks(t *testing.T) {
	const red = `"url":"https://github.test/pr/1","statusCheckRollup":[` +
		`{"__typename":"CheckRun","name":"lint","workflowName":"CI","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://github.test/runs/1"},` +
		`{"__typename":"CheckRun","name":"checks / Gate","workflowName":"Pull request","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://github.test/runs/2"},` +
		`{"__typename":"CheckRun","name":"test","status":"IN_PROGRESS"},` +
		`{"__typename":"CheckRun","name":"bare","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://github.test/runs/3"},` +
		`{"__typename":"StatusContext","context":"deploy","state":"ERROR","targetUrl":"https://ci.test/9"},` +
		`{"__typename":"CheckRun","name":"vet","status":"COMPLETED","conclusion":"SUCCESS"}]`
	const green = `"url":"https://github.test/pr/2","statusCheckRollup":[` +
		`{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"SUCCESS"}]`
	got := statusOf(t, red)
	want := []Check{
		{Name: "bare", State: "FAILURE", URL: "https://github.test/runs/3"},
		{Name: "CI / lint", State: "FAILURE", URL: "https://github.test/runs/1"},
		{Name: "deploy", State: "ERROR", URL: "https://ci.test/9"},
		{Name: "Pull request / Gate", State: "FAILURE", URL: "https://github.test/runs/2"},
	}
	if got.Checks != ChecksFailing || !reflect.DeepEqual(got.Failing, want) {
		t.Errorf("red PR = %v %+v, want failing %+v", got.Checks, got.Failing, want)
	}
	if green := statusOf(t, green); green.Failing != nil {
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

// statusOf is StatusOf over the pull request a `gh pr view --json` answer of
// body's fields decodes to — the rollup in the shape gh prints it.
func statusOf(t *testing.T, body string) PRStatus {
	t.Helper()
	pr, err := NewWithRunner(&fakeRunner{out: "{" + body + "}"}).ViewPR("/repos/nat", "7")
	if err != nil {
		t.Fatalf("ViewPR() = %v", err)
	}
	return StatusOf(pr)
}

// TestStatusOfCarriesStateAndMergeTime passes GitHub's lifecycle word and the
// merge's time through as read.
func TestStatusOfCarriesStateAndMergeTime(t *testing.T) {
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	got := StatusOf(PR{State: PRStateMerged, MergedAt: at})
	if got.State != PRStateMerged || !got.MergedAt.Equal(at) {
		t.Errorf("StatusOf() = %+v, want MERGED at %v", got, at)
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
