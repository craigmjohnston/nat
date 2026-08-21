// Package gh drives the GitHub CLI. Agents hand a slice back as a pushed
// branch and open nothing themselves, so the one thing nat asks of gh is to
// turn such a branch into a pull request once the work has been reviewed.
//
// It is a thin wrapper on purpose: gh already knows which repository a
// directory belongs to, which remote to push at and how to authenticate, and
// none of that is worth reimplementing here.
package gh

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

// Binary is the GitHub CLI as it is invoked, found on PATH.
const Binary = "gh"

// ghTimeout bounds a call to gh. Opening a pull request is a round trip to
// GitHub rather than a local read, so it is generous compared with the tmux
// layer's — but it is bounded, because the board is waiting on it.
const ghTimeout = 60 * time.Second

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
// gh that is not installed, say — is returned as os/exec reported it.
func (ExecRunner) Run(dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
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

// ExitError is a gh that ran and refused: its exit code, and whatever it wrote
// to stderr on the way out.
type ExitError struct {
	Code   int
	Stderr string
}

// Error describes the exit code, with gh's own message when it wrote one —
// "a pull request for branch X already exists", say, which is the whole reason
// the failure is worth showing.
func (e *ExitError) Error() string {
	if s := firstLine(e.Stderr); s != "" {
		return s
	}
	return fmt.Sprintf("gh exited %d", e.Code)
}

// firstLine is the first non-empty line of gh's stderr. gh follows its message
// with usage text on a misuse, and the message is the part worth a toast.
func firstLine(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return s
		}
	}
	return ""
}

// CLI opens pull requests through the gh binary.
type CLI struct {
	runner Runner
}

// New returns a CLI driving the real gh on PATH.
func New() CLI { return CLI{runner: ExecRunner{}} }

// NewWithRunner returns a CLI that executes through r.
func NewWithRunner(r Runner) CLI { return CLI{runner: r} }

// CreatePR opens a pull request for an already pushed branch of the repository
// at dir, and returns its URL.
//
// The title and body are the description the agent wrote at hand-back, read off
// the slice's Notion page by the caller. An empty title is a hand-back that left
// none — every one written before there was a flag for it — and gh fills the
// pull request from the branch's commits instead (--fill), which is what it
// always did; either way nothing is asked for at a prompt, since a prompt is not
// something a board key can answer. The base is left to gh, which uses the
// repository's own default branch.
func (c CLI) CreatePR(dir, branch, title, body string) (string, error) {
	args := []string{"pr", "create", "--head", branch, "--fill"}
	if title != "" {
		args = []string{"pr", "create", "--head", branch, "--title", title, "--body", body}
	}
	out, err := c.runner.Run(dir, Binary, args...)
	if err != nil {
		logging.Error("could not open a pull request", "dir", dir, "branch", branch, "error", err)
		return "", err
	}
	url := prURL(out)
	if url == "" {
		logging.Error("gh opened a pull request but printed no URL", "dir", dir, "branch", branch)
		return "", fmt.Errorf("%s pr create printed no pull request URL", Binary)
	}
	logging.Action("pull request opened", "dir", dir, "branch", branch, "url", url)
	return url, nil
}

// prURL is the pull request gh reported: the last URL it printed. gh writes
// its progress to stderr and the URL alone to stdout, but it has also been
// known to prepend a line about the branch it pushed, so the last one wins
// rather than the first.
func prURL(out string) string {
	var url string
	for _, line := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(line); strings.HasPrefix(s, "https://") {
			url = s
		}
	}
	return url
}
