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

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/store"
)

// projectOpenFolder records a folder that already holds a local plan as a
// project this machine tracks — the starter card's "From filesystem" tile. It
// is `project-create --local` for a plan that exists: nothing is written to
// the plan, and the config entry is written only once a plan is found.
//
// What it looks for is the file a local project's plan is: `<slug of project
// ID>.db` inside the folder, holding a project row. A folder that has none is
// refused with what was looked for, so the caller can say it as it is. A
// folder whose project is already in config answers with that entry rather
// than a second one — the folder is where its plan lives, and a config that
// says otherwise is left as the user set it.
func projectOpenFolder(_ context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("project-open-folder", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageErrorf("project-open-folder: want exactly one folder, given %d", len(rest))
	}
	dir, err := absPlanDir(rest[0])
	if err != nil {
		return err
	}
	if dir == "" {
		return usageErrorf("project-open-folder: the folder is empty")
	}

	id, name, err := findFolderPlan(dir)
	if err != nil {
		return err
	}

	cfg, _, err := env.Load()
	if err != nil {
		return err
	}
	entry, known := cfg.Projects[id]
	if !known {
		entry = config.ProjectConfig{Name: name, Backend: config.BackendLocal, PlanDir: dir}
		if cfg.Projects == nil {
			cfg.Projects = map[string]config.ProjectConfig{}
		}
		cfg.Projects[id] = entry
		if err := env.Save(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		logging.Action("project opened", "project", id, "name", name, "backend", config.BackendLocal)
		env.nudged()
	}

	if *asJSON {
		return writeJSON(env.Out, projectCreatedJSON{Project: createdProjectJSON{
			ID: id, Name: entry.Name, WorkingDir: entry.WorkingDir, Backend: config.BackendLocal, PlanDir: entry.PlanDir,
		}})
	}
	_, err = io.WriteString(env.Out, localProjectOpenedMarkdown(id, entry))
	return err
}

// findFolderPlan is the one plan a folder holds: its ID and name. A folder
// with none is refused by saying what was looked for; one with several is
// refused too, since picking for the caller would open a project they may not
// have meant — nat keeps one plan file per folder it chooses.
func findFolderPlan(dir string) (id, name string, err error) {
	notFound := fmt.Errorf("no plan found in %s: looked for a nat plan file (<project id>.db holding a project)", dir)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", "", fmt.Errorf("%s is not a folder", dir)
	}
	// The pattern is escaped, so the one error Glob has (a malformed pattern)
	// cannot happen.
	matches, _ := filepath.Glob(filepath.Join(globEscape(dir), "*.db"))
	var found []string
	for _, path := range matches {
		pid, pname, perr := store.ReadLocalPlan(path)
		switch {
		case perr == nil:
			// A plan whose file is not named for its own project would open
			// as an empty one: the name is how every later command finds it.
			if filepath.Base(path) != store.PlanFileName(pid) {
				continue
			}
			id, name = pid, pname
			found = append(found, path)
		case errors.Is(perr, store.ErrNotAPlan):
		default:
			return "", "", perr
		}
	}
	switch len(found) {
	case 0:
		return "", "", notFound
	case 1:
		return id, name, nil
	}
	return "", "", fmt.Errorf("%d plans found in %s; open a folder that holds one", len(found), dir)
}

// globEscape makes a directory safe to sit in a Glob pattern.
func globEscape(dir string) string {
	return strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`).Replace(dir)
}

// localProjectOpenedMarkdown reports a folder's plan as opened.
func localProjectOpenedMarkdown(id string, entry config.ProjectConfig) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", entry.Name)
	b.WriteString("Opened from the plan already in that folder — no Notion workspace behind it.\n\n")
	fmt.Fprintf(&b, "- Project ID: %s\n", id)
	fmt.Fprintf(&b, "- Plan directory: %s\n", entry.PlanDir)
	if entry.WorkingDir != "" {
		fmt.Fprintf(&b, "- Working directory: %s\n", entry.WorkingDir)
	}
	return b.String()
}
