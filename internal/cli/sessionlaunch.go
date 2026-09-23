package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/store"
)

// sessionLaunch starts an ad hoc session: a bare Claude Code with no slice
// and no prompt, for a user who just wants an agent in the repo. It writes
// nothing to any plan but the session itself — [store.Store.AddSession]
// files it locally only, exactly as it does for a project tracked in Notion
// — and claims nothing, because there is no slice here to claim.
//
// The worktree it places the agent in follows [actions.PlaceSession]: a
// fresh cut on [actions.SessionBranch] off the remote's current default
// where dir is a git repository, or dir itself where it is not. The session
// is minted its ID before either the worktree or the tmux launch happens,
// since both are named from it ([agent.SessionTag], [agent.AdHocSessionName]).
func sessionLaunch(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("session-launch", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	dirFlag := flags.String("dir", "", "where the session runs; the project's own working directory if unset")
	model := flags.String("model", "", "Claude model for the agent, overriding the config's slice_agent")
	effort := flags.String("effort", "", "effort level for the agent, overriding the config's slice_agent")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("session-launch: takes no positional arguments, given %d", len(rest))
	}

	cfg, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}

	dir := strings.TrimSpace(*dirFlag)
	if dir == "" {
		dir = project.WorkingDir
	}
	dir = actions.ExpandHome(dir)
	if err := actions.ExistingDir(dir); err != nil {
		return fmt.Errorf("session-launch: %w", err)
	}

	agentModel := config.AgentModel{Model: *model, Effort: *effort}
	if agentModel.Model == "" {
		agentModel.Model = cfg.SliceAgent.Model
	}
	if agentModel.Effort == "" {
		agentModel.Effort = cfg.SliceAgent.Effort
	}
	agentModel = actions.TrimModel(agentModel)

	id := store.NewSessionID()
	placed := actions.PlaceSession(env.NewWorktrees(), env.NewGit(), dir, actions.SessionBranch(id))
	if !placed.OK {
		return fmt.Errorf("session-launch: %s", placed.Toast)
	}

	tmuxName := agent.AdHocSessionName(id)
	tag := agent.SessionTag(projectID, id)
	if err := env.NewTmux().LaunchBare(tmuxName, placed.Dir, tag, agentModel); err != nil {
		return fmt.Errorf("session-launch: %w", err)
	}

	added, err := st.AddSession(ctx, storeProject(projectID, project),
		store.NewSession{ID: id, Dir: dir, Branch: placed.Branch})
	if err != nil {
		return fmt.Errorf("launched %s but could not record the session: %w", tmuxName, err)
	}

	if *asJSON {
		return writeSessionLaunchJSON(env.Out, added, tmuxName, tag, placed)
	}
	_, err = io.WriteString(env.Out, sessionLaunchMarkdown(added, tmuxName, placed))
	return err
}

// sessionLaunchJSON is the structured form of a launched session.
type sessionLaunchJSON struct {
	Session string `json:"session"`
	Tag     string `json:"tag"`
	ID      string `json:"id"`
	Dir     string `json:"dir"`
	Branch  string `json:"branch"`
	Warning string `json:"warning,omitempty"`
}

// writeSessionLaunchJSON encodes the launch result.
func writeSessionLaunchJSON(out io.Writer, s domain.Session, tmuxName, tag string, placed actions.Placement) error {
	doc := sessionLaunchJSON{
		Session: tmuxName, Tag: tag, ID: s.ID, Dir: placed.Dir, Branch: placed.Branch, Warning: placed.Toast,
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// sessionLaunchMarkdown renders the launch result.
func sessionLaunchMarkdown(s domain.Session, tmuxName string, placed actions.Placement) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Session launched\n\n")
	fmt.Fprintf(&b, "- ID: %s\n", s.ID)
	fmt.Fprintf(&b, "- Session: %s\n", tmuxName)
	fmt.Fprintf(&b, "- Directory: %s\n", placed.Dir)
	if placed.Branch != "" {
		fmt.Fprintf(&b, "- Branch: %s\n", placed.Branch)
	}
	if placed.Toast != "" {
		fmt.Fprintf(&b, "\nWarning: %s\n", placed.Toast)
	}
	return b.String()
}
