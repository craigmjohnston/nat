// Package worktree drives the worktrunk CLI, so a slice's agent can be given a
// git worktree of its own rather than made to share the project's one checkout
// with every other agent and with the user.
//
// It is as thin as [github.com/craigmjohnston/nat/internal/gh] and
// [github.com/craigmjohnston/nat/internal/git] are, and for the same reason:
// worktrunk already knows where a repository keeps its worktrees, how to cut a
// branch from the default one and when a branch is safe to delete, and none of
// that is worth reimplementing here. The Runner seam is its own rather than
// either of theirs, for the reason git's is not gh's.
//
// A missing wt binary is its own error ([ErrNotInstalled]) rather than one
// failure among many, because it is the one the caller recovers from: an agent
// launched on a machine without worktrunk runs in the shared checkout the way
// every agent did before there were worktrees, and that is a decision only a
// distinguishable error can drive.
package worktree

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/craigmjohnston/nat/internal/logging"
)

// Binary is worktrunk as it is invoked, found on PATH. The shell function of
// the same name that worktrunk installs is what changes the caller's directory
// after a switch; a subprocess started here reaches the binary underneath it,
// which is why every command is told not to bother changing directory at all.
const Binary = "wt"

// wtTimeout bounds a call to worktrunk. Creating a worktree is a local
// checkout, but it runs the repository's own hooks on the way — an install, a
// build — so it is given the room the GitHub CLI gets rather than git's.
const wtTimeout = 60 * time.Second

// ErrNotInstalled is a machine with no worktrunk on it. It is wrapped around
// os/exec's own report rather than replacing it, so a caller may test for
// either, and it is what the launch path falls back to the shared checkout on.
var ErrNotInstalled = errors.New("worktrunk (" + Binary + ") is not installed")

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
// worktrunk that is not installed, say — is returned as os/exec reported it.
func (ExecRunner) Run(dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), wtTimeout)
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

// ExitError is a worktrunk that ran and refused: its exit code, and whatever it
// wrote to stderr on the way out.
type ExitError struct {
	Code   int
	Stderr string
}

// Error describes the exit code, with worktrunk's own message when it wrote
// one — "worktree has uncommitted changes", say, which is the whole reason the
// refusal is worth showing rather than swallowing.
func (e *ExitError) Error() string {
	if s := firstLine(e.Stderr); s != "" {
		return s
	}
	return fmt.Sprintf("%s exited %d", Binary, e.Code)
}

// firstLine is the first non-empty line of worktrunk's stderr. worktrunk
// follows its message with the hint that would fix it and, on a misuse, its
// usage text; the message is the part worth showing.
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

// CLI manages worktrees through the worktrunk binary.
type CLI struct {
	runner Runner
}

// New returns a CLI driving the real worktrunk on PATH.
func New() CLI { return CLI{runner: ExecRunner{}} }

// NewWithRunner returns a CLI that executes through r.
func NewWithRunner(r Runner) CLI { return CLI{runner: r} }

// Create cuts branch as a new worktree of the repository at dir, based on base,
// and returns the path worktrunk put it at.
//
// The base is the caller's to resolve and is passed through as it stands
// (--base), which is what keeps this wrapper thin: what the right ref is — the
// remote's default branch, and how current it is — is a question about the
// project rather than about worktrunk. An empty base says nothing at all, and
// leaves worktrunk to its own answer.
//
// The switch is told not to change directory (--no-cd) because there is no
// shell here for it to change, and to skip approvals (-y) because there is
// nobody at this end to answer one. The path is read back with a second call
// rather than parsed out of the first: switch prints for a person, and
// [CLI.Path] asks worktrunk for JSON.
func (c CLI) Create(dir, branch, base string) (string, error) {
	args := []string{"switch", "--create", branch, "--no-cd", "-y"}
	if base != "" {
		args = append(args, "--base", base)
	}
	if _, err := c.runner.Run(dir, Binary, args...); err != nil {
		err = classify(err)
		logging.Error("could not create a worktree", "dir", dir, "branch", branch, "base", base, "error", err)
		return "", err
	}
	path, err := c.Path(dir, branch)
	if err != nil {
		return "", err
	}
	logging.Action("worktree created", "dir", dir, "branch", branch, "base", base, "path", path)
	return path, nil
}

// Remove takes the worktree for branch off the repository at dir.
//
// What that means is worktrunk's own rule rather than one imposed here: the
// branch is deleted only where it has been merged, and a worktree with
// uncommitted changes in it is refused outright — which is right, since a
// refusal is recoverable and work thrown away is not. The refusal is what a
// caller waits for and gets: worktrunk checks before it does anything and only
// then takes the removal itself into the background, so a nil error here means
// the worktree is going rather than already gone.
func (c CLI) Remove(dir, branch string) error {
	if _, err := c.runner.Run(dir, Binary, "remove", branch, "-y"); err != nil {
		err = classify(err)
		logging.Error("could not remove a worktree", "dir", dir, "branch", branch, "error", err)
		return err
	}
	logging.Action("worktree removed", "dir", dir, "branch", branch)
	return nil
}

// Path is where the repository at dir keeps branch's worktree, read off
// `wt list --format json` — the one machine-readable answer worktrunk gives.
// A branch worktrunk knows about but has no worktree for has no path, and is
// reported as such rather than as an empty one.
func (c CLI) Path(dir, branch string) (string, error) {
	out, err := c.runner.Run(dir, Binary, "list", "--format", "json")
	if err != nil {
		err = classify(err)
		logging.Error("could not list worktrees", "dir", dir, "error", err)
		return "", err
	}
	var entries []struct {
		Branch string `json:"branch"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		logging.Error("could not read the worktree list", "dir", dir, "error", err)
		return "", fmt.Errorf("%s list printed no readable worktrees: %w", Binary, err)
	}
	for _, e := range entries {
		if e.Branch == branch && e.Path != "" {
			return e.Path, nil
		}
	}
	return "", fmt.Errorf("%s list names no worktree for %s", Binary, branch)
}
