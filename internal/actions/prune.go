package actions

import (
	"context"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/logging"
	"github.com/craigmjohnston/nat/internal/store"
)

// MilestonePruner is what [PruneEmptied] asks of a store: a fresh read of the
// plan, the removal itself, and the shape read again between removals.
type MilestonePruner interface {
	Plan(ctx context.Context, p store.Project) (store.Plan, error)
	Shape(ctx context.Context, p store.Project) (store.Shape, error)
	RemoveMilestone(ctx context.Context, p store.Project, sh store.Shape, name string) (domain.Milestone, error)
}

// PruneEmptied removes each milestone a write just took a slice out of — named
// by the ID the slice was filed under — where a fresh read of the plan finds no
// slice of any status still filed under it, and answers with the names of the
// ones removed, in plan order. It runs only after the slice write succeeded,
// and only over the milestones that write left: a milestone empty before it is
// none of this.
//
// Nothing here fails the caller. The slice write is what was asked for, so a
// plan read or a removal that fails is logged and that milestone simply stays,
// for removing by hand. A source project is skipped outright — its containers
// are the plugin's — and an empty ID (a slice under no milestone) has nothing
// to prune.
func PruneEmptied(ctx context.Context, st MilestonePruner, sp store.Project, left ...string) []string {
	if sp.Source != "" {
		return nil
	}
	want := map[string]bool{}
	for _, id := range left {
		if id != "" {
			want[id] = true
		}
	}
	if len(want) == 0 {
		return nil
	}
	plan, err := st.Plan(ctx, sp)
	if err != nil {
		logging.Action("could not read the plan to prune emptied milestones", "project", sp.ID, "err", err)
		return nil
	}
	for _, s := range plan.Project.Slices {
		delete(want, s.MilestoneID)
	}
	shape := plan.Shape
	var removed []string
	for _, m := range plan.Project.Milestones {
		if !want[m.ID] {
			continue
		}
		if removed != nil {
			// The shape is a read of the milestones as they stood before the
			// last removal; read the next one rather than hand back a stale list.
			if shape, err = st.Shape(ctx, sp); err != nil {
				logging.Action("could not re-read the shape to prune emptied milestones", "project", sp.ID, "err", err)
				return removed
			}
		}
		if _, err := st.RemoveMilestone(ctx, sp, shape, m.Name); err != nil {
			logging.Action("could not remove an emptied milestone", "project", sp.ID, "milestone", m.Name, "err", err)
			continue
		}
		logging.Action("emptied milestone removed", "project", sp.ID, "milestone", m.Name)
		removed = append(removed, m.Name)
	}
	return removed
}
