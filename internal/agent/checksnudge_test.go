package agent

import (
	"strings"
	"testing"
)

// The nudge names the pull request, the two gh reads it relaxes the ban for,
// and the branch a fix is pushed to — and nothing that would let the agent
// touch the pull request itself.
func TestChecksFailingPrompt(t *testing.T) {
	got := ChecksFailingPrompt("https://github.test/pr/7", "slice/red")
	for _, want := range []string{
		"checks on your pull request are failing: https://github.test/pr/7",
		"gh pr checks https://github.test/pr/7",
		"gh pr view https://github.test/pr/7",
		"push\nslice/red again",
		"Never open, merge, close or reopen a pull request.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the prompt does not say %q:\n%s", want, got)
		}
	}
}
