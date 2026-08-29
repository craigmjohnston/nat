package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/craigmjohnston/nat/internal/gh"
)

// checkedPR is the sample pull request with a rollup on it.
func checkedPR(checks ...gh.Check) gh.PR {
	pr := samplePR()
	pr.Checks = checks
	return pr
}

// The three rollups the section is drawn from: one where everything stands, one
// with a failure in it, and one still going — which are the three readings the
// refresh key is pressed between.
var (
	passingChecks = []gh.Check{
		{Name: "build", State: "SUCCESS"},
		{Name: "test (ubuntu-latest)", State: "SUCCESS"},
		{Name: "lint", State: "SUCCESS"},
	}
	failingChecks = []gh.Check{
		{Name: "build", State: "SUCCESS"},
		{Name: "test (ubuntu-latest)", State: "FAILURE"},
		{Name: "lint", State: "TIMED_OUT"},
		{Name: "coverage", State: "SKIPPED"},
	}
	pendingChecks = []gh.Check{
		{Name: "build", State: "SUCCESS"},
		{Name: "test (ubuntu-latest)", State: "IN_PROGRESS"},
		{Name: "lint", State: "QUEUED"},
	}
)

// TestCheckOutcomeOf covers where each of GitHub's words puts a check, and that
// a word this build does not know is one still going rather than one that
// passed.
func TestCheckOutcomeOf(t *testing.T) {
	tests := []struct {
		state string
		want  checkOutcome
	}{
		{"SUCCESS", checkPassing},
		{"FAILURE", checkFailing},
		{"ERROR", checkFailing},
		{"TIMED_OUT", checkFailing},
		{"STARTUP_FAILURE", checkFailing},
		{"ACTION_REQUIRED", checkFailing},
		{"SKIPPED", checkSkipped},
		{"NEUTRAL", checkSkipped},
		{"CANCELLED", checkSkipped},
		{"STALE", checkSkipped},
		{"IN_PROGRESS", checkPending},
		{"QUEUED", checkPending},
		{"PENDING", checkPending},
		{"EXPECTED", checkPending},
		{"", checkPending},
		{"REBOOTING", checkPending},
		{" success ", checkPassing},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			if got := outcomeOf(gh.Check{State: tt.state}); got != tt.want {
				t.Errorf("outcome of %q = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

// TestCheckStateWord covers the state as it is drawn beside a name: GitHub's
// word in the case the rest of the interface is written in, and a check
// reported with no state at all saying so rather than leaving the column blank.
func TestCheckStateWord(t *testing.T) {
	tests := []struct{ state, want string }{
		{"SUCCESS", "success"},
		{"TIMED_OUT", "timed out"},
		{"", "unknown"},
		{"  ", "unknown"},
	}
	for _, tt := range tests {
		if got := checkStateWord(gh.Check{State: tt.state}); got != tt.want {
			t.Errorf("word for %q = %q, want %q", tt.state, got, tt.want)
		}
	}
}

// TestCheckSummaryAgreesWithTheRows covers the rollup line: every outcome any
// check is in, counted, worst first — and no mention of the outcomes none of
// them is in.
func TestCheckSummaryAgreesWithTheRows(t *testing.T) {
	tests := []struct {
		name   string
		checks []gh.Check
		want   string
		rollup checkOutcome
	}{
		{"all passing", passingChecks, "3 passing", checkPassing},
		{"a failure among them", failingChecks,
			"2 failing · 1 passing · 1 skipped", checkFailing},
		{"still going", pendingChecks, "2 pending · 1 passing", checkPending},
		{"nothing but skipped", []gh.Check{{Name: "deploy", State: "SKIPPED"}},
			"1 skipped", checkSkipped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkSummary(tt.checks); got != tt.want {
				t.Errorf("summary = %q, want %q", got, tt.want)
			}
			if got := checkRollup(tt.checks); got != tt.rollup {
				t.Errorf("rollup = %v, want %v", got, tt.rollup)
			}
			// The counts are the rows: every check is in exactly one of them.
			counted := 0
			for _, o := range checkOutcomes {
				counted += checkCounts(tt.checks)[o]
			}
			if counted != len(tt.checks) {
				t.Errorf("counted %d checks, want %d", counted, len(tt.checks))
			}
		})
	}
	// A rollup of nothing is the one line the section draws instead, and never
	// reaches the colouring; asked anyway, it is the outcome nothing is wrong in.
	if got := checkRollup(nil); got != checkPassing {
		t.Errorf("rollup of no checks = %v, want %v", got, checkPassing)
	}
}

// TestPRViewChecksRender pins the section on each of the three rollups: the
// heading and its summary, and under them a row per check with its name and
// what GitHub says of it, the states in a column of their own.
func TestPRViewChecksRender(t *testing.T) {
	for _, tt := range []struct {
		name   string
		checks []gh.Check
	}{
		{"pr-checks-passing", passingChecks},
		{"pr-checks-failing", failingChecks},
		{"pr-checks-pending", pendingChecks},
	} {
		t.Run(tt.name, func(t *testing.T) {
			golden(t, tt.name, readyPRView(checkedPR(tt.checks...)).checksSection())
		})
	}
}

// TestPRViewChecksInTheBody covers where the section sits: under the
// description, in the viewport with it, so a rollup longer than the screen
// scrolls.
func TestPRViewChecksInTheBody(t *testing.T) {
	body := readyPRView(checkedPR(failingChecks...)).vp.GetContent()
	described := strings.Index(body, "A screen over the board")
	checks := strings.Index(body, "Checks")
	if described < 0 || checks < 0 {
		t.Fatalf("body = %q, want the description and the checks in it", body)
	}
	if checks < described {
		t.Errorf("body = %q, want the checks under the description", body)
	}
	for _, want := range []string{"test (ubuntu-latest)", "timed out", "2 failing"} {
		if !strings.Contains(body, want) {
			t.Errorf("body = %q, want %q in it", body, want)
		}
	}
}

// TestPRViewWithoutChecks covers a pull request nothing runs on: one line
// saying so, rather than a heading over an empty section.
func TestPRViewWithoutChecks(t *testing.T) {
	p := readyPRView(samplePR())
	section := p.checksSection()
	if !strings.Contains(section, "No checks have run") {
		t.Errorf("section = %q, want the absence said out loud", section)
	}
	if strings.Contains(section, "\n") {
		t.Errorf("section = %q, want one line", section)
	}
	if !strings.Contains(p.View(""), "No checks have run") {
		t.Error("the line belongs on the screen")
	}
}

// TestPRViewChecksFitTheWidth covers a narrow screen: a check named longer than
// the window is cut to it rather than wrapping onto a line the layout has not
// left room for.
func TestPRViewChecksFitTheWidth(t *testing.T) {
	p := readyPRView(checkedPR(gh.Check{
		Name:  strings.Repeat("a-very-long-workflow-name", 4),
		State: "SUCCESS",
	}))
	p.SetSize(40, 20)
	for _, line := range strings.Split(p.checksSection(), "\n") {
		if got := lipgloss.Width(line); got > 40 {
			t.Errorf("line %q is %d columns, want no more than 40", line, got)
		}
	}
}
