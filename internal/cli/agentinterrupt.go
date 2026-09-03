package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// agentInterrupt sends Claude Code's own interrupt key to a live agent
// session — [agent.Tmux.Interrupt], reached headlessly rather than from the
// board.
func agentInterrupt(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("agent-interrupt", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("agent-interrupt: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("agent-interrupt", rest[0])
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

	if err := tmux.Interrupt(session); err != nil {
		return fmt.Errorf("send interrupt: %w", err)
	}
	return nil
}
