package actions

import (
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
)

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

// RemoveSliceWorktree is [RemoveWorktree] for one slice whose work has ended —
// merged, closed Done with no pull request, or trashed — in the repository
// and on the branch the launch placed its agent by: [WorkdirFor] and
// [AgentBranch], the pair that must never disagree. A slice with no
// repository at all (a source project's task that never recorded one,
// [RepoUnknown]) has nothing to remove, and git is not asked.
func RemoveSliceWorktree(w Worktrees, s domain.Slice, p config.ProjectConfig) bool {
	dir := sliceRepo(s, p)
	if dir == "" {
		return true
	}
	return RemoveWorktree(w, dir, AgentBranch(s))
}

// sliceRepo is the repository a slice's worktree was cut from, as the launch
// read it: [WorkdirFor], with a leading ~ expanded.
func sliceRepo(s domain.Slice, p config.ProjectConfig) string {
	return ExpandHome(strings.TrimSpace(WorkdirFor(s, p)))
}

// SweepLanded removes the worktree of every named slice that still has one:
// the clean-up for the work no command witnessed ending — a merge made while
// nothing was running, and every worktree left behind before the headless
// commands removed their own. Which slices have landed is the caller's to
// say; this decides only what git has to be asked.
//
// Each repository the slices name is listed once ([Worktrees.Branches]), and
// only a slice whose [AgentBranch] that listing names goes further, so a
// sweep with nothing left to do costs one git per repository and nothing per
// slice. A repository whose listing cannot be read is logged and skipped.
//
// A slice with a live agent is never swept: live is asked only once there is
// something to remove, and a tmux that cannot say concludes nothing — the
// sweep removes nothing at all and the next one asks again.
func SweepLanded(w Worktrees, live func() (map[string]string, error), p config.ProjectConfig, landed []domain.Slice) {
	var dirs []string
	byDir := map[string][]domain.Slice{}
	for _, s := range landed {
		dir := sliceRepo(s, p)
		if dir == "" {
			continue
		}
		if _, seen := byDir[dir]; !seen {
			dirs = append(dirs, dir)
		}
		byDir[dir] = append(byDir[dir], s)
	}

	type job struct{ dir, branch, id string }
	var jobs []job
	for _, dir := range dirs {
		branches, err := w.Branches(dir)
		if err != nil {
			logging.Action("left a repository's worktrees unswept", "dir", dir, "error", err)
			continue
		}
		out := map[string]bool{}
		for _, b := range branches {
			out[b] = true
		}
		for _, s := range byDir[dir] {
			if b := AgentBranch(s); out[b] {
				jobs = append(jobs, job{dir: dir, branch: b, id: s.ID})
			}
		}
	}
	if len(jobs) == 0 {
		return
	}

	running, err := live()
	if err != nil {
		logging.Action("left landed worktrees unswept: live sessions unread", "error", err)
		return
	}
	for _, j := range jobs {
		if _, ok := running[j.id]; ok {
			continue
		}
		RemoveWorktree(w, j.dir, j.branch)
	}
}
