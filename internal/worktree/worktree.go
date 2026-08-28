// Package worktree gives a slice's agent a git worktree of its own, so it works
// on its own branch in its own directory rather than sharing the project's one
// checkout with every other agent and with the user.
//
// It drives the git binary directly and is as thin as
// [github.com/craigmjohnston/nat/internal/gh] and
// [github.com/craigmjohnston/nat/internal/git] are: git already knows which
// repository a directory belongs to, where each of its worktrees is and when a
// branch is safe to delete, and none of that is worth reimplementing here. The
// Runner seam is its own rather than either of theirs, for the reason git's is
// not gh's.
//
// The one thing git has no opinion about is where a new worktree goes, so that
// convention is nat's: a sibling <repo>.worktrees directory, one entry per
// branch. It has to be a rule rather than a choice, because a relaunched slice
// finds the worktree the last session was working in by asking git for it —
// [CLI.Path] — and cuts a fresh one only where there is none.
//
// A missing git binary is its own error ([ErrNotInstalled]) rather than one
// failure among many, because it is the one the caller recovers from: an agent
// launched on a machine that cannot cut a worktree runs in the shared checkout
// the way every agent did before there were worktrees, and that is a decision
// only a distinguishable error can drive.
package worktree

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// Binary is git as it is invoked, found on PATH.
const Binary = "git"

// gitTimeout bounds a call to git. Every call here is local, but cutting a
// worktree is a checkout and runs the repository's own hooks on the way — an
// install, a build — so it is given the room the GitHub CLI gets rather than
// the tighter bound [github.com/craigmjohnston/nat/internal/git] reads with.
const gitTimeout = 60 * time.Second

// dirSuffix names the directory a repository's worktrees are kept in, beside
// the repository itself: /repos/nat has them under /repos/nat.worktrees. A
// sibling rather than a child, so nothing nat cuts ever appears inside the
// checkout the user works in or the diffs taken from it.
const dirSuffix = ".worktrees"

// ErrNotInstalled is a machine with no git on it. It is wrapped around os/exec's
// own report rather than replacing it, so a caller may test for either, and it
// is what the launch path falls back to the shared checkout on.
var ErrNotInstalled = errors.New(Binary + " is not installed")

// Runner runs a command in a working directory and returns its standard
// output. It is the seam the tests replace: the real one starts a subprocess.
type Runner interface {
	Run(dir, name string, args ...string) (string, error)
}

// ExecRunner is a Runner backed by real subprocesses.
type ExecRunner struct{}

var _ Runner = ExecRunner{}

// Run executes name with args in dir, returning its standard output. A
// non-zero exit becomes an error carrying what the command wrote to stderr,
// which explains the failure better than the exit code does; anything else — a
// git that is not installed, say — is returned as os/exec reported it.
func (ExecRunner) Run(dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return stdout.String(), &ExitError{Code: exitErr.ExitCode(), Stderr: stderr.String()}
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

// ExitError is a git that ran and refused: its exit code, and whatever it wrote
// to stderr on the way out.
type ExitError struct {
	Code   int
	Stderr string
}

// Error describes the exit code, with git's own message when it wrote one —
// "contains modified or untracked files, use --force to delete it", say, which
// is the whole reason the refusal is worth showing rather than swallowing.
func (e *ExitError) Error() string {
	if s := firstLine(e.Stderr); s != "" {
		return s
	}
	return fmt.Sprintf("%s exited %d", Binary, e.Code)
}

// firstLine is the first non-empty line of git's stderr. git follows its
// message with the hint that would fix it and, on a misuse, its usage text; the
// message is the part worth showing.
func firstLine(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return s
		}
	}
	return ""
}

// classify turns os/exec's missing-binary report into [ErrNotInstalled],
// wrapping rather than replacing it, and leaves every other failure — an exit,
// a timeout — exactly as it arrived.
func classify(err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%w: %w", ErrNotInstalled, err)
	}
	return err
}

// CLI manages worktrees through the git binary.
type CLI struct {
	runner Runner
}

// New returns a CLI driving the real git on PATH.
func New() CLI { return CLI{runner: ExecRunner{}} }

// NewWithRunner returns a CLI that executes through r.
func NewWithRunner(r Runner) CLI { return CLI{runner: r} }

