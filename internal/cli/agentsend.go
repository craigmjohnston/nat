package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
)

// agentSend types a prompt at a live agent session and submits it — the same
// paste [agent.Tmux.SendPrompt] makes for the diff screen's review comments,
// reached headlessly rather than from the board.
func agentSend(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("agent-send", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	text := flags.String("text", "", "the prompt to send; `-` or absent reads from stdin")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("agent-send: want exactly one slice, by URL or ID, given %d", len(rest))
	}
	id, err := pageID("agent-send", rest[0])
	if err != nil {
		return err
	}

	if _, _, _, err := env.projectFor(*projectRef); err != nil {
		return err
	}

	// A prompt of any length runs into a shell's argument limit before it runs
	// into anything of nat's, so `-` or an absent flag both mean: read it from
	// stdin instead.
	prompt := *text
	if strings.TrimSpace(prompt) == "-" || strings.TrimSpace(prompt) == "" {
		if env.In == nil {
			return fmt.Errorf("agent-send: no stdin to read the prompt from")
		}
		b, err := io.ReadAll(env.In)
		if err != nil {
			return fmt.Errorf("read the prompt: %w", err)
		}
		prompt = string(b)
	}
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("agent-send: no prompt given: pass --text or pipe one in")
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

	if err := tmux.SendPrompt(session, prompt); err != nil {
		return fmt.Errorf("send the prompt: %w", err)
	}
	return nil
}
