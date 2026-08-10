// Package cli holds nat's headless commands: what the binary does when it is
// given a subcommand instead of being asked for the board. It is the surface an
// agent reads the tracker through, so nothing here touches the TUI or the
// terminal — a command writes to a writer and returns an error.
package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/notion"
)

// API is the part of *notion.Client the commands use. It is an interface so a
// command can be driven by a fake in tests.
type API interface {
	QueryDataSource(ctx context.Context, id string, filter map[string]any, sorts []notion.Sort) ([]notion.Page, error)
	GetPage(ctx context.Context, id string) (*notion.Page, error)
	GetBlockChildren(ctx context.Context, id string) ([]notion.Block, error)
	AppendBlockChildren(ctx context.Context, id string, children []map[string]any) ([]notion.Block, error)
	UpdatePageProperties(ctx context.Context, pageID string, properties map[string]notion.PropertyValue) (*notion.Page, error)
}

// NewClientFunc builds an API from a source of bearer tokens.
type NewClientFunc func(token notion.TokenFunc) API

// DefaultNewClient builds a real Notion client that re-reads the token for
// every request, the same way the TUI does.
func DefaultNewClient(token notion.TokenFunc) API { return notion.NewWithToken(token) }

// Env is everything a command needs from the process around it, held as fields
// so a test can stand in for each edge.
type Env struct {
	// Tokens supplies the Notion bearer token.
	Tokens config.TokenSource
	// Load reads local config; it is config.Load in production.
	Load func() (config.Config, bool, error)
	// NewClient builds the Notion client a command talks through.
	NewClient NewClientFunc
	// Out is where a command writes its output.
	Out io.Writer
	// In is where a command reads input a flag was not given for; it is stdin
	// in production, and may be nil where nothing is ever piped in.
	In io.Reader
}

// Usage is the help text, listing every way the binary can be run.
const Usage = `nat — track project work in Notion

usage:
  nat                 open the board
  nat info [--json]   print the active project's conventions, milestones and slices
  nat next-slice [--json]
                      claim the next Todo slice and print its brief
  nat complete-slice <slice> [--pr URL] [--summary TEXT] [--blocked]
                      close out a slice you claimed: Done, its PR, and a
                      summary appended to its page — or, with --blocked, left
                      Claimed with a note saying what stopped it
  nat help            show this message
`

// UsageError is a command line this package cannot make sense of: an unknown
// command, a bad flag, an argument too many. It is a distinct type so a caller
// can tell a misuse from a failed call.
type UsageError struct{ Message string }

// Error implements error.
func (e *UsageError) Error() string { return e.Message }

// usageErrorf builds a UsageError, pointing the way to the help text — a
// mistyped command is the moment someone most wants to be told what the
// commands are.
func usageErrorf(format string, args ...any) error {
	return &UsageError{Message: fmt.Sprintf(format, args...) + "\nrun `nat help` for the commands available"}
}

// IsCommand reports whether the process arguments (os.Args[1:]) ask for a
// headless command rather than the board. Anything at all does: the board takes
// no arguments, so a stray one is a command — a wrong one, which Run says so
// about, rather than something to silently ignore on the way to a TUI.
func IsCommand(args []string) bool { return len(args) > 0 }

// Run executes the command named by args[0] and returns when it is done.
func Run(ctx context.Context, args []string, env Env) error {
	if len(args) == 0 {
		return usageErrorf("no command given")
	}
	switch args[0] {
	case "info":
		return info(ctx, args[1:], env)
	case "next-slice":
		return nextSlice(ctx, args[1:], env)
	case "complete-slice":
		return completeSlice(ctx, args[1:], env)
	case "help", "-h", "--help":
		_, err := io.WriteString(env.Out, Usage)
		return err
	default:
		return usageErrorf("unknown command %q", args[0])
	}
}

// activeProject resolves the project a command works on: the one local config
// points at. A missing config file is reported as the setup that has not
// happened yet rather than as a parse failure, because that is what it is.
func (e Env) activeProject() (config.Config, config.ProjectConfig, error) {
	cfg, found, err := e.Load()
	if err != nil {
		return cfg, config.ProjectConfig{}, err
	}
	if !found {
		return cfg, config.ProjectConfig{}, fmt.Errorf("no configuration yet: run `nat` once to set it up")
	}
	if cfg.ActiveProjectID == "" {
		return cfg, config.ProjectConfig{}, fmt.Errorf("no active project: open the board with `nat` and pick one")
	}
	p, ok := cfg.Projects[cfg.ActiveProjectID]
	if !ok {
		return cfg, config.ProjectConfig{}, fmt.Errorf("active project %s is not in the config file", cfg.ActiveProjectID)
	}
	return cfg, p, nil
}
