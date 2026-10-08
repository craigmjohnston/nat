package gh

import (
	"fmt"

	"github.com/craigmjohnston/nat/internal/logging"
)

// MergeOptions is how a pull request is merged, said out loud rather than left
// to gh. Without a strategy flag gh asks which one to use at an interactive
// prompt, and a [Runner] is a subprocess with nothing on its standard input to
// answer one with — so the merge would hang rather than happen.
//
// Method is merge, squash or rebase — gh's own flag names — and the empty
// string is merge, a merge commit, which is how this repository's own history
// reads. DeleteBranch adds --delete-branch.
type MergeOptions struct {
	Method       string
	DeleteBranch bool
}

// mergeMethods are the strategies gh pr merge has a flag for.
var mergeMethods = map[string]bool{"merge": true, "squash": true, "rebase": true}

// args is the strategy flags for gh pr merge, or an error naming a method gh
// has no flag for — refused before gh runs, since a hand-edited config is the
// only way to reach one.
func (o MergeOptions) args() ([]string, error) {
	method := o.Method
	if method == "" {
		method = "merge"
	}
	if !mergeMethods[method] {
		return nil, fmt.Errorf("no merge method %q: want merge, squash or rebase", o.Method)
	}
	args := []string{"--" + method}
	if o.DeleteBranch {
		args = append(args, "--delete-branch")
	}
	return args, nil
}

// MergePR merges the pull request ref names, in the repository at dir, as
// opts says.
//
// The ref is whatever identifies it, exactly as [CLI.ViewPR] takes one, and is
// required for the same reason: gh with nothing named merges the pull request
// of whatever branch the directory happens to be on, which in a shared checkout
// is nobody's slice in particular — and a merge is not a reading that can be
// taken again.
//
// Whether the pull request can merge at all is the caller's question and is
// asked of the merge box before this is ever reached. What is left here is
// everything GitHub knows and nat does not — a branch protection rule, a review
// dismissed by a push, a check that went red between the reading and the key —
// and all of it comes back as the [*ExitError] the runner made of it, carrying
// gh's own first line, which is the sentence worth showing.
func (c CLI) MergePR(dir, ref string, opts MergeOptions) error {
	if ref == "" {
		return fmt.Errorf("%s pr merge needs a pull request to merge", Binary)
	}
	strategy, err := opts.args()
	if err != nil {
		return err
	}
	if _, err := c.runner.Run(dir, Binary, append([]string{"pr", "merge", ref}, strategy...)...); err != nil {
		logging.Error("could not merge a pull request", "dir", dir, "ref", ref, "error", err)
		return err
	}
	logging.Action("pull request merged", "dir", dir, "ref", ref)
	return nil
}
