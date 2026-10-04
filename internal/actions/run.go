package actions

import (
	"errors"
	"fmt"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// RunBranch is the branch of nat's own run checkout: the worktree a project's
// global runs are run in, kept at the latest origin/main. It is nat's alone —
// no slice, agent or user works on it — which is what makes hard-resetting it
// before every run safe.
const RunBranch = "run/main"

// GlobalRunDir is where a project's global run is run from: the run checkout
// of the repository at dir, on [RunBranch], cut by the worktree package where
// there is none and, before every run, brought to the remote's default branch
// as it stands now — origin fetched, then the checkout hard-reset to
// [Repo.Base]. A fetch that fails is logged by Repo and swallowed, so the run
// goes on from the refs as last known, the posture a launch takes. The user's
// own checkout at dir is never checked out or reset: only its refs are
// fetched.
func GlobalRunDir(w Worktrees, r Repo, dir string) (string, error) {
	dir = ExpandHome(strings.TrimSpace(dir))
	if dir == "" {
		return "", errors.New("the project has no working directory to run from")
	}
	if !InRepo(dir) {
		return "", fmt.Errorf("%s is not a git repository: a global run is run from its origin/main", dir)
	}
	r.Fetch(dir)
	base := r.Base(dir)
	path, err := w.Path(dir, RunBranch)
	if err != nil {
		if path, err = w.Create(dir, RunBranch, base); err != nil {
			return "", fmt.Errorf("could not make the run checkout: %w", err)
		}
	}
	if err := w.Reset(path, base); err != nil {
		return "", fmt.Errorf("could not bring the run checkout to %s: %w", base, err)
	}
	return path, nil
}

// SliceRunDir is where a slice-scoped run is run from: the worktree of the
// slice's [AgentBranch] in the repository [WorkdirFor] names — the same
// lookup a launch places its agent by and a merge removes the worktree by.
//
// Refused: a merged slice (Done with a pull request or branch: its worktree is
// gone), a source project's task with no repository recorded ([RepoUnknown]),
// and a slice whose branch has no worktree, which leaves nothing to run in.
func SliceRunDir(w Worktrees, s domain.Slice, p config.ProjectConfig) (string, error) {
	if s.Status == domain.SliceDone && (s.PRURL != "" || s.Branch != "") {
		return "", fmt.Errorf("%q is merged: its worktree is gone, so there is nothing to run in", s.Name)
	}
	dir := WorkdirFor(s, p)
	if RepoUnknown(dir, p) {
		return "", fmt.Errorf("%q has no repository recorded yet, so there is nothing to run in", s.Name)
	}
	dir = ExpandHome(strings.TrimSpace(dir))
	if dir == "" {
		return "", errors.New("the project has no working directory to run from")
	}
	branch := AgentBranch(s)
	path, err := w.Path(dir, branch)
	if err != nil {
		return "", fmt.Errorf("%q has no worktree for %s, so there is nothing to run in", s.Name, branch)
	}
	return path, nil
}
