package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/nudge"
	"github.com/craigmjohnston/nat/internal/store"
)

// nudgePathFunc is a hook for testing, allowing the test to stub out nudge.Path
var nudgePathFunc = nudge.Path

// paths prints the paths to the configuration file, log directory, and nudge
// marker file. It requires no Notion client and no config file to succeed —
// paths are derivable regardless. With --project it also prints that
// project's plan file ([planFileOf]), which does need the config.
func paths(args []string, env Env) error {
	flags := flag.NewFlagSet("paths", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of aligned text")
	projectID := flags.String("project", "", "also print this project's plan file, by page `ID`")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("paths: takes no arguments, given %d", len(rest))
	}

	paths, err := pathsForPrinting()
	if err != nil {
		return err
	}
	if *projectID != "" {
		if paths.Plan, err = planFileOf(env, *projectID); err != nil {
			return err
		}
	}

	if *asJSON {
		return writePathsJSON(env.Out, paths)
	}
	_, err = io.WriteString(env.Out, pathsMarkdown(paths))
	return err
}

// planFileOf is where the plan file of the project id names is kept
// ([store.PlanPath]: its plan directory, else nat's own data directory) —
// empty for a project in Notion, whose plan is its workspace's. The project
// is matched as every command matches one, but by [Env.namedProject] rather
// than [Env.projectFor], since a source project's name, which asks its plugin,
// is no part of where its file is.
func planFileOf(env Env, id string) (string, error) {
	_, key, project, err := env.namedProject(id)
	if err != nil {
		return "", err
	}
	if !project.IsLocal() && !project.IsSource() {
		return "", nil
	}
	return store.PlanPath(store.Project{ID: key, PlanDir: project.PlanDir})
}

// pathsForPrinting resolves all three system paths used by the app.
type printPaths struct {
	Config string
	LogDir string
	Nudge  string
	// Plan is a project's plan file, where --project asked for one that has
	// a file; empty otherwise.
	Plan string
}

func pathsForPrinting() (*printPaths, error) {
	configPath, err := config.Path()
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}
	logDir, err := logging.Dir()
	if err != nil {
		return nil, fmt.Errorf("resolve log dir: %w", err)
	}
	nudgePath, err := nudgePathFunc()
	if err != nil {
		return nil, fmt.Errorf("resolve nudge path: %w", err)
	}
	return &printPaths{
		Config: configPath,
		LogDir: logDir,
		Nudge:  nudgePath,
	}, nil
}

// pathsJSON is the structured form of the paths output.
type pathsJSON struct {
	Config string `json:"config"`
	LogDir string `json:"log_dir"`
	Nudge  string `json:"nudge"`
	Plan   string `json:"plan,omitempty"`
}

// writePathsJSON encodes the paths as JSON, indented.
func writePathsJSON(out io.Writer, paths *printPaths) error {
	doc := pathsJSON{
		Config: paths.Config,
		LogDir: paths.LogDir,
		Nudge:  paths.Nudge,
		Plan:   paths.Plan,
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// pathsMarkdown renders the paths as aligned plain text, so the caller can see
// where things are without parsing JSON. The labels are aligned to the longest,
// which is "Log dir:" at 8 characters. A plan file, where there is one, is
// a fourth line.
func pathsMarkdown(paths *printPaths) string {
	const maxLen = len("Log dir:")
	text := fmt.Sprintf("%-*s %s\n%-*s %s\n%-*s %s\n",
		maxLen, "Config:", paths.Config,
		maxLen, "Log dir:", paths.LogDir,
		maxLen, "Nudge:", paths.Nudge)
	if paths.Plan != "" {
		text += fmt.Sprintf("%-*s %s\n", maxLen, "Plan:", paths.Plan)
	}
	return text
}
