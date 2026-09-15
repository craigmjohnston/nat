package actions

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/store"
)

// fakePlanReader stands in for a store's Plan and Body reads, which is all a
// planning launch's [RenderedPlan] asks of one.
type fakePlanReader struct {
	plan        store.Plan
	planErr     error
	conventions string
	bodyErr     error
}

var _ PlanReader = (*fakePlanReader)(nil)

func (f *fakePlanReader) Plan(ctx context.Context, p store.Project) (store.Plan, error) {
	return f.plan, f.planErr
}

func (f *fakePlanReader) Body(ctx context.Context, id string) (string, error) {
	return f.conventions, f.bodyErr
}

// A successful read of both the plan and the conventions renders the same
// document `nat info` prints.
func TestRenderedPlanRendersThePlanAndConventions(t *testing.T) {
	r := &fakePlanReader{
		plan:        store.Plan{Project: domain.Project{Name: "nat"}},
		conventions: "Go, Bubble Tea v2.",
	}
	got := RenderedPlan(context.Background(), r, store.Project{ID: "p1", Name: "nat"})
	if want := "Go, Bubble Tea v2."; !strings.Contains(got, want) {
		t.Errorf("plan = %q, want it to carry the conventions %q", got, want)
	}
	if !strings.Contains(got, "# nat") {
		t.Errorf("plan = %q, want the project name as a heading", got)
	}
}

// A plan that could not be read at all gives back "" — the caller's prompt
// falls back to naming nat info instead, rather than the launch failing.
func TestRenderedPlanGivesBackNothingWhenThePlanFailsToRead(t *testing.T) {
	r := &fakePlanReader{planErr: errors.New("notion: 500")}
	if got := RenderedPlan(context.Background(), r, store.Project{ID: "p1"}); got != "" {
		t.Errorf("plan = %q, want \"\" when the plan itself could not be read", got)
	}
}

// Conventions that failed to read leave the rest of the document standing,
// with nothing where they would have gone.
func TestRenderedPlanRendersWithoutConventionsWhenTheyFailToRead(t *testing.T) {
	r := &fakePlanReader{
		plan:    store.Plan{Project: domain.Project{Name: "nat"}},
		bodyErr: errors.New("notion: 500"),
	}
	got := RenderedPlan(context.Background(), r, store.Project{ID: "p1"})
	if got == "" {
		t.Fatal("plan = \"\", want the milestones and slices to still render")
	}
	if !strings.Contains(got, "# nat") {
		t.Errorf("plan = %q, want the project name as a heading", got)
	}
}
