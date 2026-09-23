package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/agent"
)

// agentKill ends a slice's agent session outright — [agent.Tmux.Kill],
// reached headlessly the way [agentInterrupt] reaches the interrupt. With
// --workshop it ends the project's planning agent instead, resolved by its
// plan tag ([agent.LivePlan]) rather than by page ID — a planning session is
// never named by one, since it is scoped to the whole project and not to any
// slice on it. The two are mutually exclusive: a positional slice names one
// session and --workshop names a different one, so both together is not one
// request but two.
//
// It is the one way an agent session is ever ended from outside the session
// itself: detaching a viewer leaves it running, which is right while there is
// still work in it and is how a finished slice's session sits on the server
// forever. A slice (or planning agent) nothing is running on is refused
// rather than passed over silently, exactly as agent-interrupt's is: the
// caller named a session it believed in, and saying so is the whole answer.
func agentKill(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("agent-kill", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	projectRef := projectFlag(flags)
	workshop := flags.Bool("workshop", false, "kill the project's planning agent instead of a slice")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if *workshop && len(rest) != 0 {
		return usageErrorf("agent-kill: --workshop takes no slice argument, given %d", len(rest))
	}
	if !*workshop && len(rest) != 1 {
		return usageErrorf("agent-kill: want exactly one slice, by URL or ID, given %d", len(rest))
	}

	_, projectID, _, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}

	tmux := env.NewTmux()
	live, err := tmux.LiveSlices()
	if err != nil {
		return fmt.Errorf("could not read live sessions: %w", err)
	}

	if *workshop {
		_, session := agent.LivePlan(live, projectID)
		if session == "" {
			return fmt.Errorf("no live planning session for this project")
		}
		if err := tmux.Kill(session); err != nil {
			return fmt.Errorf("kill the session: %w", err)
		}
		return nil
	}

	id, err := tagOrPageID("agent-kill", rest[0])
	if err != nil {
		return err
	}
	session, ok := live[id]
	if !ok {
		return fmt.Errorf("no live session for %s", id)
	}

	if err := tmux.Kill(session); err != nil {
		return fmt.Errorf("kill the session: %w", err)
	}
	return nil
}
