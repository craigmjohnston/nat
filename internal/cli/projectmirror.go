package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/notion"
	"github.com/craigmjohnston/nat/internal/store"
)

// projectMirror puts a project of nat's own into Notion: it makes the project
// page under the place --parent names, files the local plan into the Slices
// database beneath it, and re-registers the project as one tracked in Notion —
// which is what "mirrored" is, since every project with a workspace behind it
// keeps a local replica of its plan and writes through to that workspace.
//
// The project's ID changes. A project in Notion is known by its page's ID and
// its slices by their pages' IDs, and a project of nat's own has neither, so
// the mirror is a new entry in config under the page's ID, carrying the name and
// working directory over, and the old entry is removed once the plan is in. The
// old plan file is left where it is: nothing here deletes a plan.
//
// Only a plan nobody has started is mirrored. A slice already claimed, handed
// back or done has a branch, a pull request or a worktree named by the ID it has
// now, and none of that would follow it to a page; the command refuses it by
// name rather than filing a plan that lost its history.
//
// Everything that can be refused is refused before Notion is touched. After
// that there is no rollback, in the same stance plan-apply takes: a run that
// fails while filing leaves the project page and what was filed, with the new
// entry in config beside the old one, and says so — the local project is
// untouched until the plan is entirely in.
func projectMirror(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("project-mirror", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	projectRef := projectFlag(flags)
	parentID := flags.String("parent", "", "the page, or database's data source, to put the project page under (required)")
	parentKind := flags.String("parent-kind", "", "whether --parent is a `page` or a `database` (required)")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("project-mirror: takes no arguments, given %d", len(rest))
	}
	parent, err := mirrorParent(strings.TrimSpace(*parentID), strings.TrimSpace(*parentKind))
	if err != nil {
		return err
	}

	cfg, oldID, project, err := env.projectFor(*projectRef)
	if err != nil {
		return err
	}
	if !project.IsLocal() {
		return fmt.Errorf("project-mirror: %q is already tracked in Notion: only a project of nat's own is mirrored", project.Name)
	}
	oldStore, err := env.storeFor(ctx, oldID, project)
	if err != nil {
		return err
	}
	sp := storeProject(oldID, project)
	local, err := oldStore.Plan(ctx, sp)
	if err != nil {
		return fmt.Errorf("read the plan: %w", err)
	}
	if err := refuseStarted(local.Project.Slices); err != nil {
		return err
	}
	// The conventions are the prose kept against the project's own ID, a brief
	// the prose kept against a slice's.
	prose := make(map[string]string, len(local.Project.Slices)+1)
	ids := []string{oldID}
	for _, s := range local.Project.Slices {
		ids = append(ids, s.ID)
	}
	for _, id := range ids {
		if prose[id], err = oldStore.Body(ctx, id); err != nil {
			return fmt.Errorf("read the plan's prose: %w", err)
		}
	}

	client := env.NewClient(env.Tokens.Token)
	assignee := cfg.AssigneeUserID != ""
	s, err := client.CreateProjectIn(ctx, parent, project.Name, assignee)
	switch {
	case s != nil:
		// As project-create has it: a structure with an error is a project that
		// exists but whose schema did not read back as expected. It is recorded
		// rather than orphaned, and the error reported at the end.
	case err != nil:
		return fmt.Errorf("create the project page: %w", err)
	default:
		return errors.New("create the project page: no project was returned")
	}
	logging.Action("project created", "project", s.PageID, "name", project.Name, "mirrored", oldID)

	mirrored := config.ProjectConfig{Name: project.Name, SlicesDSID: s.SlicesDSID, WorkingDir: project.WorkingDir}
	// The entry goes in before the plan does, so a plan that fails partway is one
	// a later command can be pointed at rather than pages nothing knows about.
	fresh, _, err := env.Load()
	if err != nil {
		return err
	}
	if fresh.Projects == nil {
		fresh.Projects = map[string]config.ProjectConfig{}
	}
	fresh.Projects[s.PageID] = mirrored
	if err := env.Save(fresh); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if blocks := notion.BlocksFromMarkdown(prose[oldID]); len(blocks) > 0 {
		if err := appendPageBody(ctx, client, s.PageID, blocks); err != nil {
			return mirrorHalf(s, err)
		}
	}
	filed, err := fileLocalPlan(ctx, env, client, s.PageID, mirrored, local.Project, prose)
	env.nudged()
	if err != nil {
		return mirrorHalf(s, err)
	}

	// The plan is entirely in Notion: the local entry has done its job. The
	// active project follows the project it became, so the board is not left
	// pointing at an entry that no longer exists.
	delete(fresh.Projects, oldID)
	if fresh.ActiveProjectID == oldID {
		fresh.ActiveProjectID = s.PageID
	}
	if err := env.Save(fresh); err != nil {
		return mirrorHalf(s, fmt.Errorf("save config: %w", err))
	}
	env.nudged()
	logging.Action("project mirrored", "from", oldID, "project", s.PageID,
		"milestones", len(local.Project.Milestones), "slices", filed)

	if *asJSON {
		return writeJSON(env.Out, projectMirroredJSON{
			Project: createdProjectJSON{
				ID: s.PageID, Name: project.Name, URL: s.PageURL, SlicesDBID: s.SlicesDBID,
				SlicesDSID: s.SlicesDSID, WorkingDir: project.WorkingDir, Assignee: assignee,
			},
			Replaced: oldID, Milestones: len(local.Project.Milestones), Slices: filed,
		})
	}
	_, err = fmt.Fprintf(env.Out, "Mirrored %q to Notion as project %s: %s.\n%s\n",
		project.Name, s.PageID, counts(len(local.Project.Milestones), filed), s.PageURL)
	return err
}

