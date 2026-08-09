// Package agent launches Claude Code agents for slices: it builds the prompt a
// fresh agent session starts from, and manages the detached tmux sessions those
// agents run in. One live tmux session per slice, named after the slice's page.
package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// TmuxBinary is the name of the tmux binary, looked up on PATH.
const TmuxBinary = "tmux"

// SessionPrefix namespaces our sessions inside the user's tmux server, so a
// session list can be told apart from whatever else they have running.
const SessionPrefix = "nat-"

// sessionIDLen is how much of a slice's page ID goes into its session name.
// Eight hex digits is what tmux can show without truncating the status line,
// and is far beyond collision range for one project's slices.
const sessionIDLen = 8

// tmuxTimeout caps how long we wait for a tmux command. These are local calls
// to a socket in /tmp, so anything slower than this is a hang.
const tmuxTimeout = 10 * time.Second

// Runner executes a command and returns its standard output. It exists so the
// tmux calls can be faked in tests; there is no mock mode for a subprocess.
type Runner interface {
	Run(name string, args ...string) (string, error)
}

// ExitError reports a command that ran to completion and exited non-zero. The
// exit code is carried separately because tmux uses it to distinguish "no
// server is running" (1) from a genuine failure.
type ExitError struct {
	Code   int
	Stderr string
}

// Error describes the exit code, with the command's own stderr when it wrote
// any — it explains the failure better than we can guess at it.
func (e *ExitError) Error() string {
	if s := strings.TrimSpace(e.Stderr); s != "" {
		return fmt.Sprintf("exit status %d: %s", e.Code, s)
	}
	return fmt.Sprintf("exit status %d", e.Code)
}

// ExecRunner is a Runner backed by real subprocesses.
type ExecRunner struct{}

var _ Runner = ExecRunner{}

// Run executes name with args, returning its standard output. A non-zero exit
// becomes an *ExitError; anything else (a missing binary, say) is returned as
// it came back from os/exec.
func (ExecRunner) Run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
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

// Tmux drives the tmux sessions agents run in.
type Tmux struct {
	runner Runner
}

// NewTmux returns a Tmux driving the real tmux binary on PATH.
func NewTmux() *Tmux { return &Tmux{runner: ExecRunner{}} }

// NewTmuxWithRunner returns a Tmux that executes through r.
func NewTmuxWithRunner(r Runner) *Tmux { return &Tmux{runner: r} }

// SessionName is the tmux session name for a slice's page ID: the prefix plus
// the first eight hex digits of the ID, with the UUID dashes dropped. IDs are
// already hex, but non-hex characters are skipped rather than trusted, so a
// surprising ID cannot produce a name tmux would reject.
func SessionName(slicePageID string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(slicePageID) {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			b.WriteRune(r)
			if b.Len() == sessionIDLen {
				break
			}
		}
	}
	return SessionPrefix + b.String()
}

// LiveSessions is the set of our session names currently running, keyed by
// session name. Sessions that are not ours are filtered out.
//
// tmux exits 1 when no server is running at all, which is the ordinary state
// before the first agent launches — that reads as an empty set, not an error.
func (t *Tmux) LiveSessions() (map[string]bool, error) {
	out, err := t.runner.Run(TmuxBinary, "list-sessions", "-F", "#{session_name}")
	if err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) && exitErr.Code == 1 {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("list tmux sessions: %w", err)
	}

	live := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if strings.HasPrefix(name, SessionPrefix) {
			live[name] = true
		}
	}
	return live, nil
}

// Launch starts a detached tmux session named session, with workdir as its
// working directory, running an agent seeded with the prompt in promptFile.
func (t *Tmux) Launch(session, workdir, promptFile string) error {
	if _, err := t.runner.Run(TmuxBinary, LaunchArgs(session, workdir, promptFile)...); err != nil {
		return fmt.Errorf("launch tmux session %s: %w", session, err)
	}
	return nil
}

// LaunchArgs is the tmux argv for launching an agent session. Assembling it in
// one place keeps the shape of an agent invocation — the binary, its flags, how
// the prompt reaches it — in a single spot, ready for per-project agent
// definitions to vary it later.
//
// The prompt is passed as a positional argument read back from the file rather
// than inlined, so a long prompt cannot run into the argv size limit.
func LaunchArgs(session, workdir, promptFile string) []string {
	return []string{
		"new-session", "-d",
		"-s", session,
		"-c", workdir,
		"sh", "-c", agentCommand(promptFile),
	}
}

// agentCommand is the shell command the session runs: start Claude Code with
// the contents of promptFile as its prompt.
func agentCommand(promptFile string) string {
	return fmt.Sprintf(`claude "$(cat %s)"`, shellQuote(promptFile))
}

// AttachCmd is the command that attaches the terminal to a running session,
// for the caller to hand to tea.ExecProcess.
func AttachCmd(session string) *exec.Cmd {
	return exec.Command(TmuxBinary, "attach-session", "-t", session)
}

// shellQuote wraps s in single quotes so /bin/sh reads it as one literal word,
// escaping any single quotes it contains.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
