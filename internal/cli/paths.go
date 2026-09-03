package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/nudge"
)

// nudgePathFunc is a hook for testing, allowing the test to stub out nudge.Path
var nudgePathFunc = nudge.Path

// paths prints the paths to the configuration file, log directory, and nudge
// marker file. It requires no Notion client and no config file to succeed —
// paths are derivable regardless.
func paths(args []string, env Env) error {
	flags := flag.NewFlagSet("paths", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of aligned text")
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

	if *asJSON {
		return writePathsJSON(env.Out, paths.Config, paths.LogDir, paths.Nudge)
	}
	_, err = io.WriteString(env.Out, pathsMarkdown(paths.Config, paths.LogDir, paths.Nudge))
	return err
}

// pathsForPrinting resolves all three system paths used by the app.
type printPaths struct {
	Config string
	LogDir string
	Nudge  string
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
}

// writePathsJSON encodes the paths as JSON, indented.
func writePathsJSON(out io.Writer, configPath, logDir, nudgePath string) error {
	doc := pathsJSON{
		Config: configPath,
		LogDir: logDir,
		Nudge:  nudgePath,
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// pathsMarkdown renders the paths as aligned plain text, so the caller can see
// where things are without parsing JSON. The labels are aligned to the longest,
// which is "Log dir:" at 8 characters.
func pathsMarkdown(configPath, logDir, nudgePath string) string {
	const maxLen = len("Log dir:")
	return fmt.Sprintf("%-*s %s\n%-*s %s\n%-*s %s\n",
		maxLen, "Config:", configPath,
		maxLen, "Log dir:", logDir,
		maxLen, "Nudge:", nudgePath)
}
