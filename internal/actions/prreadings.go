package actions

import (
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
)

// PRsWorthAsking is every slice of slices whose pull request a reading should
// ask GitHub about, in the plan's own order: each In progress slice with one
// recorded — its review, its checks, or a merge nat did not witness — and a
// Done one only while its worktree still exists, the one thing left to settle
// about it (a legacy Done-at-approve slice whose pull request is still open,
// [ReopenUnmerged]'s; or a landed one whose checkout is still to sweep). A
// Done slice with no worktree has nothing left for a reading to change, and a
// mature plan holds hundreds of them, so it is not asked about at all — read
// as unread, as a slice outside the reading always was.
//
// Each repository holding a Done slice with a pull request is listed once
// ([Worktrees.Branches]); a listing that fails asks about none of its Done
// slices, logged, since no worktree can be said to exist.
func PRsWorthAsking(w Worktrees, p config.ProjectConfig, slices []domain.Slice) []domain.Slice {
	listed := map[string]map[string]bool{}
	hasWorktree := func(s domain.Slice) bool {
		dir := sliceRepo(s, p)
		if dir == "" {
			return false
		}
		branches, seen := listed[dir]
		if !seen {
			names, err := w.Branches(dir)
			if err != nil {
				logging.Action("left a repository's Done pull requests unread: worktrees unlisted", "dir", dir, "error", err)
			}
			branches = map[string]bool{}
			for _, b := range names {
				branches[b] = true
			}
			listed[dir] = branches
		}
		return branches[AgentBranch(s)]
	}
	var out []domain.Slice
	for _, s := range slices {
		if s.PRURL == "" {
			continue
		}
		switch s.Status {
		case domain.SliceClaimed:
			out = append(out, s)
		case domain.SliceDone:
			if hasWorktree(s) {
				out = append(out, s)
			}
		}
	}
	return out
}

// ListedOnce is w with [Worktrees.Branches] answered from one `git worktree
// list` per repository for the wrapper's life — so one run that decides what
// is worth asking about ([PRsWorthAsking]) and then sweeps what landed
// ([SweepLanded]) lists each repository once between them. Every other call
// goes straight to w. A listing that failed is not kept: the next asks again.
func ListedOnce(w Worktrees) Worktrees {
	return &listedOnce{Worktrees: w, lists: map[string][]string{}}
}

type listedOnce struct {
	Worktrees
	lists map[string][]string
}

func (l *listedOnce) Branches(dir string) ([]string, error) {
	if branches, ok := l.lists[dir]; ok {
		return branches, nil
	}
	branches, err := l.Worktrees.Branches(dir)
	if err != nil {
		return nil, err
	}
	l.lists[dir] = branches
	return branches, nil
}
