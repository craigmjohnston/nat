package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/agent"
	"github.com/craigmjohnston/nat/internal/config"
)

// workshopLaunch launches a planning agent detached in tmux, on the project's
// default working directory. With --request it is the board's w with the
// question already answered: the text is folded into [agent.PlanPrompt] so
// the session starts on it. Without one it launches a plain [agent.PlanPrompt]
// session, and reads nothing of the project page.
//
// A planning session already live on this project is refused: one is enough to
// hold its plan in its head, the same rule the board's own w and W apply. It is
// that project's own session that refuses, so a second project can be
// workshopped at the same time — and a bare pre-upgrade planning session, which
// belongs to no project, refuses every project, since it is the one any of them
// would attach.
func workshopLaunch(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("workshop-launch", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	model := flags.String("model", "", "Claude model for the agent, overriding the config's workshop_agent")
	effort := flags.String("effort", "", "effort level for the agent, overriding the config's workshop_agent")
	requestFlag := flags.String("request", "", "what to workshop, folded into the agent's prompt; - reads it from stdin")
	frontendFlag := flags.String("frontend", "", `which surface launched this: "tui" or "gnat"; empty says nothing about where the user is`)
	projectRef := projectFlag(flags)
	workspace := flags.String("workspace", "", "launch the new-project planning agent for the app's untitled tab with this id, instead of on a --project")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("workshop-launch: takes no arguments, given %d", len(rest))
	}
	request, err := briefText("workshop-launch", "--request", *requestFlag, env.In)
	if err != nil {
		return err
	}
	frontend, err := agent.ParseFrontend(*frontendFlag)
	if err != nil {
		return usageErrorf("workshop-launch: %v", err)
	}
	if ws := strings.TrimSpace(*workspace); ws != "" {
		if strings.TrimSpace(*projectRef) != "" {
			return usageErrorf("workshop-launch: --workspace and --project name different sessions, not both")
		}
		return workshopLaunchWorkspace(env, ws, request, *model, *effort, *asJSON)
	}

	cfg, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}

	if live, err := env.NewTmux().LiveSlices(); err == nil {
		if _, session := agent.LivePlan(live, projectID); session != "" {
			return fmt.Errorf("a planning agent is already live: %s", session)
		}
	}

	workdir := actions.ExpandHome(project.WorkingDir)
	// A store that failed to open is not this command's own failure to
	// return: [actions.RenderedPlan] only ever needs one to read the plan
	// inline, and a launch with none simply falls its prompt back to naming
	// `nat info` instead.
	plan := ""
	if st, err := env.storeFor(ctx, projectID, project); err == nil {
		plan = actions.RenderedPlan(ctx, st, storeProject(projectID, project))
	}
	prompt := agent.PlanPrompt(projectID, project.Name, workdir, request, plan, frontend)

	agentModel := config.AgentModel{Model: *model, Effort: *effort}
	if agentModel.Model == "" {
		agentModel.Model = cfg.WorkshopAgent.Model
	}
	if agentModel.Effort == "" {
		agentModel.Effort = cfg.WorkshopAgent.Effort
	}
	agentModel = actions.TrimModel(agentModel)

	session := agent.PlanSessionName(projectID)
	file, err := agent.WritePromptFile(session, prompt)
	if err != nil {
		return fmt.Errorf("launch planning agent: %w", err)
	}
	if err := env.NewTmux().Launch(session, workdir, file, agent.PlanTag(projectID), agentModel); err != nil {
		return err
	}

	if *asJSON {
		return writeWorkshopLaunchJSON(env.Out, session, workdir)
	}
	_, err = io.WriteString(env.Out, workshopLaunchMarkdown(session, workdir))
	return err
}

// workspacesDirName is the scratch working directories' own directory under
// nat's state directory, beside [proposalsDirName].
const workspacesDirName = "workspaces"

// workspaceDir is the scratch directory one workspace's planning session runs
// in: there is no project, and so no working directory, to run it in, and a
// session that never touches a repository wants nowhere in particular.
func workspaceDir(workspaceID string) (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, workspacesDirName, workspaceID), nil
}

// workshopLaunchWorkspace is workshop-launch for a new-project tab of the app:
// a planning agent on [agent.NewProjectPrompt], keyed by the tab's workspace
// id where every other one is keyed by a project, so it collides with no
// project's own planning agent, and refuses a second launch on the same
// workspace the way a project's does — the caller attaches to the one live.
//
// The request is the starter card's text and is required: the prompt tells the
// agent to start on it, and an empty one would leave it nothing to start on.
// It is never logged — it is the user's own words.
func workshopLaunchWorkspace(env Env, workspace, request, model, effort string, asJSON bool) error {
	if request == "" {
		return usageErrorf("workshop-launch: --workspace needs a --request: what the new project is")
	}
	if live, err := env.NewTmux().LiveSlices(); err == nil {
		if session := live[agent.PlanTag(workspace)]; session != "" {
			return fmt.Errorf("a planning agent is already live: %s", session)
		}
	}

	workdir, err := workspaceDir(workspace)
	if err != nil {
		return fmt.Errorf("launch planning agent: %w", err)
	}
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return fmt.Errorf("launch planning agent: create the scratch directory: %w", err)
	}

	// A config that will not load is no reason to refuse: the model pair is
	// the only thing read from it, and unset halves contribute no flag.
	cfg, _, _ := env.Load()
	agentModel := config.AgentModel{Model: model, Effort: effort}
	if agentModel.Model == "" {
		agentModel.Model = cfg.WorkshopAgent.Model
	}
	if agentModel.Effort == "" {
		agentModel.Effort = cfg.WorkshopAgent.Effort
	}
	agentModel = actions.TrimModel(agentModel)

	session := agent.PlanSessionName(workspace)
	file, err := agent.WritePromptFile(session, agent.NewProjectPrompt(workspace, request))
	if err != nil {
		return fmt.Errorf("launch planning agent: %w", err)
	}
	if err := env.NewTmux().Launch(session, workdir, file, agent.PlanTag(workspace), agentModel); err != nil {
		return err
	}

	if asJSON {
		return writeWorkshopLaunchJSON(env.Out, session, workdir)
	}
	_, err = io.WriteString(env.Out, workshopLaunchMarkdown(session, workdir))
	return err
}

// workshopLaunchJSON is the structured form of the launch output.
type workshopLaunchJSON struct {
	Session string `json:"session"`
	Workdir string `json:"workdir"`
}

// writeWorkshopLaunchJSON encodes the launch result.
func writeWorkshopLaunchJSON(out io.Writer, session, workdir string) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(workshopLaunchJSON{Session: session, Workdir: workdir})
}

// workshopLaunchMarkdown renders the launch result.
func workshopLaunchMarkdown(session, workdir string) string {
	return fmt.Sprintf("# Planning agent launched\n\n- Session: %s\n- Working directory: %s\n", session, workdir)
}
