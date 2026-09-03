package actions

import (
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/gh"
)

// mergeOutcome is what one fact bearing on a merge amounts to: it stands, it
// stands against, it is not settled yet, or it was never asked. It is the pure
// half of internal/tui/prmerge.go's checkOutcome — the same four-way read of a
// review, a checks rollup and a mergeability, with no styling on it, since
// MergeRefusal below only ever needs to know which of the three is failing.
type mergeOutcome int

const (
	mergePassing mergeOutcome = iota
	mergeFailing
	mergePending
	mergeSkipped
)

// The words GitHub answers a review decision in. An empty decision is a
// repository that requires no review at all, and is not among them: it is the
// absence of the question rather than an answer to it.
const (
	reviewApproved         = "APPROVED"
	reviewChangesRequested = "CHANGES_REQUESTED"
	reviewRequired         = "REVIEW_REQUIRED"
)

// The words GitHub answers mergeability in — the mergeable flag, and the merge
// state status that says what stands in the way when it is not simply clean.
const (
	mergeableYes     = "MERGEABLE"
	mergeConflicting = "CONFLICTING"
	mergeStateDirty  = "DIRTY"
	mergeStateBehind = "BEHIND"
)

// mergeVerdict is one of the three facts a merge is weighed on: what it is
// about, GitHub's answer put into words, and what that answer amounts to.
type mergeVerdict struct {
	label   string
	word    string
	outcome mergeOutcome
}

// mergeStateWord reads a word GitHub might not have written, in the case and
// spacing the rest of the interface is written in.
func mergeStateWord(word string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(word), "_", " "))
}

// baseOf is the branch a verdict names as the other side of the merge.
func baseOf(pr gh.PR) string {
	if base := strings.TrimSpace(pr.BaseRefName); base != "" {
		return base
	}
	return "its base"
}

// reviewVerdict is where the review stands.
func reviewVerdict(pr gh.PR) mergeVerdict {
	switch decision := strings.ToUpper(strings.TrimSpace(pr.ReviewDecision)); decision {
	case reviewApproved:
		return mergeVerdict{"review", "approved", mergePassing}
	case reviewChangesRequested:
		return mergeVerdict{"review", "changes requested", mergeFailing}
	case reviewRequired:
		return mergeVerdict{"review", "review required", mergePending}
	case "":
		return mergeVerdict{"review", "no review required", mergeSkipped}
	default:
		return mergeVerdict{"review", mergeStateWord(decision), mergePending}
	}
}

// checkStates classifies a check the way the pull request screen's checks
// section does: the states that are a machine done and either happy or not,
// with everything else read as one still going rather than as a pass, since a
// check nobody can classify is exactly the check to keep watching.
var checkStates = map[string]mergeOutcome{
	"SUCCESS":         mergePassing,
	"FAILURE":         mergeFailing,
	"ERROR":           mergeFailing,
	"TIMED_OUT":       mergeFailing,
	"STARTUP_FAILURE": mergeFailing,
	"ACTION_REQUIRED": mergeFailing,
	"SKIPPED":         mergeSkipped,
	"NEUTRAL":         mergeSkipped,
	"CANCELLED":       mergeSkipped,
	"STALE":           mergeSkipped,
}

// checkOutcomeOf is where one check stands.
func checkOutcomeOf(c gh.Check) mergeOutcome {
	if o, ok := checkStates[strings.ToUpper(strings.TrimSpace(c.State))]; ok {
		return o
	}
	return mergePending
}

// checkOutcomeOrder is worst-first, the order a rollup and a summary are read
// in: what is wrong, what is not settled, what stands, what never ran.
var checkOutcomeOrder = []mergeOutcome{mergeFailing, mergePending, mergePassing, mergeSkipped}

// checkOutcomeWord names a count of that outcome, in the rollup line's words.
func checkOutcomeWord(o mergeOutcome) string {
	switch o {
	case mergeFailing:
		return "failing"
	case mergePending:
		return "pending"
	case mergeSkipped:
		return "skipped"
	default:
		return "passing"
	}
}

// checksSummary is every outcome any check is in, counted, worst first — "2
// failing · 1 pending · 5 passing" — the same line the checks section draws.
func checksSummary(checks []gh.Check) string {
	counts := make(map[mergeOutcome]int, len(checkOutcomeOrder))
	for _, c := range checks {
		counts[checkOutcomeOf(c)]++
	}
	parts := make([]string, 0, len(checkOutcomeOrder))
	for _, o := range checkOutcomeOrder {
		if n := counts[o]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, checkOutcomeWord(o)))
		}
	}
	return strings.Join(parts, " · ")
}

// checksRollup is the worst outcome any check is in.
func checksRollup(checks []gh.Check) mergeOutcome {
	counts := make(map[mergeOutcome]int, len(checkOutcomeOrder))
	for _, c := range checks {
		counts[checkOutcomeOf(c)]++
	}
	for _, o := range checkOutcomeOrder {
		if counts[o] > 0 {
			return o
		}
	}
	return mergePassing
}

// checksVerdict is the checks as one verdict.
func checksVerdict(pr gh.PR) mergeVerdict {
	if len(pr.Checks) == 0 {
		return mergeVerdict{"checks", "no checks", mergeSkipped}
	}
	return mergeVerdict{"checks", checksSummary(pr.Checks), checksRollup(pr.Checks)}
}

// mergeableVerdict is whether the branch can go in as it stands.
func mergeableVerdict(pr gh.PR) mergeVerdict {
	mergeable := strings.ToUpper(strings.TrimSpace(pr.Mergeable))
	state := strings.ToUpper(strings.TrimSpace(pr.MergeStateStatus))
	base := baseOf(pr)
	switch {
	case mergeable == mergeConflicting || state == mergeStateDirty:
		return mergeVerdict{"mergeable", "conflicting with " + base, mergeFailing}
	case state == mergeStateBehind:
		return mergeVerdict{"mergeable", "behind " + base, mergePending}
	case mergeable == mergeableYes:
		return mergeVerdict{"mergeable", "no conflicts with " + base, mergePassing}
	default:
		return mergeVerdict{"mergeable", "mergeability unknown", mergePending}
	}
}

// mergeVerdicts are the three lines a merge is weighed on, in the order they
// are read in: who has to say yes, what has to pass, and whether the branch
// goes in as it stands.
func mergeVerdicts(pr gh.PR) []mergeVerdict {
	return []mergeVerdict{reviewVerdict(pr), checksVerdict(pr), mergeableVerdict(pr)}
}

// MergeRefusal is why a merge of pr would be refused before gh is ever asked to
// attempt it, in the same words the board's own merge box would refuse it with
// — the first failing verdict, of the review, the checks and the mergeability,
// read in that order.
//
// This is a port of internal/tui/prmerge.go's mergeRefusal rather than a call
// to it: cli must not import tui, which pulls in bubbletea, huh, lipgloss and
// glamour for a headless command that draws nothing. The two are kept level by
// hand — see the doc comment there — and a change to either's wording belongs
// in both.
//
// Only a failing verdict refuses. A verdict still to come — a review not yet
// left, checks still running, a mergeability GitHub has not computed — is not a
// no, and GitHub is the one to say whether it will take the merge anyway; what
// is refused here is what would already be refused looking at the pull request
// screen, since running gh over it could only produce the same answer more
// slowly and less clearly.
func MergeRefusal(pr gh.PR) (string, bool) {
	for _, v := range mergeVerdicts(pr) {
		if v.outcome == mergeFailing {
			return v.label + ": " + v.word, true
		}
	}
	return "", false
}
