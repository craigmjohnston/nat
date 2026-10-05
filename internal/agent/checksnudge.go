package agent

import (
	"fmt"
	"strings"
)

// FailedCheck is one check a pull request failed, as a nudge names it: what it
// is called and where its run can be read.
type FailedCheck struct {
	Name string
	URL  string
}

// ChecksContext is everything [ChecksPrompt] says: the slice the session is
// on, the pull request whose checks failed, which of them failed, the branch
// the fix goes on, and the project every `nat` command is pinned to.
type ChecksContext struct {
	SliceID   string
	ProjectID string
	PRURL     string
	Branch    string
	Failing   []FailedCheck
}

// ChecksPrompt is the one turn typed at a live agent whose slice's pull request
// has just been read with a failing check: what failed and where, the `nat`
// command that reads the failures' logs, and that the fix ends the way a
// slice does — pushed to the same branch, then handed back with
// `complete-slice`, which is what tells the record the fix is in.
//
// It names no `gh` at all: `nat slice-checks` is the one way an agent reads
// CI, so the standing ban on `gh` holds here as everywhere else. Opening,
// merging and closing a pull request stay the user's alone.
func ChecksPrompt(c ChecksContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The checks on your pull request are failing: %s\n\n", c.PRURL)
	for _, check := range c.Failing {
		if check.URL != "" {
			fmt.Fprintf(&b, "- %s: %s\n", check.Name, check.URL)
		} else {
			fmt.Fprintf(&b, "- %s\n", check.Name)
		}
	}
	b.WriteString("\nRead what failed, with the failed steps' logs, with:\n\n")
	fmt.Fprintf(&b, "    nat slice-checks %s --log --project %s\n\n", c.SliceID, c.ProjectID)
	b.WriteString(runningChecksSentence)
	b.WriteString(rerunPassage(c.SliceID, c.ProjectID))
	b.WriteString("\nOtherwise fix the cause on the same branch, run the project's verification gate,\n")
	fmt.Fprintf(&b, "then commit and push %s — the branch the pull request is built\n", c.Branch)
	b.WriteString("from, which picks up the push by itself. Then hand it back as a slice\n")
	b.WriteString("ends:\n\n")
	fmt.Fprintf(&b, "    nat complete-slice %s --branch %s --summary '<what you fixed>' --project %s\n\n",
		c.SliceID, c.Branch, c.ProjectID)
	b.WriteString("Never run `gh`, and never open, merge, close or reopen a pull request.\n")
	return b.String()
}
