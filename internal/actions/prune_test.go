package actions

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/craigmjohnston/nat/internal/domain"
	"github.com/craigmjohnston/nat/internal/store"
)

// fakePruner answers a plan read from its own milestones and slices and
// records each removal it is asked for, dropping the milestone from what the
// next read sees.
type fakePruner struct {
	milestones []domain.Milestone
	slices     []domain.Slice
	planErr    error
	shapeErr   error
	removeErr  map[string]error
	planReads  int
	shapeReads int
	removed    []string
}

var _ MilestonePruner = (*fakePruner)(nil)

func (f *fakePruner) Plan(context.Context, store.Project) (store.Plan, error) {
	f.planReads++
	return store.Plan{Project: domain.Project{Milestones: f.milestones, Slices: f.slices}}, f.planErr
}

func (f *fakePruner) Shape(context.Context, store.Project) (store.Shape, error) {
	f.shapeReads++
	return store.Shape{}, f.shapeErr
}

func (f *fakePruner) RemoveMilestone(_ context.Context, _ store.Project, _ store.Shape, name string) (domain.Milestone, error) {
	if err := f.removeErr[name]; err != nil {
		return domain.Milestone{}, err
	}
	f.removed = append(f.removed, name)
	return domain.Milestone{Name: name}, nil
}

func pruneFixture() *fakePruner {
	return &fakePruner{
		milestones: []domain.Milestone{
			{ID: "M1", Name: "M1"}, {ID: "M2", Name: "M2"}, {ID: "M3", Name: "M3"}, {ID: "M4", Name: "M4"},
		},
		// M2 holds only a Done slice, which still keeps it; M4 was empty
		// already, and is never named as left.
		slices: []domain.Slice{{ID: "s1", MilestoneID: "M2", Status: domain.SliceDone}},
	}
}

// Every milestone named as left that a fresh read finds empty is removed, in
// plan order, and one still holding a slice of any status stays.
func TestPruneEmptiedRemovesOnlyTheEmptiedOnes(t *testing.T) {
	f := pruneFixture()
	got := PruneEmptied(context.Background(), f, store.Project{ID: "p"}, "M3", "M2", "M1", "M1")
	if want := []string{"M1", "M3"}; !reflect.DeepEqual(got, want) || !reflect.DeepEqual(f.removed, want) {
		t.Errorf("removed = %v (store saw %v), want %v", got, f.removed, want)
	}
	if f.planReads != 1 || f.shapeReads != 1 {
		t.Errorf("plan reads = %d, shape reads = %d, want 1 and 1 (the shape re-read between removals)", f.planReads, f.shapeReads)
	}
}

// A source project, an empty ID and no IDs at all each read nothing and
// remove nothing.
func TestPruneEmptiedSkipsWithoutReading(t *testing.T) {
	for name, tc := range map[string]struct {
		sp   store.Project
		left []string
	}{
		"source project": {store.Project{ID: "p", Source: "shortcut"}, []string{"M1"}},
		"no milestone":   {store.Project{ID: "p"}, []string{""}},
		"nothing left":   {store.Project{ID: "p"}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			f := pruneFixture()
			if got := PruneEmptied(context.Background(), f, tc.sp, tc.left...); got != nil {
				t.Errorf("removed = %v, want nothing", got)
			}
			if f.planReads != 0 || f.removed != nil {
				t.Errorf("plan reads = %d, removals = %v, want none", f.planReads, f.removed)
			}
		})
	}
}

// A plan read that fails concludes nothing: no removal is attempted.
func TestPruneEmptiedPlanReadFailureRemovesNothing(t *testing.T) {
	f := pruneFixture()
	f.planErr = errors.New("notion: 500")
	if got := PruneEmptied(context.Background(), f, store.Project{ID: "p"}, "M1"); got != nil || f.removed != nil {
		t.Errorf("removed = %v (store saw %v), want nothing", got, f.removed)
	}
}

// A removal that fails is logged and passed over; the others still go.
func TestPruneEmptiedRemovalFailureIsPassedOver(t *testing.T) {
	f := pruneFixture()
	f.removeErr = map[string]error{"M1": errors.New("notion: 409")}
	got := PruneEmptied(context.Background(), f, store.Project{ID: "p"}, "M1", "M3")
	if want := []string{"M3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("removed = %v, want %v", got, want)
	}
}

// A shape re-read that fails stops the pruning with what was removed so far.
func TestPruneEmptiedShapeReadFailureStops(t *testing.T) {
	f := pruneFixture()
	f.shapeErr = errors.New("notion: 500")
	got := PruneEmptied(context.Background(), f, store.Project{ID: "p"}, "M1", "M3")
	if want := []string{"M1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("removed = %v, want %v", got, want)
	}
}