// projectMirroredJSON is project-mirror's structured output: the project as
// project-create reports it, the ID of the local project it replaces, and how
// much of the plan went in.
type projectMirroredJSON struct {
	Project    createdProjectJSON `json:"project"`
	Replaced   string             `json:"replaced"`
	Milestones int                `json:"milestones"`
	Slices     int                `json:"slices"`
}

// mirrorParent turns what --parent and --parent-kind said into the parent a
// project page is created under.
func mirrorParent(id, kind string) (notion.Parent, error) {
	if id == "" {
		return notion.Parent{}, usageErrorf("project-mirror: no --parent given: the page or database to put the project page under")
	}
	switch kind {
	case parentKindPage:
		return notion.PageParent(id), nil
	case parentKindDatabase:
		return notion.DataSourceParent(id), nil
	}
	return notion.Parent{}, usageErrorf("project-mirror: --parent-kind is %q, want %q or %q", kind, parentKindPage, parentKindDatabase)
}

// refuseStarted refuses a plan any of whose slices has left Todo or carries a
// branch or pull request, naming the first.
func refuseStarted(slices []domain.Slice) error {
	for _, s := range slices {
		if s.Status != domain.SliceTodo || s.Branch != "" || s.PRURL != "" {
			return fmt.Errorf("project-mirror: %q has been started (%s): only a plan nobody has begun is mirrored, "+
				"since its history would not follow it to Notion", s.Name, s.StatusName)
		}
	}
	return nil
}

// mirrorHalf is the error of a mirror that stopped after the project page was
// made, saying what exists so the plan can be finished or the page removed.
func mirrorHalf(s *notion.ProjectStructure, err error) error {
	return fmt.Errorf("the project page %s was created, but the plan did not all reach it (the local project is untouched): %w",
		s.PageID, err)
}

// fileLocalPlan writes a local plan into a freshly made Notion project through
// its ordinary store: the milestones in plan order, then the slices in the
// order they were on the board, then the dependencies once every slice exists to
// be named. It returns how many slices were filed.
func fileLocalPlan(ctx context.Context, env Env, client API, projectID string, entry config.ProjectConfig, p domain.Project, briefs map[string]string) (int, error) {
	st, err := env.storeFor(ctx, projectID, entry)
	if err != nil {
		return 0, err
	}
	remote := store.Over(client)
	sp := storeProject(projectID, entry)
	names := make([]string, len(p.Milestones))
	for i, m := range p.Milestones {
		names[i] = m.Name
	}
	// The shape is read off the file, so it fails only where the file cannot be
	// read at all, and says the same thing as a milestone the workspace refused.
	shape, err := st.Shape(ctx, sp)
	var added []domain.Milestone
	if err == nil {
		added, err = st.AddMilestones(ctx, sp, shape, names)
	}
	if err != nil {
		return 0, fmt.Errorf("create the milestones: %w", err)
	}
	byName := make(map[string]domain.Milestone, len(added))
	for _, m := range added {
		byName[m.Name] = m
	}
	// A milestone is what a slice's Milestone column names, so the local
	// milestone's ID maps to the new one through its name.
	newMilestone := map[string]domain.Milestone{}
	for _, m := range p.Milestones {
		newMilestone[m.ID] = byName[m.Name]
	}

	newID := make(map[string]string, len(p.Slices))
	filed := 0
	for _, s := range p.Slices {
		made, err := st.AddSlice(ctx, sp, store.NewSlice{
			Title: s.Name, Brief: briefs[s.ID], Repo: s.Repo, Milestone: newMilestone[s.MilestoneID],
		})
		if err != nil {
			return filed, fmt.Errorf("create the slice %q (%d of %d): %w", s.Name, filed+1, len(p.Slices), err)
		}
		newID[s.ID] = made.ID
		filed++
	}
	for _, s := range p.Slices {
		if len(s.DependsOn) == 0 {
			continue
		}
		on := make([]string, len(s.DependsOn))
		for i, d := range s.DependsOn {
			on[i] = newID[d]
		}
		// The plan's own store writes the file first and pushes second, swallowing
		// a push that fails so the file can send it later — right for a plan being
		// worked, wrong here, where nothing would ever say a relation never
		// reached Notion. So the workspace is written to directly first, where a
		// failure is an error, and the store's own write then only records it.
		if _, err := remote.SetDependencies(ctx, newID[s.ID], on); err != nil {
			return filed, fmt.Errorf("record what %q waits on: %w", s.Name, err)
		}
		if _, err := st.SetDependencies(ctx, newID[s.ID], on); err != nil {
			return filed, fmt.Errorf("record what %q waits on: %w", s.Name, err)
		}
	}
	return filed, nil
}
