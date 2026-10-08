package actions

import (
	"errors"
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// HandBackGit is what a hand-back needs of git: the branch a worktree has
// checked out, what it holds uncommitted, and the push. [git.CLI] answers all
// three.
type HandBackGit interface {
	CurrentBranch(dir string) (string, error)
	DirtyPaths(dir string) ([]string, error)
	Push(dir, branch string) error
}

// ErrNoWorktree is a hand-back that named no branch for a slice with no
// worktree to read one off: nothing says which branch holds the work, and
// guessing would hand back the wrong one or none.
var ErrNoWorktree = errors.New("the slice has no worktree to read its branch off")

// DirtyError is a hand-back refused because the worktree holds work that is
// not committed: a hand-back is a claim that the branch holds the work, and a
// dirty tree says it does not.
type DirtyError struct {
	Dir   string
	Paths []string
}

func (e *DirtyError) Error() string {
	return fmt.Sprintf("%s has uncommitted changes — commit them (or remove them) and hand back again:\n  %s",
		e.Dir, strings.Join(e.Paths, "\n  "))
}

// PushHandBack is the git half of handing a slice's branch back, done before
// anything is written to the page: it settles which branch that is, refuses a
// worktree with uncommitted changes, and pushes the branch to origin. It
// answers the branch pushed.
//
// branch is the one the caller named, or "" for the one the slice's worktree
// — [AgentBranch]'s, in [WorkdirFor]'s repository — has checked out. A named
// branch with no worktree is pushed from the repository itself, where it
// exists locally with nothing checked out to be dirty; an unnamed one with no
// worktree is [ErrNoWorktree].
func PushHandBack(w Worktrees, g HandBackGit, s domain.Slice, p config.ProjectConfig, branch string) (string, error) {
	repo := sliceRepo(s, p)
	if repo == "" {
		if branch == "" {
			return "", ErrNoWorktree
		}
		return "", fmt.Errorf("%q has no repository recorded to push %s from: record it with nat slice-repo",
			s.Name, branch)
	}
	lookup := branch
	if lookup == "" {
		lookup = AgentBranch(s)
	}
	dir, err := w.Path(repo, lookup)
	if err != nil {
		if branch == "" {
			return "", ErrNoWorktree
		}
		return branch, g.Push(repo, branch)
	}
	if branch == "" {
		if branch, err = g.CurrentBranch(dir); err != nil {
			return "", fmt.Errorf("read the branch %s has checked out: %w", dir, err)
		}
		if branch == "" {
			return "", fmt.Errorf("%s has no branch checked out (a detached HEAD): check the slice's branch out there, or name it with --branch", dir)
		}
	}
	dirty, err := g.DirtyPaths(dir)
	if err != nil {
		return "", fmt.Errorf("read what %s holds uncommitted: %w", dir, err)
	}
	if len(dirty) > 0 {
		return "", &DirtyError{Dir: dir, Paths: dirty}
	}
	return branch, g.Push(dir, branch)
}
