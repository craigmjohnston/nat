package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/craigmjohnston/nat/internal/logging"
)

// rebaseDirs are the directories git keeps under a worktree's own git dir
// while a rebase is stopped: rebase-merge for the merge backend (every
// rebase git starts by default), rebase-apply for the older apply one.
var rebaseDirs = []string{"rebase-merge", "rebase-apply"}

// RebaseInProgress says whether the worktree at dir is part way through a
// rebase, and if so which paths are still conflicted in it. The directories
// are looked up with `git rev-parse --git-path`, which answers the
// worktree's own git dir (.git/worktrees/<name> in a linked worktree) rather
// than the repository's. An error is a reading that could not be made — the
// caller must then start no rebase of its own, since one may be under way.
func (c CLI) RebaseInProgress(dir string) (conflicts []string, inProgress bool, err error) {
	for _, name := range rebaseDirs {
		out, err := c.runner.Run(dir, Binary, "rev-parse", "--git-path", name)
		if err != nil {
			logging.Error("could not find a worktree's git dir", "dir", dir, "error", err)
			return nil, false, err
		}
		path := strings.TrimSpace(out)
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		if _, err := os.Stat(path); err == nil {
			conflicts, err := c.ConflictedPaths(dir)
			return conflicts, true, err
		} else if !errors.Is(err, os.ErrNotExist) {
			logging.Error("could not tell whether a rebase is under way", "dir", dir, "error", err)
			return nil, false, err
		}
	}
	return nil, false, nil
}

// ConflictedPaths is every path the worktree at dir holds unmerged — `git diff
// --name-only --diff-filter=U` — in git's order. None is a tree with no
// conflict left in it.
func (c CLI) ConflictedPaths(dir string) ([]string, error) {
	out, err := c.runner.Run(dir, Binary, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		logging.Error("could not read a worktree's conflicted paths", "dir", dir, "error", err)
		return nil, err
	}
	var paths []string
	for line := range strings.SplitSeq(out, "\n") {
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

// Rebase rebases the branch the worktree at dir is on onto base: `git rebase
// <base>`. A rebase that goes through answers no conflicts and no error; one
// that stops on a conflicting commit answers the paths left conflicted, the
// rebase left stopped there for whoever resolves them. Anything else — a dirty
// worktree git refuses to rebase, a hook that failed, a stop with nothing
// conflicted — is aborted (`git rebase --abort`, its own failure logged), so
// the worktree is left on the branch as it was, and returned as the error.
func (c CLI) Rebase(dir, base string) ([]string, error) {
	_, rebaseErr := c.runner.Run(dir, Binary, "rebase", base)
	if rebaseErr == nil {
		logging.Action("rebased a branch onto its base", "dir", dir, "base", base)
		return nil, nil
	}
	if conflicts, err := c.ConflictedPaths(dir); err == nil && len(conflicts) > 0 {
		logging.Action("a rebase stopped on conflicts", "dir", dir, "base", base, "conflicts", len(conflicts))
		return conflicts, nil
	}
	logging.Error("could not rebase a branch onto its base", "dir", dir, "base", base, "error", rebaseErr)
	if _, err := c.runner.Run(dir, Binary, "rebase", "--abort"); err != nil {
		logging.Error("could not abort a failed rebase", "dir", dir, "error", err)
	}
	return nil, fmt.Errorf("git rebase %s: %w", base, rebaseErr)
}
