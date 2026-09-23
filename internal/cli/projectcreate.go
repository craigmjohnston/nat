package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/craigmjohnston/nat/internal/actions"
	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// getwd is where a project-create given no --repo puts the project's agents:
// the directory the command was typed in. Held as a variable so a test can
// stand in for the one edge of it that cannot otherwise be reached.
var getwd = os.Getwd

// projectCreate creates a whole tracked project without the board: the project
// row, the Slices database under it, its conventions on the page, and the entry
// in local config that makes it a project this machine knows about. It is what
// a session that has just workshopped a plan runs before `nat plan-apply` files
// that plan — the one step of the workflow that otherwise needed the TUI.
//
// It deliberately does not make the new project the active one. The active
// project is the board's alone — every headless command names the project it
// works on — so moving it from here would move the board out from under the
// user for no gain; the board's own switch picker is how a new project gets
// opened, and it reads the config this wrote. Until then `--project` with the
// ID this printed is how a session reaches the project it just made.
func projectCreate(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("project-create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	repo := flags.String("repo", "", "where this project's agents work; defaults to the current directory")
	description := flags.String("description", "", "the conventions to write on the project page; `-` reads them from stdin")
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	local := flags.Bool("local", false, "keep the plan in a file of nat's own, with no Notion workspace behind it")
	planDir := flags.String("plan-dir", "", "with --local: the directory to keep the plan file in; defaults to nat's own data directory")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("project-create: want exactly one project name, given %d", len(rest))
	}
	name := strings.TrimSpace(rest[0])
	if name == "" {
		return usageErrorf("project-create: the project name is empty")
	}
	if *planDir != "" && !*local {
		return usageErrorf("project-create: --plan-dir only means something with --local: a plan in Notion is kept there")
	}
	// Both are settled before Notion is touched, so a project-create whose stdin
	// cannot be read, or which has no directory to name, fails having created
	// nothing.
	info, err := briefText("project-create", "--description", *description, env.In)
	if err != nil {
		return err
	}
	workdir, err := workingDir(*repo)
	if err != nil {
		return err
	}

	if *local {
		return projectCreateLocal(ctx, env, name, info, workdir, *planDir, *asJSON)
	}

	cfg, err := env.workspace()
	if err != nil {
		return err
	}
	client := env.NewClient(env.Tokens.Token)

	// The Assignee column follows the configured user: on a workspace where
	// claiming a slice writes a person, the table needs somewhere to write them,
	// and on one where it does not a people column nobody ever fills is noise.
	// The board asks the question; a headless command has only this to go on.
	assignee := cfg.AssigneeUserID != ""
	s, err := client.CreateProject(ctx, cfg.ProjectDBDataSourceID, name, assignee)
	switch {
	case s != nil:
		// A structure with an error is a project that exists but whose schema did
		// not read back as expected: recorded and reported, rather than orphaned.
	case err != nil:
		return fmt.Errorf("create project: %w", err)
	default:
		return errors.New("create project: no project was returned")
	}
	logging.Action("project created", "project", s.PageID, "name", name, "assignee", assignee)

	if blocks := notion.BlocksFromMarkdown(info); len(blocks) > 0 {
		if aerr := appendPageBody(ctx, client, s.PageID, blocks); aerr != nil {
			err = errors.Join(err, aerr)
		}
	}
	if serr := registerProject(env, cfg, s, name, workdir); serr != nil {
		err = errors.Join(err, serr)
	}
	env.nudged()

	if werr := printProject(env, *asJSON, s, name, workdir, assignee); werr != nil {
		return errors.Join(err, werr)
	}
	return err
}

