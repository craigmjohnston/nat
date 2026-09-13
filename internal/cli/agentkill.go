package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// agentKill ends a slice's agent session outright — [agent.Tmux.Kill],
// reached headlessly the way [agentInterrupt] reaches the interrupt.
//
// It is the one way an agent session is ever ended from outside the session
// itself: detaching a viewer leaves it running, which is right while there is
// still work in it and is how a finished slice's session sits on the server
// forever. A slice nothing is running on is refused rather than passed over
// silently, exactly as agent-interrupt's is: the caller named a session it
// believed in, and saying so is the whole answer.
func agentKill(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("agent-kill", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("agent-kill: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("agent-kill", rest[0])
	if err != nil {
		return err
	}

	if _, _, _, err := env.projectFor(*projectRef); err != nil {
		return err
	}

	tmux := env.NewTmux()
	live, err := tmux.LiveSlices()
	if err != nil {
		return fmt.Errorf("could not read live sessions: %w", err)
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
