package actions

import "github.com/craigmjohnston/nat/internal/logging"

// RemoveWorktree takes a slice's worktree off its repository, and reports
// whether there is anything left to remove there — which a removal git
// refused is the only case of.
//
// A branch git names no worktree for is not a failure at all: it is a slice
// whose worktree has already gone, which is every slice on every sweep after
// the first, so it passes silently rather than filling the log with a line a
// load. What git does refuse — a worktree holding uncommitted changes, a
// repository it will not answer about — is dropped after one line: the pull
// request is merged and the slice is Done whatever became of the checkout,
// and git's own rules mean a refusal never costs any work.
func RemoveWorktree(w Worktrees, dir, branch string) bool {
	if _, err := w.Path(dir, branch); err != nil {
		return true
	}
	if err := w.Remove(dir, branch); err != nil {
		// git's own failure is already in the log; this is the decision
		// taken about it.
		logging.Action("left the slice's worktree in place", "dir", dir, "branch", branch, "error", err)
		return false
	}
	return true
}
