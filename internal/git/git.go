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

// gitTimeout bounds a call to git. Every call here is a local read — no fetch,
// no network — so it is tighter than the GitHub CLI's, but it is bounded all
// the same, because the board is waiting on it.
const gitTimeout = 30 * time.Second

// DefaultBase is what a slice's branch is diffed against when the repository
// does not say what its default branch is. The project's own convention is that
// every branch is cut from main and every pull request merges into it.
const DefaultBase = "main"

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

// Base is the branch a diff is taken against: whatever origin's HEAD points at,
// which is the repository's default branch as the clone last recorded it, and
// [DefaultBase] when there is no such ref to read — a repository with no origin
// at all, or one cloned without it. A failure here is logged and swallowed
// rather than returned: the fallback is right for every repository this project
// works on, and refusing to show a diff over it would be worse than showing one
// against main.
func (c CLI) Base(dir string) string {
	out, err := c.runner.Run(dir, Binary, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		logging.Action("could not read the repository's default branch; diffing against "+DefaultBase,
			"dir", dir, "error", err)
		return DefaultBase
	}
	if ref := strings.TrimSpace(out); ref != "" {
		return ref
	}
	return DefaultBase
}
