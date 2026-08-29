package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/gh"
)

// checkOutcome is what a status check amounts to for a reader: it passed, it
// failed, it is still going, or it never ran at all. GitHub has a word for
// every way each of those happens — a run that timed out and one that failed
// outright are two words for the same news — and the four here are what the
// rollup line counts and the rows are coloured by, since a summary that used
// GitHub's own vocabulary would have a column per word.
type checkOutcome int

const (
	checkPassing checkOutcome = iota
	checkFailing
	checkPending
	checkSkipped
)

// checkOutcomes is the order the rollup line reads its counts in, which is the
// order the news matters in: what is wrong first, what is not settled yet
// after it, and what stands last.
var checkOutcomes = []checkOutcome{checkFailing, checkPending, checkPassing, checkSkipped}

// word is what the rollup line calls a count of that outcome.
func (o checkOutcome) word() string {
	switch o {
	case checkFailing:
		return "failing"
	case checkPending:
		return "pending"
	case checkSkipped:
		return "skipped"
	default:
		return "passing"
	}
}

// mark is the glyph a row of that outcome opens with, one cell wide so every
// name starts at the same column.
func (o checkOutcome) mark() string {
	switch o {
	case checkFailing:
		return "✗"
	case checkPending:
		return "●"
	case checkSkipped:
		return "○"
	default:
		return "✓"
	}
}

// style is what an outcome is drawn in.
func (o checkOutcome) style(s Styles) lipgloss.Style {
	switch o {
	case checkFailing:
		return s.CheckFail
	case checkPending:
		return s.CheckPending
	case checkSkipped:
		return s.CheckSkip
	default:
		return s.CheckPass
	}
}

// The states a check arrives in that are not a machine still working. They are
// GitHub's own words — a CheckRun's conclusion, or a StatusContext's state,
// which [gh.Check] has already reduced to the one field — and everything not
// named here is a check that has not finished: QUEUED, IN_PROGRESS, PENDING,
// WAITING, REQUESTED, the EXPECTED of a status nothing has reported yet, and
// whatever GitHub adds next, all of which are things
// the refresh key is worth pressing over.
var checkStates = map[string]checkOutcome{
	"SUCCESS":         checkPassing,
	"FAILURE":         checkFailing,
	"ERROR":           checkFailing,
	"TIMED_OUT":       checkFailing,
	"STARTUP_FAILURE": checkFailing,
	"ACTION_REQUIRED": checkFailing,
	"SKIPPED":         checkSkipped,
	"NEUTRAL":         checkSkipped,
	"CANCELLED":       checkSkipped,
	"STALE":           checkSkipped,
}

// outcomeOf is where a check stands. A state this build does not know reads as
// one still going rather than as one that passed: a check nobody can classify
// is exactly the check to keep watching, and calling it a pass would have the
// rollup line say the work is ready when nothing said so.
func outcomeOf(c gh.Check) checkOutcome {
	if o, ok := checkStates[strings.ToUpper(strings.TrimSpace(c.State))]; ok {
		return o
	}
	return checkPending
}

// checkStateWord is how a check's own state is drawn beside its name: GitHub's
// word as GitHub writes it, in the case and spacing the rest of the interface
// is written in, since the screen is prose rather than an API response. A check
// GitHub reported no state for at all — one queued so recently it has none —
// says so rather than leaving the column blank.
func checkStateWord(c gh.Check) string {
	state := strings.TrimSpace(c.State)
	if state == "" {
		return "unknown"
	}
	return strings.ToLower(strings.ReplaceAll(state, "_", " "))
}

// checkCounts is how many checks are in each outcome, addressed by the outcome.
func checkCounts(checks []gh.Check) map[checkOutcome]int {
	counts := make(map[checkOutcome]int, len(checkOutcomes))
	for _, c := range checks {
		counts[outcomeOf(c)]++
	}
	return counts
}

// checkSummary is the rollup line's own text: every outcome that any check is
// in, counted, worst first — "2 failing · 1 pending · 5 passing" — so the line
// says exactly what the rows under it do and nothing about the outcomes none of
// them is in. It is only ever called with checks to count, the section drawing
// one line and no rollup at all where there are none.
func checkSummary(checks []gh.Check) string {
	counts := checkCounts(checks)
	parts := make([]string, 0, len(checkOutcomes))
	for _, o := range checkOutcomes {
		if n := counts[o]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, o.word()))
		}
	}
	return strings.Join(parts, " · ")
}

// checkRollup is the outcome the rollup line is coloured by: the worst any
// check is in, in the same order the counts are read in, which is GitHub's own
// banner — one failure is what the line is about however many passed.
func checkRollup(checks []gh.Check) checkOutcome {
	counts := checkCounts(checks)
	for _, o := range checkOutcomes {
		if counts[o] > 0 {
			return o
		}
	}
	// Only reachable through a caller of its own: a rollup of no checks at all
	// is the one line the section draws instead.
	return checkPassing
}

// checksSection is the checks as the pull request screen draws them: a heading
// with the rollup beside it, and under it one row per check — its mark, its
// name and the state GitHub reports it in, coloured by what that state amounts
// to. The names are padded to the longest of them so the states line up in a
// column of their own.
//
// A pull request with no checks at all — a repository that has none configured,
// or one whose workflows do not run on it — is one line saying so: an empty
// section under a heading would read as checks that have yet to report.
func (p PRView) checksSection() string {
	if len(p.pr.Checks) == 0 {
		return fit(p.styles.Faint.Render("No checks have run on this pull request."), p.width)
	}
	rollup := checkRollup(p.pr.Checks)
	lines := []string{fit(p.styles.Title.Render("Checks")+"  "+
		rollup.style(p.styles).Render(checkSummary(p.pr.Checks)), p.width)}
	names := 0
	for _, c := range p.pr.Checks {
		names = max(names, lipgloss.Width(c.Name))
	}
	for _, c := range p.pr.Checks {
		outcome := outcomeOf(c)
		style := outcome.style(p.styles)
		name := c.Name + strings.Repeat(" ", names-lipgloss.Width(c.Name))
		lines = append(lines, fit("  "+style.Render(outcome.mark())+" "+name+"  "+
			style.Render(checkStateWord(c)), p.width))
	}
	return strings.Join(lines, "\n")
}
