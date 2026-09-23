package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
)

// homeDir is where a scratch project's sessions start when --dir names nothing.
// Held as a variable so a test can stand in for the one edge of it that cannot
// otherwise be reached.
var homeDir = os.UserHomeDir

// scratchName is what the reserved project is called.
const scratchName = "Scratch"

// scratchConventions is the page body the scratch project's plan carries. A
// scratch project has no conventions to speak of; this only says what it is.
const scratchConventions = "Scratch: ad hoc work with no plan set up first. A local project like any other."

// scratchOpen makes sure the reserved scratch project exists and prints its ID.
// The first call creates it through the same path as `project-create --local`
// and records it in config under scratch_project; every later call only reads
// that back, so the app can run it on every launch. --dir picks the working
// directory on creation only — once the project exists it is config-set's to
// change, and a flag that silently did nothing is reported as such by not
// being consulted at all.
func scratchOpen(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("scratch-open", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	dir := flags.String("dir", "", "the working directory of the scratch project, on creation only; defaults to the home directory")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("scratch-open: takes no positional arguments, given %d", len(rest))
	}

	cfg, _, err := env.Load()
	if err != nil {
		return err
	}
	id, created := cfg.ScratchProject, false
	if _, ok := cfg.Projects[id]; id == "" || !ok {
		workdir := strings.TrimSpace(*dir)
		if workdir == "" {
			if workdir, err = homeDir(); err != nil {
				return fmt.Errorf("resolve the home directory: %w", err)
			}
		}
		if id, _, err = createLocalProject(ctx, env, scratchName, scratchConventions, workdir, ""); err != nil {
			return err
		}
		// Reload rather than reuse cfg: createLocalProject saved the new project
		// into the file, and writing the stale copy back would drop it.
		if cfg, _, err = env.Load(); err != nil {
			return err
		}
		cfg.ScratchProject = id
		if err := env.Save(cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		created = true
	}

	if *asJSON {
		return writeJSON(env.Out, scratchOpenJSON{ID: id, Created: created})
	}
	_, err = fmt.Fprintln(env.Out, id)
	return err
}

// scratchOpenJSON is the structured form of scratch-open.
type scratchOpenJSON struct {
	ID      string `json:"id"`
	Created bool   `json:"created"`
}

// doneClear empties a local project of what is finished: every Done slice,
// every ended ad hoc session, and then every milestone left holding no slices.
// It exists for the scratch project, where finished work is clutter and not a
// record — and refuses any project with a workspace behind it, because there
// Done slices are the record of the work and this command must never be able
// to trash Notion pages. It refuses nothing else: in-progress and Todo slices,
// and sessions still running, are left exactly as they are.
func doneClear(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("done-clear", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("done-clear: takes no positional arguments, given %d", len(rest))
	}

	_, projectID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	if !project.IsLocal() {
		return fmt.Errorf("done-clear: %q has a workspace behind it, and its Done slices are the record of the work — only a project kept in a local file is cleared", project.Name)
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}
	sp := storeProject(projectID, project)

	plan, err := st.Plan(ctx, sp)
	if err != nil {
		return fmt.Errorf("read the plan: %w", err)
	}
	out := doneClearJSON{Slices: []string{}, Sessions: []string{}, Milestones: []string{}}
	for _, s := range plan.Project.Slices {
		if s.Status != domain.SliceDone {
			continue
		}
		if err := st.DeleteSlice(ctx, s.ID); err != nil {
			return fmt.Errorf("delete slice %q: %w", s.Name, err)
		}
		out.Slices = append(out.Slices, s.Name)
	}

	sessions, err := st.Sessions(ctx, sp)
	if err != nil {
		return fmt.Errorf("read the sessions: %w", err)
	}
	for _, s := range sessions {
		if !s.Ended() {
			continue
		}
		if err := st.DeleteSession(ctx, s.ID); err != nil {
			return fmt.Errorf("delete session %s: %w", s.ID, err)
		}
		out.Sessions = append(out.Sessions, s.ID)
	}

	// Milestones are judged after the deletes, on a fresh read: one whose only
	// slices were Done is empty now, and one that was empty already goes too.
	plan, err = st.Plan(ctx, sp)
	if err != nil {
		return fmt.Errorf("re-read the plan: %w", err)
	}
	filed := map[string]bool{}
	for _, s := range plan.Project.Slices {
		filed[s.MilestoneID] = true
	}
	shape := plan.Shape
	for _, m := range plan.Project.Milestones {
		if filed[m.ID] {
			continue
		}
		if _, err := st.RemoveMilestone(ctx, sp, shape, m.Name); err != nil {
			return fmt.Errorf("remove milestone %q: %w", m.Name, err)
		}
		out.Milestones = append(out.Milestones, m.Name)
		// The shape is a read of the milestones as they stood; read the next
		// one from the store rather than hand back a list that is now stale.
		if shape, err = st.Shape(ctx, sp); err != nil {
			return fmt.Errorf("re-read the shape: %w", err)
		}
	}
	sort.Strings(out.Slices)
	logging.Action("done cleared", "project", projectID,
		"slices", len(out.Slices), "sessions", len(out.Sessions), "milestones", len(out.Milestones))
	env.nudged()

	if *asJSON {
		return writeJSON(env.Out, out)
	}
	_, err = io.WriteString(env.Out, doneClearMarkdown(project.Name, out))
	return err
}

// doneClearJSON is what a done-clear removed, by name for the slices and
// milestones and by ID for the sessions, which have no name.
type doneClearJSON struct {
	Slices     []string `json:"slices"`
	Sessions   []string `json:"sessions"`
	Milestones []string `json:"milestones"`
}

// doneClearMarkdown reports what was removed.
func doneClearMarkdown(project string, r doneClearJSON) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", project)
	fmt.Fprintf(&b, "Removed %d Done slices, %d ended sessions and %d empty milestones.\n", len(r.Slices), len(r.Sessions), len(r.Milestones))
	for _, group := range []struct {
		title string
		names []string
	}{{"Slices", r.Slices}, {"Sessions", r.Sessions}, {"Milestones", r.Milestones}} {
		if len(group.names) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n", group.title)
		for _, n := range group.names {
			fmt.Fprintf(&b, "- %s\n", n)
		}
	}
	return b.String()
}
