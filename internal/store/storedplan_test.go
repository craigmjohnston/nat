package store

import (
	"context"
	"testing"
	"time"

	"github.com/craigmjohnston/nat/internal/notion"
)

// StoredPlan on a Mirrored never pulls even once the file's copy has gone
// stale — the one thing that tells it apart from Plan itself, which would
// re-pull against the fake API this test means to leave untouched.
func TestStoredPlanNeverPullsAStaleMirrored(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)
	stampHydrated(t, l)
	write(t, l, `UPDATE project SET synced_at = ? WHERE id = ?`,
		timeStamp(time.Now().Add(-time.Hour)), "proj")
	m := Mirror(l, Over(&fakeAPI{}), Project{ID: "proj"})

	plan, err := StoredPlan(context.Background(), m, Project{ID: "proj"})
	if err != nil {
		t.Fatalf("StoredPlan: %v", err)
	}
	if len(plan.Project.Slices) == 0 {
		t.Error("Plan = empty, want the file's own plan read despite being stale")
	}
}

// A Mirrored never hydrated at all is still hydrated once, the one case
// StoredPlan cannot skip: there is nothing in the file yet for it to read.
func TestStoredPlanHydratesAMirroredNeverPulled(t *testing.T) {
	l, _ := openPlan(t)
	api := fullPlanAPI()
	m := Mirror(l, Over(api), Project{ID: "proj"})

	plan, err := StoredPlan(context.Background(), m, Project{ID: "proj"})
	if err != nil {
		t.Fatalf("StoredPlan: %v", err)
	}
	if len(plan.Project.Slices) == 0 {
		t.Error("Plan = empty, want the first hydrate's plan")
	}
}

// A failed first hydrate is StoredPlan's own failure to report — there is no
// file copy yet to fall back on.
func TestStoredPlanReportsAFailedFirstHydrate(t *testing.T) {
	l, _ := openPlan(t)
	api := &fakeAPI{dataSource: func(string) (*notion.DataSource, error) { return nil, errBoom }}
	m := Mirror(l, Over(api), Project{ID: "proj"})

	if _, err := StoredPlan(context.Background(), m, Project{ID: "proj"}); err == nil {
		t.Fatal("StoredPlan = nil error, want the failed hydrate reported")
	}
}

// Any store other than a Mirrored has no staleness rule to skip, so StoredPlan
// answers Store.Plan unchanged — a Local plan of its own, which is the one
// other backend tracked today.
func TestStoredPlanDelegatesForAnyOtherStore(t *testing.T) {
	l, _ := openPlan(t)
	fillPlan(t, l)

	plan, err := StoredPlan(context.Background(), l, Project{ID: "proj"})
	if err != nil {
		t.Fatalf("StoredPlan: %v", err)
	}
	if len(plan.Project.Slices) == 0 {
		t.Error("Plan = empty, want the local store's own plan")
	}
}
