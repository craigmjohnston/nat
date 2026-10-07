package gh

import (
	"strings"
	"time"
)

// GitHub's own words for the two facts that decide whether a pull request is
// still waiting on anyone. reviewDecision is empty on a repository that
// requires no review and has had none, and otherwise one of APPROVED,
// CHANGES_REQUESTED or REVIEW_REQUIRED; mergeable is MERGEABLE, CONFLICTING or
// UNKNOWN, the last being GitHub still working the merge out. Only the two
// affirmative ones mean anything here — everything else is a pull request that
// is not ready, which is what an unread one is taken as too.
//
// A conflict is the one negative said positively: mergeable CONFLICTING, or a
// merge state of DIRTY — the words the merge refusal reads. UNKNOWN is not
// one, nor is a mergeability never read.
const (
	reviewApproved   = "APPROVED"
	stateMergeable   = "MERGEABLE"
	stateConflicting = "CONFLICTING"
	mergeStateDirty  = "DIRTY"
)

// PRStatus is what gh says about a pull request that bears on whether it is
// still waiting to be reviewed: whether a review has approved it, whether
// GitHub can merge it as it stands, and how its checks stand. Both false is a
// pull request with a review still to come — and equally the zero value, which
// is what a read that never happened comes back as.
//
// Conflicting is GitHub positively saying the branch conflicts with Base —
// which Mergeable false cannot say, since a mergeability still being worked
// out is false too.
//
// Failing is every check the rollup has failed, in the order gh listed them —
// empty unless Checks is [ChecksFailing]. Its run URLs are what tells one red
// reading from the next: a re-push that fails again fails in a new run.
//
// All is every check the verdict was rolled up from, in the order gh listed
// them — GitHub's own, workflow-led names, as `pr-view` and `slice-checks`
// print them — each with the raw state [Check.Outcome] reads.
//
// State is GitHub's word for where the pull request is — OPEN, MERGED or
// CLOSED — and MergedAt when it merged, zero for one that has not.
type PRStatus struct {
	Approved    bool
	Mergeable   bool
	Conflicting bool
	Base        string
	Checks      ChecksVerdict
	Failing     []Check
	All         []Check
	State       string
	MergedAt    time.Time
}

// StatusOf is what a pull request's reading says about whether it is still
// waiting on anyone. Only APPROVED and MERGEABLE count as true; every other
// GitHub word is "not true".
func StatusOf(pr PR) PRStatus {
	checks, failing := Verdict(pr.Checks)
	return PRStatus{
		Approved:    pr.ReviewDecision == reviewApproved,
		Mergeable:   pr.Mergeable == stateMergeable,
		Conflicting: conflicting(pr.Mergeable, pr.MergeStateStatus),
		Base:        strings.TrimSpace(pr.BaseRefName),
		Checks:      checks,
		Failing:     failing,
		All:         pr.Checks,
		State:       pr.State,
		MergedAt:    pr.MergedAt,
	}
}

// ChecksVerdict is a pull request's whole status check rollup said as one word.
// The zero value is a pull request with no checks at all, which is no verdict
// rather than a pass: nothing ran, so nothing can be said to have gone green.
type ChecksVerdict int

const (
	// ChecksNone is a pull request that reported no checks.
	ChecksNone ChecksVerdict = iota
	// ChecksPassing is every check finished and none of them failed.
	ChecksPassing
	// ChecksPending is no check failed but at least one has not finished — or
	// is in a state this build does not know, which is read the same way.
	ChecksPending
	// ChecksFailing is at least one check failed, whatever the rest are doing.
	ChecksFailing
)

// String names the verdict for logs and test failures.
func (v ChecksVerdict) String() string {
	switch v {
	case ChecksPassing:
		return "passing"
	case ChecksPending:
		return "pending"
	case ChecksFailing:
		return "failing"
	default:
		return "none"
	}
}

// CheckOutcome is what one status check amounts to for a reader: it is still
// going, it passed, it failed, or it finished without counting either way.
// GitHub has a word for every way each of those happens — a run that timed out
// and one that failed outright are two words for the same news — and this is
// the one place those words are classified, so the board's verdict, the pull
// request screen and the merge refusal can never disagree about whether CI is
// red.
//
// The zero value is pending, which is also what a state nobody can classify
// reads as.
type CheckOutcome int

const (
	// CheckPending is a check not finished — QUEUED, IN_PROGRESS, PENDING,
	// WAITING, REQUESTED, the EXPECTED of a status nothing has reported yet —
	// or one in a state this build does not know: a check nobody can classify
	// is exactly the check to keep watching, and calling it a pass would have
	// work called ready that nothing said was.
	CheckPending CheckOutcome = iota
	// CheckPassing is a check that finished and succeeded.
	CheckPassing
	// CheckFailing is a check that finished and failed, in any of GitHub's
	// words for it.
	CheckFailing
	// CheckSkipped is a check that finished without counting either way:
	// skipped, neutral, cancelled or stale. It holds nothing up.
	CheckSkipped
)

// checkOutcomes is every finished state a check arrives in — a CheckRun's
// conclusion or a StatusContext's state, as [ghRoll.check] reduces them to the
// one field — and what it amounts to. Everything not named here is pending.
var checkOutcomes = map[string]CheckOutcome{
	"SUCCESS":         CheckPassing,
	"FAILURE":         CheckFailing,
	"ERROR":           CheckFailing,
	"TIMED_OUT":       CheckFailing,
	"STARTUP_FAILURE": CheckFailing,
	"ACTION_REQUIRED": CheckFailing,
	"SKIPPED":         CheckSkipped,
	"NEUTRAL":         CheckSkipped,
	"CANCELLED":       CheckSkipped,
	"STALE":           CheckSkipped,
}

// Outcome is where the check stands, read off its state whatever its case or
// spacing.
func (c Check) Outcome() CheckOutcome {
	return checkOutcomes[strings.ToUpper(strings.TrimSpace(c.State))]
}

// Verdict rolls a pull request's checks into one verdict: any failure fails
// the lot, then any check unfinished leaves it pending. The checks that failed
// come back with it, every one of them, in the order they were given. It is
// the one roll-up, so the board's listing and a single pull request's view —
// `nat slice-checks` reads [PR.Checks] through it — can never disagree.
func Verdict(checks []Check) (ChecksVerdict, []Check) {
	if len(checks) == 0 {
		return ChecksNone, nil
	}
	verdict := ChecksPassing
	var failing []Check
	for _, check := range checks {
		switch check.Outcome() {
		case CheckFailing:
			verdict = ChecksFailing
			failing = append(failing, check)
		case CheckPending:
			if verdict != ChecksFailing {
				verdict = ChecksPending
			}
		}
	}
	return verdict, failing
}

// conflicting reports whether GitHub positively said a pull request's branch
// conflicts with its base, whatever the case or spacing of its words.
func conflicting(mergeable, mergeState string) bool {
	return strings.ToUpper(strings.TrimSpace(mergeable)) == stateConflicting ||
		strings.ToUpper(strings.TrimSpace(mergeState)) == mergeStateDirty
}

// NormaliseURL is a pull request URL as the listing is keyed by it, so a URL
// typed onto a Notion page matches the canonical one gh prints. A query string
// or a fragment is whatever the link was copied from — a review, a file, a
// comment — and names the same pull request, a trailing slash is nothing at
// all, and the case of an owner or a repository is not a distinction GitHub
// makes.
func NormaliseURL(url string) string {
	url, _, _ = strings.Cut(url, "?")
	url, _, _ = strings.Cut(url, "#")
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(url), "/"))
}
