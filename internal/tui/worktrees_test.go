package tui

import (
	"fmt"
	"path/filepath"
)

// worktreeCall is one thing the fake was asked about: the repository, the
// branch it was asked about there, and — for a create — the ref that branch
// was to be cut from.
type worktreeCall struct{ dir, branch, base string }

// fakeWorktrees stands in for git's worktrees. Nothing it is asked about exists
// unless the test says so, which is the ordinary case: a slice nobody has
// worked yet has no worktree and no branch.
type fakeWorktrees struct {
	// existing are the branches already checked out somewhere, by path.
	existing map[string]string
	// pathErr is what a branch not in existing answers with, createErr what the
	// create that follows fails with, and removeErr what a removal is refused
	// with; all nil is git working.
	pathErr   error
	createErr error
	removeErr error

	looks   []worktreeCall
	creates []worktreeCall
	removes []worktreeCall
}

var _ Worktrees = (*fakeWorktrees)(nil)

func (f *fakeWorktrees) Path(dir, branch string) (string, error) {
	f.looks = append(f.looks, worktreeCall{dir: dir, branch: branch})
	if path, ok := f.existing[branch]; ok {
		return path, nil
	}
	if f.pathErr != nil {
		return "", f.pathErr
	}
	return "", fmt.Errorf("git names no worktree for %s", branch)
}

func (f *fakeWorktrees) Create(dir, branch, base string) (string, error) {
	f.creates = append(f.creates, worktreeCall{dir, branch, base})
	if f.createErr != nil {
		return "", f.createErr
	}
	return filepath.Join(dir+"-worktrees", branch), nil
}

func (f *fakeWorktrees) Remove(dir, branch string) error {
	f.removes = append(f.removes, worktreeCall{dir: dir, branch: branch})
	return f.removeErr
}

// fakeRepo stands in for git: what the fetch was asked of, and what origin's
// HEAD is read as afterwards. The real one never fails at either — a fetch that
// could not reach the remote is swallowed, and an unreadable HEAD falls back to
// main — so there is nothing here for a test to make go wrong.
type fakeRepo struct {
	base    string
	fetches []string
}

var _ Repo = (*fakeRepo)(nil)

func (f *fakeRepo) Fetch(dir string) { f.fetches = append(f.fetches, dir) }

func (f *fakeRepo) Base(string) string { return f.base }
