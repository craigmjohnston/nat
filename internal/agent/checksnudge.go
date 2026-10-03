package agent

import (
	"fmt"
	"strings"
)

// ChecksFailingPrompt is the one turn typed at a live agent whose slice's pull
// request has just been read with a failing check: what happened, the two `gh`
// reads that say what failed, and that a push to branch is the whole of the
// fix.
//
// Those two reads are the fix prompt's own (see [fixPrompt]) and relaxed the
// same way, for exactly them: the agent was launched under the standing ban on
// `gh`, and this is the one thing it needs from it to act on the news. Opening,
// merging and closing a pull request stay the user's alone.
func ChecksFailingPrompt(prURL, branch string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The checks on your pull request are failing: %s\n\n", prURL)
	b.WriteString("Read what failed with these two commands — the only `gh` you may run,\n")
	b.WriteString("an exception to the usual ban for exactly them:\n\n")
	fmt.Fprintf(&b, "    gh pr checks %s\n", prURL)
	fmt.Fprintf(&b, "    gh pr view %s\n\n", prURL)
	b.WriteString("Fix the cause, run the project's verification gate, then commit and push\n")
	fmt.Fprintf(&b, "%s again — the branch the pull request is built from, which picks up\n", branch)
	b.WriteString("the push by itself. Never open, merge, close or reopen a pull request.\n")
	return b.String()
}
