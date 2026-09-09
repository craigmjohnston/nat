package actions

import (
	"testing"

	"github.com/craigmjohnston/nat/internal/gh"
)

func TestMergeRefusalNothingFailing(t *testing.T) {
	pr := gh.PR{ReviewDecision: "APPROVED", Mergeable: "MERGEABLE"}

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

func TestMergeRefusalReviewRequiredIsNotRefused(t *testing.T) {
	// A review not yet left is a verdict still to come, not a no: GitHub is the
	// one to say whether it takes the merge anyway.
	pr := gh.PR{ReviewDecision: "REVIEW_REQUIRED", Mergeable: "MERGEABLE"}

	if _, refused := MergeRefusal(pr); refused {
		t.Error("MergeRefusal() refused a review still pending, want not refused")
	}
}

func TestMergeRefusalUnknownReviewDecisionIsNotRefused(t *testing.T) {
	pr := gh.PR{ReviewDecision: "SOMETHING_NEW", Mergeable: "MERGEABLE"}

	if _, refused := MergeRefusal(pr); refused {
		t.Error("MergeRefusal() refused an unknown review decision, want not refused")
	}
}

func TestMergeRefusalNoReviewRequiredIsNotRefused(t *testing.T) {
	pr := gh.PR{ReviewDecision: "", Mergeable: "MERGEABLE"}

	if _, refused := MergeRefusal(pr); refused {
		t.Error("MergeRefusal() refused a repository that requires no review, want not refused")
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

func TestMergeRefusalChecksPendingIsNotRefused(t *testing.T) {
	pr := gh.PR{
		ReviewDecision: "APPROVED",
		Mergeable:      "MERGEABLE",
		Checks:         []gh.Check{{Name: "build", State: "IN_PROGRESS"}},
	}

	if _, refused := MergeRefusal(pr); refused {
		t.Error("MergeRefusal() refused checks still running, want not refused")
	}
}

func TestMergeRefusalNoChecksIsNotRefused(t *testing.T) {
	pr := gh.PR{ReviewDecision: "APPROVED", Mergeable: "MERGEABLE"}

	if _, refused := MergeRefusal(pr); refused {
		t.Error("MergeRefusal() refused a pull request with no checks, want not refused")
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

func TestMergeRefusalBehindIsNotRefused(t *testing.T) {
	pr := gh.PR{ReviewDecision: "APPROVED", MergeStateStatus: "BEHIND"}

	if _, refused := MergeRefusal(pr); refused {
		t.Error("MergeRefusal() refused a branch merely behind its base, want not refused")
	}
}

func TestMergeRefusalUnknownMergeabilityIsNotRefused(t *testing.T) {
	pr := gh.PR{ReviewDecision: "APPROVED", Mergeable: "UNKNOWN"}

	if _, refused := MergeRefusal(pr); refused {
		t.Error("MergeRefusal() refused an unsettled mergeability, want not refused")
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
