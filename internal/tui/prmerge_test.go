package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/gh"
)

// The four pull requests the merge box is drawn from: one everything stands on,
// one whose branch conflicts with its base, one whose checks failed, and one a
// reviewer has asked for changes on — which are the four readings the section
// exists to tell apart.
func cleanPR() gh.PR {
	pr := checkedPR(passingChecks...)
	pr.ReviewDecision = "APPROVED"
	pr.Mergeable = "MERGEABLE"
	pr.MergeStateStatus = "CLEAN"
	return pr
}

func conflictingPR() gh.PR {
	pr := cleanPR()
	pr.Mergeable = "CONFLICTING"
	pr.MergeStateStatus = "DIRTY"
	return pr
}

func checksFailingPR() gh.PR {
	pr := cleanPR()
	pr.Checks = failingChecks
	pr.MergeStateStatus = "UNSTABLE"
	return pr
}

func changesRequestedPR() gh.PR {
	pr := cleanPR()
	pr.ReviewDecision = "CHANGES_REQUESTED"
	pr.MergeStateStatus = "BLOCKED"
	return pr
}

// TestMergeReviewVerdict covers each of GitHub's review decisions, the empty
// one of a repository that requires no review, and a word this build does not
// know — which is a verdict still to come rather than one that stands.
func TestMergeReviewVerdict(t *testing.T) {
	tests := []struct {
		decision string
		word     string
		outcome  checkOutcome
	}{
		{"APPROVED", "approved", checkPassing},
		{"CHANGES_REQUESTED", "changes requested", checkFailing},
		{"REVIEW_REQUIRED", "review required", checkPending},
		{"", "no review required", checkSkipped},
		{"   ", "no review required", checkSkipped},
		{" approved ", "approved", checkPassing},
		{"SECOND_OPINION", "second opinion", checkPending},
	}
	for _, tt := range tests {
		t.Run(tt.decision, func(t *testing.T) {
			got := reviewVerdict(gh.PR{ReviewDecision: tt.decision})
			if got.label != "review" || got.word != tt.word || got.outcome != tt.outcome {
				t.Errorf("verdict for %q = %+v, want %q/%v", tt.decision, got, tt.word, tt.outcome)
			}
		})
	}
}

// TestMergeChecksVerdict covers the checks line: the very rollup the checks
// section draws its own heading from, and a pull request nothing runs on saying
// so rather than reading as a pass.
func TestMergeChecksVerdict(t *testing.T) {
	tests := []struct {
		name    string
		checks  []gh.Check
		word    string
		outcome checkOutcome
	}{
		{"all passing", passingChecks, "3 passing", checkPassing},
		{"a failure among them", failingChecks, "2 failing · 1 passing · 1 skipped", checkFailing},
		{"still going", pendingChecks, "2 pending · 1 passing", checkPending},
		{"none at all", nil, "no checks", checkSkipped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checksVerdict(gh.PR{Checks: tt.checks})
			if got.label != "checks" || got.word != tt.word || got.outcome != tt.outcome {
				t.Errorf("verdict = %+v, want %q/%v", got, tt.word, tt.outcome)
			}
		})
	}
}

// TestMergeMergeableVerdict covers the branch itself: a conflict named as one
// however GitHub reports it, a branch merely behind its base named as behind,
// and everything GitHub has not settled read as a verdict still to come.
func TestMergeMergeableVerdict(t *testing.T) {
	tests := []struct {
		name      string
		mergeable string
		state     string
		word      string
		outcome   checkOutcome
	}{
		{"clean", "MERGEABLE", "CLEAN", "no conflicts with main", checkPassing},
		{"blocked on a review", "MERGEABLE", "BLOCKED", "no conflicts with main", checkPassing},
		{"conflicting", "CONFLICTING", "DIRTY", "conflicting with main", checkFailing},
		{"conflicting flag alone", "CONFLICTING", "", "conflicting with main", checkFailing},
		{"dirty state alone", "UNKNOWN", "DIRTY", "conflicting with main", checkFailing},
		{"behind", "MERGEABLE", "BEHIND", "behind main", checkPending},
		{"not computed yet", "UNKNOWN", "UNKNOWN", "mergeability unknown", checkPending},
		{"nothing said at all", "", "", "mergeability unknown", checkPending},
		{"a word this build does not know", "PERHAPS", "SOMEHOW", "mergeability unknown", checkPending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeableVerdict(gh.PR{
				BaseRefName:      "main",
				Mergeable:        tt.mergeable,
				MergeStateStatus: tt.state,
			})
			if got.label != "mergeable" || got.word != tt.word || got.outcome != tt.outcome {
				t.Errorf("verdict = %+v, want %q/%v", got, tt.word, tt.outcome)
			}
		})
	}
	// A pull request read with no base branch is still worth a verdict, named in
	// words rather than by a branch nobody can see.
	if got := mergeableVerdict(gh.PR{Mergeable: "MERGEABLE"}).word; got != "no conflicts with its base" {
		t.Errorf("verdict with no base = %q", got)
	}
}

