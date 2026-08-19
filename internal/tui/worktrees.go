package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/worktree"
)

// Worktrees is what the board needs of worktrunk: where a slice's branch is
// already checked out, a worktree for it where it is not, and that worktree
// taken away again once the work has become a pull request. It is an interface
// for the reason [AgentLauncher] is one — so a launch can be driven without the
// binary — and looking up is its own call rather than part of creating because
// a relaunched slice wants the worktree it was working in, not a second one.
type Worktrees interface {
	Path(dir, branch string) (string, error)
	Create(dir, branch string) (string, error)
	Remove(dir, branch string) error
}

// newWorktrees is the edge, held as a variable so the tests can stand in for
// it: the real one drives the wt binary on PATH.
var newWorktrees = defaultWorktrees

// defaultWorktrees is the real worktrunk.
func defaultWorktrees() Worktrees { return worktree.New() }

// branchPrefix is what every slice's branch is named under, matching the
// convention the project's own branches already follow.
const branchPrefix = "slice/"

// sliceBranch is the branch a slice's agent works on: its title as a slug under
// slice/. It is derived rather than asked for because nothing records it until
// the agent hands the work back, and it has to be the same string on a
// relaunch — that is what finds the worktree the last session was working in.
//
// A title with no letters or digits in it at all slugs to nothing, so such a
// slice takes its session's name instead: a branch of "slice/" is no branch.
func sliceBranch(s domain.Slice) string {
	if slug := slugify(s.Name); slug != "" {
		return branchPrefix + slug
	}
	return branchPrefix + agent.SessionName(s.ID)
}

// slugify lowercases a title and collapses every run of anything that is not a
// letter or a digit into a single hyphen, with none left at either end. ASCII
// alone counts: the result is a branch name and a directory name after it.
func slugify(name string) string {
	var b strings.Builder
	gap := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if gap && b.Len() > 0 {
				b.WriteByte('-')
			}
			gap = false
			b.WriteRune(r)
		default:
			gap = true
		}
	}
	return b.String()
}

// placement is where a launch puts its agent: the directory the session starts
// in, the branch a worktree there is already on and the checkout it was cut
// from — both empty where there is no worktree — and whatever the status bar
// has to say about it. ok is false only where nothing should be launched at
// all.
type placement struct {
	dir    string
	branch string
	repo   string
	toast  string
	sev    severity
	ok     bool
}

// placeAgent gives a slice's agent a worktree of its own, so it works on its
// own branch in its own directory rather than sharing the one checkout with
// every other agent and with the user.
//
// The branch's existing worktree wins where there is one: a relaunched slice
// wants its work so far, not an empty second copy of the repository.
//
// Two ways out fall back to the shared checkout — a machine with no worktrunk
// on it, and a working directory that is not in a git repository at all — since
// both are launches that worked before there were worktrees and neither is
// anything gone wrong. They say so on the status bar rather than silently,
// because where the agent is working decides what its branch instructions mean.
// A worktrunk that ran and refused is different: something is wrong with the
// repository, and the agent is not launched half-placed on the strength of it.
func placeAgent(w Worktrees, dir string, s domain.Slice) placement {
	shared := func(why string) placement {
		return placement{dir: dir, toast: why + " — the agent runs in the shared checkout.", sev: sevWarning, ok: true}
	}
	if !inRepo(dir) {
		return shared(dir + " is not a git repository")
	}
	branch := sliceBranch(s)
	// A branch worktrunk has no worktree for is the ordinary case — a slice
	// nobody has worked yet — so only the missing binary is read off this.
	if path, err := w.Path(dir, branch); err == nil {
		return placement{dir: path, branch: branch, repo: dir, ok: true}
	} else if errors.Is(err, worktree.ErrNotInstalled) {
		return shared(err.Error())
	}
	path, err := w.Create(dir, branch)
	if err != nil {
		if errors.Is(err, worktree.ErrNotInstalled) {
			return shared(err.Error())
		}
		return placement{toast: fmt.Sprintf("Could not make a worktree for %q: %v.", s.Name, err), sev: sevError}
	}
	return placement{dir: path, branch: branch, repo: dir, ok: true}
}

// inRepo reports whether dir is inside a git working tree, by git's own rule: a
// .git here or in some parent — an entry rather than a directory, since a
// worktree's own is a file. It is a stat rather than a subprocess because its
// whole job is deciding whether to run one.
func inRepo(dir string) bool {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}
