// Package cli holds nat's headless commands: what the binary does when it is
// given a subcommand instead of being asked for the board. It is the surface an
// agent reads the tracker through, so nothing here touches the TUI or the
// terminal — a command writes to a writer and returns an error.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
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
	CreateProject(ctx context.Context, projectsDSID, name string, assignee bool) (*notion.ProjectStructure, error)
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
	// Save writes local config back, for the one command that changes it: a
	// project created headlessly has to be recorded somewhere this machine will
	// read it again. It is config.Save in production.
	Save func(config.Config) error
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

Every command below that acts on a project requires --project, naming one of
the config file's projects by its page ID; run one without it to be told the
projects this machine tracks. There is no fallback to the project the board is
on: that is the board's own, and the user moves it while an agent works. setup
and project-create take no such flag: neither acts on a project already
tracked.

usage:
  nat                 open the board
  nat setup [--json]  install the agent skills into ~/.claude/skills
  nat info [--json] --project ID
                      print the project's conventions, milestones and slices
  nat next-slice [--json] --project ID
                      claim the next Todo slice and print its brief
  nat start-slice <slice> [--json] --project ID
                      claim one named Todo slice, by URL or ID, and print its
                      brief
  nat project-create <name> [--repo DIR] [--description TEXT|-] [--json]
                      create a project and its Slices database, register it in
                      local config and write the description as its page body;
                      --description - reads it from stdin. The board is left on
                      whatever project it was on
  nat milestone-add <name> [--json] --project ID
                      add a Queued milestone at the end of the plan
  nat slice-add <title> --milestone <name> [--description TEXT|-]
                        [--repo DIR] [--depends-on <slice>]... [--json]
                        --project ID
                      add a Todo slice under a milestone, its description
                      written on the page; --description - reads it from stdin
  nat slice-depends <slice> [--on <slice>]... [--clear] [--json] --project ID
                      record the slices a slice waits on, by URL or ID; --clear
                      drops what is there first, so on its own it frees the
                      slice
  nat wishlist [--json] --project ID
                      print the project's pending wishlist items, with their
                      block IDs under --json
  nat wishlist-clear <block-id>... --project ID
                      trash exactly the named wishlist items, leaving the
                      section with one empty bullet for the next idea
  nat plan-apply [FILE] [--json] --project ID
                      create a whole plan of milestones and slices from a JSON
                      document, read from FILE or stdin
  nat complete-slice <slice> [--branch NAME] [--pr URL] [--summary TEXT]
                      [--pr-description TEXT|-] [--blocked] --project ID
                      close out a slice you claimed: with --branch, handed back
                      for review — the branch recorded, the slice left in
                      progress, and the board's approve key what opens the pull
                      request; with --pr, Done with its pull request recorded;
                      with --blocked, left in progress with a note saying what
                      stopped it. A summary is appended to the page either way,
                      and --pr-description records beside it the text the board
                      opens the pull request with: its first line the title,
                      the rest the body
  nat release-slice <slice> --project ID
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
	case "project-create":
		return projectCreate(ctx, args[1:], env)
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

// projectFlag registers --project on a command's flag set. Every command that
// acts on a project already tracked takes it, and every one of them takes it
// the same way — one of the config file's projects by its page ID — and every
// one of them requires it.
func projectFlag(flags *flag.FlagSet) *string {
	return flags.String("project", "",
		"the project to act on, by page `ID` (required)")
}

// projectFor resolves the project a command works on: the one its --project
// flag names, and nothing at all when it names none. There is no fallback to
// the active project, because the active project is the board's own idea of
// where the user is looking and the user moves it while an agent works — a
// headless write that took it would land in whichever project the board had got
// to rather than the one the session was launched on.
//
// The project's page ID comes back beside its config entry, since a command
// that reads the project page itself — its conventions, its wishlist — has no
// other source for it.
func (e Env) projectFor(id string) (config.Config, string, config.ProjectConfig, error) {
	if strings.TrimSpace(id) == "" {
		return e.noProject()
	}
	return e.namedProject(strings.TrimSpace(id))
}

// noProject is the refusal a project-scoped command gives when it was told no
// project at all. It lists what the config file does hold, so the ID to pass is
// one failed call away rather than something to go and read the config for; a
// config that will not load or is not there yet lists nothing, since the flag is
// missing either way and a second complaint would only bury the first.
func (e Env) noProject() (config.Config, string, config.ProjectConfig, error) {
	cfg, _, _ := e.Load()
	return cfg, "", config.ProjectConfig{}, fmt.Errorf(
		"no project given: pass --project with a project's page ID%s", knownProjects(cfg))
}

// namedProject resolves one project of the config file by its ID — a key of the
// Projects map, which is the project page's own ID. An ID the config does not
// know is refused by name and the ones it does know are listed, since the whole
// difficulty of naming a project is remembering how it is spelled.
//
// The ID is matched as written first and normalised afterwards: the keys come
// from Notion dashed, and an ID copied out of a page URL has no dashes at all.
// The key is what comes back rather than the ID as it was typed, since that is
// the form the page is addressed by.
func (e Env) namedProject(id string) (config.Config, string, config.ProjectConfig, error) {
	cfg, found, err := e.Load()
	if err != nil {
		return cfg, "", config.ProjectConfig{}, err
	}
	if !found {
		return cfg, "", config.ProjectConfig{}, fmt.Errorf("no configuration yet: run `nat` once to set it up")
	}
	if p, ok := cfg.Projects[id]; ok {
		return cfg, id, p, nil
	}
	want := domain.NormaliseID(id)
	for key, p := range cfg.Projects {
		if domain.NormaliseID(key) == want {
			return cfg, key, p, nil
		}
	}
	return cfg, "", config.ProjectConfig{}, fmt.Errorf("no project %s in the config file%s", id, knownProjects(cfg))
}

// knownProjects lists what the config file does hold, for the error that says
// it does not hold what was asked for. Sorted, because the map's own
// order is no order and an error that reads differently every run is one nobody
// trusts.
func knownProjects(cfg config.Config) string {
	if len(cfg.Projects) == 0 {
		return ": it tracks no projects yet"
	}
	known := make([]string, 0, len(cfg.Projects))
	for id, p := range cfg.Projects {
		known = append(known, fmt.Sprintf("%s (%s)", id, p.Name))
	}
	sort.Strings(known)
	return fmt.Sprintf(": it tracks %s", strings.Join(known, ", "))
}