// projectCreateLocal is project-create for a project with no workspace behind
// it, which touches Notion nowhere: no page, no projects database, no token. Its
// ID is nat's own, shaped like a page ID so that everything which carries one
// around cannot tell the two apart.
//
// The plan file is written before the config entry, and the entry is not
// written at all if the file could not be laid down: a config naming a project
// whose plan is not there is one every later command fails on.
func projectCreateLocal(ctx context.Context, env Env, name, conventions, workdir, planDir string, asJSON bool) error {
	id, dir, err := createLocalProject(ctx, env, name, conventions, workdir, planDir)
	if err != nil {
		return err
	}

	if asJSON {
		return writeJSON(env.Out, projectCreatedJSON{Project: createdProjectJSON{
			ID: id, Name: name, WorkingDir: workdir, Backend: config.BackendLocal, PlanDir: dir,
		}})
	}
	_, err = io.WriteString(env.Out, localProjectCreatedMarkdown(id, name, workdir, dir))
	return err
}

// createLocalProject is the one way a local project comes to exist: shared by
// project-create --local and scratch-open, so there is no second way to be
// local. It returns the new ID and the plan directory as config now keeps it.
func createLocalProject(ctx context.Context, env Env, name, conventions, workdir, planDir string) (string, string, error) {
	dir, err := absPlanDir(planDir)
	if err != nil {
		return "", "", err
	}
	// No configuration yet is not an obstacle here: the first thing a machine
	// with no Notion workspace does is make a project, and that is what starts one.
	cfg, _, err := env.Load()
	if err != nil {
		return "", "", err
	}
	id := store.NewProjectID()
	entry := config.ProjectConfig{Name: name, WorkingDir: workdir, Backend: config.BackendLocal, PlanDir: dir}
	if err := store.CreateLocalProject(ctx, store.ProjectOf(id, entry), conventions); err != nil {
		return "", "", fmt.Errorf("create the plan: %w", err)
	}
	logging.Action("project created", "project", id, "name", name, "backend", config.BackendLocal)
	if cfg.Projects == nil {
		cfg.Projects = map[string]config.ProjectConfig{}
	}
	cfg.Projects[id] = entry
	if err := env.Save(cfg); err != nil {
		return "", "", fmt.Errorf("save config: %w", err)
	}
	env.nudged()
	return id, dir, nil
}

// absPlanDir is the plan directory as config keeps it: home expanded and made
// absolute, so the entry means the same wherever a later command is run from.
// Empty stays empty — nat's own data directory is not written down.
func absPlanDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", nil
	}
	dir = actions.ExpandHome(dir)
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir), nil
	}
	cwd, err := getwd()
	if err != nil {
		return "", fmt.Errorf("resolve --plan-dir: %w", err)
	}
	return filepath.Join(cwd, dir), nil
}

// localProjectCreatedMarkdown reports a project of nat's own as created.
func localProjectCreatedMarkdown(id, name, workdir, planDir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", name)
	b.WriteString("Created, with an empty plan kept in a file of nat's own — no Notion workspace behind it.\n\n")
	fmt.Fprintf(&b, "- Project ID: %s\n", id)
	if planDir != "" {
		fmt.Fprintf(&b, "- Plan directory: %s\n", planDir)
	}
	fmt.Fprintf(&b, "- Working directory: %s\n", workdir)
	fmt.Fprintf(&b, "- %s\n", switchNote)
	return b.String()
}

// appendPageBody writes the project's conventions onto its page. It is the page
// body rather than a property because that is what `nat info` prints as the
// project's conventions — a project created from a plan should arrive with the
// brief its agents read already on it.
func appendPageBody(ctx context.Context, client API, pageID string, blocks []map[string]any) error {
	if _, err := client.AppendBlockChildren(ctx, pageID, blocks); err != nil {
		return fmt.Errorf("write the project page: %w", err)
	}
	return nil
}

