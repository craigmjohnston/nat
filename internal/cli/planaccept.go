package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/craigmjohnston/nat/internal/config"
	"github.com/craigmjohnston/nat/internal/logging"
)

// planProposal reads back the proposal file plan-propose wrote for a
// workspace or a project — the app's new-project tab, or a tracked project's
// own workshop, polls it on every nudge, so a proposal reaches the rail
// without the app knowing where nat keeps the file. No proposal yet is the
// ordinary state of a workshop that has not drafted anything, so it is
// answered as `{"proposal": null}` rather than as an error the app would
// have to tell apart from a real failure.
//
// A file that will not parse is an error, not a null: the app logs it and
// leaves the rail as it was, and the agent's next plan-propose replaces it.
func planProposal(_ context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("plan-proposal", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	_ = flags.Bool("json", false, "accepted for symmetry: the answer is always JSON")
	workspace := flags.String("workspace", "", "the app's own id for the new-project session (exclusive with --project)")
	projectRef := projectFlag(flags)
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("plan-proposal: takes no arguments, given %d", len(rest))
	}
	ws := strings.TrimSpace(*workspace)
	proj := strings.TrimSpace(*projectRef)
	if (ws == "") == (proj == "") {
		return usageErrorf("plan-proposal: give exactly one of --workspace or --project")
	}
	key := ws
	if key == "" {
		key = proj
	}
	doc, err := loadProposal(key)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return writeJSON(env.Out, proposalAnswer{})
	case err != nil:
		return err
	}
	return writeJSON(env.Out, proposalAnswer{Proposal: &doc})
}

// proposalAnswer is plan-proposal's output: the proposal, or null.
type proposalAnswer struct {
	Proposal *proposalDoc `json:"proposal"`
}

// loadProposal loads and parses one workspace's proposal file. The file was
// validated when it was written, so only its shape is checked again here.
func loadProposal(workspaceID string) (proposalDoc, error) {
	path, err := proposalPath(workspaceID)
	if err != nil {
		return proposalDoc{}, fmt.Errorf("resolve the proposal file: %w", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return proposalDoc{}, fmt.Errorf("read the proposal: %w", err)
	}
	var doc proposalDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return proposalDoc{}, fmt.Errorf("the proposal for workspace %s is not valid JSON: %w", workspaceID, err)
	}
	return doc, nil
}

// planAccept is the user's Accept on a proposal — exactly one of --workspace
// (a brand-new project, still being workshopped) or --project (a revision to
// a project already tracked) names which proposal and what accepting it does.
// It is the app's doing and not the agent's — the agent only ever proposes.
//
// --workspace makes a local project named --name, files the proposal's plan
// into it, and drops the proposal file, in that order: the project and its
// config entry exist before the plan is applied, and the proposal is dropped
// only once the plan is in. Its plan lives where every local project's does
// (nat's own data directory), and its working directory is left empty, as it
// is for a plan opened from a folder: the workshop's scratch directory held
// nothing but the agent's session, and where the project's code lives is
// Settings' to say. --name is required and --project is not given.
//
// --project applies the proposal's plan to a project that already exists —
// no --name, which the project already has — validated against that
// project's *current* plan with the same [validateAgainstProject] plan-apply
// and plan-propose both run, so a plan the live project has since outgrown
// (a milestone renamed, a slice the proposal's depends_on named since
// deleted) is refused here rather than half-applied. The proposal file is
// dropped only once the plan is in, same order as --workspace.
//
// A run that fails while applying the plan leaves the project (made or
// already there) and whatever was filed, and the proposal in place, and says
// so — the same no-rollback stance plan-apply takes.
func planAccept(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("plan-accept", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	workspace := flags.String("workspace", "", "the app's own id for the new-project session (exclusive with --project)")
	projectRef := projectFlag(flags)
	name := flags.String("name", "", "the new project's name (required with --workspace; refused with --project)")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("plan-accept: takes no arguments, given %d", len(rest))
	}
	ws := strings.TrimSpace(*workspace)
	proj := strings.TrimSpace(*projectRef)
	if (ws == "") == (proj == "") {
		return usageErrorf("plan-accept: give exactly one of --workspace or --project")
	}
	projectName := strings.TrimSpace(*name)
	if ws != "" {
		if projectName == "" {
			return usageErrorf("plan-accept: no --name given: the project's name")
		}
		return acceptIntoNewProject(ctx, env, ws, projectName, *asJSON)
	}
	if projectName != "" {
		return usageErrorf("plan-accept: --name is not given with --project: the project already has one")
	}
	return acceptIntoProject(ctx, env, proj, *asJSON)
}

