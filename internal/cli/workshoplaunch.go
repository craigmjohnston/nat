package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

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
