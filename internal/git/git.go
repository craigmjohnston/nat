// Package git drives the git binary for the one thing the board asks of it:
// the diff of a slice's handed-back branch, so the work can be read before the
// approve key turns it into a pull request.
//
// It is as thin as [github.com/craigmjohnston/nat/internal/gh] is, and for the
// same reason: git already knows the repository a directory belongs to and
// where the branch is. The Runner seam is its own rather than gh's, because a
// package about git has no business importing the GitHub CLI to borrow a type
// off it.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// Binary is git as it is invoked, found on PATH.
const Binary = "git"

// gitTimeout bounds a call to git. Nearly every call here is a local read, and
// [CLI.Fetch] — the one that goes to the network — is allowed to fail: a fetch
// that outruns this bound is the same as one that could not reach the remote,
// which leaves the repository at the state it last fetched rather than stopping
// anything. The bound matters because the board is waiting on all of it.
const gitTimeout = 30 * time.Second

// DefaultBase is what a slice's branch is diffed against, and what a slice's
// worktree is cut from, when the repository does not say what its default
// branch is. The project's own convention is that every branch is cut from main
// and every pull request merges into it.
const DefaultBase = "main"

// RemoteDefaultBase is origin's own copy of that branch, and the base a
// repository that names no default branch falls back to before the local one:
// see [CLI.fallbackBase] for why the order matters.
const RemoteDefaultBase = "origin/" + DefaultBase

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
// "unknown revision or path not in the working tree", say, which is what tells
// the user the branch is not where nat looked for it.
func (e *ExitError) Error() string {
	if s := firstLine(e.Stderr); s != "" {
		return s
	}
	return fmt.Sprintf("%s exited %d", Binary, e.Code)
}

// firstLine is the first non-empty line of git's stderr. git follows its
// message with hints and usage text, and the message is the part worth showing.
func firstLine(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return s
		}
	}
	return ""
}

// CLI reads diffs through the git binary.
type CLI struct {
	runner Runner
}

// New returns a CLI driving the real git on PATH.
func New() CLI { return CLI{runner: ExecRunner{}} }

// NewWithRunner returns a CLI that executes through r.
func NewWithRunner(r Runner) CLI { return CLI{runner: r} }

// Diff is the change a branch makes to the repository at dir, as a unified
// diff, alongside the base it was taken against.
//
// The base is the repository's own default branch where it names one and main
// where it does not, and the comparison is made against the merge base rather
// than against the tip: what the branch did is the point, not everything the
// base has moved on by since it was cut.
//
// The prefixes are pinned and any external diff driver is refused, because the
// output is going to be parsed rather than shown to a person as it stands, and
// a repository configured with diff.noprefix or a diff driver of its own would
// otherwise hand back something else entirely.
func (c CLI) Diff(dir, branch string) (base, diff string, err error) {
	base = c.Base(dir)
	out, err := c.runner.Run(dir, Binary, "diff", "--no-color", "--no-ext-diff",
		"--src-prefix=a/", "--dst-prefix=b/", "--merge-base", base, branch)
	if err != nil {
		logging.Error("could not read the diff of a branch", "dir", dir, "branch", branch,
			"base", base, "error", err)
		return base, "", err
	}
	return base, out, nil
}

// Show is a file's own lines as the branch leaves it, which is what fills the
// gaps a unified diff leaves between its hunks: git shows a few lines of context
// around each change and the rest of the file is simply absent, so the only
// place the skipped lines can come from is the file itself.
//
// The path is the one the diff names on the branch's side, and textconv is
// refused for the reason [CLI.Diff] refuses an external diff driver: what comes
// back is lined up against the diff's own numbers rather than shown to a person,
// and a repository configured to render a file through a filter would hand back
// something with no such lines in it.
//
// A file the branch does not have — one the change deleted — is a refusal, and
// so is a path git will not resolve. Neither is worth dressing up: the caller
// has a diff either way, and only the expanding around it goes without.
func (c CLI) Show(dir, branch, path string) ([]string, error) {
	out, err := c.runner.Run(dir, Binary, "show", "--no-textconv", branch+":"+path)
	if err != nil {
		logging.Action("could not read a file's content at a branch", "dir", dir,
			"branch", branch, "path", path, "error", err)
		return nil, err
	}
	return splitLines(out), nil
}

// Base is the branch a diff is taken against, and the one a slice's worktree is
// cut from: whatever origin's HEAD points at, which is the repository's default
// branch as the clone last recorded it, and [CLI.fallbackBase] when there is no
// such ref to read. A failure here is logged and swallowed rather than returned:
// the fallback is right for every repository this project works on, and refusing
// to show a diff over it would be worse than showing one against main.
//
// The ref is missing more often than it sounds: git writes it at clone time and
// nothing maintains it afterwards, so a checkout made any other way — or one it
// was pruned from — has none, and only `git remote set-head origin --auto` puts
// it back.
func (c CLI) Base(dir string) string {
	out, err := c.runner.Run(dir, Binary, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		logging.Action("could not read the repository's default branch", "dir", dir, "error", err)
		return c.fallbackBase(dir)
	}
	if ref := strings.TrimSpace(out); ref != "" {
		return ref
	}
	logging.Action("the repository names no default branch", "dir", dir)
	return c.fallbackBase(dir)
}

// fallbackBase is the base for a repository whose origin/HEAD says nothing:
// origin's own copy of the project's default branch where there is one, and the
// local branch only where there is not.
//
// The remote ref first, because the local one is the very staleness [CLI.Fetch]
// runs before a worktree to avoid. A shared checkout the user has not pulled in
// a fortnight has a `main` a fortnight behind whatever the fetch just brought
// down, and a slice cut from it starts life behind by exactly that much —
// which is no better than not fetching at all. `origin/main` is current the
// moment the fetch returns.
//
// The local branch is kept for the one repository the remote ref cannot serve:
// one with no origin at all, where it is the only thing there is to cut from
// or diff against. A repository whose default branch is neither main nor named
// by origin/HEAD is beyond guessing, and gets the project's own convention.
func (c CLI) fallbackBase(dir string) string {
	// The full ref rather than the short name, since a local branch may be
	// called origin/main and rev-parse would answer for that one instead.
	_, err := c.runner.Run(dir, Binary, "rev-parse", "--verify", "--quiet",
		"refs/remotes/"+RemoteDefaultBase)
	if err != nil {
		logging.Action("no "+RemoteDefaultBase+" either; falling back to "+DefaultBase,
			"dir", dir, "error", err)
		return DefaultBase
	}
	logging.Action("falling back to "+RemoteDefaultBase, "dir", dir)
	return RemoteDefaultBase
}

// Fetch brings origin's refs up to date, so [CLI.Base] names a tip that is
// current rather than whatever the checkout last happened to hear about. It is
// what a worktree is cut after: a branch based on a stale origin/main starts
// life behind, and nothing later in the slice puts that right.
//
// Nothing is returned. A fetch that fails — no network, no origin, a remote
// that refused — is logged and swallowed, because working against the refs as
// last fetched is what every offline git command already does, and it is far
// better than refusing to launch an agent over it.
func (c CLI) Fetch(dir string) {
	if _, err := c.runner.Run(dir, Binary, "fetch", "origin"); err != nil {
		logging.Action("could not fetch origin; working from the refs as last fetched",
			"dir", dir, "error", err)
	}
}
