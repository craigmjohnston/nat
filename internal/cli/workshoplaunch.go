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
	"github.com/craigmjohnston/nat/internal/notion"
)

// workshopLaunch launches a planning agent detached in tmux, on the project's
// default working directory. With --request it is the board's w with the
// question already answered: the text is folded into [agent.PlanPrompt] so
// the session starts on it. Without one it is the board's W: where the
// project page's wishlist has pending items, the agent is launched on it
// exactly as W is — [agent.WishlistPrompt] rather than a plain
// [agent.PlanPrompt] — and otherwise on a plain session with no request. A
// request wins over a pending wishlist for the reason w's does: the user has
// just said what they want to workshop, and the wishlist is not it.
//
// A planning session already live is refused: one is enough to hold a plan in
// its head, the same rule the board's own w and W apply.
func workshopLaunch(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("workshop-launch", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	model := flags.String("model", "", "Claude model for the agent, overriding the config's workshop_agent")
	effort := flags.String("effort", "", "effort level for the agent, overriding the config's workshop_agent")
	requestFlag := flags.String("request", "", "what to workshop, folded into the agent's prompt; - reads it from stdin")
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

	cfg, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}

	if live, err := env.NewTmux().LiveSlices(); err == nil {
		if session, ok := live[agent.PlanSentinel]; ok {
			return fmt.Errorf("a planning agent is already live: %s", session)
		}
	}

	workdir := actions.ExpandHome(project.WorkingDir)
	prompt := agent.PlanPrompt(projectID, project.Name, workdir, request)
	wishlist := false
	// The wishlist is only read when there is no request to outrank it — a
	// launch that carries its own question has no use for the page.
	if request == "" {
		client := env.NewClient(env.Tokens.Token)
		blocks, err := client.GetBlockChildren(ctx, projectID)
		if err != nil {
			return fmt.Errorf("load project page: %w", err)
		}
		items := notion.WishlistOf(blocks)
		if len(items) > 0 {
			wishlist = true
			prompt = agent.WishlistPrompt(projectID, project.Name, workdir, items)
		}
	}

	agentModel := config.AgentModel{Model: *model, Effort: *effort}
	if agentModel.Model == "" {
		agentModel.Model = cfg.WorkshopAgent.Model
	}
	if agentModel.Effort == "" {
		agentModel.Effort = cfg.WorkshopAgent.Effort
	}
	agentModel = actions.TrimModel(agentModel)

	file, err := agent.WritePromptFile(agent.PlanSession, prompt)
	if err != nil {
		return fmt.Errorf("launch planning agent: %w", err)
	}
	if err := env.NewTmux().Launch(agent.PlanSession, workdir, file, agent.PlanSentinel, agentModel); err != nil {
		return err
	}

	if *asJSON {
		return writeWorkshopLaunchJSON(env.Out, agent.PlanSession, workdir, wishlist)
	}
	_, err = io.WriteString(env.Out, workshopLaunchMarkdown(agent.PlanSession, workdir, wishlist))
	return err
}

// workshopLaunchJSON is the structured form of the launch output.
type workshopLaunchJSON struct {
	Session  string `json:"session"`
	Workdir  string `json:"workdir"`
	Wishlist bool   `json:"wishlist"`
}

// writeWorkshopLaunchJSON encodes the launch result.
func writeWorkshopLaunchJSON(out io.Writer, session, workdir string, wishlist bool) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(workshopLaunchJSON{Session: session, Workdir: workdir, Wishlist: wishlist})
}

// workshopLaunchMarkdown renders the launch result.
func workshopLaunchMarkdown(session, workdir string, wishlist bool) string {
	out := fmt.Sprintf("# Planning agent launched\n\n- Session: %s\n- Working directory: %s\n", session, workdir)
	if wishlist {
		out += "- Launched on the project's pending wishlist.\n"
	}
	return out
}
