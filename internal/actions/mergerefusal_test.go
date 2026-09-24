package actions

import (
	"testing"

	"github.com/craigmjohnston/nat/internal/gh"
)

func TestMergeRefusalNothingFailing(t *testing.T) {
	pr := gh.PR{ReviewDecision: "APPROVED", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"}

	reason, refused := MergeRefusal(pr)

	if refused {
		t.Errorf("MergeRefusal() = (%q, true), want not refused", reason)
	}
	if reason != "" {
		t.Errorf("reason = %q, want empty", reason)
	}
}

func TestMergeRefusalReviewFailing(t *testing.T) {
	pr := gh.PR{ReviewDecision: "CHANGES_REQUESTED", Mergeable: "MERGEABLE"}

	reason, refused := MergeRefusal(pr)

	if !refused || reason != "review: changes requested" {
		t.Errorf("MergeRefusal() = (%q, %v), want (%q, true)", reason, refused, "review: changes requested")
	}
}

func TestMergeRefusalChecksFailing(t *testing.T) {
	pr := gh.PR{
		ReviewDecision: "APPROVED",
		Mergeable:      "MERGEABLE",
		Checks: []gh.Check{
			{Name: "test", State: "SUCCESS"},
			{Name: "lint", State: "FAILURE"},
			{Name: "legacy", State: "CANCELLED"},
		},
	}

	reason, refused := MergeRefusal(pr)

	want := "checks: 1 failing · 1 passing · 1 skipped"
	if !refused || reason != want {
		t.Errorf("MergeRefusal() = (%q, %v), want (%q, true)", reason, refused, want)
	}
}

func TestMergeRefusalConflicting(t *testing.T) {
	pr := gh.PR{ReviewDecision: "APPROVED", Mergeable: "CONFLICTING", BaseRefName: "main"}

	reason, refused := MergeRefusal(pr)

	if !refused || reason != "mergeable: conflicting with main" {
		t.Errorf("MergeRefusal() = (%q, %v), want (%q, true)", reason, refused, "mergeable: conflicting with main")
	}
}

func TestMergeRefusalDirtyIsConflicting(t *testing.T) {
	pr := gh.PR{ReviewDecision: "APPROVED", MergeStateStatus: "DIRTY"}

	reason, refused := MergeRefusal(pr)

	if !refused || reason != "mergeable: conflicting with its base" {
		t.Errorf("MergeRefusal() = (%q, %v), want %q with its base named", reason, refused, "mergeable: conflicting")
	}
}

// TestMergeRefusalOrder pins the order the three verdicts are read in: review
// first, so a review that failed is the reason even when the checks and the
// mergeability are failing too.
func TestMergeRefusalOrder(t *testing.T) {
	pr := gh.PR{
		ReviewDecision: "CHANGES_REQUESTED",
		Mergeable:      "CONFLICTING",
		Checks:         []gh.Check{{Name: "lint", State: "FAILURE"}},
	}

	reason, refused := MergeRefusal(pr)

	if !refused || reason != "review: changes requested" {
		t.Errorf("MergeRefusal() = (%q, %v), want the review named first", reason, refused)
	}
}

// The rollup and summary functions fall back to a check with no state at all
// reading as pending, and to their unreachable-in-practice zero case when
// asked about no checks — both worth pinning directly, since neither is
// reached through a pull request MergeRefusal would ever be asked about a
// second time.
func TestCheckOutcomeOfUnknownStateIsPending(t *testing.T) {
	if o := checkOutcomeOf(gh.Check{State: "QUEUED"}); o != mergePending {
		t.Errorf("checkOutcomeOf(QUEUED) = %v, want pending", o)
	}
}

func TestChecksRollupOfNoChecksIsPassing(t *testing.T) {
	if o := checksRollup(nil); o != mergePassing {
		t.Errorf("checksRollup(nil) = %v, want passing", o)
	}
}

func TestChecksSummaryOfNoChecksIsEmpty(t *testing.T) {
	if s := checksSummary(nil); s != "" {
		t.Errorf("checksSummary(nil) = %q, want empty", s)
	}
}

func TestBaseOfEmptyBranch(t *testing.T) {
	if base := baseOf(gh.PR{}); base != "its base" {
		t.Errorf("baseOf({}) = %q, want %q", base, "its base")
	}
}

// TestMergeRefusalMergeStateGate mirrors GitHub's merge button: only CLEAN,
// HAS_HOOKS and UNSTABLE go through; everything else refuses with a named
// reason. internal/tui/prmerge_test.go and macos's PRPresentationTests carry
// the same table.
func TestMergeRefusalMergeStateGate(t *testing.T) {
	pendingChecks := []gh.Check{{Name: "build", State: "IN_PROGRESS"}}
	tests := []struct {
		name string
		pr   gh.PR
		want string // empty: allowed
	}{
		{"clean", gh.PR{MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE"}, ""},
		{"has hooks", gh.PR{MergeStateStatus: "HAS_HOOKS"}, ""},
		{"unstable", gh.PR{MergeStateStatus: "UNSTABLE", Checks: pendingChecks}, ""},
		{"lower case clean", gh.PR{MergeStateStatus: " clean "}, ""},
		{"blocked by pending checks", gh.PR{MergeStateStatus: "BLOCKED", Mergeable: "MERGEABLE", Checks: pendingChecks}, "blocked by checks: 1 pending"},
		{"blocked by review", gh.PR{MergeStateStatus: "BLOCKED", Mergeable: "MERGEABLE", ReviewDecision: "REVIEW_REQUIRED"}, "blocked by review: review required"},
		{"blocked with nothing pending", gh.PR{MergeStateStatus: "BLOCKED", Mergeable: "MERGEABLE"}, "blocked: required checks or reviews are not yet satisfied"},
		{"behind", gh.PR{MergeStateStatus: "BEHIND", BaseRefName: "main"}, "mergeable: behind main"},
		{"dirty", gh.PR{MergeStateStatus: "DIRTY", BaseRefName: "main"}, "mergeable: conflicting with main"},
		{"draft state", gh.PR{MergeStateStatus: "DRAFT"}, "draft: mark the pull request ready for review"},
		{"draft flag", gh.PR{MergeStateStatus: "CLEAN", IsDraft: true}, "draft: mark the pull request ready for review"},
		{"empty", gh.PR{}, "mergeable: mergeability unknown"},
		{"unknown", gh.PR{MergeStateStatus: "UNKNOWN"}, "mergeable: mergeability unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, refused := MergeRefusal(tt.pr)
			if refused != (tt.want != "") || reason != tt.want {
				t.Errorf("MergeRefusal() = (%q, %v), want %q", reason, refused, tt.want)
			}
		})
	}
}

func TestReviewVerdictUnknownDecisionIsPendingInItsOwnWords(t *testing.T) {
	v := reviewVerdict(gh.PR{ReviewDecision: "SOMETHING_NEW"})
	if v.word != "something new" || v.outcome != mergePending {
		t.Errorf("reviewVerdict = %+v, want pending %q", v, "something new")
	}
}
