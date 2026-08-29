package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/gh"
)

// The words GitHub answers a review decision in. An empty decision is a
// repository that requires no review at all and is not among them: it is the
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

// mergeVerdict is one line of the merge box: what the verdict is about, what it
// amounts to for a reader, and GitHub's answer put into words. The outcome is
// the same four [checkOutcome] the checks section is coloured by, since a
// verdict is read exactly as a check is — it stands, it does not, it is not
// settled yet, or it was never asked.
type mergeVerdict struct {
	label   string
	word    string
	outcome checkOutcome
}

// mergeState reads a word GitHub might not have written, in the case and
// spacing the rest of the interface is written in: a state this build does not
// know is still worth saying out loud rather than drawing as a blank.
func mergeState(word string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(word), "_", " "))
}

// baseOf is the branch a verdict names as the other side of the merge. A pull
// request read with no base at all — one gh answered before this build asked
// for the field, or a reading that dropped it — is still worth a verdict, so it
// is named in words rather than by a branch nobody can see.
func baseOf(pr gh.PR) string {
	if base := strings.TrimSpace(pr.BaseRefName); base != "" {
		return base
	}
	return "its base"
}

// reviewVerdict is where the review stands. GitHub's three answers are the
// three a reader acts on; a repository that requires no review at all reports
// no decision, which is the question never having been asked rather than an
// approval, and anything else GitHub says is a decision nobody here can read as
// settled — which is the verdict to keep watching rather than one to pass.
func reviewVerdict(pr gh.PR) mergeVerdict {
	switch decision := strings.ToUpper(strings.TrimSpace(pr.ReviewDecision)); decision {
	case reviewApproved:
		return mergeVerdict{"review", "approved", checkPassing}
	case reviewChangesRequested:
		return mergeVerdict{"review", "changes requested", checkFailing}
	case reviewRequired:
		return mergeVerdict{"review", "review required", checkPending}
	case "":
		return mergeVerdict{"review", "no review required", checkSkipped}
	default:
		return mergeVerdict{"review", mergeState(decision), checkPending}
	}
}

// checksVerdict is the checks as one line, off the very rollup the checks
// section draws its own heading from, so the two can never disagree about
// whether the machines are happy.
func checksVerdict(pr gh.PR) mergeVerdict {
	if len(pr.Checks) == 0 {
		return mergeVerdict{"checks", "no checks", checkSkipped}
	}
	return mergeVerdict{"checks", checkSummary(pr.Checks), checkRollup(pr.Checks)}
}

// mergeableVerdict is whether the branch can go in as it stands, read off both
// of GitHub's fields rather than the mergeable flag alone: a branch that
// conflicts with its base is named as conflicting, which is the one thing on
// this section the author has to go and do something about, and one merely
// behind its base is named as behind — neither is "unmergeable", which says
// what is not true and nothing about why.
//
// Anything else GitHub reports — the UNKNOWN of a mergeability it has not
// computed yet, the empty answer of a pull request read before it did, a word
// this build does not know — is a verdict still to come, for the reason an
// unrecognised check is: it is what the refresh key is pressed over.
func mergeableVerdict(pr gh.PR) mergeVerdict {
	mergeable := strings.ToUpper(strings.TrimSpace(pr.Mergeable))
	state := strings.ToUpper(strings.TrimSpace(pr.MergeStateStatus))
	base := baseOf(pr)
	switch {
	case mergeable == mergeConflicting || state == mergeStateDirty:
		return mergeVerdict{"mergeable", "conflicting with " + base, checkFailing}
	case state == mergeStateBehind:
		return mergeVerdict{"mergeable", "behind " + base, checkPending}
	case mergeable == mergeableYes:
		return mergeVerdict{"mergeable", "no conflicts with " + base, checkPassing}
	default:
		return mergeVerdict{"mergeable", "mergeability unknown", checkPending}
	}
}

// mergeVerdicts are the three lines in the order they are read in: who has to
// say yes, what has to pass, and whether the branch goes in as it stands.
func mergeVerdicts(pr gh.PR) []mergeVerdict {
	return []mergeVerdict{reviewVerdict(pr), checksVerdict(pr), mergeableVerdict(pr)}
}

// mergeRollup is the worst of the verdicts, in the same worst-first order the
// checks rollup is read in, and what the heading is coloured by: one failing
// verdict is what the section is about however many stand.
func mergeRollup(verdicts []mergeVerdict) checkOutcome {
	in := make(map[checkOutcome]bool, len(verdicts))
	for _, v := range verdicts {
		in[v.outcome] = true
	}
	for _, o := range checkOutcomes {
		if in[o] {
			return o
		}
	}
	// Unreachable through [mergeVerdicts], which always answers with three.
	return checkPassing
}

// mergeSummary is the heading's own answer to the question the section asks:
// green across the board means yes, one verdict still to come means not yet,
// and a failing one means no.
func mergeSummary(rollup checkOutcome) string {
	switch rollup {
	case checkFailing:
		return "cannot merge"
	case checkPending:
		return "not ready to merge"
	default:
		return "ready to merge"
	}
}

// mergeSection is GitHub's merge box as the pull request screen draws it: a
// heading answering whether the pull request can merge, and under it one line
// per verdict — the review, the checks and the branch itself — each with a mark
// and a colour saying what it amounts to. The labels are padded so the answers
// line up in a column of their own, the way the checks section pads its names.
//
// It goes under the checks rather than over them because it is the conclusion
// the rows above are read into, and it reads the very same rollup they do, so
// no two lines of this screen can disagree about the machines.
//
// A pull request that has merged or been closed has the section replaced by
// that ending: there is nothing left to weigh up, and three verdicts about a
// branch already in — or already given up on — would read as a question still
// open.
func (p PRView) mergeSection() string {
	heading := p.styles.Title.Render("Merge") + "  "
	switch p.pr.State {
	case gh.PRStateMerged:
		return fit(heading+p.styles.PRMerged.Render("merged into "+baseOf(p.pr)), p.width)
	case gh.PRStateClosed:
		return fit(heading+p.styles.PRClosed.Render("closed without merging"), p.width)
	}
	verdicts := mergeVerdicts(p.pr)
	rollup := mergeRollup(verdicts)
	lines := []string{fit(heading+rollup.style(p.styles).Render(mergeSummary(rollup)), p.width)}
	labels := 0
	for _, v := range verdicts {
		labels = max(labels, lipgloss.Width(v.label))
	}
	for _, v := range verdicts {
		style := v.outcome.style(p.styles)
		label := v.label + strings.Repeat(" ", labels-lipgloss.Width(v.label))
		lines = append(lines, fit(fmt.Sprintf("  %s %s  %s",
			style.Render(v.outcome.mark()), label, style.Render(v.word)), p.width))
	}
	return strings.Join(lines, "\n")
}
