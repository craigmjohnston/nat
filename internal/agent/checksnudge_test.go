package agent

import (
	"strings"
	"testing"
)

// The nudge names the pull request, each failed check with its run URL (a
// check with none by name alone), the pinned slice-checks read, the branch a
// fix is pushed to and the complete-slice hand-back it ends in — and no `gh`.
func TestChecksPrompt(t *testing.T) {
	got := ChecksPrompt(ChecksContext{
		SliceID: "s1", ProjectID: testProjectID, PRURL: "https://github.test/pr/7", Branch: "slice/red",
		Failing: []FailedCheck{{Name: "test", URL: "https://github.test/runs/1"}, {Name: "deploy"}},
	})
	for _, want := range []string{
		"checks on your pull request are failing: https://github.test/pr/7",
		"- test: https://github.test/runs/1\n",
		"- deploy\n",
		"nat slice-checks s1 --log --project " + testProjectID,
		"shows what a check still running is doing",
		"nat slice-checks-rerun s1 --check '<check name>' --project " + testProjectID,
		"push slice/red",
		"nat complete-slice s1 --branch slice/red --summary '<what you fixed>' --project " + testProjectID,
		"Never run `gh`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the prompt does not say %q:\n%s", want, got)
		}
	}
	for _, cmd := range natCommands(got) {
		if !strings.Contains(cmd, "--project "+testProjectID) {
			t.Errorf("the checks prompt runs %q without naming the project", cmd)
		}
	}
}
