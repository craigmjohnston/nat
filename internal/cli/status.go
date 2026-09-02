package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/craigmjohnston/nat/internal/agent"
)

// status reads live tmux sessions and agent activity from the tmux server.
// No Notion is involved; the caller filters by slice IDs it knows. The output
// is all running agents sorted by slice ID, with agents gone omitted.
func status(args []string, env Env) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	if err := flags.Parse(args); err != nil {
		return usageErrorf("status: %s", err)
	}
	if flags.NArg() > 0 {
		return usageErrorf("status: unexpected argument %q", flags.Arg(0))
	}

	tmux := env.NewTmux()

	live, err := tmux.LiveSlices()
	if err != nil {
		return fmt.Errorf("read live slices: %w", err)
	}

	activity, err := tmux.Activity()
	if err != nil {
		return fmt.Errorf("read agent activity: %w", err)
	}

	if *asJSON {
		return writeStatusJSON(env.Out, live, activity)
	}
	return writeStatusMarkdown(env.Out, live, activity)
}

// statusJSON is the structured form of the status output.
type statusJSON struct {
	Agents []agentStatusJSON `json:"agents"`
}

type agentStatusJSON struct {
	SliceID  string `json:"slice_id"`
	Session  string `json:"session"`
	Activity string `json:"activity"`
}

// writeStatusJSON encodes the agent statuses as JSON.
func writeStatusJSON(out io.Writer, live map[string]string, activity map[string]agent.Activity) error {
	// Build list of agents not gone, sorted by slice ID.
	agents := buildAgentList(live, activity)

	doc := statusJSON{Agents: agents}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// writeStatusMarkdown renders agent statuses as plain text: one aligned line
// per agent, or "no agents running".
func writeStatusMarkdown(out io.Writer, live map[string]string, activity map[string]agent.Activity) error {
	agents := buildAgentList(live, activity)
	if len(agents) == 0 {
		_, err := io.WriteString(out, "no agents running\n")
		return err
	}

	// Calculate max slice ID width for alignment.
	maxWidth := 0
	for _, agent := range agents {
		if len(agent.SliceID) > maxWidth {
			maxWidth = len(agent.SliceID)
		}
	}

	var b strings.Builder
	for _, agent := range agents {
		fmt.Fprintf(&b, "%-*s  %s  %s\n", maxWidth, agent.SliceID, agent.Session, agent.Activity)
	}

	_, err := io.WriteString(out, b.String())
	return err
}

// buildAgentList constructs the list of agents from live and activity, filtering gone agents and sorting by ID.
func buildAgentList(live map[string]string, activity map[string]agent.Activity) []agentStatusJSON {
	// Collect slice IDs from live sessions.
	sliceIDs := make([]string, 0, len(live))
	for sliceID := range live {
		sliceIDs = append(sliceIDs, sliceID)
	}
	sort.Strings(sliceIDs)

	// Build agent list, omitting gone agents.
	agents := make([]agentStatusJSON, 0, len(sliceIDs))
	for _, sliceID := range sliceIDs {
		a, found := activity[sliceID]
		if found && a == agent.ActivityGone {
			continue
		}

		// Use found activity, or unknown if not found.
		act := agent.ActivityUnknown
		if found {
			act = a
		}

		agents = append(agents, agentStatusJSON{
			SliceID:  sliceID,
			Session:  live[sliceID],
			Activity: act.String(),
		})
	}

	return agents
}
