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
// workspace — the app's new-project tab polls it on every nudge, so a proposal
// reaches the rail without the app knowing where nat keeps the file. No
// proposal yet is the ordinary state of a workshop that has not drafted
// anything, so it is answered as `{"proposal": null}` rather than as an error
// the app would have to tell apart from a real failure.
//
// A file that will not parse is an error, not a null: the app logs it and
// leaves the rail as it was, and the agent's next plan-propose replaces it.
func planProposal(_ context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("plan-proposal", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	_ = flags.Bool("json", false, "accepted for symmetry: the answer is always JSON")
	workspace := flags.String("workspace", "", "the app's own id for the new-project session (required)")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("plan-proposal: takes no arguments, given %d", len(rest))
	}
	ws := strings.TrimSpace(*workspace)
	if ws == "" {
		return usageErrorf("plan-proposal: no --workspace given: the app's own id for this session, with no fallback")
	}
	doc, err := loadProposal(ws)
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

// planAccept is the user's Accept on a proposal: it makes a local project
// named --name, files the proposal's plan into it, and drops the proposal
// file. It is the app's doing and not the agent's — the agent only ever
// proposes — and it is one command so the order is nat's: the project and its
// config entry exist before the plan is applied, and the proposal is dropped
// only once the plan is in.
//
// The project's plan lives where every local project's does (nat's own data
// directory), and its working directory is left empty, as it is for a plan
// opened from a folder: the workshop's scratch directory held nothing but the
// agent's session, and where the project's code lives is Settings' to say.
//
// A run that fails while applying the plan leaves the project and whatever
// was filed, and the proposal in place, and says so — the same no-rollback
// stance plan-apply takes.
func planAccept(ctx context.Context, args []string, env Env) error {
	flags := flag.NewFlagSet("plan-accept", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print structured JSON instead of markdown")
	workspace := flags.String("workspace", "", "the app's own id for the new-project session (required)")
	name := flags.String("name", "", "the project's name (required)")
	rest, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return usageErrorf("plan-accept: takes no arguments, given %d", len(rest))
	}
	ws := strings.TrimSpace(*workspace)
	if ws == "" {
		return usageErrorf("plan-accept: no --workspace given: the app's own id for this session, with no fallback")
	}
	projectName := strings.TrimSpace(*name)
	if projectName == "" {
		return usageErrorf("plan-accept: no --name given: the project's name")
	}

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

	if *asJSON {
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

// planAcceptedJSON is plan-accept's structured output: the project as
// project-create --local reports it, and how much of the plan went in.
type planAcceptedJSON struct {
	Project    createdProjectJSON `json:"project"`
	Milestones int                `json:"milestones"`
	Slices     int                `json:"slices"`
}
