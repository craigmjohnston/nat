package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/craigmjohnston/nat/internal/git"
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

// Branches lists existing's branches, sorted — what the board's reading asks
// to tell a Done slice with a worktree still to settle from one without.
func (f *fakeWorktrees) Branches(dir string) ([]string, error) {
	var out []string
	for b := range f.existing {
		out = append(out, b)
	}
	slices.Sort(out)
	return out, nil
}

// Reset is never asked of the board: only `nat run` resets a worktree.
func (f *fakeWorktrees) Reset(path, ref string) error { return nil }

// fakeRepo stands in for git: what the fetch was asked of, what origin's HEAD
// is read as afterwards, and the log/diff-stat gather a resume launch
// makes once the worktree is placed. The real one never fails a fetch or a
// Base read, so there is nothing here for a test to make go wrong on those
// two.
type fakeRepo struct {
	base    string
	fetches []string
	log     string
	stat    string
}

var _ Repo = (*fakeRepo)(nil)

func (f *fakeRepo) Fetch(dir string) { f.fetches = append(f.fetches, dir) }

func (f *fakeRepo) LogOneline(dir, base, branch string) (string, error) { return f.log, nil }

func (f *fakeRepo) DiffStat(dir, base, branch string) (string, error) { return f.stat, nil }

func (f *fakeRepo) Base(string) string { return f.base }

func (f *fakeRepo) ConflictsWithBase(dir, branch string) git.MergeState { return git.MergeUnknown }

// withBase gives the real git a project's configured base, and leaves a fake
// — or a project with none — as it is.
func TestWithBase(t *testing.T) {
	var real Repo = git.New()
	if got := withBase(real, "develop"); got == real {
		t.Error("withBase left the real git without the configured base")
	}
	if got := withBase(real, ""); got != real {
		t.Error("withBase changed the git of a project with no base")
	}
	fake := &fakeRepo{}
	if got := withBase[Repo](fake, "develop"); got != Repo(fake) {
		t.Error("withBase replaced a fake")
	}
}

// wantsMore is a driver interface git.CLI does not satisfy, held by something
// that can still be given a base.
type wantsMore interface {
	WithBase(string) git.CLI
	More()
}

type basedButMore struct{}

func (basedButMore) WithBase(string) git.CLI { return git.New() }
func (basedButMore) More()                   {}

// A driver whose based copy is no longer the type asked for is kept as it is.
func TestWithBaseKeepsADriverItCannotRetype(t *testing.T) {
	var r wantsMore = basedButMore{}
	if got := withBase(r, "develop"); got != r {
		t.Error("withBase replaced a driver its based copy could not stand in for")
	}
}
