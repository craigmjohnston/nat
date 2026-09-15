package actions

import (
	"context"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/store"
)

// PlanReader is what a planning launch needs to render the current plan
// inline into its own prompt: the same two reads `nat info` makes. Narrower
// than [Store], since a planning launch neither claims nor writes anything.
type PlanReader interface {
	Plan(ctx context.Context, p store.Project) (store.Plan, error)
	Body(ctx context.Context, id string) (string, error)
}

// RenderedPlan is a planning launch's read of the current plan, rendered the
// same way `nat info` prints it — see [domain.PlanMarkdown] — so its prompt
// can carry the plan inline instead of telling the agent to run that command
// itself.
//
// The two reads fail on their own: a plan that could not be read at all
// gives back "", which falls the prompt back to the bare nat info
// instruction, and conventions that failed to read leave the rest of the
// document standing with nothing where they would have gone — the project's
// usual reads-conclude-nothing posture, so a launch never fails over this.
func RenderedPlan(ctx context.Context, r PlanReader, sp store.Project) string {
	plan, err := r.Plan(ctx, sp)
	if err != nil {
		logging.Action("could not read the plan for a planning launch's prompt", "project", sp.ID, "err", err)
		return ""
	}
	conventions, err := r.Body(ctx, sp.ID)
	if err != nil {
		logging.Action("could not read the project conventions for a planning launch's prompt", "project", sp.ID, "err", err)
		conventions = ""
	}
	return domain.PlanMarkdown(plan.Project, conventions)
}
