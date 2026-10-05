package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/agent"
)

// agentWaiting is the calling agent saying it has stopped and needs the user:
// its own pane, found from $TMUX_PANE, is flagged waiting, which is what
// `status` reads the needs-attention state from.
func agentWaiting(args []string, env Env) error {
	return markOwnPane("agent-waiting", true, args, env)
}

// agentWorking is the calling agent saying it has its answer and is back at
// work: the flag [agentWaiting] set is cleared.
func agentWorking(args []string, env Env) error {
	return markOwnPane("agent-working", false, args, env)
}

// markOwnPane sets or clears the waiting flag on the caller's own pane. Like
// `status` it is not project-scoped and takes no --project: the pane is the
// caller's, read from the environment tmux gave it, so an agent cannot mark
// any pane but its own. Nothing is written to the plan; the flag lives as
// long as the pane does. A pane already in the state asked for succeeds
// quietly, and one nat did not launch an agent in is refused before any
// tmux write.
func markOwnPane(command string, waiting bool, args []string, env Env) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		return usageErrorf("%s: %s", command, err)
	}
	if flags.NArg() > 0 {
		return usageErrorf("%s: unexpected argument %q", command, flags.Arg(0))
	}

	pane := agent.HostPane()
	if pane == "" {
		return fmt.Errorf("%s: not inside tmux ($%s is unset) — only an agent nat launched can mark its own pane",
			command, agent.PaneEnv)
	}
	if err := env.NewTmux().SetWaiting(pane, waiting); err != nil {
		if errors.Is(err, agent.ErrNotAgentPane) {
			return fmt.Errorf("%s: pane %s is not one nat launched an agent in — only a slice agent, fix session, "+
				"planning agent or ad hoc session nat launched can mark its own pane", command, pane)
		}
		return fmt.Errorf("%s: %w", command, err)
	}
	env.nudged()

	msg := "Marked as working.\n"
	if waiting {
		msg = "Marked as waiting on the user.\n"
	}
	_, err := io.WriteString(env.Out, msg)
	return err
}