// registerProject records the new project in local config, which is what makes
// it a project this machine can open: the Notion side knows nothing about where
// its work happens, and the data source ID is what every later query addresses.
//
// The active project is left exactly as it was, on purpose — see projectCreate.
func registerProject(env Env, cfg config.Config, s *notion.ProjectStructure, name, workdir string) error {
	if cfg.Projects == nil {
		cfg.Projects = map[string]config.ProjectConfig{}
	}
	cfg.Projects[s.PageID] = config.ProjectConfig{
		Name:       name,
		SlicesDSID: s.SlicesDSID,
		WorkingDir: workdir,
	}
	if err := env.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// workingDir is where the project's agents will start: what --repo named, or
// the directory the command was run in, which is the answer for a project
// created from inside its own checkout — the ordinary way to run this.
func workingDir(repo string) (string, error) {
	if dir := strings.TrimSpace(repo); dir != "" {
		return dir, nil
	}
	dir, err := getwd()
	if err != nil {
		return "", fmt.Errorf("resolve the working directory: %w", err)
	}
	return dir, nil
}

// printProject reports what was created, in whichever form was asked for.
func printProject(env Env, asJSON bool, s *notion.ProjectStructure, name, workdir string, assignee bool) error {
	if asJSON {
		return writeJSON(env.Out, projectCreatedJSON{Project: createdProjectJSON{
			ID:         s.PageID,
			Name:       name,
			URL:        s.PageURL,
			SlicesDBID: s.SlicesDBID,
			SlicesDSID: s.SlicesDSID,
			WorkingDir: workdir,
			Assignee:   assignee,
		}})
	}
	_, err := io.WriteString(env.Out, projectCreatedMarkdown(s, name, workdir, assignee))
	return err
}

// projectCreatedJSON is the structured form of a created project, wrapping it
// in a named field the way every other creation here does.
type projectCreatedJSON struct {
	Project createdProjectJSON `json:"project"`
}

// createdProjectJSON is the whole of what was made: the page it is, the
// database its plan lives in, and the directory local config now points at.
type createdProjectJSON struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	SlicesDBID string `json:"slices_db_id"`
	SlicesDSID string `json:"slices_ds_id"`
	WorkingDir string `json:"working_dir"`
	Assignee   bool   `json:"assignee"`
	// Backend and PlanDir are set only for a project of nat's own, which has no
	// page, database or data source to report.
	Backend string `json:"backend,omitempty"`
	PlanDir string `json:"plan_dir,omitempty"`
}

// projectCreatedMarkdown reports the project as created, saying the two things
// that were decided for the caller rather than by them: whether the Slices
// table tracks an assignee, and that the board is still on whatever it was.
func projectCreatedMarkdown(s *notion.ProjectStructure, name, workdir string, assignee bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", name)
	b.WriteString("Created, with its Slices database and an empty plan.\n\n")
	fmt.Fprintf(&b, "- Notion page: %s\n", s.PageID)
	if s.PageURL != "" {
		fmt.Fprintf(&b, "- Notion URL: %s\n", s.PageURL)
	}
	fmt.Fprintf(&b, "- Slices data source: %s\n", s.SlicesDSID)
	fmt.Fprintf(&b, "- Working directory: %s\n", workdir)
	fmt.Fprintf(&b, "- %s\n", assigneeNote(assignee))
	fmt.Fprintf(&b, "- %s\n", switchNote)
	return b.String()
}

// assigneeNote says which shape the Slices table was made in, since nothing on
// the command line asked for either.
func assigneeNote(assignee bool) string {
	if assignee {
		return "Slices track an assignee, since this machine's config names one."
	}
	return "Slices track no assignee: this machine's config names none, so status alone says whose turn it is."
}

// switchNote is the one thing left to do that this command will not do for you.
const switchNote = "Still not the active project — open the board and press P to switch to it."

// workspace resolves the config a command works in when it works on no project
// in particular: what is needed to create one is the projects database
// onboarding picked, not a plan already being tracked.
func (e Env) workspace() (config.Config, error) {
	cfg, found, err := e.Load()
	if err != nil {
		return cfg, err
	}
	if !found {
		return cfg, fmt.Errorf("no configuration yet: run `nat` once to set it up")
	}
	if cfg.ProjectDBDataSourceID == "" {
		return cfg, fmt.Errorf("no projects database is configured: run `nat` once to set it up")
	}
	return cfg, nil
}
