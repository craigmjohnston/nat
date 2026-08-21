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
	DataSourceOrder(ctx context.Context, dataSourceID string) ([]string, error)
	GetDataSource(ctx context.Context, id string) (*notion.DataSource, error)
	UpdateDataSourceProperties(ctx context.Context, id string, properties map[string]notion.PropertySchema) (*notion.DataSource, error)
	CreatePage(ctx context.Context, parent notion.Parent, properties map[string]notion.PropertyValue, children []map[string]any) (*notion.Page, error)
	GetPage(ctx context.Context, id string) (*notion.Page, error)
	GetBlockChildren(ctx context.Context, id string) ([]notion.Block, error)
	AppendBlockChildren(ctx context.Context, id string, children []map[string]any) ([]notion.Block, error)
	AppendBlockChildrenAfter(ctx context.Context, id, after string, children []map[string]any) ([]notion.Block, error)
	DeleteBlock(ctx context.Context, id string) error
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
	// Nudge marks that a write landed in Notion, so a board running on this
	// machine can refetch at once instead of waiting out a poll interval. It is
	// nudge.Touch in production, and may be nil where nothing listens.
	Nudge func()
}

// nudged reports a successful Notion write to whatever board may be watching.
// It is strictly fire-and-forget: a headless command never blocks, fails, or
// cares whether anyone is listening, so a missing Nudge is simply nothing to
// tell.
func (e Env) nudged() {
	if e.Nudge != nil {
		e.Nudge()
	}
}

// Usage is the help text, listing every way the binary can be run.
const Usage = `nat — track project work in Notion

usage:
  nat                 open the board
  nat setup [--json]  install the agent skills into ~/.claude/skills
  nat info [--json]   print the active project's conventions, milestones and slices
  nat next-slice [--json]
                      claim the next Todo slice and print its brief
  nat start-slice <slice> [--json]
                      claim one named Todo slice, by URL or ID, and print its
                      brief
  nat milestone-add <name> [--json]
                      add a Queued milestone at the end of the plan
  nat slice-add <title> --milestone <name> [--description TEXT|-]
                        [--repo DIR] [--depends-on <slice>]... [--json]
                      add a Todo slice under a milestone, its description
                      written on the page; --description - reads it from stdin
  nat slice-depends <slice> [--on <slice>]... [--clear] [--json]
                      record the slices a slice waits on, by URL or ID; --clear
                      drops what is there first, so on its own it frees the
                      slice
  nat wishlist [--json]
                      print the active project's pending wishlist items, with
                      their block IDs under --json
  nat wishlist-clear <block-id>...
                      trash exactly the named wishlist items, leaving the
                      section with one empty bullet for the next idea
  nat plan-apply [FILE] [--json]
                      create a whole plan of milestones and slices from a JSON
                      document, read from FILE or stdin
  nat complete-slice <slice> [--branch NAME] [--pr URL] [--summary TEXT]
                      [--pr-description TEXT|-] [--blocked]
                      close out a slice you claimed: with --branch, handed back
                      for review — the branch recorded, the slice left in
                      progress, and the board's approve key what opens the pull
                      request; with --pr, Done with its pull request recorded;
                      with --blocked, left in progress with a note saying what
                      stopped it. A summary is appended to the page either way,
                      and --pr-description records beside it the text the board
                      opens the pull request with: its first line the title,
                      the rest the body
  nat release-slice <slice>
                      hand a slice you claimed back to the plan: Todo and
                      unassigned, its brief and any branch left as they are, for
                      when the session working it ended without finishing it
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
	case "setup":
		// The one command that talks to no one: it lays down files, and a
		// machine that has not been configured yet is exactly where it is run.
		return setup(args[1:], env)
	case "info":
		return info(ctx, args[1:], env)
	case "next-slice":
		return nextSlice(ctx, args[1:], env)
	case "start-slice":
		return startSlice(ctx, args[1:], env)
	case "milestone-add":
		return milestoneAdd(ctx, args[1:], env)
	case "slice-add":
		return sliceAdd(ctx, args[1:], env)
	case "slice-depends":
		return sliceDepends(ctx, args[1:], env)
	case "wishlist":
		return wishlist(ctx, args[1:], env)
	case "wishlist-clear":
		return wishlistClear(ctx, args[1:], env)
	case "plan-apply":
		return planApply(ctx, args[1:], env)
	case "complete-slice":
		return completeSlice(ctx, args[1:], env)
	case "release-slice":
		return releaseSlice(ctx, args[1:], env)
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
