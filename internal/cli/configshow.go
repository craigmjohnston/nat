package cli

import (
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/craigmjohnston/nat/internal/config"
)

// configShow prints local configuration: the fields the settings form edits
// and nothing else — the agent split, the poll interval, the two model pairs
// and each tracked project's working directory. It touches neither Notion nor
// a project: the workspace's databases, the assignee and a project's Slices
// data source ID are the wizard's own writes rather than something meant to be
// typed over, so they are left off exactly as internal/tui/settings.go leaves
// them off its form. There is no token to print either, because there is none
// in the config file at all — it is read back from the Notion CLI on every
// request.
func configShow(args []string, env Env) error {
	flags := flag.NewFlagSet("config-show", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("config-show: takes no arguments, given %d", len(rest))
	}

	cfg, found, err := env.Load()
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no configuration yet: run `nat` once to set it up")
	}

	if *asJSON {
		return writeJSON(env.Out, configShowJSON(cfg))
	}
	_, err = io.WriteString(env.Out, configShowMarkdown(cfg))
	return err
}

// agentModelJSON is a model pair as config-show and config-set both print it.
type agentModelJSON struct {
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
}

// configProjectJSON is one tracked project's share of the config file: its
// name, for reading the listing without a second lookup, and its working
// directory, the one field config-set can change on it.
type configProjectJSON struct {
	Name       string `json:"name"`
	WorkingDir string `json:"working_dir"`
	// Backend is where the project's plan is kept, always said out loud even
	// for the Notion projects the config file leaves it unwritten for: a
	// listing is read by somebody asking which is which, and a blank would
	// make them go and work it out.
	Backend string `json:"backend"`
	// PlanDir is where a local project's plan file is kept, and empty both for
	// a Notion project and for a local one kept in nat's own data directory.
	PlanDir string `json:"plan_dir,omitempty"`
}

// configDoc is the structured form of local config.
type configDoc struct {
	AgentSplitPercent int                          `json:"agent_split_percent"`
	PollSeconds       int                          `json:"poll_seconds"`
	WorkshopAgent     agentModelJSON               `json:"workshop_agent"`
	SliceAgent        agentModelJSON               `json:"slice_agent"`
	Projects          map[string]configProjectJSON `json:"projects"`
}

// configShowJSON maps the config onto the structured form: the raw stored
// values, zero meaning unset exactly as the config file itself writes it,
// rather than the default [config.Config.SplitPercent] or
// [config.Config.PollInterval] would swap in — this is what is on disk, not
// what a launch would resolve it to.
func configShowJSON(cfg config.Config) configDoc {
	doc := configDoc{
		AgentSplitPercent: cfg.AgentSplitPercent,
		PollSeconds:       cfg.PollSeconds,
		WorkshopAgent:     agentModelJSON{Model: cfg.WorkshopAgent.Model, Effort: cfg.WorkshopAgent.Effort},
		SliceAgent:        agentModelJSON{Model: cfg.SliceAgent.Model, Effort: cfg.SliceAgent.Effort},
		Projects:          make(map[string]configProjectJSON, len(cfg.Projects)),
	}
	for id, p := range cfg.Projects {
		doc.Projects[id] = configProjectJSON{
			Name: p.Name, WorkingDir: p.WorkingDir, Backend: backendWord(p), PlanDir: p.PlanDir,
		}
	}
	return doc
}

// configShowMarkdown renders the config as a bullet list, projects sorted by
// ID so the output reads the same way twice.
func configShowMarkdown(cfg config.Config) string {
	out := "# Config\n\n"
	out += fmt.Sprintf("- Agent split percent: %d (0 = default)\n", cfg.AgentSplitPercent)
	out += fmt.Sprintf("- Poll seconds: %d (0 = default)\n", cfg.PollSeconds)
	out += fmt.Sprintf("- Workshop agent: model=%q effort=%q\n", cfg.WorkshopAgent.Model, cfg.WorkshopAgent.Effort)
	out += fmt.Sprintf("- Slice agent: model=%q effort=%q\n", cfg.SliceAgent.Model, cfg.SliceAgent.Effort)

	out += "\n## Projects\n\n"
	if len(cfg.Projects) == 0 {
		return out + "_none_\n"
	}
	ids := make([]string, 0, len(cfg.Projects))
	for id := range cfg.Projects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := cfg.Projects[id]
		out += fmt.Sprintf("- %s (%s): backend=%s working_dir=%q", id, p.Name, backendWord(p), p.WorkingDir)
		if p.PlanDir != "" {
			out += fmt.Sprintf(" plan_dir=%q", p.PlanDir)
		}
		out += "\n"
	}
	return out
}

// backendWord is where a project's plan is kept, said in full. The config file
// leaves it unwritten for a Notion project, since that is what every config
// written before there was a choice already means, but a listing that left it
// blank would only be read as a question.
func backendWord(p config.ProjectConfig) string {
	if p.IsLocal() {
		return config.BackendLocal
	}
	return config.BackendNotion
}