// acceptIntoNewProject is planAccept's --workspace half: see planAccept's own
// doc comment for the order and why.
func acceptIntoNewProject(ctx context.Context, env Env, ws, projectName string, asJSON bool) error {
	// Everything that can be refused is refused before the project is made.
	doc, err := loadProposal(ws)
	if err != nil {
		return err
	}
	// A new project has no milestones or slices of its own, so the targets
	// resolved here are the ones applying it needs.
	targets, err := validatePlan(doc.Plan, nil, nil)
	if err != nil {
		return err
	}

	id, _, err := createLocalProject(ctx, env, projectName, "", "", "")
	if err != nil {
		return err
	}
	_, projectID, project, err := env.projectFor(id)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}
	sp := storeProject(projectID, project)
	shape, err := st.Shape(ctx, sp)
	if err != nil {
		return err
	}
	applied, err := applyPlan(ctx, st, sp, shape, doc.Plan, targets, shape.Milestones)
	env.nudged()
	if err != nil {
		return err
	}

	// The plan is in: the proposal has done its job. A file that will not go is
	// no reason to fail an accept that has already succeeded.
	if path, perr := proposalPath(ws); perr == nil {
		if rerr := os.Remove(path); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			logging.Action("proposal not removed", "workspace", ws, "error", rerr.Error())
		}
	}
	logging.Action("plan accepted", "workspace", ws, "project", projectID,
		"milestones", len(applied.Milestones), "slices", len(applied.Slices))

	if asJSON {
		return writeJSON(env.Out, planAcceptedJSON{
			Project:    createdProjectJSON{ID: projectID, Name: projectName, Backend: config.BackendLocal, PlanDir: project.PlanDir},
			Milestones: len(applied.Milestones),
			Slices:     len(applied.Slices),
		})
	}
	_, err = fmt.Fprintf(env.Out, "Accepted %s as %q (project %s).\n",
		counts(len(applied.Milestones), len(applied.Slices)), projectName, projectID)
	return err
}

// acceptIntoProject is planAccept's --project half: see planAccept's own doc
// comment for the order and why.
func acceptIntoProject(ctx context.Context, env Env, projectRef string, asJSON bool) error {
	_, projectID, project, err := env.projectFor(projectRef)
	if err != nil {
		return err
	}
	doc, err := loadProposal(projectID)
	if err != nil {
		return err
	}
	st, err := env.storeFor(ctx, projectID, project)
	if err != nil {
		return err
	}
	sp := storeProject(projectID, project)
	// Validated against the project's *current* plan, not the plan as it
	// stood when the proposal was written — a plan the project has since
	// outgrown is refused here, not half-applied.
	shape, targets, err := validateAgainstProject(ctx, st, sp, doc.Plan)
	if err != nil {
		return err
	}

	applied, err := applyPlan(ctx, st, sp, shape, doc.Plan, targets, shape.Milestones)
	// Nudge only once something was actually written, the same rule plan-apply
	// itself applies.
	if len(applied.Milestones) > 0 || len(applied.Slices) > 0 || len(applied.Dependencies) > 0 {
		env.nudged()
	}
	if err != nil {
		return err
	}

	// The plan is in: the proposal has done its job. A file that will not go is
	// no reason to fail an accept that has already succeeded.
	if path, perr := proposalPath(projectID); perr == nil {
		if rerr := os.Remove(path); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			logging.Action("proposal not removed", "project", projectID, "error", rerr.Error())
		}
	}
	logging.Action("plan accepted", "project", projectID,
		"milestones", len(applied.Milestones), "slices", len(applied.Slices))

	if asJSON {
		return writeJSON(env.Out, planAcceptedJSON{
			Project: createdProjectJSON{
				ID: projectID, Name: project.Name, WorkingDir: project.WorkingDir,
				SlicesDSID: project.SlicesDSID, Assignee: shape.HasAssignee,
				Backend: project.Backend, PlanDir: project.PlanDir,
			},
			Milestones: len(applied.Milestones),
			Slices:     len(applied.Slices),
		})
	}
	_, err = fmt.Fprintf(env.Out, "Accepted %s into %q (project %s).\n",
		counts(len(applied.Milestones), len(applied.Slices)), project.Name, projectID)
	return err
}

// planAcceptedJSON is plan-accept's structured output: the project as
// project-create --local reports it, and how much of the plan went in.
type planAcceptedJSON struct {
	Project    createdProjectJSON `json:"project"`
	Milestones int                `json:"milestones"`
	Slices     int                `json:"slices"`
}
