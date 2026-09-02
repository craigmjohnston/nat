package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
)

// sliceLaunch runs the board's own l key headlessly: it claims a slice and
// starts a detached tmux session for it, through [actions.Launch] — the same
// worktree placement, claim and tmux start the board's own launch flow runs.
//
// The refusals go in the same order the board checks them: Done or an
// invented status first, then a slice still waiting on something else, then a
// session already running for it — all reads, so a mistaken invocation leaves
// the plan untouched — and only once none of them applies does the claim
// happen, which is the first write this command can make.
func sliceLaunch(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("slice-launch", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	model := flags.String("model", "", "Claude model for the agent, overriding the config's slice_agent")
	effort := flags.String("effort", "", "effort level for the agent, overriding the config's slice_agent")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("slice-launch: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("slice-launch", rest[0])
	if err != nil {
		return err
	}

	cfg, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	if cfg.AssigneeUserID == "" {
		return fmt.Errorf("no assignee in the config: open the board with `nat` and finish setting it up")
	}
	client := env.NewClient(env.Tokens.Token)

	page, err := client.GetPage(ctx, id)
	if err != nil {
		return fmt.Errorf("load the slice: %w", err)
	}
	s := domain.SliceFromPage(*page)
	if s.Status != domain.SliceTodo && s.Status != domain.SliceClaimed {
		return fmt.Errorf("%q is %s: only a Todo slice or one in progress with no live session can be launched",
			s.Name, s.StatusName)
	}
	if blockers, _ := domain.Blockers(s, dependencyIndex(ctx, client, s)); len(blockers) > 0 {
		return blockedError(s, blockers)
	}
	if live, err := env.NewTmux().LiveSlices(); err == nil {
		if session, ok := live[id]; ok {
			return fmt.Errorf("%q already has a live session: %s", s.Name, session)
		}
	}

	agentModel := config.AgentModel{Model: *model, Effort: *effort}
	if agentModel.Model == "" {
		agentModel.Model = cfg.SliceAgent.Model
	}
	if agentModel.Effort == "" {
		agentModel.Effort = cfg.SliceAgent.Effort
	}
	agentModel = actions.TrimModel(agentModel)

	promptContext := agent.PromptContext{
		ProjectID:  projectID,
		Slice:      s,
		WorkingDir: actions.WorkdirFor(s, project),
	}

	result, err := actions.Launch(ctx, env.NewTmux(), env.NewWorktrees(), env.NewGit(), client,
		cfg.AssigneeUserID, promptContext, agentModel)
	if err != nil {
		return err
	}
	// A launch that claimed nothing and started nothing — a worktree that
	// could not be cut, or a claim somebody else won first — is reported as
	// itself, the same failure the board would show as a toast: nothing has
	// gone wrong with nat, but there is no session to report success about.
	if result.Session == "" {
		return errors.New(result.Toast)
	}

	env.nudged()
	if *asJSON {
		return writeLaunchJSON(env.Out, result)
	}
	_, err = io.WriteString(env.Out, launchMarkdown(result))
	return err
}

// launchJSON is the structured form of the launch output.
type launchJSON struct {
	Session string `json:"session"`
	Workdir string `json:"workdir"`
	Branch  string `json:"branch"`
	Warning string `json:"warning,omitempty"`
}

// writeLaunchJSON encodes the launch result.
func writeLaunchJSON(out io.Writer, result actions.LaunchResult) error {
	doc := launchJSON{
		Session: result.Session,
		Workdir: result.Context.WorkingDir,
		Branch:  result.Context.Branch,
		Warning: result.Toast,
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// launchMarkdown renders the launch result.
func launchMarkdown(result actions.LaunchResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Launched\n\n")
	fmt.Fprintf(&b, "- Session: %s\n", result.Session)
	fmt.Fprintf(&b, "- Working directory: %s\n", result.Context.WorkingDir)
	fmt.Fprintf(&b, "- Branch: %s\n", result.Context.Branch)
	if result.Toast != "" {
		fmt.Fprintf(&b, "\nWarning: %s\n", result.Toast)
	}
	return b.String()
}