// TestMergeRollupAndSummary covers the heading: the worst of the three
// verdicts, and the answer it gives to the question the section asks.
func TestMergeRollupAndSummary(t *testing.T) {
	tests := []struct {
		name    string
		pr      gh.PR
		rollup  checkOutcome
		summary string
	}{
		{"clean", cleanPR(), checkPassing, "ready to merge"},
		{"conflicting", conflictingPR(), checkFailing, "cannot merge"},
		{"checks failing", checksFailingPR(), checkFailing, "cannot merge"},
		{"changes requested", changesRequestedPR(), checkFailing, "cannot merge"},
		{"nothing settled", samplePR(), checkPending, "not ready to merge"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rollup := mergeRollup(mergeVerdicts(tt.pr))
			if rollup != tt.rollup {
				t.Errorf("rollup = %v, want %v", rollup, tt.rollup)
			}
			if got := mergeSummary(rollup); got != tt.summary {
				t.Errorf("summary = %q, want %q", got, tt.summary)
			}
		})
	}
	// A pull request whose every verdict was never asked — no review required and
	// nothing to run — is one nothing stands in the way of.
	skipped := []mergeVerdict{{outcome: checkSkipped}}
	if got := mergeRollup(skipped); got != checkSkipped {
		t.Errorf("rollup of skipped verdicts = %v, want %v", got, checkSkipped)
	}
	if got := mergeSummary(checkSkipped); got != "ready to merge" {
		t.Errorf("summary of skipped = %q", got)
	}
	// Unreachable through mergeVerdicts, which always answers with three: asked
	// about none, the section is in the outcome nothing is wrong in.
	if got := mergeRollup(nil); got != checkPassing {
		t.Errorf("rollup of no verdicts = %v, want %v", got, checkPassing)
	}
}

// TestMergeSectionRender pins the section on each of the four readings it
// exists to tell apart, plus the two endings that replace it.
func TestMergeSectionRender(t *testing.T) {
	merged, closed := cleanPR(), cleanPR()
	merged.State = gh.PRStateMerged
	closed.State = gh.PRStateClosed
	for _, tt := range []struct {
		name string
		pr   gh.PR
	}{
		{"pr-merge-clean", cleanPR()},
		{"pr-merge-conflicting", conflictingPR()},
		{"pr-merge-checks-failing", checksFailingPR()},
		{"pr-merge-changes-requested", changesRequestedPR()},
		{"pr-merge-merged", merged},
		{"pr-merge-closed", closed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			golden(t, tt.name, readyPRView(tt.pr).mergeSection())
		})
	}
}

// TestMergeSectionEndings covers a pull request that has already merged or been
// closed: the verdicts are replaced by that ending, since there is nothing left
// to weigh up.
func TestMergeSectionEndings(t *testing.T) {
	for _, tt := range []struct{ state, want string }{
		{gh.PRStateMerged, "merged into main"},
		{gh.PRStateClosed, "closed without merging"},
	} {
		t.Run(tt.state, func(t *testing.T) {
			pr := cleanPR()
			pr.State = tt.state
			section := readyPRView(pr).mergeSection()
			if !strings.Contains(section, tt.want) {
				t.Errorf("section = %q, want %q in it", section, tt.want)
			}
			if strings.Contains(section, "\n") {
				t.Errorf("section = %q, want one line", section)
			}
			for _, gone := range []string{"approved", "3 passing", "no conflicts"} {
				if strings.Contains(section, gone) {
					t.Errorf("section = %q, want no verdict %q on it", section, gone)
				}
			}
		})
	}
	// A merged pull request read with no base branch still names the ending.
	merged := gh.PR{State: gh.PRStateMerged}
	if got := readyPRView(merged).mergeSection(); !strings.Contains(got, "merged into its base") {
		t.Errorf("section = %q, want the base named in words", got)
	}
}

// TestMergeSectionInTheBody covers where the section sits: under the checks, in
// the viewport with them, so the conclusion is read after the rows it is drawn
// from.
func TestMergeSectionInTheBody(t *testing.T) {
	body := readyPRView(conflictingPR()).vp.GetContent()
	checks := strings.Index(body, "Checks")
	merge := strings.Index(body, "Merge")
	if checks < 0 || merge < 0 {
		t.Fatalf("body = %q, want the checks and the merge box in it", body)
	}
	if merge < checks {
		t.Errorf("body = %q, want the merge box under the checks", body)
	}
	for _, want := range []string{"cannot merge", "conflicting with main", "approved"} {
		if !strings.Contains(body, want) {
			t.Errorf("body = %q, want %q in it", body, want)
		}
	}
}

// TestMergeSectionFitsTheWidth covers a narrow screen: a verdict about a branch
// named longer than the window is cut to it rather than wrapping onto a line
// the layout has not left room for.
func TestMergeSectionFitsTheWidth(t *testing.T) {
	pr := conflictingPR()
	pr.BaseRefName = strings.Repeat("a-very-long-base-branch-name", 4)
	p := readyPRView(pr)
	p.SetSize(40, 20)
	for _, line := range strings.Split(p.mergeSection(), "\n") {
		if got := lipgloss.Width(line); got > 40 {
			t.Errorf("line %q is %d columns, want no more than 40", line, got)
		}
	}
}
