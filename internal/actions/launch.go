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
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/store"
)

// Launcher is what a launch needs of tmux: one detached session, started to
// read the prompt file already written for it. It is narrower than the
// board's own agent launcher — which also attaches to a session, says
// something to one, and reports which are running — because none of that is
// anything a launch itself does.
type Launcher interface {
	Launch(session, workdir, promptFile, sliceID string, model config.AgentModel) error
}

// PRReviewReader is what a fix launch needs of gh to gather the review's
// state at launch time: the exact two reads its own prompt already permits
// the agent to run again itself. Narrower than gh.CLI, the way every other
// seam here is. A caller that never launches a fix session — headless
// slice-launch, which refuses a Done slice outright — has nothing to drive
// it with and passes nil; [Launch] never calls it outside c.Fix.
type PRReviewReader interface {
	ReviewComments(dir, ref string) (string, error)
	Checks(dir, ref string) (string, error)
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

// Launch gives the agent a worktree, claims the slice, fetches its brief and
// writes the opening prompt with it inline, then starts the detached session
// that reads that prompt. The claim is the board's rather than the agent's: a
// fresh Claude Code takes seconds to start, and a row that reads Todo all the
// while is one the user can launch a second agent on. Fetching the brief here
// rather than leaving the agent to read it with its own start-slice is what
// lets the prompt say nothing about running that command at all — the agent
// is simply told the slice, its body and the project's conventions, the same
// document start-slice would have printed.
//
// The claim goes before the brief is read and before tmux is asked for
// anything, so a refused claim — somebody else already holds the slice —
// stops the launch with the slice untouched, and a worktree that could not
// be cut or a prompt that could not be written leaves it exactly as it was
// too; a launch that fails after the claim leaves the slice in progress with
// no session, which is the state a release undoes. Both are reported as a
// toast rather than a Go error, for the reason a worktree failure is:
// nothing has gone wrong with the board, and the slice is still there to
// launch.
//
// The worktree is resolved here rather than by the caller because it fetches
// origin and then cuts the worktree, which is a checkout and runs the
// repository's hooks over it: a launch is already the slow key, and this is
// the goroutine it is slow in. Its answer is the working directory the
// prompt is written with and the session is started in, so the two never
// disagree about where the agent is.
func Launch(ctx context.Context, l Launcher, w Worktrees, r Repo, st Store, viewer PRReviewReader, assigneeID string,
	c agent.PromptContext, m config.AgentModel) (LaunchResult, error) {
	p := PlaceAgent(w, r, c.WorkingDir, c.Slice)
	if !p.OK {
		return LaunchResult{Toast: p.Toast, Sev: p.Sev}, nil
	}
	c.WorkingDir, c.Branch, c.Repo = p.Dir, p.Branch, p.Repo
	// A resume or a fix launch has commits already on the branch worth
	// reading; a first-time launch has nothing yet to gather.
	if c.Branch != "" && (c.Fix || agent.Resuming(c)) {
		c.GitBase, c.GitLog, c.GitDiffStat = gitSnapshot(r, c.WorkingDir, c.Branch)
	}
	// A fix session claims nothing and reads no brief: the slice is Done, its
	// record of what happened is written, and the work in flight is the pull
	// request rather than the slice. Moving it back into progress would take
	// it out of the state the approve flow left it in for a session that
	// changes none of what that flow recorded, and [agent.Prompt] sends such a
	// session at the fix prompt instead, which is handed the review gathered
	// below rather than told to read it live.
	if !c.Fix {
		if err := ClaimSlice(ctx, st, c.Slice, assigneeID); err != nil {
			return LaunchResult{Toast: fmt.Sprintf("Could not %v — no agent was launched.", err), Sev: SevError}, nil
		}
		brief, err := st.Body(ctx, c.Slice.ID)
		if err != nil {
			return LaunchResult{}, fmt.Errorf("claimed %q but could not read its brief: %w", c.Slice.Name, err)
		}
		conventions, err := st.Body(ctx, c.ProjectID)
		if err != nil {
			return LaunchResult{}, fmt.Errorf("claimed %q but could not read the project conventions: %w", c.Slice.Name, err)
		}
		c.Brief, c.Conventions = brief, conventions
		c.MilestoneDigest = milestoneDigest(ctx, st, c.Milestone, c.MilestoneSlices)
	} else {
		c.ReviewComments, c.ReviewChecks = reviewSnapshot(viewer, c.WorkingDir, c.Slice.PRURL)
	}
	session := agent.SessionName(c.Slice.ID)
	file, err := agent.WritePromptFile(session, agent.Prompt(c))
	if err != nil {
		return LaunchResult{}, fmt.Errorf("launch agent: %w", err)
	}
	if err := l.Launch(session, c.WorkingDir, file, c.Slice.ID, m); err != nil {
		return LaunchResult{}, err
	}
	return LaunchResult{Context: c, Session: session, Toast: p.Toast, Sev: p.Sev}, nil
}

// milestoneDigest reads the hand-back summary of every Done sibling and
// renders the digest a launch hands the agent in place of the "go read the
// milestone with `nat info`" step a session used to be told to run itself.
// milestone and siblings are the raw material, already read off the plan by
// the caller; a summary that fails to read is logged and left out rather
// than failing the whole launch, since a missing summary costs one line of
// context rather than the launch itself.
func milestoneDigest(ctx context.Context, st Store, milestone domain.Milestone, siblings []domain.Slice) string {
	summaries := map[string]string{}
	for _, s := range siblings {
		if s.Status != domain.SliceDone {
			continue
		}
		body, err := st.Body(ctx, s.ID)
		if err != nil {
			logging.Action("could not read a milestone sibling's hand-back summary", "slice", s.ID, "err", err)
			continue
		}
		summaries[s.ID] = store.HandbackSummaryOf(body)
	}
	return agent.MilestoneDigest(milestone, siblings, summaries)
}

// gitSnapshot is a resume or fix launch's read of the worktree, once
// [PlaceAgent] has resolved it: the commit log and diff stat [agent.Prompt]
// and [fixPrompt] render inline. Each read fails on its own — a diff stat
// read is not skipped because the log one failed — and a failed read is
// logged and left empty, which is what tells the prompt to leave the whole
// section out: the project's usual reads-conclude-nothing posture, so a
// launch never fails over this.
func gitSnapshot(r Repo, dir, branch string) (base, log, diffStat string) {
	base = r.Base(dir)
	if out, err := r.LogOneline(dir, base, branch); err == nil {
		log = out
	} else {
		logging.Action("could not read a branch's commit log for a launch prompt", "dir", dir, "branch", branch, "err", err)
	}
	if out, err := r.DiffStat(dir, base, branch); err == nil {
		diffStat = out
	} else {
		logging.Action("could not read a branch's diff stat for a launch prompt", "dir", dir, "branch", branch, "err", err)
	}
	return base, log, diffStat
}

// reviewSnapshot is a fix launch's read of the pull request's review: the
// exact two commands [fixPrompt] tells the agent it may run itself. A nil
// viewer (nothing headless ever launches a fix session with one) or a read
// that fails is logged and left empty, the same posture [gitSnapshot] keeps.
func reviewSnapshot(viewer PRReviewReader, dir, prURL string) (comments, checks string) {
	if viewer == nil {
		return "", ""
	}
	if out, err := viewer.ReviewComments(dir, prURL); err == nil {
		comments = out
	} else {
		logging.Action("could not read a pull request's comments for a launch prompt", "dir", dir, "pr", prURL, "err", err)
	}
	if out, err := viewer.Checks(dir, prURL); err == nil {
		checks = out
	} else {
		logging.Action("could not read a pull request's checks for a launch prompt", "dir", dir, "pr", prURL, "err", err)
	}
	return comments, checks
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
