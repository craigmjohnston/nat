package actions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// Launcher is what a launch needs of tmux: one detached session, started to
// read the prompt file already written for it. It is narrower than the
// board's own agent launcher — which also attaches to a session, says
// something to one, and reports which are running — because none of that is
// anything a launch itself does.
type Launcher interface {
	Launch(session, workdir, promptFile, sliceID string, model config.AgentModel) error
}

// LaunchResult is what Launch produced: the prompt context as actually
// placed — its working directory, branch and repo filled in by the worktree
// the agent was given — and the session it was started under. Session is
// empty where nothing was launched at all, a worktree that could not be cut
// or a claim that was refused, which is exactly what Toast is the whole
// report of.
type LaunchResult struct {
	Context agent.PromptContext
	Session string
	Toast   string
	Sev     Severity
}

// Launch gives the agent a worktree, writes its prompt out, claims the slice
// and starts the detached session that reads it. The claim is the board's
// rather than the agent's: a fresh Claude Code takes seconds to get as far as
// start-slice, and a row that reads Todo all the while is one the user can
// launch a second agent on. The prompt still tells the agent to run
// start-slice, which re-opens the slice it was launched on and prints the
// brief — and which refuses where somebody else holds it, so a race is
// settled before any agent is handed a brief.
//
// The claim goes last of the things that can fail before tmux is asked for
// anything, so a worktree that could not be cut or a prompt that could not be
// written leaves the slice where it was; a launch that fails after it leaves
// the slice in progress with no session, which is the state a release
// undoes. It is reported as a toast rather than a Go error, for the reason a
// worktree failure is: nothing has gone wrong with the board, and the slice
// is still there to launch.
//
// The worktree is resolved here rather than by the caller because it fetches
// origin and then cuts the worktree, which is a checkout and runs the
// repository's hooks over it: a launch is already the slow key, and this is
// the goroutine it is slow in. Its answer is the working directory the
// prompt is written with and the session is started in, so the two never
// disagree about where the agent is.
func Launch(ctx context.Context, l Launcher, w Worktrees, r Repo, client Client, assigneeID string,
	c agent.PromptContext, m config.AgentModel) (LaunchResult, error) {
	p := PlaceAgent(w, r, c.WorkingDir, c.Slice)
	if !p.OK {
		return LaunchResult{Toast: p.Toast, Sev: p.Sev}, nil
	}
	c.WorkingDir, c.Branch, c.Repo = p.Dir, p.Branch, p.Repo
	session := agent.SessionName(c.Slice.ID)
	file, err := agent.WritePromptFile(session, agent.Prompt(c))
	if err != nil {
		return LaunchResult{}, fmt.Errorf("launch agent: %w", err)
	}
	if err := ClaimSlice(ctx, client, c.Slice, assigneeID); err != nil {
		return LaunchResult{Toast: fmt.Sprintf("Could not %v — no agent was launched.", err), Sev: SevError}, nil
	}
	if err := l.Launch(session, c.WorkingDir, file, c.Slice.ID, m); err != nil {
		return LaunchResult{}, err
	}
	return LaunchResult{Context: c, Session: session, Toast: p.Toast, Sev: p.Sev}, nil
}

// WorkdirFor is the directory a slice's agent starts in: its own repo
// override, or the project's default.
func WorkdirFor(s domain.Slice, p config.ProjectConfig) string {
	if s.Repo != "" {
		return s.Repo
	}
	return p.WorkingDir
}

// TrimModel is the model pair as a launch sends it: what the user typed, with
// the spaces around it gone, since a flag value of " sonnet" is not one
// Claude Code answers to.
func TrimModel(m config.AgentModel) config.AgentModel {
	return config.AgentModel{
		Model:  strings.TrimSpace(m.Model),
		Effort: strings.TrimSpace(m.Effort),
	}
}

// ExpandHome expands a leading ~ to the user's home directory. tmux is
// handed the path as-is, and the shell that would otherwise expand it never
// sees it.
func ExpandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

// ExistingDir validates a launch's working directory. A session started
// somewhere that is not there fails inside tmux, where nobody is looking, so
// the directory is checked while there is still a form — or a command line —
// to say so on.
func ExistingDir(path string) error {
	dir := ExpandHome(strings.TrimSpace(path))
	if dir == "" {
		return errors.New("the agent needs a working directory")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("%s is not there", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	return nil
}
