package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
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
		paths.DefaultBase = defaultBaseOf(env, *projectID)
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

// defaultBaseOf is the branch the repository at the project's working
// directory names as its default (origin/HEAD, by [git.CLI.Base]'s chain with
// no configured base), as a branch name — what the project settings sheet
// shows as its base field's placeholder. Empty for a project with no working
// directory (a source project); the project itself is already known good, as
// planFileOf matched it first.
func defaultBaseOf(env Env, id string) string {
	_, _, project, _ := env.namedProject(id)
	dir := actions.ExpandHome(strings.TrimSpace(project.WorkingDir))
	if dir == "" {
		return ""
	}
	return strings.TrimPrefix(env.NewGit().Base(dir), "origin/")
}

// pathsForPrinting resolves all three system paths used by the app.
type printPaths struct {
	Config string
	LogDir string
	Nudge  string
	// Plan is a project's plan file, where --project asked for one that has
	// a file; empty otherwise.
	Plan string
	// DefaultBase is a --project's repository's own default branch by name
	// ([defaultBaseOf]); empty otherwise.
	DefaultBase string
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
	// DefaultBase is --project's repository's default branch by name — not
	// a path, but read here as the settings sheet reads the plan file, once.
	DefaultBase string `json:"default_base,omitempty"`
}

// writePathsJSON encodes the paths as JSON, indented.
func writePathsJSON(out io.Writer, paths *printPaths) error {
	doc := pathsJSON{
		Config: paths.Config,
		LogDir: paths.LogDir,
		Nudge:  paths.Nudge,
		Plan:   paths.Plan,

		DefaultBase: paths.DefaultBase,
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
	if paths.DefaultBase != "" {
		text += fmt.Sprintf("%-*s %s\n", maxLen, "Base:", paths.DefaultBase)
	}
	return text
}
