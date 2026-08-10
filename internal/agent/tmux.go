// Package agent launches Claude Code agents for slices: it builds the prompt a
// fresh agent session starts from, and manages the detached tmux sessions those
// agents run in. One live tmux session per slice, labelled after the slice's
// page and tagged with its full ID.
package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// TmuxBinary is the name of the tmux binary, looked up on PATH.
const TmuxBinary = "tmux"

// SessionPrefix namespaces our sessions inside the user's tmux server, so a
// session list can be told apart from whatever else they have running.
const SessionPrefix = "nat-"

// sessionIDLen is how much of a slice's page ID goes into its session name.
// Eight hex digits is what tmux can show without truncating the status line.
const sessionIDLen = 8

// SlicePaneOption is the tmux pane option an agent's pane is tagged with, and
// holds the full page ID of the slice that agent is working. It, not the
// session name, is what a running agent is identified by: page IDs in one
// Notion workspace share a long prefix, so any short slug of one is a name
// several slices answer to. Tagging the pane also survives the pane being
// moved into another session.
const SlicePaneOption = "@nat_slice"

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
// the last eight hex digits of the ID, with the UUID dashes dropped. IDs are
// already hex, but non-hex characters are skipped rather than trusted, so a
// surprising ID cannot produce a name tmux would reject.
//
// The tail rather than the head, because page IDs made in one workspace share
// a long leading prefix: taken from the front, every slice of a project names
// the same session, and the second launch is refused as a duplicate. The name
// is only a human label — what a session belongs to is read from
// [SlicePaneOption] — but it still has to be one tmux will accept twice.
func SessionName(slicePageID string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(slicePageID) {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			b.WriteRune(r)
		}
	}
	hex := b.String()
	if len(hex) > sessionIDLen {
		hex = hex[len(hex)-sessionIDLen:]
	}
	return SessionPrefix + hex
}

// LiveSlices maps the page ID of every slice with an agent running to the tmux
// session that agent's pane is currently in — the ID to mark the slice by, and
// the session to attach to it with. Panes that are not ours carry no slice tag
// and are left out.
//
// It is a server-wide scan of the panes rather than a look at the session
// names, because the tag is the identity: a pane the user has moved or renamed
// their way is still the agent for its slice.
//
// tmux exits 1 when no server is running at all, which is the ordinary state
// before the first agent launches — that reads as an empty set, not an error.
func (t *Tmux) LiveSlices() (map[string]string, error) {
	out, err := t.runner.Run(TmuxBinary, "list-panes", "-a", "-F", listPanesFormat())
	if err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) && exitErr.Code == 1 {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("list tmux panes: %w", err)
	}

	live := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		id, session, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok || id == "" {
			continue
		}
		// A slice with two panes tagged for it should not happen, and if it
		// does the first one found is as good an answer as the last.
		if _, seen := live[id]; !seen {
			live[id] = session
		}
	}
	return live, nil
}

// listPanesFormat is what [Tmux.LiveSlices] asks tmux to print for each pane:
// the slice tag, then the session the pane is in. A tab separates them because
// a session name can hold anything but that.
func listPanesFormat() string {
	return fmt.Sprintf("#{%s}\t#{session_name}", SlicePaneOption)
}

// Launch starts a detached tmux session named session, with workdir as its
// working directory, running an agent seeded with the prompt in promptFile for
// the slice with page ID sliceID.
//
// The pane the session starts in is tagged with sliceID, which is what
// [Tmux.LiveSlices] reads the running agents back out of. A session whose pane
// could not be tagged is left running — its agent is already working — but the
// failure is reported, because until it is tagged nothing will find it again.
func (t *Tmux) Launch(session, workdir, promptFile, sliceID string) error {
	out, err := t.runner.Run(TmuxBinary, LaunchArgs(session, workdir, promptFile)...)
	if err != nil {
		return fmt.Errorf("launch tmux session %s: %w", session, err)
	}
	pane := strings.TrimSpace(out)
	if _, err := t.runner.Run(TmuxBinary, "set-option", "-p", "-t", pane, SlicePaneOption, sliceID); err != nil {
		return fmt.Errorf("tag tmux pane %s for slice %s: %w", pane, sliceID, err)
	}
	return nil
}

// LaunchArgs is the tmux argv for launching an agent session. Assembling it in
// one place keeps the shape of an agent invocation — the binary, its flags, how
// the prompt reaches it — in a single spot, ready for per-project agent
// definitions to vary it later.
//
// The prompt is passed as a positional argument read back from the file rather
// than inlined, so a long prompt cannot run into the argv size limit. The new
// session prints its pane's ID, which is the handle the slice tag goes on:
// pane IDs are unique for the life of the server, where a name is whatever it
// has last been set to.
func LaunchArgs(session, workdir, promptFile string) []string {
	return []string{
		"new-session", "-d",
		"-s", session,
		"-c", workdir,
		"-P", "-F", "#{pane_id}",
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

// AttachCmd is the command attaching to a session, as a method so a caller
// that already holds a Tmux can launch and attach through the one thing.
func (t *Tmux) AttachCmd(session string) *exec.Cmd { return AttachCmd(session) }

// WritePromptFile writes an agent's opening prompt somewhere the session it is
// launched for can read it back, returning the file's path.
//
// The file goes in a directory of its own rather than at a predictable path in
// the shared temp dir: the agent obeys whatever it reads, so a file another
// user could have put there first would be an instruction we did not write.
func WritePromptFile(session, prompt string) (string, error) {
	dir, err := os.MkdirTemp("", "nat-prompt-")
	if err != nil {
		return "", fmt.Errorf("create prompt dir: %w", err)
	}
	return writePromptInto(dir, session, prompt)
}

// writePromptInto writes the prompt file inside dir. It is split out so that a
// write which fails — a directory that cannot be written to — is exercisable
// without arranging for MkdirTemp itself to succeed and then break.
func writePromptInto(dir, session, prompt string) (string, error) {
	path := filepath.Join(dir, session+".md")
	if err := os.WriteFile(path, []byte(prompt), 0o600); err != nil {
		return "", fmt.Errorf("write prompt file: %w", err)
	}
	return path, nil
}

// shellQuote wraps s in single quotes so /bin/sh reads it as one literal word,
// escaping any single quotes it contains.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