// Create cuts branch as a new worktree of the repository at dir, based on base,
// and returns the path it put it at.
//
// The base is the caller's to resolve and is passed through as it stands, which
// is what keeps this wrapper thin: what the right ref is — the remote's default
// branch, and how current it is — is a question about the project rather than
// about git. An empty base says nothing at all, and leaves git to cut from the
// commit the repository is on, which is what it does with no start point given.
//
// The path is nat's convention rather than anything git decides — see
// [dirSuffix] — and is derived from the branch, so it is the same path every
// time the same slice is launched.
//
// A branch that already exists is checked out rather than cut again, and the
// base is not consulted at all: it is the branch a previous session pushed and
// left behind, and its commits are exactly what a relaunch wants. That is not a
// rare case — a merged slice keeps its branch whenever [CLI.Remove] cannot
// delete it, which a squash merge always leaves it unable to do.
func (c CLI) Create(dir, branch, base string) (string, error) {
	root, err := c.root(dir)
	if err != nil {
		logging.Error("could not find the repository a worktree would go beside",
			"dir", dir, "branch", branch, "error", err)
		return "", err
	}
	path := filepath.Join(root+dirSuffix, pathSlug(branch))

	args := []string{"worktree", "add", path}
	if c.hasBranch(dir, branch) {
		args = append(args, branch)
	} else {
		args = append(args, "-b", branch)
		if base != "" {
			args = append(args, base)
		}
	}
	if _, err := c.runner.Run(dir, Binary, args...); err != nil {
		err = classify(err)
		logging.Error("could not create a worktree", "dir", dir, "branch", branch,
			"base", base, "path", path, "error", err)
		return "", err
	}
	logging.Action("worktree created", "dir", dir, "branch", branch, "base", base, "path", path)
	return path, nil
}

// Remove takes the worktree for branch off the repository at dir, and then the
// branch itself.
//
// What removal refuses is git's own rule rather than one imposed here: a
// worktree holding modified or untracked files is refused outright, which is
// right, since a refusal is recoverable and work thrown away is not. That
// refusal is what comes back, in git's own words.
//
// The branch is deleted with the safe form, so git deletes it only where its
// commits are reachable from somewhere else, and the refusal is logged and
// swallowed: a squash merge leaves a branch whose commits are nowhere else by
// that test, and a leftover branch costs nothing — [CLI.Create] checks an
// existing one out rather than tripping over it.
func (c CLI) Remove(dir, branch string) error {
	path, err := c.Path(dir, branch)
	if err != nil {
		return err
	}
	if _, err := c.runner.Run(dir, Binary, "worktree", "remove", path); err != nil {
		err = classify(err)
		logging.Error("could not remove a worktree", "dir", dir, "branch", branch,
			"path", path, "error", err)
		return err
	}
	if _, err := c.runner.Run(dir, Binary, "branch", "-d", branch); err != nil {
		logging.Action("the worktree is gone but its branch is not; leaving it",
			"dir", dir, "branch", branch, "error", err)
	}
	logging.Action("worktree removed", "dir", dir, "branch", branch, "path", path)
	return nil
}

// Path is where the repository at dir keeps branch's worktree, read off
// `git worktree list --porcelain` — the machine-readable form of the listing,
// which names each worktree's path and the branch it has checked out. A branch
// with no worktree has no path, and is reported as such rather than as an empty
// one: that is the ordinary answer for a slice nobody has worked yet.
func (c CLI) Path(dir, branch string) (string, error) {
	out, err := c.runner.Run(dir, Binary, "worktree", "list", "--porcelain")
	if err != nil {
		err = classify(err)
		logging.Error("could not list worktrees", "dir", dir, "error", err)
		return "", err
	}
	if path := worktreeOf(out, branch); path != "" {
		return path, nil
	}
	return "", fmt.Errorf("%s names no worktree for %s", Binary, branch)
}

// worktreeOf finds branch's path in a porcelain listing: one record per
// worktree, opened by its path and naming the branch it has checked out as a
// full ref — a detached or bare one names none at all, and so matches nothing.
func worktreeOf(list, branch string) string {
	ref := branch
	if !strings.HasPrefix(ref, "refs/") {
		ref = "refs/heads/" + branch
	}
	var path string
	for _, line := range strings.Split(list, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			path = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		case strings.HasPrefix(line, "branch "):
			if strings.TrimSpace(strings.TrimPrefix(line, "branch ")) == ref {
				return path
			}
		}
	}
	return ""
}

// root is the repository a worktree would go beside: the directory holding the
// git directory every worktree of the repository shares. The common one rather
// than dir's own, so a nat launched from inside one worktree cuts the next one
// beside the repository rather than beside that worktree.
func (c CLI) root(dir string) (string, error) {
	out, err := c.runner.Run(dir, Binary, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", classify(err)
	}
	common := strings.TrimSpace(out)
	if common == "" {
		return "", fmt.Errorf("%s names no repository at %s", Binary, dir)
	}
	return filepath.Dir(common), nil
}

// hasBranch reports whether the repository at dir already has branch. It is a
// question rather than a failure to recover from: a git that will not answer is
// one the create is about to fail on anyway, and with its own message.
func (c CLI) hasBranch(dir, branch string) bool {
	_, err := c.runner.Run(dir, Binary, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// pathSlug is a branch as one directory name: every run of anything that is not
// a letter, a digit, a dot, a hyphen or an underscore collapsed to a single
// hyphen, with none left at either end, so slice/worktrees becomes
// slice-worktrees. A branch of nothing but such characters would name no
// directory at all, and takes a constant instead.
func pathSlug(branch string) string {
	var b strings.Builder
	gap := false
	for _, r := range branch {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			if gap && b.Len() > 0 {
				b.WriteByte('-')
			}
			gap = false
			b.WriteRune(r)
		default:
			gap = true
		}
	}
	if s := strings.Trim(b.String(), ".-_"); s != "" {
		return s
	}
	return "worktree"
}
