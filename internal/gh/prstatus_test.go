package gh

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestPRStatusRunsGh pins the invocation: gh, in the slice's repository, asked
// about the pull request by URL and for the two fields alone.
func TestPRStatusRunsGh(t *testing.T) {
	runner := &fakeRunner{out: `{"mergeable":"MERGEABLE","reviewDecision":"APPROVED"}`}
	status, err := NewWithRunner(runner).PRStatus("/repos/nat", "https://github.test/craig/nat/pull/7")
	if err != nil {
		t.Fatalf("PRStatus() = %v, want a reading", err)
	}
	if !status.Approved || !status.Mergeable {
		t.Errorf("PRStatus() = %+v, want it approved and mergeable", status)
	}
	if runner.dir != "/repos/nat" {
		t.Errorf("ran in %q, want the slice's repository", runner.dir)
	}
	if runner.name != Binary {
		t.Errorf("ran %q, want %q", runner.name, Binary)
	}
	want := []string{"pr", "view", "https://github.test/craig/nat/pull/7", "--json", "reviewDecision,mergeable"}
	if !reflect.DeepEqual(runner.args, want) {
		t.Errorf("args = %v, want %v", runner.args, want)
	}
}

// TestPRStatusReadings walks what GitHub answers with: only the two
// affirmative words count, and every other value it uses — an unreviewed pull
// request, changes asked for, a conflicting merge, a mergeability GitHub is
// still working out — is read as the fact not being true.
func TestPRStatusReadings(t *testing.T) {
	tests := []struct {
		name          string
		out           string
		wantApproved  bool
		wantMergeable bool
	}{
		{name: "approved and mergeable", out: `{"reviewDecision":"APPROVED","mergeable":"MERGEABLE"}`,
			wantApproved: true, wantMergeable: true},
		{name: "review required", out: `{"reviewDecision":"REVIEW_REQUIRED","mergeable":"MERGEABLE"}`,
			wantMergeable: true},
		{name: "changes requested", out: `{"reviewDecision":"CHANGES_REQUESTED","mergeable":"MERGEABLE"}`,
			wantMergeable: true},
		// A repository that requires no review and has had none says nothing at
		// all, which is not an approval.
		{name: "no decision", out: `{"reviewDecision":"","mergeable":"MERGEABLE"}`, wantMergeable: true},
		{name: "conflicting", out: `{"reviewDecision":"APPROVED","mergeable":"CONFLICTING"}`,
			wantApproved: true},
		{name: "mergeability unknown", out: `{"reviewDecision":"APPROVED","mergeable":"UNKNOWN"}`,
			wantApproved: true},
		{name: "nothing said", out: `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{out: tt.out}
			status, err := NewWithRunner(runner).PRStatus("/repos/nat", "https://github.test/pr/7")
			if err != nil {
				t.Fatalf("PRStatus() = %v, want a reading", err)
			}
			if status.Approved != tt.wantApproved || status.Mergeable != tt.wantMergeable {
				t.Errorf("PRStatus() = %+v, want approved=%v mergeable=%v",
					status, tt.wantApproved, tt.wantMergeable)
			}
		})
	}
}

// TestPRStatusFailure passes gh's own refusal straight back — an
// unauthenticated gh, or a pull request that is not there — since the caller's
// answer to it is to leave the slice where it was.
func TestPRStatusFailure(t *testing.T) {
	refusal := &ExitError{Code: 1, Stderr: "gh: Not Found (HTTP 404)\n"}
	runner := &fakeRunner{err: refusal}
	status, err := NewWithRunner(runner).PRStatus("/repos/nat", "https://github.test/pr/7")
	if !errors.Is(err, error(refusal)) {
		t.Errorf("PRStatus() = %v, want gh's own refusal", err)
	}
	if status != (PRStatus{}) {
		t.Errorf("PRStatus() = %+v, want nothing read", status)
	}
}

// TestPRStatusUnreadableJSON covers a gh that exited zero and printed something
// that is not the JSON it was asked for: there is no reading in it, so it is a
// failure here rather than a pull request read as unapproved.
func TestPRStatusUnreadableJSON(t *testing.T) {
	runner := &fakeRunner{out: "not JSON at all\n"}
	_, err := NewWithRunner(runner).PRStatus("/repos/nat", "https://github.test/pr/7")
	if err == nil || !strings.Contains(err.Error(), "no readable JSON") {
		t.Errorf("PRStatus() = %v, want it to report the unreadable output", err)
	}
}
